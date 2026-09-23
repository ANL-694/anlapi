package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetAccountSharingDashboardRejectsInvalidQueries(t *testing.T) {
	svc := &UsageService{}
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		query AccountSharingQuery
	}{
		{"missing user", AccountSharingQuery{StartTime: base, EndTime: base.Add(time.Hour), Granularity: "day", Page: 1, PageSize: 20}},
		{"reversed range", AccountSharingQuery{UserID: 1, StartTime: base.Add(time.Hour), EndTime: base, Granularity: "day", Page: 1, PageSize: 20}},
		{"bad granularity", AccountSharingQuery{UserID: 1, StartTime: base, EndTime: base.Add(time.Hour), Granularity: "year", Page: 1, PageSize: 20}},
		{"bad page", AccountSharingQuery{UserID: 1, StartTime: base, EndTime: base.Add(time.Hour), Granularity: "day", Page: 0, PageSize: 20}},
		{"oversized page", AccountSharingQuery{UserID: 1, StartTime: base, EndTime: base.Add(time.Hour), Granularity: "day", Page: 1, PageSize: 1001}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.GetAccountSharingDashboard(context.Background(), tt.query)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
