package service

import (
	"context"
	"time"

	"anlapi/internal/pkg/logger"
)

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
				logger.LegacyPrintf("service.gateway", "ALERT: panic in user platform quota increment user=%d platform=%s: %v", p.User.ID, p.Platform, recovered)
			}
		}()
		write()
	}()
}
