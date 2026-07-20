package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"anlapi/internal/service"
	"github.com/lib/pq"
)

func (r *accountRepository) oauthVaultMode() service.OAuthCredentialVaultMode {
	if r == nil || r.oauthVault == nil {
		return service.OAuthCredentialVaultModeLegacy
	}
	return r.oauthVault.Mode()
}

func (r *accountRepository) oauthVaultKey(accountID int64, ownerUserID *int64) service.OAuthCredentialVaultKey {
	var owner *int64
	if ownerUserID != nil && *ownerUserID > 0 {
		value := *ownerUserID
		owner = &value
	}
	return service.OAuthCredentialVaultKey{AccountID: accountID, OwnerUserID: owner}
}

// prepareOAuthCredentialsForWrite stores the sensitive subset first and
// returns the sanitized document that is safe for the replicated database.
// A version marker makes a failed SQL write harmless: the newer Vault row is
// ignored until the main database points at that version.
func (r *accountRepository) prepareOAuthCredentialsForWrite(
	ctx context.Context,
	accountID int64,
	ownerUserID *int64,
	accountType string,
	credentials map[string]any,
) (map[string]any, error) {
	if !service.IsOAuthCredentialAccount(accountType) {
		return service.MergeOAuthCredentials(credentials, nil), nil
	}
	if r.oauthVaultMode() == service.OAuthCredentialVaultModeDisabled {
		return nil, service.ErrOAuthCredentialVaultDisabled
	}
	if r.oauthVaultMode() != service.OAuthCredentialVaultModeExternal {
		return service.MergeOAuthCredentials(credentials, nil), nil
	}
	if r.oauthVault == nil {
		return nil, errors.New("OAuth credential Vault is required in external mode")
	}
	persisted, sensitive, hasSensitive := service.SplitOAuthCredentials(credentials)
	if !hasSensitive {
		return persisted, nil
	}
	version, err := r.oauthVault.Put(ctx, r.oauthVaultKey(accountID, ownerUserID), sensitive)
	if err != nil {
		return nil, err
	}
	return service.SetOAuthCredentialVaultVersion(persisted, version), nil
}

func (r *accountRepository) rejectDisabledOAuthMutation(ctx context.Context, accountIDs []int64) error {
	if r.oauthVaultMode() != service.OAuthCredentialVaultModeDisabled || len(accountIDs) == 0 {
		return nil
	}
	if r == nil || r.sql == nil {
		return errors.New("account repository SQL executor is not configured")
	}
	var exists bool
	rows, err := r.sql.QueryContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM accounts
			WHERE id = ANY($1)
				AND deleted_at IS NULL
				AND type IN ('oauth', 'setup-token')
		)
	`, pq.Array(accountIDs))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("OAuth mutation guard returned no result")
	}
	if err := rows.Scan(&exists); err != nil {
		return err
	}
	if exists {
		return service.ErrOAuthCredentialVaultDisabled
	}
	return nil
}

func (r *accountRepository) persistedOAuthCredentials(credentials map[string]any) map[string]any {
	persisted, _, _ := service.SplitOAuthCredentials(credentials)
	return persisted
}

func (r *accountRepository) persistedCredentialsJSON(raw string) ([]byte, error) {
	if r.oauthVaultMode() != service.OAuthCredentialVaultModeExternal {
		return []byte(raw), nil
	}
	var credentials map[string]any
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return nil, err
	}
	return json.Marshal(normalizeJSONMap(r.persistedOAuthCredentials(credentials)))
}

func (r *accountRepository) persistedCredentialsJSONMap(credentials map[string]any) ([]byte, error) {
	if r.oauthVaultMode() != service.OAuthCredentialVaultModeExternal {
		return json.Marshal(normalizeJSONMap(credentials))
	}
	return json.Marshal(normalizeJSONMap(r.persistedOAuthCredentials(credentials)))
}

func (r *accountRepository) loadAccountVaultIdentity(ctx context.Context, accountID int64) (string, string, *int64, error) {
	if r == nil || r.sql == nil {
		return "", "", nil, errors.New("account repository SQL executor is not configured")
	}
	var platform, accountType string
	var ownerUserID sql.NullInt64
	rows, err := r.sql.QueryContext(ctx, `
		SELECT platform, type, owner_user_id
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
	`, accountID)
	if err != nil {
		return "", "", nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", "", nil, err
		}
		return "", "", nil, service.ErrAccountNotFound
	}
	if err := rows.Scan(&platform, &accountType, &ownerUserID); err != nil {
		return "", "", nil, err
	}
	var owner *int64
	if ownerUserID.Valid && ownerUserID.Int64 > 0 {
		value := ownerUserID.Int64
		owner = &value
	}
	return platform, accountType, owner, nil
}

func (r *accountRepository) hydrateOAuthAccount(ctx context.Context, account *service.Account) error {
	if account == nil || !service.IsOAuthCredentialAccount(account.Type) {
		return nil
	}
	if r.oauthVaultMode() == service.OAuthCredentialVaultModeDisabled {
		return service.ErrOAuthCredentialVaultDisabled
	}
	if r.oauthVaultMode() != service.OAuthCredentialVaultModeExternal || r.oauthVault == nil {
		return nil
	}
	version := service.OAuthCredentialVaultVersion(account.Credentials)
	payload, err := r.oauthVault.Get(ctx, r.oauthVaultKey(account.ID, account.OwnerUserID), version)
	if err == nil {
		account.Credentials = service.MergeOAuthCredentials(account.Credentials, payload)
		return nil
	}
	if errors.Is(err, service.ErrOAuthCredentialVaultEntryNotFound) && r.oauthVault.LegacyFallbackEnabled() {
		return nil
	}
	return fmt.Errorf("load OAuth credentials for account %d: %w", account.ID, err)
}

func (r *accountRepository) hydrateOAuthAccounts(ctx context.Context, accounts []*service.Account) ([]*service.Account, error) {
	if r.oauthVaultMode() == service.OAuthCredentialVaultModeDisabled {
		out := make([]*service.Account, 0, len(accounts))
		for _, account := range accounts {
			if account == nil || !service.IsOAuthCredentialAccount(account.Type) {
				out = append(out, account)
			}
		}
		return out, nil
	}
	for _, account := range accounts {
		if err := r.hydrateOAuthAccount(ctx, account); err != nil {
			return nil, err
		}
	}
	return accounts, nil
}

func (r *accountRepository) hydrateOAuthValueAccounts(ctx context.Context, accounts []service.Account) ([]service.Account, error) {
	if r.oauthVaultMode() == service.OAuthCredentialVaultModeDisabled {
		out := make([]service.Account, 0, len(accounts))
		for _, account := range accounts {
			if !service.IsOAuthCredentialAccount(account.Type) {
				out = append(out, account)
			}
		}
		return out, nil
	}
	for index := range accounts {
		if err := r.hydrateOAuthAccount(ctx, &accounts[index]); err != nil {
			return nil, err
		}
	}
	return accounts, nil
}
