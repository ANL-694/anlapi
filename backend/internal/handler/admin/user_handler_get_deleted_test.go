package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ikik-api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type getByIDAdminStub struct {
	service.AdminService
	includeDeletedCalled bool
}

func (s *getByIDAdminStub) GetUser(context.Context, int64) (*service.User, error) {
	return nil, service.ErrUserNotFound
}

func (s *getByIDAdminStub) GetUserIncludeDeleted(_ context.Context, id int64) (*service.User, error) {
	s.includeDeletedCalled = true
	return &service.User{ID: id, Email: "deleted@test.com", Status: service.StatusActive}, nil
}

func TestAdminUserGetByIDIncludeDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &getByIDAdminStub{}
	handler := NewUserHandler(stub)
	router := gin.New()
	router.GET("/admin/users/:id", handler.GetByID)

	t.Run("normal lookup keeps soft-delete filtering", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/admin/users/7", nil)
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("include_deleted uses the audit lookup", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/admin/users/7?include_deleted=true", nil)
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.True(t, stub.includeDeletedCalled)
	})
}
