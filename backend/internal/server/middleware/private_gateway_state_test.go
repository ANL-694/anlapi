package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type privateGatewayStateFake struct {
	mu       sync.Mutex
	nextID   int64
	records  map[string]*service.IdempotencyRecord
	storeErr error
}

func newPrivateGatewayStateFake() *privateGatewayStateFake {
	return &privateGatewayStateFake{records: make(map[string]*service.IdempotencyRecord)}
}

func (s *privateGatewayStateFake) key(scope, hash string) string { return scope + "\n" + hash }

func (s *privateGatewayStateFake) CreateProcessing(_ context.Context, record *service.IdempotencyRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		return false, s.storeErr
	}
	key := s.key(record.Scope, record.IdempotencyKeyHash)
	if _, exists := s.records[key]; exists {
		return false, nil
	}
	s.nextID++
	record.ID = s.nextID
	s.records[key] = clonePrivateGatewayTestRecord(record)
	return true, nil
}

func (s *privateGatewayStateFake) GetByScopeAndKeyHash(_ context.Context, scope, hash string) (*service.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		return nil, s.storeErr
	}
	return clonePrivateGatewayTestRecord(s.records[s.key(scope, hash)]), nil
}

func (s *privateGatewayStateFake) TryReclaim(_ context.Context, id int64, fromStatus string, now, lockedUntil, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status == fromStatus && (record.LockedUntil == nil || !record.LockedUntil.After(now)) {
			record.Status = service.IdempotencyStatusProcessing
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			return true, nil
		}
	}
	return false, nil
}

func (s *privateGatewayStateFake) ExtendProcessingLock(_ context.Context, id int64, fingerprint string, lockedUntil, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing && record.RequestFingerprint == fingerprint {
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			return true, nil
		}
	}
	return false, nil
}

func (s *privateGatewayStateFake) MarkSucceeded(_ context.Context, id int64, status int, body string, expiresAt time.Time) error {
	return s.mark(id, service.IdempotencyStatusSucceeded, status, body, "", nil, expiresAt)
}

func (s *privateGatewayStateFake) MarkPrivateGatewaySucceeded(_ context.Context, id int64, status int, body string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing {
			responseStatus := status
			responseBody := body
			record.Status = service.IdempotencyStatusSucceeded
			record.ResponseStatus = &responseStatus
			record.ResponseBody = &responseBody
			record.ErrorReason = nil
			record.LockedUntil = nil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func (s *privateGatewayStateFake) MarkFailedRetryable(_ context.Context, id int64, reason string, lockedUntil, expiresAt time.Time) error {
	return s.mark(id, service.IdempotencyStatusFailedRetryable, 0, "", reason, &lockedUntil, expiresAt)
}

func (s *privateGatewayStateFake) DeleteExpired(_ context.Context, now time.Time, _ int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for key, record := range s.records {
		if !record.ExpiresAt.After(now) {
			delete(s.records, key)
			deleted++
		}
	}
	return deleted, nil
}

func (s *privateGatewayStateFake) ReclaimPrivateGatewayExpired(_ context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && !record.ExpiresAt.After(now) && record.Status != service.IdempotencyStatusProcessing {
			record.Status = service.IdempotencyStatusProcessing
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = nil
			return true, nil
		}
	}
	return false, nil
}

func (s *privateGatewayStateFake) ReclaimPrivateGatewayRetryable(_ context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status == service.PrivateGatewayIdempotencyStatusFailedRetryable && record.LockedUntil != nil && !record.LockedUntil.After(now) && record.ExpiresAt.After(now) {
			record.Status = service.IdempotencyStatusProcessing
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = nil
			return true, nil
		}
	}
	return false, nil
}

func (s *privateGatewayStateFake) MarkPrivateGatewayFailed(_ context.Context, id int64, status int, body, reason string, expiresAt time.Time) error {
	return s.mark(id, service.PrivateGatewayIdempotencyStatusFailed, status, body, reason, nil, expiresAt)
}

func (s *privateGatewayStateFake) MarkPrivateGatewayFailedRetryable(_ context.Context, id int64, status int, body, reason string, lockedUntil, expiresAt time.Time) error {
	return s.mark(id, service.PrivateGatewayIdempotencyStatusFailedRetryable, status, body, reason, &lockedUntil, expiresAt)
}

func (s *privateGatewayStateFake) MarkPrivateGatewayOutcomeUnknown(_ context.Context, id int64, reason string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing {
			record.Status = service.PrivateGatewayIdempotencyStatusOutcomeUnknown
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = &reason
			record.LockedUntil = nil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func (s *privateGatewayStateFake) MarkPrivateGatewayExpired(_ context.Context, id int64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status != service.IdempotencyStatusProcessing {
			record.Status = service.PrivateGatewayIdempotencyStatusExpired
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = &reason
			record.LockedUntil = nil
			return nil
		}
	}
	return nil
}

func (s *privateGatewayStateFake) mark(id int64, status string, responseStatus int, body, reason string, lockedUntil *time.Time, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing {
			record.Status = status
			if responseStatus != 0 {
				record.ResponseStatus = &responseStatus
			}
			if body != "" {
				record.ResponseBody = &body
			}
			if reason != "" {
				record.ErrorReason = &reason
			}
			record.LockedUntil = lockedUntil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func clonePrivateGatewayTestRecord(record *service.IdempotencyRecord) *service.IdempotencyRecord {
	if record == nil {
		return nil
	}
	clone := *record
	if record.ResponseStatus != nil {
		value := *record.ResponseStatus
		clone.ResponseStatus = &value
	}
	if record.ResponseBody != nil {
		value := *record.ResponseBody
		clone.ResponseBody = &value
	}
	if record.ErrorReason != nil {
		value := *record.ErrorReason
		clone.ErrorReason = &value
	}
	if record.LockedUntil != nil {
		value := *record.LockedUntil
		clone.LockedUntil = &value
	}
	return &clone
}

func newPrivateGatewayStateTestRuntime(state service.PrivateGatewayStateRepository, now *time.Time) *ANLPrivateGateway {
	clock := func() time.Time { return privateGatewayTestNow }
	if now != nil {
		clock = func() time.Time { return *now }
	}
	return newANLPrivateGatewayWithRuntimeAndState(
		privateGatewayTestConfig(),
		func(_ context.Context, id int64) (*service.APIKey, error) {
			if id != 42 {
				return nil, service.ErrAPIKeyNotFound
			}
			return &service.APIKey{ID: id}, nil
		},
		newPrivateGatewayTestNonceRepo(),
		state,
		clock,
	)
}

func TestANLPrivateGatewayB04ReplayConflictAndRequiredKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayStateFake()
	gateway := newPrivateGatewayStateTestRuntime(state, nil)
	var calls atomic.Int32
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		calls.Add(1)
		c.Header("X-Test-Result", "preserved")
		c.JSON(http.StatusCreated, gin.H{"id": "chatcmpl-b04", "usage": gin.H{"total_tokens": 3}})
	})
	router.POST("/v1/images/generations", func(c *gin.Context) {
		t.Fatal("a reused key on another output path must conflict before execution")
	})

	body := `{"model":"gpt-5.6-terra"}`
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "b04-replay-key-0001")
		signPrivateGatewayTestRequest(req, []byte(body), strings.Repeat("0", 31)+string(rune('1'+attempt)), privateGatewayTestNow)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusCreated, response.Code)
		require.Equal(t, "preserved", response.Header().Get("X-Test-Result"))
		if attempt == 1 {
			require.Equal(t, "true", response.Header().Get(privateGatewayIdempotencyReplayHeader))
		}
	}
	require.Equal(t, int32(1), calls.Load())

	crossPath := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
	crossPath.Header.Set("Idempotency-Key", "b04-replay-key-0001")
	signPrivateGatewayTestRequest(crossPath, []byte(body), strings.Repeat("0", 31)+"a", privateGatewayTestNow)
	crossPathResponse := httptest.NewRecorder()
	router.ServeHTTP(crossPathResponse, crossPath)
	requirePrivateGatewayErrorCode(t, crossPathResponse, http.StatusConflict, "IDEMPOTENCY_CONFLICT")

	changed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"different"}`))
	changed.Header.Set("Idempotency-Key", "b04-replay-key-0001")
	signPrivateGatewayTestRequest(changed, []byte(`{"model":"different"}`), strings.Repeat("0", 31)+"3", privateGatewayTestNow)
	changedResponse := httptest.NewRecorder()
	router.ServeHTTP(changedResponse, changed)
	requirePrivateGatewayErrorCode(t, changedResponse, http.StatusConflict, "IDEMPOTENCY_CONFLICT")

	missing := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	signPrivateGatewayTestRequest(missing, []byte(body), strings.Repeat("0", 31)+"4", privateGatewayTestNow)
	missingResponse := httptest.NewRecorder()
	router.ServeHTTP(missingResponse, missing)
	requirePrivateGatewayErrorCode(t, missingResponse, http.StatusBadRequest, "INVALID_REQUEST")
}

func TestANLPrivateGatewayB04UnknownAndRetryableLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := privateGatewayTestNow
	state := newPrivateGatewayStateFake()
	gateway := newPrivateGatewayStateTestRuntime(state, &now)
	var calls atomic.Int32
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		if calls.Add(1) == 1 {
			MarkPrivateGatewayRetryableFailure(c)
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "UPSTREAM_TEMPORARY", "temporary")
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": "retried"})
	})

	body := `{"model":"gpt-5.6-terra"}`
	request := func(nonce string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "b04-retry-key-0001")
		signPrivateGatewayTestRequest(req, []byte(body), nonce, now)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	first := request(strings.Repeat("0", 31) + "5")
	requirePrivateGatewayErrorCode(t, first, http.StatusServiceUnavailable, "UPSTREAM_TEMPORARY")
	second := request(strings.Repeat("0", 31) + "6")
	requirePrivateGatewayErrorCode(t, second, http.StatusConflict, "IDEMPOTENCY_RETRY_BACKOFF")
	now = now.Add(privateGatewayProcessingTTL + time.Second)
	third := request(strings.Repeat("0", 31) + "7")
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, int32(2), calls.Load())

	unknownState := newPrivateGatewayStateFake()
	unknownGateway := newPrivateGatewayStateTestRuntime(unknownState, &now)
	unknownRouter := gin.New()
	unknownRouter.Use(unknownGateway.Authentication(), unknownGateway.Idempotency())
	unknownRouter.POST("/v1/chat/completions", func(c *gin.Context) { t.Fatal("unknown outcome must not execute") })
	unknownBody := []byte(`{"model":"unknown"}`)
	unknownReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(unknownBody)))
	unknownKey := "b04-unknown-key-001"
	unknownScope := privateGatewayIdempotencyScope(privateGatewayTestConfig().ServiceID, unknownReq, unknownKey)
	digest := sha256Hex(unknownBody)
	lockedUntil := now.Add(-time.Second)
	record := &service.IdempotencyRecord{Scope: unknownScope, IdempotencyKeyHash: service.HashIdempotencyKey(unknownKey), RequestFingerprint: privateGatewayIdempotencyFingerprint(privateGatewayRequestContext{serviceID: privateGatewayTestConfig().ServiceID, bodySHA256: digest}, unknownReq), Status: service.IdempotencyStatusProcessing, LockedUntil: &lockedUntil, ExpiresAt: now.Add(time.Hour)}
	owner, err := unknownState.CreateProcessing(context.Background(), record)
	require.NoError(t, err)
	require.True(t, owner)
	unknownReq.Header.Set("Idempotency-Key", unknownKey)
	signPrivateGatewayTestRequest(unknownReq, unknownBody, strings.Repeat("0", 31)+"8", now)
	unknownResponse := httptest.NewRecorder()
	unknownRouter.ServeHTTP(unknownResponse, unknownReq)
	requirePrivateGatewayErrorCode(t, unknownResponse, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
}

func TestANLPrivateGatewayLateSuccessCannotOverwriteOutcomeUnknown(t *testing.T) {
	state := newPrivateGatewayStateFake()
	now := privateGatewayTestNow
	gateway := newPrivateGatewayStateTestRuntime(state, &now)
	router := gin.New()
	router.Use(gateway.Authentication())
	router.Use(gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		state.mu.Lock()
		var recordID int64
		for _, record := range state.records {
			recordID = record.ID
			break
		}
		state.mu.Unlock()
		require.NotZero(t, recordID)
		require.NoError(t, gateway.markOutcomeUnknown(recordID))
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	body := []byte(`{"model":"gpt-test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "late-success-key-0001")
	signPrivateGatewayTestRequest(req, body, strings.Repeat("3", 32), now)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusOK, response.Code)
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, record := range state.records {
		require.Equal(t, service.PrivateGatewayIdempotencyStatusOutcomeUnknown, record.Status)
		require.Nil(t, record.ResponseBody)
	}
}

func TestANLPrivateGatewayB04StatusIsRedactedAndStreamIncompleteIsUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayStateFake()
	gateway := newPrivateGatewayStateTestRuntime(state, nil)
	router := gin.New()
	router.Use(gateway.Authentication())
	router.POST("/v1/chat/completions", gateway.Idempotency(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": "redacted", "usage": gin.H{"total_tokens": 5}, "secret": "must-not-leak-in-status"})
	})
	router.GET("/v1/sub2api/idempotency/:key", gateway.Status())

	body := []byte(`{"model":"gpt-5.6-terra"}`)
	key := "b04-status-key-0001"
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	req.Header.Set("Idempotency-Key", key)
	signPrivateGatewayTestRequest(req, body, strings.Repeat("0", 31)+"9", privateGatewayTestNow)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code)

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/sub2api/idempotency/"+key, nil)
	signPrivateGatewayTestRequest(statusReq, nil, strings.Repeat("0", 30)+"10", privateGatewayTestNow)
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, statusReq)
	require.Equal(t, http.StatusOK, statusResponse.Code)
	require.Contains(t, statusResponse.Body.String(), `"usage":{"total_tokens":5}`)
	require.NotContains(t, statusResponse.Body.String(), "secret")
	require.NotContains(t, statusResponse.Body.String(), "response_body")

	streamState := newPrivateGatewayStateFake()
	streamGateway := newPrivateGatewayStateTestRuntime(streamState, nil)
	var streamCalls atomic.Int32
	streamRouter := gin.New()
	streamRouter.Use(streamGateway.Authentication(), streamGateway.Idempotency())
	streamRouter.POST("/v1/chat/completions", func(c *gin.Context) {
		streamCalls.Add(1)
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.Write([]byte("data: {\"id\":\"first-output\"}\n\n"))
	})
	for i := 0; i < 2; i++ {
		streamReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
		streamReq.Header.Set("Idempotency-Key", "b04-stream-key-0001")
		signPrivateGatewayTestRequest(streamReq, body, strings.Repeat("1", 31)+string(rune('0'+i)), privateGatewayTestNow)
		streamResponse := httptest.NewRecorder()
		streamRouter.ServeHTTP(streamResponse, streamReq)
		if i == 1 {
			requirePrivateGatewayErrorCode(t, streamResponse, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN")
		}
	}
	require.Equal(t, int32(1), streamCalls.Load())
}

func TestANLPrivateGatewayB04StateStoreUnavailableFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := newPrivateGatewayStateFake()
	state.storeErr = errors.New("database unavailable")
	gateway := newPrivateGatewayStateTestRuntime(state, nil)
	router := gin.New()
	router.Use(gateway.Authentication(), gateway.Idempotency())
	router.POST("/v1/chat/completions", func(c *gin.Context) { t.Fatal("store failure must block execution") })
	body := []byte(`{"model":"gpt-5.6-terra"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	req.Header.Set("Idempotency-Key", "b04-store-key-0001")
	signPrivateGatewayTestRequest(req, body, strings.Repeat("2", 32), privateGatewayTestNow)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	requirePrivateGatewayErrorCode(t, response, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE")
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
