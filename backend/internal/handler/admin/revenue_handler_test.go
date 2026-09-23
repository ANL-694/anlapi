package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newRevenueHandlerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewRevenueHandler(service.NewRevenueService(nil))
	router.GET("/api/v1/admin/revenue/summary", handler.GetSummary)
	return router
}

func TestRevenueHandlerRejectsMalformedDate(t *testing.T) {
	router := newRevenueHandlerTestRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/revenue/summary?start_date=2024-13-40", nil))

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "start_date must be YYYY-MM-DD")
}

func TestRevenueHandlerRejectsInvalidGranularity(t *testing.T) {
	router := newRevenueHandlerTestRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/revenue/summary?granularity=week", nil))

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "granularity must be day or hour")
}

func TestRevenueHandlerRejectsOverlongRangeBeforeDatabaseAccess(t *testing.T) {
	router := newRevenueHandlerTestRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/revenue/summary?start_date=2024-01-01&end_date=2025-01-03", nil))

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "date range exceeds 366 days")
}
