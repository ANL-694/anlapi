package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/accountmodels"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserAccountHandler exposes the user-owned account surface. It deliberately
// has no admin service dependency: ownership comes from the authenticated JWT
// subject and is enforced again by the repository query.
type UserAccountHandler struct {
	accountService          *service.AccountService
	accountUsageService     *service.AccountUsageService
	accountTestService      *service.AccountTestService
	oauthService            *service.OAuthService
	openaiOAuthService      *service.OpenAIOAuthService
	geminiOAuthService      *service.GeminiOAuthService
	antigravityOAuthService *service.AntigravityOAuthService
	grokOAuthService        *service.GrokOAuthService
	accountBatchTaskService *service.AccountBatchTaskService
	carpoolService          *service.CarpoolService
	settingService          *service.SettingService
}

func NewUserAccountHandler(accountService *service.AccountService, accountUsageService *service.AccountUsageService, accountTestService *service.AccountTestService) *UserAccountHandler {
	return &UserAccountHandler{
		accountService:      accountService,
		accountUsageService: accountUsageService,
		accountTestService:  accountTestService,
	}
}

func (h *UserAccountHandler) SetCarpoolService(carpoolService *service.CarpoolService) {
	if h == nil {
		return
	}
	h.carpoolService = carpoolService
}

func (h *UserAccountHandler) SetSettingService(settingService *service.SettingService) {
	if h == nil {
		return
	}
	h.settingService = settingService
}

func (h *UserAccountHandler) ensureFreeModelsEnabled(ctx context.Context, extra map[string]any) error {
	if _, markedAsFreeModel := extra["free_model_provider"]; !markedAsFreeModel {
		return nil
	}
	if h.settingService == nil || !h.settingService.IsFreeModelsEnabled(ctx) {
		return service.ErrFreeModelsDisabled
	}
	return nil
}

func bindOptionalJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		response.BadRequest(c, "Invalid request: "+err.Error())
		return false
	}
	return true
}

// SetOAuthServices supplies the current official OAuth implementations after
// the handler is constructed. Keeping this as a setter avoids widening the
// baseline constructor used by focused handler tests.
func (h *UserAccountHandler) SetOAuthServices(
	oauthService *service.OAuthService,
	openaiOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
) {
	if h == nil {
		return
	}
	h.oauthService = oauthService
	h.openaiOAuthService = openaiOAuthService
	h.geminiOAuthService = geminiOAuthService
	h.antigravityOAuthService = antigravityOAuthService
	h.grokOAuthService = grokOAuthService
}

// SetAccountBatchTaskService attaches the durable worker after the handler's
// provider dependencies are constructed. Executors are registered before the
// recurring worker is started so a task can never be claimed with an empty
// operation registry.
func (h *UserAccountHandler) SetAccountBatchTaskService(batchService *service.AccountBatchTaskService) {
	if h == nil {
		return
	}
	h.accountBatchTaskService = batchService
	h.registerAccountBatchExecutors()
	if batchService != nil {
		batchService.Start()
	}
}

const (
	maxUserAccountStatsBatchSize = 100
	userPublicShareTestTimeout   = 30 * time.Second
)

type userAccountTestRequest struct {
	ModelID      string `json:"model_id"`
	Prompt       string `json:"prompt"`
	Mode         string `json:"mode"`
	ImageDataURL string `json:"image_data_url"`
	AudioDataURL string `json:"audio_data_url"`
}

type userBatchTodayStatsRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required"`
}

type userAccountBatchTaskRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required"`
}

type userBulkUpdateRequest struct {
	AccountIDs  []int64         `json:"account_ids" binding:"required"`
	ProxyID     *int64          `json:"proxy_id"`
	ShareMode   *string         `json:"share_mode"`
	Credentials *map[string]any `json:"credentials"`
	Extra       *map[string]any `json:"extra"`
}

func decodeStrictJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

type userBulkDeleteRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required"`
}

// userCredentialImportRequest only exposes account fields that are meaningful
// for a user's own OAuth credentials. Provider-specific tokens are parsed and
// validated server-side rather than accepted as an arbitrary account payload.
type userCredentialImportRequest struct {
	Contents         []string `json:"contents" binding:"required"`
	KiroConfigImport bool     `json:"kiro_config_import"`
	ShareMode        *string  `json:"share_mode"`
}

type userOAuthProxyRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

type userExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

type userOpenAIGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

type userOpenAIExchangeCodeRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	Code        string `json:"code" binding:"required"`
	State       string `json:"state" binding:"required"`
	RedirectURI string `json:"redirect_uri"`
	ProxyID     *int64 `json:"proxy_id"`
}

type userGeminiGenerateAuthURLRequest struct {
	ProxyID   *int64 `json:"proxy_id"`
	ProjectID string `json:"project_id"`
	OAuthType string `json:"oauth_type"`
	TierID    string `json:"tier_id"`
}

type userGeminiExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
	OAuthType string `json:"oauth_type"`
	TierID    string `json:"tier_id"`
}

type userAntigravityGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

type userAntigravityExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

type userGrokGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

type userGrokExchangeCodeRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	State       string `json:"state" binding:"required"`
	Code        string `json:"code" binding:"required"`
	RedirectURI string `json:"redirect_uri"`
	ProxyID     *int64 `json:"proxy_id"`
}

func userAccountSubject(c *gin.Context) (middleware.AuthSubject, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return middleware.AuthSubject{}, false
	}
	return subject, true
}

func bindOptionalUserJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		response.BadRequest(c, "Invalid request: "+err.Error())
		return false
	}
	return true
}

func rejectUserManualCredentialAuth(c *gin.Context) {
	response.BadRequest(c, "manual credential account creation is not allowed for user accounts; use official OAuth")
}

func (h *UserAccountHandler) resolveUserOAuthProxyID(c *gin.Context, ownerUserID int64, proxyID *int64) (*int64, bool) {
	id, err := h.accountService.ValidateOwnedProxyID(c.Request.Context(), ownerUserID, proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	return id, true
}

func deriveUserGeminiRedirectURI(c *gin.Context) string {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" {
		return strings.TrimRight(origin, "/") + "/auth/callback"
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := strings.TrimSpace(c.Request.Host)
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwarded != "" {
		host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return fmt.Sprintf("%s://%s/auth/callback", scheme, host)
}

func userAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

func (h *UserAccountHandler) getOwnedAccount(c *gin.Context) (*service.Account, bool) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return nil, false
	}
	accountID, ok := userAccountID(c)
	if !ok {
		return nil, false
	}
	account, err := h.accountService.GetOwnedByID(c.Request.Context(), subject.UserID, accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	return account, true
}

func normalizeUserAccountIDs(ids []int64) []int64 {
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func (h *UserAccountHandler) List(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	filters := service.AccountListFilters{
		Platform:    strings.TrimSpace(c.Query("platform")),
		AccountType: strings.TrimSpace(c.Query("type")),
		Status:      strings.TrimSpace(c.Query("status")),
		Search:      strings.TrimSpace(c.Query("search")),
		PrivacyMode: strings.TrimSpace(c.Query("privacy_mode")),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filters.GroupID = groupID
	}
	accounts, result, err := h.accountService.ListOwned(c.Request.Context(), subject.UserID, params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.Account, 0, len(accounts))
	for i := range accounts {
		out = append(out, *dto.AccountFromService(&accounts[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

func (h *UserAccountHandler) GetByID(c *gin.Context) {
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

func (h *UserAccountHandler) Create(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	var req service.UserAccountCreateRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.ensureFreeModelsEnabled(c.Request.Context(), req.Extra); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	account, err := h.accountService.CreateOwnedUser(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.AccountFromService(account))
}

// Import currently shares the same validated account payload as create. The
// richer credential-import formats remain a separate follow-up slice; keeping
// this alias avoids routing import requests into /:id and preserves the user
// page's existing contract for ordinary account payloads.
func (h *UserAccountHandler) Import(c *gin.Context) {
	h.Create(c)
}

// ImportCredentials imports explicitly supported OAuth credential formats into
// accounts owned by the authenticated user. Parse failures are reported per
// item so a bad credential cannot discard successfully imported siblings.
func (h *UserAccountHandler) ImportCredentials(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}

	var req userCredentialImportRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.Contents) == 0 {
		response.BadRequest(c, "contents is required")
		return
	}
	sources, parseErrors := service.ParseAccountCredentialImportContentsWithOptions(req.Contents, service.AccountCredentialImportOptions{
		KiroConfigImport:          req.KiroConfigImport,
		OpenAIAgentIdentityImport: true,
	})
	if len(sources) == 0 && len(parseErrors) == 0 {
		response.BadRequest(c, "No importable account credentials found")
		return
	}
	if len(sources) > service.MaxAccountCredentialImportItems {
		response.BadRequest(c, fmt.Sprintf("Too many import items; maximum is %d", service.MaxAccountCredentialImportItems))
		return
	}

	result := service.AccountCredentialImportResult{
		Total:  len(sources) + len(parseErrors),
		Errors: append([]service.AccountCredentialImportError{}, parseErrors...),
	}
	for index, source := range sources {
		if _, err := h.createOwnedAccountFromCredentialImportSource(c.Request.Context(), subject.UserID, source, req, index+1); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, service.AccountCredentialImportError{
				Index:   len(parseErrors) + index + 1,
				Kind:    string(source.Kind),
				Name:    source.Name,
				Message: err.Error(),
			})
			continue
		}
		result.Created++
	}
	result.Failed += len(parseErrors)
	response.Success(c, result)
}

func (h *UserAccountHandler) createOwnedAccountFromCredentialImportSource(
	ctx context.Context,
	ownerUserID int64,
	source service.AccountCredentialImportSource,
	defaults userCredentialImportRequest,
	sequence int,
) (*service.Account, error) {
	req := service.UserAccountCreateRequest{
		Name:        strings.TrimSpace(source.Name),
		Notes:       source.Notes,
		Platform:    source.Platform,
		Type:        service.AccountTypeOAuth,
		Credentials: source.Credentials,
		Extra:       source.Extra,
		ShareMode:   defaults.ShareMode,
	}

	switch source.Kind {
	case service.AccountCredentialImportKindOAuthCredentials:
		if !isSupportedUserCredentialImportPlatform(req.Platform) {
			return nil, fmt.Errorf("credential import is not supported for platform %q on the Sub2API 0.1.176 base", req.Platform)
		}
		if req.Name == "" {
			req.Name = service.DeriveAccountCredentialImportName(req.Platform, req.Credentials, req.Extra, sequence)
		}
	case service.AccountCredentialImportKindOpenAIRefreshToken:
		if h.openaiOAuthService == nil {
			return nil, errors.New("OpenAI OAuth service is not configured")
		}
		tokenInfo, err := h.openaiOAuthService.RefreshTokenWithClientID(ctx, source.Token, "", source.ClientID)
		if err != nil {
			return nil, fmt.Errorf("validate OpenAI refresh token: %w", err)
		}
		req.Platform = service.PlatformOpenAI
		req.Credentials = h.openaiOAuthService.BuildAccountCredentials(tokenInfo)
		req.Extra = service.BuildOpenAIAccountCredentialImportExtra(tokenInfo)
		if req.Name == "" {
			req.Name = strings.TrimSpace(tokenInfo.Email)
		}
		if req.Name == "" {
			req.Name = service.DeriveAccountCredentialImportName(req.Platform, req.Credentials, req.Extra, sequence)
		}
	case service.AccountCredentialImportKindOpenAIAgentIdentity:
		req.Platform = service.PlatformOpenAI
		if req.Name == "" {
			req.Name = service.DeriveAccountCredentialImportName(req.Platform, req.Credentials, req.Extra, sequence)
		}
	case service.AccountCredentialImportKindClaudeSessionKey:
		if h.oauthService == nil {
			return nil, errors.New("Anthropic OAuth service is not configured")
		}
		tokenInfo, err := h.oauthService.CookieAuth(ctx, &service.CookieAuthInput{
			SessionKey: source.Token,
			Scope:      "full",
		})
		if err != nil {
			return nil, fmt.Errorf("exchange Claude session key: %w", err)
		}
		req.Platform = service.PlatformAnthropic
		req.Credentials = service.BuildClaudeAccountCredentials(tokenInfo)
		req.Extra = service.BuildClaudeAccountCredentialImportExtra(tokenInfo)
		if req.Name == "" {
			req.Name = strings.TrimSpace(tokenInfo.EmailAddress)
		}
		if req.Name == "" {
			req.Name = service.DeriveAccountCredentialImportName(req.Platform, req.Credentials, req.Extra, sequence)
		}
	case service.AccountCredentialImportKindKiroConfig:
		return nil, errors.New("Kiro configuration import is not supported by the Sub2API 0.1.176 base")
	default:
		return nil, fmt.Errorf("unsupported credential import kind %q", source.Kind)
	}

	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("account name is required")
	}
	return h.accountService.CreateOwnedUser(ctx, ownerUserID, req)
}

func isSupportedUserCredentialImportPlatform(platform string) bool {
	switch platform {
	case service.PlatformAnthropic, service.PlatformOpenAI, service.PlatformGemini, service.PlatformAntigravity, service.PlatformGrok:
		return true
	default:
		return false
	}
}

func (h *UserAccountHandler) Update(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req service.UserAccountUpdateRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Extra != nil {
		if err := h.ensureFreeModelsEnabled(c.Request.Context(), *req.Extra); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	account, err := h.accountService.UpdateOwnedUser(c.Request.Context(), subject.UserID, id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// BulkUpdate applies only user-safe fields and returns a per-account result.
// Admin-only billing fields are rejected explicitly instead of being silently
// ignored, which keeps the user/admin contracts visibly separate.
func (h *UserAccountHandler) BulkUpdate(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	var req userBulkUpdateRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Extra != nil {
		if err := h.ensureFreeModelsEnabled(c.Request.Context(), *req.Extra); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	result, err := h.accountService.BulkUpdateOwnedUser(c.Request.Context(), subject.UserID, &service.BulkUpdateOwnedUserAccountsInput{
		AccountIDs:  req.AccountIDs,
		ProxyID:     req.ProxyID,
		ShareMode:   req.ShareMode,
		Credentials: req.Credentials,
		Extra:       req.Extra,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) Delete(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if err := h.accountService.DeleteOwned(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "account deleted"})
}

func (h *UserAccountHandler) BulkDelete(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	var req userBulkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.accountService.BulkDeleteOwned(c.Request.Context(), subject.UserID, req.AccountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetUsage returns usage information only after the account has been resolved
// through the authenticated user's ownership scope.
func (h *UserAccountHandler) GetUsage(c *gin.Context) {
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "Account usage service is not configured")
		return
	}

	force := c.Query("force") == "true"
	source := c.DefaultQuery("source", "active")
	var usage *service.UsageInfo
	var err error
	if source == "passive" {
		usage, err = h.accountUsageService.GetPassiveUsage(c.Request.Context(), account.ID)
	} else {
		usage, err = h.accountUsageService.GetUsage(c.Request.Context(), account.ID, force)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, usage)
}

// GetStats exposes the same bounded rolling usage statistics as the official
// admin endpoint, constrained to a user-owned account.
func (h *UserAccountHandler) GetStats(c *gin.Context) {
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "Account usage service is not configured")
		return
	}

	days := 30
	if daysValue := c.Query("days"); daysValue != "" {
		if parsed, err := strconv.Atoi(daysValue); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}
	now := timezone.Now()
	endTime := timezone.StartOfDay(now.AddDate(0, 0, 1))
	startTime := timezone.StartOfDay(now.AddDate(0, 0, -days+1))
	stats, err := h.accountUsageService.GetAccountUsageStats(c.Request.Context(), account.ID, startTime, endTime)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// GetQuotaPoolDashboard returns the caller's own account pool and the
// explicitly public shared pool; it never exposes another user's private pool.
func (h *UserAccountHandler) GetQuotaPoolDashboard(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	dashboard, err := h.accountService.GetQuotaPoolDashboard(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dashboard)
}

func (h *UserAccountHandler) GetTodayStats(c *gin.Context) {
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "Account usage service is not configured")
		return
	}
	stats, err := h.accountUsageService.GetTodayStats(c.Request.Context(), account.ID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// GetBatchTodayStats validates every requested account before executing the
// shared batched query. A mixed-owner request therefore leaks neither account
// existence nor another user's usage data.
func (h *UserAccountHandler) GetBatchTodayStats(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "Account usage service is not configured")
		return
	}

	var req userBatchTodayStatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	accountIDs := normalizeUserAccountIDs(req.AccountIDs)
	if len(accountIDs) == 0 {
		response.Success(c, gin.H{"stats": map[string]any{}})
		return
	}
	if len(accountIDs) > maxUserAccountStatsBatchSize {
		response.BadRequest(c, "Too many account IDs")
		return
	}
	for _, accountID := range accountIDs {
		if _, err := h.accountService.GetOwnedByID(c.Request.Context(), subject.UserID, accountID); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	stats, err := h.accountUsageService.GetTodayStatsBatch(c.Request.Context(), accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"stats": stats})
}

func (h *UserAccountHandler) GetAvailableModels(c *gin.Context) {
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	response.Success(c, accountmodels.ForAccount(account))
}

// Test preserves the official SSE test stream while making the target account
// invisible to callers other than its durable owner.
func (h *UserAccountHandler) Test(c *gin.Context) {
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	if h.accountTestService == nil {
		response.InternalError(c, "Account test service is not configured")
		return
	}

	var req userAccountTestRequest
	_ = c.ShouldBindJSON(&req)
	opts := service.AccountTestOptions{
		ImageDataURL: req.ImageDataURL,
		AudioDataURL: req.AudioDataURL,
	}
	if err := h.accountTestService.TestAccountConnection(c, account.ID, req.ModelID, req.Prompt, req.Mode, opts); err != nil {
		return
	}
}

// refreshOwnedAccount reuses the provider-specific refresh implementations
// from the official code path, while the final persistence operation remains
// owner-scoped. It intentionally mirrors the admin refresh semantics without
// routing a user request through an admin service.
func (h *UserAccountHandler) refreshOwnedAccount(ctx context.Context, ownerUserID int64, account *service.Account) (*service.Account, string, error) {
	if account == nil {
		return nil, "", service.ErrAccountNotFound
	}
	if !account.IsOAuth() {
		return nil, "", errors.New("cannot refresh non-OAuth account")
	}
	if account.IsCredentialShadow() {
		return nil, "", errors.New("cannot refresh credential shadow account")
	}

	var newCredentials map[string]any
	warning := ""
	switch account.Platform {
	case service.PlatformOpenAI:
		if h.openaiOAuthService == nil {
			return nil, "", errors.New("OpenAI OAuth service is not configured")
		}
		tokenInfo, err := h.openaiOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		newCredentials = h.openaiOAuthService.BuildAccountCredentials(tokenInfo)
		for key, value := range account.Credentials {
			if _, exists := newCredentials[key]; !exists {
				newCredentials[key] = value
			}
		}
		newCredentials = service.NormalizeOpenAIPersonalAccessTokenCredentials(account, tokenInfo, newCredentials)
	case service.PlatformGemini:
		if h.geminiOAuthService == nil {
			return nil, "", errors.New("Gemini OAuth service is not configured")
		}
		tokenInfo, err := h.geminiOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		newCredentials = h.geminiOAuthService.BuildAccountCredentials(tokenInfo)
		for key, value := range account.Credentials {
			if _, exists := newCredentials[key]; !exists {
				newCredentials[key] = value
			}
		}
	case service.PlatformAntigravity:
		if h.antigravityOAuthService == nil {
			return nil, "", errors.New("Antigravity OAuth service is not configured")
		}
		tokenInfo, err := h.antigravityOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		newCredentials = h.antigravityOAuthService.BuildAccountCredentials(tokenInfo)
		for key, value := range account.Credentials {
			if _, exists := newCredentials[key]; !exists {
				newCredentials[key] = value
			}
		}
		if projectID, _ := newCredentials["project_id"].(string); strings.TrimSpace(projectID) == "" {
			if oldProjectID := strings.TrimSpace(account.GetCredential("project_id")); oldProjectID != "" {
				newCredentials["project_id"] = oldProjectID
			}
		}
		if tokenInfo.ProjectIDMissing {
			warning = "missing_project_id_temporary"
		}
	case service.PlatformGrok:
		if h.grokOAuthService == nil {
			return nil, "", errors.New("Grok OAuth service is not configured")
		}
		tokenInfo, err := h.grokOAuthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		newCredentials = service.MergeCredentials(account.Credentials, h.grokOAuthService.BuildAccountCredentials(tokenInfo))
		if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
			newCredentials["base_url"] = baseURL
		}
	default:
		if h.oauthService == nil {
			return nil, "", errors.New("OAuth service is not configured")
		}
		tokenInfo, err := h.oauthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		newCredentials = make(map[string]any, len(account.Credentials)+6)
		for key, value := range account.Credentials {
			newCredentials[key] = value
		}
		newCredentials["access_token"] = tokenInfo.AccessToken
		newCredentials["token_type"] = tokenInfo.TokenType
		newCredentials["expires_in"] = strconv.FormatInt(tokenInfo.ExpiresIn, 10)
		newCredentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
		if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
			newCredentials["refresh_token"] = tokenInfo.RefreshToken
		}
		if strings.TrimSpace(tokenInfo.Scope) != "" {
			newCredentials["scope"] = tokenInfo.Scope
		}
	}

	updated, err := h.accountService.UpdateOwned(ctx, ownerUserID, account.ID, service.UpdateAccountRequest{Credentials: &newCredentials})
	if err != nil {
		return nil, "", err
	}
	return updated, warning, nil
}

func (h *UserAccountHandler) Refresh(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	updated, warning, err := h.refreshOwnedAccount(c.Request.Context(), subject.UserID, account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if warning != "" {
		response.Success(c, gin.H{
			"account": dto.AccountFromService(updated),
			"message": "Token refreshed successfully, but project_id could not be retrieved (will retry automatically)",
			"warning": warning,
		})
		return
	}
	response.Success(c, gin.H{"account": dto.AccountFromService(updated)})
}

// revalidateOwnedPublicShare runs the existing official connection probe before
// changing the user-visible public-share status. A failed probe is a normal
// validation outcome: the account remains public but pending, rather than
// being marked approved or returning an opaque server error.
func (h *UserAccountHandler) revalidateOwnedPublicShare(ctx context.Context, ownerUserID int64, account *service.Account) (*service.Account, error) {
	if account == nil {
		return nil, service.ErrAccountNotFound
	}
	if service.NormalizeAccountShareMode(account.ShareMode) != service.AccountShareModePublic {
		return nil, fmt.Errorf("only public shared accounts can be revalidated")
	}
	if h.accountTestService == nil {
		return nil, fmt.Errorf("account test service is not configured")
	}

	testCtx, cancel := context.WithTimeout(ctx, userPublicShareTestTimeout)
	defer cancel()
	result, err := h.accountTestService.RunTestBackground(testCtx, account.ID, "")
	if err != nil || result == nil || strings.TrimSpace(result.Status) != "success" {
		return h.accountService.SetOwnedPublicShareStatus(
			ctx,
			ownerUserID,
			account.ID,
			service.AccountShareStatusPending,
			"account connectivity validation failed",
		)
	}
	return h.accountService.SetOwnedPublicShareStatus(ctx, ownerUserID, account.ID, service.AccountShareStatusApproved, "")
}

func (h *UserAccountHandler) registerAccountBatchExecutors() {
	if h == nil || h.accountBatchTaskService == nil {
		return
	}
	h.accountBatchTaskService.RegisterExecutor(service.AccountBatchTaskOperationUserRefreshCredentials, h.executeUserRefreshCredentialsTaskItem)
	h.accountBatchTaskService.RegisterExecutor(service.AccountBatchTaskOperationUserRevalidateShare, h.executeUserRevalidatePublicShareTaskItem)
}

func (h *UserAccountHandler) executeUserRefreshCredentialsTaskItem(ctx context.Context, task *service.AccountBatchTask, item service.AccountBatchTaskItem) (map[string]any, error) {
	if task == nil || task.OwnerUserID == nil {
		return nil, service.ErrAccountNotFound
	}
	account, err := h.accountService.GetOwnedByID(ctx, *task.OwnerUserID, item.AccountID)
	if err != nil {
		return nil, err
	}
	updated, warning, err := h.refreshOwnedAccount(ctx, *task.OwnerUserID, account)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"account_id": updated.ID}
	if strings.TrimSpace(warning) != "" {
		result["warning"] = warning
	}
	return result, nil
}

func (h *UserAccountHandler) executeUserRevalidatePublicShareTaskItem(ctx context.Context, task *service.AccountBatchTask, item service.AccountBatchTaskItem) (map[string]any, error) {
	if task == nil || task.OwnerUserID == nil {
		return nil, service.ErrAccountNotFound
	}
	account, err := h.accountService.GetOwnedByID(ctx, *task.OwnerUserID, item.AccountID)
	if err != nil {
		return nil, err
	}
	updated, err := h.revalidateOwnedPublicShare(ctx, *task.OwnerUserID, account)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"account_id":   updated.ID,
		"share_status": updated.ShareStatus,
	}, nil
}

func (h *UserAccountHandler) createOwnedAccountBatchTask(c *gin.Context, operation string, requirePublicShare bool) (*service.AccountBatchTask, bool) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return nil, false
	}
	if h.accountBatchTaskService == nil {
		response.Error(c, 503, "Account batch task service is unavailable")
		return nil, false
	}
	var req userAccountBatchTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return nil, false
	}
	accountIDs := normalizeUserAccountIDs(req.AccountIDs)
	if len(accountIDs) == 0 {
		response.BadRequest(c, "account_ids is required")
		return nil, false
	}
	for _, accountID := range accountIDs {
		account, err := h.accountService.GetOwnedByID(c.Request.Context(), subject.UserID, accountID)
		if err != nil {
			response.ErrorFrom(c, err)
			return nil, false
		}
		if requirePublicShare && service.NormalizeAccountShareMode(account.ShareMode) != service.AccountShareModePublic {
			response.BadRequest(c, "Only public shared accounts can be revalidated")
			return nil, false
		}
	}
	ownerUserID := subject.UserID
	task, err := h.accountBatchTaskService.CreateTask(c.Request.Context(), service.CreateAccountBatchTaskInput{
		Scope:       service.AccountBatchTaskScopeUser,
		Operation:   operation,
		AccountIDs:  accountIDs,
		CreatedBy:   subject.UserID,
		OwnerUserID: &ownerUserID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	return task, true
}

func (h *UserAccountHandler) RevalidatePublicShare(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	updated, err := h.revalidateOwnedPublicShare(c.Request.Context(), subject.UserID, account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updated))
}

func (h *UserAccountHandler) CreateBatchRefreshTask(c *gin.Context) {
	task, ok := h.createOwnedAccountBatchTask(c, service.AccountBatchTaskOperationUserRefreshCredentials, false)
	if !ok {
		return
	}
	response.Accepted(c, task)
}

func (h *UserAccountHandler) CreateBatchRevalidatePublicShareTask(c *gin.Context) {
	task, ok := h.createOwnedAccountBatchTask(c, service.AccountBatchTaskOperationUserRevalidateShare, true)
	if !ok {
		return
	}
	response.Accepted(c, task)
}

func (h *UserAccountHandler) GetBatchTask(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	if h.accountBatchTaskService == nil {
		response.Error(c, 503, "Account batch task service is unavailable")
		return
	}
	taskID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || taskID <= 0 {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	task, err := h.accountBatchTaskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if task.Scope != service.AccountBatchTaskScopeUser || task.OwnerUserID == nil || *task.OwnerUserID != subject.UserID {
		response.NotFound(c, "Account batch task not found")
		return
	}
	response.Success(c, task)
}

func (h *UserAccountHandler) setOwnedAccountPrivacy(ctx context.Context, ownerUserID int64, account *service.Account) (string, error) {
	if account == nil {
		return "", service.ErrAccountNotFound
	}
	if account.Type != service.AccountTypeOAuth {
		return "", errors.New("only OAuth accounts support privacy setting")
	}
	proxyURL, err := h.accountService.OwnedProxyURL(ctx, ownerUserID, account.ProxyID)
	if err != nil {
		return "", err
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return "", errors.New("cannot set privacy: missing access_token")
	}

	var mode string
	switch account.Platform {
	case service.PlatformOpenAI:
		if h.openaiOAuthService == nil || h.openaiOAuthService.PrivacyClientFactory() == nil {
			return "", errors.New("OpenAI privacy client is not configured")
		}
		mode = service.DisableOpenAITraining(ctx, h.openaiOAuthService.PrivacyClientFactory(), accessToken, proxyURL)
	case service.PlatformAntigravity:
		mode = service.SetAntigravityPrivacy(ctx, accessToken, account.GetCredential("project_id"), proxyURL)
	default:
		return "", errors.New("only OpenAI and Antigravity OAuth accounts support privacy setting")
	}
	if strings.TrimSpace(mode) == "" {
		return "", errors.New("privacy setting failed")
	}
	extra := make(map[string]any, len(account.Extra)+1)
	for key, value := range account.Extra {
		extra[key] = value
	}
	extra["privacy_mode"] = mode
	updated, err := h.accountService.UpdateOwned(ctx, ownerUserID, account.ID, service.UpdateAccountRequest{Extra: &extra})
	if err != nil {
		return "", err
	}
	account.Extra = updated.Extra
	return mode, nil
}

func (h *UserAccountHandler) SetPrivacy(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	account, ok := h.getOwnedAccount(c)
	if !ok {
		return
	}
	if _, err := h.setOwnedAccountPrivacy(c.Request.Context(), subject.UserID, account); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updated, err := h.accountService.GetOwnedByID(c.Request.Context(), subject.UserID, account.ID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updated))
}

func (h *UserAccountHandler) ListProxies(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	proxies, err := h.accountService.ListOwnedProxies(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		out = append(out, *dto.ProxyWithAccountCountFromService(&proxies[i]))
	}
	response.Success(c, out)
}

func (h *UserAccountHandler) CreateProxy(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	var req service.CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxy, err := h.accountService.CreateOwnedProxy(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.ProxyFromService(proxy))
}

func (h *UserAccountHandler) UpdateProxy(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	var req service.UpdateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxy, err := h.accountService.UpdateOwnedProxy(c.Request.Context(), subject.UserID, id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProxyFromService(proxy))
}

func (h *UserAccountHandler) DeleteProxy(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	if err := h.accountService.DeleteOwnedProxy(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "proxy deleted"})
}

func (h *UserAccountHandler) TestProxy(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	result, err := h.accountService.TestOwnedProxy(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) CheckProxyQuality(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	result, err := h.accountService.CheckOwnedProxyQuality(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// User OAuth endpoints deliberately return token metadata only. Persisting an
// account remains a separate owner-scoped /accounts write, so an OAuth session
// cannot select another user's account by ID.
func (h *UserAccountHandler) GenerateAnthropicOAuthURL(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.oauthService == nil {
		if ok {
			response.InternalError(c, "Anthropic OAuth service is not configured")
		}
		return
	}
	var req userOAuthProxyRequest
	if !bindOptionalUserJSON(c, &req) {
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	result, err := h.oauthService.GenerateAuthURL(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) ExchangeAnthropicOAuthCode(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.oauthService == nil {
		if ok {
			response.InternalError(c, "Anthropic OAuth service is not configured")
		}
		return
	}
	var req userExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	tokenInfo, err := h.oauthService.ExchangeCode(c.Request.Context(), &service.ExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		ProxyID:   proxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *UserAccountHandler) GenerateAnthropicSetupTokenURL(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}

func (h *UserAccountHandler) ExchangeAnthropicSetupTokenCode(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}

func (h *UserAccountHandler) AnthropicCookieAuth(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}

func (h *UserAccountHandler) AnthropicSetupTokenCookieAuth(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}

func (h *UserAccountHandler) GenerateOpenAIOAuthURL(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.openaiOAuthService == nil {
		if ok {
			response.InternalError(c, "OpenAI OAuth service is not configured")
		}
		return
	}
	var req userOpenAIGenerateAuthURLRequest
	if !bindOptionalUserJSON(c, &req) {
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	result, err := h.openaiOAuthService.GenerateAuthURL(c.Request.Context(), proxyID, req.RedirectURI, service.PlatformOpenAI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) ExchangeOpenAIOAuthCode(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.openaiOAuthService == nil {
		if ok {
			response.InternalError(c, "OpenAI OAuth service is not configured")
		}
		return
	}
	var req userOpenAIExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	tokenInfo, err := h.openaiOAuthService.ExchangeCode(c.Request.Context(), &service.OpenAIExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     proxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *UserAccountHandler) RefreshOpenAIToken(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}

func (h *UserAccountHandler) GetGeminiOAuthCapabilities(c *gin.Context) {
	if _, ok := userAccountSubject(c); !ok {
		return
	}
	if h.geminiOAuthService == nil {
		response.InternalError(c, "Gemini OAuth service is not configured")
		return
	}
	response.Success(c, h.geminiOAuthService.GetOAuthConfig())
}

func (h *UserAccountHandler) GenerateGeminiOAuthURL(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.geminiOAuthService == nil {
		if ok {
			response.InternalError(c, "Gemini OAuth service is not configured")
		}
		return
	}
	var req userGeminiGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	oauthType := strings.TrimSpace(req.OAuthType)
	if oauthType == "" {
		oauthType = "code_assist"
	}
	if oauthType != "code_assist" && oauthType != "google_one" && oauthType != "ai_studio" {
		response.BadRequest(c, "Invalid oauth_type")
		return
	}
	result, err := h.geminiOAuthService.GenerateAuthURL(c.Request.Context(), proxyID, deriveUserGeminiRedirectURI(c), req.ProjectID, oauthType, req.TierID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) ExchangeGeminiOAuthCode(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.geminiOAuthService == nil {
		if ok {
			response.InternalError(c, "Gemini OAuth service is not configured")
		}
		return
	}
	var req userGeminiExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	oauthType := strings.TrimSpace(req.OAuthType)
	if oauthType == "" {
		oauthType = "code_assist"
	}
	if oauthType != "code_assist" && oauthType != "google_one" && oauthType != "ai_studio" {
		response.BadRequest(c, "Invalid oauth_type")
		return
	}
	tokenInfo, err := h.geminiOAuthService.ExchangeCode(c.Request.Context(), &service.GeminiExchangeCodeInput{
		SessionID: req.SessionID,
		State:     req.State,
		Code:      req.Code,
		ProxyID:   proxyID,
		OAuthType: oauthType,
		TierID:    req.TierID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *UserAccountHandler) GenerateAntigravityOAuthURL(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.antigravityOAuthService == nil {
		if ok {
			response.InternalError(c, "Antigravity OAuth service is not configured")
		}
		return
	}
	var req userAntigravityGenerateAuthURLRequest
	if !bindOptionalUserJSON(c, &req) {
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	result, err := h.antigravityOAuthService.GenerateAuthURL(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) ExchangeAntigravityOAuthCode(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.antigravityOAuthService == nil {
		if ok {
			response.InternalError(c, "Antigravity OAuth service is not configured")
		}
		return
	}
	var req userAntigravityExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	tokenInfo, err := h.antigravityOAuthService.ExchangeCode(c.Request.Context(), &service.AntigravityExchangeCodeInput{
		SessionID: req.SessionID,
		State:     req.State,
		Code:      req.Code,
		ProxyID:   proxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *UserAccountHandler) RefreshAntigravityToken(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}

func (h *UserAccountHandler) GenerateGrokOAuthURL(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.grokOAuthService == nil {
		if ok {
			response.InternalError(c, "Grok OAuth service is not configured")
		}
		return
	}
	var req userGrokGenerateAuthURLRequest
	if !bindOptionalUserJSON(c, &req) {
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	result, err := h.grokOAuthService.GenerateAuthURL(c.Request.Context(), proxyID, req.RedirectURI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UserAccountHandler) ExchangeGrokOAuthCode(c *gin.Context) {
	subject, ok := userAccountSubject(c)
	if !ok || h.grokOAuthService == nil {
		if ok {
			response.InternalError(c, "Grok OAuth service is not configured")
		}
		return
	}
	var req userGrokExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	proxyID, ok := h.resolveUserOAuthProxyID(c, subject.UserID, req.ProxyID)
	if !ok {
		return
	}
	tokenInfo, err := h.grokOAuthService.ExchangeCode(c.Request.Context(), &service.GrokExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     proxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *UserAccountHandler) RefreshGrokToken(c *gin.Context) {
	rejectUserManualCredentialAuth(c)
}
