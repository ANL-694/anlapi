package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"anl-api/internal/config"
	infraerrors "anl-api/internal/pkg/errors"
	"anl-api/internal/pkg/logger"
	"anl-api/internal/pkg/timezone"
	"golang.org/x/sync/singleflight"
)

// 错误定义
// 注：ErrInsufficientBalance在redeem_service.go中定义
// 注：ErrDailyLimitExceeded/ErrWeeklyLimitExceeded/ErrMonthlyLimitExceeded在subscription_service.go中定义
var (
	ErrSubscriptionInvalid       = infraerrors.Forbidden("SUBSCRIPTION_INVALID", "subscription is invalid or expired")
	ErrBillingServiceUnavailable = infraerrors.ServiceUnavailable("BILLING_SERVICE_ERROR", "Billing service temporarily unavailable. Please retry later.")
	// RPM 超限错误。gateway_handler 负责映射为 HTTP 429。
	ErrGroupRPMExceeded                  = infraerrors.TooManyRequests("GROUP_RPM_EXCEEDED", "group requests-per-minute limit exceeded")
	ErrUserRPMExceeded                   = infraerrors.TooManyRequests("USER_RPM_EXCEEDED", "user requests-per-minute limit exceeded")
	ErrUserPlatformDailyQuotaExhausted   = infraerrors.TooManyRequests("USER_PLATFORM_DAILY_QUOTA_EXHAUSTED", "Daily usage quota exhausted for this platform.")
	ErrUserPlatformWeeklyQuotaExhausted  = infraerrors.TooManyRequests("USER_PLATFORM_WEEKLY_QUOTA_EXHAUSTED", "Weekly usage quota exhausted for this platform.")
	ErrUserPlatformMonthlyQuotaExhausted = infraerrors.TooManyRequests("USER_PLATFORM_MONTHLY_QUOTA_EXHAUSTED", "Monthly usage quota exhausted for this platform.")
)

var errBillingCacheUnavailable = fmt.Errorf("billing cache unavailable")

// subscriptionCacheData 订阅缓存数据结构（内部使用）
type subscriptionCacheData struct {
	Status       string
	ExpiresAt    time.Time
	DailyUsage   float64
	WeeklyUsage  float64
	MonthlyUsage float64
	Version      int64
}

// 缓存写入任务类型
type cacheWriteKind int

const (
	cacheWriteSetBalance cacheWriteKind = iota
	cacheWriteSetSubscription
	cacheWriteUpdateSubscriptionUsage
	cacheWriteDeductBalance
	cacheWriteUpdateRateLimitUsage
)

// 异步缓存写入工作池配置
//
// 性能优化说明：
// 原实现在请求热路径中使用 goroutine 异步更新缓存，存在以下问题：
// 1. 每次请求创建新 goroutine，高并发下产生大量短生命周期 goroutine
// 2. 无法控制并发数量，可能导致 Redis 连接耗尽
// 3. goroutine 创建/销毁带来额外开销
//
// 新实现使用固定大小的工作池：
// 1. 预创建 10 个 worker goroutine，避免频繁创建销毁
// 2. 使用带缓冲的 channel（1000）作为任务队列，平滑写入峰值
// 3. 非阻塞写入，队列满时关键任务同步回退，非关键任务丢弃并告警
// 4. 统一超时控制，避免慢操作阻塞工作池
const (
	cacheWriteWorkerCount     = 10              // 工作协程数量
	cacheWriteBufferSize      = 1000            // 任务队列缓冲大小
	cacheWriteTimeout         = 2 * time.Second // 单个写入操作超时
	cacheWriteDropLogInterval = 5 * time.Second // 丢弃日志节流间隔
	balanceLoadTimeout        = 3 * time.Second
)

// cacheWriteTask 缓存写入任务
type cacheWriteTask struct {
	kind             cacheWriteKind
	userID           int64
	groupID          int64
	apiKeyID         int64
	balance          float64
	amount           float64
	subscriptionData *subscriptionCacheData
}

// apiKeyRateLimitLoader defines the interface for loading rate limit data from DB.
type apiKeyRateLimitLoader interface {
	GetRateLimitData(ctx context.Context, keyID int64) (*APIKeyRateLimitData, error)
}

type carpoolRuntimeLimitLoader interface {
	GetRuntimeMemberLimitByGroupAndUser(ctx context.Context, groupID, userID int64, now time.Time) (*CarpoolRuntimeMemberLimit, error)
}

type subscriptionCacheInvalidationPubSub interface {
	PublishSubscriptionCacheInvalidation(ctx context.Context, cacheKey string) error
	SubscribeSubscriptionCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error
}

// BillingCacheService 计费缓存服务
// 负责余额和订阅数据的缓存管理，提供高性能的计费资格检查
type BillingCacheService struct {
	cache                 BillingCache
	userRepo              UserRepository
	subRepo               UserSubscriptionRepository
	apiKeyRateLimitLoader apiKeyRateLimitLoader
	carpoolRuntimeLoader  carpoolRuntimeLimitLoader
	userRPMCache          UserRPMCache
	userGroupRateRepo     UserGroupRateRepository
	cfg                   *config.Config
	circuitBreaker        *billingCircuitBreaker
	userPlatformQuotaRepo UserPlatformQuotaRepository

	cacheWriteChan     chan cacheWriteTask
	cacheWriteWg       sync.WaitGroup
	cacheWriteStopOnce sync.Once
	cacheWriteMu       sync.RWMutex
	stopped            atomic.Bool
	balanceLoadSF      singleflight.Group
	quotaLoadSF        singleflight.Group
	// 丢弃日志节流计数器（减少高负载下日志噪音）
	cacheWriteDropFullCount     uint64
	cacheWriteDropFullLastLog   int64
	cacheWriteDropClosedCount   uint64
	cacheWriteDropClosedLastLog int64
}

// NewBillingCacheService 创建计费缓存服务
func NewBillingCacheService(deps ...any) *BillingCacheService {
	var (
		cache                 BillingCache
		userRepo              UserRepository
		subRepo               UserSubscriptionRepository
		apiKeyRepo            APIKeyRepository
		carpoolRepo           carpoolRuntimeLimitLoader
		userRPMCache          UserRPMCache
		userGroupRateRepo     UserGroupRateRepository
		userPlatformQuotaRepo UserPlatformQuotaRepository
		cfg                   *config.Config
	)
	for _, dep := range deps {
		if dep == nil {
			continue
		}
		if value, ok := dep.(BillingCache); ok {
			cache = value
		}
		if value, ok := dep.(UserRepository); ok {
			userRepo = value
		}
		if value, ok := dep.(UserSubscriptionRepository); ok {
			subRepo = value
		}
		if value, ok := dep.(APIKeyRepository); ok {
			apiKeyRepo = value
		}
		if value, ok := dep.(carpoolRuntimeLimitLoader); ok {
			carpoolRepo = value
		}
		if value, ok := dep.(UserRPMCache); ok {
			userRPMCache = value
		}
		if value, ok := dep.(UserGroupRateRepository); ok {
			userGroupRateRepo = value
		}
		if value, ok := dep.(UserPlatformQuotaRepository); ok {
			userPlatformQuotaRepo = value
		}
		if value, ok := dep.(*config.Config); ok {
			cfg = value
		}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	svc := &BillingCacheService{
		cache:                 cache,
		userRepo:              userRepo,
		subRepo:               subRepo,
		apiKeyRateLimitLoader: apiKeyRepo,
		carpoolRuntimeLoader:  carpoolRepo,
		userRPMCache:          userRPMCache,
		userGroupRateRepo:     userGroupRateRepo,
		userPlatformQuotaRepo: userPlatformQuotaRepo,
		cfg:                   cfg,
	}
	svc.circuitBreaker = newBillingCircuitBreaker(cfg.Billing.CircuitBreaker)
	svc.startCacheWriteWorkers()
	return svc
}

// Stop 关闭缓存写入工作池
func (s *BillingCacheService) Stop() {
	s.cacheWriteStopOnce.Do(func() {
		s.stopped.Store(true)

		s.cacheWriteMu.Lock()
		ch := s.cacheWriteChan
		if ch != nil {
			close(ch)
		}
		s.cacheWriteMu.Unlock()

		if ch == nil {
			return
		}
		s.cacheWriteWg.Wait()

		s.cacheWriteMu.Lock()
		if s.cacheWriteChan == ch {
			s.cacheWriteChan = nil
		}
		s.cacheWriteMu.Unlock()
	})
}

func (s *BillingCacheService) startCacheWriteWorkers() {
	ch := make(chan cacheWriteTask, cacheWriteBufferSize)
	s.cacheWriteChan = ch
	for i := 0; i < cacheWriteWorkerCount; i++ {
		s.cacheWriteWg.Add(1)
		go s.cacheWriteWorker(ch)
	}
}

// enqueueCacheWrite 尝试将任务入队，队列满时返回 false（并记录告警）。
func (s *BillingCacheService) enqueueCacheWrite(task cacheWriteTask) (enqueued bool) {
	if s.stopped.Load() {
		s.logCacheWriteDrop(task, "closed")
		return false
	}

	s.cacheWriteMu.RLock()
	defer s.cacheWriteMu.RUnlock()

	if s.cacheWriteChan == nil {
		s.logCacheWriteDrop(task, "closed")
		return false
	}

	select {
	case s.cacheWriteChan <- task:
		return true
	default:
		// 队列满时不阻塞主流程，交由调用方决定是否同步回退。
		s.logCacheWriteDrop(task, "full")
		return false
	}
}

func (s *BillingCacheService) cacheWriteWorker(ch <-chan cacheWriteTask) {
	defer s.cacheWriteWg.Done()
	for task := range ch {
		ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
		switch task.kind {
		case cacheWriteSetBalance:
			s.setBalanceCache(ctx, task.userID, task.balance)
		case cacheWriteSetSubscription:
			s.setSubscriptionCache(ctx, task.userID, task.groupID, task.subscriptionData)
		case cacheWriteUpdateSubscriptionUsage:
			if s.cache != nil {
				if err := s.cache.UpdateSubscriptionUsage(ctx, task.userID, task.groupID, task.amount); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: update subscription cache failed for user %d group %d: %v", task.userID, task.groupID, err)
				}
			}
		case cacheWriteDeductBalance:
			if s.cache != nil {
				if err := s.cache.DeductUserBalance(ctx, task.userID, task.amount); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: deduct balance cache failed for user %d: %v", task.userID, err)
				}
			}
		case cacheWriteUpdateRateLimitUsage:
			if s.cache != nil {
				if err := s.cache.UpdateAPIKeyRateLimitUsage(ctx, task.apiKeyID, task.amount); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: update rate limit usage cache failed for api key %d: %v", task.apiKeyID, err)
				}
			}
		}
		cancel()
	}
}

// cacheWriteKindName 用于日志中的任务类型标识，便于排查丢弃原因。
func cacheWriteKindName(kind cacheWriteKind) string {
	switch kind {
	case cacheWriteSetBalance:
		return "set_balance"
	case cacheWriteSetSubscription:
		return "set_subscription"
	case cacheWriteUpdateSubscriptionUsage:
		return "update_subscription_usage"
	case cacheWriteDeductBalance:
		return "deduct_balance"
	case cacheWriteUpdateRateLimitUsage:
		return "update_rate_limit_usage"
	default:
		return "unknown"
	}
}

// logCacheWriteDrop 使用节流方式记录丢弃情况，并汇总丢弃数量。
func (s *BillingCacheService) logCacheWriteDrop(task cacheWriteTask, reason string) {
	var (
		countPtr *uint64
		lastPtr  *int64
	)
	switch reason {
	case "full":
		countPtr = &s.cacheWriteDropFullCount
		lastPtr = &s.cacheWriteDropFullLastLog
	case "closed":
		countPtr = &s.cacheWriteDropClosedCount
		lastPtr = &s.cacheWriteDropClosedLastLog
	default:
		return
	}

	atomic.AddUint64(countPtr, 1)
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(lastPtr)
	if now-last < int64(cacheWriteDropLogInterval) {
		return
	}
	if !atomic.CompareAndSwapInt64(lastPtr, last, now) {
		return
	}
	dropped := atomic.SwapUint64(countPtr, 0)
	if dropped == 0 {
		return
	}
	logger.LegacyPrintf("service.billing_cache", "Warning: cache write queue %s, dropped %d tasks in last %s (latest kind=%s user %d group %d)",
		reason,
		dropped,
		cacheWriteDropLogInterval,
		cacheWriteKindName(task.kind),
		task.userID,
		task.groupID,
	)
}

// ============================================
// 余额缓存方法
// ============================================

// GetUserBalance 获取用户余额（优先从缓存读取）
func (s *BillingCacheService) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	if s.cache == nil {
		// Redis不可用，直接查询数据库
		return s.getUserBalanceFromDB(ctx, userID)
	}

	// 尝试从缓存读取
	balance, err := s.cache.GetUserBalance(ctx, userID)
	if err == nil {
		return balance, nil
	}

	// 缓存未命中：singleflight 合并同一 userID 的并发回源请求。
	value, err, _ := s.balanceLoadSF.Do(strconv.FormatInt(userID, 10), func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.Background(), balanceLoadTimeout)
		defer cancel()

		balance, err := s.getUserBalanceFromDB(loadCtx, userID)
		if err != nil {
			return nil, err
		}

		// 异步建立缓存
		_ = s.enqueueCacheWrite(cacheWriteTask{
			kind:    cacheWriteSetBalance,
			userID:  userID,
			balance: balance,
		})
		return balance, nil
	})
	if err != nil {
		return 0, err
	}
	balance, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected balance type: %T", value)
	}
	return balance, nil
}

// getUserBalanceFromDB 从数据库获取用户余额
func (s *BillingCacheService) getUserBalanceFromDB(ctx context.Context, userID int64) (float64, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get user balance: %w", err)
	}
	return user.Balance, nil
}

// setBalanceCache 设置余额缓存
func (s *BillingCacheService) setBalanceCache(ctx context.Context, userID int64, balance float64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.SetUserBalance(ctx, userID, balance); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: set balance cache failed for user %d: %v", userID, err)
	}
}

// DeductBalanceCache 扣减余额缓存（同步调用）
func (s *BillingCacheService) DeductBalanceCache(ctx context.Context, userID int64, amount float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.DeductUserBalance(ctx, userID, amount)
}

// QueueDeductBalance 异步扣减余额缓存
func (s *BillingCacheService) QueueDeductBalance(userID int64, amount float64) {
	if s.cache == nil {
		return
	}
	// 队列满时同步回退，避免关键扣减被静默丢弃。
	if s.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: userID,
		amount: amount,
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	if err := s.DeductBalanceCache(ctx, userID, amount); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: deduct balance cache fallback failed for user %d: %v", userID, err)
	}
}

// InvalidateUserBalance 失效用户余额缓存
func (s *BillingCacheService) InvalidateUserBalance(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateUserBalance(ctx, userID); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: invalidate balance cache failed for user %d: %v", userID, err)
		return err
	}
	return nil
}

// ============================================
// 订阅缓存方法
// ============================================

// GetSubscriptionStatus 获取订阅状态（优先从缓存读取）
func (s *BillingCacheService) GetSubscriptionStatus(ctx context.Context, userID, groupID int64) (*subscriptionCacheData, error) {
	if s.cache == nil {
		return s.getSubscriptionFromDB(ctx, userID, groupID)
	}

	// 尝试从缓存读取
	cacheData, err := s.cache.GetSubscriptionCache(ctx, userID, groupID)
	if err == nil && cacheData != nil {
		return s.convertFromPortsData(cacheData), nil
	}

	// 缓存未命中，从数据库读取
	data, err := s.getSubscriptionFromDB(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}

	// 异步建立缓存
	_ = s.enqueueCacheWrite(cacheWriteTask{
		kind:             cacheWriteSetSubscription,
		userID:           userID,
		groupID:          groupID,
		subscriptionData: data,
	})

	return data, nil
}

func (s *BillingCacheService) convertFromPortsData(data *SubscriptionCacheData) *subscriptionCacheData {
	return &subscriptionCacheData{
		Status:       data.Status,
		ExpiresAt:    data.ExpiresAt,
		DailyUsage:   data.DailyUsage,
		WeeklyUsage:  data.WeeklyUsage,
		MonthlyUsage: data.MonthlyUsage,
		Version:      data.Version,
	}
}

func (s *BillingCacheService) convertToPortsData(data *subscriptionCacheData) *SubscriptionCacheData {
	return &SubscriptionCacheData{
		Status:       data.Status,
		ExpiresAt:    data.ExpiresAt,
		DailyUsage:   data.DailyUsage,
		WeeklyUsage:  data.WeeklyUsage,
		MonthlyUsage: data.MonthlyUsage,
		Version:      data.Version,
	}
}

// getSubscriptionFromDB 从数据库获取订阅数据
func (s *BillingCacheService) getSubscriptionFromDB(ctx context.Context, userID, groupID int64) (*subscriptionCacheData, error) {
	sub, err := s.subRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	return &subscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		DailyUsage:   sub.DailyUsageUSD,
		WeeklyUsage:  sub.WeeklyUsageUSD,
		MonthlyUsage: sub.MonthlyUsageUSD,
		Version:      sub.UpdatedAt.Unix(),
	}, nil
}

// setSubscriptionCache 设置订阅缓存
func (s *BillingCacheService) setSubscriptionCache(ctx context.Context, userID, groupID int64, data *subscriptionCacheData) {
	if s.cache == nil || data == nil {
		return
	}
	if err := s.cache.SetSubscriptionCache(ctx, userID, groupID, s.convertToPortsData(data)); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: set subscription cache failed for user %d group %d: %v", userID, groupID, err)
	}
}

// UpdateSubscriptionUsage 更新订阅用量缓存（同步调用）
func (s *BillingCacheService) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, costUSD float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD)
}

// QueueUpdateSubscriptionUsage 异步更新订阅用量缓存
func (s *BillingCacheService) QueueUpdateSubscriptionUsage(userID, groupID int64, costUSD float64) {
	if s.cache == nil {
		return
	}
	// 队列满时同步回退，确保订阅用量及时更新。
	if s.enqueueCacheWrite(cacheWriteTask{
		kind:    cacheWriteUpdateSubscriptionUsage,
		userID:  userID,
		groupID: groupID,
		amount:  costUSD,
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	if err := s.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: update subscription cache fallback failed for user %d group %d: %v", userID, groupID, err)
	}
}

// InvalidateSubscription 失效指定订阅缓存
func (s *BillingCacheService) InvalidateSubscription(ctx context.Context, userID, groupID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateSubscriptionCache(ctx, userID, groupID); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: invalidate subscription cache failed for user %d group %d: %v", userID, groupID, err)
		return err
	}
	return nil
}

func (s *BillingCacheService) PublishSubscriptionCacheInvalidation(ctx context.Context, cacheKey string) error {
	if s.cache == nil {
		return nil
	}
	pubsub, ok := s.cache.(subscriptionCacheInvalidationPubSub)
	if !ok {
		return nil
	}
	return pubsub.PublishSubscriptionCacheInvalidation(ctx, cacheKey)
}

func (s *BillingCacheService) SubscribeSubscriptionCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	if s.cache == nil {
		return nil
	}
	pubsub, ok := s.cache.(subscriptionCacheInvalidationPubSub)
	if !ok {
		return nil
	}
	return pubsub.SubscribeSubscriptionCacheInvalidation(ctx, handler)
}

// InvalidateAPIKeyRateLimit invalidates the Redis rate-limit usage cache for an API key.
func (s *BillingCacheService) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateAPIKeyRateLimit(ctx, keyID); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: invalidate api key rate limit cache failed for key %d: %v", keyID, err)
		return err
	}
	return nil
}

// ============================================
// API Key 限速缓存方法
// ============================================

// checkAPIKeyRateLimits checks rate limit windows for an API key.
// It loads usage from Redis cache (falling back to DB on cache miss),
// resets expired windows in-memory and triggers async DB reset,
// and returns an error if any window limit is exceeded.
func (s *BillingCacheService) checkAPIKeyRateLimits(ctx context.Context, apiKey *APIKey) error {
	if s.cache == nil {
		// No cache: fall back to reading from DB directly
		if s.apiKeyRateLimitLoader == nil {
			return nil
		}
		data, err := s.apiKeyRateLimitLoader.GetRateLimitData(ctx, apiKey.ID)
		if err != nil {
			return nil // Don't block requests on DB errors
		}
		return s.evaluateRateLimits(ctx, apiKey, data.Usage5h, data.Usage1d, data.Usage7d,
			data.Window5hStart, data.Window1dStart, data.Window7dStart)
	}

	cacheData, err := s.cache.GetAPIKeyRateLimit(ctx, apiKey.ID)
	if err != nil {
		// Cache miss: load from DB and populate cache
		if s.apiKeyRateLimitLoader == nil {
			return nil
		}
		dbData, dbErr := s.apiKeyRateLimitLoader.GetRateLimitData(ctx, apiKey.ID)
		if dbErr != nil {
			return nil // Don't block requests on DB errors
		}
		// Build cache entry from DB data
		cacheEntry := &APIKeyRateLimitCacheData{
			Usage5h: dbData.Usage5h,
			Usage1d: dbData.Usage1d,
			Usage7d: dbData.Usage7d,
		}
		if dbData.Window5hStart != nil {
			cacheEntry.Window5h = dbData.Window5hStart.Unix()
		}
		if dbData.Window1dStart != nil {
			cacheEntry.Window1d = dbData.Window1dStart.Unix()
		}
		if dbData.Window7dStart != nil {
			cacheEntry.Window7d = dbData.Window7dStart.Unix()
		}
		_ = s.cache.SetAPIKeyRateLimit(ctx, apiKey.ID, cacheEntry)
		cacheData = cacheEntry
	}

	var w5h, w1d, w7d *time.Time
	if cacheData.Window5h > 0 {
		t := time.Unix(cacheData.Window5h, 0)
		w5h = &t
	}
	if cacheData.Window1d > 0 {
		t := time.Unix(cacheData.Window1d, 0)
		w1d = &t
	}
	if cacheData.Window7d > 0 {
		t := time.Unix(cacheData.Window7d, 0)
		w7d = &t
	}
	return s.evaluateRateLimits(ctx, apiKey, cacheData.Usage5h, cacheData.Usage1d, cacheData.Usage7d, w5h, w1d, w7d)
}

// evaluateRateLimits checks usage against limits, triggering async resets for expired windows.
func (s *BillingCacheService) evaluateRateLimits(ctx context.Context, apiKey *APIKey, usage5h, usage1d, usage7d float64, w5h, w1d, w7d *time.Time) error {
	needsReset := false

	// Reset expired windows in-memory for check purposes
	if IsWindowExpired(w5h, RateLimitWindow5h) {
		usage5h = 0
		needsReset = true
	}
	if IsWindowExpired(w1d, RateLimitWindow1d) {
		usage1d = 0
		needsReset = true
	}
	if IsWindowExpired(w7d, RateLimitWindow7d) {
		usage7d = 0
		needsReset = true
	}

	// Trigger async DB reset if any window expired
	if needsReset {
		keyID := apiKey.ID
		go func() {
			resetCtx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
			defer cancel()
			if s.apiKeyRateLimitLoader != nil {
				// Use the repo directly - reset then reload cache
				if loader, ok := s.apiKeyRateLimitLoader.(interface {
					ResetRateLimitWindows(ctx context.Context, id int64) error
				}); ok {
					if err := loader.ResetRateLimitWindows(resetCtx, keyID); err != nil {
						logger.LegacyPrintf("service.billing_cache", "Warning: reset rate limit windows failed for api key %d: %v", keyID, err)
					}
				}
			}
			// Invalidate cache so next request loads fresh data
			if s.cache != nil {
				if err := s.cache.InvalidateAPIKeyRateLimit(resetCtx, keyID); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: invalidate rate limit cache failed for api key %d: %v", keyID, err)
				}
			}
		}()
	}

	// Check limits
	if apiKey.RateLimit5h > 0 && usage5h >= apiKey.RateLimit5h {
		return ErrAPIKeyRateLimit5hExceeded
	}
	if apiKey.RateLimit1d > 0 && usage1d >= apiKey.RateLimit1d {
		return ErrAPIKeyRateLimit1dExceeded
	}
	if apiKey.RateLimit7d > 0 && usage7d >= apiKey.RateLimit7d {
		return ErrAPIKeyRateLimit7dExceeded
	}
	return nil
}

// QueueUpdateAPIKeyRateLimitUsage asynchronously updates rate limit usage in the cache.
func (s *BillingCacheService) QueueUpdateAPIKeyRateLimitUsage(apiKeyID int64, cost float64) {
	if s.cache == nil {
		return
	}
	s.enqueueCacheWrite(cacheWriteTask{
		kind:     cacheWriteUpdateRateLimitUsage,
		apiKeyID: apiKeyID,
		amount:   cost,
	})
}

func (s *BillingCacheService) IncrementUserPlatformQuotaUsage(userID int64, platform string, cost float64) {
	if s == nil || s.cache == nil || userID <= 0 || cost <= 0 || !IsAllowedQuotaPlatform(platform) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	markDirty := s.cfg != nil && s.cfg.Database.UserPlatformQuotaFlusherEnabled
	if err := s.cache.IncrUserPlatformQuotaUsageCache(ctx, userID, platform, cost, s.userPlatformQuotaCacheTTL(), markDirty); err != nil {
		logger.LegacyPrintf("service.billing_cache", "ALERT: increment user platform quota cache failed user=%d platform=%s cost=%f: %v", userID, platform, cost, err)
	}
}

func (s *BillingCacheService) InvalidateUserPlatformQuota(ctx context.Context, userID int64, platform string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.DeleteUserPlatformQuotaCache(ctx, userID, platform)
}

// ============================================
// 统一检查方法
// ============================================

// CheckBillingEligibility 检查用户是否有资格发起请求
// 余额模式：检查缓存余额 > 0
// 订阅模式：检查缓存用量未超过限额（Group限额从参数传入）
func (s *BillingCacheService) CheckBillingEligibility(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription, quotaPlatform ...string) error {
	platform := ""
	if len(quotaPlatform) > 0 {
		platform = quotaPlatform[0]
	}
	// 简易模式：跳过所有计费检查
	if s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	if s.circuitBreaker != nil && !s.circuitBreaker.Allow() {
		return ErrBillingServiceUnavailable
	}

	usesWallet := group == nil || !group.IsSubscriptionType() || group.IsUserPrivateScope()
	if group != nil && group.IsSubscriptionType() {
		if subscription == nil {
			return ErrSubscriptionNotFound
		}
		if err := s.checkSubscriptionEligibility(ctx, user.ID, group, subscription); err != nil {
			return err
		}
		if group.IsUserPrivateScope() {
			if err := s.checkBalanceOrPointsEligibility(ctx, user); err != nil {
				return err
			}
		}
	} else {
		if err := s.checkBalanceOrPointsEligibility(ctx, user); err != nil {
			return err
		}
	}
	if usesWallet {
		if err := s.checkUserPlatformQuotaEligibility(ctx, user.ID, platform); err != nil {
			return err
		}
	}

	// Check API Key rate limits (applies to both billing modes)
	if apiKey != nil && apiKey.HasRateLimits() {
		if err := s.checkAPIKeyRateLimits(ctx, apiKey); err != nil {
			return err
		}
	}

	// RPM 限流：级联回落（Override → Group → User），放在最后以避免为注定失败的请求增加计数。
	if err := s.checkRPM(ctx, user, group); err != nil {
		return err
	}

	return nil
}

func (s *BillingCacheService) checkUserPlatformQuotaEligibility(ctx context.Context, userID int64, platform string) error {
	if s == nil || s.userPlatformQuotaRepo == nil || userID <= 0 || !IsAllowedQuotaPlatform(platform) {
		return nil
	}
	var (
		entry    *UserPlatformQuotaCacheEntry
		hit      bool
		cacheErr error
	)
	if s.cache != nil {
		entry, hit, cacheErr = s.cache.GetUserPlatformQuotaCache(ctx, userID, platform)
	} else {
		cacheErr = errBillingCacheUnavailable
	}
	if cacheErr == nil && hit && entry != nil && entry.SchemaVersion == UserPlatformQuotaCacheSchemaV1 {
		normalized, changed := normalizePlatformQuotaEntry(entry, time.Now())
		if changed && s.cache != nil && hasPlatformQuotaLimit(normalized) {
			setCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			_ = s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, normalized, s.userPlatformQuotaCacheTTL())
			cancel()
		}
		return enforcePlatformQuota(normalized, time.Now())
	}

	key := strconv.FormatInt(userID, 10) + ":" + platform
	load := s.quotaLoadSF.DoChan(key, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.userPlatformQuotaRepo.GetByUserPlatform(loadCtx, userID, platform)
	})
	var result singleflight.Result
	select {
	case result = <-load:
	case <-ctx.Done():
		return nil
	}
	if result.Err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: load user platform quota failed user=%d platform=%s: %v (fail-open)", userID, platform, result.Err)
		return nil
	}
	record, _ := result.Val.(*UserPlatformQuotaRecord)
	if record == nil {
		if s.cache != nil && cacheErr == nil {
			now := time.Now()
			day := timezone.StartOfDay(now)
			week := timezone.StartOfWeek(now)
			sentinel := &UserPlatformQuotaCacheEntry{SchemaVersion: UserPlatformQuotaCacheSchemaV1, DailyWindowStart: &day, WeeklyWindowStart: &week, MonthlyWindowStart: &now}
			setCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, sentinel, s.userPlatformQuotaSentinelTTL()); setErr != nil {
				userPlatformQuotaSentinelSetCacheErrorTotal.Add(1)
				logger.LegacyPrintf("service.billing_cache", "Warning: set sentinel quota cache failed user=%d platform=%s: %v", userID, platform, setErr)
			}
			cancel()
		}
		return nil
	}

	now := time.Now()
	entry = platformQuotaEntryFromRecord(record)
	entry, _ = normalizePlatformQuotaEntry(entry, now)
	if s.cache != nil && cacheErr == nil {
		setCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_ = s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, entry, s.userPlatformQuotaCacheTTL())
		cancel()
	}
	return enforcePlatformQuota(entry, now)
}

func (s *BillingCacheService) userPlatformQuotaCacheTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds > 0 {
		return time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
	}
	return 24 * time.Hour
}

func (s *BillingCacheService) userPlatformQuotaSentinelTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds > 0 {
		return time.Duration(s.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds) * time.Second
	}
	return time.Hour
}

func platformQuotaEntryFromRecord(record *UserPlatformQuotaRecord) *UserPlatformQuotaCacheEntry {
	return &UserPlatformQuotaCacheEntry{
		DailyUsageUSD: record.DailyUsageUSD, WeeklyUsageUSD: record.WeeklyUsageUSD, MonthlyUsageUSD: record.MonthlyUsageUSD,
		SchemaVersion: UserPlatformQuotaCacheSchemaV1,
		DailyLimitUSD: record.DailyLimitUSD, WeeklyLimitUSD: record.WeeklyLimitUSD, MonthlyLimitUSD: record.MonthlyLimitUSD,
		DailyWindowStart: record.DailyWindowStart, WeeklyWindowStart: record.WeeklyWindowStart, MonthlyWindowStart: record.MonthlyWindowStart,
	}
}

func normalizePlatformQuotaEntry(entry *UserPlatformQuotaCacheEntry, now time.Time) (*UserPlatformQuotaCacheEntry, bool) {
	if entry == nil {
		return nil, false
	}
	copy := *entry
	changed := false
	day := timezone.StartOfDay(now)
	week := timezone.StartOfWeek(now)
	if copy.DailyWindowStart == nil || copy.DailyWindowStart.Before(day) {
		copy.DailyUsageUSD = 0
		copy.DailyWindowStart = &day
		changed = true
	}
	if copy.WeeklyWindowStart == nil || copy.WeeklyWindowStart.Before(week) {
		copy.WeeklyUsageUSD = 0
		copy.WeeklyWindowStart = &week
		changed = true
	}
	if copy.MonthlyWindowStart == nil || now.Sub(*copy.MonthlyWindowStart) >= 30*24*time.Hour {
		copy.MonthlyUsageUSD = 0
		copy.MonthlyWindowStart = &now
		changed = true
	}
	return &copy, changed
}

func hasPlatformQuotaLimit(entry *UserPlatformQuotaCacheEntry) bool {
	return entry != nil && (entry.DailyLimitUSD != nil || entry.WeeklyLimitUSD != nil || entry.MonthlyLimitUSD != nil)
}

func enforcePlatformQuota(entry *UserPlatformQuotaCacheEntry, now time.Time) error {
	if entry == nil {
		return nil
	}
	if entry.DailyLimitUSD != nil && entry.DailyUsageUSD >= *entry.DailyLimitUSD {
		return quotaErrorWithReset(ErrUserPlatformDailyQuotaExhausted, timezone.StartOfDay(now).AddDate(0, 0, 1))
	}
	if entry.WeeklyLimitUSD != nil && entry.WeeklyUsageUSD >= *entry.WeeklyLimitUSD {
		return quotaErrorWithReset(ErrUserPlatformWeeklyQuotaExhausted, timezone.StartOfWeek(now).AddDate(0, 0, 7))
	}
	if entry.MonthlyLimitUSD != nil && entry.MonthlyUsageUSD >= *entry.MonthlyLimitUSD {
		resetAt := now.Add(30 * 24 * time.Hour)
		if entry.MonthlyWindowStart != nil && now.Sub(*entry.MonthlyWindowStart) < 30*24*time.Hour {
			resetAt = entry.MonthlyWindowStart.Add(30 * 24 * time.Hour)
		}
		return quotaErrorWithReset(ErrUserPlatformMonthlyQuotaExhausted, resetAt)
	}
	return nil
}

func quotaErrorWithReset(err error, resetAt time.Time) error {
	if appErr, ok := err.(*infraerrors.ApplicationError); ok && appErr != nil {
		return appErr.WithMetadata(map[string]string{"window_resets_at": resetAt.Format(time.RFC3339)})
	}
	return err
}

func (s *BillingCacheService) HasUserPlatformQuotaLimit(ctx context.Context, userID int64, platform string) bool {
	if s == nil || s.cfg == nil || s.cfg.RunMode == config.RunModeSimple || !IsAllowedQuotaPlatform(platform) {
		return false
	}
	if s.cache == nil {
		return true
	}
	entry, hit, err := s.cache.GetUserPlatformQuotaCache(ctx, userID, platform)
	return err != nil || !hit || entry == nil || hasPlatformQuotaLimit(entry)
}

// checkRPM 执行并行 RPM 限流，所有适用的限制同时生效，任一超限即拒绝：
//
//  1. (用户, 分组) rpm_override       — 最细粒度：管理员为特定用户在特定分组设定的专属限额。
//     override=0 表示该用户在该分组免检（绿灯），但 user 级全局上限仍然生效。
//  2. group.rpm_limit                 — 分组级：该分组的统一 RPM 容量（仅当无 override 时生效）。
//  3. user.rpm_limit                  — 用户级全局硬上限：无论 override/group 如何配置，始终生效。
//
// 与旧版"级联互斥"设计不同，新版确保 user.rpm_limit 作为全局天花板不会被 group 或 override 覆盖。
// Redis 故障一律 fail-open（打 warning，不阻塞业务）。
func (s *BillingCacheService) checkRPM(ctx context.Context, user *User, group *Group) error {
	if s == nil || s.userRPMCache == nil || user == nil {
		return nil
	}

	// ── 第一层：分组级检查（override 或 group.rpm_limit） ──
	if group != nil {
		// 解析 override：优先从 auth cache snapshot，nil 时回退 DB。
		var override *int
		if user.UserGroupRPMOverride != nil {
			override = user.UserGroupRPMOverride
		} else if s.userGroupRateRepo != nil {
			dbOverride, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
			if err != nil {
				logger.LegacyPrintf(
					"service.billing_cache",
					"Warning: rpm override lookup failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
			} else {
				override = dbOverride
			}
		}

		if override != nil {
			// override=0 → 该用户在该分组免检（但 user 级仍会在下面检查）。
			if *override > 0 {
				count, incErr := s.userRPMCache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
				if incErr != nil {
					logger.LegacyPrintf(
						"service.billing_cache",
						"Warning: rpm increment (override) failed for user=%d group=%d: %v",
						user.ID, group.ID, incErr,
					)
					// fail-open
				} else if count > *override {
					return ErrGroupRPMExceeded
				}
			}
			// override 命中后跳过 group.rpm_limit（override 替代 group），但不 return——继续检查 user 级。
		} else if group.RPMLimit > 0 {
			// 无 override，检查 group.rpm_limit。
			count, err := s.userRPMCache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
			if err != nil {
				logger.LegacyPrintf(
					"service.billing_cache",
					"Warning: rpm increment (group) failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
				// fail-open
			} else if count > group.RPMLimit {
				return ErrGroupRPMExceeded
			}
		}
	}

	// ── 第二层：用户级全局硬上限（始终生效） ──
	if user.RPMLimit > 0 {
		count, err := s.userRPMCache.IncrementUserRPM(ctx, user.ID)
		if err != nil {
			logger.LegacyPrintf(
				"service.billing_cache",
				"Warning: rpm increment (user) failed for user=%d: %v",
				user.ID, err,
			)
			return nil // fail-open
		}
		if count > user.RPMLimit {
			return ErrUserRPMExceeded
		}
	}

	return nil
}

func (s *BillingCacheService) minimumBalanceReserve() float64 {
	if s == nil || s.cfg == nil || s.cfg.Billing.MinimumBalanceReserve <= 0 {
		return 0
	}
	return s.cfg.Billing.MinimumBalanceReserve
}

func (s *BillingCacheService) balanceBelowEligibilityThreshold(balance float64) bool {
	if balance <= 0 {
		return true
	}
	minimumReserve := s.minimumBalanceReserve()
	return minimumReserve > 0 && balance < minimumReserve
}

// checkBalanceEligibility 检查余额模式资格
func (s *BillingCacheService) checkBalanceEligibility(ctx context.Context, userID int64) error {
	balance, err := s.GetUserBalance(ctx, userID)
	if err != nil {
		if s.circuitBreaker != nil {
			s.circuitBreaker.OnFailure(err)
		}
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing balance check failed for user %d: %v", userID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.circuitBreaker != nil {
		s.circuitBreaker.OnSuccess()
	}

	if s.balanceBelowEligibilityThreshold(balance) {
		return ErrInsufficientBalance
	}

	return nil
}

// checkBalanceOrPointsEligibility allows requests when either withdrawable balance
// or user-enabled points can pay for the next usage request.
func (s *BillingCacheService) checkBalanceOrPointsEligibility(ctx context.Context, user *User) error {
	if user == nil {
		return ErrInsufficientBalance
	}
	if CanUsePointsForUsage(user) {
		return nil
	}
	return s.checkBalanceEligibility(ctx, user.ID)
}

// checkSubscriptionEligibility 检查订阅模式资格
func (s *BillingCacheService) checkSubscriptionEligibility(ctx context.Context, userID int64, group *Group, subscription *UserSubscription) error {
	// 获取订阅缓存数据
	subData, err := s.GetSubscriptionStatus(ctx, userID, group.ID)
	if err != nil {
		if s.circuitBreaker != nil {
			s.circuitBreaker.OnFailure(err)
		}
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing subscription check failed for user %d group %d: %v", userID, group.ID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.circuitBreaker != nil {
		s.circuitBreaker.OnSuccess()
	}

	// 检查订阅状态
	if subData.Status != SubscriptionStatusActive {
		return ErrSubscriptionInvalid
	}

	// 检查是否过期
	if time.Now().After(subData.ExpiresAt) {
		return ErrSubscriptionInvalid
	}

	var runtimeLimit *CarpoolRuntimeMemberLimit
	if s.carpoolRuntimeLoader != nil {
		var err error
		runtimeLimit, err = s.carpoolRuntimeLoader.GetRuntimeMemberLimitByGroupAndUser(ctx, group.ID, userID, time.Now())
		if err != nil {
			return ErrBillingServiceUnavailable.WithCause(err)
		}
	}

	// 检查限额（使用传入的Group限额配置；拼车池周限额优先使用成员自己的分配额度）
	if group.HasDailyLimit() && subData.DailyUsage >= *group.DailyLimitUSD {
		return ErrDailyLimitExceeded
	}

	if runtimeLimit != nil && runtimeLimit.WeeklyLimitUSD > 0 {
		if subData.WeeklyUsage >= runtimeLimit.WeeklyLimitUSD {
			return ErrWeeklyLimitExceeded
		}
	} else if group.HasWeeklyLimit() && subData.WeeklyUsage >= *group.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}

	if group.HasMonthlyLimit() && subData.MonthlyUsage >= *group.MonthlyLimitUSD {
		return ErrMonthlyLimitExceeded
	}
	// Carpool 5h usage is displayed only. Weekly usage is the hard member
	// boundary, so the legacy USD 5h fields must not reject requests here.

	return nil
}

type billingCircuitBreakerState int

const (
	billingCircuitClosed billingCircuitBreakerState = iota
	billingCircuitOpen
	billingCircuitHalfOpen
)

type billingCircuitBreaker struct {
	mu                sync.Mutex
	state             billingCircuitBreakerState
	failures          int
	openedAt          time.Time
	failureThreshold  int
	resetTimeout      time.Duration
	halfOpenRequests  int
	halfOpenRemaining int
}

func newBillingCircuitBreaker(cfg config.CircuitBreakerConfig) *billingCircuitBreaker {
	if !cfg.Enabled {
		return nil
	}
	resetTimeout := time.Duration(cfg.ResetTimeoutSeconds) * time.Second
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	halfOpen := cfg.HalfOpenRequests
	if halfOpen <= 0 {
		halfOpen = 1
	}
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 5
	}
	return &billingCircuitBreaker{
		state:            billingCircuitClosed,
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		halfOpenRequests: halfOpen,
	}
}

func (b *billingCircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case billingCircuitClosed:
		return true
	case billingCircuitOpen:
		if time.Since(b.openedAt) < b.resetTimeout {
			return false
		}
		b.state = billingCircuitHalfOpen
		b.halfOpenRemaining = b.halfOpenRequests
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker entering half-open state")
		fallthrough
	case billingCircuitHalfOpen:
		if b.halfOpenRemaining <= 0 {
			return false
		}
		b.halfOpenRemaining--
		return true
	default:
		return false
	}
}

func (b *billingCircuitBreaker) OnFailure(err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case billingCircuitOpen:
		return
	case billingCircuitHalfOpen:
		b.state = billingCircuitOpen
		b.openedAt = time.Now()
		b.halfOpenRemaining = 0
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker opened after half-open failure: %v", err)
		return
	default:
		b.failures++
		if b.failures >= b.failureThreshold {
			b.state = billingCircuitOpen
			b.openedAt = time.Now()
			b.halfOpenRemaining = 0
			logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker opened after %d failures: %v", b.failures, err)
		}
	}
}

func (b *billingCircuitBreaker) OnSuccess() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	previousState := b.state
	previousFailures := b.failures

	b.state = billingCircuitClosed
	b.failures = 0
	b.halfOpenRemaining = 0

	// 只有状态真正发生变化时才记录日志
	if previousState != billingCircuitClosed {
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker closed (was %s)", circuitStateString(previousState))
	} else if previousFailures > 0 {
		logger.LegacyPrintf("service.billing_cache", "INFO: billing circuit breaker failures reset from %d", previousFailures)
	}
}

func circuitStateString(state billingCircuitBreakerState) string {
	switch state {
	case billingCircuitClosed:
		return "closed"
	case billingCircuitOpen:
		return "open"
	case billingCircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
