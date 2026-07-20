package service

import (
	"context"
	"time"

	"anlapi/internal/pkg/logger"
)

// snapshotPlatformQuotaDefaults 把注册时解析出的模板复制到用户独立配额记录。
func (s *AuthService) snapshotPlatformQuotaDefaults(ctx context.Context, userID int64, plan *signupGrantPlan) error {
	if s == nil || s.userPlatformQuotaRepo == nil || plan == nil || userID <= 0 || len(plan.PlatformQuotas) == 0 {
		return nil
	}
	snapshotCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	records := make([]UserPlatformQuotaRecord, 0, len(plan.PlatformQuotas))
	for platform, quota := range plan.PlatformQuotas {
		record := UserPlatformQuotaRecord{UserID: userID, Platform: platform}
		if quota != nil {
			record.DailyLimitUSD = quota.DailyLimitUSD
			record.WeeklyLimitUSD = quota.WeeklyLimitUSD
			record.MonthlyLimitUSD = quota.MonthlyLimitUSD
		}
		records = append(records, record)
	}
	if err := s.userPlatformQuotaRepo.BulkInsertInitial(snapshotCtx, records); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Warning: snapshot platform quota failed user=%d: %v (fail-open)", userID, err)
	}
	return nil
}
