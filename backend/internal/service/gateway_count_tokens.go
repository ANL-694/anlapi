package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"anlapi/internal/pkg/claude"
	"anlapi/internal/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// ForwardCountTokens 转发 count_tokens 请求到上游 API。
// 此端点不记录使用量，仅支持非流式响应。
func (s *GatewayService) ForwardCountTokens(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) error {
	if parsed == nil {
		s.countTokensError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return fmt.Errorf("parse request: empty request")
	}

	if account != nil && account.IsAnthropicAPIKeyPassthroughEnabled() {
		passthroughBody := parsed.Body.Bytes()
		if reqModel := parsed.Model; reqModel != "" {
			if mappedModel := account.GetMappedModel(reqModel); mappedModel != reqModel {
				passthroughBody = s.replaceModelInBody(passthroughBody, mappedModel)
				logger.LegacyPrintf("service.gateway", "CountTokens passthrough model mapping: %s -> %s (account: %s)", reqModel, mappedModel, account.Name)
			}
		}
		return s.forwardCountTokensAnthropicAPIKeyPassthrough(ctx, c, account, passthroughBody)
	}

	if account != nil && account.IsBedrock() {
		s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported for Bedrock")
		return nil
	}

	body := parsed.Body.Bytes()
	replaceBody := func(next []byte) error {
		if err := parsed.ReplaceBody(next); err != nil {
			return fmt.Errorf("rewrite count_tokens body: %w", err)
		}
		body = parsed.Body.Bytes()
		return nil
	}
	reqModel := parsed.Model

	if err := replaceBody(StripEmptyTextBlocks(body)); err != nil {
		return err
	}

	isClaudeCodeCT := IsClaudeCodeClient(ctx) || isClaudeCodeClient(c.GetHeader("User-Agent"), parsed.MetadataUserID)
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCodeCT
	if shouldMimicClaudeCode {
		normalizeOpts := claudeOAuthNormalizeOptions{stripSystemCacheControl: true}
		normalizedBody, normalizedModel := normalizeClaudeOAuthRequestBody(body, reqModel, normalizeOpts)
		reqModel = normalizedModel
		if err := replaceBody(normalizedBody); err != nil {
			return err
		}
		if err := replaceBody(s.rewriteMessageCacheControlIfEnabled(ctx, body)); err != nil {
			return err
		}
		if rewrite := buildToolNameRewriteFromBody(body); rewrite != nil {
			if err := replaceBody(applyToolNameRewriteToBody(body, rewrite)); err != nil {
				return err
			}
		} else if err := replaceBody(applyToolsLastCacheBreakpoint(body)); err != nil {
			return err
		}
	}

	if account.Platform == PlatformAntigravity {
		s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported for this platform")
		return nil
	}

	if reqModel != "" {
		mappedModel := reqModel
		mappingSource := ""
		if account.Type == AccountTypeAPIKey {
			mappedModel = account.GetMappedModel(reqModel)
			if mappedModel != reqModel {
				mappingSource = "account"
			}
		}
		if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
			normalized := claude.NormalizeModelID(reqModel)
			if normalized != reqModel {
				mappedModel = normalized
				mappingSource = "prefix"
			}
		}
		if mappedModel != reqModel {
			originalReqModel := reqModel
			if err := replaceBody(s.replaceModelInBody(body, mappedModel)); err != nil {
				return err
			}
			reqModel = mappedModel
			parsed.Model = mappedModel
			logger.LegacyPrintf("service.gateway", "CountTokens model mapping applied: %s -> %s (account: %s, source=%s)", originalReqModel, mappedModel, account.Name, mappingSource)
		}
	}

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}

	upstreamReq, wireBody, err := s.buildCountTokensRequest(ctx, c, account, body, token, tokenType, reqModel, shouldMimicClaudeCode)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}
	acceptedWireBody := wireBody

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil && (!account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "") {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(err.Error()), "")
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}

	tooLarge := func(c *gin.Context) {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, tooLarge)
	_ = resp.Body.Close()
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		}
		return err
	}

	if resp.StatusCode == http.StatusBadRequest && s.shouldRectifySignatureError(ctx, account, respBody, reqModel) {
		filteredBody := FilterThinkingBlocksForRetry(body, reqModel)
		retryReq, retryWireBody, buildErr := s.buildCountTokensRequest(ctx, c, account, filteredBody, token, tokenType, reqModel, shouldMimicClaudeCode)
		if buildErr == nil {
			retryResp, retryErr := s.httpUpstream.DoWithTLS(retryReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
			if retryErr == nil {
				if retryResp.StatusCode < http.StatusBadRequest {
					acceptedWireBody = retryWireBody
				}
				resp = retryResp
				respBody, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, tooLarge)
				_ = resp.Body.Close()
				if err != nil {
					if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
						s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
					}
					return err
				}
			}
		}
	}

	if resp.StatusCode < http.StatusBadRequest && !bytes.Equal(acceptedWireBody, body) {
		if err := replaceBody(acceptedWireBody); err != nil {
			return err
		}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = truncateString(string(respBody), maxBytes)
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			logger.LegacyPrintf("service.gateway", "count_tokens upstream error %d (account=%d platform=%s type=%s): %s", resp.StatusCode, account.ID, account.Platform, account.Type, truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes))
		}
		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
		}
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	c.Data(resp.StatusCode, "application/json", respBody)
	return nil
}

func (s *GatewayService) forwardCountTokensAnthropicAPIKeyPassthrough(ctx context.Context, c *gin.Context, account *Account, body []byte) error {
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}
	if tokenType != "apikey" {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Invalid account token type")
		return fmt.Errorf("anthropic api key passthrough requires apikey token, got: %s", tokenType)
	}
	upstreamReq, err := s.buildCountTokensRequestAnthropicAPIKeyPassthrough(ctx, c, account, body, token)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(err.Error()), "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{Platform: account.Platform, AccountID: account.ID, AccountName: account.Name, UpstreamStatusCode: 0, UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()), Passthrough: true, Kind: "request_error", Message: sanitizeUpstreamErrorMessage(err.Error())})
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}

	tooLarge := func(c *gin.Context) {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, tooLarge)
	_ = resp.Body.Close()
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		}
		return err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if isCountTokensUnsupported404(resp.StatusCode, respBody) {
			logger.LegacyPrintf("service.gateway", "[count_tokens] Upstream does not support count_tokens (404), returning 404: account=%d name=%s msg=%s", account.ID, account.Name, truncateString(upstreamMsg, 512))
			s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported by upstream")
			return nil
		}
		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = truncateString(string(respBody), maxBytes)
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{Platform: account.Platform, AccountID: account.ID, AccountName: account.Name, UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"), UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()), Passthrough: true, Kind: "http_error", Message: upstreamMsg, Detail: upstreamDetail})
		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
		}
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)
	return nil
}

func (s *GatewayService) buildCountTokensRequestAnthropicAPIKeyPassthrough(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	targetURL := claudeAPICountTokensURL
	if baseURL := account.GetBaseURL(); baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = validatedURL + "/v1/messages/count_tokens?beta=true"
	}
	body = sanitizeCountTokensRequestBody(body)
	clientBeta := ""
	if c != nil && c.Request != nil {
		clientBeta = getHeaderRaw(c.Request.Header, "anthropic-beta")
	}
	if beta, ok := account.HeaderOverrideValue("anthropic-beta"); ok {
		clientBeta = beta
	}
	if sanitized, changed := sanitizeAnthropicBodyForBetaTokens(body, clientBeta); changed {
		body = sanitized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			if !allowedHeaders[strings.ToLower(strings.TrimSpace(key))] {
				continue
			}
			wireKey := resolveWireCasing(key)
			for _, value := range values {
				addHeaderRaw(req.Header, wireKey, value)
			}
		}
	}
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Del("cookie")
	setAnthropicAPIKeyAuthHeader(req.Header, account, token)
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func (s *GatewayService) buildCountTokensRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string, mimicClaudeCode bool) (*http.Request, []byte, error) {
	targetURL := claudeAPICountTokensURL
	if account.Type == AccountTypeAPIKey {
		if baseURL := account.GetBaseURL(); baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, nil, err
			}
			targetURL = validatedURL + "/v1/messages/count_tokens?beta=true"
		}
	} else if account.IsCustomBaseURLEnabled() {
		customURL := account.GetCustomBaseURL()
		if customURL == "" {
			return nil, nil, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
		}
		validatedURL, err := s.validateUpstreamBaseURL(customURL)
		if err != nil {
			return nil, nil, err
		}
		targetURL = s.buildCustomRelayURL(validatedURL, "/v1/messages/count_tokens", account)
	}

	clientHeaders := http.Header{}
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
	}
	ctEnableFP, ctEnableMPT := true, false
	if s.settingService != nil {
		ctEnableFP, ctEnableMPT, _ = s.settingService.GetGatewayForwardingSettings(ctx)
	}
	var fingerprint *Fingerprint
	if account.IsOAuth() && s.identityService != nil {
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, clientHeaders)
		if err == nil {
			fingerprint = fp
			if !ctEnableMPT {
				if accountUUID := account.GetExtraString("account_uuid"); accountUUID != "" && fp.ClientID != "" {
					if next, err := s.identityService.RewriteUserIDWithMasking(ctx, body, account, accountUUID, fp.ClientID, fp.UserAgent); err == nil && len(next) > 0 {
						body = next
					}
				}
			}
		}
	}
	if fingerprint != nil && ctEnableFP {
		body = syncBillingHeaderVersion(body, fingerprint.UserAgent)
	}

	dropSet := mergeDropSets(s.getBetaPolicyFilterSet(ctx, c, account, modelID))
	finalBetaHeader, finalBetaShouldSet := s.computeFinalCountTokensAnthropicBeta(tokenType, mimicClaudeCode, modelID, clientHeaders, body, dropSet)
	if beta, ok := account.HeaderOverrideValue("anthropic-beta"); ok {
		finalBetaHeader, finalBetaShouldSet = beta, true
	}
	if sanitized, changed := sanitizeAnthropicBodyForBetaTokens(body, finalBetaHeader); changed {
		body = sanitized
	}
	body = sanitizeCountTokensRequestBody(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	if tokenType == "oauth" {
		setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	} else {
		setAnthropicAPIKeyAuthHeader(req.Header, account, token)
	}
	for key, values := range clientHeaders {
		if !allowedHeaders[strings.ToLower(key)] {
			continue
		}
		wireKey := resolveWireCasing(key)
		for _, value := range values {
			addHeaderRaw(req.Header, wireKey, value)
		}
	}
	if ctEnableFP && fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
	}
	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
	}
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	}
	if tokenType == "oauth" {
		applyClaudeOAuthHeaderDefaults(req)
	}
	if tokenType == "oauth" && mimicClaudeCode {
		applyClaudeCodeMimicHeaders(req, false)
	}
	deleteHeaderAllForms(req.Header, "anthropic-beta")
	if finalBetaShouldSet {
		setHeaderRaw(req.Header, "anthropic-beta", finalBetaHeader)
	}
	if sessionHeader := getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"); sessionHeader != "" {
		if userID := gjson.GetBytes(body, "metadata.user_id").String(); userID != "" {
			if parsed := ParseMetadataUserID(userID); parsed != nil {
				setHeaderRaw(req.Header, "X-Claude-Code-Session-Id", parsed.SessionID)
			}
		}
	}
	account.ApplyHeaderOverrides(req.Header)
	if c != nil && tokenType == "oauth" {
		c.Set(claudeMimicDebugInfoKey, buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode))
	}
	if s.debugClaudeMimicEnabled() {
		logClaudeMimicDebug(req, body, account, tokenType, mimicClaudeCode)
	}
	return req, body, nil
}

func sanitizeCountTokensRequestBody(body []byte) []byte {
	out := body
	for _, path := range []string{"temperature", "top_p", "top_k", "stream", "stop_sequences", "stop"} {
		if gjson.GetBytes(out, path).Exists() {
			if next, ok := deleteJSONPathBytes(out, path); ok {
				out = next
			}
		}
	}
	return out
}

func (s *GatewayService) countTokensError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
