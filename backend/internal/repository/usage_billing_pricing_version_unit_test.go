package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingRepositoryApplyPersistsPricingVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO usage_billing_dedup \(request_id, api_key_id, request_fingerprint, pricing_version\)`).
		WithArgs("pricing-version-request", int64(7), "fingerprint", "pv1-example").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT request_fingerprint\s+FROM usage_billing_dedup_archive`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), &service.UsageBillingCommand{
		RequestID:          "pricing-version-request",
		APIKeyID:           7,
		RequestFingerprint: "fingerprint",
		PricingVersion:     "  pv1-example  ",
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}
