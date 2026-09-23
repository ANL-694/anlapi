package service

import (
	"context"
	"time"
)

const (
	PrivateGatewayIdempotencyStatusFailed          = "failed"
	PrivateGatewayIdempotencyStatusFailedRetryable = "failed_retryable"
	PrivateGatewayIdempotencyStatusOutcomeUnknown  = "outcome_unknown"
	PrivateGatewayIdempotencyStatusExpired         = "expired"
)

// PrivateGatewayStateRepository is the durable state boundary for the
// service-authenticated gateway. It reuses idempotency_records while keeping
// private-gateway transitions separate from the general coordinator.
type PrivateGatewayStateRepository interface {
	IdempotencyRepository
	ReclaimPrivateGatewayExpired(ctx context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error)
	ReclaimPrivateGatewayRetryable(ctx context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error)
	MarkPrivateGatewaySucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error
	MarkPrivateGatewayFailed(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, expiresAt time.Time) error
	MarkPrivateGatewayFailedRetryable(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, lockedUntil, expiresAt time.Time) error
	MarkPrivateGatewayOutcomeUnknown(ctx context.Context, id int64, errorReason string, expiresAt time.Time) error
	MarkPrivateGatewayExpired(ctx context.Context, id int64, errorReason string) error
}
