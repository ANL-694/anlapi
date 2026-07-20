package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	dbent "ikik-api/ent"
	"ikik-api/ent/enttest"
	"ikik-api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newSystemAPIKeySQLiteRepository(t *testing.T) (*apiKeyRepository, *dbent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:system_api_key_repo?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(driver)))
	t.Cleanup(func() { _ = client.Close() })

	_, err = db.Exec(`
		CREATE TABLE system_api_key_bindings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			api_key_id INTEGER NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
			purpose TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (user_id, purpose),
			UNIQUE (api_key_id, purpose)
		)
	`)
	require.NoError(t, err)
	return newAPIKeyRepositoryWithSQL(client, db), client
}

func TestAPIKeyRepository_SystemImageKeyReconcilesAfterSoftDelete(t *testing.T) {
	ctx := context.Background()
	repo, client := newSystemAPIKeySQLiteRepository(t)

	user, err := client.User.Create().
		SetEmail("system-image-key@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("image-group").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetAllowImageGeneration(true).
		Save(ctx)
	require.NoError(t, err)

	firstCandidate := &service.APIKey{
		UserID:  user.ID,
		Key:     "sk-system-image-first",
		Name:    "系统-图片生成",
		GroupID: &group.ID,
		GroupRoutes: []service.APIKeyGroupRoute{{
			GroupID:         group.ID,
			Priority:        100,
			Weight:          1,
			Enabled:         true,
			CooldownSeconds: 30,
		}},
		Status: service.StatusActive,
	}
	first, err := repo.EnsureSystemAPIKey(ctx, firstCandidate, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.NotZero(t, first.ID)
	require.Equal(t, service.APIKeyManagedTypeImageGeneration, first.ManagedType)

	duplicateCandidate := *firstCandidate
	duplicateCandidate.ID = 0
	duplicateCandidate.Key = "sk-system-image-duplicate"
	reused, err := repo.EnsureSystemAPIKey(ctx, &duplicateCandidate, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.Equal(t, first.ID, reused.ID)

	keyID, found, err := repo.GetSystemAPIKeyID(ctx, user.ID, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, first.ID, keyID)
	purpose, found, err := repo.GetSystemAPIKeyPurpose(ctx, user.ID, first.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, service.APIKeyManagedTypeImageGeneration, purpose)

	require.NoError(t, repo.Delete(ctx, first.ID))
	replacementCandidate := *firstCandidate
	replacementCandidate.ID = 0
	replacementCandidate.Key = "sk-system-image-replacement"
	replacement, err := repo.EnsureSystemAPIKey(ctx, &replacementCandidate, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, replacement.ID)

	keyID, found, err = repo.GetSystemAPIKeyID(ctx, user.ID, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, replacement.ID, keyID)
	purposes, err := repo.ListSystemAPIKeyPurposes(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, map[int64]string{replacement.ID: service.APIKeyManagedTypeImageGeneration}, purposes)
}
