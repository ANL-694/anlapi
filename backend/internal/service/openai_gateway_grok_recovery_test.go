package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardGrokResponsesRetriesInvalidEncryptedContentOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{
		"model":"grok",
		"input":[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"keep this summary"}],"encrypted_content":"encrypted-reasoning"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
		],
		"metadata":{"large_id":9007199254740993},
		"stream":false
	}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 4535})

	account := &Account{
		ID:          4535,
		Name:        "grok-api-key",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":  "same-token",
			"base_url": "https://api.x.ai/v1",
		},
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Xai-Request-Id": []string{"recoverable-first"},
			},
			Body: io.NopCloser(strings.NewReader(`{"code":"invalid-argument","error":"Could not decrypt the provided encrypted_content. Ensure the value is unmodified."}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Xai-Request-Id": []string{"recovered-second"},
			},
			Body: io.NopCloser(strings.NewReader(`{"id":"resp_recovered","object":"response","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}`)),
		},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "recovered-second", result.RequestID)
	require.Len(t, upstream.requests, 2)
	require.Len(t, upstream.bodies, 2)

	require.Equal(t, "reasoning", gjson.GetBytes(upstream.bodies[0], "input.0.type").String())
	require.Equal(t, "encrypted-reasoning", gjson.GetBytes(upstream.bodies[0], "input.0.encrypted_content").String())
	require.Equal(t, "reasoning", gjson.GetBytes(upstream.bodies[1], "input.0.type").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.encrypted_content").Exists())
	require.Equal(t, "keep this summary", gjson.GetBytes(upstream.bodies[1], "input.0.summary.0.text").String())
	require.Equal(t, "message", gjson.GetBytes(upstream.bodies[1], "input.1.type").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.bodies[0], "metadata.large_id").Raw)
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.bodies[1], "metadata.large_id").Raw)

	firstIdentity := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
	secondIdentity := gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String()
	require.NotEmpty(t, firstIdentity)
	require.Equal(t, firstIdentity, secondIdentity)
	for _, req := range upstream.requests {
		require.Equal(t, "Bearer same-token", req.Header.Get("Authorization"))
		require.Equal(t, firstIdentity, req.Header.Get(grokConversationIDHeader))
	}
	require.Equal(t, StatusActive, account.Status)
	_, hasUpstreamErrors := c.Get(OpsUpstreamErrorsKey)
	require.False(t, hasUpstreamErrors)
	_, hasTerminalStatus := c.Get(OpsUpstreamStatusCodeKey)
	require.False(t, hasTerminalStatus)
}

func TestForwardGrokResponsesInvalidEncryptedContentRecoveryDoesNotOvermatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	matchingError := `{"code":"invalid-argument","error":"Could not decrypt the provided encrypted_content."}`
	tests := []struct {
		name         string
		requestBody  string
		responseBody string
	}{
		{
			name:         "different top-level code",
			requestBody:  `{"model":"grok","input":[{"type":"reasoning","encrypted_content":"cipher"}],"stream":false}`,
			responseBody: `{"code":"bad-request","error":"Could not decrypt the provided encrypted_content."}`,
		},
		{
			name:         "message does not mention decryption",
			requestBody:  `{"model":"grok","input":[{"type":"reasoning","encrypted_content":"cipher"}],"stream":false}`,
			responseBody: `{"code":"invalid-argument","error":"The provided encrypted_content is invalid."}`,
		},
		{
			name:         "nested OpenAI error shape",
			requestBody:  `{"model":"grok","input":[{"type":"reasoning","encrypted_content":"cipher"}],"stream":false}`,
			responseBody: `{"code":"invalid-argument","error":{"message":"Could not decrypt the provided encrypted_content."}}`,
		},
		{
			name:         "request has no encrypted reasoning",
			requestBody:  `{"model":"grok","input":[{"type":"message","role":"user","content":"hi"}],"stream":false}`,
			responseBody: matchingError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := []byte(tt.requestBody)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

			account := &Account{
				ID:          4536,
				Name:        "grok-api-key",
				Platform:    PlatformGrok,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Concurrency: 1,
				Credentials: map[string]any{"api_key": "token", "base_url": "https://api.x.ai/v1"},
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tt.responseBody)),
			}}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}

			result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())
			require.Nil(t, result)
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
			require.Len(t, upstream.bodies, 1)
		})
	}
}

func TestForwardGrokResponsesInvalidEncryptedContentRetryFailureIsTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"grok","input":[{"type":"reasoning","encrypted_content":"cipher"},{"type":"message","role":"user","content":"hi"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	account := &Account{
		ID:          4537,
		Name:        "grok-api-key",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "same-token", "base_url": "https://api.x.ai/v1"},
	}
	newInvalidEncryptedResponse := func(requestID string) *http.Response {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Xai-Request-Id": []string{requestID},
			},
			Body: io.NopCloser(strings.NewReader(`{"code":"invalid-argument","error":"Could not decrypt the provided encrypted_content."}`)),
		}
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newInvalidEncryptedResponse("recoverable-first"),
		newInvalidEncryptedResponse("terminal-second"),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())
	require.Nil(t, result)
	require.Error(t, err)
	require.Len(t, upstream.requests, 2)
	require.Len(t, upstream.bodies, 2)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "input.0.encrypted_content").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], `input.#(type=="reasoning")`).Exists())

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, events)
	for _, event := range events {
		require.NotEqual(t, "recoverable-first", event.UpstreamRequestID)
	}
	require.Equal(t, http.StatusBadRequest, c.GetInt(OpsUpstreamStatusCodeKey))
}
