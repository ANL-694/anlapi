package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GatewayHandler) pinnedOpenAIModels(c *gin.Context, group *service.Group) {
	if c.Request.Context().Err() != nil {
		return
	}
	if h.openAIGatewayService == nil {
		writeOpenAIModelsError(c, http.StatusInternalServerError, "api_error", "OpenAI model discovery is not configured")
		return
	}
	requestedModel := strings.TrimSpace(c.Param("model"))
	ifNoneMatch := c.GetHeader("If-None-Match")
	// A pinned single-model lookup returns one raw catalogue entry, so the
	// collection ETag must not turn it into a false 304 response.
	if requestedModel != "" {
		ifNoneMatch = ""
	}
	response, account, err := h.openAIGatewayService.FetchPinnedOpenAIModelsList(
		c.Request.Context(), group, h.maxAccountSwitches, ifNoneMatch,
	)
	if c.Request.Context().Err() != nil {
		return
	}
	if err != nil {
		if errors.Is(err, service.ErrNoPinnedCodexModelsAccounts) {
			writeOpenAIModelsError(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI model discovery accounts")
			return
		}
		writeOpenAIModelsError(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
		return
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)
	if requestedModel != "" {
		writePinnedOpenAIModel(c, response.Body, requestedModel)
		return
	}
	writeOpenAIModelsResponse(c, response)
}

func writePinnedOpenAIModel(c *gin.Context, body []byte, requestedModel string) {
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeOpenAIModelsError(c, http.StatusBadGateway, "upstream_error", "Invalid OpenAI model list")
		return
	}
	for _, raw := range envelope.Data {
		var entry struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &entry); err == nil && entry.ID == requestedModel {
			c.Data(http.StatusOK, "application/json", raw)
			return
		}
	}
	writeModelNotFoundResponse(c, requestedModel)
}

func writeOpenAIModelsError(c *gin.Context, status int, errorType, message string) {
	c.JSON(status, gin.H{"error": gin.H{"type": errorType, "message": message}})
}

func writeOpenAIModelsResponse(c *gin.Context, manifest *service.OpenAIModelsResponse) {
	if manifest.ETag != "" {
		c.Header("ETag", manifest.ETag)
	}
	if manifest.NotModified {
		c.Status(http.StatusNotModified)
		c.Writer.WriteHeaderNow()
		return
	}
	c.Data(http.StatusOK, "application/json", manifest.Body)
}
