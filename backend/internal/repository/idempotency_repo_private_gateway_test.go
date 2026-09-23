package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPrivateGatewayNonceRepositoryClaimsAndReclaimsOnlyExpiredRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewPrivateGatewayNonceRepository(nil, db)
	scope := "private_gateway_nonce:" + "a" // production scope is bounded to 85 bytes
	nonceHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	expiresAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	query := regexp.QuoteMeta("INSERT INTO idempotency_records") + `(?s).*ON CONFLICT \(scope, idempotency_key_hash\) DO UPDATE.*RETURNING id`

	mock.ExpectQuery(query).
		WithArgs(scope, nonceHash, expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	claimed, err := repo.ClaimPrivateGatewayNonce(context.Background(), scope, nonceHash, expiresAt)
	require.NoError(t, err)
	require.True(t, claimed)

	mock.ExpectQuery(query).
		WithArgs(scope, nonceHash, expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	claimed, err = repo.ClaimPrivateGatewayNonce(context.Background(), scope, nonceHash, expiresAt)
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateGatewayNonceRepositoryFailsClosedOnDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewPrivateGatewayNonceRepository(nil, db)
	mock.ExpectQuery(`(?s)INSERT INTO idempotency_records.*RETURNING id`).
		WillReturnError(errors.New("database unavailable"))
	claimed, err := repo.ClaimPrivateGatewayNonce(context.Background(), "scope", "hash", time.Now().UTC())
	require.Error(t, err)
	require.False(t, claimed)
	require.NoError(t, mock.ExpectationsWereMet())
}
