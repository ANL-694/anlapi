package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AccountSharePolicyHandler struct {
	service *service.AccountSharePolicyService
}

func NewAccountSharePolicyHandler(policyService *service.AccountSharePolicyService) *AccountSharePolicyHandler {
	return &AccountSharePolicyHandler{service: policyService}
}

type createAccountSharePolicyRequest struct {
	ScopeType        string     `json:"scope_type"`
	ScopeID          *int64     `json:"scope_id"`
	Platform         *string    `json:"platform"`
	OwnerShareRatio  *float64   `json:"owner_share_ratio" binding:"required,gte=0,lte=1"`
	InviteShareRatio *float64   `json:"invite_share_ratio" binding:"omitempty,gte=0,lte=1"`
	Enabled          *bool      `json:"enabled"`
	EffectiveAt      *time.Time `json:"effective_at"`
}

type updateAccountSharePolicyRequest struct {
	ScopeType        *string    `json:"scope_type"`
	ScopeID          *int64     `json:"scope_id"`
	Platform         *string    `json:"platform"`
	OwnerShareRatio  *float64   `json:"owner_share_ratio" binding:"omitempty,gte=0,lte=1"`
	InviteShareRatio *float64   `json:"invite_share_ratio" binding:"omitempty,gte=0,lte=1"`
	Enabled          *bool      `json:"enabled"`
	EffectiveAt      *time.Time `json:"effective_at"`
}

func (h *AccountSharePolicyHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "effective_at", SortOrder: "desc"}
	filters := service.AccountSharePolicyFilters{
		ScopeType: strings.TrimSpace(c.Query("scope_type")),
		Platform:  strings.TrimSpace(c.Query("platform")),
	}
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "enabled must be a boolean")
			return
		}
		filters.Enabled = &enabled
	}
	items, result, err := h.service.List(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, result.Total, page, pageSize)
}

func (h *AccountSharePolicyHandler) GetByID(c *gin.Context) {
	id, ok := parseAccountSharePolicyID(c)
	if !ok {
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *AccountSharePolicyHandler) Create(c *gin.Context) {
	var req createAccountSharePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	var adminID *int64
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		adminID = &subject.UserID
	}
	item, err := h.service.Create(c.Request.Context(), service.CreateAccountSharePolicyInput{
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		Platform:         req.Platform,
		OwnerShareRatio:  *req.OwnerShareRatio,
		InviteShareRatio: valueOrZeroShareRatio(req.InviteShareRatio),
		Enabled:          req.Enabled,
		EffectiveAt:      req.EffectiveAt,
		CreatedByAdminID: adminID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, item)
}

func (h *AccountSharePolicyHandler) Update(c *gin.Context) {
	id, ok := parseAccountSharePolicyID(c)
	if !ok {
		return
	}
	var req updateAccountSharePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, service.UpdateAccountSharePolicyInput{
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		Platform:         req.Platform,
		OwnerShareRatio:  req.OwnerShareRatio,
		InviteShareRatio: req.InviteShareRatio,
		Enabled:          req.Enabled,
		EffectiveAt:      req.EffectiveAt,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *AccountSharePolicyHandler) Delete(c *gin.Context) {
	id, ok := parseAccountSharePolicyID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func parseAccountSharePolicyID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid policy id")
		return 0, false
	}
	return id, true
}

func valueOrZeroShareRatio(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
