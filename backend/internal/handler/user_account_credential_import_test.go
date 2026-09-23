package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userCredentialImportRepo struct {
	service.AccountRepository
	created []service.Account
}

func (r *userCredentialImportRepo) Create(_ context.Context, account *service.Account) error {
	account.ID = int64(len(r.created) + 1)
	copy := *account
	copy.Credentials = make(map[string]any, len(account.Credentials))
	for key, value := range account.Credentials {
		copy.Credentials[key] = value
	}
	r.created = append(r.created, copy)
	return nil
}

func setupUserCredentialImportRouter(ownerUserID int64) (*gin.Engine, *userCredentialImportRepo) {
	gin.SetMode(gin.TestMode)
	repo := &userCredentialImportRepo{}
	accountService := service.NewAccountService(repo, nil)
	accountHandler := NewUserAccountHandler(accountService, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: ownerUserID})
		c.Next()
	})
	router.POST("/api/v1/accounts/import-credentials", accountHandler.ImportCredentials)
	return router, repo
}

func setupUserAccountCreateRouter(ownerUserID int64) (*gin.Engine, *userCredentialImportRepo) {
	gin.SetMode(gin.TestMode)
	repo := &userCredentialImportRepo{}
	accountService := service.NewAccountService(repo, nil)
	accountHandler := NewUserAccountHandler(accountService, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: ownerUserID})
		c.Next()
	})
	router.POST("/api/v1/accounts", accountHandler.Create)
	return router, repo
}

func TestUserAccountCreateUsesFixedPersonalDefaults(t *testing.T) {
	router, repo := setupUserAccountCreateRouter(23)
	body := []byte(`{
		"name": "personal-key",
		"platform": "openai",
		"type": "apikey",
		"share_mode": "public",
		"credentials": {"api_key": "test-key", "base_url": "https://api.example.test/v1"}
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Len(t, repo.created, 1)
	created := repo.created[0]
	require.Equal(t, int64(23), *created.OwnerUserID)
	require.Equal(t, 3, created.Concurrency)
	require.Equal(t, 1, created.Priority)
	require.True(t, created.AutoPauseOnExpired)
	require.True(t, created.Schedulable)
	require.Equal(t, service.StatusActive, created.Status)
	require.Equal(t, service.AccountShareModePrivate, created.ShareMode)
	require.Equal(t, service.AccountShareStatusApproved, created.ShareStatus)
}

func TestUserAccountCreateRejectsAdminFields(t *testing.T) {
	router, repo := setupUserAccountCreateRouter(23)
	body := []byte(`{
		"name": "personal-key",
		"platform": "openai",
		"type": "apikey",
		"credentials": {"api_key": "test-key"},
		"priority": 99
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, repo.created)
}

func TestUserAccountCredentialImportCreatesOwnedOAuthAccount(t *testing.T) {
	router, repo := setupUserCredentialImportRouter(17)
	body := []byte(`{
		"contents": [
			"{\"name\":\"personal-openai\",\"platform\":\"openai\",\"credentials\":{\"access_token\":\"test-access-token\"}}"
		],
		"share_mode": "private"
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/import-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data service.AccountCredentialImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 1, response.Data.Total)
	require.Equal(t, 1, response.Data.Created)
	require.Zero(t, response.Data.Failed)
	require.Len(t, repo.created, 1)
	require.Equal(t, service.PlatformOpenAI, repo.created[0].Platform)
	require.Equal(t, service.AccountTypeOAuth, repo.created[0].Type)
	require.NotNil(t, repo.created[0].OwnerUserID)
	require.Equal(t, int64(17), *repo.created[0].OwnerUserID)
	require.Equal(t, "test-access-token", repo.created[0].Credentials["access_token"])
	require.Equal(t, 3, repo.created[0].Concurrency)
	require.Equal(t, 1, repo.created[0].Priority)
	require.True(t, repo.created[0].AutoPauseOnExpired)
}

func TestUserAccountCredentialImportRejectsAdminFields(t *testing.T) {
	router, repo := setupUserCredentialImportRouter(17)
	body := []byte(`{
		"contents": [
			"{\"name\":\"personal-openai\",\"platform\":\"openai\",\"credentials\":{\"access_token\":\"test-access-token\"}}"
		],
		"priority": 99
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/import-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, repo.created)
}

func TestUserAccountCredentialImportRejectsUnsupportedKiroWithoutCreate(t *testing.T) {
	router, repo := setupUserCredentialImportRouter(17)
	body := []byte(`{
		"contents": [
			"{\"client_id\":\"client\",\"client_secret\":\"secret\",\"refresh_token\":\"refresh\"}"
		],
		"kiro_config_import": true
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/import-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data service.AccountCredentialImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 1, response.Data.Total)
	require.Zero(t, response.Data.Created)
	require.Equal(t, 1, response.Data.Failed)
	require.Len(t, response.Data.Errors, 1)
	require.Contains(t, response.Data.Errors[0].Message, "Kiro configuration import is not supported")
	require.Empty(t, repo.created)
}
