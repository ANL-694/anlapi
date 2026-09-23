package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userAccountModelsRepo struct {
	service.AccountRepository
	account service.Account
}

func (r *userAccountModelsRepo) GetOwnedByID(_ context.Context, ownerUserID, accountID int64) (*service.Account, error) {
	if r.account.ID != accountID || r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return nil, service.ErrAccountNotFound
	}
	account := r.account
	return &account, nil
}

func (r *userAccountModelsRepo) ListOwnedWithFilters(_ context.Context, ownerUserID int64, _ pagination.PaginationParams, _, _, _, _ string, _ int64, _ string) ([]service.Account, *pagination.PaginationResult, error) {
	if r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return nil, &pagination.PaginationResult{}, nil
	}
	return []service.Account{r.account}, &pagination.PaginationResult{Total: 1}, nil
}

func setupUserAccountModelsRouter(ownerUserID int64, account service.Account) *gin.Engine {
	gin.SetMode(gin.TestMode)
	accountService := service.NewAccountService(&userAccountModelsRepo{account: account}, nil)
	userAccountHandler := NewUserAccountHandler(accountService, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: ownerUserID})
		c.Next()
	})
	router.GET("/api/v1/accounts/:id/models", userAccountHandler.GetAvailableModels)
	return router
}

func TestUserAccountHandlerGetAvailableModelsUsesOfficialResolver(t *testing.T) {
	ownerUserID := int64(7)
	router := setupUserAccountModelsRouter(ownerUserID, service.Account{
		ID:          44,
		Name:        "personal-openai",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		OwnerUserID: &ownerUserID,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"},
		},
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/44/models", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, []string{"gpt-5.6-sol"}, []string{response.Data[0].ID})
}

func TestUserAccountHandlerGetAvailableModelsHidesOtherUsersAccount(t *testing.T) {
	ownerUserID := int64(7)
	otherUserID := int64(8)
	router := setupUserAccountModelsRouter(ownerUserID, service.Account{
		ID:          45,
		Name:        "another-users-account",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		OwnerUserID: &otherUserID,
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/45/models", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestNormalizeUserAccountIDsDropsInvalidAndDuplicates(t *testing.T) {
	require.Equal(t, []int64{3, 4}, normalizeUserAccountIDs([]int64{0, 3, 3, -2, 4}))
}
