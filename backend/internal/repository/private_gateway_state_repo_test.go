package repository

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"anlapi/internal/service"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPrivateGatewayStateRepositoryClaimNonceIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &privateGatewayStateRepository{sql: db}
	expiresAt := time.Unix(1_785_475_641, 0).UTC().Add(10 * time.Minute)
	query := regexp.QuoteMeta("INSERT INTO idempotency_records")
	mock.ExpectExec(query).
		WithArgs("private_gateway_nonce:test", "nonce-hash", service.IdempotencyStatusSucceeded, expiresAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	claimed, err := repo.ClaimPrivateGatewayNonce(context.Background(), "private_gateway_nonce:test", "nonce-hash", expiresAt)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayStateRepositoryMarksUnknownWithoutReclaiming(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &privateGatewayStateRepository{sql: db}
	expiresAt := time.Unix(1_785_475_641, 0).UTC().Add(24 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(7), service.PrivateGatewayIdempotencyStatusOutcomeUnknown, "IDEMPOTENCY_OUTCOME_UNKNOWN", expiresAt, service.IdempotencyStatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkPrivateGatewayOutcomeUnknown(context.Background(), 7, "IDEMPOTENCY_OUTCOME_UNKNOWN", expiresAt))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayStateRepositoryRetryableLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &privateGatewayStateRepository{sql: db}
	now := time.Unix(1_785_475_641, 0).UTC()
	lockedUntil := now.Add(30 * time.Second)
	expiresAt := now.Add(24 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(9), service.PrivateGatewayIdempotencyStatusFailedRetryable, http.StatusServiceUnavailable,
			`{"status":503}`, "UPSTREAM_TEMPORARY", lockedUntil, expiresAt, service.IdempotencyStatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkPrivateGatewayFailedRetryable(context.Background(), 9, http.StatusServiceUnavailable,
		`{"status":503}`, "UPSTREAM_TEMPORARY", lockedUntil, expiresAt))

	newLockedUntil := now.Add(time.Minute)
	newExpiresAt := now.Add(25 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(9), service.IdempotencyStatusProcessing, newLockedUntil, newExpiresAt,
			service.PrivateGatewayIdempotencyStatusFailedRetryable, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	reclaimed, err := repo.ReclaimPrivateGatewayRetryable(context.Background(), 9, now, newLockedUntil, newExpiresAt)
	require.NoError(t, err)
	require.True(t, reclaimed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayStateRepositoryMarksExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &privateGatewayStateRepository{sql: db}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(12), service.PrivateGatewayIdempotencyStatusExpired, "IDEMPOTENCY_EXPIRED", service.IdempotencyStatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkPrivateGatewayExpired(context.Background(), 12, "IDEMPOTENCY_EXPIRED"))
	require.NoError(t, mock.ExpectationsWereMet())
}
