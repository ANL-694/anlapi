package migrations

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserScopedGroupMigrationsHavePrerequisitesAndFinalConstraint(t *testing.T) {
	files, err := FS.ReadDir(".")
	require.NoError(t, err)

	names := make([]string, 0, len(files))
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sql") {
			names = append(names, file.Name())
		}
	}
	sort.Strings(names)

	indexOf := func(name string) int {
		for i, candidate := range names {
			if candidate == name {
				return i
			}
		}
		return -1
	}

	prerequisite := indexOf("185a_add_group_scope_prerequisites.sql")
	carpool := indexOf("186_user_carpool_groups.sql")
	ownerIsolation := indexOf("222_private_account_owner_isolation.sql")
	finalConstraint := indexOf("243_allow_user_carpool_group_scope.sql")
	require.GreaterOrEqual(t, prerequisite, 0)
	require.Greater(t, carpool, prerequisite)
	require.Greater(t, ownerIsolation, carpool)
	require.Greater(t, finalConstraint, ownerIsolation)

	prerequisiteSQL, err := FS.ReadFile("185a_add_group_scope_prerequisites.sql")
	require.NoError(t, err)
	require.Contains(t, string(prerequisiteSQL), "ADD COLUMN IF NOT EXISTS owner_user_id")
	require.Contains(t, string(prerequisiteSQL), "ADD COLUMN IF NOT EXISTS scope")

	finalSQL, err := FS.ReadFile("243_allow_user_carpool_group_scope.sql")
	require.NoError(t, err)
	require.Contains(t, string(finalSQL), "scope IN ('user_private', 'user_carpool')")
	require.Contains(t, string(finalSQL), "scope IN ('public', 'user_private', 'user_carpool')")
}
