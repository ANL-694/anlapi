package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type OAuthCredentialIsolationReport struct {
	Accounts            int
	SensitiveRows       int
	MarkerRows          int
	MissingVaultEntries int
	MigratedRows        int
	ReclaimedVersions   int
	CompletedDeletes    int
	PendingDeletes      int
	PendingWrites       int
}

type oauthCredentialMigrationRow struct {
	id          int64
	credentials map[string]any
	raw         []byte
}

// MigrateOAuthCredentialsToVault moves bearer credentials out of the
// replicated accounts table. The table lock prevents token refreshes from
// racing the one-time migration; every write is tracked for compensating
// deletion if the business transaction later rolls back.
func MigrateOAuthCredentialsToVault(ctx context.Context, db *sql.DB, vault service.OAuthCredentialVault) (*OAuthCredentialIsolationReport, error) {
	if db == nil {
		return nil, errors.New("business database is required")
	}
	if vault == nil || vault.Mode() != service.OAuthCredentialVaultModeExternal {
		return nil, errors.New("external OAuth credential Vault is required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin OAuth credential migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	written := make([]struct {
		key     service.OAuthCredentialVaultKey
		version string
	}, 0)
	committed := false
	commitAttempted := false
	defer func() {
		if !shouldDiscardOAuthVaultWrite(committed, commitAttempted) {
			return
		}
		janitor, ok := vault.(service.OAuthCredentialVaultVersionJanitor)
		if !ok {
			return
		}
		for _, entry := range written {
			recoveryCtx, cancel := oauthVaultRecoveryContext(context.Background())
			_ = janitor.DeleteVersion(recoveryCtx, entry.key, entry.version)
			cancel()
		}
	}()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, fmt.Errorf("lock accounts for OAuth credential migration: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, credentials
		FROM accounts
		WHERE deleted_at IS NULL AND type IN ('oauth', 'setup-token')
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("read OAuth credential accounts: %w", err)
	}
	migrationRows, err := scanOAuthCredentialMigrationRows(rows)
	if err != nil {
		return nil, err
	}
	report := &OAuthCredentialIsolationReport{Accounts: len(migrationRows)}
	for _, row := range migrationRows {
		persisted, sensitive, hasSensitive := service.SplitOAuthCredentials(row.credentials)
		if !hasSensitive {
			if service.OAuthCredentialVaultVersion(persisted) != "" {
				report.MarkerRows++
			}
			continue
		}
		report.SensitiveRows++
		version, err := vault.Put(ctx, service.OAuthCredentialVaultKey{AccountID: row.id}, sensitive)
		if err != nil {
			return nil, fmt.Errorf("write OAuth credential Vault entry for account %d: %w", row.id, err)
		}
		written = append(written, struct {
			key     service.OAuthCredentialVaultKey
			version string
		}{key: service.OAuthCredentialVaultKey{AccountID: row.id}, version: version})
		persisted = service.SetOAuthCredentialVaultVersion(persisted, version)
		payload, err := json.Marshal(persisted)
		if err != nil {
			return nil, fmt.Errorf("encode sanitized OAuth credentials for account %d: %w", row.id, err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE accounts SET credentials = $1::jsonb, updated_at = NOW()
			WHERE id = $2 AND credentials = $3::jsonb
		`, string(payload), row.id, string(row.raw))
		if err != nil {
			return nil, fmt.Errorf("sanitize OAuth credentials for account %d: %w", row.id, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("read OAuth migration result for account %d: %w", row.id, err)
		}
		if affected != 1 {
			return nil, fmt.Errorf("OAuth credential account %d changed during migration", row.id)
		}
		report.MigratedRows++
		report.MarkerRows++
	}
	commitAttempted = true
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit OAuth credential migration: %w", err)
	}
	committed = true
	coordinator, ok := vault.(service.OAuthCredentialVaultWriteCoordinator)
	if !ok {
		return nil, errors.New("external OAuth credential Vault does not support version activation")
	}
	for _, entry := range written {
		if err := coordinator.ActivateVersion(ctx, entry.key, entry.version); err != nil {
			return nil, fmt.Errorf("activate migrated OAuth credential Vault entry for account %d: %w", entry.key.AccountID, err)
		}
	}
	verified, err := AuditOAuthCredentialIsolation(ctx, db, vault)
	if err != nil {
		return nil, err
	}
	verified.MigratedRows = report.MigratedRows
	if verified.SensitiveRows != 0 || verified.MissingVaultEntries != 0 {
		return verified, fmt.Errorf("OAuth credential isolation verification failed: sensitive_rows=%d missing_vault_entries=%d", verified.SensitiveRows, verified.MissingVaultEntries)
	}
	return verified, nil
}

func AuditOAuthCredentialIsolation(ctx context.Context, db *sql.DB, vault service.OAuthCredentialVault) (*OAuthCredentialIsolationReport, error) {
	if db == nil {
		return nil, errors.New("business database is required")
	}
	if vault == nil || vault.Mode() != service.OAuthCredentialVaultModeExternal {
		return nil, errors.New("external OAuth credential Vault is required")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, credentials
		FROM accounts
		WHERE deleted_at IS NULL AND type IN ('oauth', 'setup-token')
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("read OAuth credential accounts for audit: %w", err)
	}
	auditRows, err := scanOAuthCredentialMigrationRows(rows)
	if err != nil {
		return nil, err
	}
	report := &OAuthCredentialIsolationReport{Accounts: len(auditRows)}
	for _, row := range auditRows {
		_, _, hasSensitive := service.SplitOAuthCredentials(row.credentials)
		if hasSensitive {
			report.SensitiveRows++
		}
		version := service.OAuthCredentialVaultVersion(row.credentials)
		if version == "" {
			if !hasSensitive {
				report.MissingVaultEntries++
			}
			continue
		}
		report.MarkerRows++
		if _, err := vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: row.id}, version); err != nil {
			if errors.Is(err, service.ErrOAuthCredentialVaultEntryNotFound) {
				report.MissingVaultEntries++
				continue
			}
			return nil, fmt.Errorf("verify OAuth credential Vault entry for account %d: %w", row.id, err)
		}
	}
	return report, nil
}

// ReconcileOAuthCredentialVault removes only versions that are no longer
// referenced by an OAuth account and have outlived the grace period. It also
// completes durable deletion tombstones for accounts that no longer exist, or
// cancels stale tombstones when the business transaction never committed.
func ReconcileOAuthCredentialVault(ctx context.Context, db *sql.DB, vault service.OAuthCredentialVault, olderThan time.Time) (*OAuthCredentialIsolationReport, error) {
	if db == nil {
		return nil, errors.New("business database is required")
	}
	janitor, ok := vault.(service.OAuthCredentialVaultVersionJanitor)
	if !ok {
		return nil, errors.New("OAuth credential Vault does not support reconciliation")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, credentials
		FROM accounts
		WHERE deleted_at IS NULL AND type IN ('oauth', 'setup-token')`)
	if err != nil {
		return nil, fmt.Errorf("read OAuth credential accounts for reconciliation: %w", err)
	}
	accounts, err := scanOAuthCredentialMigrationRows(rows)
	if err != nil {
		return nil, err
	}
	references := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		references[account.id] = service.OAuthCredentialVaultVersion(account.credentials)
	}
	cleanup, err := janitor.CleanupUnreferenced(ctx, references, olderThan)
	if err != nil {
		return nil, fmt.Errorf("reconcile OAuth credential Vault: %w", err)
	}
	report, err := AuditOAuthCredentialIsolation(ctx, db, vault)
	if err != nil {
		return nil, err
	}
	report.ReclaimedVersions = cleanup.ReclaimedVersions
	report.CompletedDeletes = cleanup.CompletedDeletes
	report.PendingDeletes = cleanup.PendingDeletes
	report.PendingWrites = cleanup.PendingWrites
	return report, nil
}

func scanOAuthCredentialMigrationRows(rows *sql.Rows) ([]oauthCredentialMigrationRow, error) {
	defer func() { _ = rows.Close() }()
	result := make([]oauthCredentialMigrationRow, 0)
	for rows.Next() {
		var row oauthCredentialMigrationRow
		if err := rows.Scan(&row.id, &row.raw); err != nil {
			return nil, fmt.Errorf("scan OAuth credential account: %w", err)
		}
		if err := json.Unmarshal(row.raw, &row.credentials); err != nil {
			return nil, fmt.Errorf("decode OAuth credentials for account %d: %w", row.id, err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate OAuth credential accounts: %w", err)
	}
	return result, nil
}
