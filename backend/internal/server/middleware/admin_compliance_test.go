package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"anl-api/internal/config"
	"anl-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type complianceGuardRepoStub struct {
	values map[string]string
}

func (r *complianceGuardRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	if value, ok := r.values[key]; ok {
		return &service.Setting{Key: key, Value: value}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (r *complianceGuardRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *complianceGuardRepoStub) Set(context.Context, string, string) error { return nil }

func (r *complianceGuardRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *complianceGuardRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (r *complianceGuardRepoStub) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *complianceGuardRepoStub) Delete(context.Context, string) error { return nil }

func TestAdminComplianceGuardBlocksAdminRouteWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(&complianceGuardRepoStub{}, &config.Config{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 1})
		c.Next()
	})
	router.Use(AdminComplianceGuard(svc))
	router.GET("/api/v1/admin/users", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusLocked, w.Code)
	require.Contains(t, w.Body.String(), "ADMIN_COMPLIANCE_ACK_REQUIRED")
}

func TestAdminComplianceGuardBypassesComplianceEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(&complianceGuardRepoStub{}, &config.Config{})
	router := gin.New()
	router.Use(AdminComplianceGuard(svc))
	router.GET("/api/v1/admin/compliance", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/compliance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ok", w.Body.String())
}
