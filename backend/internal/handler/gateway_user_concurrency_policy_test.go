package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"anlapi/internal/config"
	middleware2 "anlapi/internal/server/middleware"
	"anlapi/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyHelperAcquireUserSlotWithQueueUsesUserScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

	streamStarted := false
	release, err := helper.AcquireUserSlotWithQueue(c, 101, 2, false, &streamStarted)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.releaseUserCalled))
}

func TestGatewayHandlerUserMessageQueueIsDisabled(t *testing.T) {
	h := &GatewayHandler{
		cfg:                &config.Config{Gateway: config.GatewayConfig{UserMessageQueue: config.UserMessageQueueConfig{Mode: config.UMQModeSerialize}}},
		userMsgQueueHelper: &UserMsgQueueHelper{},
	}
	require.Empty(t, h.getUserMsgQueueMode(&service.Account{}, &service.ParsedRequest{}))
}

func TestGatewayCountTokensUsesUserConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return false, errors.New("user slot unavailable")
		},
	}
	h := &GatewayHandler{
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
	c, recorder := newCountTokensConcurrencyContext(t)
	h.CountTokens(c)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.acquireUserCalled))
}

func TestOpenAIGatewayCountTokensUsesUserConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return false, errors.New("user slot unavailable")
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	c, recorder := newCountTokensConcurrencyContext(t)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      1,
		UserID:  101,
		GroupID: int64Pointer(1),
		Group: &service.Group{
			ID:                    1,
			Platform:              service.PlatformOpenAI,
			AllowMessagesDispatch: true,
		},
		User: &service.User{ID: 101},
	})
	h.CountTokens(c)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Service temporarily unavailable")
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.acquireUserCalled))
}

func newCountTokensConcurrencyContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:     1,
		UserID: 101,
		User:   &service.User{ID: 101},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 101, Concurrency: 1})
	return c, recorder
}

func int64Pointer(value int64) *int64 {
	return &value
}
