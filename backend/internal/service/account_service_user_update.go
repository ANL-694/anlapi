package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// UserAccountUpdateRequest is the public user-account mutation contract.
// Scheduler, billing, expiry, grouping, and account-status fields deliberately
// do not exist here; internal refresh/privacy flows continue to use the
// broader UpdateAccountRequest type.
type UserAccountUpdateRequest struct {
	Name        *string         `json:"name"`
	Notes       *string         `json:"notes"`
	Credentials *map[string]any `json:"credentials"`
	Extra       *map[string]any `json:"extra"`
	ProxyID     *int64          `json:"proxy_id"`
	ShareMode   *string         `json:"share_mode"`
}

// UserAccountCreateRequest is the user-owned account creation contract.
// Account scheduling, billing, expiry, grouping, and status are server-owned
// defaults and intentionally have no representation in this request.
type UserAccountCreateRequest struct {
	Name        string         `json:"name"`
	Notes       *string        `json:"notes"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyID     *int64         `json:"proxy_id"`
	ShareMode   *string        `json:"share_mode"`
}

// BulkUpdateOwnedUserAccountsInput is the safe user-facing bulk equivalent
// of UserAccountUpdateRequest.
type BulkUpdateOwnedUserAccountsInput struct {
	AccountIDs  []int64
	ProxyID     *int64
	ShareMode   *string
	Credentials *map[string]any
	Extra       *map[string]any
}

var userAccountMutableExtraKeys = map[string]struct{}{
	"free_model_disabled_models": {},
	"free_model_last_filter_at":  {},
}

var userAccountCreateForbiddenExtraKeys = map[string]struct{}{
	"openai_responses_supported":                    {},
	"openai_oauth_responses_websockets_v2_mode":     {},
	"openai_oauth_responses_websockets_v2_enabled":  {},
	"openai_apikey_responses_websockets_v2_mode":    {},
	"openai_apikey_responses_websockets_v2_enabled": {},
	"responses_websockets_v2_enabled":               {},
	"openai_ws_enabled":                             {},
	"openai_passthrough":                            {},
	"openai_oauth_passthrough":                      {},
	"openai_responses_flatten_namespaces":           {},
	"openai_long_context_billing_enabled":           {},
	"codex_cli_only":                                {},
	"codex_cli_only_allowed_clients":                {},
	"codex_cli_only_allow_app_server":               {},
	"codex_fingerprint_mode":                        {},
	"openai_compact_mode":                           {},
	"openai_responses_mode":                         {},
	"anthropic_passthrough":                         {},
	"anthropic_apikey_auth_scheme":                  {},
	"web_search_emulation":                          {},
	"window_cost_limit":                             {},
	"window_cost_sticky_reserve":                    {},
	"max_sessions":                                  {},
	"session_idle_timeout_minutes":                  {},
	"base_rpm":                                      {},
	"rpm_strategy":                                  {},
	"rpm_sticky_buffer":                             {},
	"user_msg_queue_mode":                           {},
	"enable_tls_fingerprint":                        {},
	"tls_fingerprint_profile_id":                    {},
	"session_id_masking_enabled":                    {},
	"cache_ttl_override_enabled":                    {},
	"cache_ttl_override_target":                     {},
	"custom_base_url_enabled":                       {},
	"custom_base_url":                               {},
}

var userAccountCreateForbiddenCredentialKeys = map[string]struct{}{
	"pool_mode":                    {},
	"pool_mode_retry_count":        {},
	"pool_mode_retry_status_codes": {},
	"custom_error_codes_enabled":   {},
	"custom_error_codes":           {},
	"intercept_warmup_requests":    {},
	"temp_unschedulable_enabled":   {},
	"temp_unschedulable_rules":     {},
	"openai_capabilities":          {},
	"header_override_enabled":      {},
	"header_overrides":             {},
}

const (
	userAccountDefaultConcurrency = 3
	userAccountDefaultPriority    = 1
)

func userAccountFieldNotAllowed(field, key string) error {
	return infraerrors.BadRequest("USER_ACCOUNT_FIELD_NOT_ALLOWED", fmt.Sprintf("%s.%s is not allowed for user accounts", field, key)).WithMetadata(map[string]string{
		"field": field,
		"key":   key,
	})
}

func validateUserAccountCreateMap(values map[string]any, field string, forbidden map[string]struct{}) error {
	for key := range values {
		if _, blocked := forbidden[key]; blocked {
			return userAccountFieldNotAllowed(field, key)
		}
		if field == "extra" && strings.HasPrefix(key, "quota_") {
			return userAccountFieldNotAllowed(field, key)
		}
	}
	return nil
}

func cloneAccountMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

// jsonValuesEqual compares values after JSON normalization so values loaded
// from JSONB (for example int) compare correctly with request values decoded as
// float64. It falls back to reflect for values that cannot be JSON encoded.
func jsonValuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr == nil && rightErr == nil {
		return string(leftJSON) == string(rightJSON)
	}
	return reflect.DeepEqual(left, right)
}

func userAccountMutableCredentialKeys(account *Account) map[string]struct{} {
	keys := map[string]struct{}{
		"model_mapping":         {},
		"model_whitelist":       {}, // legacy representation retained for compatibility
		"compact_model_mapping": {},
	}
	if account == nil {
		return keys
	}
	if account.Type == AccountTypeAPIKey {
		keys["api_key"] = struct{}{}
		keys["base_url"] = struct{}{}
	}
	if account.Platform == PlatformAntigravity {
		keys["antigravity_project_id"] = struct{}{}
	}
	return keys
}

func mergeUserAccountMap(existing, incoming map[string]any, field string, mutable map[string]struct{}) (map[string]any, error) {
	next := cloneAccountMap(existing)
	if next == nil {
		next = make(map[string]any)
	}
	for key, value := range incoming {
		if _, allowed := mutable[key]; allowed {
			if value == nil {
				delete(next, key)
			} else {
				next[key] = value
			}
			continue
		}
		existingValue, exists := existing[key]
		// A full-object UI update may echo server-managed keys. Equal values are
		// preserved; only an actual client-side mutation is rejected.
		if !exists || !jsonValuesEqual(existingValue, value) {
			return nil, userAccountFieldNotAllowed(field, key)
		}
	}
	return next, nil
}

func mergeUserAccountCredentials(account *Account, incoming map[string]any) (map[string]any, error) {
	if account == nil {
		return nil, ErrAccountNotFound
	}
	mutable := userAccountMutableCredentialKeys(account)
	next, err := mergeUserAccountMap(account.Credentials, incoming, "credentials", mutable)
	if err != nil {
		return nil, err
	}
	if value, ok := incoming["api_key"]; ok {
		if account.Type != AccountTypeAPIKey {
			return nil, userAccountFieldNotAllowed("credentials", "api_key")
		}
		if valueString, ok := value.(string); !ok || strings.TrimSpace(valueString) == "" {
			return nil, infraerrors.BadRequest("USER_ACCOUNT_CREDENTIAL_INVALID", "credentials.api_key must be a non-empty string")
		}
	}
	return SanitizeStoredCredentials(account.Platform, next), nil
}

func mergeUserAccountExtra(account *Account, incoming map[string]any) (map[string]any, error) {
	return mergeUserAccountMap(account.Extra, incoming, "extra", userAccountMutableExtraKeys)
}

// UpdateOwnedUser applies only fields exposed by the user account UI and keeps
// the owner predicate on the durable write. Existing server-managed nested
// settings may be echoed unchanged, but cannot be changed through this path.
func (s *AccountService) UpdateOwnedUser(ctx context.Context, ownerUserID, accountID int64, req UserAccountUpdateRequest) (*Account, error) {
	account, err := s.GetOwnedByID(ctx, ownerUserID, accountID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		account.Name = *req.Name
	}
	if req.Notes != nil {
		account.Notes = normalizeAccountNotes(req.Notes)
	}
	if req.Credentials != nil {
		account.Credentials, err = mergeUserAccountCredentials(account, *req.Credentials)
		if err != nil {
			return nil, err
		}
	}
	if req.Extra != nil {
		account.Extra, err = mergeUserAccountExtra(account, *req.Extra)
		if err != nil {
			return nil, err
		}
	}
	if req.ProxyID != nil {
		validatedProxyID, validateErr := s.ValidateOwnedProxyID(ctx, ownerUserID, req.ProxyID)
		if validateErr != nil {
			return nil, validateErr
		}
		account.ProxyID = validatedProxyID
	}
	if req.ShareMode != nil {
		// API-key accounts must remain private even if a stale client submits a
		// public share mode. OAuth accounts retain the normal approval flow.
		if account.Type == AccountTypeAPIKey {
			account.ShareMode = AccountShareModePrivate
			account.ShareStatus = AccountShareStatusApproved
		} else {
			account.ShareMode = NormalizeAccountShareMode(*req.ShareMode)
			if account.ShareMode == AccountShareModePublic && NormalizeAccountShareStatus(account.ShareStatus) == AccountShareStatusApproved {
				account.ShareStatus = AccountShareStatusPending
			}
			if account.ShareMode == AccountShareModePrivate {
				account.ShareStatus = AccountShareStatusApproved
			}
		}
	}
	if account.Type == AccountTypeAPIKey {
		account.ShareMode = AccountShareModePrivate
		account.ShareStatus = AccountShareStatusApproved
	}
	if err := validateConfiguredFreeModelSource(account.Platform, account.Type, account.Credentials, account.Extra); err != nil {
		return nil, err
	}
	if err := s.updateOwnedAccount(ctx, ownerUserID, account); err != nil {
		return nil, fmt.Errorf("update owned user account: %w", err)
	}
	return account, nil
}

// CreateOwnedUser creates an account using only user-controlled fields. The
// owner is always taken from the authenticated subject and all operational
// fields use the product's fixed personal-account defaults.
func (s *AccountService) CreateOwnedUser(ctx context.Context, ownerUserID int64, req UserAccountCreateRequest) (*Account, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.Type) == "" {
		return nil, infraerrors.BadRequest("OWNED_ACCOUNT_FIELDS_REQUIRED", "name, platform and type are required")
	}
	if err := validateUserAccountCreateMap(req.Credentials, "credentials", userAccountCreateForbiddenCredentialKeys); err != nil {
		return nil, err
	}
	if err := validateUserAccountCreateMap(req.Extra, "extra", userAccountCreateForbiddenExtraKeys); err != nil {
		return nil, err
	}
	if err := validateConfiguredFreeModelSource(req.Platform, req.Type, req.Credentials, req.Extra); err != nil {
		return nil, err
	}

	var validatedProxyID *int64
	if req.ProxyID != nil {
		var err error
		validatedProxyID, err = s.ValidateOwnedProxyID(ctx, ownerUserID, req.ProxyID)
		if err != nil {
			return nil, err
		}
	}

	shareMode := AccountShareModePrivate
	if req.ShareMode != nil {
		shareMode = NormalizeAccountShareMode(*req.ShareMode)
	}
	// An API key is a user secret and cannot be published through the user
	// account surface, regardless of a stale or forged share_mode value.
	if req.Type == AccountTypeAPIKey {
		shareMode = AccountShareModePrivate
	}
	shareStatus := AccountShareStatusApproved
	if shareMode == AccountShareModePublic {
		shareStatus = AccountShareStatusPending
	}

	account := &Account{
		Name:               req.Name,
		Notes:              normalizeAccountNotes(req.Notes),
		Platform:           req.Platform,
		Type:               req.Type,
		Credentials:        SanitizeStoredCredentials(req.Platform, req.Credentials),
		Extra:              cloneAccountMap(req.Extra),
		ProxyID:            validatedProxyID,
		OwnerUserID:        &ownerUserID,
		ShareMode:          shareMode,
		ShareStatus:        shareStatus,
		Concurrency:        userAccountDefaultConcurrency,
		Priority:           userAccountDefaultPriority,
		Status:             StatusActive,
		Schedulable:        true,
		AutoPauseOnExpired: true,
	}
	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create owned user account: %w", err)
	}
	return account, nil
}

// BulkUpdateOwnedUser performs the same safe mutation contract per account.
// One invalid account is reported independently so valid siblings still apply.
func (s *AccountService) BulkUpdateOwnedUser(ctx context.Context, ownerUserID int64, input *BulkUpdateOwnedUserAccountsInput) (*UserBulkAccountOperationResult, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	if input == nil || len(input.AccountIDs) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_IDS_REQUIRED", "account_ids is required")
	}
	if len(input.AccountIDs) > 1000 {
		return nil, infraerrors.BadRequest("ACCOUNT_BATCH_TOO_LARGE", "too many account_ids")
	}
	ids := uniquePositiveAccountIDs(input.AccountIDs)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_IDS_REQUIRED", "account_ids is required")
	}
	result := &UserBulkAccountOperationResult{Results: make([]UserBulkAccountResult, 0, len(ids))}
	for _, id := range ids {
		_, err := s.UpdateOwnedUser(ctx, ownerUserID, id, UserAccountUpdateRequest{
			ProxyID:     input.ProxyID,
			ShareMode:   input.ShareMode,
			Credentials: input.Credentials,
			Extra:       input.Extra,
		})
		entry := UserBulkAccountResult{AccountID: id, Success: err == nil}
		if err != nil {
			entry.Error = err.Error()
			result.Failed++
			result.FailedIDs = append(result.FailedIDs, id)
		} else {
			result.Success++
			result.SuccessIDs = append(result.SuccessIDs, id)
		}
		result.Results = append(result.Results, entry)
	}
	return result, nil
}
