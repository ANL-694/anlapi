package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"anl-api/internal/service"
)

type OAuthCredentialIsolationReport struct {
	Accounts            int
	SensitiveRows       int
	MarkerRows          int
	MissingVaultEntries int
	MigratedRows        int
}

type oauthCredentialMigrationRow struct {
	id          int64
	ownerUserID *int64
	credentials map[string]any
	raw         []byte
}

// MigrateOAuthCredentialsToVault moves bearer credentials out of the
// replicated accounts table. The table lock prevents token refreshes from
// racing the one-time migration; append-only Vault versions keep an orphaned
// write harmless if the business transaction later rolls back.
func MigrateOAuthCredentialsToVault(
	ctx context.Context,
	db *sql.DB,
	vault service.OAuthCredentialVault,
) (*OAuthCredentialIsolationReport, error) {
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

	if _, err := tx.ExecContext(ctx, `LOCK TABLE accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, fmt.Errorf("lock accounts for OAuth credential migration: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, owner_user_id, credentials
		FROM accounts
		WHERE deleted_at IS NULL
			AND type IN ('oauth', 'setup-token')
		ORDER BY id ASC
	`)
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
		version, err := vault.Put(ctx, oauthMigrationVaultKey(row.id, row.ownerUserID), sensitive)
		if err != nil {
			return nil, fmt.Errorf("write OAuth credential Vault entry for account %d: %w", row.id, err)
		}
		persisted = service.SetOAuthCredentialVaultVersion(persisted, version)
		payload, err := json.Marshal(persisted)
		if err != nil {
			return nil, fmt.Errorf("encode sanitized OAuth credentials for account %d: %w", row.id, err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET credentials = $1::jsonb,
				updated_at = NOW()
			WHERE id = $2
				AND owner_user_id IS NOT DISTINCT FROM $3
				AND credentials = $4::jsonb
		`, string(payload), row.id, nullableOwnerUserID(row.ownerUserID), string(row.raw))
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

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit OAuth credential migration: %w", err)
	}
	verified, err := AuditOAuthCredentialIsolation(ctx, db, vault)
	if err != nil {
		return nil, err
	}
	verified.MigratedRows = report.MigratedRows
	if verified.SensitiveRows != 0 || verified.MissingVaultEntries != 0 {
		return verified, fmt.Errorf(
			"OAuth credential isolation verification failed: sensitive_rows=%d missing_vault_entries=%d",
			verified.SensitiveRows,
			verified.MissingVaultEntries,
		)
	}
	return verified, nil
}

func AuditOAuthCredentialIsolation(
	ctx context.Context,
	db *sql.DB,
	vault service.OAuthCredentialVault,
) (*OAuthCredentialIsolationReport, error) {
	if db == nil {
		return nil, errors.New("business database is required")
	}
	if vault == nil || vault.Mode() != service.OAuthCredentialVaultModeExternal {
		return nil, errors.New("external OAuth credential Vault is required")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_user_id, credentials
		FROM accounts
		WHERE deleted_at IS NULL
			AND type IN ('oauth', 'setup-token')
		ORDER BY id ASC
	`)
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
			continue
		}
		report.MarkerRows++
		if _, err := vault.Get(ctx, oauthMigrationVaultKey(row.id, row.ownerUserID), version); err != nil {
			if errors.Is(err, service.ErrOAuthCredentialVaultEntryNotFound) {
				report.MissingVaultEntries++
				continue
			}
			return nil, fmt.Errorf("verify OAuth credential Vault entry for account %d: %w", row.id, err)
		}
	}
	return report, nil
}

func scanOAuthCredentialMigrationRows(rows *sql.Rows) ([]oauthCredentialMigrationRow, error) {
	defer func() { _ = rows.Close() }()
	result := make([]oauthCredentialMigrationRow, 0)
	for rows.Next() {
		var (
			row     oauthCredentialMigrationRow
			ownerID sql.NullInt64
		)
		if err := rows.Scan(&row.id, &ownerID, &row.raw); err != nil {
			return nil, fmt.Errorf("scan OAuth credential account: %w", err)
		}
		if ownerID.Valid && ownerID.Int64 > 0 {
			value := ownerID.Int64
			row.ownerUserID = &value
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

func oauthMigrationVaultKey(accountID int64, ownerUserID *int64) service.OAuthCredentialVaultKey {
	return service.OAuthCredentialVaultKey{AccountID: accountID, OwnerUserID: ownerUserID}
}

func nullableOwnerUserID(ownerUserID *int64) any {
	if ownerUserID == nil {
		return nil
	}
	return *ownerUserID
}
