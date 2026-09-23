//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_OwnerAndPrivateGroupBindingsFailClosed(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user1 := mustCreateUser(t, client, &service.User{})
	user2 := mustCreateUser(t, client, &service.User{})

	private := mustCreateGroup(t, client, &service.Group{
		Name:        fmt.Sprintf("private-%d", user1.ID),
		Platform:    service.PlatformAnthropic,
		Scope:       service.GroupScopeUserPrivate,
		OwnerUserID: &user1.ID,
	})
	_, err := client.Group.UpdateOneID(private.ID).
		SetScope(dbgroup.ScopeUserPrivate).
		SetOwnerUserID(user1.ID).
		Save(ctx)
	require.NoError(t, err)
	public := mustCreateGroup(t, client, &service.Group{
		Name:     fmt.Sprintf("public-%d", user1.ID),
		Platform: service.PlatformAnthropic,
		Scope:    service.GroupScopePublic,
	})
	_, err = client.Group.UpdateOneID(public.ID).
		SetScope(dbgroup.ScopePublic).
		ClearOwnerUserID().
		Save(ctx)
	require.NoError(t, err)
	var sharedID int64

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id IN ($1, $2)", private.ID, public.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE owner_user_id IN ($1, $2) OR id = $3", user1.ID, user2.ID, sharedID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id IN ($1, $2)", private.ID, public.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id IN ($1, $2)", user1.ID, user2.ID)
	})

	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	owned := &service.Account{
		Name:        "owned",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &user1.ID,
		Status:      service.StatusActive,
		Schedulable: true,
	}
	require.NoError(t, repo.CreateWithAccountGroups(ctx, owned, []service.AccountGroup{{GroupID: private.ID, Priority: 1}}))

	crossOwner := &service.Account{
		Name:        "cross-owner",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &user2.ID,
		Status:      service.StatusActive,
		Schedulable: true,
	}
	require.Error(t, repo.CreateWithAccountGroups(ctx, crossOwner, []service.AccountGroup{{GroupID: private.ID, Priority: 1}}))

	publicOwned := &service.Account{
		Name:        "public-owned",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &user1.ID,
		Status:      service.StatusActive,
		Schedulable: true,
	}
	require.Error(t, repo.CreateWithAccountGroups(ctx, publicOwned, []service.AccountGroup{{GroupID: public.ID, Priority: 1}}))
	shared := &service.Account{
		Name:        fmt.Sprintf("shared-%d", user1.ID),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		Status:      service.StatusActive,
		Schedulable: true,
	}
	require.NoError(t, repo.Create(ctx, shared))
	sharedID = shared.ID
	require.NoError(t, repo.AddToGroup(ctx, shared.ID, public.ID, 1))
	ownedUngrouped := &service.Account{
		Name:        fmt.Sprintf("owned-ungrouped-%d", user1.ID),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &user1.ID,
		Status:      service.StatusActive,
		Schedulable: true,
	}
	require.NoError(t, repo.Create(ctx, ownedUngrouped))

	privateAccounts, err := repo.ListSchedulableByGroupID(ctx, private.ID)
	require.NoError(t, err)
	require.Len(t, privateAccounts, 1)
	require.Equal(t, user1.ID, *privateAccounts[0].OwnerUserID)
	capacityRows, err := repo.ListSchedulableCapacityByGroupIDs(ctx, []int64{private.ID, public.ID})
	require.NoError(t, err)
	require.Len(t, capacityRows, 2)
	capacityByGroup := make(map[int64]int64, len(capacityRows))
	for _, row := range capacityRows {
		capacityByGroup[row.GroupID] = row.AccountID
	}
	require.Equal(t, owned.ID, capacityByGroup[private.ID])
	require.Equal(t, shared.ID, capacityByGroup[public.ID])

	sharedAccounts, err := repo.ListSchedulable(ctx)
	require.NoError(t, err)
	for _, account := range sharedAccounts {
		require.Nil(t, account.OwnerUserID, "owned accounts must never enter public scheduler candidates")
	}
	for _, accounts := range [][]service.Account{
		mustSchedulableByPlatform(t, repo, ctx),
		mustSchedulableByPlatforms(t, repo, ctx),
	} {
		foundShared := false
		for _, account := range accounts {
			require.Nil(t, account.OwnerUserID, "global scheduler candidates must exclude owned accounts")
			if account.ID == shared.ID {
				foundShared = true
			}
		}
		require.True(t, foundShared, "shared account must remain eligible")
	}
	for _, accounts := range [][]service.Account{
		mustSchedulableUngroupedByPlatform(t, repo, ctx),
		mustSchedulableUngroupedByPlatforms(t, repo, ctx),
	} {
		for _, account := range accounts {
			require.Nil(t, account.OwnerUserID, "ungrouped scheduler candidates must exclude owned accounts")
			require.NotEqual(t, shared.ID, account.ID, "grouped shared accounts must not enter ungrouped candidates")
		}
	}

	ownedGroups, err := repo.GetGroups(ctx, owned.ID)
	require.NoError(t, err)
	require.Len(t, ownedGroups, 1)
	require.Equal(t, private.ID, ownedGroups[0].ID)
	// Simulate a legacy illegal public-to-owned join. Reads must fail closed
	// instead of exposing the stale public binding to callers or caches.
	_, err = integrationDB.ExecContext(ctx, "INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ($1, $2, 1, NOW()) ON CONFLICT DO NOTHING", owned.ID, public.ID)
	require.NoError(t, err)
	ownedGroups, err = repo.GetGroups(ctx, owned.ID)
	require.NoError(t, err)
	require.Len(t, ownedGroups, 1)
	require.Equal(t, private.ID, ownedGroups[0].ID)
	loadedOwned, err := repo.GetByID(ctx, owned.ID)
	require.NoError(t, err)
	require.Len(t, loadedOwned.Groups, 1)
	require.Equal(t, private.ID, loadedOwned.Groups[0].ID)
	require.Len(t, loadedOwned.AccountGroups, 1)
	require.Equal(t, private.ID, loadedOwned.AccountGroups[0].GroupID)
}

func mustSchedulableByPlatform(t *testing.T, repo *accountRepository, ctx context.Context) []service.Account {
	accounts, err := repo.ListSchedulableByPlatform(ctx, service.PlatformAnthropic)
	require.NoError(t, err)
	return accounts
}

func mustSchedulableByPlatforms(t *testing.T, repo *accountRepository, ctx context.Context) []service.Account {
	accounts, err := repo.ListSchedulableByPlatforms(ctx, []string{service.PlatformAnthropic})
	require.NoError(t, err)
	return accounts
}

func mustSchedulableUngroupedByPlatform(t *testing.T, repo *accountRepository, ctx context.Context) []service.Account {
	accounts, err := repo.ListSchedulableUngroupedByPlatform(ctx, service.PlatformAnthropic)
	require.NoError(t, err)
	return accounts
}

func mustSchedulableUngroupedByPlatforms(t *testing.T, repo *accountRepository, ctx context.Context) []service.Account {
	accounts, err := repo.ListSchedulableUngroupedByPlatforms(ctx, []string{service.PlatformAnthropic})
	require.NoError(t, err)
	return accounts
}

func TestAccountRepository_OwnerGroupPlatformMismatchRollsBack(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{})
	group := mustCreateGroup(t, client, &service.Group{
		Name:        fmt.Sprintf("private-openai-%d", user.ID),
		Platform:    service.PlatformOpenAI,
		Scope:       service.GroupScopeUserPrivate,
		OwnerUserID: &user.ID,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", group.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	account := &service.Account{
		Name:        "wrong-platform",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &user.ID,
		Status:      service.StatusActive,
		Schedulable: true,
	}
	require.Error(t, repo.CreateWithAccountGroups(ctx, account, []service.AccountGroup{{GroupID: group.ID, Priority: 1}}))

	count, err := client.Account.Query().Where(dbaccount.OwnerUserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count, "failed binding must roll back the account insert")
}

func TestAccountRepository_OwnedMutationsApplyOwnerPredicate(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	owner := mustCreateUser(t, client, &service.User{})
	other := mustCreateUser(t, client, &service.User{})
	account := mustCreateAccount(t, client, &service.Account{
		Name:        fmt.Sprintf("owned-mutation-%d", owner.ID),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test"},
		OwnerUserID: &owner.ID,
	})
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM account_groups WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduled_test_plans WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id IN ($1, $2)", owner.ID, other.ID)
	})

	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)

	// A stale caller can present the right account ID with a different owner
	// in its in-memory snapshot. The final UPDATE must still affect zero rows.
	stale := *account
	stale.OwnerUserID = &other.ID
	stale.Name = "must-not-update"
	err := repo.UpdateOwned(ctx, other.ID, &stale)
	require.ErrorIs(t, err, service.ErrAccountNotFound)
	stored, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, account.Name, stored.Name)
	require.Equal(t, owner.ID, *stored.OwnerUserID)

	err = repo.DeleteOwned(ctx, other.ID, account.ID)
	require.ErrorIs(t, err, service.ErrAccountNotFound)
	exists, err := client.Account.Query().Where(dbaccount.IDEQ(account.ID)).Exist(ctx)
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, repo.DeleteOwned(ctx, owner.ID, account.ID))
	exists, err = client.Account.Query().Where(dbaccount.IDEQ(account.ID)).Exist(ctx)
	require.NoError(t, err)
	require.False(t, exists)
}
