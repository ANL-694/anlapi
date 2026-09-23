package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

	t.Run("mapped pro uses native responses despite stale probe", func(t *testing.T) {
		account := newAccount(map[string]any{openai_compat.ExtraKeyResponsesSupported: false})
		require.True(t, shouldUseResponsesAPIForAccountModel(account, "public-pro", ""))
	})

	t.Run("manual force chat mode wins after mapping", func(t *testing.T) {
		account := newAccount(map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		})
		require.False(t, shouldUseResponsesAPIForAccountModel(account, "public-pro", ""))
	})
}

func TestOpenAIGatewayService_DeepSeekV4RoutesByModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newAccount := func(model string) *Account {
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
			Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
		}
	}

	for _, model := range []string{openai_compat.DeepSeekV4FlashModel, openai_compat.DeepSeekV4ProModel} {
		t.Run(model, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"id":"resp_deepseek",
					"object":"response",
					"status":"completed",
					"model":"deepseek-v4-pro",
					"output":[],
					"usage":{"input_tokens":1000,"output_tokens":50,"total_tokens":1050,"input_tokens_details":{"cached_tokens":900}}
				}`)),
			}}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := `{"model":"` + model + `","input":"hello","stream":false}`
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

			result, err := svc.Forward(context.Background(), c, newAccount(model), []byte(body))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "https://api.deepseek.com/v1/responses", upstream.lastReq.URL.String())
			require.Equal(t, 1000, result.Usage.InputTokens)
			require.Equal(t, 900, result.Usage.CacheReadInputTokens)
		})
	}
}

func TestOpenAIGatewayService_DeepSeekV4ChatIngressRoutesByFinalModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newAccount := func(extra map[string]any) *Account {
		return &Account{
			ID:          41002,
			Name:        "deepseek-official-chat-ingress",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://api.deepseek.com/v1",
				"model_mapping": map[string]any{
					"public-pro": openai_compat.DeepSeekV4ProModel,
				},
			},
			Extra: extra,
		}
	}

	run := func(t *testing.T, account *Account) string {
		t.Helper()

		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop before response parsing"}}`)),
		}}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"public-pro","messages":[{"role":"user","content":"hello"}],"stream":false}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))

		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		require.Error(t, err)
		require.Nil(t, result)
		require.NotNil(t, upstream.lastReq)
		return upstream.lastReq.URL.String()
	}

	t.Run("mapped V4 model overrides stale negative probe", func(t *testing.T) {
		account := newAccount(map[string]any{openai_compat.ExtraKeyResponsesSupported: false})
		require.Equal(t, "https://api.deepseek.com/v1/responses", run(t, account))
	})

	t.Run("explicit force chat still wins", func(t *testing.T) {
		account := newAccount(map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		})
		require.Equal(t, "https://api.deepseek.com/v1/chat/completions", run(t, account))
	})
}

func TestResolveOpenAIForwardModels_DeepSeekUsesActualOutboundModel(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://api.deepseek.com/v1",
			"model_mapping": map[string]any{
				"public-model": openai_compat.DeepSeekV4ProModel,
			},
			"compact_model_mapping": map[string]any{
				"public-model": openai_compat.DeepSeekV4FlashModel,
			},
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	normal := resolveOpenAIForwardModels(account, "public-model", "", false, false)
	require.Equal(t, openai_compat.DeepSeekV4ProModel, normal.BillingModel)
	require.Equal(t, openai_compat.DeepSeekV4ProModel, normal.UpstreamModel)
	require.True(t, shouldUseResponsesAPIForResolvedAccountModel(account, normal.UpstreamModel))

	passthrough := resolveOpenAIForwardModels(account, "public-model", "", true, false)
	require.Equal(t, "public-model", passthrough.BillingModel)
	require.Equal(t, "public-model", passthrough.UpstreamModel)
	require.False(t, shouldUseResponsesAPIForResolvedAccountModel(account, passthrough.UpstreamModel))

	compactPassthrough := resolveOpenAIForwardModels(account, "public-model", "", true, true)
	require.Equal(t, "public-model", compactPassthrough.BillingModel)
	require.Equal(t, openai_compat.DeepSeekV4FlashModel, compactPassthrough.UpstreamModel)
	require.True(t, compactPassthrough.CompactMapped)
	require.True(t, shouldUseResponsesAPIForResolvedAccountModel(account, compactPassthrough.UpstreamModel))

	oauthAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	legacyCompactPassthrough := resolveOpenAIForwardModels(oauthAccount, "gpt-5.1-codex", "", true, true)
	require.Equal(t, "gpt-5.1-codex", legacyCompactPassthrough.UpstreamModel)
	require.False(t, legacyCompactPassthrough.CompactMapped)
}

func TestOpenAIGatewayService_DeepSeekForwardKeepsProtocolAndBodyModelAligned(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("passthrough ignores ordinary model mapping", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl_passthrough","object":"chat.completion","model":"public-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			)),
		}}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		account := &Account{
			ID: 41003, Name: "deepseek-passthrough", Platform: PlatformOpenAI,
			Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{
				"api_key": "sk-test", "base_url": "https://api.deepseek.com/v1",
				"model_mapping": map[string]any{"public-model": openai_compat.DeepSeekV4ProModel},
			},
			Extra: map[string]any{
				"openai_passthrough":                     true,
				openai_compat.ExtraKeyResponsesSupported: false,
			},
		}
		body := []byte(`{"model":"public-model","input":"hello","stream":false}`)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
		require.Equal(t, "public-model", gjson.GetBytes(upstream.lastBody, "model").String())
	})

	t.Run("passthrough compact mapping selects responses and rewrites body", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"resp_compact","object":"response","status":"completed","model":"deepseek-v4-flash","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`,
			)),
		}}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		account := &Account{
			ID: 41004, Name: "deepseek-compact-passthrough", Platform: PlatformOpenAI,
			Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{
				"api_key": "sk-test", "base_url": "https://api.deepseek.com/v1",
				"model_mapping":         map[string]any{"public-model": "stale-normal-mapping"},
				"compact_model_mapping": map[string]any{"public-model": openai_compat.DeepSeekV4FlashModel},
			},
			Extra: map[string]any{
				"openai_passthrough":                     true,
				openai_compat.ExtraKeyResponsesSupported: false,
			},
		}
		body := []byte(`{"model":"public-model","input":"hello"}`)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(string(body)))
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "https://api.deepseek.com/v1/responses/compact", upstream.lastReq.URL.String())
		require.Equal(t, openai_compat.DeepSeekV4FlashModel, gjson.GetBytes(upstream.lastBody, "model").String())
	})

	t.Run("chat fallback does not apply a second mapping", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl_once","object":"chat.completion","model":"intermediate-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			)),
		}}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		account := &Account{
			ID: 41005, Name: "deepseek-single-map", Platform: PlatformOpenAI,
			Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{
				"api_key": "sk-test", "base_url": "https://api.deepseek.com/v1",
				"model_mapping": map[string]any{
					"public-model":       "intermediate-model",
					"intermediate-model": openai_compat.DeepSeekV4ProModel,
				},
			},
			Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
		}
		body := []byte(`{"model":"public-model","input":"hello","stream":false}`)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
		require.Equal(t, "intermediate-model", gjson.GetBytes(upstream.lastBody, "model").String())
	})
}

func TestAccountTestService_DeepSeekV4UsesSingleMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n")),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &AccountTestService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID: 41006, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test", "base_url": "https://api.deepseek.com/v1",
			"model_mapping": map[string]any{
				"public-model":                   openai_compat.DeepSeekV4ProModel,
				openai_compat.DeepSeekV4ProModel: "legacy-second-hop",
			},
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/41006/test", nil)

	err := svc.testOpenAIAccountConnection(c, account, "public-model", "hello", "")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, openai_compat.DeepSeekV4ProModel, gjson.GetBytes(upstream.lastBody, "model").String())
}

func TestDeepSeekChatUsageParsingPreservesCacheHitAndMiss(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantInput int
		wantCache int
	}{
		{
			name:      "aggregate prompt tokens stay authoritative",
			body:      `{"usage":{"prompt_tokens":1000,"completion_tokens":50,"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100}}`,
			wantInput: 1000, wantCache: 900,
		},
		{
			name:      "hit and miss reconstruct missing aggregate",
			body:      `{"usage":{"completion_tokens":50,"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100}}`,
			wantInput: 1000, wantCache: 900,
		},
		{
			name:      "explicit zero hit overrides legacy alias",
			body:      `{"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_cache_hit_tokens":0,"cached_tokens":80}}`,
			wantInput: 100, wantCache: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage, ok := extractOpenAIUsageFromJSONBytes([]byte(tt.body))
			require.True(t, ok)
			require.Equal(t, tt.wantInput, usage.InputTokens)
			require.Equal(t, tt.wantCache, usage.CacheReadInputTokens)
		})
	}
}

func TestForwardAsRawChatCompletions_DeepSeekCacheUsageStreamingAndNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "non_streaming", true: "streaming"}[stream], func(t *testing.T) {
			usageJSON := `{"prompt_tokens":1000,"completion_tokens":50,"total_tokens":1050,"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100}`
			responseBody := `{"id":"chatcmpl_cache","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":` + usageJSON + `}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				responseBody = strings.Join([]string{
					`data: {"id":"chatcmpl_cache","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
					"",
					`data: {"id":"chatcmpl_cache","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[],"usage":` + usageJSON + `}`,
					"",
					"data: [DONE]",
					"",
				}, "\n")
			}

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := &Account{
				ID: 41007, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.deepseek.com/v1"},
			}
			body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":` + map[bool]string{false: "false", true: "true"}[stream] + `}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))

			result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1000, result.Usage.InputTokens)
			require.Equal(t, 900, result.Usage.CacheReadInputTokens)
			require.Equal(t, 50, result.Usage.OutputTokens)
		})
	}
}
