package repository

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
)

const oauthCredentialVaultSchema = `
CREATE TABLE IF NOT EXISTS oauth_credential_vault (
	account_id BIGINT NOT NULL,
	version TEXT NOT NULL,
	payload TEXT NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	unreferenced_since TIMESTAMPTZ NULL,
	state TEXT NOT NULL DEFAULT 'active',
	PRIMARY KEY (account_id, version)
);
ALTER TABLE oauth_credential_vault
	ADD COLUMN IF NOT EXISTS unreferenced_since TIMESTAMPTZ NULL;
ALTER TABLE oauth_credential_vault
	ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'active';
CREATE INDEX IF NOT EXISTS oauth_credential_vault_updated_at_idx
	ON oauth_credential_vault (updated_at);
CREATE TABLE IF NOT EXISTS oauth_credential_vault_delete_pending (
	account_id BIGINT PRIMARY KEY,
	requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

type oauthCredentialVault struct {
	db                  *sql.DB
	key                 []byte
	mode                service.OAuthCredentialVaultMode
	allowLegacyFallback bool
}

type noopOAuthCredentialVault struct {
	mode                service.OAuthCredentialVaultMode
	allowLegacyFallback bool
}

// NewOAuthCredentialVault creates the independent OAuth store. Legacy and
// disabled modes deliberately avoid opening a second database connection.
func NewOAuthCredentialVault(cfg *config.Config) (service.OAuthCredentialVault, error) {
	if cfg == nil {
		return nil, errors.New("nil config for OAuth credential Vault")
	}
	mode := service.OAuthCredentialVaultMode(strings.ToLower(strings.TrimSpace(cfg.OAuthVault.Mode)))
	if mode == "" {
		mode = service.OAuthCredentialVaultModeLegacy
	}
	if mode != service.OAuthCredentialVaultModeExternal {
		return &noopOAuthCredentialVault{mode: mode, allowLegacyFallback: cfg.OAuthVault.AllowLegacyFallback}, nil
	}
	key, err := hex.DecodeString(strings.TrimSpace(cfg.OAuthVault.EncryptionKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("invalid OAuth credential Vault encryption key")
	}
	db, err := sql.Open("postgres", cfg.OAuthVault.DSN)
	if err != nil {
		return nil, fmt.Errorf("open OAuth credential Vault database: %w", err)
	}
	closeOnError := func(cause error) (service.OAuthCredentialVault, error) {
		_ = db.Close()
		return nil, cause
	}
	if cfg.OAuthVault.ConnMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.OAuthVault.ConnMaxLifetimeMinutes) * time.Minute)
	}
	db.SetMaxOpenConns(cfg.OAuthVault.MaxOpenConns)
	db.SetMaxIdleConns(cfg.OAuthVault.MaxIdleConns)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("ping OAuth credential Vault database: %w", err))
	}
	if err := verifyOAuthVaultDatabaseIsolation(ctx, cfg, db); err != nil {
		return closeOnError(err)
	}
	if _, err := db.ExecContext(ctx, oauthCredentialVaultSchema); err != nil {
		return closeOnError(fmt.Errorf("initialize OAuth credential Vault schema: %w", err))
	}
	return &oauthCredentialVault{db: db, key: key, mode: mode, allowLegacyFallback: cfg.OAuthVault.AllowLegacyFallback}, nil
}

type postgresDatabaseIdentity struct {
	SystemID string
	Address  string
	Port     int
	Database string
}

func verifyOAuthVaultDatabaseIsolation(ctx context.Context, cfg *config.Config, vaultDB *sql.DB) error {
	businessDB, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		return fmt.Errorf("open business database for OAuth Vault isolation check: %w", err)
	}
	defer func() { _ = businessDB.Close() }()
	vaultIdentity, err := loadPostgresDatabaseIdentity(ctx, vaultDB)
	if err != nil {
		return fmt.Errorf("identify OAuth credential Vault database: %w", err)
	}
	businessIdentity, err := loadPostgresDatabaseIdentity(ctx, businessDB)
	if err != nil {
		return fmt.Errorf("identify business database for OAuth Vault isolation check: %w", err)
	}
	if samePostgresDatabase(vaultIdentity, businessIdentity) {
		return errors.New("OAuth credential Vault resolves to the business PostgreSQL database")
	}
	return nil
}

func samePostgresDatabase(left, right postgresDatabaseIdentity) bool {
	return left.SystemID != "" && right.SystemID != "" &&
		left.SystemID == right.SystemID && left.Database == right.Database
}

func loadPostgresDatabaseIdentity(ctx context.Context, db *sql.DB) (postgresDatabaseIdentity, error) {
	var identity postgresDatabaseIdentity
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(inet_server_addr()::text, ''), inet_server_port(), current_database()
	`).Scan(&identity.Address, &identity.Port, &identity.Database); err != nil {
		return postgresDatabaseIdentity{}, err
	}
	if err := db.QueryRowContext(ctx, `SELECT system_identifier::text FROM pg_control_system()`).Scan(&identity.SystemID); err != nil {
		return postgresDatabaseIdentity{}, fmt.Errorf("read PostgreSQL system identifier: %w", err)
	}
	if strings.TrimSpace(identity.SystemID) == "" {
		return postgresDatabaseIdentity{}, errors.New("PostgreSQL system identifier is empty")
	}
	return identity, nil
}

func (v *oauthCredentialVault) Mode() service.OAuthCredentialVaultMode {
	if v == nil {
		return service.OAuthCredentialVaultModeLegacy
	}
	return v.mode
}

func (v *oauthCredentialVault) LegacyFallbackEnabled() bool {
	return v != nil && v.allowLegacyFallback
}

func (v *oauthCredentialVault) Get(ctx context.Context, key service.OAuthCredentialVaultKey, version string) (map[string]any, error) {
	if v == nil || v.db == nil {
		return nil, service.ErrOAuthCredentialVaultEntryNotFound
	}
	var encrypted string
	query := `SELECT payload FROM oauth_credential_vault WHERE account_id = $1`
	args := []any{key.AccountID}
	if strings.TrimSpace(version) != "" {
		query += ` AND version = $2`
		args = append(args, strings.TrimSpace(version))
	} else {
		query += ` ORDER BY updated_at DESC LIMIT 1`
	}
	err := v.db.QueryRowContext(ctx, query, args...).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrOAuthCredentialVaultEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read OAuth credential Vault entry: %w", err)
	}
	plaintext, err := v.decrypt(encrypted, key)
	if err != nil {
		return nil, fmt.Errorf("decrypt OAuth credential Vault entry: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("decode OAuth credential Vault entry: %w", err)
	}
	return payload, nil
}

func (v *oauthCredentialVault) Put(ctx context.Context, key service.OAuthCredentialVaultKey, payload map[string]any) (string, error) {
	if v == nil || v.db == nil {
		return "", errors.New("OAuth credential Vault is not configured")
	}
	if key.AccountID <= 0 {
		return "", errors.New("OAuth credential Vault account ID must be positive")
	}
	version, err := newOAuthVaultVersion()
	if err != nil {
		return "", err
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode OAuth credential Vault payload: %w", err)
	}
	encrypted, err := v.encrypt(plaintext, key)
	if err != nil {
		return "", fmt.Errorf("encrypt OAuth credential Vault payload: %w", err)
	}
	_, err = v.db.ExecContext(ctx, `
		INSERT INTO oauth_credential_vault (account_id, version, payload, updated_at, state)
		VALUES ($1, $2, $3, NOW(), 'pending')
	`, key.AccountID, version, encrypted)
	if err != nil {
		return "", fmt.Errorf("write OAuth credential Vault entry: %w", err)
	}
	return version, nil
}

func (v *oauthCredentialVault) ActivateVersion(ctx context.Context, key service.OAuthCredentialVaultKey, version string) error {
	if v == nil || v.db == nil {
		return errors.New("OAuth credential Vault is not configured")
	}
	result, err := v.db.ExecContext(ctx, `
		UPDATE oauth_credential_vault
		SET state = 'active', unreferenced_since = NULL
		WHERE account_id = $1 AND version = $2
	`, key.AccountID, strings.TrimSpace(version))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return service.ErrOAuthCredentialVaultEntryNotFound
	}
	return nil
}

func (v *oauthCredentialVault) Delete(ctx context.Context, key service.OAuthCredentialVaultKey) error {
	if v == nil || v.db == nil {
		return nil
	}
	_, err := v.db.ExecContext(ctx, `DELETE FROM oauth_credential_vault WHERE account_id = $1`, key.AccountID)
	return err
}

func (v *oauthCredentialVault) BeginDelete(ctx context.Context, key service.OAuthCredentialVaultKey) error {
	if v == nil || v.db == nil {
		return errors.New("OAuth credential Vault is not configured")
	}
	if key.AccountID <= 0 {
		return errors.New("OAuth credential Vault account ID must be positive")
	}
	_, err := v.db.ExecContext(ctx, `
		INSERT INTO oauth_credential_vault_delete_pending (account_id, requested_at)
		VALUES ($1, NOW())
		ON CONFLICT (account_id) DO UPDATE SET requested_at = EXCLUDED.requested_at
	`, key.AccountID)
	return err
}

func (v *oauthCredentialVault) CancelDelete(ctx context.Context, key service.OAuthCredentialVaultKey) error {
	if v == nil || v.db == nil {
		return errors.New("OAuth credential Vault is not configured")
	}
	_, err := v.db.ExecContext(ctx, `DELETE FROM oauth_credential_vault_delete_pending WHERE account_id = $1`, key.AccountID)
	return err
}

func (v *oauthCredentialVault) CompleteDelete(ctx context.Context, key service.OAuthCredentialVaultKey) error {
	if v == nil || v.db == nil {
		return errors.New("OAuth credential Vault is not configured")
	}
	tx, err := v.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_credential_vault WHERE account_id = $1`, key.AccountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_credential_vault_delete_pending WHERE account_id = $1`, key.AccountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (v *oauthCredentialVault) DeleteVersion(ctx context.Context, key service.OAuthCredentialVaultKey, version string) error {
	if v == nil || v.db == nil || strings.TrimSpace(version) == "" {
		return nil
	}
	_, err := v.db.ExecContext(ctx, `DELETE FROM oauth_credential_vault WHERE account_id = $1 AND version = $2`, key.AccountID, strings.TrimSpace(version))
	return err
}

func (v *oauthCredentialVault) CleanupUnreferenced(ctx context.Context, references map[int64]string, olderThan time.Time) (service.OAuthCredentialVaultCleanupReport, error) {
	report := service.OAuthCredentialVaultCleanupReport{}
	if v == nil || v.db == nil {
		return report, errors.New("OAuth credential Vault is not configured")
	}
	rows, err := v.db.QueryContext(ctx, `
		SELECT account_id, version, unreferenced_since, state
		FROM oauth_credential_vault
	`)
	if err != nil {
		return report, err
	}
	type candidate struct {
		accountID         int64
		version           string
		unreferencedSince sql.NullTime
		state             string
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var accountID int64
		var version string
		var unreferencedSince sql.NullTime
		var state string
		if err := rows.Scan(&accountID, &version, &unreferencedSince, &state); err != nil {
			_ = rows.Close()
			return report, err
		}
		candidates = append(candidates, candidate{accountID: accountID, version: version, unreferencedSince: unreferencedSince, state: state})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return report, err
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	for _, candidate := range candidates {
		if references[candidate.accountID] == candidate.version {
			if candidate.state != "active" || candidate.unreferencedSince.Valid {
				if _, err := v.db.ExecContext(ctx, `
					UPDATE oauth_credential_vault
					SET state = 'active', unreferenced_since = NULL
					WHERE account_id = $1 AND version = $2
				`, candidate.accountID, candidate.version); err != nil {
					return report, err
				}
			}
			continue
		}
		if candidate.state != "active" {
			report.PendingWrites++
			continue
		}
		if !candidate.unreferencedSince.Valid {
			if _, err := v.db.ExecContext(ctx, `
				UPDATE oauth_credential_vault
				SET unreferenced_since = NOW()
				WHERE account_id = $1 AND version = $2 AND unreferenced_since IS NULL
			`, candidate.accountID, candidate.version); err != nil {
				return report, err
			}
			continue
		}
		if !candidate.unreferencedSince.Time.Before(olderThan) {
			continue
		}
		result, err := v.db.ExecContext(ctx, `
			DELETE FROM oauth_credential_vault
			WHERE account_id = $1 AND version = $2 AND unreferenced_since < $3
		`, candidate.accountID, candidate.version, olderThan)
		if err != nil {
			return report, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return report, err
		}
		report.ReclaimedVersions += int(count)
	}
	pendingRows, err := v.db.QueryContext(ctx, `
		SELECT account_id
		FROM oauth_credential_vault_delete_pending
		WHERE requested_at < $1
	`, olderThan)
	if err != nil {
		return report, err
	}
	pendingIDs := make([]int64, 0)
	for pendingRows.Next() {
		var accountID int64
		if err := pendingRows.Scan(&accountID); err != nil {
			_ = pendingRows.Close()
			return report, err
		}
		pendingIDs = append(pendingIDs, accountID)
	}
	if err := pendingRows.Close(); err != nil {
		return report, err
	}
	for _, accountID := range pendingIDs {
		if _, exists := references[accountID]; exists {
			if _, err := v.db.ExecContext(ctx, `DELETE FROM oauth_credential_vault_delete_pending WHERE account_id = $1`, accountID); err != nil {
				return report, err
			}
			continue
		}
		if err := v.CompleteDelete(ctx, service.OAuthCredentialVaultKey{AccountID: accountID}); err != nil {
			return report, err
		}
		report.CompletedDeletes++
	}
	if err := v.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM oauth_credential_vault_delete_pending`).Scan(&report.PendingDeletes); err != nil {
		return report, err
	}
	return report, nil
}

func (v *oauthCredentialVault) Close() error {
	if v == nil || v.db == nil {
		return nil
	}
	return v.db.Close()
}

func (v *oauthCredentialVault) encrypt(plaintext []byte, key service.OAuthCredentialVaultKey) (string, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, oauthVaultAAD(key))
	return base64.RawStdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (v *oauthCredentialVault) decrypt(encoded string, key service.OAuthCredentialVaultKey) ([]byte, error) {
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("OAuth credential Vault ciphertext is too short")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], oauthVaultAAD(key))
}

func oauthVaultAAD(key service.OAuthCredentialVaultKey) []byte {
	return []byte(fmt.Sprintf("oauth-vault:%d", key.AccountID))
}

func newOAuthVaultVersion() (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", fmt.Errorf("generate OAuth credential Vault version: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

func (v *noopOAuthCredentialVault) Mode() service.OAuthCredentialVaultMode {
	if v == nil || v.mode == "" {
		return service.OAuthCredentialVaultModeLegacy
	}
	return v.mode
}

func (v *noopOAuthCredentialVault) LegacyFallbackEnabled() bool {
	return v != nil && v.allowLegacyFallback
}

func (v *noopOAuthCredentialVault) Get(context.Context, service.OAuthCredentialVaultKey, string) (map[string]any, error) {
	return nil, service.ErrOAuthCredentialVaultEntryNotFound
}

func (v *noopOAuthCredentialVault) Put(context.Context, service.OAuthCredentialVaultKey, map[string]any) (string, error) {
	if v != nil && v.mode == service.OAuthCredentialVaultModeDisabled {
		return "", service.ErrOAuthCredentialVaultDisabled
	}
	return "", errors.New("OAuth credential Vault is not configured")
}
func (v *noopOAuthCredentialVault) ActivateVersion(context.Context, service.OAuthCredentialVaultKey, string) error {
	return errors.New("OAuth credential Vault is not configured")
}

func (v *noopOAuthCredentialVault) Delete(context.Context, service.OAuthCredentialVaultKey) error {
	return nil
}
func (v *noopOAuthCredentialVault) BeginDelete(context.Context, service.OAuthCredentialVaultKey) error {
	return errors.New("OAuth credential Vault is not configured")
}
func (v *noopOAuthCredentialVault) CancelDelete(context.Context, service.OAuthCredentialVaultKey) error {
	return errors.New("OAuth credential Vault is not configured")
}
func (v *noopOAuthCredentialVault) CompleteDelete(context.Context, service.OAuthCredentialVaultKey) error {
	return errors.New("OAuth credential Vault is not configured")
}
func (v *noopOAuthCredentialVault) DeleteVersion(context.Context, service.OAuthCredentialVaultKey, string) error {
	return nil
}
func (v *noopOAuthCredentialVault) CleanupUnreferenced(context.Context, map[int64]string, time.Time) (service.OAuthCredentialVaultCleanupReport, error) {
	return service.OAuthCredentialVaultCleanupReport{}, errors.New("OAuth credential Vault is not configured")
}
func (v *noopOAuthCredentialVault) Close() error { return nil }
