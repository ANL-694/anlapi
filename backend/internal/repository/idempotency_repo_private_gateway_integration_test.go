//go:build integration

package repository

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPrivateGatewayNonceClaim_ConcurrentAcrossDatabaseConnections(t *testing.T) {
	repo := NewPrivateGatewayNonceRepository(integrationEntClient, integrationDB)
	scope := "private_gateway_nonce:" + hashedTestValue(t, "nonce-scope")
	nonceHash := hashedTestValue(t, "nonce-hash")
	firstExpiresAt := time.Now().UTC().Add(10 * time.Minute)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(),
			"DELETE FROM idempotency_records WHERE scope = $1 AND idempotency_key_hash = $2",
			scope,
			nonceHash,
		)
	})

	const concurrency = 24
	start := make(chan struct{})
	type setupResult struct {
		conn *sql.Conn
		pid  int
		err  error
	}
	type claimResult struct {
		claimed bool
		err     error
	}
	setup := make(chan setupResult, concurrency)
	claims := make(chan claimResult, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := integrationDB.Conn(context.Background())
			if err != nil {
				setup <- setupResult{err: err}
				return
			}
			var pid int
			err = conn.QueryRowContext(context.Background(), "SELECT pg_backend_pid()").Scan(&pid)
			if err != nil {
				_ = conn.Close()
				setup <- setupResult{err: err}
				return
			}
			setup <- setupResult{conn: conn, pid: pid}
			<-start
			workerRepo := &idempotencyRepository{sql: conn}
			claimed, err := workerRepo.ClaimPrivateGatewayNonce(
				context.Background(),
				scope,
				nonceHash,
				firstExpiresAt,
			)
			claims <- claimResult{claimed: claimed, err: err}
			_ = conn.Close()
		}()
	}
	startClosed := false
	defer func() {
		if !startClosed {
			close(start)
		}
	}()
	pids := make(map[int]struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		result := <-setup
		require.NoError(t, result.err)
		if result.err != nil {
			return
		}
		pids[result.pid] = struct{}{}
	}
	require.GreaterOrEqual(t, len(pids), 2, "workers must hold at least two physical PostgreSQL sessions")
	close(start)
	startClosed = true
	wg.Wait()
	close(claims)

	claimedCount := 0
	for result := range claims {
		require.NoError(t, result.err)
		if result.claimed {
			claimedCount++
		}
	}
	require.Equal(t, 1, claimedCount, "one concurrent database claim must win")

	_, err := integrationDB.ExecContext(
		context.Background(),
		"UPDATE idempotency_records SET expires_at = NOW() - INTERVAL '1 second' WHERE scope = $1 AND idempotency_key_hash = $2",
		scope,
		nonceHash,
	)
	require.NoError(t, err)

	reclaimed, err := repo.ClaimPrivateGatewayNonce(
		context.Background(),
		scope,
		nonceHash,
		time.Now().UTC().Add(10*time.Minute),
	)
	require.NoError(t, err)
	require.True(t, reclaimed, "an expired nonce claim may be atomically reclaimed")
}

func TestPrivateGatewayRetryableReclaim_ConcurrentAcrossDatabaseConnections(t *testing.T) {
	ctx := context.Background()
	scope := "private_gateway_state:" + hashedTestValue(t, "retryable-scope")
	keyHash := hashedTestValue(t, "retryable-key")
	fingerprint := hashedTestValue(t, "retryable-fingerprint")
	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	lockedUntil := now.Add(-time.Second)

	var recordID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, error_reason, locked_until, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, scope, keyHash, fingerprint, service.PrivateGatewayIdempotencyStatusFailedRetryable, "UPSTREAM_TEMPORARY", lockedUntil, expiresAt).Scan(&recordID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx,
			"DELETE FROM idempotency_records WHERE id = $1",
			recordID,
		)
	})

	const concurrency = 24
	start := make(chan struct{})
	type setupResult struct {
		conn *sql.Conn
		pid  int
		err  error
	}
	type reclaimResult struct {
		taken bool
		err   error
	}
	setup := make(chan setupResult, concurrency)
	results := make(chan reclaimResult, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, connErr := integrationDB.Conn(ctx)
			if connErr != nil {
				setup <- setupResult{err: connErr}
				return
			}
			var pid int
			if connErr = conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); connErr != nil {
				_ = conn.Close()
				setup <- setupResult{err: connErr}
				return
			}
			setup <- setupResult{conn: conn, pid: pid}
			<-start
			workerRepo := &idempotencyRepository{sql: conn}
			taken, reclaimErr := workerRepo.ReclaimPrivateGatewayRetryable(
				ctx,
				recordID,
				now,
				now.Add(30*time.Second),
				now.Add(24*time.Hour),
			)
			results <- reclaimResult{taken: taken, err: reclaimErr}
			_ = conn.Close()
		}()
	}

	startClosed := false
	defer func() {
		if !startClosed {
			close(start)
		}
	}()
	pids := make(map[int]struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		result := <-setup
		require.NoError(t, result.err)
		if result.err != nil {
			return
		}
		pids[result.pid] = struct{}{}
	}
	require.GreaterOrEqual(t, len(pids), 2, "workers must hold at least two physical PostgreSQL sessions")
	close(start)
	startClosed = true
	wg.Wait()
	close(results)

	takenCount := 0
	for result := range results {
		require.NoError(t, result.err)
		if result.taken {
			takenCount++
		}
	}
	require.Equal(t, 1, takenCount, "one concurrent retryable reclaim must win")

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status FROM idempotency_records WHERE id = $1",
		recordID,
	).Scan(&status))
	require.Equal(t, service.IdempotencyStatusProcessing, status)
}

func TestPrivateGatewayTerminalAndUnknownUpdates_CompeteAcrossDatabaseConnections(t *testing.T) {
	ctx := context.Background()
	scope := "private_gateway_state:" + hashedTestValue(t, "terminal-unknown-scope")
	keyHash := hashedTestValue(t, "terminal-unknown-key")
	fingerprint := hashedTestValue(t, "terminal-unknown-fingerprint")
	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	lockedUntil := now.Add(30 * time.Second)

	var recordID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_fingerprint, status, locked_until, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, scope, keyHash, fingerprint, service.IdempotencyStatusProcessing, lockedUntil, expiresAt).Scan(&recordID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx,
			"DELETE FROM idempotency_records WHERE id = $1",
			recordID,
		)
	})

	start := make(chan struct{})
	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"succeeded", "unknown"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, connErr := integrationDB.Conn(ctx)
			if connErr != nil {
				results <- result{name: name, err: connErr}
				return
			}
			defer conn.Close()
			<-start
			workerRepo := &idempotencyRepository{sql: conn}
			if name == "succeeded" {
				connErr = workerRepo.MarkPrivateGatewaySucceeded(ctx, recordID, 200, `{"ok":true}`, expiresAt)
			} else {
				connErr = workerRepo.MarkPrivateGatewayOutcomeUnknown(ctx, recordID, "IDEMPOTENCY_OUTCOME_UNKNOWN", expiresAt)
			}
			results <- result{name: name, err: connErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for item := range results {
		require.NoError(t, item.err, item.name)
	}

	var status string
	var responseStatus sql.NullInt64
	var responseBody sql.NullString
	var errorReason sql.NullString
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, response_status, response_body, error_reason
		FROM idempotency_records WHERE id = $1
	`, recordID).Scan(&status, &responseStatus, &responseBody, &errorReason))
	switch status {
	case service.IdempotencyStatusSucceeded:
		require.True(t, responseStatus.Valid)
		require.Equal(t, int64(200), responseStatus.Int64)
		require.True(t, responseBody.Valid)
		require.Equal(t, `{"ok":true}`, responseBody.String)
		require.False(t, errorReason.Valid)
	case service.PrivateGatewayIdempotencyStatusOutcomeUnknown:
		require.False(t, responseStatus.Valid)
		require.False(t, responseBody.Valid)
		require.True(t, errorReason.Valid)
		require.Equal(t, "IDEMPOTENCY_OUTCOME_UNKNOWN", errorReason.String)
	default:
		t.Fatalf("concurrent terminal/unknown updates left invalid status %q", status)
	}
}
