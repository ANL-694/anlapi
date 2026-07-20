package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"anlapi/internal/handler/quotaview"
	"anlapi/internal/pkg/response"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
)

type UpdateUserPlatformQuotasRequest struct {
	Quotas []PlatformQuotaInput `json:"quotas" binding:"required"`
}

type PlatformQuotaInput struct {
	Platform        string   `json:"platform" binding:"required"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
}

type ResetUserPlatformQuotaWindowRequest struct {
	Platform string `json:"platform" binding:"required"`
	Window   string `json:"window" binding:"required"`
}

var allowedWindowsForQuotaReset = map[string]struct{}{
	"daily": {}, "weekly": {}, "monthly": {},
}

func (h *UserHandler) GetUserPlatformQuotas(c *gin.Context) {
	userID, ok := parsePlatformQuotaUserID(c)
	if !ok {
		return
	}
	if h.userPlatformQuotaRepo == nil {
		response.Success(c, map[string]any{"platform_quotas": []any{}})
		return
	}
	if _, err := h.adminService.GetUser(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	records, err := h.userPlatformQuotaRepo.ListByUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"platform_quotas": platformQuotaResponse(records, time.Now().UTC(), true)})
}

func (h *UserHandler) UpdateUserPlatformQuotas(c *gin.Context) {
	if h.userPlatformQuotaRepo == nil {
		response.Error(c, 503, "platform quota service not available")
		return
	}
	userID, ok := parsePlatformQuotaUserID(c)
	if !ok {
		return
	}
	var req UpdateUserPlatformQuotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := validatePlatformQuotaInputs(req.Quotas); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	ctx := c.Request.Context()
	if _, err := h.adminService.GetUser(ctx, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	before, beforeErr := h.userPlatformQuotaRepo.ListByUser(ctx, userID)
	if beforeErr != nil {
		slog.Warn("quota audit before snapshot failed", "user_id", userID, "err", beforeErr)
	}
	records := make([]service.UserPlatformQuotaRecord, 0, len(req.Quotas))
	for _, quota := range req.Quotas {
		records = append(records, service.UserPlatformQuotaRecord{
			UserID: userID, Platform: quota.Platform,
			DailyLimitUSD: quota.DailyLimitUSD, WeeklyLimitUSD: quota.WeeklyLimitUSD, MonthlyLimitUSD: quota.MonthlyLimitUSD,
		})
	}
	if err := h.userPlatformQuotaRepo.UpsertForUser(ctx, userID, records); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	slog.Info("admin.quota_updated",
		"actor_admin_id", getAdminIDFromContext(c),
		"target_user_id", userID,
		"platform_count", len(records),
		"before_snapshot_available", beforeErr == nil,
		"changes", buildPlatformQuotaAuditChanges(before, records),
	)
	h.invalidateUserPlatformQuotaCache(ctx, userID, service.AllowedQuotaPlatforms)

	updated, err := h.userPlatformQuotaRepo.ListByUser(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"platform_quotas": platformQuotaResponse(updated, time.Now().UTC(), true)})
}

func (h *UserHandler) ResetUserPlatformQuotaWindow(c *gin.Context) {
	if h.userPlatformQuotaRepo == nil {
		response.Error(c, 503, "platform quota service not available")
		return
	}
	userID, ok := parsePlatformQuotaUserID(c)
	if !ok {
		return
	}
	var req ResetUserPlatformQuotaWindowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !service.IsAllowedQuotaPlatform(req.Platform) {
		response.BadRequest(c, "invalid platform: "+req.Platform)
		return
	}
	if _, valid := allowedWindowsForQuotaReset[req.Window]; !valid {
		response.BadRequest(c, "invalid window: "+req.Window)
		return
	}

	ctx := c.Request.Context()
	if _, err := h.adminService.GetUser(ctx, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	now := time.Now().UTC()
	if err := h.userPlatformQuotaRepo.ResetExpiredWindow(ctx, userID, req.Platform, req.Window, now); err != nil {
		if errors.Is(err, service.ErrUserPlatformQuotaNotFound) {
			response.NotFound(c, "user platform quota not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	slog.Info("admin.quota_window_reset",
		"actor_admin_id", getAdminIDFromContext(c),
		"target_user_id", userID,
		"platform", req.Platform,
		"window", req.Window,
	)
	h.invalidateUserPlatformQuotaCache(ctx, userID, []string{req.Platform})

	records, err := h.userPlatformQuotaRepo.ListByUser(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"platform_quotas": platformQuotaResponse(records, now, true)})
}

func parsePlatformQuotaUserID(c *gin.Context) (int64, bool) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return 0, false
	}
	return userID, true
}

func validatePlatformQuotaInputs(quotas []PlatformQuotaInput) error {
	if len(quotas) > len(service.AllowedQuotaPlatforms) {
		return fmt.Errorf("quotas length must be <= %d", len(service.AllowedQuotaPlatforms))
	}
	seen := make(map[string]struct{}, len(quotas))
	for _, quota := range quotas {
		if !service.IsAllowedQuotaPlatform(quota.Platform) {
			return fmt.Errorf("invalid platform: %s", quota.Platform)
		}
		if _, exists := seen[quota.Platform]; exists {
			return fmt.Errorf("duplicate platform: %s", quota.Platform)
		}
		seen[quota.Platform] = struct{}{}
		limits := []struct {
			name  string
			value *float64
		}{
			{"daily_limit_usd", quota.DailyLimitUSD},
			{"weekly_limit_usd", quota.WeeklyLimitUSD},
			{"monthly_limit_usd", quota.MonthlyLimitUSD},
		}
		for _, limit := range limits {
			if limit.value == nil {
				continue
			}
			if *limit.value < 0 {
				return fmt.Errorf("%s must be >= 0", limit.name)
			}
			if math.IsNaN(*limit.value) || math.IsInf(*limit.value, 0) {
				return fmt.Errorf("%s must be a finite number", limit.name)
			}
		}
	}
	return nil
}

func (h *UserHandler) invalidateUserPlatformQuotaCache(ctx context.Context, userID int64, platforms []string) {
	if h.billingCache == nil {
		return
	}
	for _, platform := range platforms {
		if err := h.billingCache.DeleteUserPlatformQuotaCache(ctx, userID, platform); err != nil {
			slog.Error("ALERT: quota cache invalidation failed", "user_id", userID, "platform", platform, "err", err)
		}
	}
}

func platformQuotaResponse(records []service.UserPlatformQuotaRecord, now time.Time, includeWindowStart bool) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, quotaview.LazyZeroQuotaForResponse(record, now, includeWindowStart))
	}
	return out
}

func buildPlatformQuotaAuditChanges(before, after []service.UserPlatformQuotaRecord) []map[string]any {
	beforeByPlatform := make(map[string]service.UserPlatformQuotaRecord, len(before))
	for _, record := range before {
		beforeByPlatform[record.Platform] = record
	}
	kept := make(map[string]struct{}, len(after))
	changes := make([]map[string]any, 0, len(before)+len(after))
	for _, record := range after {
		kept[record.Platform] = struct{}{}
		entry := map[string]any{
			"platform": record.Platform, "daily_limit_usd": record.DailyLimitUSD,
			"weekly_limit_usd": record.WeeklyLimitUSD, "monthly_limit_usd": record.MonthlyLimitUSD,
		}
		if previous, exists := beforeByPlatform[record.Platform]; exists {
			entry["before_daily_limit_usd"] = previous.DailyLimitUSD
			entry["before_weekly_limit_usd"] = previous.WeeklyLimitUSD
			entry["before_monthly_limit_usd"] = previous.MonthlyLimitUSD
		}
		changes = append(changes, entry)
	}
	for _, previous := range before {
		if _, exists := kept[previous.Platform]; exists {
			continue
		}
		changes = append(changes, map[string]any{
			"platform": previous.Platform, "removed": true,
			"before_daily_limit_usd":   previous.DailyLimitUSD,
			"before_weekly_limit_usd":  previous.WeeklyLimitUSD,
			"before_monthly_limit_usd": previous.MonthlyLimitUSD,
		})
	}
	return changes
}
