package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const groupRateScheduleMultiplierEpsilon = 0.0000001

type groupRateScheduleRepository struct {
	db  *sql.DB
	sql sqlExecutor
}

func NewGroupRateScheduleRepository(db *sql.DB) service.GroupRateScheduleRepository {
	return &groupRateScheduleRepository{db: db, sql: db}
}

func (r *groupRateScheduleRepository) ListByGroupID(ctx context.Context, groupID int64) ([]service.GroupRateSchedule, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT s.id, s.group_id, s.target_user_id,
			COALESCE(u.username, ''), COALESCE(u.email, ''),
			s.start_minute, s.end_minute, s.rate_multiplier, s.enabled, s.created_at, s.updated_at
		FROM group_rate_schedules s
		LEFT JOIN users u ON u.id = s.target_user_id AND u.deleted_at IS NULL
		WHERE s.group_id = $1
		ORDER BY s.target_user_id NULLS FIRST, s.start_minute, s.end_minute, s.id
	`, groupID)
	if err != nil {
		return nil, err
	}
	return scanGroupRateSchedules(rows)
}

func (r *groupRateScheduleRepository) ReplaceForGroup(ctx context.Context, groupID int64, schedules []service.GroupRateScheduleInput) ([]service.GroupRateSchedule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int64
	if err := scanSingleRow(ctx, tx, `SELECT id FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, []any{groupID}, &existingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrGroupNotFound
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_rate_schedules WHERE group_id = $1`, groupID); err != nil {
		return nil, err
	}
	now := time.Now()
	for _, schedule := range schedules {
		var targetUserID any
		if schedule.TargetUserID != nil {
			targetUserID = *schedule.TargetUserID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO group_rate_schedules (group_id, target_user_id, start_minute, end_minute, rate_multiplier, enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		`, groupID, targetUserID, schedule.StartMinute, schedule.EndMinute, schedule.RateMultiplier, schedule.Enabled, now); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT s.id, s.group_id, s.target_user_id,
			COALESCE(u.username, ''), COALESCE(u.email, ''),
			s.start_minute, s.end_minute, s.rate_multiplier, s.enabled, s.created_at, s.updated_at
		FROM group_rate_schedules s
		LEFT JOIN users u ON u.id = s.target_user_id AND u.deleted_at IS NULL
		WHERE s.group_id = $1
		ORDER BY s.target_user_id NULLS FIRST, s.start_minute, s.end_minute, s.id
	`, groupID)
	if err != nil {
		return nil, err
	}
	out, err := scanGroupRateSchedules(rows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *groupRateScheduleRepository) ListEnabled(ctx context.Context) ([]service.GroupRateSchedule, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT s.id, s.group_id, s.target_user_id,
			COALESCE(u.username, ''), COALESCE(u.email, ''),
			s.start_minute, s.end_minute, s.rate_multiplier, s.enabled, s.created_at, s.updated_at
		FROM group_rate_schedules s
		JOIN groups g ON g.id = s.group_id AND g.deleted_at IS NULL
		LEFT JOIN users u ON u.id = s.target_user_id AND u.deleted_at IS NULL
		WHERE s.enabled = TRUE
		ORDER BY s.group_id, s.target_user_id NULLS FIRST, s.start_minute, s.end_minute, s.id
	`)
	if err != nil {
		return nil, err
	}
	return scanGroupRateSchedules(rows)
}

func (r *groupRateScheduleRepository) ListManagedGroupIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT group_id FROM (
			SELECT DISTINCT s.group_id FROM group_rate_schedules s JOIN groups g ON g.id = s.group_id AND g.deleted_at IS NULL WHERE s.enabled = TRUE
			UNION SELECT st.group_id FROM group_rate_schedule_states st JOIN groups g ON g.id = st.group_id AND g.deleted_at IS NULL
			UNION SELECT ust.group_id FROM group_rate_schedule_user_states ust JOIN groups g ON g.id = ust.group_id AND g.deleted_at IS NULL
		) AS managed ORDER BY group_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *groupRateScheduleRepository) ListManagedTargetUserIDs(ctx context.Context, groupID int64) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT user_id FROM (
			SELECT DISTINCT s.target_user_id AS user_id FROM group_rate_schedules s JOIN users u ON u.id = s.target_user_id AND u.deleted_at IS NULL WHERE s.group_id = $1 AND s.enabled = TRUE AND s.target_user_id IS NOT NULL
			UNION SELECT ust.user_id FROM group_rate_schedule_user_states ust JOIN users u ON u.id = ust.user_id AND u.deleted_at IS NULL WHERE ust.group_id = $1
		) AS managed ORDER BY user_id
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *groupRateScheduleRepository) ApplyScheduledMultiplier(ctx context.Context, groupID, scheduleID int64, rateMultiplier float64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var current float64
	if err := scanSingleRow(ctx, tx, `SELECT rate_multiplier FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, []any{groupID}, &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, service.ErrGroupNotFound
		}
		return false, err
	}
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_rate_schedule_states (group_id, base_rate_multiplier, applied_schedule_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (group_id) DO UPDATE SET applied_schedule_id = EXCLUDED.applied_schedule_id, updated_at = EXCLUDED.updated_at
	`, groupID, current, scheduleID, now); err != nil {
		return false, err
	}
	changed := groupRateScheduleAbs(current-rateMultiplier) > groupRateScheduleMultiplierEpsilon
	if changed {
		if _, err := tx.ExecContext(ctx, `UPDATE groups SET rate_multiplier = $2, updated_at = $3 WHERE id = $1 AND deleted_at IS NULL`, groupID, rateMultiplier, now); err != nil {
			return false, err
		}
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func (r *groupRateScheduleRepository) RestoreBaseMultiplier(ctx context.Context, groupID int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var base float64
	if err := scanSingleRow(ctx, tx, `SELECT base_rate_multiplier FROM group_rate_schedule_states WHERE group_id = $1 FOR UPDATE`, []any{groupID}, &base); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	var current float64
	if err := scanSingleRow(ctx, tx, `SELECT rate_multiplier FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, []any{groupID}, &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, service.ErrGroupNotFound
		}
		return false, err
	}
	changed := groupRateScheduleAbs(current-base) > groupRateScheduleMultiplierEpsilon
	if changed {
		now := time.Now()
		if _, err := tx.ExecContext(ctx, `UPDATE groups SET rate_multiplier = $2, updated_at = $3 WHERE id = $1 AND deleted_at IS NULL`, groupID, base, now); err != nil {
			return false, err
		}
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_rate_schedule_states WHERE group_id = $1`, groupID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func (r *groupRateScheduleRepository) ApplyScheduledUserMultiplier(ctx context.Context, groupID, userID, scheduleID int64, rateMultiplier float64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var current sql.NullFloat64
	err = scanSingleRow(ctx, tx, `SELECT rate_multiplier FROM user_group_rate_multipliers WHERE group_id = $1 AND user_id = $2 FOR UPDATE`, []any{groupID, userID}, &current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var base any
	if current.Valid {
		base = current.Float64
	}
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_rate_schedule_user_states (group_id, user_id, base_rate_multiplier, applied_schedule_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (group_id, user_id) DO UPDATE SET applied_schedule_id = EXCLUDED.applied_schedule_id, updated_at = EXCLUDED.updated_at
	`, groupID, userID, base, scheduleID, now); err != nil {
		return false, err
	}
	changed := !current.Valid || groupRateScheduleAbs(current.Float64-rateMultiplier) > groupRateScheduleMultiplierEpsilon
	if changed {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4)
			ON CONFLICT (user_id, group_id) DO UPDATE SET rate_multiplier = EXCLUDED.rate_multiplier, updated_at = EXCLUDED.updated_at
		`, userID, groupID, rateMultiplier, now); err != nil {
			return false, err
		}
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func (r *groupRateScheduleRepository) RestoreBaseUserMultiplier(ctx context.Context, groupID, userID int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var base sql.NullFloat64
	if err := scanSingleRow(ctx, tx, `SELECT base_rate_multiplier FROM group_rate_schedule_user_states WHERE group_id = $1 AND user_id = $2 FOR UPDATE`, []any{groupID, userID}, &base); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	var current sql.NullFloat64
	err = scanSingleRow(ctx, tx, `SELECT rate_multiplier FROM user_group_rate_multipliers WHERE group_id = $1 AND user_id = $2 FOR UPDATE`, []any{groupID, userID}, &current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	changed := groupRateScheduleNullFloatChanged(current, base)
	if changed {
		now := time.Now()
		if base.Valid {
			if _, err := tx.ExecContext(ctx, `INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at) VALUES ($1, $2, $3, $4, $4) ON CONFLICT (user_id, group_id) DO UPDATE SET rate_multiplier = EXCLUDED.rate_multiplier, updated_at = EXCLUDED.updated_at`, userID, groupID, base.Float64, now); err != nil {
				return false, err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE user_group_rate_multipliers SET rate_multiplier = NULL, updated_at = $3 WHERE group_id = $1 AND user_id = $2`, groupID, userID, now); err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE group_id = $1 AND user_id = $2 AND rate_multiplier IS NULL AND rpm_override IS NULL`, groupID, userID); err != nil {
				return false, err
			}
		}
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_rate_schedule_user_states WHERE group_id = $1 AND user_id = $2`, groupID, userID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func scanGroupRateSchedules(rows *sql.Rows) ([]service.GroupRateSchedule, error) {
	defer func() { _ = rows.Close() }()
	var out []service.GroupRateSchedule
	for rows.Next() {
		var item service.GroupRateSchedule
		var target sql.NullInt64
		if err := rows.Scan(&item.ID, &item.GroupID, &target, &item.TargetUserName, &item.TargetUserEmail, &item.StartMinute, &item.EndMinute, &item.RateMultiplier, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if target.Valid {
			id := target.Int64
			item.TargetUserID = &id
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func groupRateScheduleAbs(v float64) float64 { return math.Abs(v) }

func groupRateScheduleNullFloatChanged(a, b sql.NullFloat64) bool {
	if a.Valid != b.Valid {
		return true
	}
	return a.Valid && groupRateScheduleAbs(a.Float64-b.Float64) > groupRateScheduleMultiplierEpsilon
}
