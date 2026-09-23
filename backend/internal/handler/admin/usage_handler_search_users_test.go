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

// 捕获 ListUsers 入参、返回一个已删用户的 admin service 桩。
type searchUsersAdminStub struct {
	service.AdminService
	gotFilters service.UserListFilters
}

type searchAPIKeysRepoStub struct {
	service.APIKeyRepository
	keys []service.APIKey
}

func (s *searchAPIKeysRepoStub) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return s.keys, nil
}

func (s *searchUsersAdminStub) ListUsers(ctx context.Context, page, pageSize int, filters service.UserListFilters, sortBy, sortOrder string) ([]service.User, int64, error) {
	s.gotFilters = filters
	ts := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	return []service.User{
		{ID: 1, Email: "active@test.com"},
		{ID: 2, Email: "deleted@test.com", DeletedAt: &ts},
	}, 2, nil
}

func TestAdminUsageSearchUsers_IncludesDeletedAndFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &searchUsersAdminStub{}
	handler := NewUsageHandler(nil, nil, stub, nil)
	router := gin.New()
	router.GET("/admin/usage/search-users", handler.SearchUsers)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/search-users?q=test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, stub.gotFilters.IncludeDeleted, "SearchUsers 必须请求 IncludeDeleted")

	var resp struct {
		Data []struct {
			ID      int64  `json:"id"`
			Email   string `json:"email"`
			Deleted bool   `json:"deleted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	require.False(t, resp.Data[0].Deleted)
	require.True(t, resp.Data[1].Deleted, "已删用户必须标记 deleted=true")
}

func TestAdminUsageSearchAPIKeys_DoesNotReturnKeyMaterial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKeySvc := service.NewAPIKeyService(&searchAPIKeysRepoStub{
		keys: []service.APIKey{{
			ID:     7,
			UserID: 42,
			Name:   "prod-key",
			Key:    "fixture-admin-search-1234567890",
		}},
	}, nil, nil, nil, nil, nil, nil)
	handler := NewUsageHandler(nil, apiKeySvc, nil, nil)
	router := gin.New()
	router.GET("/admin/usage/search-api-keys", handler.SearchAPIKeys)

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/search-api-keys?user_id=42&q=prod", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "fixture-admin-search-1234567890")
	require.NotContains(t, rec.Body.String(), "\"key\"")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	items, ok := payload["data"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(7), item["id"])
	require.Equal(t, "prod-key", item["name"])
	require.Equal(t, float64(42), item["user_id"])
}
