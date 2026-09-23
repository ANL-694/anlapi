package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrAccountNotFound               = infraerrors.NotFound("ACCOUNT_NOT_FOUND", "account not found")
	ErrAccountNilInput               = infraerrors.BadRequest("ACCOUNT_NIL_INPUT", "account input cannot be nil")
	ErrAccountNotInFallback          = infraerrors.BadRequest("ACCOUNT_NOT_IN_FALLBACK", "account is not in proxy fallback state")
	ErrUserPrivateProxyLimitExceeded = infraerrors.BadRequest("USER_PRIVATE_PROXY_LIMIT_EXCEEDED", "user private proxy limit exceeded")
	ErrUserPrivateProxyInvalid       = infraerrors.BadRequest("USER_PRIVATE_PROXY_INVALID", "invalid private proxy configuration")
	ErrUserPrivateProxyUnavailable   = infraerrors.InternalServer("USER_PRIVATE_PROXY_UNAVAILABLE", "user private proxy repository is unavailable")
)

const AccountListGroupUngrouped int64 = -1
const AccountPrivacyModeUnsetFilter = "__unset__"
const UserPrivateProxyLimit = 3

// OAuthRefreshPageOptions describes one bounded, cursor-stable scan of OAuth
// accounts. Candidate platforms are supplied by TokenRefreshService's refresher
// registry so repository eligibility cannot drift from registered providers.
type OAuthRefreshPageOptions struct {
	Platforms            []string
	AfterID              int64
	Limit                int
	ActiveOnly           bool
	IncludeSetupToken    bool
	RequireRefreshToken  bool
	ExcludeRetryCooldown bool
}

// OAuthRefreshCandidatePage keeps cursor metadata from the raw SQL ID page.
// Hydration may legitimately lose a concurrently deleted row, but callers can
// still advance past the raw page without truncating or duplicating the scan.
type OAuthRefreshCandidatePage struct {
	Accounts    []Account
	NextAfterID int64
	HasMore     bool
}

// OAuthRefreshCandidatePager is intentionally narrower than AccountRepository.
// Production refresh cycles fail closed when the repository does not implement
// this bounded contract instead of silently falling back to an unpaged scan.
type OAuthRefreshCandidatePager interface {
	ListOAuthRefreshCandidatePage(ctx context.Context, options OAuthRefreshPageOptions) (*OAuthRefreshCandidatePage, error)
}

type AccountRepository interface {
	Create(ctx context.Context, account *Account) error
	GetByID(ctx context.Context, id int64) (*Account, error)
	// GetByIDs fetches accounts by IDs in a single query.
	// It should return all accounts found (missing IDs are ignored).
	GetByIDs(ctx context.Context, ids []int64) ([]*Account, error)
	// ExistsByID 检查账号是否存在，仅返回布尔值，用于删除前的轻量级存在性检查
	ExistsByID(ctx context.Context, id int64) (bool, error)
	// GetByCRSAccountID finds an account previously synced from CRS.
	// Returns (nil, nil) if not found.
	GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error)
	// FindByExtraField 根据 extra 字段中的键值对查找账号
	FindByExtraField(ctx context.Context, key string, value any) ([]Account, error)
	// ListCRSAccountIDs returns a map of crs_account_id -> local account ID
	// for all accounts that have been synced from CRS.
	ListCRSAccountIDs(ctx context.Context) (map[string]int64, error)
	Update(ctx context.Context, account *Account) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error)
	// ListAllWithFilters 返回符合过滤条件的全部账号（不分页），用于账号列表页
	// 计算 OpenAI 调度分数的过滤范围池。
	ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, error)
	ListByGroup(ctx context.Context, groupID int64) ([]Account, error)
	ListActive(ctx context.Context) ([]Account, error)
	ListByPlatform(ctx context.Context, platform string) ([]Account, error)

	UpdateLastUsed(ctx context.Context, id int64) error
	BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	ClearError(ctx context.Context, id int64) error
	SetSchedulable(ctx context.Context, id int64, schedulable bool) error
	AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error)
	BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error

	ListSchedulable(ctx context.Context) ([]Account, error)
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
	ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error)
	ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error)
	ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error)
	ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error)
	// ListModelAvailabilityCandidates returns accounts that are enabled by
	// persistent configuration (active + schedulable) for model-support
	// diagnosis. It deliberately does not filter transient runtime state such
	// as rate-limit, overload, temporary-unschedulable, or expiry windows.
	// When groupID is nil, includeGrouped controls whether the query scans all
	// matching accounts or only accounts without a group binding.
	ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]Account, error)

	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error
	SetOverloaded(ctx context.Context, id int64, until time.Time) error
	SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error
	ClearTempUnschedulable(ctx context.Context, id int64) error
	ClearRateLimit(ctx context.Context, id int64) error
	ClearAntigravityQuotaScopes(ctx context.Context, id int64) error
	ClearModelRateLimits(ctx context.Context, id int64) error
	UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error
	// UpdateSessionWindowEnd 仅更新 5h 窗口的结束时间，不动 start / status。
	// 用于 active poll 拿到新 ResetsAt 后回写，避免覆盖请求路径上记录的 status。
	UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error)
	// IncrementQuotaUsed 原子递增 API Key 账号的配额用量（总/日/周）
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error
	// ResetQuotaUsedAndClearRateLimitCooldown atomically resets API Key quota usage
	// and clears only the account-level rate-limit cooldown.
	ResetQuotaUsedAndClearRateLimitCooldown(ctx context.Context, id int64) error
	// RevertProxyFallback 将账号的 proxy_id 切回 proxy_fallback_origin_id，并清空 origin 字段。
	// 仅当 proxy_fallback_origin_id IS NOT NULL 时更新，否则视为账号不存在（返回 ErrAccountNotFound）。
	RevertProxyFallback(ctx context.Context, accountID int64) error
	// ListShadowsByParent 返回指定父账号的影子账号；当前实现仅查 quota_dimension='spark'（唯一预设）。
	// ⚠️ 新增影子维度时：须更新此函数（或新增维度专用列举），并检查所有调用点（级联删除/一母一影校验/type 守卫），否则会静默漏掉新维度。
	ListShadowsByParent(ctx context.Context, parentID int64) ([]*Account, error)
}

// AccountListFilters is the user-facing subset of account filters. Admin-only
// scheduling and billing filters stay on the admin service surface.
type AccountListFilters struct {
	Platform    string
	AccountType string
	Status      string
	Search      string
	GroupID     int64
	PrivacyMode string
}

// OwnedAccountRepository is an optional narrow capability for user-owned
// account management. Keeping it separate preserves existing repository test
// doubles and prevents gateway code from depending on user CRUD semantics.
type OwnedAccountRepository interface {
	ListOwnedWithFilters(ctx context.Context, ownerUserID int64, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error)
	GetOwnedByID(ctx context.Context, ownerUserID, accountID int64) (*Account, error)
}

// OwnedAccountMutationRepository provides owner-predicate writes for the
// user-facing account surface. Implementations must apply ownerUserID in the
// same database mutation as the account ID; a prior owner-scoped read is not
// sufficient under concurrent ownership changes or account deletion.
//
// It remains optional so existing read-only and service test doubles do not
// need to implement user CRUD semantics. Production repositories implement it.
type OwnedAccountMutationRepository interface {
	UpdateOwned(ctx context.Context, ownerUserID int64, account *Account) error
	DeleteOwned(ctx context.Context, ownerUserID, accountID int64) error
}

type userPrivateProxyRepository interface {
	Create(ctx context.Context, proxy *Proxy) error
	GetOwnedByID(ctx context.Context, ownerUserID, id int64) (*Proxy, error)
	ListOwnedByUserID(ctx context.Context, ownerUserID int64) ([]ProxyWithAccountCount, error)
	CountByOwnerUserID(ctx context.Context, ownerUserID int64) (int64, error)
	CountOwnedAccountsByProxyID(ctx context.Context, ownerUserID, proxyID int64) (int64, error)
	Update(ctx context.Context, proxy *Proxy) error
	DeleteOwned(ctx context.Context, ownerUserID, id int64) error
}

type AccountDuplicateRepository interface {
	// CreateWithAccountGroups atomically persists an account, its exact group priorities,
	// and the scheduler outbox event for the new routing snapshot.
	CreateWithAccountGroups(ctx context.Context, account *Account, groups []AccountGroup) error
}

// AccountBillingSettingsRepository applies an admin edit without overwriting a
// rate_multiplier that a successful upstream probe synchronized after the edit
// form was loaded. A nil rateMultiplier means the request did not edit it.
type AccountBillingSettingsRepository interface {
	UpdateWithAccountBillingSettings(
		ctx context.Context,
		account *Account,
		probeEnabled *bool,
		rateSyncEnabled *bool,
		rateMultiplier *float64,
	) error
}

// AdminAccountRepository makes the account-duplication write capability an explicit
// construction dependency without forcing read-only gateway test doubles to implement it.
type AdminAccountRepository interface {
	AccountRepository
	AccountDuplicateRepository
	AccountBillingSettingsRepository
}

// AccountBulkUpdate describes the fields that can be updated in a bulk operation.
// Nil pointers mean "do not change".
type AccountBulkUpdate struct {
	Name           *string
	ProxyID        *int64
	Concurrency    *int
	Priority       *int
	RateMultiplier *float64
	LoadFactor     *int
	Status         *string
	Schedulable    *bool
	Credentials    map[string]any
	Extra          map[string]any
	ProbeEnabled   *bool
	// EnsureCodexFingerprintSeed asks the repository to atomically preserve an
	// existing valid Codex fingerprint seed or create one for eligible rows.
	EnsureCodexFingerprintSeed bool
}

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	Name               string         `json:"name"`
	Notes              *string        `json:"notes"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra"`
	ProxyID            *int64         `json:"proxy_id"`
	ShareMode          *string        `json:"share_mode"`
	ShareStatus        *string        `json:"share_status"`
	Concurrency        int            `json:"concurrency"`
	LoadFactor         *int           `json:"load_factor"`
	Priority           int            `json:"priority"`
	GroupIDs           []int64        `json:"group_ids"`
	ExpiresAt          *time.Time     `json:"expires_at"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired"`
}

// UpdateAccountRequest 更新账号请求
type UpdateAccountRequest struct {
	Name               *string         `json:"name"`
	Notes              *string         `json:"notes"`
	Credentials        *map[string]any `json:"credentials"`
	Extra              *map[string]any `json:"extra"`
	ProxyID            *int64          `json:"proxy_id"`
	ShareMode          *string         `json:"share_mode"`
	ShareStatus        *string         `json:"share_status"`
	Concurrency        *int            `json:"concurrency"`
	LoadFactor         *int            `json:"load_factor"`
	Priority           *int            `json:"priority"`
	Status             *string         `json:"status"`
	Schedulable        *bool           `json:"schedulable"`
	GroupIDs           *[]int64        `json:"group_ids"`
	ExpiresAt          *time.Time      `json:"expires_at"`
	AutoPauseOnExpired *bool           `json:"auto_pause_on_expired"`
}

// AccountService 账号管理服务
type AccountService struct {
	accountRepo AccountRepository
	groupRepo   GroupRepository
	proxyRepo   ProxyRepository
	proxyProber ProxyExitInfoProber
}

// UserBulkAccountResult is the stable per-account result returned by the
// user-scoped bulk endpoints. A failed item never aborts successful siblings.
type UserBulkAccountResult struct {
	AccountID int64  `json:"account_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

type UserBulkAccountOperationResult struct {
	Success    int                     `json:"success"`
	Failed     int                     `json:"failed"`
	SuccessIDs []int64                 `json:"success_ids,omitempty"`
	FailedIDs  []int64                 `json:"failed_ids,omitempty"`
	Results    []UserBulkAccountResult `json:"results"`
}

type BulkUpdateOwnedAccountsInput struct {
	AccountIDs  []int64
	Concurrency *int
	LoadFactor  *int
	Priority    *int
	Status      *string
	Schedulable *bool
	ProxyID     *int64
	ShareMode   *string
	ShareStatus *string
	GroupIDs    *[]int64
	Credentials map[string]any
	Extra       map[string]any
}

type groupExistenceBatchChecker interface {
	ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error)
}

func (s *AccountService) SetProxyRepository(repo ProxyRepository) {
	if s != nil {
		s.proxyRepo = repo
	}
}

func (s *AccountService) SetProxyProber(prober ProxyExitInfoProber) {
	if s != nil {
		s.proxyProber = prober
	}
}

// NewAccountService 创建账号服务实例
func NewAccountService(accountRepo AccountRepository, groupRepo GroupRepository) *AccountService {
	return &AccountService{
		accountRepo: accountRepo,
		groupRepo:   groupRepo,
	}
}

// Create 创建账号
func (s *AccountService) Create(ctx context.Context, req CreateAccountRequest) (*Account, error) {
	// 验证分组是否存在（如果指定了分组）
	if len(req.GroupIDs) > 0 {
		if err := s.validateGroupIDsExist(ctx, req.GroupIDs); err != nil {
			return nil, err
		}
	}

	// 创建账号
	account := &Account{
		Name:        req.Name,
		Notes:       normalizeAccountNotes(req.Notes),
		Platform:    req.Platform,
		Type:        req.Type,
		Credentials: SanitizeStoredCredentials(req.Platform, req.Credentials),
		Extra:       prepareCodexFingerprintExtraForCreate(req.Platform, req.Type, req.Extra),
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		Status:      StatusActive,
		ExpiresAt:   req.ExpiresAt,
	}
	if req.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *req.AutoPauseOnExpired
	} else {
		account.AutoPauseOnExpired = true
	}

	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	// require_oauth_only 检查：apikey 类型账号不可加入限制分组
	if account.Type == AccountTypeAPIKey && len(req.GroupIDs) > 0 {
		for _, gid := range req.GroupIDs {
			g, err := s.groupRepo.GetByID(ctx, gid)
			if err != nil {
				return nil, err
			}
			if g.RequireOAuthOnly && (g.Platform == PlatformOpenAI || g.Platform == PlatformAntigravity || g.Platform == PlatformAnthropic || g.Platform == PlatformGemini || g.Platform == PlatformGrok) {
				return nil, fmt.Errorf("分组 [%s] 仅允许 OAuth 账号，apikey 类型账号无法加入", g.Name)
			}
		}
	}

	// 绑定分组
	if len(req.GroupIDs) > 0 {
		if err := s.accountRepo.BindGroups(ctx, account.ID, req.GroupIDs); err != nil {
			return nil, fmt.Errorf("bind groups: %w", err)
		}
	}

	return account, nil
}

// GetByID 根据ID获取账号
func (s *AccountService) GetByID(ctx context.Context, id int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return account, nil
}

// ListOwned returns only accounts whose durable owner is the authenticated user.
func (s *AccountService) ListOwned(ctx context.Context, ownerUserID int64, params pagination.PaginationParams, filters AccountListFilters) ([]Account, *pagination.PaginationResult, error) {
	if ownerUserID <= 0 {
		return nil, nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, ok := s.accountRepo.(OwnedAccountRepository)
	if !ok {
		return nil, nil, infraerrors.InternalServer("OWNED_ACCOUNT_REPOSITORY_UNAVAILABLE", "owned account repository is unavailable")
	}
	accounts, result, err := repo.ListOwnedWithFilters(ctx, ownerUserID, params, filters.Platform, filters.AccountType, filters.Status, filters.Search, filters.GroupID, filters.PrivacyMode)
	if err != nil {
		return nil, nil, fmt.Errorf("list owned accounts: %w", err)
	}
	return accounts, result, nil
}

// GetOwnedByID resolves an account through an owner-scoped query. A different
// user's account is intentionally indistinguishable from a missing account.
func (s *AccountService) GetOwnedByID(ctx context.Context, ownerUserID, accountID int64) (*Account, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, ok := s.accountRepo.(OwnedAccountRepository)
	if !ok {
		return nil, infraerrors.InternalServer("OWNED_ACCOUNT_REPOSITORY_UNAVAILABLE", "owned account repository is unavailable")
	}
	account, err := repo.GetOwnedByID(ctx, ownerUserID, accountID)
	if err != nil {
		return nil, fmt.Errorf("get owned account: %w", err)
	}
	return account, nil
}

func (s *AccountService) userPrivateProxyRepo() (userPrivateProxyRepository, error) {
	if s == nil || s.proxyRepo == nil {
		return nil, ErrUserPrivateProxyUnavailable
	}
	repo, ok := s.proxyRepo.(userPrivateProxyRepository)
	if !ok {
		return nil, ErrUserPrivateProxyUnavailable
	}
	return repo, nil
}

func normalizeUserPrivateProxyCreate(req CreateProxyRequest) (*Proxy, error) {
	name := strings.TrimSpace(req.Name)
	protocol := strings.ToLower(strings.TrimSpace(req.Protocol))
	host := strings.TrimSpace(req.Host)
	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	port := req.Port
	if port == 0 {
		port = 443
	}
	if name == "" || host == "" || port <= 0 || port > 65535 {
		return nil, ErrUserPrivateProxyInvalid
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#") {
		return nil, ErrUserPrivateProxyInvalid
	}
	switch protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, ErrUserPrivateProxyInvalid
	}
	return &Proxy{
		Name:           name,
		Protocol:       protocol,
		Host:           host,
		Port:           port,
		Username:       username,
		Password:       password,
		Status:         StatusActive,
		FallbackMode:   FallbackModeNone,
		ExpiryWarnDays: 7,
	}, nil
}

func (s *AccountService) ListOwnedProxies(ctx context.Context, ownerUserID int64) ([]ProxyWithAccountCount, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return nil, err
	}
	return repo.ListOwnedByUserID(ctx, ownerUserID)
}

func (s *AccountService) CreateOwnedProxy(ctx context.Context, ownerUserID int64, req CreateProxyRequest) (*Proxy, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return nil, err
	}
	count, err := repo.CountByOwnerUserID(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	if count >= UserPrivateProxyLimit {
		return nil, ErrUserPrivateProxyLimitExceeded.WithMetadata(map[string]string{
			"limit": fmt.Sprintf("%d", UserPrivateProxyLimit),
		})
	}
	proxy, err := normalizeUserPrivateProxyCreate(req)
	if err != nil {
		return nil, err
	}
	proxy.OwnerUserID = &ownerUserID
	if err := repo.Create(ctx, proxy); err != nil {
		return nil, fmt.Errorf("create user private proxy: %w", err)
	}
	return proxy, nil
}

func (s *AccountService) UpdateOwnedProxy(ctx context.Context, ownerUserID, proxyID int64, req UpdateProxyRequest) (*Proxy, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return nil, err
	}
	proxy, err := repo.GetOwnedByID(ctx, ownerUserID, proxyID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		proxy.Name = *req.Name
	}
	if req.Protocol != nil {
		proxy.Protocol = *req.Protocol
	}
	if req.Host != nil {
		proxy.Host = *req.Host
	}
	if req.Port != nil {
		proxy.Port = *req.Port
	}
	if req.Username != nil {
		proxy.Username = *req.Username
	}
	if req.Password != nil {
		proxy.Password = *req.Password
	}
	if req.Status != nil {
		proxy.Status = strings.ToLower(strings.TrimSpace(*req.Status))
	}
	normalized, err := normalizeUserPrivateProxyCreate(CreateProxyRequest{
		Name: proxy.Name, Protocol: proxy.Protocol, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Password: proxy.Password,
	})
	if err != nil {
		return nil, err
	}
	proxy.Name = normalized.Name
	proxy.Protocol = normalized.Protocol
	proxy.Host = normalized.Host
	proxy.Port = normalized.Port
	proxy.Username = normalized.Username
	proxy.Password = normalized.Password
	switch proxy.Status {
	case "", StatusActive:
		proxy.Status = StatusActive
	case "inactive", StatusDisabled:
		proxy.Status = "inactive"
	default:
		return nil, ErrUserPrivateProxyInvalid
	}
	proxy.OwnerUserID = &ownerUserID
	if err := repo.Update(ctx, proxy); err != nil {
		return nil, fmt.Errorf("update user private proxy: %w", err)
	}
	return proxy, nil
}

func (s *AccountService) DeleteOwnedProxy(ctx context.Context, ownerUserID, proxyID int64) error {
	if ownerUserID <= 0 {
		return infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return err
	}
	if _, err := repo.GetOwnedByID(ctx, ownerUserID, proxyID); err != nil {
		return err
	}
	count, err := repo.CountOwnedAccountsByProxyID(ctx, ownerUserID, proxyID)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProxyInUse
	}
	if err := repo.DeleteOwned(ctx, ownerUserID, proxyID); err != nil {
		return fmt.Errorf("delete user private proxy: %w", err)
	}
	return nil
}

func (s *AccountService) TestOwnedProxy(ctx context.Context, ownerUserID, proxyID int64) (*ProxyTestResult, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return nil, err
	}
	if _, err := repo.GetOwnedByID(ctx, ownerUserID, proxyID); err != nil {
		return nil, err
	}
	if s.proxyProber == nil {
		return nil, ErrProxyProbeUnavailable
	}
	return (&adminServiceImpl{proxyRepo: s.proxyRepo, proxyProber: s.proxyProber}).TestProxy(ctx, proxyID)
}

func (s *AccountService) CheckOwnedProxyQuality(ctx context.Context, ownerUserID, proxyID int64) (*ProxyQualityCheckResult, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return nil, err
	}
	if _, err := repo.GetOwnedByID(ctx, ownerUserID, proxyID); err != nil {
		return nil, err
	}
	if s.proxyProber == nil {
		return nil, ErrProxyProbeUnavailable
	}
	return (&adminServiceImpl{proxyRepo: s.proxyRepo, proxyProber: s.proxyProber}).CheckProxyQuality(ctx, proxyID)
}

func (s *AccountService) ValidateOwnedProxyID(ctx context.Context, ownerUserID int64, proxyID *int64) (*int64, error) {
	if proxyID == nil || *proxyID <= 0 {
		return nil, nil
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return nil, err
	}
	proxy, err := repo.GetOwnedByID(ctx, ownerUserID, *proxyID)
	if err != nil {
		return nil, err
	}
	if proxy.Status != StatusActive {
		return nil, ErrUserPrivateProxyInvalid
	}
	id := proxy.ID
	return &id, nil
}

// OwnedProxyURL resolves a proxy only through the authenticated owner's
// private-proxy scope. It is used by user-scoped provider actions that need the
// same egress path as the account without exposing a global proxy lookup.
func (s *AccountService) OwnedProxyURL(ctx context.Context, ownerUserID int64, proxyID *int64) (string, error) {
	if proxyID == nil || *proxyID <= 0 {
		return "", nil
	}
	if ownerUserID <= 0 {
		return "", infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	repo, err := s.userPrivateProxyRepo()
	if err != nil {
		return "", err
	}
	proxy, err := repo.GetOwnedByID(ctx, ownerUserID, *proxyID)
	if err != nil {
		return "", err
	}
	if proxy == nil || proxy.Status != StatusActive {
		return "", ErrUserPrivateProxyInvalid
	}
	return proxy.URL(), nil
}

// CreateOwned creates an account with ownership forced from the JWT subject.
// Any owner_user_id supplied by a client is not part of the service request and
// therefore cannot override this value.
func (s *AccountService) CreateOwned(ctx context.Context, ownerUserID int64, req CreateAccountRequest) (*Account, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	if req.Name == "" || req.Platform == "" || req.Type == "" {
		return nil, infraerrors.BadRequest("OWNED_ACCOUNT_FIELDS_REQUIRED", "name, platform and type are required")
	}
	var validatedProxyID *int64
	if req.ProxyID != nil {
		var err error
		validatedProxyID, err = s.ValidateOwnedProxyID(ctx, ownerUserID, req.ProxyID)
		if err != nil {
			return nil, err
		}
	}
	account := &Account{
		Name:        req.Name,
		Notes:       normalizeAccountNotes(req.Notes),
		Platform:    req.Platform,
		Type:        req.Type,
		Credentials: SanitizeStoredCredentials(req.Platform, req.Credentials),
		Extra:       req.Extra,
		ProxyID:     validatedProxyID,
		OwnerUserID: &ownerUserID,
		ShareMode:   AccountShareModePrivate,
		ShareStatus: AccountShareStatusApproved,
		Concurrency: req.Concurrency,
		LoadFactor:  req.LoadFactor,
		Priority:    req.Priority,
		Status:      StatusActive,
		Schedulable: true,
		ExpiresAt:   req.ExpiresAt,
	}
	if req.ShareMode != nil {
		account.ShareMode = NormalizeAccountShareMode(*req.ShareMode)
		if account.ShareMode == AccountShareModePublic {
			account.ShareStatus = AccountShareStatusPending
		}
	}
	if req.ShareStatus != nil {
		account.ShareStatus = NormalizeAccountShareStatus(*req.ShareStatus)
	}
	if account.Concurrency <= 0 {
		account.Concurrency = 3
	}
	if account.Priority <= 0 {
		account.Priority = 50
	}
	if req.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *req.AutoPauseOnExpired
	} else {
		account.AutoPauseOnExpired = true
	}
	if len(req.GroupIDs) > 0 {
		if err := s.validateOwnedGroupIDs(ctx, ownerUserID, req.GroupIDs); err != nil {
			return nil, err
		}
		if repo, ok := s.accountRepo.(AccountDuplicateRepository); ok {
			groups := make([]AccountGroup, 0, len(req.GroupIDs))
			for i, groupID := range req.GroupIDs {
				groups = append(groups, AccountGroup{GroupID: groupID, Priority: i + 1})
			}
			if err := repo.CreateWithAccountGroups(ctx, account, groups); err != nil {
				return nil, fmt.Errorf("create owned account: %w", err)
			}
		} else {
			if err := s.accountRepo.Create(ctx, account); err != nil {
				return nil, fmt.Errorf("create owned account: %w", err)
			}
			if err := s.accountRepo.BindGroups(ctx, account.ID, req.GroupIDs); err != nil {
				return nil, fmt.Errorf("bind owned account groups: %w", err)
			}
		}
	} else if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create owned account: %w", err)
	}
	return account, nil
}

// UpdateOwned applies the same account field rules as the official service,
// but resolves the target through the authenticated owner's scope first.
func (s *AccountService) UpdateOwned(ctx context.Context, ownerUserID, accountID int64, req UpdateAccountRequest) (*Account, error) {
	account, err := s.GetOwnedByID(ctx, ownerUserID, accountID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		account.Name = *req.Name
	}
	if req.Notes != nil {
		account.Notes = normalizeAccountNotes(req.Notes)
	}
	if req.Credentials != nil {
		account.Credentials = SanitizeStoredCredentials(account.Platform, *req.Credentials)
	}
	if req.Extra != nil {
		account.Extra = *req.Extra
	}
	if req.ProxyID != nil {
		validatedProxyID, err := s.ValidateOwnedProxyID(ctx, ownerUserID, req.ProxyID)
		if err != nil {
			return nil, err
		}
		account.ProxyID = validatedProxyID
	}
	if req.ShareMode != nil {
		account.ShareMode = NormalizeAccountShareMode(*req.ShareMode)
		if account.ShareMode == AccountShareModePublic && NormalizeAccountShareStatus(account.ShareStatus) == AccountShareStatusApproved {
			account.ShareStatus = AccountShareStatusPending
		}
		if account.ShareMode == AccountShareModePrivate {
			account.ShareStatus = AccountShareStatusApproved
		}
	}
	if req.ShareStatus != nil {
		account.ShareStatus = NormalizeAccountShareStatus(*req.ShareStatus)
	}
	if req.Concurrency != nil {
		account.Concurrency = *req.Concurrency
	}
	if req.LoadFactor != nil {
		account.LoadFactor = req.LoadFactor
	}
	if req.Priority != nil {
		account.Priority = *req.Priority
	}
	if req.Status != nil {
		account.Status = *req.Status
	}
	if req.Schedulable != nil {
		account.Schedulable = *req.Schedulable
	}
	if req.ExpiresAt != nil {
		account.ExpiresAt = req.ExpiresAt
	}
	if req.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *req.AutoPauseOnExpired
	}
	if req.GroupIDs != nil {
		if err := s.validateOwnedGroupIDs(ctx, ownerUserID, *req.GroupIDs); err != nil {
			return nil, err
		}
	}
	if err := s.updateOwnedAccount(ctx, ownerUserID, account); err != nil {
		return nil, fmt.Errorf("update owned account: %w", err)
	}
	if req.GroupIDs != nil {
		if err := s.accountRepo.BindGroups(ctx, account.ID, *req.GroupIDs); err != nil {
			return nil, fmt.Errorf("bind owned account groups: %w", err)
		}
	}
	return account, nil
}

// SetOwnedPublicShareStatus is an internal server-side transition used after
// a connectivity validation. User-facing update payloads must not be able to
// set share_status directly, otherwise an account could be marked approved
// without the validation step.
func (s *AccountService) SetOwnedPublicShareStatus(ctx context.Context, ownerUserID, accountID int64, status, reason string) (*Account, error) {
	account, err := s.GetOwnedByID(ctx, ownerUserID, accountID)
	if err != nil {
		return nil, err
	}
	if NormalizeAccountShareMode(account.ShareMode) != AccountShareModePublic {
		return nil, infraerrors.BadRequest("ACCOUNT_PUBLIC_SHARE_REQUIRED", "account is not configured for public sharing")
	}
	normalizedStatus := NormalizeAccountShareStatus(status)
	switch normalizedStatus {
	case AccountShareStatusPending, AccountShareStatusApproved, AccountShareStatusSuspended:
	default:
		return nil, infraerrors.BadRequest("ACCOUNT_SHARE_STATUS_INVALID", "invalid public share status")
	}
	account.ShareStatus = normalizedStatus
	if normalizedStatus == AccountShareStatusApproved {
		account.ErrorMessage = ""
	} else if strings.TrimSpace(reason) != "" {
		account.ErrorMessage = trimAccountShareReason(reason)
	}
	if err := s.updateOwnedAccount(ctx, ownerUserID, account); err != nil {
		return nil, fmt.Errorf("update owned public share status: %w", err)
	}
	return account, nil
}

func (s *AccountService) updateOwnedAccount(ctx context.Context, ownerUserID int64, account *Account) error {
	if repo, ok := s.accountRepo.(OwnedAccountMutationRepository); ok {
		return repo.UpdateOwned(ctx, ownerUserID, account)
	}
	// Keep lightweight service doubles compatible while the production
	// repository uses the atomic owner-predicate capability above.
	return s.accountRepo.Update(ctx, account)
}

func trimAccountShareReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		return reason[:500]
	}
	return reason
}

// BulkUpdateOwned applies a bounded set of user-owned updates one account at a
// time. The result is intentionally per-item so one invalid account cannot
// cause the UI to lose the successful siblings.
func (s *AccountService) BulkUpdateOwned(ctx context.Context, ownerUserID int64, input *BulkUpdateOwnedAccountsInput) (*UserBulkAccountOperationResult, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	if input == nil || len(input.AccountIDs) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_IDS_REQUIRED", "account_ids is required")
	}
	if len(input.AccountIDs) > 1000 {
		return nil, infraerrors.BadRequest("ACCOUNT_BATCH_TOO_LARGE", "too many account_ids")
	}
	ids := uniquePositiveAccountIDs(input.AccountIDs)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_IDS_REQUIRED", "account_ids is required")
	}
	if input.GroupIDs != nil {
		if err := s.validateOwnedGroupIDs(ctx, ownerUserID, *input.GroupIDs); err != nil {
			return nil, err
		}
	}
	if input.ProxyID != nil {
		if _, err := s.ValidateOwnedProxyID(ctx, ownerUserID, input.ProxyID); err != nil {
			return nil, err
		}
	}

	result := &UserBulkAccountOperationResult{Results: make([]UserBulkAccountResult, 0, len(ids))}
	for _, id := range ids {
		updates := UpdateAccountRequest{
			Concurrency: input.Concurrency,
			LoadFactor:  input.LoadFactor,
			Priority:    input.Priority,
			Status:      input.Status,
			Schedulable: input.Schedulable,
			ProxyID:     input.ProxyID,
			ShareMode:   input.ShareMode,
			ShareStatus: input.ShareStatus,
			GroupIDs:    input.GroupIDs,
		}
		if len(input.Credentials) > 0 {
			credentials := input.Credentials
			updates.Credentials = &credentials
		}
		if len(input.Extra) > 0 {
			extra := input.Extra
			updates.Extra = &extra
		}
		_, err := s.UpdateOwned(ctx, ownerUserID, id, updates)
		entry := UserBulkAccountResult{AccountID: id, Success: err == nil}
		if err != nil {
			entry.Error = err.Error()
			result.Failed++
			result.FailedIDs = append(result.FailedIDs, id)
		} else {
			result.Success++
			result.SuccessIDs = append(result.SuccessIDs, id)
		}
		result.Results = append(result.Results, entry)
	}
	return result, nil
}

// BulkDeleteOwned deletes only accounts resolved through the authenticated
// owner's scope and reports each item independently.
func (s *AccountService) BulkDeleteOwned(ctx context.Context, ownerUserID int64, accountIDs []int64) (*UserBulkAccountOperationResult, error) {
	if ownerUserID <= 0 {
		return nil, infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	ids := uniquePositiveAccountIDs(accountIDs)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_IDS_REQUIRED", "account_ids is required")
	}
	if len(ids) > 1000 {
		return nil, infraerrors.BadRequest("ACCOUNT_BATCH_TOO_LARGE", "too many account_ids")
	}
	result := &UserBulkAccountOperationResult{Results: make([]UserBulkAccountResult, 0, len(ids))}
	for _, id := range ids {
		err := s.DeleteOwned(ctx, ownerUserID, id)
		entry := UserBulkAccountResult{AccountID: id, Success: err == nil}
		if err != nil {
			entry.Error = err.Error()
			result.Failed++
			result.FailedIDs = append(result.FailedIDs, id)
		} else {
			result.Success++
			result.SuccessIDs = append(result.SuccessIDs, id)
		}
		result.Results = append(result.Results, entry)
	}
	return result, nil
}

func uniquePositiveAccountIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// DeleteOwned hides cross-user account existence and deletes only an account
// previously resolved through the owner's scope.
func (s *AccountService) DeleteOwned(ctx context.Context, ownerUserID, accountID int64) error {
	if _, err := s.GetOwnedByID(ctx, ownerUserID, accountID); err != nil {
		return err
	}
	if repo, ok := s.accountRepo.(OwnedAccountMutationRepository); ok {
		if err := repo.DeleteOwned(ctx, ownerUserID, accountID); err != nil {
			return fmt.Errorf("delete owned account: %w", err)
		}
		return nil
	}
	if err := s.accountRepo.Delete(ctx, accountID); err != nil {
		return fmt.Errorf("delete owned account: %w", err)
	}
	return nil
}

// List 获取账号列表
func (s *AccountService) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	accounts, pagination, err := s.accountRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list accounts: %w", err)
	}
	return accounts, pagination, nil
}

// ListByPlatform 根据平台获取账号列表
func (s *AccountService) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	accounts, err := s.accountRepo.ListByPlatform(ctx, platform)
	if err != nil {
		return nil, fmt.Errorf("list accounts by platform: %w", err)
	}
	return accounts, nil
}

// ListByGroup 根据分组获取账号列表
func (s *AccountService) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	accounts, err := s.accountRepo.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list accounts by group: %w", err)
	}
	return accounts, nil
}

// Update 更新账号
func (s *AccountService) Update(ctx context.Context, id int64, req UpdateAccountRequest) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	// 更新字段
	if req.Name != nil {
		account.Name = *req.Name
	}
	if req.Notes != nil {
		account.Notes = normalizeAccountNotes(req.Notes)
	}

	if req.Credentials != nil {
		account.Credentials = SanitizeStoredCredentials(account.Platform, *req.Credentials)
	}

	if req.Extra != nil {
		extra := make(map[string]any, len(*req.Extra))
		for key, value := range *req.Extra {
			extra[key] = value
		}
		delete(extra, OllamaCloudUsageSessionExtraKey)
		delete(extra, OllamaCloudUsageAutoRefreshExtraKey)
		delete(extra, OllamaCloudUsageSnapshotExtraKey)
		account.Extra = prepareCodexFingerprintExtraForUpdate(account, extra)
	} else {
		account.Extra = prepareCodexFingerprintExtraForUpdate(account, account.Extra)
	}

	if req.ProxyID != nil {
		account.ProxyID = req.ProxyID
	}

	if req.Concurrency != nil {
		account.Concurrency = *req.Concurrency
	}

	if req.Priority != nil {
		account.Priority = *req.Priority
	}

	if req.Status != nil {
		account.Status = *req.Status
	}
	if req.ExpiresAt != nil {
		account.ExpiresAt = req.ExpiresAt
	}
	if req.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *req.AutoPauseOnExpired
	}

	// 先验证分组是否存在（在任何写操作之前）
	if req.GroupIDs != nil {
		if err := s.validateGroupIDsExist(ctx, *req.GroupIDs); err != nil {
			return nil, err
		}
	}

	// 执行更新
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("update account: %w", err)
	}

	// require_oauth_only 检查
	if account.Type == AccountTypeAPIKey && req.GroupIDs != nil {
		for _, gid := range *req.GroupIDs {
			g, err := s.groupRepo.GetByID(ctx, gid)
			if err != nil {
				return nil, err
			}
			if g.RequireOAuthOnly && (g.Platform == PlatformOpenAI || g.Platform == PlatformAntigravity || g.Platform == PlatformAnthropic || g.Platform == PlatformGemini || g.Platform == PlatformGrok) {
				return nil, fmt.Errorf("分组 [%s] 仅允许 OAuth 账号，apikey 类型账号无法加入", g.Name)
			}
		}
	}

	// 绑定分组
	if req.GroupIDs != nil {
		if err := s.accountRepo.BindGroups(ctx, account.ID, *req.GroupIDs); err != nil {
			return nil, fmt.Errorf("bind groups: %w", err)
		}
	}

	return account, nil
}

// Delete 删除账号
// 优化：使用 ExistsByID 替代 GetByID 进行存在性检查，
// 避免加载完整账号对象及其关联数据，提升删除操作的性能
func (s *AccountService) Delete(ctx context.Context, id int64) error {
	// 使用轻量级的存在性检查，而非加载完整账号对象
	exists, err := s.accountRepo.ExistsByID(ctx, id)
	if err != nil {
		return fmt.Errorf("check account: %w", err)
	}
	// 明确返回账号不存在错误，便于调用方区分错误类型
	if !exists {
		return ErrAccountNotFound
	}

	// 注意:此处不级联删除 spark 影子账号。当前唯一的后台删除入口走 AdminService.DeleteAccount
	// (已 ListShadowsByParent 先删影子再删母)。本方法目前无删除调用方;若未来有调用方经此
	// 删除母账号,需在此补级联,否则会留下孤儿影子(外审第6轮 P3:当前不可达,记为残留)。
	if err := s.accountRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}

	return nil
}

func (s *AccountService) validateGroupIDsExist(ctx context.Context, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	if s.groupRepo == nil {
		return fmt.Errorf("group repository not configured")
	}

	if batchChecker, ok := s.groupRepo.(groupExistenceBatchChecker); ok {
		existsByID, err := batchChecker.ExistsByIDs(ctx, groupIDs)
		if err != nil {
			return fmt.Errorf("check groups exists: %w", err)
		}
		for _, groupID := range groupIDs {
			if groupID <= 0 {
				return fmt.Errorf("get group: %w", ErrGroupNotFound)
			}
			if !existsByID[groupID] {
				return fmt.Errorf("get group: %w", ErrGroupNotFound)
			}
		}
		return nil
	}

	for _, groupID := range groupIDs {
		_, err := s.groupRepo.GetByID(ctx, groupID)
		if err != nil {
			return fmt.Errorf("get group: %w", err)
		}
	}
	return nil
}

// validateOwnedGroupIDs enforces the same public/private group contract used
// by the user-facing group list. Existence alone is insufficient: a private
// group owned by another user must be indistinguishable from an unavailable
// group for account binding purposes.
func (s *AccountService) validateOwnedGroupIDs(ctx context.Context, ownerUserID int64, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	if ownerUserID <= 0 || s.groupRepo == nil {
		return infraerrors.Unauthorized("ACCOUNT_OWNER_REQUIRED", "authenticated user required")
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			return ErrGroupNotFound
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		group, err := s.groupRepo.GetByID(ctx, groupID)
		if err != nil {
			return ErrGroupNotFound
		}
		if !GroupAccessAllowedForUser(group, ownerUserID) {
			return ErrGroupNotFound
		}
	}
	return nil
}

// UpdateStatus 更新账号状态
func (s *AccountService) UpdateStatus(ctx context.Context, id int64, status string, errorMessage string) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}

	account.Status = status
	account.ErrorMessage = errorMessage

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return fmt.Errorf("update account: %w", err)
	}

	return nil
}

// UpdateLastUsed 更新最后使用时间
func (s *AccountService) UpdateLastUsed(ctx context.Context, id int64) error {
	if err := s.accountRepo.UpdateLastUsed(ctx, id); err != nil {
		return fmt.Errorf("update last used: %w", err)
	}
	return nil
}

// GetCredential 获取账号凭证（安全访问）
func (s *AccountService) GetCredential(ctx context.Context, id int64, key string) (string, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get account: %w", err)
	}

	return account.GetCredential(key), nil
}

// TestCredentials 测试账号凭证是否有效（需要实现具体平台的测试逻辑）
func (s *AccountService) TestCredentials(ctx context.Context, id int64) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}

	// 根据平台执行不同的测试逻辑
	switch account.Platform {
	case PlatformAnthropic:
		// TODO: 测试Anthropic API凭证
		return nil
	case PlatformOpenAI:
		// TODO: 测试OpenAI API凭证
		return nil
	case PlatformGemini:
		// TODO: 测试Gemini API凭证
		return nil
	case PlatformGrok:
		// Grok OAuth credentials are validated via token exchange/refresh and request-path probes.
		return nil
	case PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo:
		// 国产 OpenAI 兼容供应商与 OpenCode：凭证为 API Key，实际可用性经余额/额度探测与转发路径验证。
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", account.Platform)
	}
}
