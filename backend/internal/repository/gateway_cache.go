package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"anlapi/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"
const liveCallPrefix = "live:call:"
const liveControllerLeaseTTLSeconds = 60

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

func (c *gatewayCache) GetSessionString(ctx context.Context, groupID int64, sessionHash string) (string, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Result()
}

var _ service.LiveCallStore = (*gatewayCache)(nil)

func (c *gatewayCache) SetSessionString(ctx context.Context, groupID int64, sessionHash string, value string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *gatewayCache) DeleteSessionString(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

var claimLiveControllerScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local target = ARGV[1]
	local owner = ARGV[2]
	local ttl = tonumber(ARGV[3])
	local now = tonumber(redis.call('TIME')[1])
	local current = redis.call('HGET', key, 'controller')
	if current == false or current == 'closed' then
		return 0
	end
	local expiresAt = tonumber(redis.call('HGET', key, 'controller_expires_at') or '0')
	if current ~= 'pending' and expiresAt <= now then
		redis.call('HSET', key, 'controller', 'pending', 'controller_owner', '', 'controller_expires_at', 0)
		current = 'pending'
	end
	if target == 'observer' and current ~= 'pending' then
		return 0
	end
	if target == 'proxy' and current ~= 'pending' and current ~= 'observer' and
		(current ~= 'proxy' or redis.call('HGET', key, 'controller_owner') ~= owner) then
		return 0
	end
	redis.call('HSET', key, 'controller', target, 'controller_owner', owner, 'controller_expires_at', now + ttl)
	return 1
`)

var refreshLiveControllerScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local owner = ARGV[1]
	local ttl = tonumber(ARGV[2])
	local controller = redis.call('HGET', key, 'controller')
	if controller == false or controller == 'closed' or redis.call('HGET', key, 'controller_owner') ~= owner then
		return 0
	end
	local now = tonumber(redis.call('TIME')[1])
	local expiresAt = tonumber(redis.call('HGET', key, 'controller_expires_at') or '0')
	if expiresAt <= now then
		redis.call('HSET', key, 'controller', 'pending', 'controller_owner', '', 'controller_expires_at', 0)
		return 0
	end
	redis.call('HSET', key, 'controller_expires_at', now + ttl)
	return 1
`)

var getLiveControllerScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local current = redis.call('HGET', key, 'controller')
	if current == false then
		return false
	end
	if current ~= 'pending' and current ~= 'closed' then
		local expiresAt = tonumber(redis.call('HGET', key, 'controller_expires_at') or '0')
		local now = tonumber(redis.call('TIME')[1])
		if expiresAt <= now then
			redis.call('HSET', key, 'controller', 'pending', 'controller_owner', '', 'controller_expires_at', 0)
			return 'pending'
		end
	end
	return current
`)

var markLiveCallClosedScript = redis.NewScript(`
	local key = KEYS[1]
	if redis.call('EXISTS', key) == 0 then
		return 0
	end
	if redis.call('HGET', key, 'controller') == 'closed' then
		return 0
	end
	redis.call('HSET', key, 'controller', 'closed', 'controller_owner', '', 'controller_expires_at', 0)
	redis.call('EXPIRE', key, ARGV[1])
	return 1
`)

var releaseLiveControllerScript = redis.NewScript(`
	local key = KEYS[1]
	local controller = redis.call('HGET', key, 'controller')
	if controller == false or controller == 'pending' or controller == 'closed' or
		redis.call('HGET', key, 'controller_owner') ~= ARGV[1] then
		return 0
	end
	redis.call('HSET', key, 'controller', 'pending', 'controller_owner', '', 'controller_expires_at', 0)
	return 1
`)

func liveCallKey(callHash string) string {
	return liveCallPrefix + callHash
}

func HashLiveCallID(callID string) string {
	sum := sha256.Sum256([]byte(callID))
	return hex.EncodeToString(sum[:])
}

func (c *gatewayCache) SaveLiveCall(ctx context.Context, record *service.LiveCallRecord, ttl time.Duration) error {
	if record == nil || record.CallHash == "" || record.CallID == "" {
		return fmt.Errorf("invalid live call record")
	}
	values := map[string]any{
		"call_id":               record.CallID,
		"account_id":            record.AccountID,
		"api_key_id":            record.APIKeyID,
		"user_id":               record.UserID,
		"group_id":              record.GroupID,
		"subscription_id":       record.SubscriptionID,
		"lease_id":              record.LeaseID,
		"model":                 record.Model,
		"created_at":            record.CreatedAt.UnixMilli(),
		"expires_at":            record.ExpiresAt.UnixMilli(),
		"controller":            record.Controller,
		"controller_owner":      record.ControllerOwner,
		"controller_expires_at": 0,
		"user_agent":            record.UserAgent,
		"ip_address":            record.IPAddress,
		"inbound_endpoint":      record.InboundEndpoint,
		"session_id":            record.SessionID,
		"attestation":           record.AttestationCiphertext,
	}
	key := liveCallKey(record.CallHash)
	pipe := c.rdb.TxPipeline()
	pipe.HSet(ctx, key, values)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *gatewayCache) GetLiveCall(ctx context.Context, callHash string) (*service.LiveCallRecord, error) {
	values, err := c.rdb.HGetAll(ctx, liveCallKey(callHash)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, service.ErrLiveCallNotFound
	}
	parseInt := func(field string) int64 {
		value, _ := strconv.ParseInt(values[field], 10, 64)
		return value
	}
	createdAt := time.UnixMilli(parseInt("created_at"))
	expiresAt := time.UnixMilli(parseInt("expires_at"))
	return &service.LiveCallRecord{
		CallID:                values["call_id"],
		CallHash:              callHash,
		AccountID:             parseInt("account_id"),
		APIKeyID:              parseInt("api_key_id"),
		UserID:                parseInt("user_id"),
		GroupID:               parseInt("group_id"),
		SubscriptionID:        parseInt("subscription_id"),
		LeaseID:               values["lease_id"],
		Model:                 values["model"],
		CreatedAt:             createdAt,
		ExpiresAt:             expiresAt,
		Controller:            values["controller"],
		ControllerOwner:       values["controller_owner"],
		UserAgent:             values["user_agent"],
		IPAddress:             values["ip_address"],
		InboundEndpoint:       values["inbound_endpoint"],
		SessionID:             values["session_id"],
		AttestationCiphertext: values["attestation"],
	}, nil
}

func (c *gatewayCache) ClaimLiveController(ctx context.Context, callHash, controller, owner string) (bool, error) {
	result, err := claimLiveControllerScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}, controller, owner, liveControllerLeaseTTLSeconds).Int()
	return result == 1, err
}

func (c *gatewayCache) RefreshLiveController(ctx context.Context, callHash, owner string, ttl time.Duration) (bool, error) {
	result, err := refreshLiveControllerScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}, owner, int64(ttl.Seconds())).Int()
	return result == 1, err
}

func (c *gatewayCache) GetLiveController(ctx context.Context, callHash string) (string, error) {
	value, err := getLiveControllerScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}).Text()
	if err == redis.Nil {
		return "", service.ErrLiveCallNotFound
	}
	return value, err
}

func (c *gatewayCache) ReleaseLiveController(ctx context.Context, callHash, owner string) (bool, error) {
	result, err := releaseLiveControllerScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}, owner).Int()
	return result == 1, err
}

func (c *gatewayCache) MarkLiveCallClosed(ctx context.Context, callHash string, ttl time.Duration) (bool, error) {
	result, err := markLiveCallClosedScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}, int64(ttl.Seconds())).Int()
	return result == 1, err
}
