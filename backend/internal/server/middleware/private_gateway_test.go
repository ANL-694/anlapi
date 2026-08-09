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

func newPrivateGatewayTestRuntimeWithState(state service.PrivateGatewayStateRepository) *ANLPrivateGateway {
	return newPrivateGatewayTestRuntimeWithClock(state, func() time.Time { return privateGatewayTestNow })
}

func newPrivateGatewayTestRuntimeWithClock(state service.PrivateGatewayStateRepository, now func() time.Time) *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntimeAndState(
		privateGatewayTestConfig(),
		func(_ context.Context, id int64) (*service.APIKey, error) {
			if id != 42 {
				return nil, service.ErrAPIKeyNotFound
			}
			return &service.APIKey{ID: id}, nil
		},
		now,
		state,
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

func TestANLPrivateGatewayIdempotencyKeyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("accepts Hub maximum and replays once", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		var calls atomic.Int32
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			c.JSON(http.StatusOK, gin.H{"id": "chatcmpl-max-key", "usage": gin.H{"total_tokens": 4}})
		})
		router.GET("/v1/sub2api/idempotency/:key", gateway.Status())

		key := strings.Repeat("k", 160)
		require.Equal(t, strings.Repeat("k", 16), normalizePrivateGatewayIdempotencyKey(strings.Repeat("k", 16)))
		body := []byte(`{"model":"gpt-5.6-terra"}`)
		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", key)
			signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x170+attempt), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code)
			if attempt == 1 {
				require.Equal(t, "true", response.Header().Get(privateGatewayIdempotencyReplayHeader))
			}
		}
		require.Equal(t, int32(1), calls.Load())

		statusRequest := httptest.NewRequest(http.MethodGet, "/v1/sub2api/idempotency/"+key, nil)
		signPrivateGatewayTestRequest(statusRequest, nil, "00000000000000000000000000000172", privateGatewayTestNow)
		statusResponse := httptest.NewRecorder()
		router.ServeHTTP(statusResponse, statusRequest)
		require.Equal(t, http.StatusOK, statusResponse.Code)
		var receipt map[string]any
		require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &receipt))
		require.Equal(t, "pending", receipt["billing_status"])
		require.Equal(t, "reported", receipt["usage_status"])
		require.NotContains(t, receipt, "charged_amount")
		require.NotContains(t, receipt, "currency")
	})

	t.Run("rejects keys outside Hub contract", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		var calls atomic.Int32
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			c.Status(http.StatusNoContent)
		})

		for index, key := range []string{strings.Repeat("k", 15), strings.Repeat("k", 161), strings.Repeat("k", 15) + "!"} {
			body := []byte(`{"model":"gpt-5.6-terra"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Idempotency-Key", key)
			signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x180+index), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			requirePrivateGatewayErrorCode(t, response, http.StatusBadRequest, "INVALID_REQUEST")
		}
		require.Zero(t, calls.Load())
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

func TestANLPrivateGatewayReplayCaptureTwoMiBBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("replays response below two MiB", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		var calls atomic.Int32
		body := bytes.Repeat([]byte("x"), (2<<20)-1)
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			c.Data(http.StatusOK, "application/octet-stream", body)
		})

		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-5.6-terra"}`)))
			req.Header.Set("Idempotency-Key", "idem-two-mib-replay")
			signPrivateGatewayTestRequest(req, []byte(`{"model":"gpt-5.6-terra"}`), fmt.Sprintf("%032x", 0x90+attempt), privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, body, response.Body.Bytes())
			if attempt == 1 {
				require.Equal(t, "true", response.Header().Get(privateGatewayIdempotencyReplayHeader))
			}
		}
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("marks response above two MiB unknown", func(t *testing.T) {
		gateway := newPrivateGatewayTestRuntime()
		body := bytes.Repeat([]byte("x"), (2<<20)+1)
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			c.Data(http.StatusOK, "application/octet-stream", body)
		})

		requestBody := []byte(`{"model":"gpt-5.6-terra"}`)
		firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestBody))
		firstReq.Header.Set("Idempotency-Key", "idem-two-mib-overflow")
		signPrivateGatewayTestRequest(firstReq, requestBody, "00000000000000000000000000000091", privateGatewayTestNow)
		firstResponse := httptest.NewRecorder()
		router.ServeHTTP(firstResponse, firstReq)
		require.Equal(t, http.StatusOK, firstResponse.Code)

		secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestBody))
		secondReq.Header.Set("Idempotency-Key", "idem-two-mib-overflow")
		signPrivateGatewayTestRequest(secondReq, requestBody, "00000000000000000000000000000092", privateGatewayTestNow)
		secondResponse := httptest.NewRecorder()
		router.ServeHTTP(secondResponse, secondReq)
		requirePrivateGatewayErrorCode(t, secondResponse, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
	})
}

func TestANLPrivateGatewaySharedStateSurvivesNewGatewayInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayMemoryState(func() time.Time { return privateGatewayTestNow })
	first := newPrivateGatewayTestRuntimeWithState(state)
	second := newPrivateGatewayTestRuntimeWithState(state)

	var calls atomic.Int32
	newRouter := func(gateway *ANLPrivateGateway) *gin.Engine {
		router := gin.New()
		router.Use(gateway.Authentication(), gateway.Idempotency())
		router.POST("/v1/chat/completions", func(c *gin.Context) {
			calls.Add(1)
			c.JSON(http.StatusCreated, gin.H{"id": "shared-replay", "usage": gin.H{"total_tokens": 9}})
		})
		return router
	}

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	firstRouter := newRouter(first)
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	firstReq.Header.Set("Idempotency-Key", "shared-instance-key")
	signPrivateGatewayTestRequest(firstReq, body, "00000000000000000000000000000101", privateGatewayTestNow)
	firstResponse := httptest.NewRecorder()
	firstRouter.ServeHTTP(firstResponse, firstReq)
	require.Equal(t, http.StatusCreated, firstResponse.Code)

	secondRouter := newRouter(second)
	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	secondReq.Header.Set("Idempotency-Key", "shared-instance-key")
	signPrivateGatewayTestRequest(secondReq, body, "00000000000000000000000000000102", privateGatewayTestNow)
	secondResponse := httptest.NewRecorder()
	secondRouter.ServeHTTP(secondResponse, secondReq)
	require.Equal(t, http.StatusCreated, secondResponse.Code)
	require.Equal(t, "true", secondResponse.Header().Get(privateGatewayIdempotencyReplayHeader))
	require.Equal(t, firstResponse.Body.Bytes(), secondResponse.Body.Bytes())
	require.Equal(t, int32(1), calls.Load())
}

func TestANLPrivateGatewayExpiredProcessingBecomesOutcomeUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayMemoryState(func() time.Time { return privateGatewayTestNow })
	gateway := newPrivateGatewayTestRuntimeWithState(state)
	body := []byte(`{"model":"gpt-5.6-terra"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	requestContext := privateGatewayRequestContext{
		serviceID:  "anl-hub-test",
		bodySHA256: func() string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }(),
	}
	lockedUntil := privateGatewayTestNow.Add(-time.Second)
	expiresAt := privateGatewayTestNow.Add(privateGatewayIdempotencyTTL)
	record := &service.IdempotencyRecord{
		Scope:              privateGatewayIdempotencyScope(requestContext.serviceID, req, "stale-idempotency-key"),
		IdempotencyKeyHash: service.HashIdempotencyKey("stale-idempotency-key"),
		RequestFingerprint: privateGatewayIdempotencyFingerprint(requestContext, req),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        &lockedUntil,
		ExpiresAt:          expiresAt,
	}
	owner, err := state.CreateProcessing(context.Background(), record)
	require.NoError(t, err)
	require.True(t, owner)

	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	var calls atomic.Int32
	router.POST("/v1/chat/completions", func(c *gin.Context) { calls.Add(1); c.Status(http.StatusOK) })
	req.Header.Set("Idempotency-Key", "stale-idempotency-key")
	signPrivateGatewayTestRequest(req, body, "00000000000000000000000000000103", privateGatewayTestNow)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	requirePrivateGatewayErrorCode(t, response, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
	require.Zero(t, calls.Load())

	loaded, err := state.GetByScopeAndKeyHash(context.Background(), record.Scope, record.IdempotencyKeyHash)
	require.NoError(t, err)
	require.Equal(t, service.PrivateGatewayIdempotencyStatusOutcomeUnknown, loaded.Status)
}

func TestANLPrivateGatewayClientDisconnectDoesNotDeleteProcessingRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayMemoryState(func() time.Time { return privateGatewayTestNow })
	gateway := newPrivateGatewayTestRuntimeWithState(state)
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	var calls atomic.Int32
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		calls.Add(1)
		ctx, cancel := context.WithCancel(c.Request.Context())
		cancel()
		c.Request = c.Request.WithContext(ctx)
		c.JSON(http.StatusOK, gin.H{"id": "unknown"})
	})

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "disconnect-idempotency-key")
		signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x104+attempt), privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if attempt == 0 {
			require.Equal(t, http.StatusOK, response.Code)
		} else {
			requirePrivateGatewayErrorCode(t, response, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
		}
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestANLPrivateGatewayRetryableFailureUsesSameKeyAfterBackoff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := privateGatewayTestNow
	clock := func() time.Time { return now }
	state := newPrivateGatewayMemoryState(clock)
	gateway := newPrivateGatewayTestRuntimeWithClock(state, clock)
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	var calls atomic.Int32
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		if calls.Add(1) == 1 {
			MarkPrivateGatewayRetryableFailure(c)
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "UPSTREAM_TEMPORARY", "Upstream is temporarily unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": "chatcmpl-retried", "usage": gin.H{"total_tokens": 7}})
	})

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	request := func(nonce string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "retryable-idempotency-key")
		signPrivateGatewayTestRequest(req, body, nonce, now)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	first := request("00000000000000000000000000000111")
	requirePrivateGatewayErrorCode(t, first, http.StatusServiceUnavailable, "UPSTREAM_TEMPORARY")
	second := request("00000000000000000000000000000112")
	requirePrivateGatewayErrorCode(t, second, http.StatusConflict, "IDEMPOTENCY_RETRY_BACKOFF")
	require.Equal(t, int32(1), calls.Load())

	now = now.Add(privateGatewayProcessingTTL + time.Second)
	third := request("00000000000000000000000000000113")
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, int32(2), calls.Load())
}

func TestANLPrivateGatewayFallbackUsageReceiptIsExactlyOnceForSameKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayMemoryState(func() time.Time { return privateGatewayTestNow })
	gateway := newPrivateGatewayTestRuntimeWithState(state)
	var preOutputFailures atomic.Int32
	var fallbackAttempts atomic.Int32
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		preOutputFailures.Add(1)
		fallbackAttempts.Add(1)
		c.Header("Content-Type", "text/event-stream")
		_, err := c.Writer.Write([]byte("data: {\"id\":\"fallback-first-output\",\"usage\":{\"total_tokens\":3}}\n\ndata: [DONE]\n\n"))
		require.NoError(t, err)
	})
	router.GET("/v1/sub2api/idempotency/:key", gateway.Status())

	body := []byte(`{"model":"gpt-5.6-terra","stream":true}`)
	request := func(nonce string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "first-output-fallback-key")
		signPrivateGatewayTestRequest(req, body, nonce, privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	first := request("00000000000000000000000000000141")
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), "fallback-first-output")
	require.Equal(t, int32(1), preOutputFailures.Load())
	require.Equal(t, int32(1), fallbackAttempts.Load())

	replay := request("00000000000000000000000000000142")
	require.Equal(t, http.StatusOK, replay.Code)
	require.Equal(t, "true", replay.Header().Get(privateGatewayIdempotencyReplayHeader))
	require.Equal(t, first.Body.Bytes(), replay.Body.Bytes())
	require.Equal(t, int32(1), preOutputFailures.Load())
	require.Equal(t, int32(1), fallbackAttempts.Load())

	for attempt := 0; attempt < 2; attempt++ {
		statusRequest := httptest.NewRequest(http.MethodGet, "/v1/sub2api/idempotency/first-output-fallback-key", nil)
		signPrivateGatewayTestRequest(statusRequest, nil, fmt.Sprintf("%032x", 0x143+attempt), privateGatewayTestNow)
		statusResponse := httptest.NewRecorder()
		router.ServeHTTP(statusResponse, statusRequest)
		require.Equal(t, http.StatusOK, statusResponse.Code)

		var receipt struct {
			Status      string         `json:"status"`
			Billing     string         `json:"billing_status"`
			UsageStatus string         `json:"usage_status"`
			Usage       map[string]any `json:"usage"`
		}
		require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &receipt))
		require.Equal(t, service.IdempotencyStatusSucceeded, receipt.Status)
		require.Equal(t, "pending", receipt.Billing)
		require.Equal(t, "reported", receipt.UsageStatus)
		require.Equal(t, float64(3), receipt.Usage["total_tokens"])
	}
	require.Equal(t, int32(1), preOutputFailures.Load())
	require.Equal(t, int32(1), fallbackAttempts.Load())
}

func TestANLPrivateGatewayStopsFallbackAfterFirstSemanticStreamOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayMemoryState(func() time.Time { return privateGatewayTestNow })
	gateway := newPrivateGatewayTestRuntimeWithState(state)
	var primaryAttempts atomic.Int32
	var fallbackAttempts atomic.Int32
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		primaryAttempts.Add(1)
		c.Header("Content-Type", "text/event-stream")
		_, err := c.Writer.Write([]byte("data: {\"id\":\"primary-first-output\"}\n\n"))
		require.NoError(t, err)
	})

	body := []byte(`{"model":"gpt-5.6-terra","stream":true}`)
	request := func(nonce string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "first-output-no-fallback-key")
		signPrivateGatewayTestRequest(req, body, nonce, privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	first := request("00000000000000000000000000000151")
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), "primary-first-output")
	require.Equal(t, int32(1), primaryAttempts.Load())
	require.Zero(t, fallbackAttempts.Load())

	second := request("00000000000000000000000000000152")
	requirePrivateGatewayErrorCode(t, second, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
	require.Equal(t, int32(1), primaryAttempts.Load())
	require.Zero(t, fallbackAttempts.Load())
}

func TestANLPrivateGatewayPanicBecomesOutcomeUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayMemoryState(func() time.Time { return privateGatewayTestNow })
	gateway := newPrivateGatewayTestRuntimeWithState(state)
	router := gin.New()
	router.Use(gin.Recovery(), gateway.Authentication(), gateway.Idempotency())
	var calls atomic.Int32
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		calls.Add(1)
		panic("synthetic handler panic")
	})

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "panic-idempotency-key")
		signPrivateGatewayTestRequest(req, body, fmt.Sprintf("%032x", 0x120+attempt), privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if attempt == 0 {
			require.Equal(t, http.StatusInternalServerError, response.Code)
		} else {
			requirePrivateGatewayErrorCode(t, response, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
		}
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestPrivateGatewayStoredResponseIncludesUsageAndStreamTerminal(t *testing.T) {
	response := privateGatewayHTTPResponse{
		status:    http.StatusOK,
		header:    http.Header{"Content-Type": []string{"text/event-stream"}},
		body:      []byte("data: {\"id\":\"chunk-1\",\"usage\":null}\n\ndata: {\"id\":\"chunk-2\",\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"),
		errorCode: "",
	}
	require.True(t, privateGatewayStreamHasUsageAndTerminal(response.body))
	require.False(t, privateGatewayStreamHasUsageAndTerminal([]byte("data: {\"usage\":{\"total_tokens\":5}}\n\n")))
	require.False(t, privateGatewayStreamHasUsageAndTerminal([]byte("data: {\"id\":\"chunk-1\"}\n\ndata: [DONE]\n\n")))
	response.usage = privateGatewayExtractUsage(response.body)
	stored, err := encodePrivateGatewayStoredResponse(response)
	require.NoError(t, err)
	decoded, err := decodePrivateGatewayStoredResponse(&stored)
	require.NoError(t, err)
	require.JSONEq(t, `{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}`, string(decoded.usage))
	require.Equal(t, "text/event-stream", decoded.header.Get("Content-Type"))
	require.Equal(t, response.body, decoded.body)
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

func TestANLPrivateGatewayStatusReturnsStoredUsageForOriginalOutputRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newPrivateGatewayTestRuntime()
	router := gin.New()
	router.Use(gateway.Authentication())
	router.POST("/v1/chat/completions", gateway.Idempotency(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": "chatcmpl-status", "usage": gin.H{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3}})
	})
	router.GET("/v1/sub2api/idempotency/:key", gateway.Status())

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "status-usage-key")
	signPrivateGatewayTestRequest(request, body, "00000000000000000000000000000131", privateGatewayTestNow)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	statusRequest := httptest.NewRequest(http.MethodGet, "/v1/sub2api/idempotency/status-usage-key", nil)
	signPrivateGatewayTestRequest(statusRequest, nil, "00000000000000000000000000000132", privateGatewayTestNow)
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, statusRequest)
	require.Equal(t, http.StatusOK, statusResponse.Code)
	var payload struct {
		Status      string         `json:"status"`
		Billing     string         `json:"billing_status"`
		UsageStatus string         `json:"usage_status"`
		Usage       map[string]any `json:"usage"`
		TotalTokens float64        `json:"-"`
	}
	require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &payload))
	require.Equal(t, service.IdempotencyStatusSucceeded, payload.Status)
	require.Equal(t, "pending", payload.Billing)
	require.Equal(t, "reported", payload.UsageStatus)
	require.Equal(t, float64(3), payload.Usage["total_tokens"])
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
