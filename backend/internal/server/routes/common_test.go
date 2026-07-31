package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHealthContractReturnsExactJSONStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))
	require.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}
