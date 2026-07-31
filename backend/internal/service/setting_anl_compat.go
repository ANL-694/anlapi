package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	SettingKeyHomeStatsGroupID                          = "home_stats_group_id"
	SettingKeySystemImageGenerationGroupID              = "system_image_generation_group_id"
	SettingKeyUserPrivateGroupDailyLimitUSD             = "user_private_group_daily_limit_usd"
	SettingKeyUserPrivateGroupWeeklyLimitUSD            = "user_private_group_weekly_limit_usd"
	SettingKeyUserPrivateGroupMonthlyLimitUSD           = "user_private_group_monthly_limit_usd"
	SettingKeyUserPrivateGroupRateMultiplier            = "user_private_group_rate_multiplier"
	SettingKeyUserPrivateGroupRPMLimit                  = "user_private_group_rpm_limit"
	SettingKeyUserPrivateGroupCommissionRate            = "user_private_group_commission_rate"
	SettingKeyAutoModelSettings                         = "auto_model_settings"
	SettingKeyFreeModelsEnabled                         = "free_models_enabled"
	SettingKeyCarpoolEnabled                            = "carpool_enabled"
	SettingKeyCarpoolBaseServiceFeeUSD                  = "carpool_base_service_fee_usd"
	SettingKeyCarpoolSystemProxyFeeUSD                  = "carpool_system_proxy_fee_usd"
	SettingKeyCarpoolRiskControlFeeUSD                  = "carpool_risk_control_fee_usd"
	SettingKeyOpenAIFreeAccountRepairEnabled            = "openai_free_account_repair_enabled"
	SettingKeyOpenAIFreeAccountRepairWeeklyThresholdUSD = "openai_free_account_repair_weekly_threshold_usd"
)

const (
	CarpoolBaseServiceFeeUSDDefault = 75.0
	CarpoolSystemProxyFeeUSDDefault = 10.0
	CarpoolRiskControlFeeUSDDefault = 15.0
)

type UserPrivateGroupTemplate struct {
	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64
	RateMultiplier  float64
	RPMLimit        int
	CommissionRate  float64
}

func (s *SettingService) IsCarpoolEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyCarpoolEnabled)
	return err == nil && value == "true"
}

func (s *SettingService) GetOpenAIFreeAccountRepairSettings(ctx context.Context) (enabled bool, weeklyThresholdUSD float64) {
	if s == nil || s.settingRepo == nil {
		return false, 0
	}
	rawEnabled, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIFreeAccountRepairEnabled)
	if err != nil || !strings.EqualFold(strings.TrimSpace(rawEnabled), "true") {
		return false, 0
	}
	threshold := 60.0
	rawThreshold, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIFreeAccountRepairWeeklyThresholdUSD)
	if err == nil && strings.TrimSpace(rawThreshold) != "" {
		parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(rawThreshold), 64)
		if parseErr != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return false, 0
		}
		threshold = parsed
	} else if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return false, 0
	}
	return true, threshold
}

func (s *SettingService) GetSystemImageGenerationGroupID(ctx context.Context) (int64, error) {
	if s == nil || s.settingRepo == nil {
		return 0, ErrServiceUnavailable
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeySystemImageGenerationGroupID)
	if errors.Is(err, ErrSettingNotFound) || strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get system image generation group setting: %w", err)
	}
	groupID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse system image generation group setting: %w", err)
	}
	if groupID < 0 {
		return 0, fmt.Errorf("system image generation group setting must be non-negative")
	}
	return groupID, nil
}

func (s *SettingService) GetUserPrivateGroupTemplate(ctx context.Context) (*UserPrivateGroupTemplate, error) {
	if s == nil || s.settingRepo == nil {
		return nil, ErrServiceUnavailable
	}
	template := &UserPrivateGroupTemplate{
		RateMultiplier: 1,
		CommissionRate: 0.005,
	}
	var err error
	if template.DailyLimitUSD, err = s.userPrivateGroupLimit(ctx, SettingKeyUserPrivateGroupDailyLimitUSD); err != nil {
		return nil, err
	}
	if template.WeeklyLimitUSD, err = s.userPrivateGroupLimit(ctx, SettingKeyUserPrivateGroupWeeklyLimitUSD); err != nil {
		return nil, err
	}
	if template.MonthlyLimitUSD, err = s.userPrivateGroupLimit(ctx, SettingKeyUserPrivateGroupMonthlyLimitUSD); err != nil {
		return nil, err
	}
	if raw, err := s.optionalSettingValue(ctx, SettingKeyUserPrivateGroupRateMultiplier); err != nil {
		return nil, err
	} else if raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("parse user private group rate multiplier: %w", parseErr)
		}
		if value > 0 {
			template.RateMultiplier = value
		}
	}
	if raw, err := s.optionalSettingValue(ctx, SettingKeyUserPrivateGroupRPMLimit); err != nil {
		return nil, err
	} else if raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("parse user private group rpm limit: %w", parseErr)
		}
		if value > 0 {
			template.RPMLimit = value
		}
	}
	if raw, err := s.optionalSettingValue(ctx, SettingKeyUserPrivateGroupCommissionRate); err != nil {
		return nil, err
	} else if raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("parse user private group commission rate: %w", parseErr)
		}
		if value >= 0 {
			template.CommissionRate = value
		}
	}
	return template, nil
}

func (s *SettingService) AddContentModerationGroup(ctx context.Context, groupID int64) error {
	return s.updateContentModerationGroup(ctx, groupID, true)
}

func (s *SettingService) RemoveContentModerationGroup(ctx context.Context, groupID int64) error {
	return s.updateContentModerationGroup(ctx, groupID, false)
}

func (s *SettingService) optionalSettingValue(ctx context.Context, key string) (string, error) {
	raw, err := s.settingRepo.GetValue(ctx, key)
	if errors.Is(err, ErrSettingNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get %s: %w", key, err)
	}
	return strings.TrimSpace(raw), nil
}

func (s *SettingService) userPrivateGroupLimit(ctx context.Context, key string) (*float64, error) {
	raw, err := s.optionalSettingValue(ctx, key)
	if err != nil || raw == "" {
		return nil, err
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", key, err)
	}
	if value <= 0 {
		return nil, nil
	}
	return &value, nil
}

func (s *SettingService) updateContentModerationGroup(ctx context.Context, groupID int64, add bool) error {
	if s == nil || s.settingRepo == nil {
		return ErrServiceUnavailable
	}
	if groupID <= 0 {
		return nil
	}
	raw, err := s.optionalSettingValue(ctx, SettingKeyContentModerationConfig)
	if err != nil {
		return err
	}
	config, err := parseContentModerationConfig(raw)
	if err != nil {
		return fmt.Errorf("parse content moderation config: %w", err)
	}
	groupIDs := append([]int64(nil), config.GroupIDs...)
	if add {
		groupIDs = append(groupIDs, groupID)
	} else {
		filtered := groupIDs[:0]
		for _, configuredID := range groupIDs {
			if configuredID != groupID {
				filtered = append(filtered, configuredID)
			}
		}
		groupIDs = filtered
	}
	config.GroupIDs = normalizeInt64IDs(groupIDs)
	config.normalize()
	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal content moderation config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyContentModerationConfig, string(encoded)); err != nil {
		return fmt.Errorf("save content moderation config: %w", err)
	}
	return nil
}

func formatPositiveOptionalFloat(value *float64) string {
	if value == nil || *value <= 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "0"
	}
	return strconv.FormatFloat(*value, 'f', 8, 64)
}

func formatNonNegativeSettingFloat(value float64, fallback float64) string {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		value = fallback
	}
	return strconv.FormatFloat(value, 'f', 8, 64)
}

func parseNonNegativeSettingFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	return parsed
}

func parseNonNegativeSettingInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func parsePositiveOptionalFloat(value string) *float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}
