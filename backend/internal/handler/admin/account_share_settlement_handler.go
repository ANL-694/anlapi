package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AccountShareSettlementHandler struct {
	service *service.AccountShareSettlementService
}

func NewAccountShareSettlementHandler(settlementService *service.AccountShareSettlementService) *AccountShareSettlementHandler {
	return &AccountShareSettlementHandler{service: settlementService}
}

func (h *AccountShareSettlementHandler) List(c *gin.Context) {
	startTime, endTime, ok := parseAccountShareSettlementRange(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err := h.service.List(c.Request.Context(), service.AccountShareSettlementQuery{
		StartTime: startTime,
		EndTime:   endTime,
		Page:      page,
		PageSize:  pageSize,
		Search:    c.Query("search"),
		Status:    c.Query("status"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func parseAccountShareSettlementRange(c *gin.Context) (time.Time, time.Time, bool) {
	location := time.UTC
	if raw := strings.TrimSpace(c.Query("timezone")); raw != "" {
		if parsed, err := time.LoadLocation(raw); err == nil {
			location = parsed
		}
	}
	now := time.Now().In(location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -6)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
	if raw := strings.TrimSpace(c.Query("start_date")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			response.BadRequest(c, "start_date must use YYYY-MM-DD")
			return time.Time{}, time.Time{}, false
		}
		start = parsed
	}
	if raw := strings.TrimSpace(c.Query("end_date")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			response.BadRequest(c, "end_date must use YYYY-MM-DD")
			return time.Time{}, time.Time{}, false
		}
		end = parsed.AddDate(0, 0, 1)
	}
	if !start.Before(end) {
		response.BadRequest(c, "end_date must be on or after start_date")
		return time.Time{}, time.Time{}, false
	}
	if end.Sub(start) > 31*24*time.Hour {
		response.BadRequest(c, "account share settlement range cannot exceed 31 days")
		return time.Time{}, time.Time{}, false
	}
	return start.UTC(), end.UTC(), true
}

func parseAccountShareSettlementID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid settlement id")
		return 0, false
	}
	return id, true
}
