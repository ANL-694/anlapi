package repository

import (
	"context"
	"time"

	"ikik-api/internal/service"

	"github.com/redis/go-redis/v9"
)

const leaderLockKeyPrefix = "leader:lock:"

var leaderLockReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type leaderLockCache struct {
	rdb *redis.Client
}

// NewLeaderLockCache provides compare-and-delete Redis locks for singleton
// background jobs across multiple application instances.
func NewLeaderLockCache(rdb *redis.Client) service.LeaderLockCache {
	return &leaderLockCache{rdb: rdb}
}

func (c *leaderLockCache) TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, leaderLockKeyPrefix+key, owner, ttl).Result()
}

func (c *leaderLockCache) ReleaseLeaderLock(ctx context.Context, key, owner string) error {
	return leaderLockReleaseScript.Run(ctx, c.rdb, []string{leaderLockKeyPrefix + key}, owner).Err()
}
