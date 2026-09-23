package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var privateGatewayTestNow = time.Unix(1_785_475_641, 0).UTC()

type privateGatewayTestNonceRepo struct {
	mu       sync.Mutex
	claims   map[string]time.Time
	err      error
	claimCnt atomic.Int32
}

func newPrivateGatewayTestNonceRepo() *privateGatewayTestNonceRepo {
	return &privateGatewayTestNonceRepo{claims: make(map[string]time.Time)}
}

func (r *privateGatewayTestNonceRepo) ClaimPrivateGatewayNonce(_ context.Context, scope, nonceHash string, expiresAt time.Time) (bool, error) {
	r.claimCnt.Add(1)
	if r.err != nil {
		return false, r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := privateGatewayTestNow
	key := scope + "\n" + nonceHash
	if existing, ok := r.claims[key]; ok && existing.After(now) {
		return false, nil
	}
	r.claims[key] = expiresAt
	return true, nil
}

func privateGatewayTestConfig() config.PrivateGatewayConfig {
	return config.PrivateGatewayConfig{
		Enabled:    true,
		ServiceID:  "anl-hub-test",
		SigningKey: "test-signing-key-for-anlapi-service-123456",
		APIKeyID:   42,
	}
}

func newPrivateGatewayTestRuntime(repo service.PrivateGatewayNonceRepository) *ANLPrivateGateway {
	return newPrivateGatewayTestRuntimeWithLoader(repo, func(_ context.Context, id int64) (*service.APIKey, error) {
		if id != 42 {
			return nil, service.ErrAPIKeyNotFound
		}
		return &service.APIKey{ID: id}, nil
	})
}

func newPrivateGatewayTestRuntimeWithLoader(repo service.PrivateGatewayNonceRepository, loader PrivateGatewayAPIKeyLoader) *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntime(
		privateGatewayTestConfig(),
		loader,
		repo,
		func() time.Time { return privateGatewayTestNow },
	)
}

func signPrivateGatewayTestRequest(req *http.Request, body []byte, nonce string, timestamp time.Time) {
	digest := sha256.Sum256(body)
	timestampRaw := strconvFormatPrivateGatewayTestTimestamp(timestamp)
	req.Header.Set(privateGatewayServiceIDHeader, privateGatewayTestConfig().ServiceID)
	req.Header.Set(privateGatewayTimestampHeader, timestampRaw)
	req.Header.Set(privateGatewayNonceHeader, nonce)
	req.Header.Set(privateGatewayBodySHA256Header, hex.EncodeToString(digest[:]))
	canonical := strings.Join([]string{
		privateGatewayProtocol,
		privateGatewayTestConfig().ServiceID,
		timestampRaw,
		nonce,
		strings.ToUpper(req.Method),
		privateGatewayEscapedPathAndQuery(req),
		hex.EncodeToString(digest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(privateGatewayTestConfig().SigningKey))
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set(privateGatewaySignatureHeader, hex.EncodeToString(mac.Sum(nil)))
}

func strconvFormatPrivateGatewayTestTimestamp(value time.Time) string {
	return strconv.FormatInt(value.Unix(), 10)
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
			repo := newPrivateGatewayTestNonceRepo()
			gateway := newPrivateGatewayTestRuntime(repo)
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
				require.Empty(t, c.GetHeader("Authorization"))
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			signPrivateGatewayTestRequest(req, []byte(route.body), fmt.Sprintf("%032x", i+1), privateGatewayTestNow)
			req.Header.Set("Authorization", "Bearer should-be-ignored")
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
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
		require.Zero(t, calls.Load())
	})

	t.Run("body tamper", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"tampered":true}`))
		signPrivateGatewayTestRequest(req, []byte(`{"original":true}`), "00000000000000000000000000000011", privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
		require.Zero(t, calls.Load())
	})

	t.Run("signature tamper", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
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
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000012", privateGatewayTestNow.Add(-61*time.Second))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_EXPIRED")
		require.Zero(t, calls.Load())
	})

	t.Run("invalid nonce format", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		signPrivateGatewayTestRequest(req, []byte(`{}`), "not-a-hex-nonce", privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
		require.Zero(t, calls.Load())
	})

	t.Run("future timestamp", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000015", privateGatewayTestNow.Add(61*time.Second))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_EXPIRED")
		require.Zero(t, calls.Load())
	})

	t.Run("nonce replay", func(t *testing.T) {
		var calls atomic.Int32
		router := newRouter(newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()), &calls)
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
	repo := newPrivateGatewayTestNonceRepo()
	gateway := newPrivateGatewayTestRuntime(repo)
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

func TestANLPrivateGatewayAllowsExistingPublicAPIKeyBranchAndDisabledMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, gateway := range map[string]*ANLPrivateGateway{
		"enabled":  newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo()),
		"disabled": newANLPrivateGatewayWithRuntime(config.PrivateGatewayConfig{}, nil, newPrivateGatewayTestNonceRepo(), func() time.Time { return privateGatewayTestNow }),
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(gateway.Authentication())
			router.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set("Authorization", "Bearer public-test-key")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusNoContent, response.Code)
		})
	}
}

func TestANLPrivateGatewayFailsClosedWhenNonceStoreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newPrivateGatewayTestNonceRepo()
	repo.err = errors.New("database unavailable")
	gateway := newPrivateGatewayTestRuntime(repo)
	router := gin.New()
	router.Use(gateway.Authentication())
	router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000031", privateGatewayTestNow)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	requirePrivateGatewayErrorCode(t, response, http.StatusServiceUnavailable, "ANL_SERVICE_AUTH_UNAVAILABLE")
	require.Equal(t, int32(1), repo.claimCnt.Load())
}

func TestANLPrivateGatewayReturnsUnavailableWhenAPIKeyStoreFailsWithoutClaimingNonce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newPrivateGatewayTestNonceRepo()
	gateway := newPrivateGatewayTestRuntimeWithLoader(repo, func(context.Context, int64) (*service.APIKey, error) {
		return nil, errors.New("database unavailable")
	})
	router := gin.New()
	router.Use(gateway.Authentication())
	router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000032", privateGatewayTestNow)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	requirePrivateGatewayErrorCode(t, response, http.StatusServiceUnavailable, "ANL_SERVICE_AUTH_UNAVAILABLE")
	require.Zero(t, repo.claimCnt.Load(), "a backend API key failure must not consume the nonce")
}

func TestANLPrivateGatewayRejectsMissingAPIKeyBeforeClaimingNonce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		loader PrivateGatewayAPIKeyLoader
	}{
		{
			name: "not found",
			loader: func(context.Context, int64) (*service.APIKey, error) {
				return nil, service.ErrAPIKeyNotFound
			},
		},
		{
			name: "nil key",
			loader: func(context.Context, int64) (*service.APIKey, error) {
				return nil, nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newPrivateGatewayTestNonceRepo()
			gateway := newPrivateGatewayTestRuntimeWithLoader(repo, tc.loader)
			router := gin.New()
			router.Use(gateway.Authentication())
			router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
			signPrivateGatewayTestRequest(req, []byte(`{}`), "00000000000000000000000000000033", privateGatewayTestNow)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
			require.Zero(t, repo.claimCnt.Load(), "a missing configured API key must not consume the nonce")
		})
	}
}

func TestPrivateGatewayNonceScopeIsBoundedAndServiceSpecific(t *testing.T) {
	first := privateGatewayNonceScope(strings.Repeat("x", 128))
	second := privateGatewayNonceScope(strings.Repeat("y", 128))
	require.Len(t, first, len("private_gateway_nonce:")+64)
	require.Len(t, second, len("private_gateway_nonce:")+64)
	require.NotEqual(t, first, second)
}

func TestANLPrivateGatewayCanonicalQueryTamperIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newPrivateGatewayTestRuntime(newPrivateGatewayTestNonceRepo())
	router := gin.New()
	router.Use(gateway.Authentication())
	router.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/v1/models?a=1&b=2", nil)
	signPrivateGatewayTestRequest(req, nil, "00000000000000000000000000000041", privateGatewayTestNow)
	req.URL.RawQuery = "a=1&b=3"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	requirePrivateGatewayErrorCode(t, response, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID")
}
