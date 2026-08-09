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
	PrivateGatewayIdempotencyStatusConflict        = "conflict"
)

// PrivateGatewayStateRepository is the durable state boundary for the
// Hub-to-anlapi private gateway. It deliberately reuses the idempotency
// record shape without exposing upstream account or credential data.
type PrivateGatewayStateRepository interface {
	CreateProcessing(ctx context.Context, record *IdempotencyRecord) (bool, error)
	GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*IdempotencyRecord, error)
	ReclaimPrivateGatewayExpired(ctx context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error)
	ReclaimPrivateGatewayRetryable(ctx context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error)
	MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error
	ClaimPrivateGatewayNonce(ctx context.Context, scope, nonceHash string, expiresAt time.Time) (bool, error)
	MarkPrivateGatewayFailed(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, expiresAt time.Time) error
	MarkPrivateGatewayFailedRetryable(ctx context.Context, id int64, responseStatus int, responseBody, errorReason string, lockedUntil, expiresAt time.Time) error
	MarkPrivateGatewayOutcomeUnknown(ctx context.Context, id int64, errorReason string, expiresAt time.Time) error
	MarkPrivateGatewayExpired(ctx context.Context, id int64, errorReason string) error
}
