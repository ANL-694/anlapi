package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDashboardAccountSharingRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/usage/dashboard/account-sharing", nil)

	(&UsageHandler{}).DashboardAccountSharing(c)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestParseAccountSharingPaginationBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantPage  int
		wantSize  int
		wantError bool
	}{
		{name: "defaults", query: "", wantPage: 1, wantSize: 20},
		{name: "max page size", query: "account_page=2&account_page_size=1000", wantPage: 2, wantSize: 1000},
		{name: "zero page", query: "account_page=0", wantError: true},
		{name: "negative size", query: "account_page_size=-1", wantError: true},
		{name: "oversized", query: "account_page_size=1001", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/usage/dashboard/account-sharing?"+tt.query, nil)
			page, pageSize, err := parseAccountSharingPagination(c)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantPage, page)
			require.Equal(t, tt.wantSize, pageSize)
		})
	}
}

func TestParseUserDashboardTimeRangeStrictValidatesDates(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantError bool
	}{
		{name: "default range", query: ""},
		{name: "valid explicit range", query: "start_date=2026-08-01&end_date=2026-08-07"},
		{name: "invalid date", query: "start_date=not-a-date", wantError: true},
		{name: "reversed range", query: "start_date=2026-08-08&end_date=2026-08-01", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/usage/dashboard/account-sharing?"+tt.query, nil)
			start, end, err := parseUserDashboardTimeRangeStrict(c)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, end.After(start))
		})
	}
}
