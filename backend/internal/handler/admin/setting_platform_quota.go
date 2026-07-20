package admin

import "anl-api/internal/service"

func equalDefaultPlatformQuotas(a, b map[string]*service.DefaultPlatformQuotaSetting) bool {
	if len(a) != len(b) {
		return false
	}
	for platform, left := range a {
		right, ok := b[platform]
		if !ok || !equalDefaultPlatformQuota(left, right) {
			return false
		}
	}
	return true
}

func equalDefaultPlatformQuota(a, b *service.DefaultPlatformQuotaSetting) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalOptionalFloat(a.DailyLimitUSD, b.DailyLimitUSD) &&
		equalOptionalFloat(a.WeeklyLimitUSD, b.WeeklyLimitUSD) &&
		equalOptionalFloat(a.MonthlyLimitUSD, b.MonthlyLimitUSD)
}

func equalOptionalFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func platformQuotaMapValueOrDefault(
	providedFields map[string]struct{},
	field string,
	value map[string]*service.DefaultPlatformQuotaSetting,
	fallback map[string]*service.DefaultPlatformQuotaSetting,
) map[string]*service.DefaultPlatformQuotaSetting {
	if _, provided := providedFields[field]; provided {
		if value == nil {
			return map[string]*service.DefaultPlatformQuotaSetting{}
		}
		return cloneDefaultPlatformQuotaMap(value)
	}
	return cloneDefaultPlatformQuotaMap(fallback)
}

func cloneDefaultPlatformQuotaMap(input map[string]*service.DefaultPlatformQuotaSetting) map[string]*service.DefaultPlatformQuotaSetting {
	if input == nil {
		return nil
	}
	out := make(map[string]*service.DefaultPlatformQuotaSetting, len(input))
	for platform, quota := range input {
		if quota == nil {
			out[platform] = nil
			continue
		}
		copy := *quota
		out[platform] = &copy
	}
	return out
}
