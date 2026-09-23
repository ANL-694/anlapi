package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPrivateGatewayStateRepositoryMarksOutcomeUnknownWithProcessingGuard(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewPrivateGatewayStateRepository(nil, db)
	expiresAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(7), "outcome_unknown", "IDEMPOTENCY_OUTCOME_UNKNOWN", expiresAt, "processing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkPrivateGatewayOutcomeUnknown(context.Background(), 7, "IDEMPOTENCY_OUTCOME_UNKNOWN", expiresAt))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayStateRepositoryRetryableLifecycleUsesAtomicGuards(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewPrivateGatewayStateRepository(nil, db)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	lockedUntil := now.Add(30 * time.Second)
	expiresAt := now.Add(24 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(9), "failed_retryable", 503, `{"error":"temporary"}`, "UPSTREAM_TEMPORARY", lockedUntil, expiresAt, "processing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkPrivateGatewayFailedRetryable(context.Background(), 9, 503, `{"error":"temporary"}`, "UPSTREAM_TEMPORARY", lockedUntil, expiresAt))

	newLockedUntil := now.Add(time.Minute)
	newExpiresAt := now.Add(25 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(9), "processing", newLockedUntil, newExpiresAt, "failed_retryable", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	reclaimed, err := repo.ReclaimPrivateGatewayRetryable(context.Background(), 9, now, newLockedUntil, newExpiresAt)
	require.NoError(t, err)
	require.True(t, reclaimed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayStateRepositorySuccessUsesProcessingGuard(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewPrivateGatewayStateRepository(nil, db)
	expiresAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(13), "succeeded", 200, `{"ok":true}`, expiresAt, "processing").
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, repo.MarkPrivateGatewaySucceeded(context.Background(), 13, 200, `{"ok":true}`, expiresAt))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayStateRepositoryExpiredClaimDoesNotReclaimProcessing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewPrivateGatewayStateRepository(nil, db)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	lockedUntil := now.Add(30 * time.Second)
	expiresAt := now.Add(24 * time.Hour)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE idempotency_records")).
		WithArgs(int64(11), "processing", lockedUntil, expiresAt, now, "succeeded", "failed", "outcome_unknown").
		WillReturnResult(sqlmock.NewResult(0, 0))
	reclaimed, err := repo.ReclaimPrivateGatewayExpired(context.Background(), 11, now, lockedUntil, expiresAt)
	require.NoError(t, err)
	require.False(t, reclaimed)
	require.NoError(t, mock.ExpectationsWereMet())
}
