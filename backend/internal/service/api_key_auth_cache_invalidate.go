package service

import (
	"context"
	"time"
)

func (s *APIKeyService) authCacheRevocationTTL() time.Duration {
	ttl := s.authCfg.l2TTL
	if s.authCfg.l1TTL > ttl {
		ttl = s.authCfg.l1TTL
	}
	if s.authCfg.negativeTTL > ttl {
		ttl = s.authCfg.negativeTTL
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if s.authCfg.jitterPercent > 0 {
		percent := s.authCfg.jitterPercent
		if percent > 100 {
			percent = 100
		}
		ttl += time.Duration(float64(ttl) * float64(percent) / 100)
	}
	return ttl + time.Minute
}

func (s *APIKeyService) markAuthCacheRevoked(cacheKey string) {
	if cacheKey == "" {
		return
	}
	s.revokedAuthKeys.Store(cacheKey, time.Now().Add(s.authCacheRevocationTTL()))
}

func (s *APIKeyService) isAuthCacheRevoked(cacheKey string) bool {
	value, ok := s.revokedAuthKeys.Load(cacheKey)
	if !ok {
		return false
	}
	expiresAt, ok := value.(time.Time)
	if !ok || time.Now().After(expiresAt) {
		s.revokedAuthKeys.Delete(cacheKey)
		return false
	}
	return true
}

func (s *APIKeyService) revokeAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	cacheKey := s.authCacheKey(key)
	s.markAuthCacheRevoked(cacheKey)
	s.deleteAuthCache(ctx, cacheKey)
	if s.cache != nil {
		_ = s.cache.PublishAuthCacheInvalidation(ctx, authCacheRevokeMessagePrefix+cacheKey)
	}
}

func (s *APIKeyService) restoreAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	cacheKey := s.authCacheKey(key)
	s.revokedAuthKeys.Delete(cacheKey)
	s.deleteAuthCache(ctx, cacheKey)
	if s.cache != nil {
		_ = s.cache.PublishAuthCacheInvalidation(ctx, authCacheRestoreMessagePrefix+cacheKey)
	}
}

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	cacheKey := s.authCacheKey(key)
	s.deleteAuthCache(ctx, cacheKey)
}

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

func (s *APIKeyService) deleteAuthCacheByKeys(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		s.deleteAuthCache(ctx, s.authCacheKey(key))
	}
}
