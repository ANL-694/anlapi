package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitOAuthCredentialsSeparatesBearerSecretsAndRetainsIdentity(t *testing.T) {
	credentials := map[string]any{
		"access_token":       "access",
		"refresh_token":      "refresh",
		"id_token":           "id",
		"cookie":             "cookie-value",
		"chatgpt_account_id": "account-id",
		"project_id":         "project-id",
		"expires_at":         "2030-01-01T00:00:00Z",
		"token_type":         "bearer",
		"_token_version":     int64(2),
	}

	persisted, sensitive, hasSensitive := SplitOAuthCredentials(credentials)

	require.True(t, hasSensitive)
	require.Equal(t, map[string]any{
		"chatgpt_account_id": "account-id",
		"project_id":         "project-id",
		"expires_at":         "2030-01-01T00:00:00Z",
		"token_type":         "bearer",
		"_token_version":     int64(2),
	}, persisted)
	require.Equal(t, map[string]any{
		"access_token":  "access",
		"refresh_token": "refresh",
		"id_token":      "id",
		"cookie":        "cookie-value",
	}, sensitive)
}

func TestOAuthCredentialVaultVersionMarkerRoundTrip(t *testing.T) {
	credentials := SetOAuthCredentialVaultVersion(map[string]any{"account_id": "a"}, "v1")
	require.Equal(t, "v1", OAuthCredentialVaultVersion(credentials))

	withoutMarker := SetOAuthCredentialVaultVersion(credentials, "")
	require.Empty(t, OAuthCredentialVaultVersion(withoutMarker))
	require.NotContains(t, withoutMarker, OAuthCredentialVaultMarkerKey)
}
