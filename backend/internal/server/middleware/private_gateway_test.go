package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"anlapi/internal/config"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var privateGatewayTestNow = time.Unix(1_785_475_641, 0).UTC()

func privateGatewayTestConfig() config.PrivateGatewayConfig {
	return config.PrivateGatewayConfig{
		Enabled:    true,
		ServiceID:  "anl-hub-test",
		SigningKey: "test-signing-key-for-anlapi-service-123456",
		APIKeyID:   42,
	}
}

func newPrivateGatewayTestRuntime() *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntime(
		privateGatewayTestConfig(),
		func(_ context.Context, id int64) (*service.APIKey, error) {
			if id != 42 {
				return nil, service.ErrAPIKeyNotFound
			}
			return &service.APIKey{ID: id}, nil
		},
		func() time.Time { return privateGatewayTestNow },
	)
}

func TestANLPrivateGatewayAuthenticationAcceptsContractRoutesAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for i, route := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/v1/models?preview=true", ""},
		{http.MethodPost, "/v1/chat/completions?preview=true", `{"model":"gpt-5.6-terra"}`},
		{http.MethodPost, "/v1/images/generations", `{"model":"gpt-image-2"}`},
		{http.MethodPost, "/v1/images/edits", "multipart-test-body"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			gateway := newPrivateGatewayTestRuntime()
			router := gin.New()
			router.Use(gateway.Authentication())
			router.Any("/*path", func(c *gin.Context) {
				apiKey, ok := takePrivateGatewayPreauthenticatedAPIKey(c)
				require.True(t, ok)
				require.Equal(t, int64(42), apiKey.ID)
				restored, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, route.body, string(restored))
				require.Empty(t, c.GetHeader(privateGatewaySignatureHeader))
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			signPrivateGatewayTestRequest(req, []byte(route.body), fmt.Sprintf("%032x", i+1), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)

			require.Equal(t, http.StatusNoContent, response.Code)
		})
	}
}

func TestANLPrivateGatewayAuthenticationRejectsInvalidExpiredAndReplayedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newRouter := func(gateway *ANLPrivateGateway, calls *atomic.Int32) *gin.Engine {
		router := gin.New()
		router.Use(gateway.Authentication())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			c.Status(http.StatusNoContent)
		})
		return router
	}

	t.Run("missing headers", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(), &calls)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
		require.Zero(t, calls.Load())
	})

	t.Run("body tamper", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"tampered":true}`))
		signPrivateGatewayTestRequest(req, []byte(`{"original":true}`), "00000000000000000000000000000011", privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
		require.Zero(t, calls.Load())
	})

	t.Run("signature tamper", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000014", privateGatewayTestNow)
		req.Header.Set(privateGatewaySignatureHeader, strings.Repeat("0", sha256.Size*2))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
		require.Zero(t, calls.Load())
	})

	t.Run("expired timestamp", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000012", privateGatewayTestNow.Add(-61*time.Second))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_EXPIRED")
		require.Zero(t, calls.Load())
	})

	t.Run("nonce replay", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(), &calls)
		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
			signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000013", privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if attempt == 0 {
				require.Equal(t, http.StatusNoContent, response.Code)
			} else {
				requirePrivateGatewayErrorCode(t, response, http.StatusConflict, "ANL_SERVICE_REPLAYED")
			}
		}
		require.Equal(t, int32(1), calls.Load())
	})
}

func TestANLPrivateGatewayNonceReplayCheckIsAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newPrivateGatewayTestRuntime()
	var calls atomic.Int32
	router := gin.New()
	router.Use(gateway.Authentication())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		calls.Add(1)
		c.Status(http.StatusNoContent)
	})

	const concurrency = 16
	results := make(chan int, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
			signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000021", privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			results <- response.Code
		}()
	}

	successes := 0
	replays := 0
	for i := 0; i < concurrency; i++ {
		switch <-results {
		case http.StatusNoContent:
			successes++
		case http.StatusConflict:
			replays++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, concurrency-1, replays)
	require.Equal(t, int32(1), calls.Load())
}

func TestANLPrivateGatewayAllowsExistingPublicAPIKeyBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newPrivateGatewayTestRuntime()
	router := gin.New()
	router.Use(gateway.Authentication())
	router.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer public-test-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestANLPrivateGatewayIdempotencyReplayConflictAndRequiredKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("replays terminal response", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		var calls atomic.Int32
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			c.Header("X-Test-Result", "preserved")
			c.JSON(http.StatusCreated, gin.H{"id": "chatcmpl-test", "usage": gin.H{"total_tokens": 3}})
		})

		var firstBody []byte
		for attempt := 0; attempt < 2; attempt++ {
			body := []byte(`{"model":"gpt-5.6-terra"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", "idem-replay-test")
			signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x30+attempt), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusCreated, response.Code)
			require.Equal(t, "preserved", response.Header().Get("X-Test-Result"))
			if attempt == 0 {
				firstBody = append([]byte(nil), response.Body.Bytes()...)
				require.Empty(t, response.Header().Get(privateGatewayIdempotencyReplayHeader))
			} else {
				require.Equal(t, "true", response.Header().Get(privateGatewayIdempotencyReplayHeader))
				require.Equal(t, firstBody, response.Body.Bytes())
			}
		}
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("replays terminal failure", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		var calls atomic.Int32
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "UPSTREAM_TEMPORARY", "Upstream is temporarily unavailable")
		})

		for attempt := 0; attempt < 2; attempt++ {
			body := []byte(`{"model":"gpt-5.6-terra"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", "idem-failure-test")
			signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x70+attempt), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			requirePrivateGatewayErrorCode(t, response, http.StatusServiceUnavailable, "UPSTREAM_TEMPORARY")
			if attempt == 1 {
				require.Equal(t, "true", response.Header().Get(privateGatewayIdempotencyReplayHeader))
			}
		}
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("rejects changed request", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/images/generations", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": []any{}}) })

		for attempt, body := range [][]byte{[]byte(`{"prompt":"one"}`), []byte(`{"prompt":"two"}`)} {
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", "idem-conflict-test")
			signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x40+attempt), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if attempt == 0 {
				require.Equal(t, http.StatusOK, response.Code)
			} else {
				requirePrivateGatewayErrorCode(t, response, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
			}
		}
	})

	t.Run("requires key", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/images/edits", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		body := []byte("multipart-test-body")
		req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
		signPrivateGatewayTestRequest(req, body, "00000000000000000000000000000051", privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusBadRequest, "INVALID_REQUEST")
	})
}

func TestANLPrivateGatewayIdempotencyRejectsConcurrentDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newPrivateGatewayTestRuntime()
	started := make(chan struct{})
	release := make(chan struct{})
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		close(started)
		<-release
		c.JSON(http.StatusOK, gin.H{"id": "chatcmpl-test"})
	})

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "idem-in-progress-test")
		signPrivateGatewayTestRequest(req, body, "00000000000000000000000000000061", privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		firstDone <- response
	}()
	<-started

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	secondReq.Header.Set("Idempotency-Key", "idem-in-progress-test")
	signPrivateGatewayTestRequest(secondReq, body, "00000000000000000000000000000062", privateGatewayTestNow)
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondReq)
	requirePrivateGatewayErrorCode(t, secondResponse, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS")
	require.Equal(t, "1", secondResponse.Header().Get("Retry-After"))

	close(release)
	require.Equal(t, http.StatusOK, (<-firstDone).Code)
}

func signPrivateGatewayTestRequest(req *http.Request, body []byte, nonce string, timestamp time.Time) {
	cfg := privateGatewayTestConfig()
	bodyDigest := sha256.Sum256(body)
	timestampRaw := strconvFormatPrivateGatewayTestTimestamp(timestamp)
	canonical := strings.Join([]string{
		privateGatewayProtocol,
		cfg.ServiceID,
		timestampRaw,
		nonce,
		strings.ToUpper(req.Method),
		privateGatewayEscapedPathAndQuery(req),
		hex.EncodeToString(bodyDigest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(cfg.SigningKey))
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set(privateGatewayServiceIDHeader, cfg.ServiceID)
	req.Header.Set(privateGatewayTimestampHeader, timestampRaw)
	req.Header.Set(privateGatewayNonceHeader, nonce)
	req.Header.Set(privateGatewayBodySHA256Header, hex.EncodeToString(bodyDigest[:]))
	req.Header.Set(privateGatewaySignatureHeader, hex.EncodeToString(mac.Sum(nil)))
}

func strconvFormatPrivateGatewayTestTimestamp(value time.Time) string {
	return fmt.Sprintf("%d", value.UTC().Unix())
}

func requirePrivateGatewayErrorCode(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, response.Code)
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, code, payload.Error.Code)
}
