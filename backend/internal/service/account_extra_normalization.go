package service

import (
	"maps"

	infraerrors "anlapi/internal/pkg/errors"
)

func updatesUpstreamBillingProbeIdentity(credentials map[string]any) bool {
	for _, key := range []string{"api_key", "base_url", credKeyHeaderOverrideEnabled, credKeyHeaderOverrides} {
		if _, ok := credentials[key]; ok {
			return true
		}
	}
	return false
}

func normalizeOpenAILongContextBillingUpdateExtra(account *Account, input *UpdateAccountInput) (map[string]any, error) {
	normalized, err := normalizeOpenAILongContextBillingExtra(account.Platform, input.Extra)
	if err != nil || account.Platform != PlatformOpenAI {
		return normalized, err
	}

	_, provided := input.Extra[openAILongContextBillingEnabledKey]
	current, hasCurrent := account.Extra[openAILongContextBillingEnabledKey].(bool)
	if !provided && hasCurrent {
		normalized[openAILongContextBillingEnabledKey] = current
	}
	return normalized, nil
}

// ValidateGrokMediaEligibilityExtra validates the optional media-routing
// override. A null value removes the override and restores automatic routing.
func ValidateGrokMediaEligibilityExtra(platform string, extra map[string]any) error {
	if platform != PlatformGrok || extra == nil {
		return nil
	}
	raw, exists := extra[GrokMediaEligibleExtraKey]
	if !exists || raw == nil {
		return nil
	}
	if _, ok := raw.(bool); !ok {
		return infraerrors.BadRequest(
			"GROK_MEDIA_ELIGIBILITY_INVALID",
			"grok_media_eligible must be a boolean or null",
		)
	}
	return nil
}

func normalizeGrokMediaEligibilityExtra(platform string, extra map[string]any) (map[string]any, error) {
	if platform != PlatformGrok {
		return extra, nil
	}
	if err := ValidateGrokMediaEligibilityExtra(platform, extra); err != nil {
		return nil, err
	}
	normalized := maps.Clone(extra)
	if normalized != nil && normalized[GrokMediaEligibleExtraKey] == nil {
		delete(normalized, GrokMediaEligibleExtraKey)
	}
	return normalized, nil
}

func normalizeGrokMediaEligibilityUpdateExtra(account *Account, input *UpdateAccountInput, normalized map[string]any) (map[string]any, error) {
	if account == nil || account.Platform != PlatformGrok {
		return normalized, nil
	}
	if err := ValidateGrokMediaEligibilityExtra(account.Platform, input.Extra); err != nil {
		return nil, err
	}
	normalized = maps.Clone(normalized)
	if normalized == nil {
		normalized = make(map[string]any)
	}
	raw, provided := input.Extra[GrokMediaEligibleExtraKey]
	if provided {
		if raw == nil {
			delete(normalized, GrokMediaEligibleExtraKey)
		}
		return normalized, nil
	}
	if current, ok := account.Extra[GrokMediaEligibleExtraKey].(bool); ok {
		normalized[GrokMediaEligibleExtraKey] = current
	}
	return normalized, nil
}
