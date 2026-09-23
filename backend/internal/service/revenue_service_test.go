package service

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newRevenueTestClient(t *testing.T) (*dbent.Client, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	return client, mock
}

func TestRevenueSnapshotDateRangeUsesShanghaiDay(t *testing.T) {
	params := RevenueQueryParams{
		StartTime: time.Date(2024, 1, 1, 16, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC),
	}

	startDate, endDate := revenueSnapshotDateRange(params)

	require.Equal(t, "2024-01-02", startDate)
	require.Equal(t, "2024-01-03", endDate)
}

func TestShouldUseRevenueDailySnapshotsRequiresShanghaiFullDay(t *testing.T) {
	loc := revenueSnapshotBusinessLocation()
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)

	require.True(t, shouldUseRevenueDailySnapshots(RevenueQueryParams{
		StartTime:   start.UTC(),
		EndTime:     end.UTC(),
		Granularity: RevenueGranularityDay,
		Timezone:    revenueSnapshotBusinessTimezone,
	}))
	require.False(t, shouldUseRevenueDailySnapshots(RevenueQueryParams{
		StartTime:   start.UTC(),
		EndTime:     end.UTC(),
		Granularity: RevenueGranularityDay,
		Timezone:    "UTC",
	}))
	require.False(t, shouldUseRevenueDailySnapshots(RevenueQueryParams{
		StartTime:   start.Add(time.Hour),
		EndTime:     end,
		Granularity: RevenueGranularityDay,
		Timezone:    revenueSnapshotBusinessTimezone,
	}))
}

func TestRevenueSummaryValidatesRangeBeforeDatabaseAvailability(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := NewRevenueService(nil).GetSummary(context.Background(), RevenueQueryParams{
		StartTime:   start,
		EndTime:     start.AddDate(1, 1, 2),
		Granularity: RevenueGranularityDay,
		Timezone:    "UTC",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "date range exceeds 366 days")
}

func TestRevenueWalletBreakdownSkipsMissingPointsLedger(t *testing.T) {
	client, mock := newRevenueTestClient(t)
	service := NewRevenueService(client)
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	mock.ExpectQuery(`(?s)FROM user_balance_ledger.*reason = \$3`).
		WithArgs(start, end, "usage_charge").
		WillReturnRows(sqlmock.NewRows([]string{"balance_consumed"}).AddRow(4.25))
	mock.ExpectQuery(`(?s)information_schema\.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`(?s)FROM user_balance_ledger.*reason = \$4.*GROUP BY 1`).
		WithArgs(start, end, "UTC", "usage_charge").
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "balance_consumed"}).AddRow("2024-01-02", 4.25))

	out := &RevenueSummary{Trend: []RevenueTrendPoint{{Date: "2024-01-02"}}}
	err := service.fillRevenueWalletBreakdownStats(context.Background(), RevenueQueryParams{
		StartTime:   start,
		EndTime:     end,
		Granularity: RevenueGranularityDay,
		Timezone:    "UTC",
	}, out, map[string]int{"2024-01-02": 0})

	require.NoError(t, err)
	require.Equal(t, 4.25, out.Usage.BalanceConsumedAmount)
	require.Zero(t, out.Usage.PointsConsumedAmount)
	require.Zero(t, out.Usage.PointsIssuedAmount)
	require.Equal(t, 4.25, out.Trend[0].BalanceConsumedAmount)
	require.Zero(t, out.Trend[0].PointsConsumedAmount)
	require.Zero(t, out.Trend[0].PointsIssuedAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}
