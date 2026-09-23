//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_DeleteUser_CleansOwnedRoutingStateAtomically(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{})
	var groupID, accountID int64
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if accountID != 0 {
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM oauth_credential_vault_delete_pending WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM oauth_credential_vault WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", accountID)
		}
		if groupID != 0 {
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE group_id = $1", groupID)
		}
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM account_groups WHERE account_id = $1 OR group_id = $2", accountID, groupID)
		if accountID != 0 {
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduled_test_plans WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM accounts WHERE id = $1", accountID)
		}
		if groupID != 0 {
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM groups WHERE id = $1", groupID)
		}
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:     fmt.Sprintf("delete-private-%d", user.ID),
		Platform: service.PlatformAnthropic,
	})
	groupID = group.ID
	_, err := client.Group.UpdateOneID(group.ID).
		SetScope(dbgroup.ScopeUserPrivate).
		SetOwnerUserID(user.ID).
		Save(ctx)
	require.NoError(t, err)
	account := mustCreateAccount(t, client, &service.Account{
		Name:        fmt.Sprintf("delete-owned-%d", user.ID),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &user.ID,
	})
	accountID = account.ID
	_, err = client.AccountGroup.Create().SetAccountID(account.ID).SetGroupID(group.ID).SetPriority(1).Save(ctx)
	require.NoError(t, err)

	repo := NewUserRepository(client, integrationDB, nil)
	require.NoError(t, repo.Delete(ctx, user.ID))

	var owner any
	var deleted any
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT owner_user_id, deleted_at FROM accounts WHERE id = $1`, account.ID).Scan(&owner, &deleted))
	require.Nil(t, owner)
	require.NotNil(t, deleted)

	var groupOwner any
	var groupScope string
	var groupDeleted any
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT owner_user_id, scope, deleted_at FROM groups WHERE id = $1`, group.ID).Scan(&groupOwner, &groupScope, &groupDeleted))
	require.Nil(t, groupOwner)
	require.Equal(t, service.GroupScopePublic, groupScope)
	require.NotNil(t, groupDeleted)

	var joins int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_groups WHERE account_id = $1 OR group_id = $2`, account.ID, group.ID).Scan(&joins))
	require.Zero(t, joins)
}

func TestUserRepository_DeleteUser_CompletesOwnedOAuthVaultDeletes(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	vault := &oauthCredentialVault{
		db:   integrationDB,
		key:  make([]byte, 32),
		mode: service.OAuthCredentialVaultModeExternal,
	}
	_, err := integrationDB.ExecContext(ctx, oauthCredentialVaultSchema)
	require.NoError(t, err)
	user := mustCreateUser(t, client, &service.User{})
	var accountID int64
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if accountID != 0 {
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM oauth_credential_vault_delete_pending WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM oauth_credential_vault WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM account_groups WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduled_test_plans WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM accounts WHERE id = $1", accountID)
		}
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil, vault)
	account := &service.Account{
		Name:        fmt.Sprintf("delete-owned-oauth-%d", time.Now().UnixNano()),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		OwnerUserID: &user.ID,
		Credentials: map[string]any{"refresh_token": "test-delete-refresh-token"},
	}
	require.NoError(t, accountRepo.Create(ctx, account))
	accountID = account.ID

	userRepo := NewUserRepository(client, integrationDB, vault)
	require.NoError(t, userRepo.Delete(ctx, user.ID))

	stored, err := client.Account.Query().Where(dbaccount.IDEQ(account.ID)).Only(mixins.SkipSoftDelete(ctx))
	require.NoError(t, err)
	require.NotNil(t, stored.DeletedAt)
	require.Nil(t, stored.OwnerUserID)
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, "")
	require.True(t, errors.Is(err, service.ErrOAuthCredentialVaultEntryNotFound))
}

func TestUserRepository_DeleteUser_CallerTransactionTombstoneReconcilesCommitOrRollback(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	vault := &oauthCredentialVault{
		db:   integrationDB,
		key:  make([]byte, 32),
		mode: service.OAuthCredentialVaultModeExternal,
	}
	_, err := integrationDB.ExecContext(ctx, oauthCredentialVaultSchema)
	require.NoError(t, err)
	user := mustCreateUser(t, client, &service.User{})
	var accountID int64
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if accountID != 0 {
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM oauth_credential_vault_delete_pending WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM oauth_credential_vault WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM account_groups WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduled_test_plans WHERE account_id = $1", accountID)
			_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM accounts WHERE id = $1", accountID)
		}
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil, vault)
	account := &service.Account{
		Name:        fmt.Sprintf("delete-owned-oauth-tx-%d", time.Now().UnixNano()),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		OwnerUserID: &user.ID,
		Credentials: map[string]any{"refresh_token": "test-delete-tx-refresh-token"},
	}
	require.NoError(t, accountRepo.Create(ctx, account))
	accountID = account.ID

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	userRepo := NewUserRepository(client, integrationDB, vault)
	err = userRepo.Delete(dbent.NewTxContext(ctx, tx), user.ID)
	require.NoError(t, err)
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, "")
	require.NoError(t, err, "tombstone must not delete credentials before outer commit is known")
	require.NoError(t, tx.Rollback())

	exists, err := client.Account.Query().Where(dbaccount.IDEQ(account.ID)).Exist(ctx)
	require.NoError(t, err)
	require.True(t, exists)
	_, err = ReconcileOAuthCredentialVault(ctx, integrationDB, vault, time.Now().Add(time.Minute))
	require.NoError(t, err)
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, "")
	require.NoError(t, err)

	tx, err = client.Tx(ctx)
	require.NoError(t, err)
	err = userRepo.Delete(dbent.NewTxContext(ctx, tx), user.ID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	_, err = ReconcileOAuthCredentialVault(ctx, integrationDB, vault, time.Now().Add(time.Minute))
	require.NoError(t, err)
	_, err = vault.Get(ctx, service.OAuthCredentialVaultKey{AccountID: account.ID}, "")
	require.True(t, errors.Is(err, service.ErrOAuthCredentialVaultEntryNotFound))
}
