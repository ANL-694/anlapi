//go:build unit

package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"anlapi/internal/config"
	"github.com/stretchr/testify/require"
)

func newInvalidAuthLimiterForTest(threshold, capacity int) *invalidAuthAbuseLimiter {
	cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{
		InvalidAbuse: config.InvalidAuthAbuseConfig{
			Enabled: true, Threshold: threshold, WindowSeconds: 60, BlockSeconds: 10, Capacity: capacity,
		},
	}}
	return newInvalidAuthAbuseLimiter(cfg)
}

func TestInvalidAuthAbuseLimiterBlocksAndExpires(t *testing.T) {
	limiter := newInvalidAuthLimiterForTest(3, 16)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for range 3 {
		limiter.record("203.0.113.1")
	}
	retry, blocked := limiter.check("203.0.113.1")
	require.True(t, blocked)
	require.Equal(t, 10*time.Second, retry)

	now = now.Add(11 * time.Second)
	_, blocked = limiter.check("203.0.113.1")
	require.False(t, blocked)
	now = now.Add(61 * time.Second)
	_, blocked = limiter.check("203.0.113.1")
	require.False(t, blocked)
	require.Zero(t, limiter.health().Tracked)
}

func TestInvalidAuthAbuseLimiterCapacityPreservesSourceSpecificBlocks(t *testing.T) {
	const capacity = 16
	limiter := newInvalidAuthLimiterForTest(2, capacity)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for index := range capacity {
		limiter.record(fmt.Sprintf("tracked-source-%d", index))
	}
	attackerKey := "attacker-source"
	limiter.record(attackerKey)
	limiter.record(attackerKey)

	_, blocked := limiter.check(attackerKey)
	require.True(t, blocked)
	_, unrelatedBlocked := limiter.check("unrelated-source")
	require.False(t, unrelatedBlocked, "a blocked source must not affect another source")
	_, trackedBlocked := limiter.check("tracked-source-1")
	require.False(t, trackedBlocked, "source-specific blocks should spare other tracked sources")
	health := limiter.health()
	require.Equal(t, int64(capacity), health.Tracked)
	require.Equal(t, uint64(1), health.Evicted)
}

func TestInvalidAuthAbuseLimiterKeepsBlockAfterCapacityReclaim(t *testing.T) {
	const capacity = 16
	limiter := newInvalidAuthLimiterForTest(2, capacity)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	limiter.window = time.Second
	primaryKeys := make([]string, 0, capacity)
	for index := range capacity {
		primaryKey := fmt.Sprintf("primary-source-%d", index)
		primaryKeys = append(primaryKeys, primaryKey)
		limiter.record(primaryKey)
	}
	blockedKey := "blocked-source"
	limiter.record(blockedKey)
	limiter.record(blockedKey)
	_, blocked := limiter.check(blockedKey)
	require.True(t, blocked)

	now = now.Add(2 * time.Second)
	for _, primaryKey := range primaryKeys {
		limiter.check(primaryKey)
		if limiter.health().Tracked < capacity {
			break
		}
	}
	require.Less(t, limiter.health().Tracked, int64(capacity))
	_, blocked = limiter.check(blockedKey)
	require.True(t, blocked)
}

func TestInvalidAuthAbuseLimiterConcurrentCapacityIsBounded(t *testing.T) {
	const capacity = 64
	limiter := newInvalidAuthLimiterForTest(1000, capacity)
	var waitGroup sync.WaitGroup
	for index := range 1000 {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			limiter.record(fmt.Sprintf("198.51.100.%d", index))
		}(index)
	}
	waitGroup.Wait()
	health := limiter.health()
	require.LessOrEqual(t, health.Tracked, int64(capacity))
	require.Equal(t, uint64(1000), health.Recorded)
	require.Equal(t, uint64(1000-capacity), health.Evicted+health.Dropped)
}

func TestInvalidAuthAbuseLimiterReclaimsExpiredCapacity(t *testing.T) {
	const capacity = 16
	limiter := newInvalidAuthLimiterForTest(100, capacity)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for index := range capacity {
		limiter.record(fmt.Sprintf("source-%d", index))
	}
	require.Equal(t, int64(capacity), limiter.health().Tracked)

	now = now.Add(61 * time.Second)
	for index := range invalidAuthAbuseShardCount {
		limiter.check(fmt.Sprintf("new-source-%d", index))
		now = now.Add(101 * time.Millisecond)
	}
	require.Less(t, limiter.health().Tracked, int64(capacity))
	limiter.record("fresh-source")
	require.LessOrEqual(t, limiter.health().Tracked, int64(capacity))
}
