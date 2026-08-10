package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anlapi/internal/config"
	"anlapi/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldUseResponsesAPIForAccountModel_DeepSeekV4(t *testing.T) {
	newAccount := func(extra map[string]any) *Account {
		return &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://api.deepseek.com/v1",
				"model_mapping": map[string]any{
					"public-flash": "deepseek-v4-flash",
					"public-pro":   "deepseek-v4-pro",
				},
			},
			Extra: extra,
		}
	}

	t.Run("mapped flash uses native responses despite stale probe", func(t *testing.T) {
		account := newAccount(map[string]any{openai_compat.ExtraKeyResponsesSupported: false})
		require.True(t, shouldUseResponsesAPIForAccountModel(account, "public-flash", ""))
	})

	t.Run("mapped pro uses chat despite positive account probe", func(t *testing.T) {
		account := newAccount(map[string]any{openai_compat.ExtraKeyResponsesSupported: true})
		require.False(t, shouldUseResponsesAPIForAccountModel(account, "public-pro", ""))
	})

	t.Run("manual force mode wins after mapping", func(t *testing.T) {
		account := newAccount(map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
		})
		require.True(t, shouldUseResponsesAPIForAccountModel(account, "public-pro", ""))
	})
}

func TestOpenAIGatewayService_DeepSeekV4RoutesByModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newAccount := func(extra map[string]any) *Account {
		return &Account{
			ID:          41001,
			Name:        "deepseek-official",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://api.deepseek.com/v1",
			},
			Extra: extra,
		}
	}

	t.Run("flash uses native responses", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"resp_deepseek_flash",
				"object":"response",
				"status":"completed",
				"model":"deepseek-v4-flash",
				"output":[],
				"usage":{"input_tokens":1000,"output_tokens":50,"total_tokens":1050,"input_tokens_details":{"cached_tokens":900}}
			}`)),
		}}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(
			context.Background(),
			c,
			newAccount(map[string]any{openai_compat.ExtraKeyResponsesSupported: false}),
			[]byte(`{"model":"deepseek-v4-flash","input":"hello","stream":false}`),
		)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, upstream.lastReq)
		require.Equal(t, "https://api.deepseek.com/v1/responses", upstream.lastReq.URL.String())
		require.Equal(t, 1000, result.Usage.InputTokens)
		require.Equal(t, 900, result.Usage.CacheReadInputTokens)
	})

	t.Run("pro falls back to chat completions", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"chatcmpl_deepseek_pro",
				"object":"chat.completion",
				"model":"deepseek-v4-pro",
				"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":1000,"completion_tokens":50,"total_tokens":1050,"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100}
			}`)),
		}}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(
			context.Background(),
			c,
			newAccount(map[string]any{openai_compat.ExtraKeyResponsesSupported: true}),
			[]byte(`{"model":"deepseek-v4-pro","input":"hello","stream":false}`),
		)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, upstream.lastReq)
		require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
		require.Equal(t, 1000, result.Usage.InputTokens)
		require.Equal(t, 900, result.Usage.CacheReadInputTokens)
	})
}

func TestOpenAIGatewayService_DeepSeekSessionAffinityKeepsAppendOnlyPrefixStable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	svc := &OpenAIGatewayService{}

	first := []byte(`{
		"model":"deepseek-v4-flash",
		"messages":[
			{"role":"system","content":"You are helpful."},
			{"role":"user","content":"Hello"}
		],
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]
	}`)
	appended := []byte(`{
		"model":"deepseek-v4-flash",
		"messages":[
			{"role":"system","content":"You are helpful."},
			{"role":"user","content":"Hello"},
			{"role":"assistant","content":"Hi"},
			{"role":"user","content":"Continue"}
		],
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]
	}`)

	firstHash := svc.GenerateSessionHash(c, first)
	appendedHash := svc.GenerateSessionHash(c, appended)
	require.NotEmpty(t, firstHash)
	require.Equal(t, firstHash, appendedHash)
}
