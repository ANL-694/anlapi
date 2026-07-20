package service

import (
	"context"
	"errors"
	"strings"
)

type OAuthCredentialVaultMode string

const (
	OAuthCredentialVaultModeLegacy   OAuthCredentialVaultMode = "legacy"
	OAuthCredentialVaultModeExternal OAuthCredentialVaultMode = "external"
	OAuthCredentialVaultModeDisabled OAuthCredentialVaultMode = "disabled"
)

const OAuthCredentialVaultMarkerKey = "_oauth_vault"

var (
	ErrOAuthCredentialVaultEntryNotFound = errors.New("oauth credential vault entry not found")
	ErrOAuthCredentialVaultDisabled      = errors.New("OAuth credentials are disabled on this node")
)

// OAuthCredentialVaultKey is deliberately composite even though account IDs
// are currently globally unique. The owner is part of the contract so a user
// account can never be resolved by another user's identity or email.
type OAuthCredentialVaultKey struct {
	AccountID   int64
	OwnerUserID *int64
}

// OAuthCredentialVault stores only sensitive OAuth fields outside the
// replicated business database. Implementations must encrypt payloads before
// persistence and bind reads/writes to the complete key.
type OAuthCredentialVault interface {
	Mode() OAuthCredentialVaultMode
	LegacyFallbackEnabled() bool
	Get(ctx context.Context, key OAuthCredentialVaultKey, version string) (map[string]any, error)
	Put(ctx context.Context, key OAuthCredentialVaultKey, payload map[string]any) (string, error)
	Delete(ctx context.Context, key OAuthCredentialVaultKey) error
	Close() error
}

// IsOAuthCredentialAccount identifies account types whose credentials may
// contain provider tokens. Setup tokens are included because they are bearer
// credentials even though they are not refreshed like OAuth accounts.
func IsOAuthCredentialAccount(accountType string) bool {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case AccountTypeOAuth, AccountTypeSetupToken:
		return true
	default:
		return false
	}
}

// SplitOAuthCredentials removes sensitive fields from the replicated copy and
// returns them as a separate payload. The marker is retained so a later read
// can select the exact Vault version and compare-and-set mutations can still
// compare the sanitized database document.
func SplitOAuthCredentials(credentials map[string]any) (persisted, sensitive map[string]any, hasSensitive bool) {
	persisted = cloneCredentialMap(credentials)
	sensitive = make(map[string]any)
	for key, value := range persisted {
		if isOAuthSensitiveCredentialKey(key) {
			sensitive[key] = value
			delete(persisted, key)
		}
	}
	return persisted, sensitive, len(sensitive) > 0
}

func MergeOAuthCredentials(persisted, sensitive map[string]any) map[string]any {
	merged := cloneCredentialMap(persisted)
	for key, value := range sensitive {
		merged[key] = value
	}
	return merged
}

func OAuthCredentialVaultVersion(credentials map[string]any) string {
	marker, ok := credentials[OAuthCredentialVaultMarkerKey].(map[string]any)
	if !ok {
		return ""
	}
	version, _ := marker["version"].(string)
	return strings.TrimSpace(version)
}

func SetOAuthCredentialVaultVersion(credentials map[string]any, version string) map[string]any {
	persisted := cloneCredentialMap(credentials)
	if strings.TrimSpace(version) == "" {
		delete(persisted, OAuthCredentialVaultMarkerKey)
		return persisted
	}
	persisted[OAuthCredentialVaultMarkerKey] = map[string]any{
		"version": strings.TrimSpace(version),
	}
	return persisted
}

func cloneCredentialMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func isOAuthSensitiveCredentialKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "access_token",
		"refresh_token",
		"id_token",
		"oauth_token",
		"oauth_access_token",
		"oauth_refresh_token",
		"session_token",
		"session_key",
		"claude_session_key",
		"cookie",
		"cookies",
		"browser_cookie",
		"set_cookie",
		"client_secret",
		"authorization",
		"authorization_header":
		return true
	default:
		return strings.HasSuffix(normalized, "_secret") ||
			(strings.Contains(normalized, "token") &&
				normalized != "token_type" &&
				normalized != "oauth_type" &&
				normalized != "token_version" &&
				normalized != "_token_version" &&
				normalized != "oauth_token_version")
	}
}
