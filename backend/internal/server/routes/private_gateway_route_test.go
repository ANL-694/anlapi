package routes

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesPrivateAuthRunsBeforePublicAPIKeyAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024, TextMaxBodySize: 1024 * 1024},
		PrivateGateway: config.PrivateGatewayConfig{
			Enabled:    true,
			ServiceID:  "anl-hub-test",
			SigningKey: "test-signing-key-for-anlapi-service-123456",
			APIKeyID:   42,
		},
	}
	var apiKeyCalls atomic.Int32
	router := gin.New()
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			apiKeyCalls.Add(1)
			c.Status(http.StatusNoContent)
		}),
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		servermiddleware.NewANLPrivateGateway(cfg, nil, nil),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-ANL-Service-ID", "anl-hub-test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Zero(t, apiKeyCalls.Load())
}

func TestCommonHealthRouteRemainsPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}
