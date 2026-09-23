package service

import (
	"context"
	"errors"
	"strings"
	"time"
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

// OAuthCredentialVaultKey is deliberately explicit so the B07b ownership
// extension can add an owner without changing the storage interface again.
type OAuthCredentialVaultKey struct {
	AccountID int64
}

// OAuthCredentialVault stores only bearer OAuth fields outside the replicated
// business database. Implementations must encrypt payloads before persistence.
type OAuthCredentialVault interface {
	Mode() OAuthCredentialVaultMode
	LegacyFallbackEnabled() bool
	Get(ctx context.Context, key OAuthCredentialVaultKey, version string) (map[string]any, error)
	Put(ctx context.Context, key OAuthCredentialVaultKey, payload map[string]any) (string, error)
	Delete(ctx context.Context, key OAuthCredentialVaultKey) error
	Close() error
}

// OAuthCredentialVaultWriteCoordinator closes the cross-database write
// protocol. Put creates a durable pending version; the business repository
// activates it only after the marker transaction commits. Reconciliation may
// also activate a pending version once it observes the committed marker.
type OAuthCredentialVaultWriteCoordinator interface {
	ActivateVersion(ctx context.Context, key OAuthCredentialVaultKey, version string) error
}

// OAuthCredentialVaultDeleteCoordinator makes cross-database account deletion
// recoverable. The pending marker lives in the Vault database, so a business
// transaction that commits before the final Vault delete still has a durable
// retry record.
type OAuthCredentialVaultDeleteCoordinator interface {
	BeginDelete(ctx context.Context, key OAuthCredentialVaultKey) error
	CancelDelete(ctx context.Context, key OAuthCredentialVaultKey) error
	CompleteDelete(ctx context.Context, key OAuthCredentialVaultKey) error
}

// OAuthCredentialVaultVersionJanitor is intentionally optional on the base
// interface. It lets the repository compensate failed writes and lets the
// maintenance command remove old/unreferenced ciphertext without introducing
// an in-process credential cache.
type OAuthCredentialVaultVersionJanitor interface {
	DeleteVersion(ctx context.Context, key OAuthCredentialVaultKey, version string) error
	CleanupUnreferenced(ctx context.Context, references map[int64]string, olderThan time.Time) (OAuthCredentialVaultCleanupReport, error)
}

type OAuthCredentialVaultCleanupReport struct {
	ReclaimedVersions int
	CompletedDeletes  int
	PendingDeletes    int
	PendingWrites     int
}

func IsOAuthCredentialAccount(accountType string) bool {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case AccountTypeOAuth, AccountTypeSetupToken:
		return true
	default:
		return false
	}
}

func SplitOAuthCredentials(credentials map[string]any) (persisted, sensitive map[string]any, hasSensitive bool) {
	persisted = cloneOAuthCredentialMap(credentials)
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
	merged := cloneOAuthCredentialMap(persisted)
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
	persisted := cloneOAuthCredentialMap(credentials)
	if strings.TrimSpace(version) == "" {
		delete(persisted, OAuthCredentialVaultMarkerKey)
		return persisted
	}
	persisted[OAuthCredentialVaultMarkerKey] = map[string]any{"version": strings.TrimSpace(version)}
	return persisted
}

func cloneOAuthCredentialMap(values map[string]any) map[string]any {
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
	case "access_token", "refresh_token", "id_token", "oauth_token",
		"oauth_access_token", "oauth_refresh_token", "session_token",
		"session_key", "claude_session_key", "cookie", "cookies",
		"browser_cookie", "set_cookie", "client_secret", "authorization",
		"authorization_header":
		return true
	default:
		return strings.HasSuffix(normalized, "_secret") ||
			(strings.Contains(normalized, "token") && normalized != "token_type" &&
				normalized != "oauth_type" && normalized != "token_version" &&
				normalized != "_token_version" && normalized != "oauth_token_version")
	}
}
