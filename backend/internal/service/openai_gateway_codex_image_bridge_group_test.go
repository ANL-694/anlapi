package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldEnableCodexImageGenerationBridgeRequiresGroupPermission(t *testing.T) {
	tests := []struct {
		name     string
		group    *Group
		extra    map[string]any
		expected bool
	}{
		{name: "ungrouped keeps legacy behavior", expected: true},
		{name: "disabled group", group: &Group{AllowImageGeneration: false}},
		{
			name:  "account cannot enable a disabled group",
			group: &Group{AllowImageGeneration: false},
			extra: map[string]any{featureKeyCodexImageGenerationBridge: true},
		},
		{name: "enabled group", group: &Group{AllowImageGeneration: true}, expected: true},
		{
			name:  "account can disable an enabled group",
			group: &Group{AllowImageGeneration: true},
			extra: map[string]any{featureKeyCodexImageGenerationBridge: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Platform: PlatformOpenAI, Extra: tt.extra}
			require.Equal(t, tt.expected, shouldEnableCodexImageGenerationBridge(tt.group, account))
		})
	}
}

func TestOpenAIGatewayServiceCodexImageBridgeRespectsGroupPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, allowImages := range []bool{false, true} {
		name := "disabled group"
		if allowImages {
			name = "enabled group"
		}
		t.Run(name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_test","object":"response","model":"gpt-5.6-sol","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
				)),
			}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(allowImages, codexCLIUserAgent)
			c.Request.Header.Set("Originator", "codex_cli_rs")
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			account := newOpenAIImageGenerationControlTestAccount()
			body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"write code"}`)

			result, err := svc.Forward(context.Background(), c, account, body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			hasImageTool := gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation").type`).String() == "image_generation"
			require.Equal(t, allowImages, hasImageTool)
		})
	}
}
