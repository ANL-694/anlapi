package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"anlapi/internal/handler"
	adminhandler "anlapi/internal/handler/admin"
	servermiddleware "anlapi/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingProbeAdminRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		Account: adminhandler.NewAccountHandler(nil),
	}}
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil)

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/admin/accounts/upstream-billing-probe/settings"},
		{method: http.MethodPut, path: "/api/v1/admin/accounts/upstream-billing-probe/settings"},
		{method: http.MethodPost, path: "/api/v1/admin/accounts/upstream-billing-probe/batch"},
	} {
		t.Run(testCase.method+" "+testCase.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}
