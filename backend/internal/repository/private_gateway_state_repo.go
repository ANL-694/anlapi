package repository

import (
	"context"
	"database/sql"
	"time"

	dbent "anlapi/ent"
	"anlapi/internal/service"
)

// privateGatewayStateRepository uses the existing idempotency_records table.
// The private gateway is isolated by scope; no upstream credentials are
// persisted here.
type privateGatewayStateRepository struct {
	sql sqlExecutor
}

func NewPrivateGatewayStateRepository(_ *dbent.Client, sqlDB *sql.DB) service.PrivateGatewayStateRepository {
	return &privateGatewayStateRepository{sql: sqlDB}
}

func (r *privateGatewayStateRepository) CreateProcessing(ctx context.Context, record *service.IdempotencyRecord) (bool, error) {
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
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return true, nil
}

func (r *privateGatewayStateRepository) GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	query := `
		SELECT id, scope, idempotency_key_hash, request_fingerprint, status,
			response_status, response_body, error_reason, locked_until,
			expires_at, created_at, updated_at
		FROM idempotency_records
		WHERE scope = $1 AND idempotency_key_hash = $2
	`
	record := &service.IdempotencyRecord{}
	var responseStatus sql.NullInt64
	var responseBody sql.NullString
	var errorReason sql.NullString
	var lockedUntil sql.NullTime
	err := scanSingleRow(ctx, r.sql, query, []any{scope, keyHash},
		&record.ID, &record.Scope, &record.IdempotencyKeyHash,
		&record.RequestFingerprint, &record.Status, &responseStatus,
		&responseBody, &errorReason, &lockedUntil, &record.ExpiresAt,
		&record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if responseStatus.Valid {
		value := int(responseStatus.Int64)
		record.ResponseStatus = &value
	}
	if responseBody.Valid {
		value := responseBody.String
		record.ResponseBody = &value
	}
	if errorReason.Valid {
		value := errorReason.String
		record.ErrorReason = &value
	}
	if lockedUntil.Valid {
		value := lockedUntil.Time
		record.LockedUntil = &value
	}
	return record, nil
}

func (r *privateGatewayStateRepository) MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = $3, response_body = $4,
			error_reason = NULL, locked_until = NULL, expires_at = $5,
			updated_at = NOW()
		WHERE id = $1 AND status = $6
	`
	_, err := r.sql.ExecContext(ctx, query, id, service.IdempotencyStatusSucceeded,
		responseStatus, responseBody, expiresAt, service.IdempotencyStatusProcessing)
	return err
}

func (r *privateGatewayStateRepository) ReclaimPrivateGatewayExpired(ctx context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error) {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = NULL, response_body = NULL,
			error_reason = NULL, locked_until = $3, expires_at = $4,
			updated_at = NOW()
		WHERE id = $1 AND expires_at <= $5 AND status IN ($6, $7, $8)
	`
	result, err := r.sql.ExecContext(ctx, query, id, service.IdempotencyStatusProcessing,
		lockedUntil, expiresAt, now, service.IdempotencyStatusSucceeded,
		service.PrivateGatewayIdempotencyStatusFailed, service.PrivateGatewayIdempotencyStatusOutcomeUnknown)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *privateGatewayStateRepository) ReclaimPrivateGatewayRetryable(ctx context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error) {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = NULL, response_body = NULL,
			error_reason = NULL, locked_until = $3, expires_at = $4,
			updated_at = NOW()
		WHERE id = $1 AND status = $5 AND locked_until <= $6 AND expires_at > $6
	`
	result, err := r.sql.ExecContext(ctx, query, id, service.IdempotencyStatusProcessing,
		lockedUntil, expiresAt, service.PrivateGatewayIdempotencyStatusFailedRetryable, now)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *privateGatewayStateRepository) MarkFailed(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, expiresAt time.Time) error {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = $3, response_body = $4,
			error_reason = $5, locked_until = NULL, expires_at = $6,
			updated_at = NOW()
		WHERE id = $1 AND status = $7
	`
	_, err := r.sql.ExecContext(ctx, query, id, service.PrivateGatewayIdempotencyStatusFailed,
		responseStatus, responseBody, errorReason, expiresAt, service.IdempotencyStatusProcessing)
	return err
}

func (r *privateGatewayStateRepository) MarkPrivateGatewayFailed(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, expiresAt time.Time) error {
	return r.MarkFailed(ctx, id, responseStatus, responseBody, errorReason, expiresAt)
}

func (r *privateGatewayStateRepository) MarkPrivateGatewayFailedRetryable(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, lockedUntil, expiresAt time.Time) error {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = $3, response_body = $4,
			error_reason = $5, locked_until = $6, expires_at = $7,
			updated_at = NOW()
		WHERE id = $1 AND status = $8
	`
	_, err := r.sql.ExecContext(ctx, query, id, service.PrivateGatewayIdempotencyStatusFailedRetryable,
		responseStatus, responseBody, errorReason, lockedUntil, expiresAt, service.IdempotencyStatusProcessing)
	return err
}

func (r *privateGatewayStateRepository) MarkPrivateGatewayOutcomeUnknown(ctx context.Context, id int64, errorReason string, expiresAt time.Time) error {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = NULL, response_body = NULL,
			error_reason = $3, locked_until = NULL, expires_at = $4,
			updated_at = NOW()
		WHERE id = $1 AND status = $5
	`
	_, err := r.sql.ExecContext(ctx, query, id, service.PrivateGatewayIdempotencyStatusOutcomeUnknown,
		errorReason, expiresAt, service.IdempotencyStatusProcessing)
	return err
}

func (r *privateGatewayStateRepository) MarkPrivateGatewayExpired(ctx context.Context, id int64, errorReason string) error {
	query := `
		UPDATE idempotency_records
		SET status = $2, response_status = NULL, response_body = NULL,
			error_reason = $3, locked_until = NULL, updated_at = NOW()
		WHERE id = $1 AND status <> $4
	`
	_, err := r.sql.ExecContext(ctx, query, id, service.PrivateGatewayIdempotencyStatusExpired, errorReason, service.IdempotencyStatusProcessing)
	return err
}

func (r *privateGatewayStateRepository) ClaimPrivateGatewayNonce(ctx context.Context, scope, nonceHash string, expiresAt time.Time) (bool, error) {
	query := `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, expires_at
		) VALUES ($1, $2, $2, $3, $4)
		ON CONFLICT (scope, idempotency_key_hash) DO UPDATE
		SET expires_at = EXCLUDED.expires_at, updated_at = NOW()
		WHERE idempotency_records.expires_at <= NOW()
	`
	result, err := r.sql.ExecContext(ctx, query, scope, nonceHash,
		service.IdempotencyStatusSucceeded, expiresAt)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}
