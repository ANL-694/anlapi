package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const userAccountDataPageCap = 1000

func (h *UserAccountHandler) ExportData(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}

	selectedIDs, err := parseUserAccountDataIDs(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	accounts, err := h.resolveOwnedExportAccounts(c, subject.UserID, selectedIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// The user export is intentionally account-scoped. Private proxy records
	// are not exported here, so the endpoint cannot disclose another object's
	// proxy credentials or turn a proxy ID into a cross-user lookup primitive.
	response.Success(c, service.BuildAccountDataPayload(accounts, nil, buildUserAccountDataProxyKey))
}

func (h *UserAccountHandler) resolveOwnedExportAccounts(c *gin.Context, ownerUserID int64, ids []int64) ([]service.Account, error) {
	if len(ids) > 0 {
		out := make([]service.Account, 0, len(ids))
		for _, id := range ids {
			account, err := h.accountService.GetOwnedByID(c.Request.Context(), ownerUserID, id)
			if err != nil {
				return nil, err
			}
			out = append(out, *account)
		}
		return out, nil
	}

	filters, err := parseUserAccountDataFilters(c)
	if err != nil {
		return nil, err
	}
	page := 1
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	out := make([]service.Account, 0)
	for {
		accounts, result, err := h.accountService.ListOwned(c.Request.Context(), ownerUserID, pagination.PaginationParams{
			Page:      page,
			PageSize:  userAccountDataPageCap,
			SortBy:    sortBy,
			SortOrder: sortOrder,
		}, filters)
		if err != nil {
			return nil, err
		}
		out = append(out, accounts...)
		if result == nil || len(out) >= int(result.Total) || len(accounts) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func parseUserAccountDataFilters(c *gin.Context) (service.AccountListFilters, error) {
	filters := service.AccountListFilters{
		Platform:    strings.TrimSpace(c.Query("platform")),
		AccountType: strings.TrimSpace(c.Query("type")),
		Status:      strings.TrimSpace(c.Query("status")),
		Search:      strings.TrimSpace(c.Query("search")),
		PrivacyMode: strings.TrimSpace(c.Query("privacy_mode")),
	}
	if len(filters.Search) > 100 {
		filters.Search = filters.Search[:100]
	}
	if groupIDValue := strings.TrimSpace(c.Query("group_id")); groupIDValue != "" {
		groupID, err := strconv.ParseInt(groupIDValue, 10, 64)
		if err != nil || groupID <= 0 {
			return filters, errors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter")
		}
		filters.GroupID = groupID
	}
	return filters, nil
}

func parseUserAccountDataIDs(c *gin.Context) ([]int64, error) {
	values := c.QueryArray("ids")
	if len(values) == 0 {
		if raw := strings.TrimSpace(c.Query("ids")); raw != "" {
			values = []string{raw}
		}
	}
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid account id: %s", part)
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func buildUserAccountDataProxyKey(protocol, host string, port int, username, password string) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", strings.TrimSpace(protocol), strings.TrimSpace(host), port, strings.TrimSpace(username), strings.TrimSpace(password))
}
