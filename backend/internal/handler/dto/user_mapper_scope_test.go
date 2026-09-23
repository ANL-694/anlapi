//go:build unit

package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserMappersKeepAdminFieldsOutOfUserPayload(t *testing.T) {
	user := &service.User{
		ID:                   7,
		Email:                "user@example.com",
		Role:                 service.RoleUser,
		Notes:                "internal note",
		GroupRates:           map[int64]float64{3: 1.25},
		RestrictPublicGroups: true,
		APIKeys: []service.APIKey{{
			ID:     9,
			UserID: 7,
			Key:    "fixture-test-1234567890-example",
			Name:   "primary",
		}},
	}

	userDTO := UserFromService(user)
	require.NotNil(t, userDTO)
	require.Len(t, userDTO.APIKeys, 1)
	require.Equal(t, "fixtur...mple", userDTO.APIKeys[0].Key)

	userJSON, err := json.Marshal(userDTO)
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "internal note")
	require.NotContains(t, string(userJSON), "group_rates")
	require.NotContains(t, string(userJSON), "restrict_public_groups")
	require.NotContains(t, string(userJSON), "fixture-test-1234567890-example")

	adminDTO := UserFromServiceAdmin(user)
	require.NotNil(t, adminDTO)
	require.Equal(t, user.Notes, adminDTO.Notes)
	require.Equal(t, user.GroupRates, adminDTO.GroupRates)
	require.True(t, adminDTO.RestrictPublicGroups)
}
