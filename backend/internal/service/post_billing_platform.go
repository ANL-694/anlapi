package service

import (
	"context"
	"sync/atomic"
	"time"

	"anlapi/internal/pkg/ctxkey"
	"anlapi/internal/pkg/logger"
)

var (
	userPlatformQuotaDBIncrErrorTotal           atomic.Int64
	userPlatformQuotaDBIncrLegacyErrorTotal     atomic.Int64
	userPlatformQuotaSentinelSetCacheErrorTotal atomic.Int64
)

func PlatformFromAPIKey(apiKey *APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

func QuotaPlatform(ctx context.Context, apiKey *APIKey) string {
	if ctx != nil {
		if forced, ok := ctx.Value(ctxkey.ForcePlatform).(string); ok && forced != "" {
			return forced
		}
	}
	platform := PlatformFromAPIKey(apiKey)
	if platform != PlatformComposite || apiKey == nil || apiKey.Group == nil {
		return platform
	}
	if resolved, ok := ResolvedTargetPlatformForGroup(ctx, apiKey.Group.ID); ok {
		return resolved
	}
	return ""
}

// platformQuotaUsageCost 返回应计入余额平台配额的费用。
// 普通订阅由订阅额度约束；私人订阅仍会扣钱包佣金，因此只累计实际佣金。
func platformQuotaUsageCost(p *postUsageBillingParams, result *UsageBillingApplyResult) float64 {
	if p == nil || p.Cost == nil || p.Cost.ActualCost <= 0 {
		return 0
	}
	if !p.IsSubscriptionBill {
		return p.Cost.ActualCost
	}
	if p.APIKey == nil || p.APIKey.Group == nil || !p.APIKey.Group.IsUserPrivateScope() {
		return 0
	}
	if result != nil && result.CommissionDeducted > 0 {
		return result.CommissionDeducted
	}
	return calculatePrivateGroupCommissionCost(p)
}

// recordUserPlatformQuotaUsage 同步更新 Redis enforcement 状态，并按配置持久化到 DB。
func recordUserPlatformQuotaUsage(ctx context.Context, p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult, syncDB bool) {
	if p == nil || p.User == nil || deps == nil || deps.billingCacheService == nil || deps.userPlatformQuotaRepo == nil || !IsAllowedQuotaPlatform(p.Platform) {
		return
	}
	cost := platformQuotaUsageCost(p, result)
	if cost <= 0 {
		return
	}

	billingCtx, cancel := detachedBillingContext(ctx)
	if !deps.billingCacheService.HasUserPlatformQuotaLimit(billingCtx, p.User.ID, p.Platform) {
		cancel()
		return
	}
	deps.billingCacheService.IncrementUserPlatformQuotaUsage(p.User.ID, p.Platform, cost)
	if deps.cfg != nil && deps.cfg.Database.UserPlatformQuotaFlusherEnabled {
		cancel()
		return
	}

	write := func() {
		defer cancel()
		if err := deps.userPlatformQuotaRepo.IncrementUsageWithReset(billingCtx, p.User.ID, p.Platform, cost, time.Now().UTC()); err != nil {
			if syncDB {
				userPlatformQuotaDBIncrLegacyErrorTotal.Add(1)
			} else {
				userPlatformQuotaDBIncrErrorTotal.Add(1)
			}
			logger.LegacyPrintf("service.gateway", "ALERT: increment user platform quota DB failed user=%d platform=%s cost=%f: %v", p.User.ID, p.Platform, cost, err)
		}
	}
	if syncDB {
		write()
		return
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				userPlatformQuotaDBIncrErrorTotal.Add(1)
				logger.LegacyPrintf("service.gateway", "ALERT: panic in user platform quota increment user=%d platform=%s: %v", p.User.ID, p.Platform, recovered)
			}
		}()
		write()
	}()
}

func GatewayUserPlatformQuotaIncrStats() (mainPathErr, legacyPathErr, sentinelSetErr int64) {
	return userPlatformQuotaDBIncrErrorTotal.Load(),
		userPlatformQuotaDBIncrLegacyErrorTotal.Load(),
		userPlatformQuotaSentinelSetCacheErrorTotal.Load()
}

func GatewayUserPlatformQuotaFlusherStats(flusher *UserPlatformQuotaUsageFlusher) map[string]int64 {
	if flusher == nil || flusher.metrics == nil {
		return nil
	}
	metrics := flusher.metrics
	return map[string]int64{
		"flush_success":        metrics.FlushSuccessTotal.Load(),
		"flush_error":          metrics.FlushErrorTotal.Load(),
		"flush_batch_size":     metrics.FlushBatchSizeTotal.Load(),
		"flush_latency_ms_max": metrics.FlushLatencyMsMax.Load(),
		"dirty_readd":          metrics.DirtyReaddTotal.Load(),
		"dirty_lost":           metrics.DirtyLostTotal.Load(),
		"flush_fk_violation":   metrics.FlushFKViolationTotal.Load(),
	}
}
