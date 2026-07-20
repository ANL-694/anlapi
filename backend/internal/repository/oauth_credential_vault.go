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

	_ "github.com/lib/pq"
	"ikik-api/internal/config"
	"ikik-api/internal/service"
)

const oauthCredentialVaultSchema = `
CREATE TABLE IF NOT EXISTS oauth_credential_vault (
	account_id BIGINT NOT NULL,
	owner_user_id BIGINT NOT NULL DEFAULT 0,
	version TEXT NOT NULL,
	payload TEXT NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (account_id, owner_user_id, version)
);
DO $$
DECLARE
	primary_key_name TEXT;
	primary_key_columns INTEGER;
BEGIN
	SELECT conname, cardinality(conkey)
	INTO primary_key_name, primary_key_columns
	FROM pg_constraint
	WHERE conrelid = 'oauth_credential_vault'::regclass
		AND contype = 'p';
	IF primary_key_name IS NOT NULL AND primary_key_columns <> 3 THEN
		EXECUTE format('ALTER TABLE oauth_credential_vault DROP CONSTRAINT %I', primary_key_name);
		primary_key_name := NULL;
	END IF;
	IF primary_key_name IS NULL THEN
		ALTER TABLE oauth_credential_vault
			ADD PRIMARY KEY (account_id, owner_user_id, version);
	END IF;
END $$;
CREATE INDEX IF NOT EXISTS oauth_credential_vault_updated_at_idx
	ON oauth_credential_vault (updated_at);`

type oauthCredentialVault struct {
	db                  *sql.DB
	key                 []byte
	mode                service.OAuthCredentialVaultMode
	allowLegacyFallback bool
}

type noopOAuthCredentialVault struct {
	mode service.OAuthCredentialVaultMode
}

// NewOAuthCredentialVault creates the independent OAuth store. Legacy and
// disabled nodes deliberately avoid opening a second database connection.
func NewOAuthCredentialVault(cfg *config.Config) (service.OAuthCredentialVault, error) {
	if cfg == nil {
		return nil, errors.New("nil config for OAuth credential Vault")
	}
	mode := service.OAuthCredentialVaultMode(strings.ToLower(strings.TrimSpace(cfg.OAuthVault.Mode)))
	if mode == "" {
		mode = service.OAuthCredentialVaultModeLegacy
	}
	if mode != service.OAuthCredentialVaultModeExternal {
		return &noopOAuthCredentialVault{mode: mode}, nil
	}

	key, err := hex.DecodeString(cfg.OAuthVault.EncryptionKey)
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
	if _, err := db.ExecContext(ctx, oauthCredentialVaultSchema); err != nil {
		return closeOnError(fmt.Errorf("initialize OAuth credential Vault schema: %w", err))
	}
	return &oauthCredentialVault{
		db:                  db,
		key:                 key,
		mode:                mode,
		allowLegacyFallback: cfg.OAuthVault.AllowLegacyFallback,
	}, nil
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
	ownerID := oauthVaultOwnerID(key)
	var encrypted string
	query := `SELECT payload FROM oauth_credential_vault WHERE account_id = $1 AND owner_user_id = $2`
	args := []any{key.AccountID, ownerID}
	if version != "" {
		query += ` AND version = $3`
		args = append(args, version)
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
	if err := json.Unmarshal([]byte(plaintext), &payload); err != nil {
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
		INSERT INTO oauth_credential_vault (account_id, owner_user_id, version, payload, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, key.AccountID, oauthVaultOwnerID(key), version, encrypted)
	if err != nil {
		return "", fmt.Errorf("write OAuth credential Vault entry: %w", err)
	}
	return version, nil
}

func (v *oauthCredentialVault) Delete(ctx context.Context, key service.OAuthCredentialVaultKey) error {
	if v == nil || v.db == nil {
		return nil
	}
	_, err := v.db.ExecContext(ctx,
		`DELETE FROM oauth_credential_vault WHERE account_id = $1 AND owner_user_id = $2`,
		key.AccountID, oauthVaultOwnerID(key))
	return err
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

func oauthVaultOwnerID(key service.OAuthCredentialVaultKey) int64 {
	if key.OwnerUserID == nil || *key.OwnerUserID <= 0 {
		return 0
	}
	return *key.OwnerUserID
}

func oauthVaultAAD(key service.OAuthCredentialVaultKey) []byte {
	return []byte(fmt.Sprintf("oauth-vault:%d:%d", key.AccountID, oauthVaultOwnerID(key)))
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

func (v *noopOAuthCredentialVault) LegacyFallbackEnabled() bool { return true }

func (v *noopOAuthCredentialVault) Get(context.Context, service.OAuthCredentialVaultKey, string) (map[string]any, error) {
	return nil, service.ErrOAuthCredentialVaultEntryNotFound
}

func (v *noopOAuthCredentialVault) Put(context.Context, service.OAuthCredentialVaultKey, map[string]any) (string, error) {
	return "", errors.New("OAuth credential Vault is not configured")
}

func (v *noopOAuthCredentialVault) Delete(context.Context, service.OAuthCredentialVaultKey) error {
	return nil
}
func (v *noopOAuthCredentialVault) Close() error { return nil }
