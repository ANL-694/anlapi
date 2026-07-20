package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	infraerrors "anlapi/internal/pkg/errors"
)

const maxAccountNameRunes = 100
const duplicateAccountOperationIDExtraKey = "duplicate_operation_id"

func duplicateAccountName(sourceName string) string {
	const suffix = " (Copy)"
	nameRunes := []rune(strings.TrimSpace(sourceName))
	maxBaseRunes := maxAccountNameRunes - len([]rune(suffix))
	if len(nameRunes) > maxBaseRunes {
		nameRunes = nameRunes[:maxBaseRunes]
	}
	return string(nameRunes) + suffix
}

func cloneAccountJSONMap(value map[string]any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	cloned := make(map[string]any, len(value))
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

var duplicateAccountDiscardedExtraKeys = map[string]struct{}{
	// A retry identity belongs to the operation that created one copy, not to later copies.
	duplicateAccountOperationIDExtraKey: {},
	// External sync identity belongs to one local account only.
	"crs_account_id": {},
	"crs_kind":       {},
	"crs_synced_at":  {},
	// Local quota usage and derived window timestamps must start fresh.
	"quota_used":            {},
	"quota_daily_used":      {},
	"quota_weekly_used":     {},
	"quota_daily_start":     {},
	"quota_weekly_start":    {},
	"quota_daily_reset_at":  {},
	"quota_weekly_reset_at": {},
	// Provider observations, capability probes, and transient scheduling state.
	"model_rate_limits":                      {},
	"session_window_utilization":             {},
	"passive_usage_7d_utilization":           {},
	"passive_usage_7d_reset":                 {},
	"passive_usage_7d_oi_utilization":        {},
	"passive_usage_7d_oi_reset":              {},
	"passive_usage_sampled_at":               {},
	"grok_usage_snapshot":                    {},
	"grok_billing_snapshot":                  {},
	"openai_responses_supported":             {},
	"openai_compact_supported":               {},
	"openai_compact_checked_at":              {},
	"openai_compact_last_status":             {},
	"openai_compact_last_error":              {},
	"antigravity_credits_overages":           {},
	"antigravity_force_token_refresh":        {},
	"antigravity_force_token_refresh_at":     {},
	"antigravity_force_token_refresh_reason": {},
	"drive_storage_limit":                    {},
	"drive_storage_usage":                    {},
	"drive_tier_updated_at":                  {},
	"codex_primary_used_percent":             {},
	"codex_primary_reset_after_seconds":      {},
	"codex_primary_window_minutes":           {},
	"codex_secondary_used_percent":           {},
	"codex_secondary_reset_after_seconds":    {},
	"codex_secondary_window_minutes":         {},
	"codex_primary_over_secondary_percent":   {},
	"codex_usage_updated_at":                 {},
	"codex_5h_used_percent":                  {},
	"codex_5h_reset_after_seconds":           {},
	"codex_5h_window_minutes":                {},
	"codex_5h_reset_at":                      {},
	"codex_7d_used_percent":                  {},
	"codex_7d_reset_after_seconds":           {},
	"codex_7d_window_minutes":                {},
	"codex_7d_reset_at":                      {},
}

func duplicateAccountExtra(value map[string]any) (map[string]any, error) {
	cloned, err := cloneAccountJSONMap(value)
	if err != nil {
		return nil, err
	}
	for key := range duplicateAccountDiscardedExtraKeys {
		delete(cloned, key)
	}
	return cloned, nil
}

func canDuplicateAccountType(accountType string) bool {
	switch accountType {
	case AccountTypeAPIKey, AccountTypeUpstream, AccountTypeBedrock, AccountTypeServiceAccount:
		return true
	default:
		return false
	}
}

func duplicateAccountGroups(source *Account) ([]AccountGroup, []int64) {
	if len(source.AccountGroups) > 0 {
		groups := make([]AccountGroup, 0, len(source.AccountGroups))
		groupIDs := make([]int64, 0, len(source.AccountGroups))
		for _, sourceGroup := range source.AccountGroups {
			groups = append(groups, AccountGroup{GroupID: sourceGroup.GroupID, Priority: sourceGroup.Priority})
			groupIDs = append(groupIDs, sourceGroup.GroupID)
		}
		return groups, groupIDs
	}

	groups := make([]AccountGroup, 0, len(source.GroupIDs))
	groupIDs := append([]int64(nil), source.GroupIDs...)
	for i, groupID := range groupIDs {
		groups = append(groups, AccountGroup{GroupID: groupID, Priority: i + 1})
	}
	return groups, groupIDs
}

func duplicateAccountOperationID(sourceID int64, actorScope, operationKey string) string {
	operationKey = strings.TrimSpace(operationKey)
	if operationKey == "" {
		return ""
	}
	actorScope = strings.TrimSpace(actorScope)
	if actorScope == "" {
		actorScope = "admin:0"
	}
	payload := "admin.accounts.duplicate\x00" + actorScope + "\x00" + strconv.FormatInt(sourceID, 10) + "\x00" + operationKey
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", digest)
}

func (s *adminServiceImpl) findDuplicateByOperationID(ctx context.Context, operationID string) (*Account, error) {
	if operationID == "" {
		return nil, nil
	}
	accounts, err := s.accountRepo.FindByExtraField(ctx, duplicateAccountOperationIDExtraKey, operationID)
	if err != nil {
		return nil, fmt.Errorf("find duplicate account operation: %w", err)
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	account := accounts[0]
	return &account, nil
}

// RecoverDuplicateAccount performs a read-only lookup for an already committed duplicate.
// It is used when the idempotency coordinator cannot confirm whether response persistence
// succeeded, and deliberately never repeats the create side effect.
func (s *adminServiceImpl) RecoverDuplicateAccount(ctx context.Context, id int64, actorScope, operationKey string) (*Account, error) {
	return s.findDuplicateByOperationID(ctx, duplicateAccountOperationID(id, actorScope, operationKey))
}

func cloneAccountValuePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// DuplicateAccount creates a paused account from source configuration without carrying first-class
// runtime state. Credentials and extra configuration are deep-copied so normalization of the new
// account cannot mutate the in-memory source. Linked credential shadows are excluded because they
// intentionally do not own credentials and must be created through CreateShadow.
func (s *adminServiceImpl) DuplicateAccount(ctx context.Context, id int64, actorScope, operationKey string) (*Account, error) {
	operationID := duplicateAccountOperationID(id, actorScope, operationKey)
	existing, err := s.RecoverDuplicateAccount(ctx, id, actorScope, operationKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	source, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if source.IsCredentialShadow() {
		return nil, infraerrors.BadRequest(
			"ACCOUNT_DUPLICATE_SHADOW_UNSUPPORTED",
			"linked credential shadow accounts cannot be duplicated; duplicate the parent account instead",
		)
	}
	if !canDuplicateAccountType(source.Type) {
		return nil, infraerrors.BadRequest(
			"ACCOUNT_DUPLICATE_CREDENTIAL_TYPE_UNSUPPORTED",
			"accounts with rotating or unsupported credential types cannot be duplicated",
		)
	}

	credentials, err := cloneAccountJSONMap(source.Credentials)
	if err != nil {
		return nil, fmt.Errorf("clone account credentials: %w", err)
	}
	extra, err := duplicateAccountExtra(source.Extra)
	if err != nil {
		return nil, fmt.Errorf("clone account extra configuration: %w", err)
	}
	if operationID != "" {
		if extra == nil {
			extra = make(map[string]any, 1)
		}
		extra[duplicateAccountOperationIDExtraKey] = operationID
	}

	var expiresAt *int64
	if source.ExpiresAt != nil {
		unix := source.ExpiresAt.Unix()
		expiresAt = &unix
	}
	autoPauseOnExpired := source.AutoPauseOnExpired
	groups, groupIDs := duplicateAccountGroups(source)
	proxyID := source.ProxyID
	if source.ProxyFallbackOriginID != nil {
		// Proxy fallback is transient runtime state; duplicate the configured origin.
		proxyID = source.ProxyFallbackOriginID
	}
	input := &CreateAccountInput{
		Name:                  duplicateAccountName(source.Name),
		Notes:                 cloneAccountValuePointer(source.Notes),
		Platform:              source.Platform,
		AccountLevel:          source.AccountLevel,
		Type:                  source.Type,
		Credentials:           credentials,
		Extra:                 extra,
		OwnerUserID:           cloneAccountValuePointer(source.OwnerUserID),
		ShareMode:             source.ShareMode,
		ShareStatus:           source.ShareStatus,
		SharePolicyID:         cloneAccountValuePointer(source.SharePolicyID),
		ProxyID:               cloneAccountValuePointer(proxyID),
		Concurrency:           source.Concurrency,
		Priority:              source.Priority,
		RateMultiplier:        cloneAccountValuePointer(source.RateMultiplier),
		LoadFactor:            cloneAccountValuePointer(source.LoadFactor),
		GroupIDs:              groupIDs,
		ExpiresAt:             expiresAt,
		AutoPauseOnExpired:    &autoPauseOnExpired,
		SkipDefaultGroupBind:  true,
		SkipMixedChannelCheck: true,
	}
	accountExtra, err := normalizeDuplicateOpenAILongContextBillingExtra(input.Platform, input.Extra)
	if err != nil {
		return nil, fmt.Errorf("normalize duplicate account extra: %w", err)
	}
	if err := NormalizeHeaderOverrideCredentials(input.Credentials); err != nil {
		return nil, err
	}
	duplicate, err := buildDuplicateAccountForCreate(input, accountExtra)
	if err != nil {
		return nil, err
	}
	// A copied credential must be reviewed before it can share live traffic with its source.
	duplicate.Schedulable = false
	if s.accountDuplicateRepo == nil {
		return nil, errors.New("account duplicate repository is not configured")
	}
	if err := s.accountDuplicateRepo.CreateWithAccountGroups(ctx, duplicate, groups); err != nil {
		return nil, fmt.Errorf("create duplicate account: %w", err)
	}
	for i := range groups {
		groups[i].AccountID = duplicate.ID
	}
	duplicate.AccountGroups = groups
	duplicate.GroupIDs = groupIDs
	return duplicate, nil
}

func normalizeDuplicateOpenAILongContextBillingExtra(platform string, extra map[string]any) (map[string]any, error) {
	if platform != PlatformOpenAI {
		return extra, nil
	}
	if err := ValidateOpenAILongContextBillingExtra(platform, extra); err != nil {
		return nil, err
	}
	normalized := make(map[string]any, len(extra)+1)
	for key, value := range extra {
		normalized[key] = value
	}
	if _, exists := normalized[openAILongContextBillingEnabledKey]; !exists {
		normalized[openAILongContextBillingEnabledKey] = false
	}
	return normalized, nil
}

func ValidateOpenAILongContextBillingExtra(platform string, extra map[string]any) error {
	if platform != PlatformOpenAI {
		return nil
	}
	raw, exists := extra[openAILongContextBillingEnabledKey]
	if !exists {
		return nil
	}
	if _, ok := raw.(bool); !ok {
		return infraerrors.BadRequest(
			"OPENAI_LONG_CONTEXT_BILLING_INVALID",
			"openai_long_context_billing_enabled must be a boolean",
		)
	}
	return nil
}

func normalizeOpenAILongContextBillingExtra(platform string, extra map[string]any) (map[string]any, error) {
	return normalizeDuplicateOpenAILongContextBillingExtra(platform, extra)
}

func buildDuplicateAccountForCreate(input *CreateAccountInput, accountExtra map[string]any) (*Account, error) {
	delete(accountExtra, UpstreamBillingProbeEnabledExtraKey)
	delete(accountExtra, UpstreamBillingProbeExtraKey)
	accountLevel := NormalizeOpenAIAccountLevel(input.Platform, input.AccountLevel, input.Credentials, accountExtra)
	concurrency, err := NormalizeOpenAIPlusConcurrency(input.Platform, accountLevel, input.Concurrency)
	if err != nil {
		return nil, err
	}
	account := &Account{
		Name:          input.Name,
		Notes:         normalizeAccountNotes(input.Notes),
		Platform:      input.Platform,
		AccountLevel:  accountLevel,
		Type:          input.Type,
		Credentials:   input.Credentials,
		Extra:         accountExtra,
		OwnerUserID:   input.OwnerUserID,
		ShareMode:     NormalizeAccountShareMode(input.ShareMode),
		ShareStatus:   NormalizeAccountShareStatus(input.ShareStatus),
		SharePolicyID: input.SharePolicyID,
		ProxyID:       input.ProxyID,
		Concurrency:   normalizeAccountConcurrency(input.Platform, input.Type, concurrency),
		Priority:      input.Priority,
		Status:        StatusActive,
		Schedulable:   true,
	}
	if account.Extra != nil {
		if err := ValidateQuotaResetConfig(account.Extra); err != nil {
			return nil, err
		}
		ComputeQuotaResetAt(account.Extra)
	}
	if input.ExpiresAt != nil && *input.ExpiresAt > 0 {
		expiresAt := time.Unix(*input.ExpiresAt, 0)
		account.ExpiresAt = &expiresAt
	}
	if input.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *input.AutoPauseOnExpired
	} else {
		account.AutoPauseOnExpired = true
	}
	if input.RateMultiplier != nil {
		if *input.RateMultiplier < 0 {
			return nil, errors.New("rate_multiplier must be >= 0")
		}
		account.RateMultiplier = input.RateMultiplier
	}
	if input.LoadFactor != nil && *input.LoadFactor > 0 {
		if err := ValidateAccountLoadFactor(input.LoadFactor); err != nil {
			return nil, err
		}
		account.LoadFactor = input.LoadFactor
	}
	return account, nil
}
