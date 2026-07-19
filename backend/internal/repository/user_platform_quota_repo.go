package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "ikik-api/ent"
	"ikik-api/ent/userplatformquota"
	"ikik-api/internal/pkg/timezone"
	"ikik-api/internal/service"

	"github.com/lib/pq"
)

type userPlatformQuotaRepository struct {
	client *dbent.Client
}

func NewUserPlatformQuotaRepository(client *dbent.Client) service.UserPlatformQuotaRepository {
	return &userPlatformQuotaRepository{client: client}
}

func (r *userPlatformQuotaRepository) BulkInsertInitial(ctx context.Context, records []service.UserPlatformQuotaRecord) error {
	if len(records) == 0 {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	var query strings.Builder
	_, _ = query.WriteString("INSERT INTO user_platform_quotas (user_id, platform, daily_limit_usd, weekly_limit_usd, monthly_limit_usd, daily_usage_usd, weekly_usage_usd, monthly_usage_usd, created_at, updated_at) VALUES ")
	args := make([]any, 0, len(records)*6)
	now := time.Now()
	for i, record := range records {
		if i > 0 {
			_, _ = query.WriteString(",")
		}
		base := i * 6
		fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,0,0,0,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+6)
		args = append(args, record.UserID, record.Platform, record.DailyLimitUSD, record.WeeklyLimitUSD, record.MonthlyLimitUSD, now)
	}
	_, _ = query.WriteString(` ON CONFLICT (user_id, platform) WHERE deleted_at IS NULL
		DO UPDATE SET
			daily_limit_usd = COALESCE(user_platform_quotas.daily_limit_usd, EXCLUDED.daily_limit_usd),
			weekly_limit_usd = COALESCE(user_platform_quotas.weekly_limit_usd, EXCLUDED.weekly_limit_usd),
			monthly_limit_usd = COALESCE(user_platform_quotas.monthly_limit_usd, EXCLUDED.monthly_limit_usd),
			updated_at = EXCLUDED.updated_at`)
	_, err := client.ExecContext(ctx, query.String(), args...)
	return err
}

func (r *userPlatformQuotaRepository) GetByUserPlatform(ctx context.Context, userID int64, platform string) (*service.UserPlatformQuotaRecord, error) {
	entity, err := clientFromContext(ctx, r.client).UserPlatformQuota.Query().
		Where(
			userplatformquota.UserIDEQ(userID),
			userplatformquota.PlatformEQ(platform),
			userplatformquota.DeletedAtIsNil(),
		).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return platformQuotaRecord(entity), nil
}

func (r *userPlatformQuotaRepository) ListByUser(ctx context.Context, userID int64) ([]service.UserPlatformQuotaRecord, error) {
	entities, err := clientFromContext(ctx, r.client).UserPlatformQuota.Query().
		Where(userplatformquota.UserIDEQ(userID), userplatformquota.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]service.UserPlatformQuotaRecord, 0, len(entities))
	for _, entity := range entities {
		records = append(records, *platformQuotaRecord(entity))
	}
	return records, nil
}

func (r *userPlatformQuotaRepository) IncrementUsageWithReset(ctx context.Context, userID int64, platform string, cost float64, now time.Time) error {
	return r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		entity, err := client.UserPlatformQuota.Query().
			Where(userplatformquota.UserIDEQ(userID), userplatformquota.PlatformEQ(platform), userplatformquota.DeletedAtIsNil()).
			ForUpdate().
			Only(txCtx)
		if dbent.IsNotFound(err) {
			const insert = `INSERT INTO user_platform_quotas
				(user_id, platform, daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
				 daily_window_start, weekly_window_start, monthly_window_start, created_at, updated_at)
				VALUES ($1, $2, $3, $3, $3, $4, $5, $6, $7, $7)
				ON CONFLICT (user_id, platform) WHERE deleted_at IS NULL DO UPDATE SET
					daily_usage_usd = user_platform_quotas.daily_usage_usd + EXCLUDED.daily_usage_usd,
					weekly_usage_usd = user_platform_quotas.weekly_usage_usd + EXCLUDED.weekly_usage_usd,
					monthly_usage_usd = user_platform_quotas.monthly_usage_usd + EXCLUDED.monthly_usage_usd,
					updated_at = EXCLUDED.updated_at`
			_, execErr := client.ExecContext(txCtx, insert, userID, platform, cost, timezone.StartOfDay(now), timezone.StartOfWeek(now), now, now)
			return execErr
		}
		if err != nil {
			return err
		}

		dailyStart := timezone.StartOfDay(now)
		weeklyStart := timezone.StartOfWeek(now)
		dailyUsage := resetOrIncrement(entity.DailyUsageUsd, entity.DailyWindowStart, dailyStart, cost)
		weeklyUsage := resetOrIncrement(entity.WeeklyUsageUsd, entity.WeeklyWindowStart, weeklyStart, cost)
		monthlyUsage, monthlyStart := rollingMonthlyUsage(entity.MonthlyUsageUsd, entity.MonthlyWindowStart, cost, now)
		_, err = entity.Update().
			SetDailyUsageUsd(dailyUsage).
			SetWeeklyUsageUsd(weeklyUsage).
			SetMonthlyUsageUsd(monthlyUsage).
			SetDailyWindowStart(dailyStart).
			SetWeeklyWindowStart(weeklyStart).
			SetMonthlyWindowStart(monthlyStart).
			Save(txCtx)
		return err
	})
}

func (r *userPlatformQuotaRepository) ResetExpiredWindow(ctx context.Context, userID int64, platform, window string, newStart time.Time) error {
	update := clientFromContext(ctx, r.client).UserPlatformQuota.Update().
		Where(userplatformquota.UserIDEQ(userID), userplatformquota.PlatformEQ(platform), userplatformquota.DeletedAtIsNil())
	switch window {
	case "daily":
		update = update.SetDailyUsageUsd(0).SetDailyWindowStart(newStart)
	case "weekly":
		update = update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(newStart)
	case "monthly":
		update = update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(newStart)
	default:
		return fmt.Errorf("unknown window %q", window)
	}
	count, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return service.ErrUserPlatformQuotaNotFound
	}
	return nil
}

func (r *userPlatformQuotaRepository) UpsertForUser(ctx context.Context, userID int64, records []service.UserPlatformQuotaRecord) error {
	return r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		platforms := make([]string, 0, len(records))
		for _, record := range records {
			platforms = append(platforms, record.Platform)
		}
		now := time.Now()
		if err := softDeletePlatformQuotas(txCtx, client, userID, platforms, now); err != nil {
			return err
		}
		for _, record := range records {
			if err := upsertPlatformQuotaLimits(txCtx, client, userID, record, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *userPlatformQuotaRepository) BatchSnapshotUsage(ctx context.Context, snapshots []service.UserPlatformQuotaSnapshot, now time.Time) error {
	const batchSize = 6000
	client := clientFromContext(ctx, r.client)
	for start := 0; start < len(snapshots); start += batchSize {
		end := start + batchSize
		if end > len(snapshots) {
			end = len(snapshots)
		}
		batch := snapshots[start:end]
		var query strings.Builder
		_, _ = query.WriteString("INSERT INTO user_platform_quotas (user_id, platform, daily_usage_usd, weekly_usage_usd, monthly_usage_usd, daily_window_start, weekly_window_start, monthly_window_start, created_at, updated_at) VALUES ")
		args := []any{now}
		for i, snapshot := range batch {
			if i > 0 {
				_, _ = query.WriteString(",")
			}
			base := len(args)
			fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$1,$1)", base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
			args = append(args, snapshot.UserID, snapshot.Platform, snapshot.DailyUsageUSD, snapshot.WeeklyUsageUSD, snapshot.MonthlyUsageUSD, snapshot.DailyWindowStart, snapshot.WeeklyWindowStart, snapshot.MonthlyWindowStart)
		}
		_, _ = query.WriteString(` ON CONFLICT (user_id, platform) WHERE deleted_at IS NULL DO UPDATE SET
			daily_usage_usd = EXCLUDED.daily_usage_usd,
			weekly_usage_usd = EXCLUDED.weekly_usage_usd,
			monthly_usage_usd = EXCLUDED.monthly_usage_usd,
			daily_window_start = EXCLUDED.daily_window_start,
			weekly_window_start = EXCLUDED.weekly_window_start,
			monthly_window_start = EXCLUDED.monthly_window_start,
			updated_at = EXCLUDED.updated_at`)
		if _, err := client.ExecContext(ctx, query.String(), args...); err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23503" {
				return service.ErrUserPlatformQuotaFKViolation
			}
			return err
		}
	}
	return nil
}

func (r *userPlatformQuotaRepository) withTx(ctx context.Context, fn func(context.Context, *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin user platform quota transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}
	return tx.Commit()
}

func platformQuotaRecord(entity *dbent.UserPlatformQuota) *service.UserPlatformQuotaRecord {
	return &service.UserPlatformQuotaRecord{
		UserID: entity.UserID, Platform: entity.Platform,
		DailyLimitUSD: entity.DailyLimitUsd, WeeklyLimitUSD: entity.WeeklyLimitUsd, MonthlyLimitUSD: entity.MonthlyLimitUsd,
		DailyUsageUSD: entity.DailyUsageUsd, WeeklyUsageUSD: entity.WeeklyUsageUsd, MonthlyUsageUSD: entity.MonthlyUsageUsd,
		DailyWindowStart: entity.DailyWindowStart, WeeklyWindowStart: entity.WeeklyWindowStart, MonthlyWindowStart: entity.MonthlyWindowStart,
	}
}

func resetOrIncrement(usage float64, previous *time.Time, current time.Time, cost float64) float64 {
	if previous == nil || !previous.Equal(current) {
		return cost
	}
	return usage + cost
}

func rollingMonthlyUsage(usage float64, previous *time.Time, cost float64, now time.Time) (float64, time.Time) {
	if previous == nil || now.Sub(*previous) >= 30*24*time.Hour {
		return cost, now
	}
	return usage + cost, *previous
}

func softDeletePlatformQuotas(ctx context.Context, client *dbent.Client, userID int64, keep []string, now time.Time) error {
	if len(keep) == 0 {
		_, err := client.ExecContext(ctx, `UPDATE user_platform_quotas SET deleted_at=$2, updated_at=$2 WHERE user_id=$1 AND deleted_at IS NULL`, userID, now)
		return err
	}
	placeholders := make([]string, len(keep))
	args := []any{userID, now}
	for i, platform := range keep {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args = append(args, platform)
	}
	query := fmt.Sprintf(`UPDATE user_platform_quotas SET deleted_at=$2, updated_at=$2 WHERE user_id=$1 AND deleted_at IS NULL AND platform NOT IN (%s)`, strings.Join(placeholders, ","))
	_, err := client.ExecContext(ctx, query, args...)
	return err
}

func upsertPlatformQuotaLimits(ctx context.Context, client *dbent.Client, userID int64, record service.UserPlatformQuotaRecord, now time.Time) error {
	const update = `UPDATE user_platform_quotas SET daily_limit_usd=$1, weekly_limit_usd=$2, monthly_limit_usd=$3, deleted_at=NULL, updated_at=$4 WHERE user_id=$5 AND platform=$6 AND deleted_at IS NULL`
	result, err := client.ExecContext(ctx, update, record.DailyLimitUSD, record.WeeklyLimitUSD, record.MonthlyLimitUSD, now, userID, record.Platform)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count > 0 {
		return err
	}
	const insert = `INSERT INTO user_platform_quotas (user_id, platform, daily_limit_usd, weekly_limit_usd, monthly_limit_usd, daily_usage_usd, weekly_usage_usd, monthly_usage_usd, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,0,0,0,$6,$6) ON CONFLICT (user_id, platform) WHERE deleted_at IS NULL DO NOTHING`
	result, err = client.ExecContext(ctx, insert, userID, record.Platform, record.DailyLimitUSD, record.WeeklyLimitUSD, record.MonthlyLimitUSD, now)
	if err != nil {
		return err
	}
	count, err = result.RowsAffected()
	if err != nil || count > 0 {
		return err
	}
	_, err = client.ExecContext(ctx, update, record.DailyLimitUSD, record.WeeklyLimitUSD, record.MonthlyLimitUSD, now, userID, record.Platform)
	return err
}
