package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type idempotencyRepository struct {
	sql sqlExecutor
}

func NewIdempotencyRepository(_ *dbent.Client, sqlDB *sql.DB) service.IdempotencyRepository {
	return &idempotencyRepository{sql: sqlDB}
}

// NewPrivateGatewayNonceRepository exposes the same durable SQL repository
// through the narrow private-gateway nonce port without widening the general
// idempotency service contract.
func NewPrivateGatewayNonceRepository(_ *dbent.Client, sqlDB *sql.DB) service.PrivateGatewayNonceRepository {
	return &idempotencyRepository{sql: sqlDB}
}

// NewPrivateGatewayStateRepository exposes the same durable table through the
// private gateway state port. The production implementation remains SQL-backed;
// tests may inject a controlled fake through the middleware constructor.
func NewPrivateGatewayStateRepository(_ *dbent.Client, sqlDB *sql.DB) service.PrivateGatewayStateRepository {
	return &idempotencyRepository{sql: sqlDB}
}

func (r *idempotencyRepository) ClaimPrivateGatewayNonce(ctx context.Context, scope, nonceHash string, expiresAt time.Time) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("idempotency repository is unavailable")
	}
	query := `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, expires_at
		) VALUES ($1, $2, $2, 'private_gateway_nonce', $3)
		ON CONFLICT (scope, idempotency_key_hash) DO UPDATE
		SET expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
		WHERE idempotency_records.expires_at <= NOW()
		RETURNING id
	`
	var id int64
	err := scanSingleRow(ctx, r.sql, query, []any{scope, nonceHash, expiresAt}, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *idempotencyRepository) CreateProcessing(ctx context.Context, record *service.IdempotencyRecord) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("idempotency repository is unavailable")
	}
	if record == nil {
		return false, nil
	}
	query := `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, locked_until, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (scope, idempotency_key_hash) DO NOTHING
		RETURNING id, created_at, updated_at
	`
	var createdAt time.Time
	var updatedAt time.Time
	err := scanSingleRow(ctx, r.sql, query, []any{
		record.Scope,
		record.IdempotencyKeyHash,
		record.RequestFingerprint,
		record.Status,
		record.LockedUntil,
		record.ExpiresAt,
	}, &record.ID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return true, nil
}

func (r *idempotencyRepository) GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("idempotency repository is unavailable")
	}
	query := `
		SELECT
			id, scope, idempotency_key_hash, request_fingerprint, status, response_status,
			response_body, error_reason, locked_until, expires_at, created_at, updated_at
		FROM idempotency_records
		WHERE scope = $1 AND idempotency_key_hash = $2
	`
	record := &service.IdempotencyRecord{}
	var responseStatus sql.NullInt64
	var responseBody sql.NullString
	var errorReason sql.NullString
	var lockedUntil sql.NullTime
	err := scanSingleRow(ctx, r.sql, query, []any{scope, keyHash},
		&record.ID,
		&record.Scope,
		&record.IdempotencyKeyHash,
		&record.RequestFingerprint,
		&record.Status,
		&responseStatus,
		&responseBody,
		&errorReason,
		&lockedUntil,
		&record.ExpiresAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if responseStatus.Valid {
		v := int(responseStatus.Int64)
		record.ResponseStatus = &v
	}
	if responseBody.Valid {
		v := responseBody.String
		record.ResponseBody = &v
	}
	if errorReason.Valid {
		v := errorReason.String
		record.ErrorReason = &v
	}
	if lockedUntil.Valid {
		v := lockedUntil.Time
		record.LockedUntil = &v
	}
	return record, nil
}

func (r *idempotencyRepository) TryReclaim(
	ctx context.Context,
	id int64,
	fromStatus string,
	now, newLockedUntil, newExpiresAt time.Time,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			locked_until = $3,
			error_reason = NULL,
			updated_at = NOW(),
			expires_at = $4
		WHERE id = $1
			AND status = $5
			AND (locked_until IS NULL OR locked_until <= $6)
	`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		service.IdempotencyStatusProcessing,
		newLockedUntil,
		newExpiresAt,
		fromStatus,
		now,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *idempotencyRepository) ReclaimPrivateGatewayExpired(
	ctx context.Context,
	id int64,
	now, lockedUntil, expiresAt time.Time,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = NULL,
			response_body = NULL,
			error_reason = NULL,
			locked_until = $3,
			expires_at = $4,
			updated_at = NOW()
		WHERE id = $1
			AND expires_at <= $5
			AND status IN ($6, $7, $8)
	`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		service.IdempotencyStatusProcessing,
		lockedUntil,
		expiresAt,
		now,
		service.IdempotencyStatusSucceeded,
		service.PrivateGatewayIdempotencyStatusFailed,
		service.PrivateGatewayIdempotencyStatusOutcomeUnknown,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func (r *idempotencyRepository) ReclaimPrivateGatewayRetryable(
	ctx context.Context,
	id int64,
	now, lockedUntil, expiresAt time.Time,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = NULL,
			response_body = NULL,
			error_reason = NULL,
			locked_until = $3,
			expires_at = $4,
			updated_at = NOW()
		WHERE id = $1
			AND status = $5
			AND locked_until <= $6
			AND expires_at > $6
	`
	res, err := r.sql.ExecContext(ctx, query,
		id,
		service.IdempotencyStatusProcessing,
		lockedUntil,
		expiresAt,
		service.PrivateGatewayIdempotencyStatusFailedRetryable,
		now,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func (r *idempotencyRepository) MarkPrivateGatewayFailed(
	ctx context.Context,
	id int64,
	responseStatus int,
	responseBody, errorReason string,
	expiresAt time.Time,
) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = $3,
			response_body = $4,
			error_reason = $5,
			locked_until = NULL,
			expires_at = $6,
			updated_at = NOW()
		WHERE id = $1 AND status = $7
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.PrivateGatewayIdempotencyStatusFailed,
		responseStatus,
		responseBody,
		errorReason,
		expiresAt,
		service.IdempotencyStatusProcessing,
	)
	return err
}

func (r *idempotencyRepository) MarkPrivateGatewayFailedRetryable(
	ctx context.Context,
	id int64,
	responseStatus int,
	responseBody, errorReason string,
	lockedUntil, expiresAt time.Time,
) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = $3,
			response_body = $4,
			error_reason = $5,
			locked_until = $6,
			expires_at = $7,
			updated_at = NOW()
		WHERE id = $1 AND status = $8
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.PrivateGatewayIdempotencyStatusFailedRetryable,
		responseStatus,
		responseBody,
		errorReason,
		lockedUntil,
		expiresAt,
		service.IdempotencyStatusProcessing,
	)
	return err
}

func (r *idempotencyRepository) MarkPrivateGatewayOutcomeUnknown(
	ctx context.Context,
	id int64,
	errorReason string,
	expiresAt time.Time,
) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = NULL,
			response_body = NULL,
			error_reason = $3,
			locked_until = NULL,
			expires_at = $4,
			updated_at = NOW()
		WHERE id = $1 AND status = $5
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.PrivateGatewayIdempotencyStatusOutcomeUnknown,
		errorReason,
		expiresAt,
		service.IdempotencyStatusProcessing,
	)
	return err
}

func (r *idempotencyRepository) MarkPrivateGatewayExpired(
	ctx context.Context,
	id int64,
	errorReason string,
) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = NULL,
			response_body = NULL,
			error_reason = $3,
			locked_until = NULL,
			updated_at = NOW()
		WHERE id = $1 AND status <> $4
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.PrivateGatewayIdempotencyStatusExpired,
		errorReason,
		service.IdempotencyStatusProcessing,
	)
	return err
}

func (r *idempotencyRepository) ExtendProcessingLock(
	ctx context.Context,
	id int64,
	requestFingerprint string,
	newLockedUntil,
	newExpiresAt time.Time,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET locked_until = $2,
			expires_at = $3,
			updated_at = NOW()
		WHERE id = $1
			AND status = $4
			AND request_fingerprint = $5
	`
	res, err := r.sql.ExecContext(
		ctx,
		query,
		id,
		newLockedUntil,
		newExpiresAt,
		service.IdempotencyStatusProcessing,
		requestFingerprint,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *idempotencyRepository) MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = $3,
			response_body = $4,
			error_reason = NULL,
			locked_until = NULL,
			expires_at = $5,
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.IdempotencyStatusSucceeded,
		responseStatus,
		responseBody,
		expiresAt,
	)
	return err
}

// MarkPrivateGatewaySucceeded only finalizes a record that this request still
// owns. A late response must not overwrite an outcome_unknown or a reclaimed
// record after the processing lease has expired.
func (r *idempotencyRepository) MarkPrivateGatewaySucceeded(
	ctx context.Context,
	id int64,
	responseStatus int,
	responseBody string,
	expiresAt time.Time,
) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			response_status = $3,
			response_body = $4,
			error_reason = NULL,
			locked_until = NULL,
			expires_at = $5,
			updated_at = NOW()
		WHERE id = $1 AND status = $6
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.IdempotencyStatusSucceeded,
		responseStatus,
		responseBody,
		expiresAt,
		service.IdempotencyStatusProcessing,
	)
	return err
}

func (r *idempotencyRepository) MarkFailedRetryable(ctx context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	if r == nil || r.sql == nil {
		return errors.New("idempotency repository is unavailable")
	}
	query := `
		UPDATE idempotency_records
		SET status = $2,
			error_reason = $3,
			locked_until = $4,
			expires_at = $5,
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.sql.ExecContext(ctx, query,
		id,
		service.IdempotencyStatusFailedRetryable,
		errorReason,
		lockedUntil,
		expiresAt,
	)
	return err
}

func (r *idempotencyRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if r == nil || r.sql == nil {
		return 0, errors.New("idempotency repository is unavailable")
	}
	if limit <= 0 {
		limit = 500
	}
	query := `
		WITH victims AS (
			SELECT id
			FROM idempotency_records
			WHERE expires_at <= $1
			ORDER BY expires_at ASC
			LIMIT $2
		)
		DELETE FROM idempotency_records
		WHERE id IN (SELECT id FROM victims)
	`
	res, err := r.sql.ExecContext(ctx, query, now, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
