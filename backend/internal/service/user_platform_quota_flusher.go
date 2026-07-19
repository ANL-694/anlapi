package service

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"ikik-api/internal/config"
	"ikik-api/internal/pkg/logger"
)

// quotaDirtyCache 是 flusher 依赖的窄接口（来自 BillingCache）。
type quotaDirtyCache interface {
	PopDirtyUserPlatformQuotaKeys(ctx context.Context, n int) ([]UserPlatformQuotaKey, error)
	ReaddDirtyUserPlatformQuotaKeys(ctx context.Context, keys []UserPlatformQuotaKey) error
	BatchGetUserPlatformQuotaCache(ctx context.Context, keys []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error)
}

// quotaSnapshotWriter 是 flusher 依赖的 DB 写入窄接口。
type quotaSnapshotWriter interface {
	BatchSnapshotUsage(ctx context.Context, snapshots []UserPlatformQuotaSnapshot, now time.Time) error
}

// FlusherMetrics 记录 flusher 运行时指标（原子量，零值可用）。
type FlusherMetrics struct {
	FlushSuccessTotal     atomic.Int64
	FlushErrorTotal       atomic.Int64
	FlushBatchSizeTotal   atomic.Int64
	FlushLatencyMsMax     atomic.Int64
	DirtyReaddTotal       atomic.Int64
	DirtyLostTotal        atomic.Int64
	FlushFKViolationTotal atomic.Int64
}

const flusherMaxBatchesPerTick = 16

// maxFlushBatchSize 必须不大于 repository.BatchSnapshotUsage 的单批上限。
const maxFlushBatchSize = 6000

const defaultFlushBatchSize = 1000

// UserPlatformQuotaUsageFlusher 将 Redis 脏集快照定期批量写入 DB。
type UserPlatformQuotaUsageFlusher struct {
	cache        quotaDirtyCache
	quotaRepo    quotaSnapshotWriter
	timingWheel  *TimingWheelService
	enabled      bool
	interval     time.Duration
	batchSize    int
	flushTimeout time.Duration
	metrics      *FlusherMetrics
	stopped      atomic.Bool
}

// NewUserPlatformQuotaUsageFlusher 创建 UserPlatformQuotaUsageFlusher。
func NewUserPlatformQuotaUsageFlusher(cfg *config.Config, cache BillingCache, quotaRepo UserPlatformQuotaRepository, tw *TimingWheelService) *UserPlatformQuotaUsageFlusher {
	batchSize := cfg.Database.UserPlatformQuotaFlushBatchSize
	if batchSize <= 0 {
		batchSize = defaultFlushBatchSize
	}
	if batchSize > maxFlushBatchSize {
		logger.LegacyPrintf("quota_flusher",
			"[QuotaFlusher] flush_batch_size %d 超过上限 %d,已 clamp(避免 BatchSnapshotUsage 多子批非原子)",
			cfg.Database.UserPlatformQuotaFlushBatchSize, maxFlushBatchSize)
		batchSize = maxFlushBatchSize
	}
	interval := time.Duration(cfg.Database.UserPlatformQuotaFlushIntervalMs) * time.Millisecond
	if interval <= 0 {
		logger.LegacyPrintf("quota_flusher", "[QuotaFlusher] flush_interval_ms %d 非法,回退 2000ms", cfg.Database.UserPlatformQuotaFlushIntervalMs)
		interval = 2 * time.Second
	}
	return &UserPlatformQuotaUsageFlusher{
		cache:        cache,
		quotaRepo:    quotaRepo,
		timingWheel:  tw,
		enabled:      cfg.Database.UserPlatformQuotaFlusherEnabled,
		interval:     interval,
		batchSize:    batchSize,
		flushTimeout: 3 * time.Second,
		metrics:      &FlusherMetrics{},
	}
}

func (s *UserPlatformQuotaUsageFlusher) updateLatencyMax(ms int64) {
	for {
		old := s.metrics.FlushLatencyMsMax.Load()
		if ms <= old {
			return
		}
		if s.metrics.FlushLatencyMsMax.CompareAndSwap(old, ms) {
			return
		}
	}
}

func (s *UserPlatformQuotaUsageFlusher) readdOrCountLost(ctx context.Context, keys []UserPlatformQuotaKey, stage string) {
	if err := s.cache.ReaddDirtyUserPlatformQuotaKeys(ctx, keys); err != nil {
		s.metrics.DirtyLostTotal.Add(int64(len(keys)))
		logger.LegacyPrintf("quota_flusher", "[QuotaFlusher] ALERT: Readd after %s failed, %d keys 丢出脏集(DB 镜像缺这批,Redis 仍权威,活跃 key 下次 SADD 自愈): %v", stage, len(keys), err)
		return
	}
	s.metrics.DirtyReaddTotal.Add(int64(len(keys)))
}

// flushOneBatch 处理单批：Pop → BatchGet → 组装 snapshots → BatchSnapshotUsage。
func (s *UserPlatformQuotaUsageFlusher) flushOneBatch(parentCtx context.Context) bool {
	ctx, cancel := context.WithTimeout(parentCtx, s.flushTimeout)
	defer cancel()

	keys, err := s.cache.PopDirtyUserPlatformQuotaKeys(ctx, s.batchSize)
	if err != nil {
		s.metrics.FlushErrorTotal.Add(1)
		logger.LegacyPrintf("quota_flusher", "[QuotaFlusher] PopDirty error: %v", err)
		return false
	}
	if len(keys) == 0 {
		return false
	}

	entries, err := s.cache.BatchGetUserPlatformQuotaCache(ctx, keys)
	if err != nil {
		s.metrics.FlushErrorTotal.Add(1)
		s.readdOrCountLost(ctx, keys, "BatchGet")
		logger.LegacyPrintf("quota_flusher", "[QuotaFlusher] BatchGet error: %v", err)
		return false
	}

	snaps := make([]UserPlatformQuotaSnapshot, 0, len(keys))
	for i, key := range keys {
		if i >= len(entries) {
			break
		}
		e := entries[i]
		if e == nil || e.DailyWindowStart == nil || e.WeeklyWindowStart == nil || e.MonthlyWindowStart == nil {
			continue
		}
		snaps = append(snaps, UserPlatformQuotaSnapshot{
			UserID:             key.UserID,
			Platform:           key.Platform,
			DailyUsageUSD:      e.DailyUsageUSD,
			WeeklyUsageUSD:     e.WeeklyUsageUSD,
			MonthlyUsageUSD:    e.MonthlyUsageUSD,
			DailyWindowStart:   *e.DailyWindowStart,
			WeeklyWindowStart:  *e.WeeklyWindowStart,
			MonthlyWindowStart: *e.MonthlyWindowStart,
		})
	}

	if len(snaps) == 0 {
		return len(keys) >= s.batchSize
	}

	start := time.Now()
	writeErr := s.quotaRepo.BatchSnapshotUsage(ctx, snaps, time.Now().UTC())
	s.updateLatencyMax(time.Since(start).Milliseconds())
	if writeErr != nil {
		if errors.Is(writeErr, ErrUserPlatformQuotaFKViolation) {
			s.metrics.FlushFKViolationTotal.Add(1)
			s.metrics.FlushErrorTotal.Add(1)
			logger.LegacyPrintf("quota_flusher", "[QuotaFlusher] FK violation (dropped %d snaps): %v", len(snaps), writeErr)
		} else {
			s.metrics.FlushErrorTotal.Add(1)
			s.readdOrCountLost(ctx, keys, "BatchSnapshotUsage")
			logger.LegacyPrintf("quota_flusher", "[QuotaFlusher] BatchSnapshotUsage error: %v", writeErr)
		}
		return false
	}

	s.metrics.FlushSuccessTotal.Add(1)
	s.metrics.FlushBatchSizeTotal.Add(int64(len(snaps)))
	return len(keys) >= s.batchSize
}

func (s *UserPlatformQuotaUsageFlusher) flush() {
	if s == nil || s.cache == nil || s.quotaRepo == nil {
		return
	}
	parentCtx := context.Background()
	for batch := 0; batch < flusherMaxBatchesPerTick; batch++ {
		if !s.flushOneBatch(parentCtx) {
			return
		}
	}
	logger.LegacyPrintf("quota_flusher",
		"[QuotaFlusher] 单 tick 达到 max batches 上限(%d × batchSize=%d),脏集仍非空,积压顺延至下一 tick",
		flusherMaxBatchesPerTick, s.batchSize)
}

func (s *UserPlatformQuotaUsageFlusher) tick() {
	if s == nil || s.stopped.Load() {
		return
	}
	s.flush()
}

func (s *UserPlatformQuotaUsageFlusher) Start() {
	if s == nil || !s.enabled || s.timingWheel == nil {
		return
	}
	s.timingWheel.ScheduleRecurring("deferred:platform_quota", s.interval, s.tick)
}

func (s *UserPlatformQuotaUsageFlusher) Stop() {
	if s == nil {
		return
	}
	s.stopped.Store(true)
	if s.timingWheel != nil {
		s.timingWheel.Cancel("deferred:platform_quota")
	}
	s.flush()
}
