package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"anlapi/internal/config"
	pkghttputil "anlapi/internal/pkg/httputil"
	"anlapi/internal/server/middleware"
	"anlapi/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestOpenAIHandleStreamingAwareError_JSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
	}{
		{
			name:    "包含双引号的消息",
			errType: "server_error",
			message: `upstream returned "invalid" response`,
		},
		{
			name:    "包含反斜杠的消息",
			errType: "server_error",
			message: `path C:\Users\test\file.txt not found`,
		},
		{
			name:    "包含双引号和反斜杠的消息",
			errType: "upstream_error",
			message: `error parsing "key\value": unexpected token`,
		},
		{
			name:    "包含换行符的消息",
			errType: "server_error",
			message: "line1\nline2\ttab",
		},
		{
			name:    "普通消息",
			errType: "upstream_error",
			message: "Upstream service temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			h := &OpenAIGatewayHandler{}
			h.handleStreamingAwareError(c, http.StatusBadGateway, tt.errType, tt.message, true)

			body := w.Body.String()

			// 验证 SSE 格式：event: error\ndata: {JSON}\n\n
			assert.True(t, strings.HasPrefix(body, "event: error\n"), "应以 'event: error\\n' 开头")
			assert.True(t, strings.HasSuffix(body, "\n\n"), "应以 '\\n\\n' 结尾")

			// 提取 data 部分
			lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
			require.Len(t, lines, 2, "应有 event 行和 data 行")
			dataLine := lines[1]
			require.True(t, strings.HasPrefix(dataLine, "data: "), "第二行应以 'data: ' 开头")
			jsonStr := strings.TrimPrefix(dataLine, "data: ")

			// 验证 JSON 合法性
			var parsed map[string]any
			err := json.Unmarshal([]byte(jsonStr), &parsed)
			require.NoError(t, err, "JSON 应能被成功解析，原始 JSON: %s", jsonStr)

			// 验证结构
			errorObj, ok := parsed["error"].(map[string]any)
			require.True(t, ok, "应包含 error 对象")
			assert.Equal(t, tt.errType, errorObj["type"])
			assert.Equal(t, tt.message, errorObj["message"])
		})
	}
}

func TestOpenAIHandleStreamingAwareErrorWithCode_EmitsStableClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareErrorWithCode(
		c,
		http.StatusBadGateway,
		"upstream_error",
		service.OpenAIUpstreamHTTP2StreamErrorCode,
		"Upstream HTTP/2 stream failed",
		true,
		true,
	)

	body := recorder.Body.String()
	require.Contains(t, body, "event: error\n")
	jsonStart := strings.Index(body, "{")
	require.NotEqual(t, -1, jsonStart)
	require.Equal(t, "upstream_error", gjson.Get(body[jsonStart:], "error.type").String())
	require.Equal(t, service.OpenAIUpstreamHTTP2StreamErrorCode, gjson.Get(body[jsonStart:], "error.code").String())
	require.NotContains(t, body, "stream ID")

	streamErr, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.True(t, streamErr.CountTowardsSLA)
	require.Equal(t, http.StatusBadGateway, streamErr.IntendedStatus)
}

func TestOpenAIHandleStreamingAwareError_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "test error", false)

	// 非流式应返回 JSON 响应
	assert.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "test error", errorObj["message"])
}

func TestReadRequestBodyWithPrealloc(t *testing.T) {
	payload := `{"model":"gpt-5","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.NoError(t, err)
	require.Equal(t, payload, string(body))
}

func TestReadRequestBodyWithPrealloc_MaxBytesError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(strings.Repeat("x", 8)))
	req.Body = http.MaxBytesReader(rec, req.Body, 4)

	_, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.Error(t, err)
	var maxErr *http.MaxBytesError
	require.ErrorAs(t, err, &maxErr)
}

func TestOpenAIEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIEnsureForwardErrorResponse_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIEnsureForwardErrorResponse_AfterDeltaAppendsSingleValidResponseFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)

	delta := `{"type":"response.output_text.delta","delta":"ok","sequence_number":1}`
	_, err := c.Writer.WriteString("event: response.output_text.delta\ndata: " + delta + "\n\n")
	require.NoError(t, err)

	h := &OpenAIGatewayHandler{}
	require.True(t, h.ensureForwardErrorResponse(c, true))

	frames := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n\n")
	require.Len(t, frames, 2)
	errorEvents := 0
	for _, frame := range frames {
		lines := strings.Split(frame, "\n")
		require.Len(t, lines, 2)
		require.True(t, strings.HasPrefix(lines[0], "event: "))
		require.True(t, strings.HasPrefix(lines[1], "data: "))

		eventType := strings.TrimPrefix(lines[0], "event: ")
		data := strings.TrimPrefix(lines[1], "data: ")
		require.True(t, json.Valid([]byte(data)), "each downstream SSE frame must contain valid JSON")
		var event struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &event))
		require.Equal(t, eventType, event.Type)
		if eventType == "response.failed" {
			errorEvents++
		}
	}
	require.Equal(t, 1, errorEvents)
}

func TestOpenAIEnsureForwardErrorResponse_ImageJSONKeepaliveWritesSingleJSONFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	stop := service.StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	before := service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
	require.Equal(t, before, service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c))
	require.False(t, openAIForwardErrorAlreadyCommunicated(c, before, errors.New("read upstream response: unexpected EOF")))

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusOK, w.Code, "heartbeat already committed the status")
	require.True(t, json.Valid(w.Body.Bytes()), w.Body.String())
	require.NotContains(t, w.Body.String(), "event:")
	require.NotContains(t, w.Body.String(), "data:")

	decoder := json.NewDecoder(strings.NewReader(w.Body.String()))
	var payload map[string]any
	require.NoError(t, decoder.Decode(&payload))
	require.ErrorIs(t, decoder.Decode(&payload), io.EOF)
	require.Equal(t, "upstream_error", gjson.Get(w.Body.String(), "error.type").String())
	require.Equal(t, "Upstream request failed", gjson.Get(w.Body.String(), "error.message").String())
}

func TestOpenAIEnsureForwardErrorResponse_ImageJSONKeepalivePreservesCompletedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	stop := service.StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	before := service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
	c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"b64_json": "aW1hZ2U="}}})
	completedBody := w.Body.String()
	require.True(t, json.Valid([]byte(completedBody)), completedBody)
	require.Greater(t, service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), before)
	require.False(t, openAIForwardErrorAlreadyCommunicated(c, before, errors.New("read upstream trailer: unexpected EOF")))

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote, "the completed Images JSON already communicated the response")
	require.Equal(t, completedBody, w.Body.String())
	require.NotContains(t, w.Body.String(), "event:")
	require.NotContains(t, w.Body.String(), "data:")

	decoder := json.NewDecoder(strings.NewReader(w.Body.String()))
	var payload map[string]any
	require.NoError(t, decoder.Decode(&payload))
	require.ErrorIs(t, decoder.Decode(&payload), io.EOF)
	require.Equal(t, "aW1hZ2U=", gjson.Get(w.Body.String(), "data.0.b64_json").String())
}

func TestOpenAIEnsureForwardErrorResponse_FastImageJSONKeepalivePreservesCompletedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	stop := service.StartOpenAIImagesJSONKeepalive(c, time.Hour)
	defer stop()
	before := service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
	c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"b64_json": "ZmFzdC1pbWFnZQ=="}}})
	completedBody := w.Body.String()
	require.True(t, json.Valid([]byte(completedBody)), completedBody)
	require.Greater(t, service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), before)
	require.False(t, openAIForwardErrorAlreadyCommunicated(c, before, errors.New("read upstream trailer: unexpected EOF")))

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote, "fast completed Images JSON already communicated the response")
	require.Equal(t, completedBody, w.Body.String())
	require.NotContains(t, w.Body.String(), "event:")
	require.NotContains(t, w.Body.String(), "data:")

	decoder := json.NewDecoder(strings.NewReader(w.Body.String()))
	var payload map[string]any
	require.NoError(t, decoder.Decode(&payload))
	require.ErrorIs(t, decoder.Decode(&payload), io.EOF)
	require.Equal(t, "ZmFzdC1pbWFnZQ==", gjson.Get(w.Body.String(), "data.0.b64_json").String())
}

func TestShouldLogOpenAIForwardFailureAsWarn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("fallback_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, true))
	})

	t.Run("context_nil_should_not_downgrade", func(t *testing.T) {
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(nil, false))
	})

	t.Run("response_not_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})

	t.Run("response_already_written_should_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.String(http.StatusForbidden, "already written")
		require.True(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})
}

func TestOpenAIRecoverResponsesPanic_WritesFallbackResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIRecoverResponsesPanic_NoPanicNoWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
		}()
	})

	require.False(t, c.Writer.Written())
	assert.Equal(t, "", w.Body.String())
}

func TestOpenAIRecoverResponsesPanic_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIMissingResponsesDependencies(t *testing.T) {
	t.Run("nil_handler", func(t *testing.T) {
		var h *OpenAIGatewayHandler
		require.Equal(t, []string{"handler"}, h.missingResponsesDependencies())
	})

	t.Run("all_dependencies_missing", func(t *testing.T) {
		h := &OpenAIGatewayHandler{}
		require.Equal(t,
			[]string{"gatewayService", "billingCacheService", "apiKeyService", "concurrencyHelper"},
			h.missingResponsesDependencies(),
		)
	})

	t.Run("all_dependencies_present", func(t *testing.T) {
		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		require.Empty(t, h.missingResponsesDependencies())
	})
}

func TestOpenAIEnsureResponsesDependencies(t *testing.T) {
	t.Run("missing_dependencies_returns_503", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		var parsed map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &parsed)
		require.NoError(t, err)
		errorObj, exists := parsed["error"].(map[string]any)
		require.True(t, exists)
		assert.Equal(t, "api_error", errorObj["type"])
		assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
	})

	t.Run("already_written_response_not_overridden", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.String(http.StatusTeapot, "already written")

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusTeapot, w.Code)
		assert.Equal(t, "already written", w.Body.String())
	})

	t.Run("dependencies_ready_returns_true_and_no_write", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		ok := h.ensureResponsesDependencies(c, nil)

		require.True(t, ok)
		require.False(t, c.Writer.Written())
		assert.Equal(t, "", w.Body.String())
	})
}

func TestResolveOpenAIForwardDefaultMappedModel(t *testing.T) {
	t.Run("prefers_explicit_fallback_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.2", resolveOpenAIForwardDefaultMappedModel(apiKey, " gpt-5.2 "))
	})

	t.Run("uses_group_default_when_explicit_fallback_absent", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.4", resolveOpenAIForwardDefaultMappedModel(apiKey, ""))
	})

	t.Run("returns_empty_without_group_default", func(t *testing.T) {
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(nil, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{}, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{
			Group: &service.Group{},
		}, ""))
	})
}

func TestResolveOpenAIMessagesDispatchMappedModel(t *testing.T) {
	t.Run("exact_claude_model_override_wins", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
					SonnetMappedModel: "gpt-5.2",
					ExactModelMappings: map[string]string{
						"claude-sonnet-4-5-20250929": "gpt-5.4-mini-high",
						"claude-fable-5":             "gpt-5.6-sol",
					},
				},
			},
		}
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
		require.Equal(t, "gpt-5.6-sol", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-fable-5"))
	})

	t.Run("uses_family_default_when_no_override", func(t *testing.T) {
		apiKey := &service.APIKey{Group: &service.Group{}}
		require.Equal(t, "gpt-5.4", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-opus-4-6"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-haiku-4-5-20251001"))
	})

	t.Run("returns_empty_for_non_claude_or_missing_group", func(t *testing.T) {
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(nil, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{}, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{Group: &service.Group{}}, "gpt-5.4"))
	})

	t.Run("does_not_fall_back_to_group_default_mapped_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				DefaultMappedModel: "gpt-5.4",
			},
		}
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(apiKey, "gpt-5.4"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})
}

func TestOpenAIResponses_MissingDependencies_ReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	// 故意使用未初始化依赖，验证快速失败而不是崩溃。
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.Responses(c)
	})

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errorObj["type"])
	assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
}

func TestOpenAIResponses_SetsClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &OpenAIGatewayHandler{}
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponses_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.1","stream":false,"previous_response_id":"msg_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id must be a response.id")
}

func TestOpenAIResponses_RejectsHTTPContinuationPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "Responses WebSocket v2")
	require.Contains(t, w.Body.String(), "previous_response_id")
}

func TestOpenAIResponses_FunctionCallOutputHTTPGuidanceDoesNotSuggestPreviousResponseReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.1","stream":false,"input":[{"type":"function_call_output","output":"{}"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "Responses WebSocket v2")
	require.NotContains(t, w.Body.String(), "reuse previous_response_id")
}

func TestOpenAIResponsesWebSocket_SetsClientTransportWSWhenUpgradeValid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_InvalidUpgradeDoesNotSetTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUpgradeRequired, w.Code)
	require.Equal(t, service.OpenAIClientTransportUnknown, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_IgnoresAPIKeyIngressCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireIngressLeaseFn: func(context.Context, int64, int, string) (bool, error) {
			return false, nil
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	h.cfg = &config.Config{}
	h.cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey = 1
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, response, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	require.NotNil(t, clientConn)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	_ = clientConn.CloseNow()
}

func TestOpenAIResponsesWebSocket_IgnoresAPIKeyIngressLeaseBackend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireIngressLeaseFn: func(context.Context, int64, int, string) (bool, error) {
			return false, errors.New("redis unavailable")
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	h.cfg = &config.Config{}
	h.cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey = 1
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, response, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	require.NotNil(t, clientConn)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	_ = clientConn.CloseNow()
}

func TestOpenAIResponsesWebSocket_FirstMessageTimeoutUsesConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.cfg = &config.Config{}
	h.cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds = 1
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	started := time.Now()
	readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	elapsed := time.Since(started)

	require.Error(t, err)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, elapsed, 500*time.Millisecond)
	require.Less(t, elapsed, 4*time.Second)
	require.Eventually(t, func() bool {
		readTimeout, ok := logSink.LatestFieldValueForMessage("openai.websocket_read_first_message_failed", "read_timeout")
		return ok && readTimeout == time.Second &&
			logSink.ContainsMessageAtLevel("openai.websocket_read_first_message_failed", "warn")
	}, time.Second, 10*time.Millisecond)
}

func TestOpenAIResponsesWebSocket_IngressLeaseReleasedOnEarlyReturn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireIngressLeaseFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	h.cfg = &config.Config{}
	h.cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey = 1
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageBinary, []byte("not a response.create frame"))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Zero(t, atomic.LoadInt32(&cache.releaseIngressCalled))
}

func TestOpenAIResponsesWebSocket_IngressLeaseReleasedWhenUpgradeFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireIngressLeaseFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	h.cfg = &config.Config{}
	h.cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey = 1
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	req, err := http.NewRequest(http.MethodGet, wsServer.URL+"/openai/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.NotEqual(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.Zero(t, atomic.LoadInt32(&cache.releaseIngressCalled))
}

func TestOpenAIResponsesWebSocket_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"msg_abc123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "previous_response_id")
}

func TestOpenAIResponsesWebSocket_PreviousResponseIDKindLoggedBeforeAcquireFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return false, errors.New("user slot unavailable")
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusInternalError, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "failed to acquire user concurrency slot")
}

func TestSetOpenAIClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportHTTP(c)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestSetOpenAIClientTransportWS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportWS(c)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

// TestOpenAIHandler_GjsonExtraction 验证 gjson 从请求体中提取 model/stream 的正确性
func TestOpenAIHandler_GjsonExtraction(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantModel  string
		wantStream bool
	}{
		{"正常提取", `{"model":"gpt-4","stream":true,"input":"hello"}`, "gpt-4", true},
		{"stream false", `{"model":"gpt-4","stream":false}`, "gpt-4", false},
		{"无 stream 字段", `{"model":"gpt-4"}`, "gpt-4", false},
		{"model 缺失", `{"stream":true}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			modelResult := gjson.GetBytes(body, "model")
			model := ""
			if modelResult.Type == gjson.String {
				model = modelResult.String()
			}
			stream := gjson.GetBytes(body, "stream").Bool()
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
		})
	}
}

// TestOpenAIHandler_GjsonValidation 验证修复后的 JSON 合法性和类型校验
func TestOpenAIHandler_GjsonValidation(t *testing.T) {
	// 非法 JSON 被 gjson.ValidBytes 拦截
	require.False(t, gjson.ValidBytes([]byte(`{invalid json`)))

	// model 为数字 → 类型不是 gjson.String，应被拒绝
	body := []byte(`{"model":123}`)
	modelResult := gjson.GetBytes(body, "model")
	require.True(t, modelResult.Exists())
	require.NotEqual(t, gjson.String, modelResult.Type)

	// model 为 null → 类型不是 gjson.String，应被拒绝
	body2 := []byte(`{"model":null}`)
	modelResult2 := gjson.GetBytes(body2, "model")
	require.True(t, modelResult2.Exists())
	require.NotEqual(t, gjson.String, modelResult2.Type)

	// stream 为 string → 类型既不是 True 也不是 False，应被拒绝
	body3 := []byte(`{"model":"gpt-4","stream":"true"}`)
	streamResult := gjson.GetBytes(body3, "stream")
	require.True(t, streamResult.Exists())
	require.NotEqual(t, gjson.True, streamResult.Type)
	require.NotEqual(t, gjson.False, streamResult.Type)

	// stream 为 int → 同上
	body4 := []byte(`{"model":"gpt-4","stream":1}`)
	streamResult2 := gjson.GetBytes(body4, "stream")
	require.True(t, streamResult2.Exists())
	require.NotEqual(t, gjson.True, streamResult2.Type)
	require.NotEqual(t, gjson.False, streamResult2.Type)
}

// TestOpenAIHandler_InstructionsInjection 验证 instructions 的 gjson/sjson 注入逻辑
func TestOpenAIHandler_InstructionsInjection(t *testing.T) {
	// 测试 1：无 instructions → 注入
	body := []byte(`{"model":"gpt-4"}`)
	existing := gjson.GetBytes(body, "instructions").String()
	require.Empty(t, existing)
	newBody, err := sjson.SetBytes(body, "instructions", "test instruction")
	require.NoError(t, err)
	require.Equal(t, "test instruction", gjson.GetBytes(newBody, "instructions").String())

	// 测试 2：已有 instructions → 不覆盖
	body2 := []byte(`{"model":"gpt-4","instructions":"existing"}`)
	existing2 := gjson.GetBytes(body2, "instructions").String()
	require.Equal(t, "existing", existing2)

	// 测试 3：空白 instructions → 注入
	body3 := []byte(`{"model":"gpt-4","instructions":"   "}`)
	existing3 := strings.TrimSpace(gjson.GetBytes(body3, "instructions").String())
	require.Empty(t, existing3)

	// 测试 4：sjson.SetBytes 返回错误时不应 panic
	// 正常 JSON 不会产生 sjson 错误，验证返回值被正确处理
	validBody := []byte(`{"model":"gpt-4"}`)
	result, setErr := sjson.SetBytes(validBody, "instructions", "hello")
	require.NoError(t, setErr)
	require.True(t, gjson.ValidBytes(result))
}

func newOpenAIHandlerForPreviousResponseIDValidation(t *testing.T, cache *concurrencyCacheMock) *OpenAIGatewayHandler {
	t.Helper()
	if cache == nil {
		cache = &concurrencyCacheMock{
			acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
			acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
		}
	}
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}

func newOpenAIWSHandlerTestServer(t *testing.T, h *OpenAIGatewayHandler, subject middleware.AuthSubject) *httptest.Server {
	t.Helper()
	groupID := int64(2)
	group := &service.Group{
		ID:       groupID,
		Platform: service.PlatformOpenAI,
		Status:   service.StatusActive,
	}
	apiKey := &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: subject.UserID},
		Group:   group,
		GroupRoutes: []service.APIKeyGroupRoute{{
			APIKeyID:        101,
			GroupID:         groupID,
			Priority:        100,
			Weight:          1,
			Enabled:         true,
			CooldownSeconds: 30,
			Group:           group,
		}},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), subject)
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	return httptest.NewServer(router)
}
