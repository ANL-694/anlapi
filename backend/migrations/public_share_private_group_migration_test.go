package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration200RestoresPrivateBindingsWithoutRemovingPublicBindings(t *testing.T) {
	content, err := FS.ReadFile("200_restore_private_group_bindings_for_public_owned_accounts.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "account.share_mode = 'public'")
	require.Contains(t, sql, "private_group.scope = 'user_private'")
	require.Contains(t, sql, "account_groups AS binding")
	require.Contains(t, sql, "ON CONFLICT (account_id, group_id) DO NOTHING")
	require.NotContains(t, sql, "DELETE FROM account_groups")
}
