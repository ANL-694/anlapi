package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	infraerrors "ikik-api/internal/pkg/errors"
)

// DefaultPlatformQuotaSetting 是新用户注册时复制的平台配额模板。
type DefaultPlatformQuotaSetting struct {
	DailyLimitUSD   *float64 `json:"daily"`
	WeeklyLimitUSD  *float64 `json:"weekly"`
	MonthlyLimitUSD *float64 `json:"monthly"`
}

func (s *SettingService) GetDefaultPlatformQuotas(ctx context.Context) (map[string]*DefaultPlatformQuotaSetting, error) {
	out := emptyDefaultPlatformQuotaMap()
	if s == nil || s.settingRepo == nil {
		return out, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultPlatformQuotas)
	if err != nil || raw == "" {
		return out, nil
	}
	parsed := map[string]*DefaultPlatformQuotaSetting{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		slog.Warn("[Setting] unmarshal default_platform_quotas failed (fail-open)", "error", err)
		return out, nil
	}
	for _, platform := range AllowedQuotaPlatforms {
		if value := parsed[platform]; value != nil {
			out[platform] = cloneDefaultPlatformQuota(value)
		}
	}
	return out, nil
}

func (s *SettingService) GetAuthSourcePlatformQuotas(ctx context.Context, source string) map[string]*DefaultPlatformQuotaSetting {
	out := map[string]*DefaultPlatformQuotaSetting{}
	if s == nil || s.settingRepo == nil {
		return out
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAuthSourcePlatformQuotas(source))
	if err != nil || raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		slog.Warn("[Setting] unmarshal auth source platform quotas failed (fail-open)", "source", source, "error", err)
		return map[string]*DefaultPlatformQuotaSetting{}
	}
	return out
}

func mergePlatformQuotaDefaults(dst, src *DefaultPlatformQuotaSetting) {
	if dst == nil || src == nil {
		return
	}
	if src.DailyLimitUSD != nil {
		dst.DailyLimitUSD = src.DailyLimitUSD
	}
	if src.WeeklyLimitUSD != nil {
		dst.WeeklyLimitUSD = src.WeeklyLimitUSD
	}
	if src.MonthlyLimitUSD != nil {
		dst.MonthlyLimitUSD = src.MonthlyLimitUSD
	}
}

func validateDefaultPlatformQuotaMap(values map[string]*DefaultPlatformQuotaSetting) error {
	for platform, quota := range values {
		if !IsAllowedQuotaPlatform(platform) {
			return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", fmt.Sprintf("unknown platform %q", platform))
		}
		if quota == nil {
			continue
		}
		for _, value := range []*float64{quota.DailyLimitUSD, quota.WeeklyLimitUSD, quota.MonthlyLimitUSD} {
			if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", "platform quota limit must be a finite non-negative number")
			}
		}
	}
	return nil
}

func parseDefaultPlatformQuotaSettings(raw string) map[string]*DefaultPlatformQuotaSetting {
	out := emptyDefaultPlatformQuotaMap()
	if raw == "" {
		return out
	}
	parsed := map[string]*DefaultPlatformQuotaSetting{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	for _, platform := range AllowedQuotaPlatforms {
		if value := parsed[platform]; value != nil {
			out[platform] = cloneDefaultPlatformQuota(value)
		}
	}
	return out
}

func emptyDefaultPlatformQuotaMap() map[string]*DefaultPlatformQuotaSetting {
	out := make(map[string]*DefaultPlatformQuotaSetting, len(AllowedQuotaPlatforms))
	for _, platform := range AllowedQuotaPlatforms {
		out[platform] = &DefaultPlatformQuotaSetting{}
	}
	return out
}

func cloneDefaultPlatformQuota(value *DefaultPlatformQuotaSetting) *DefaultPlatformQuotaSetting {
	if value == nil {
		return &DefaultPlatformQuotaSetting{}
	}
	return &DefaultPlatformQuotaSetting{
		DailyLimitUSD: value.DailyLimitUSD, WeeklyLimitUSD: value.WeeklyLimitUSD, MonthlyLimitUSD: value.MonthlyLimitUSD,
	}
}
