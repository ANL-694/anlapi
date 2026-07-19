package admin

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	infraerrors "ikik-api/internal/pkg/errors"
	"ikik-api/internal/pkg/response"
	"ikik-api/internal/service"
)

type ApplyOAuthCredentialsRequest struct {
	Type        string         `json:"type" binding:"required,oneof=oauth setup-token"`
	Credentials map[string]any `json:"credentials" binding:"required"`
	Extra       map[string]any `json:"extra"`
}

func (h *AccountHandler) ApplyOAuthCredentials(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req ApplyOAuthCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	existing, err := h.adminService.GetAccount(ctx, accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}
	if !existing.IsOAuth() {
		response.ErrorFrom(c, infraerrors.BadRequest("NOT_OAUTH", "cannot apply oauth credentials to non-OAuth account"))
		return
	}
	if err := service.ValidateOpenAILongContextBillingExtra(existing.Platform, req.Extra); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	updatedAccount, err := h.adminService.UpdateAccount(ctx, accountID, &service.UpdateAccountInput{
		Type:        req.Type,
		Credentials: req.Credentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if len(req.Extra) > 0 {
		if extraErr := h.adminService.UpdateAccountExtra(ctx, accountID, req.Extra); extraErr != nil {
			slog.Error("apply_oauth_credentials.update_extra_failed", "account_id", accountID, "err", extraErr)
		}
	}
	if cleared, clearErr := h.adminService.ClearAccountError(ctx, accountID); clearErr != nil {
		slog.Warn("apply_oauth_credentials.clear_error_failed", "account_id", accountID, "err", clearErr)
	} else if cleared != nil {
		updatedAccount = cleared
	}
	if h.tokenCacheInvalidator != nil && updatedAccount.IsOAuth() {
		if invalidateErr := h.tokenCacheInvalidator.InvalidateToken(ctx, updatedAccount); invalidateErr != nil {
			slog.Warn("apply_oauth_credentials.invalidate_token_failed", "account_id", accountID, "err", invalidateErr)
		}
	}
	response.Success(c, h.buildAccountResponseWithRuntime(ctx, updatedAccount))
}
