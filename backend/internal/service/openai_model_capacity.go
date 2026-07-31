package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

func isOpenAIModelCapacityError(upstreamStatusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if upstreamStatusCode > 0 && upstreamStatusCode < http.StatusBadRequest {
		return false
	}
	parts := make([]string, 0, 8)
	if upstreamMsg != "" {
		parts = append(parts, upstreamMsg)
	}
	if len(upstreamBody) > 0 {
		for _, path := range []string{
			"error.message", "error.code", "error.type",
			"response.error.message", "response.error.code", "response.error.type",
			"message", "code", "type",
		} {
			if value := strings.TrimSpace(gjson.GetBytes(upstreamBody, path).String()); value != "" {
				parts = append(parts, value)
			}
		}
		parts = append(parts, string(upstreamBody))
	}
	return isOpenAIModelCapacityText(strings.Join(parts, " "))
}

func isOpenAIModelCapacityText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "selected model is at capacity") || strings.Contains(lower, "model is at capacity") {
		return true
	}
	if strings.Contains(lower, "try a different model") && strings.Contains(lower, "capacity") {
		return true
	}
	if strings.Contains(lower, "capacity_exhaust") || (strings.Contains(lower, "model_capacity") && strings.Contains(lower, "exhaust")) {
		return true
	}
	return strings.Contains(lower, "no capacity available") && strings.Contains(lower, "model")
}

func (s *OpenAIGatewayService) persistOpenAIModelCapacitySignal(
	ctx context.Context,
	account *Account,
	headers http.Header,
	responseBody []byte,
	upstreamMsg string,
	canonicalModel ...string,
) bool {
	if s == nil || account == nil || account.Platform != PlatformOpenAI ||
		!isOpenAIModelCapacityError(http.StatusServiceUnavailable, upstreamMsg, responseBody) {
		return false
	}
	s.handleOpenAIAccountUpstreamError(
		ctx,
		account,
		http.StatusServiceUnavailable,
		headers,
		responseBody,
		canonicalModel...,
	)
	return true
}
