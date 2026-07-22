package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"anlapi/internal/pkg/logger"
	"anlapi/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

var openaiCCRawAllowedHeaders = map[string]bool{
	"accept-language": true,
	"user-agent":      true,
}

func (s *OpenAIGatewayService) forwardAsRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := gjson.GetBytes(body, "model").String()
	if originalModel == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()
	serviceTier := extractOpenAIServiceTierFromBody(body)

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	grokCacheIdentity := ""
	if account.Platform == PlatformGrok {
		grokCacheIdentity = resolveGrokCacheIdentity(c, body, "", upstreamModel)
	}
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)

	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, upstreamBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeChatCompletionsError(c, http.StatusForbidden, "permission_error", blocked.Message)
		}
		return nil, policyErr
	}
	upstreamBody = updatedBody
	if clientStream {
		var usageErr error
		upstreamBody, usageErr = ensureOpenAIChatStreamUsage(upstreamBody)
		if usageErr != nil {
			return nil, fmt.Errorf("enable stream usage: %w", usageErr)
		}
	}
	if account.Platform == PlatformGrok {
		strippedBody, stripErr := stripGrokChatPromptCacheKey(upstreamBody)
		if stripErr != nil {
			return nil, fmt.Errorf("remove Responses-only Grok prompt cache key: %w", stripErr)
		}
		upstreamBody = strippedBody
	}

	token, tokenKind, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("account %d missing %s credential", account.ID, tokenKind)
	}

	var targetURL string
	if account.Platform == PlatformGrok {
		targetURL, err = buildGrokChatCompletionsURL(account, s.cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid grok base_url: %w", err)
		}
	} else {
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			if account.IsKiro() {
				return nil, fmt.Errorf("account %d missing base_url", account.ID)
			}
			baseURL = "https://api.openai.com"
		}
		validatedURL, validateErr := s.validateUpstreamBaseURL(baseURL)
		if validateErr != nil {
			return nil, fmt.Errorf("invalid base_url: %w", validateErr)
		}
		targetURL = buildOpenAIChatCompletionsURL(validatedURL)
	}
	if account.Platform == PlatformGrok {
		SetActualOpenAIUpstreamEndpoint(c, grokChatRawEndpoint)
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, clientStream)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiCCRawAllowedHeaders[lowerKey] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA == "" && account.IsGrokOAuth() {
		customUA = "sub2api-grok/1.0"
	}
	if customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
	}
	if account.Platform == PlatformGrok {
		if account.IsGrokOAuth() {
			applyGrokCLIHeaders(upstreamReq.Header)
		}
		applyGrokCacheHeaders(upstreamReq.Header, grokCacheIdentity)
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	logger.L().Debug("openai chat_completions raw: forwarding without responses conversion",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.String("target_url", targetURL),
		zap.Bool("stream", clientStream),
	)

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if account.Platform == PlatformGrok {
			kind := "http_error"
			if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
				kind = "failover"
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
				Kind:               kind,
				Message:            upstreamMsg,
			})
			s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
				return nil, &UpstreamFailoverError{
					StatusCode:             resp.StatusCode,
					ResponseBody:           respBody,
					ResponseHeaders:        resp.Header.Clone(),
					RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
				}
			}
			return s.handleChatCompletionsErrorResponse(resp, c, account, upstreamModel)
		}
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: !shouldDisable && account.IsPoolMode() && (account.IsPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleChatCompletionsErrorResponse(resp, c, account, upstreamModel)
	}
	if account.Platform == PlatformGrok {
		s.updateGrokUsageFromResponse(ctx, account, resp.Header, resp.StatusCode)
	}

	var result *OpenAIForwardResult
	var forwardErr error
	if clientStream {
		result, forwardErr = s.streamRawChatCompletions(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	} else {
		result, forwardErr = s.bufferRawChatCompletions(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	}
	if result != nil && account.Platform == PlatformGrok {
		result.UpstreamEndpoint = grokChatRawEndpoint
		result.ResponseHeaders = resp.Header.Clone()
	}
	return result, forwardErr
}

func (s *OpenAIGatewayService) streamRawChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var usage OpenAIUsage
	var firstTokenMs *int
	clientDisconnected := false

	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			trimmedPayload := strings.TrimSpace(payload)
			if trimmedPayload != "" && trimmedPayload != "[DONE]" {
				usageOnlyChunk := isOpenAIChatUsageOnlyStreamChunk(payload)
				if u := extractCCStreamUsage(payload); u != nil {
					usage = *u
				}
				if firstTokenMs == nil && !usageOnlyChunk {
					elapsed := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &elapsed
				}
			}
		}

		if !clientDisconnected {
			if _, err := c.Writer.WriteString(line + "\n"); err != nil {
				clientDisconnected = true
				logger.L().Debug("openai chat_completions raw: client disconnected, continuing to drain upstream for billing",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
			}
			if line == "" && !clientDisconnected {
				c.Writer.Flush()
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai chat_completions raw: stream read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
		return &OpenAIForwardResult{
			RequestID:       requestID,
			Usage:           usage,
			Model:           originalModel,
			BillingModel:    billingModel,
			UpstreamModel:   upstreamModel,
			ReasoningEffort: reasoningEffort,
			ServiceTier:     serviceTier,
			Stream:          true,
			Duration:        time.Since(startTime),
			FirstTokenMs:    firstTokenMs,
		}, fmt.Errorf("stream usage incomplete: %w", err)
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func ensureOpenAIChatStreamUsage(body []byte) ([]byte, error) {
	updated, err := sjson.SetBytes(body, "stream_options.include_usage", true)
	if err != nil {
		return body, err
	}
	return updated, nil
}

func isOpenAIChatUsageOnlyStreamChunk(payload string) bool {
	if strings.TrimSpace(payload) == "" {
		return false
	}
	if !gjson.Get(payload, "usage").Exists() {
		return false
	}
	choices := gjson.Get(payload, "choices")
	return choices.Exists() && choices.IsArray() && len(choices.Array()) == 0
}

func extractCCStreamUsage(payload string) *OpenAIUsage {
	usageResult := gjson.Get(payload, "usage")
	if !usageResult.Exists() || !usageResult.IsObject() {
		return nil
	}
	u, ok := openAIUsageFromGJSON(usageResult)
	if !ok {
		return nil
	}
	return &u
}

func (s *OpenAIGatewayService) bufferRawChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	var usage OpenAIUsage
	if parsed, ok := openAIUsageFromGJSON(gjson.GetBytes(respBody, "usage")); ok {
		usage = parsed
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(respBody)

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

func buildOpenAIChatCompletionsURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/chat/completions")
}

func buildOpenAIEndpointURL(base string, defaultPath string) string {
	normalized := strings.TrimSpace(base)
	defaultPath = "/" + strings.TrimLeft(strings.TrimSpace(defaultPath), "/")
	relative := strings.TrimPrefix(defaultPath, "/v1")
	parsed, err := url.Parse(normalized)
	if err != nil {
		return strings.TrimRight(normalized, "/") + defaultPath
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, defaultPath) && !strings.HasSuffix(path, relative) {
		if openAIBaseURLHasVersionSuffix(path) {
			path += relative
		} else {
			path += defaultPath
		}
	}
	parsed.Path = path
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String()
}

func openAIBaseURLHasVersionSuffix(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}

	pathValue := ""
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		pathValue = parsed.Path
	} else if slash := strings.Index(trimmed, "/"); slash >= 0 {
		pathValue = trimmed[slash:]
	}
	pathValue = strings.TrimRight(pathValue, "/")
	if pathValue == "" {
		return false
	}
	lastSlash := strings.LastIndex(pathValue, "/")
	segment := pathValue
	if lastSlash >= 0 {
		segment = pathValue[lastSlash+1:]
	}
	return isOpenAIAPIVersionSegment(segment)
}

func isOpenAIAPIVersionSegment(segment string) bool {
	s := strings.ToLower(strings.TrimSpace(segment))
	if len(s) < 2 || s[0] != 'v' || !isASCIIDigit(s[1]) {
		return false
	}

	i := 1
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	if i == len(s) {
		return true
	}
	if s[i] == '.' {
		i++
		if i == len(s) || !isASCIIDigit(s[i]) {
			return false
		}
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
		}
		return i == len(s)
	}

	suffix := s[i:]
	return strings.HasPrefix(suffix, "alpha") ||
		strings.HasPrefix(suffix, "beta") ||
		strings.HasPrefix(suffix, "preview")
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
