package service

import (
	"context"
	"strings"

	"ikik-api/internal/config"
)

// DiagnoseModelAvailabilityForPlatform reports whether the requested model is
// configured on any OpenAI-compatible account in the group.
func (s *OpenAIGatewayService) DiagnoseModelAvailabilityForPlatform(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	platform string,
) ModelAvailabilityDiagnosis {
	if s == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	repo, ok := s.accountRepo.(ModelAvailabilityCandidateRepository)
	if !ok || repo == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	platform = normalizeOpenAICompatiblePlatform(platform)
	queryGroupID := groupID
	includeGrouped := false
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		queryGroupID = nil
		includeGrouped = true
	}
	accounts, err := repo.ListModelAvailabilityCandidates(ctx, queryGroupID, []string{platform}, includeGrouped)
	if err != nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	diag := ModelAvailabilityDiagnosis{}
	for i := range accounts {
		diag.HasAccountsInPool = true
		if accounts[i].IsModelSupported(requestedModel) {
			diag.HasModelSupport = true
			return diag
		}
	}
	return diag
}
