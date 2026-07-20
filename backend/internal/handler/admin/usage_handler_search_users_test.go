package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"anl-api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type searchUsersAdminStub struct {
	service.AdminService
	filters service.UserListFilters
}

func (s *searchUsersAdminStub) ListUsers(_ context.Context, _, _ int, filters service.UserListFilters, _, _ string) ([]service.User, int64, error) {
	s.filters = filters
	deletedAt := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	return []service.User{
		{ID: 1, Email: "active@test.com"},
		{ID: 2, Email: "deleted@test.com", DeletedAt: &deletedAt},
	}, 2, nil
}

func TestAdminUsageSearchUsersIncludesDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &searchUsersAdminStub{}
	handler := NewUsageHandler(nil, nil, stub, nil)
	router := gin.New()
	router.GET("/admin/usage/search-users", handler.SearchUsers)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/usage/search-users?q=test", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, stub.filters.IncludeDeleted)

	var response struct {
		Data []struct {
			ID      int64  `json:"id"`
			Email   string `json:"email"`
			Deleted bool   `json:"deleted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	require.False(t, response.Data[0].Deleted)
	require.True(t, response.Data[1].Deleted)
}
