package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anlapi/internal/config"
	"anlapi/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareOpenAIWSHTTPBridgeBodyPreservesContinuationAndNumbers(t *testing.T) {
	body, err := prepareOpenAIWSHTTPBridgeBody([]byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":false,"previous_response_id":"resp_prev","max_output_tokens":9007199254740993,"input":"hi"}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "type").Exists())
	require.False(t, gjson.GetBytes(body, "generate").Exists())
	require.Equal(t, "resp_prev", gjson.GetBytes(body, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(body, "max_output_tokens").Raw)
	require.Equal(t, "gpt-5", gjson.GetBytes(body, "model").String())
	require.True(t, gjson.GetBytes(body, "stream").Bool())
}

func TestShouldBridgeOpenAIWSHTTP(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 100

	require.False(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformOpenAI}, 99, ""))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformOpenAI}, 100, ""))
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformOpenAI}, 1000, "resp_existing"))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformGrok}, 1, "resp_existing"))
}

func TestProxyOpenAIWSHTTPBridgeTurnPromotesCodexAdditionalToolsForMixedCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_grok_codex_lite","model":"grok-4.5"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_grok_codex_lite","model":"grok-4.5","usage":{"input_tokens":4,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          73,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"base_url":          xai.DefaultCLIBaseURL,
			"subscription_tier": "free",
		},
	}
	payload := []byte(`{
		"type":"response.create","generate":true,"model":"grok","stream":true,
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"function","name":"lookup","parameters":{"type":"object"}},
				{"type":"function","name":"web_search","parameters":{"type":"object"}},
				{"type":"custom","name":"apply_patch"},
				{"type":"namespace","name":"collaboration"}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
		]
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set(grokClientToolCacheOptInHeader, "prefer-cache")
	var events [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok", "", "", "", "isolated-ws-cache-id", 1,
		func(message []byte) error {
			events = append(events, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, events, 2)
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools")`).Exists())
	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="custom")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="namespace")`).Exists())
	require.Equal(t, "isolated-ws-cache-id", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "isolated-ws-cache-id", upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Empty(t, upstream.lastReq.Header.Get(grokClientToolCacheOptInHeader))
}
