package handler

import (
	"bytes"
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

type userAccountUpdateRepo struct {
	service.AccountRepository
	account service.Account
	updated bool
}

func (r *userAccountUpdateRepo) GetOwnedByID(_ context.Context, ownerUserID, accountID int64) (*service.Account, error) {
	if r.account.ID != accountID || r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return nil, service.ErrAccountNotFound
	}
	account := r.account
	return &account, nil
}

func (r *userAccountUpdateRepo) ListOwnedWithFilters(_ context.Context, ownerUserID int64, _ pagination.PaginationParams, _, _, _, _ string, _ int64, _ string) ([]service.Account, *pagination.PaginationResult, error) {
	if r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return nil, &pagination.PaginationResult{}, nil
	}
	return []service.Account{r.account}, &pagination.PaginationResult{Total: 1}, nil
}

func (r *userAccountUpdateRepo) UpdateOwned(_ context.Context, ownerUserID int64, account *service.Account) error {
	if account == nil || account.OwnerUserID == nil || *account.OwnerUserID != ownerUserID {
		return service.ErrAccountNotFound
	}
	r.account = *account
	r.updated = true
	return nil
}

func (r *userAccountUpdateRepo) DeleteOwned(_ context.Context, ownerUserID, accountID int64) error {
	if r.account.ID != accountID || r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return service.ErrAccountNotFound
	}
	return nil
}

func setupUserAccountUpdateRouter(ownerUserID int64, account service.Account) (*gin.Engine, *userAccountUpdateRepo) {
	gin.SetMode(gin.TestMode)
	repo := &userAccountUpdateRepo{account: account}
	accountService := service.NewAccountService(repo, nil)
	accountHandler := NewUserAccountHandler(accountService, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: ownerUserID})
		c.Next()
	})
	router.PUT("/api/v1/accounts/:id", accountHandler.Update)
	router.POST("/api/v1/accounts/bulk-update", accountHandler.BulkUpdate)
	return router, repo
}

func TestUserAccountUpdateRejectsAdminFields(t *testing.T) {
	ownerID := int64(7)
	router, repo := setupUserAccountUpdateRouter(ownerID, service.Account{
		ID:          52,
		OwnerUserID: &ownerID,
		Name:        "personal",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Priority:    50,
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/accounts/52", bytes.NewBufferString(`{"status":"disabled","priority":1}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, repo.updated)
	require.Equal(t, service.StatusActive, repo.account.Status)
	require.Equal(t, 50, repo.account.Priority)
}

func TestUserAccountUpdateKeepsUserCapabilities(t *testing.T) {
	ownerID := int64(7)
	router, repo := setupUserAccountUpdateRouter(ownerID, service.Account{
		ID:          53,
		OwnerUserID: &ownerID,
		Name:        "old name",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "old-key",
			"base_url": "https://api.deepseek.com",
		},
		Extra: map[string]any{
			"free_model_disabled_models": map[string]any{},
			"quota_limit":                float64(10),
		},
	})
	body := []byte(`{
		"name":"new name",
		"credentials":{"api_key":"new-key","base_url":"https://api.deepseek.com/v1","model_mapping":{"deepseek-chat":"deepseek-chat"}},
		"extra":{"quota_limit":10,"free_model_disabled_models":{"deepseek-reasoner":{"error":"unavailable"}},"free_model_last_filter_at":"2026-08-18T00:00:00Z"}
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/accounts/53", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, repo.updated)
	require.Equal(t, "new name", repo.account.Name)
	require.Equal(t, "new-key", repo.account.Credentials["api_key"])
	require.Equal(t, "https://api.deepseek.com/v1", repo.account.Credentials["base_url"])
	require.Equal(t, float64(10), repo.account.Extra["quota_limit"])
	require.Contains(t, repo.account.Extra, "free_model_last_filter_at")

	var response struct {
		Data struct {
			Credentials       map[string]any  `json:"credentials"`
			CredentialsStatus map[string]bool `json:"credentials_status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotContains(t, response.Data.Credentials, "api_key")
	require.True(t, response.Data.CredentialsStatus["has_api_key"])
}

func TestUserAccountBulkUpdateRejectsAdminFields(t *testing.T) {
	ownerID := int64(7)
	router, repo := setupUserAccountUpdateRouter(ownerID, service.Account{
		ID:          54,
		OwnerUserID: &ownerID,
		Name:        "personal",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/bulk-update", bytes.NewBufferString(`{"account_ids":[54],"schedulable":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, repo.updated)
}

func TestUserAccountUpdateRejectsFreeModelWhenFeatureDisabled(t *testing.T) {
	ownerID := int64(7)
	router, repo := setupUserAccountUpdateRouter(ownerID, service.Account{
		ID:          55,
		OwnerUserID: &ownerID,
		Name:        "personal",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "old-key",
			"base_url": "https://api.groq.com/openai/v1",
		},
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/accounts/55", bytes.NewBufferString(`{"extra":{"free_model_provider":"groq"}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.False(t, repo.updated)
	require.NotContains(t, recorder.Body.String(), "old-key")
}
