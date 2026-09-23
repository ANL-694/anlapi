package repository

import (
	"context"
	"testing"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEnsureSystemAPIKeyPersistsAndReusesBinding(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()

	_, err := repo.sql.ExecContext(ctx, `
		CREATE TABLE system_api_key_bindings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			api_key_id INTEGER NOT NULL,
			purpose TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, purpose),
			UNIQUE(api_key_id, purpose)
		)`)
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("system-key@example.com").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	first, err := repo.EnsureSystemAPIKey(ctx, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-system-first",
		Name:   "GPT 生图专线",
		Status: service.StatusAPIKeyActive,
	}, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.NotZero(t, first.ID)

	second, err := repo.EnsureSystemAPIKey(ctx, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-system-second",
		Name:   "GPT 生图专线",
		Status: service.StatusAPIKeyActive,
	}, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	count, err := client.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	_, err = repo.sql.ExecContext(ctx, `UPDATE api_keys SET key = ?, deleted_at = ? WHERE id = ?`, "__deleted_system__first", time.Now(), first.ID)
	require.NoError(t, err)
	third, err := repo.EnsureSystemAPIKey(ctx, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-system-third",
		Name:   "GPT 生图专线",
		Status: service.StatusAPIKeyActive,
	}, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, third.ID)
	require.Equal(t, "sk-system-third", third.Key)

	rows, err := repo.sql.QueryContext(ctx, `SELECT api_key_id FROM system_api_key_bindings WHERE user_id = ? AND purpose = ?`, user.ID, service.APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var boundID int64
	require.NoError(t, rows.Scan(&boundID))
	require.Equal(t, third.ID, boundID)
}
