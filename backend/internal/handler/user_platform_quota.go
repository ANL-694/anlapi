package handler

import (
	"time"

	"anl-api/internal/handler/quotaview"
	"anl-api/internal/pkg/response"
	middleware2 "anl-api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// GetMyPlatformQuotas 返回当前用户的平台配额状态。
func (h *UserHandler) GetMyPlatformQuotas(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h.userPlatformQuotaRepo == nil {
		response.Success(c, map[string]any{"platform_quotas": []any{}})
		return
	}
	records, err := h.userPlatformQuotaRepo.ListByUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	now := time.Now().UTC()
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, quotaview.LazyZeroQuotaForResponse(record, now, false))
	}
	response.Success(c, map[string]any{"platform_quotas": out})
}
