package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"anlapi/internal/config"
	middleware2 "anlapi/internal/server/middleware"
	"anlapi/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageToggleSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func (r *imageToggleSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, nil
}
func (r *imageToggleSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.values[key], nil
}
func (r *imageToggleSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}
func (r *imageToggleSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *imageToggleSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *imageToggleSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *imageToggleSettingRepo) Delete(context.Context, string) error { return nil }

type imageToggleEncryptor struct{}

func (imageToggleEncryptor) Encrypt(plaintext string) (string, error)  { return plaintext, nil }
func (imageToggleEncryptor) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }

type imageToggleStorage struct{}

func (imageToggleStorage) Save(context.Context, string, string, []byte) (string, error) {
	return "https://cdn.example.test/object.png", nil
}

func TestAsyncImageEnablesWithoutRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &imageToggleSettingRepo{values: map[string]string{}}
	backup := service.NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, imageToggleEncryptor{}, nil, nil)
	factory := func(context.Context, *config.ImageStorageConfig) (service.ImageStorage, error) {
		return imageToggleStorage{}, nil
	}
	settings := service.NewImageStorageSettingService(repo, imageToggleEncryptor{}, backup, factory, config.ImageStorageConfig{})
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithResolver(store, settings.Resolver(), time.Hour, time.Minute)
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"created": 1, "data": []gin.H{{"url": "https://upstream.test/i.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: 9, UserID: 7, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	submit := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"a lighthouse"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	require.Equal(t, http.StatusNotFound, submit().Code)
	_, err := settings.Update(context.Background(), service.ImageStorageSettings{
		Enabled: true, Bucket: "my-images", Endpoint: "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	require.NoError(t, err)

	rec := submit()
	require.Equal(t, http.StatusAccepted, rec.Code)
	var accepted struct {
		TaskID  string `json:"task_id"`
		PollURL string `json:"poll_url"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))

	_, err = settings.Update(context.Background(), service.ImageStorageSettings{Enabled: false})
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, submit().Code)
	pollRec := httptest.NewRecorder()
	router.ServeHTTP(pollRec, httptest.NewRequest(http.MethodGet, accepted.PollURL, nil))
	require.Equal(t, http.StatusOK, pollRec.Code)
}
