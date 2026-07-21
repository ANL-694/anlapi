package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	infraerrors "anlapi/internal/pkg/errors"
	middleware2 "anlapi/internal/server/middleware"
	"anlapi/internal/service"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_url}/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. The manifest is proxied verbatim from the selected account's ChatGPT
// backend or custom API key upstream. API key manifests use a short-lived
// cache; stale entries are served until expiry so all upstream refreshes stay
// inside a user-concurrency-guarded request.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI groups")
		return
	}
	if h.concurrencyHelper == nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User concurrency service is unavailable")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	streamStarted := false
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithQueue(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many concurrent requests, please retry later")
		return
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	defer userReleaseFunc()

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastUpstreamErr error

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, "", "", failedAccountIDs)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if lastUpstreamErr != nil {
				h.errorResponse(c, infraerrors.Code(lastUpstreamErr), "upstream_error", infraerrors.Message(lastUpstreamErr))
				return
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI accounts")
			return
		}

		manifest, err := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), c.GetHeader("If-None-Match"))
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if service.IsRetryableCodexModelsManifestError(err) && switchCount < maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				switchCount++
				lastUpstreamErr = err
				continue
			}
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}

		if manifest.ETag != "" {
			c.Header("ETag", manifest.ETag)
		}
		if manifest.NotModified {
			c.Status(http.StatusNotModified)
			return
		}
		c.Data(http.StatusOK, "application/json", manifest.Body)
		return
	}
}
