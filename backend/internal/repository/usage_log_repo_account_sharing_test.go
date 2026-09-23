package repository

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetAccountSharingDashboardUsesAppliedSettlementsAndPublicDetails(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)

	accountRows := sqlmock.NewRows([]string{
		"owned_accounts", "public_accounts", "private_accounts", "public_pending_accounts", "public_approved_accounts", "public_suspended_accounts",
		"self_requests", "self_tokens", "self_actual_cost", "self_account_cost",
		"external_requests", "external_consumer_charge", "external_account_cost", "external_owner_credit", "external_platform_fee",
		"account_id", "name", "platform", "share_mode", "share_status",
		"account_self_requests", "account_self_tokens", "account_self_actual_cost", "account_self_cost",
		"account_external_requests", "account_external_charge", "account_external_cost", "account_owner_credit", "account_platform_fee",
	}).AddRow(
		3, 2, 1, 1, 1, 0,
		4, 100, 1.5, 1.2,
		3, 3.0, 1.4, 0.9, 0.7,
		99, "shared-account", "openai", "public", "approved",
		2, 50, 0.7, 0.6,
		3, 3.0, 1.4, 0.9, 0.7,
	)
	mock.ExpectQuery(regexp.QuoteMeta("WITH self_usage AS (")).
		WithArgs(int64(7), start, end, 20, 0).
		WillReturnRows(accountRows)

	trendRows := sqlmock.NewRows([]string{
		"date", "self_requests", "self_tokens", "self_actual_cost", "self_account_cost",
		"external_requests", "external_consumer_charge", "external_account_cost", "external_owner_credit", "external_platform_fee",
	}).AddRow("2026-08-01", 4, 100, 1.5, 1.2, 3, 3.0, 1.4, 0.9, 0.7)
	mock.ExpectQuery(regexp.QuoteMeta("WITH self_usage AS (")).
		WithArgs(int64(7), start, end).
		WillReturnRows(trendRows)

	result, err := repo.GetAccountSharingDashboard(context.Background(), service.AccountSharingQuery{
		UserID: 7, StartTime: start, EndTime: end, Granularity: "day", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Summary.OwnedAccounts)
	require.Equal(t, int64(1), result.Summary.PrivateAccounts)
	require.InDelta(t, 0.9, result.Summary.ExternalOwnerCredit, 1e-9)
	require.InDelta(t, 1.5, result.Summary.SelfActualCost, 1e-9)
	require.InDelta(t, -0.6, result.Summary.BalanceNetChange, 1e-9)
	require.Equal(t, int64(2), result.AccountsPagination.Total)
	require.Len(t, result.Accounts, 1)
	require.Equal(t, "public", result.Accounts[0].ShareMode)
	require.Len(t, result.Trend, 1)
	require.Equal(t, "2026-08-01", result.Trend[0].Date)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAccountSharingDashboardQueryContainsSuccessAndSettlementGuards(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	nilAccountValues := make([]driver.Value, 14)
	accountRows := sqlmock.NewRows(make([]string, 29)).AddRow(append([]driver.Value{
		1, 0, 1, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0, 0,
	}, nilAccountValues...)...)
	mock.ExpectQuery(`(?s)WITH self_usage AS .*ul\.actual_cost > 0.*FROM account_share_settlement_entries ase\s+JOIN accounts a\s+ON a\.id = ase\.account_id\s+AND a\.owner_user_id = ase\.owner_user_id\s+AND a\.deleted_at IS NULL.*ase\.status = 'applied'.*WHERE share_mode = 'public'`).
		WithArgs(int64(9), start, end, 20, 0).
		WillReturnRows(accountRows)
	mock.ExpectQuery(`(?s)WITH self_usage AS .*ul\.actual_cost > 0.*TO_CHAR\(ase\.created_at, 'YYYY-MM-DD'\) AS date.*FROM account_share_settlement_entries ase\s+JOIN accounts a\s+ON a\.id = ase\.account_id\s+AND a\.owner_user_id = ase\.owner_user_id\s+AND a\.deleted_at IS NULL.*ase\.status = 'applied'`).
		WithArgs(int64(9), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"date", "self_requests", "self_tokens", "self_actual_cost", "self_account_cost", "external_requests", "external_consumer_charge", "external_account_cost", "external_owner_credit", "external_platform_fee"}))

	result, err := repo.GetAccountSharingDashboard(context.Background(), service.AccountSharingQuery{
		UserID: 9, StartTime: start, EndTime: end, Granularity: "day", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Empty(t, result.Accounts)
	require.Equal(t, int64(1), result.Summary.PrivateAccounts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountSharingPagesUsesOnePageForEmptyResults(t *testing.T) {
	require.Equal(t, 1, accountSharingPages(0, 20))
	require.Equal(t, 2, accountSharingPages(21, 20))
}
