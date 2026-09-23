//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type failingCompleteOAuthVault struct {
	*oauthCredentialVault
}

type failingDeleteSchedulerCache struct {
	*schedulerCacheRecorder
}

func (s *failingDeleteSchedulerCache) DeleteAccount(context.Context, int64) error {
	return errors.New("simulated scheduler cache deletion failure")
}

func (v *failingCompleteOAuthVault) CompleteDelete(context.Context, service.OAuthCredentialVaultKey) error {
	return errors.New("simulated Vault cleanup failure")
}

func TestOAuthCredentialVaultExternalAccountLifecycle(t *testing.T) {
	ctx := context.Background()
	vault := &oauthCredentialVault{
		db:   integrationDB,
		key:  make([]byte, 32),
		mode: service.OAuthCredentialVaultModeExternal,
	}
	_, err := integrationDB.ExecContext(ctx, oauthCredentialVaultSchema)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil, vault)
	account := &service.Account{
		Name:     fmt.Sprintf("oauth-vault-%d", time.Now().UnixNano()),
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"base_url":      "https://example.test",
			"refresh_token": "test-refresh-token",
		},
	}

	require.NoError(t, repo.Create(ctx, account))
	t.Cleanup(func() { _ = repo.Delete(ctx, account.ID) })

	stored, err := testEntClient(t).Account.Query().Where(dbaccount.IDEQ(account.ID)).Only(ctx)
	require.NoError(t, err)
	require.NotContains(t, stored.Credentials, "refresh_token")
	firstVersion := service.OAuthCredentialVaultVersion(stored.Credentials)
	require.NotEmpty(t, firstVersion)
	var firstState string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT state FROM oauth_credential_vault WHERE account_id = $1 AND version = $2", account.ID, firstVersion,
	).Scan(&firstState))
	require.Equal(t, "active", firstState)

	hydrated, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "test-refresh-token", hydrated.Credentials["refresh_token"])

	require.NoError(t, repo.UpdateCredentials(ctx, account.ID, map[string]any{
		"base_url":      "https://example.test",
		"refresh_token": "replacement-refresh-token",
	}))
	stored, err = testEntClient(t).Account.Query().Where(dbaccount.IDEQ(account.ID)).Only(ctx)
	require.NoError(t, err)
	secondVersion := service.OAuthCredentialVaultVersion(stored.Credentials)
	require.NotEqual(t, firstVersion, secondVersion)
	var secondState string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT state FROM oauth_credential_vault WHERE account_id = $1 AND version = $2", account.ID, secondVersion,
	).Scan(&secondState))
	require.Equal(t, "active", secondState)
	// Keep the old version until reconcile grace expires: an in-flight request
	// may have read its marker before this update and must still hydrate safely.
	previous, err := vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, firstVersion)
	require.NoError(t, err)
	require.Equal(t, "test-refresh-token", previous["refresh_token"])
	replacement, err := vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, secondVersion)
	require.NoError(t, err)
	require.Equal(t, "replacement-refresh-token", replacement["refresh_token"])

	// A stale tombstone for an account whose business row still exists is
	// cancelled by reconciliation; the current marker remains reachable while
	// the old version is reclaimed after the grace boundary.
	require.NoError(t, vault.BeginDelete(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}))
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE oauth_credential_vault_delete_pending
		SET requested_at = NOW() - INTERVAL '1 hour'
		WHERE account_id = $1
	`, account.ID)
	require.NoError(t, err)
	cleanup, err := vault.CleanupUnreferenced(ctx, map[int64]string{account.ID: secondVersion}, time.Now())
	require.NoError(t, err)
	require.Zero(t, cleanup.ReclaimedVersions, "first reconciliation only starts the grace period")
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, firstVersion)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE oauth_credential_vault
		SET unreferenced_since = NOW() - INTERVAL '1 hour'
		WHERE account_id = $1 AND version = $2
	`, account.ID, firstVersion)
	require.NoError(t, err)
	cleanup, err = vault.CleanupUnreferenced(ctx, map[int64]string{account.ID: secondVersion}, time.Now().Add(-15*time.Minute))
	require.NoError(t, err)
	require.GreaterOrEqual(t, cleanup.ReclaimedVersions, 1)
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, firstVersion)
	require.ErrorIs(t, err, service.ErrOAuthCredentialVaultEntryNotFound)
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, secondVersion)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, account.ID))
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, "")
	require.True(t, errors.Is(err, service.ErrOAuthCredentialVaultEntryNotFound))
}

func TestOAuthCredentialVaultDeleteFailureStillInvalidatesScheduler(t *testing.T) {
	ctx := context.Background()
	baseVault := &oauthCredentialVault{
		db:   integrationDB,
		key:  make([]byte, 32),
		mode: service.OAuthCredentialVaultModeExternal,
	}
	_, err := integrationDB.ExecContext(ctx, oauthCredentialVaultSchema)
	require.NoError(t, err)
	vault := &failingCompleteOAuthVault{oauthCredentialVault: baseVault}
	cache := &schedulerCacheRecorder{accounts: make(map[int64]*service.Account)}
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, cache, vault)
	account := &service.Account{
		Name:     fmt.Sprintf("oauth-vault-delete-failure-%d", time.Now().UnixNano()),
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"refresh_token": "delete-failure-refresh-token",
		},
	}
	require.NoError(t, repo.Create(ctx, account))
	cache.accounts[account.ID] = account

	err = repo.Delete(ctx, account.ID)
	require.ErrorContains(t, err, "simulated Vault cleanup failure")
	require.Contains(t, cache.deleteIDs, account.ID)
	exists, queryErr := testEntClient(t).Account.Query().Where(dbaccount.IDEQ(account.ID)).Exist(ctx)
	require.NoError(t, queryErr)
	require.False(t, exists)
	var pending int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM oauth_credential_vault_delete_pending WHERE account_id = $1", account.ID,
	).Scan(&pending))
	require.Equal(t, 1, pending)
	require.NoError(t, baseVault.CompleteDelete(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}))
}

func TestOAuthCredentialVaultDeleteCommitsOutboxWhenCacheInvalidationFails(t *testing.T) {
	ctx := context.Background()
	vault := &oauthCredentialVault{db: integrationDB, key: make([]byte, 32), mode: service.OAuthCredentialVaultModeExternal}
	_, err := integrationDB.ExecContext(ctx, oauthCredentialVaultSchema)
	require.NoError(t, err)
	cache := &failingDeleteSchedulerCache{schedulerCacheRecorder: &schedulerCacheRecorder{accounts: make(map[int64]*service.Account)}}
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, cache, vault)
	account := &service.Account{
		Name:        fmt.Sprintf("oauth-vault-cache-failure-%d", time.Now().UnixNano()),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "cache-failure-refresh-token"},
	}
	require.NoError(t, repo.Create(ctx, account))

	err = repo.Delete(ctx, account.ID)
	require.ErrorContains(t, err, "scheduler cache deletion failure")
	exists, queryErr := testEntClient(t).Account.Query().Where(dbaccount.IDEQ(account.ID)).Exist(ctx)
	require.NoError(t, queryErr)
	require.False(t, exists)
	var outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scheduler_outbox
		WHERE account_id = $1 AND event_type = $2
	`, account.ID, service.SchedulerOutboxEventAccountChanged).Scan(&outboxCount))
	require.GreaterOrEqual(t, outboxCount, 1)
}
