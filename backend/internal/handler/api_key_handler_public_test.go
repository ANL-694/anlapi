package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type apiKeyHandlerRepoStub struct {
	service.APIKeyRepository
	keys map[int64]*service.APIKey
}

func newAPIKeyHandlerRepoStub() *apiKeyHandlerRepoStub {
	return &apiKeyHandlerRepoStub{keys: map[int64]*service.APIKey{
		7: {
			ID:     7,
			UserID: 42,
			Name:   "prod-key",
			Key:    "fixture-user-handler-1234567890",
			Status: service.StatusAPIKeyActive,
		},
	}}
}

func (s *apiKeyHandlerRepoStub) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	items := make([]service.APIKey, 0, len(s.keys))
	for _, key := range s.keys {
		if key.UserID == userID {
			items = append(items, *key)
		}
	}
	return items, &pagination.PaginationResult{Total: int64(len(items)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *apiKeyHandlerRepoStub) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	key, ok := s.keys[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (s *apiKeyHandlerRepoStub) Update(ctx context.Context, key *service.APIKey, fields service.APIKeyUpdateFields) error {
	clone := *key
	s.keys[key.ID] = &clone
	return nil
}

func setupUserAPIKeyHandlerRouter(repo *apiKeyHandlerRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	h := NewAPIKeyHandler(service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil))
	router.GET("/api/v1/api-keys", h.List)
	router.GET("/api/v1/api-keys/:id", h.GetByID)
	router.PUT("/api/v1/api-keys/:id", h.Update)
	return router
}

func TestAPIKeyHandlerReadEndpointsMaskKeyMaterial(t *testing.T) {
	router := setupUserAPIKeyHandlerRouter(newAPIKeyHandlerRepoStub())

	for _, target := range []string{"/api/v1/api-keys", "/api/v1/api-keys/7"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.NotContains(t, rec.Body.String(), "fixture-user-handler-1234567890")
		require.Contains(t, rec.Body.String(), "fixtur...7890")
	}
}

func TestAPIKeyHandlerUpdateMasksKeyMaterial(t *testing.T) {
	router := setupUserAPIKeyHandlerRouter(newAPIKeyHandlerRepoStub())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/api-keys/7", strings.NewReader("{\"name\":\"renamed\"}"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "fixture-user-handler-1234567890")
	require.Contains(t, rec.Body.String(), "fixtur...7890")
}
