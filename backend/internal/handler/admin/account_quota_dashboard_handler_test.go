package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaDashboardAdminService struct {
	*stubAdminService
	dashboard *service.AccountQuotaDashboard
}

func (s *quotaDashboardAdminService) GetAccountQuotaDashboard(context.Context) (*service.AccountQuotaDashboard, error) {
	return s.dashboard, nil
}

func TestAccountHandlerGetQuotaDashboard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dashboard := &service.AccountQuotaDashboard{
		GeneratedAt: time.Unix(100, 0).UTC(),
		Totals: service.AccountQuotaSummary{
			Platform:           "all",
			Type:               "all",
			AccountCount:       3,
			ActiveAccountCount: 2,
		},
	}
	adminService := &quotaDashboardAdminService{stubAdminService: newStubAdminService(), dashboard: dashboard}
	handler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/accounts/quota-dashboard", handler.GetQuotaDashboard)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/quota-dashboard", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload struct {
		Data service.AccountQuotaDashboard `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, int(3), payload.Data.Totals.AccountCount)
}

func TestAccountHandlerGetQuotaDashboardReturnsUnavailableWhenUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/accounts/quota-dashboard", handler.GetQuotaDashboard)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/quota-dashboard", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
