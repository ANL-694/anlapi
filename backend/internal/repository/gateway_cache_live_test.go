package repository

import (
	"context"
	"testing"
	"time"

	"anlapi/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheLiveCallIdentityAndController(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	otherInstance, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	record := &service.LiveCallRecord{
		CallID:                "call_secret",
		CallHash:              HashLiveCallID("call_secret"),
		AccountID:             11,
		APIKeyID:              22,
		UserID:                33,
		GroupID:               44,
		LeaseID:               "lease",
		Model:                 "gpt-live-test",
		SessionID:             "session-live-123",
		AttestationCiphertext: "encrypted-attestation",
		CreatedAt:             time.Now(),
		ExpiresAt:             time.Now().Add(time.Hour),
		Controller:            service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))

	loaded, err := otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.CallID, loaded.CallID)
	require.Equal(t, record.AccountID, loaded.AccountID)
	require.Equal(t, record.SessionID, loaded.SessionID)
	require.Equal(t, record.AttestationCiphertext, loaded.AttestationCiphertext)

	claimed, err := cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerObserver, "observer-1")
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerProxy, "proxy-1")
	require.NoError(t, err)
	require.True(t, claimed)
	controller, err := cache.GetLiveController(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, service.LiveControllerProxy, controller)

	released, err := cache.ReleaseLiveController(context.Background(), record.CallHash, "proxy-1")
	require.NoError(t, err)
	require.True(t, released)
	closed, err := cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.True(t, closed)
	closed, err = cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.False(t, closed)
}

func TestGatewayCacheRefreshLiveControllerOwnerOnly(t *testing.T) {
	newStore := func(t *testing.T) (*miniredis.Miniredis, service.LiveCallStore, *service.LiveCallRecord, time.Time) {
		t.Helper()
		redisServer := miniredis.RunT(t)
		baseTime := time.Unix(1_800_000_000, 0)
		redisServer.SetTime(baseTime)
		client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
		store, ok := NewGatewayCache(client).(service.LiveCallStore)
		require.True(t, ok)
		record := &service.LiveCallRecord{
			CallID:     "call_refresh",
			CallHash:   HashLiveCallID("call_refresh"),
			CreatedAt:  baseTime,
			ExpiresAt:  baseTime.Add(time.Hour),
			Controller: service.LiveControllerPending,
		}
		require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
		claimed, err := store.ClaimLiveController(
			context.Background(),
			record.CallHash,
			service.LiveControllerObserver,
			"owner-1",
		)
		require.NoError(t, err)
		require.True(t, claimed)
		return redisServer, store, record, baseTime
	}

	t.Run("different owner cannot refresh", func(t *testing.T) {
		redisServer, store, record, baseTime := newStore(t)
		refreshed, err := store.RefreshLiveController(
			context.Background(),
			record.CallHash,
			"owner-2",
			2*time.Minute,
		)
		require.NoError(t, err)
		require.False(t, refreshed)

		redisServer.SetTime(baseTime.Add(61 * time.Second))
		controller, err := store.GetLiveController(context.Background(), record.CallHash)
		require.NoError(t, err)
		require.Equal(t, service.LiveControllerPending, controller)
	})

	t.Run("owner refreshes for requested ttl", func(t *testing.T) {
		redisServer, store, record, baseTime := newStore(t)
		refreshed, err := store.RefreshLiveController(
			context.Background(),
			record.CallHash,
			"owner-1",
			2*time.Minute,
		)
		require.NoError(t, err)
		require.True(t, refreshed)

		redisServer.SetTime(baseTime.Add(61 * time.Second))
		controller, err := store.GetLiveController(context.Background(), record.CallHash)
		require.NoError(t, err)
		require.Equal(t, service.LiveControllerObserver, controller)

		redisServer.SetTime(baseTime.Add(121 * time.Second))
		controller, err = store.GetLiveController(context.Background(), record.CallHash)
		require.NoError(t, err)
		require.Equal(t, service.LiveControllerPending, controller)
	})
}
