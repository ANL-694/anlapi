package service

import (
	"context"
	"log/slog"
	"net/http"
)

// isOpenAICompatibleAccountEligibleForRequest applies the provider-neutral
// eligibility rules used by OpenAI-compatible scheduling paths.
func isOpenAICompatibleAccountEligibleForRequest(ctx context.Context, account *Account, platform string, requestedModel string, requireCompact bool, requiredCapability OpenAIEndpointCapability) bool {
	platform = normalizeOpenAICompatiblePlatform(platform)
	if account == nil || account.Platform != platform || !account.IsOpenAICompatible() || !account.IsSchedulableForModelWithContext(ctx, requestedModel) {
		return false
	}
	if account.IsOpenAI() {
		if paused, reason := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
			slog.Debug("account_auto_paused_by_quota",
				"account_id", account.ID,
				"window", reason.window,
				"threshold", reason.threshold,
				"utilization", reason.utilization,
			)
			return false
		}
	}
	if account.IsGrok() {
		if paused, reason := shouldAutoPauseGrokAccountByQuota(account); paused {
			slog.Debug("grok_account_auto_paused_by_quota",
				"account_id", account.ID,
				"window", reason.window,
				"threshold", reason.threshold,
				"utilization", reason.utilization,
			)
			return false
		}
	}
	if requestedModel != "" && !account.IsModelSupported(requestedModel) {
		return false
	}
	if !account.SupportsOpenAIEndpointCapability(requiredCapability) {
		if account.IsGrok() && requiredCapability == OpenAIEndpointCapabilityGrokMediaGeneration {
			_, reason := account.GrokMediaGenerationEligibility()
			slog.Debug("grok_media_account_ineligible", "account_id", account.ID, "reason", reason)
		}
		return false
	}
	if requireCompact && openAICompactSupportTier(account) == 0 {
		return false
	}
	return true
}

func (s *OpenAIGatewayService) isCarpoolSchedulingAccount(ctx context.Context, account *Account) bool {
	if s == nil || account == nil {
		return false
	}
	return isCarpoolSchedulingAccountAllowed(ctx, s.carpoolRepo, currentRequestGroupID(ctx), account)
}

func (s *OpenAIGatewayService) isAccountSchedulableForSchedulingRequest(ctx context.Context, account *Account) bool {
	if account == nil {
		return false
	}
	if account.IsSchedulable() {
		return true
	}
	if s.isCarpoolSchedulingAccount(ctx, account) {
		return isCarpoolAccountSchedulable(account)
	}
	return false
}

func (s *OpenAIGatewayService) isOpenAIAccountEligibleForSchedulingRequest(ctx context.Context, account *Account, platform string, requestedModel string, requireCompact bool, requiredCapability OpenAIEndpointCapability) bool {
	platform = normalizeOpenAICompatiblePlatform(platform)
	if account == nil || account.Platform != platform || !s.isAccountSchedulableForSchedulingRequest(ctx, account) || !account.IsOpenAICompatible() {
		return false
	}
	if account.IsOpenAI() {
		if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
			return false
		}
	}
	if account.IsGrok() {
		if paused, _ := shouldAutoPauseGrokAccountByQuota(account); paused {
			return false
		}
	}
	if requestedModel != "" && !account.IsModelSupported(requestedModel) {
		return false
	}
	if !account.SupportsOpenAIEndpointCapability(requiredCapability) {
		return false
	}
	if requireCompact && openAICompactSupportTier(account) == 0 {
		return false
	}
	return true
}

func (s *OpenAIGatewayService) shouldClearStickySessionForSchedulingRequest(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	if s.isCarpoolSchedulingAccount(ctx, account) {
		return !isCarpoolAccountSchedulable(account)
	}
	return shouldClearStickySession(account, requestedModel)
}

func (s *OpenAIGatewayService) shouldSkipPersistentRateLimitForCarpool(ctx context.Context, account *Account, statusCode int) bool {
	return statusCode == http.StatusTooManyRequests && s.isCarpoolSchedulingAccount(ctx, account)
}
