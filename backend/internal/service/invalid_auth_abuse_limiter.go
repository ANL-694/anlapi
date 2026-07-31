package service

import (
	"sync"
	"sync/atomic"
	"time"

	"anlapi/internal/config"
)

const invalidAuthAbuseShardCount = 16

type invalidAuthAbuseEntry struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

type invalidAuthAbuseShard struct {
	mu      sync.Mutex
	entries map[string]*invalidAuthAbuseEntry
}

type invalidAuthAbuseLimiter struct {
	threshold int
	window    time.Duration
	block     time.Duration
	capacity  int64
	shards    [invalidAuthAbuseShardCount]invalidAuthAbuseShard
	now       func() time.Time

	tracked       atomic.Int64
	recorded      atomic.Uint64
	blocked       atomic.Uint64
	rejected      atomic.Uint64
	expired       atomic.Uint64
	evicted       atomic.Uint64
	dropped       atomic.Uint64
	cleanupNext   atomic.Int64
	cleanupCursor atomic.Uint32
}

type InvalidAuthAbuseHealth struct {
	Enabled  bool   `json:"enabled"`
	Tracked  int64  `json:"tracked"`
	Capacity int64  `json:"capacity"`
	Recorded uint64 `json:"recorded"`
	Blocks   uint64 `json:"blocks"`
	Rejected uint64 `json:"rejected"`
	Expired  uint64 `json:"expired"`
	Evicted  uint64 `json:"evicted"`
	Dropped  uint64 `json:"dropped"`
}

func newInvalidAuthAbuseLimiter(cfg *config.Config) *invalidAuthAbuseLimiter {
	if cfg == nil || !cfg.APIKeyAuth.InvalidAbuse.Enabled {
		return nil
	}
	settings := cfg.APIKeyAuth.InvalidAbuse
	if settings.Threshold <= 0 || settings.WindowSeconds <= 0 || settings.BlockSeconds <= 0 || settings.Capacity <= 0 {
		return nil
	}
	limiter := &invalidAuthAbuseLimiter{
		threshold: settings.Threshold,
		window:    time.Duration(settings.WindowSeconds) * time.Second,
		block:     time.Duration(settings.BlockSeconds) * time.Second,
		capacity:  int64(settings.Capacity),
		now:       time.Now,
	}
	for shardIndex := range limiter.shards {
		limiter.shards[shardIndex].entries = make(map[string]*invalidAuthAbuseEntry)
	}
	return limiter
}

func (s *APIKeyService) CheckInvalidAuthAbuse(clientKey string) (time.Duration, bool) {
	if s == nil || s.invalidAuthAbuse == nil {
		return 0, false
	}
	return s.invalidAuthAbuse.check(clientKey)
}

func (s *APIKeyService) RecordInvalidAuthFailure(clientKey string) {
	if s == nil || s.invalidAuthAbuse == nil {
		return
	}
	s.invalidAuthAbuse.record(clientKey)
}

func (s *APIKeyService) InvalidAuthAbuseHealth() InvalidAuthAbuseHealth {
	if s == nil || s.invalidAuthAbuse == nil {
		return InvalidAuthAbuseHealth{}
	}
	return s.invalidAuthAbuse.health()
}

func (limiter *invalidAuthAbuseLimiter) check(clientKey string) (time.Duration, bool) {
	if limiter == nil || clientKey == "" {
		return 0, false
	}
	now := limiter.now()
	limiter.maybeCleanupAtCapacity(now)
	shard := limiter.shard(clientKey)
	shard.mu.Lock()
	entry := shard.entries[clientKey]
	if entry != nil && limiter.entryExpired(entry, now) {
		delete(shard.entries, clientKey)
		limiter.tracked.Add(-1)
		limiter.expired.Add(1)
		entry = nil
	}
	if entry != nil && entry.blockedUntil.After(now) {
		retry := entry.blockedUntil.Sub(now)
		shard.mu.Unlock()
		limiter.rejected.Add(1)
		return retry, true
	}
	shard.mu.Unlock()
	return 0, false
}

func (limiter *invalidAuthAbuseLimiter) record(clientKey string) {
	if limiter == nil || clientKey == "" {
		return
	}
	limiter.recorded.Add(1)
	now := limiter.now()
	limiter.maybeCleanupAtCapacity(now)
	shard := limiter.shard(clientKey)
	shard.mu.Lock()
	entry := shard.entries[clientKey]
	if entry != nil && limiter.entryExpired(entry, now) {
		delete(shard.entries, clientKey)
		limiter.tracked.Add(-1)
		limiter.expired.Add(1)
		entry = nil
	}
	if entry == nil {
		if !limiter.reserveEntry() {
			if !limiter.evictOldestUnblockedEntry(shard, now) {
				shard.mu.Unlock()
				limiter.dropped.Add(1)
				return
			}
		}
		entry = &invalidAuthAbuseEntry{windowStart: now}
		shard.entries[clientKey] = entry
	}
	if entry.blockedUntil.After(now) {
		shard.mu.Unlock()
		return
	}
	if entry.windowStart.After(now) || !now.Before(entry.windowStart.Add(limiter.window)) {
		entry.windowStart = now
		entry.failures = 0
	}
	entry.failures++
	if entry.failures >= limiter.threshold {
		entry.failures = 0
		entry.blockedUntil = now.Add(limiter.block)
		entry.windowStart = entry.blockedUntil
		limiter.blocked.Add(1)
	}
	shard.mu.Unlock()
}

func (limiter *invalidAuthAbuseLimiter) reserveEntry() bool {
	for {
		current := limiter.tracked.Load()
		if current >= limiter.capacity {
			return false
		}
		if limiter.tracked.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (limiter *invalidAuthAbuseLimiter) maybeCleanupAtCapacity(now time.Time) {
	if limiter.tracked.Load() < limiter.capacity {
		return
	}
	nowUnixNano := now.UnixNano()
	for {
		next := limiter.cleanupNext.Load()
		if nowUnixNano < next {
			return
		}
		if limiter.cleanupNext.CompareAndSwap(next, now.Add(100*time.Millisecond).UnixNano()) {
			break
		}
	}
	shardIndex := limiter.cleanupCursor.Add(1) - 1
	shard := &limiter.shards[shardIndex%invalidAuthAbuseShardCount]
	shard.mu.Lock()
	for key, entry := range shard.entries {
		if limiter.entryExpired(entry, now) {
			delete(shard.entries, key)
			limiter.tracked.Add(-1)
			limiter.expired.Add(1)
		}
	}
	shard.mu.Unlock()
}

func (limiter *invalidAuthAbuseLimiter) entryExpired(entry *invalidAuthAbuseEntry, now time.Time) bool {
	return entry != nil && !entry.blockedUntil.After(now) && !entry.windowStart.After(now) && !now.Before(entry.windowStart.Add(limiter.window))
}

func (limiter *invalidAuthAbuseLimiter) shard(clientKey string) *invalidAuthAbuseShard {
	return &limiter.shards[limiter.hash(clientKey)%invalidAuthAbuseShardCount]
}

func (limiter *invalidAuthAbuseLimiter) hash(clientKey string) uint32 {
	const fnvOffset32 = uint32(2166136261)
	const fnvPrime32 = uint32(16777619)
	hash := fnvOffset32
	for index := 0; index < len(clientKey); index++ {
		hash ^= uint32(clientKey[index])
		hash *= fnvPrime32
	}
	return hash
}

func (limiter *invalidAuthAbuseLimiter) evictOldestUnblockedEntry(shard *invalidAuthAbuseShard, now time.Time) bool {
	var candidateKey string
	var candidateStart time.Time
	for key, entry := range shard.entries {
		if limiter.entryExpired(entry, now) {
			delete(shard.entries, key)
			limiter.tracked.Add(-1)
			limiter.expired.Add(1)
			return limiter.reserveEntry()
		}
		if entry.blockedUntil.After(now) {
			continue
		}
		if candidateKey == "" || entry.windowStart.Before(candidateStart) {
			candidateKey = key
			candidateStart = entry.windowStart
		}
	}
	if candidateKey == "" {
		return false
	}
	delete(shard.entries, candidateKey)
	limiter.evicted.Add(1)
	return true
}

func (limiter *invalidAuthAbuseLimiter) health() InvalidAuthAbuseHealth {
	return InvalidAuthAbuseHealth{
		Enabled:  true,
		Tracked:  limiter.tracked.Load(),
		Capacity: limiter.capacity,
		Recorded: limiter.recorded.Load(),
		Blocks:   limiter.blocked.Load(),
		Rejected: limiter.rejected.Load(),
		Expired:  limiter.expired.Load(),
		Evicted:  limiter.evicted.Load(),
		Dropped:  limiter.dropped.Load(),
	}
}
