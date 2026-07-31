package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"anlapi/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type liveTestFrame struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

type liveTestFrameConn struct {
	reads     chan liveTestFrame
	writes    chan liveTestFrame
	closed    chan struct{}
	closeOnce sync.Once
}

func newLiveTestFrameConn() *liveTestFrameConn {
	return &liveTestFrameConn{
		reads:  make(chan liveTestFrame, 8),
		writes: make(chan liveTestFrame, 8),
		closed: make(chan struct{}),
	}
}

func (c *liveTestFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	select {
	case frame := <-c.reads:
		return frame.messageType, frame.payload, frame.err
	case <-c.closed:
		return coderws.MessageText, nil, coderws.CloseError{Code: coderws.StatusNormalClosure}
	case <-ctx.Done():
		return coderws.MessageText, nil, context.Cause(ctx)
	}
}

func (c *liveTestFrameConn) WriteFrame(ctx context.Context, messageType coderws.MessageType, payload []byte) error {
	frame := liveTestFrame{messageType: messageType, payload: append([]byte(nil), payload...)}
	select {
	case c.writes <- frame:
		return nil
	case <-c.closed:
		return errors.New("connection closed")
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (c *liveTestFrameConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func (c *liveTestFrameConn) ReadMessage(ctx context.Context) ([]byte, error) {
	_, payload, err := c.ReadFrame(ctx)
	return payload, err
}

func (c *liveTestFrameConn) Ping(context.Context) error { return nil }

func (c *liveTestFrameConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

type liveTestDialer struct {
	conn    *liveTestFrameConn
	url     string
	headers http.Header
}

type liveRetryDialer struct {
	mu       sync.Mutex
	attempts int
	failures int
	advance  func()
	onDial   func(int)
	conn     *liveTestFrameConn
	block    bool
	canceled chan struct{}
}

func (d *liveRetryDialer) Dial(
	ctx context.Context,
	_ string,
	_ http.Header,
	_ string,
) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	d.attempts++
	attempt := d.attempts
	d.mu.Unlock()
	if d.advance != nil {
		d.advance()
	}
	if d.onDial != nil {
		d.onDial(attempt)
	}
	if d.block {
		<-ctx.Done()
		if d.canceled != nil {
			close(d.canceled)
		}
		return nil, 0, nil, context.Cause(ctx)
	}
	if attempt <= d.failures {
		return nil, http.StatusBadGateway, nil, errors.New("temporary sideband dial failure")
	}
	return d.conn, http.StatusSwitchingProtocols, nil, nil
}

func (d *liveTestDialer) Dial(
	_ context.Context,
	wsURL string,
	headers http.Header,
	_ string,
) (openAIWSClientConn, int, http.Header, error) {
	d.url = wsURL
	d.headers = headers.Clone()
	return d.conn, http.StatusSwitchingProtocols, nil, nil
}

type liveTestAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *liveTestAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

type liveTestStore struct {
	GatewayCache
	mu     sync.Mutex
	record *LiveCallRecord
}

func (s *liveTestStore) SaveLiveCall(_ context.Context, record *LiveCallRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *record
	s.record = &copy
	return nil
}

func (s *liveTestStore) GetLiveCall(_ context.Context, callHash string) (*LiveCallRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash {
		return nil, ErrLiveCallNotFound
	}
	copy := *s.record
	return &copy, nil
}

func (s *liveTestStore) ClaimLiveController(_ context.Context, callHash, controller, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash || s.record.Controller == LiveControllerClosed {
		return false, nil
	}
	if controller == LiveControllerObserver && s.record.Controller != LiveControllerPending {
		return false, nil
	}
	if controller == LiveControllerProxy && s.record.Controller != LiveControllerPending && s.record.Controller != LiveControllerObserver {
		return false, nil
	}
	s.record.Controller = controller
	s.record.ControllerOwner = owner
	return true, nil
}

func (s *liveTestStore) ReleaseLiveController(_ context.Context, callHash, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash || s.record.ControllerOwner != owner {
		return false, nil
	}
	s.record.Controller = LiveControllerPending
	s.record.ControllerOwner = ""
	return true, nil
}

func (s *liveTestStore) RefreshLiveController(_ context.Context, callHash, owner string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash || s.record.ControllerOwner != owner {
		return false, nil
	}
	return true, nil
}

type liveControllerRefreshTestStore struct {
	*liveTestStore
	refresh func(context.Context, string, string, time.Duration) (bool, error)
}

func (s *liveControllerRefreshTestStore) RefreshLiveController(
	ctx context.Context,
	callHash string,
	owner string,
	ttl time.Duration,
) (bool, error) {
	return s.refresh(ctx, callHash, owner, ttl)
}

func (s *liveTestStore) GetLiveController(_ context.Context, callHash string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash {
		return "", ErrLiveCallNotFound
	}
	return s.record.Controller, nil
}

func (s *liveTestStore) MarkLiveCallClosed(_ context.Context, callHash string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash || s.record.Controller == LiveControllerClosed {
		return false, nil
	}
	s.record.Controller = LiveControllerClosed
	s.record.ControllerOwner = ""
	return true, nil
}

type liveTestConcurrencyCache struct {
	ConcurrencyCache
	mu       sync.Mutex
	releases int
}

type liveRetryConcurrencyCache struct {
	ConcurrencyCache
	mu        sync.Mutex
	now       time.Duration
	expiresAt time.Duration
	ttl       time.Duration
	refreshes int
	releases  int
	leaseLost bool
}

func (c *liveRetryConcurrencyCache) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now += duration
	c.mu.Unlock()
}

func (c *liveRetryConcurrencyCache) AcquireLiveLease(
	context.Context,
	int64,
	int,
	int64,
	int,
	int64,
	string,
	bool,
) (bool, error) {
	return true, nil
}

func (c *liveRetryConcurrencyCache) RefreshLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.leaseLost || c.now > c.expiresAt {
		return false, nil
	}
	c.refreshes++
	c.expiresAt = c.now + c.ttl
	return true, nil
}

func (c *liveRetryConcurrencyCache) ReleaseLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) error {
	c.mu.Lock()
	c.releases++
	c.mu.Unlock()
	return nil
}

func (c *liveTestConcurrencyCache) AcquireLiveLease(
	context.Context,
	int64,
	int,
	int64,
	int,
	int64,
	string,
	bool,
) (bool, error) {
	return true, nil
}

func (c *liveTestConcurrencyCache) RefreshLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) (bool, error) {
	return true, nil
}

func (c *liveTestConcurrencyCache) ReleaseLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) error {
	c.mu.Lock()
	c.releases++
	c.mu.Unlock()
	return nil
}

type liveTestUsageRepo struct {
	UsageLogRepository
	mu   sync.Mutex
	logs []*UsageLog
}

func (r *liveTestUsageRepo) Create(_ context.Context, log *UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *log
	r.logs = append(r.logs, &copy)
	return true, nil
}

func TestRunLiveControllerClosesExpiredSession(t *testing.T) {
	upstream := newLiveTestFrameConn()
	record := &LiveCallRecord{ExpiresAt: time.Now().Add(20 * time.Millisecond)}
	service := &OpenAIGatewayService{}

	err := service.runLiveController(context.Background(), record, upstream, "", make(chan error))
	require.ErrorIs(t, err, context.DeadlineExceeded)

	select {
	case frame := <-upstream.writes:
		require.Equal(t, coderws.MessageText, frame.messageType)
		require.JSONEq(t, `{"type":"session.close"}`, string(frame.payload))
	case <-time.After(time.Second):
		t.Fatal("没有向上游发送 session.close")
	}
}

func TestRefreshLiveController(t *testing.T) {
	t.Run("store unavailable", func(t *testing.T) {
		service := &OpenAIGatewayService{}
		require.False(t, service.refreshLiveController("call-hash", "owner"))
	})

	tests := []struct {
		name      string
		refreshed bool
		err       error
		want      bool
	}{
		{name: "redis error", err: errors.New("redis unavailable")},
		{name: "ownership lost", refreshed: false},
		{name: "owner refreshed", refreshed: true, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &liveControllerRefreshTestStore{
				liveTestStore: &liveTestStore{},
				refresh: func(ctx context.Context, callHash, owner string, ttl time.Duration) (bool, error) {
					deadline, ok := ctx.Deadline()
					require.True(t, ok)
					require.WithinDuration(t, time.Now().Add(liveRedisOperationTimeout), deadline, time.Second)
					require.Equal(t, "call-hash", callHash)
					require.Equal(t, "owner", owner)
					require.Equal(t, liveControllerLeaseTTL, ttl)
					return test.refreshed, test.err
				},
			}
			service := &OpenAIGatewayService{cache: store}
			require.Equal(t, test.want, service.refreshLiveController("call-hash", "owner"))
		})
	}

	t.Run("timeout", func(t *testing.T) {
		store := &liveControllerRefreshTestStore{
			liveTestStore: &liveTestStore{},
			refresh: func(ctx context.Context, _, _ string, _ time.Duration) (bool, error) {
				<-ctx.Done()
				return false, ctx.Err()
			},
		}
		service := &OpenAIGatewayService{cache: store}
		started := time.Now()
		require.False(t, service.refreshLiveController("call-hash", "owner"))
		require.GreaterOrEqual(t, time.Since(started), liveRedisOperationTimeout)
	})
}

func TestRunLiveObserverConnectionStopsWhenOwnershipIsLost(t *testing.T) {
	store := &liveControllerRefreshTestStore{
		liveTestStore: &liveTestStore{},
		refresh: func(context.Context, string, string, time.Duration) (bool, error) {
			return false, nil
		},
	}
	service := &OpenAIGatewayService{cache: store}
	record := &LiveCallRecord{
		CallHash:  hashLiveCallID("call_lost_owner"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := service.runLiveObserverConnection(record, newLiveTestFrameConn(), "stale-owner")
	require.ErrorIs(t, err, ErrLiveControllerChanged)
}

func TestObserveLiveCallRefreshesLeaseAcrossRetryWindowAndRecovers(t *testing.T) {
	account := &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-access-token", "chatgpt_account_id": "acct_test"},
	}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "live-retry-test-secret"}}
	cipher := newLiveAttestationCipher(cfg)
	record := &LiveCallRecord{
		CallID:     "call_retry",
		CallHash:   hashLiveCallID("call_retry"),
		AccountID:  account.ID,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-retry",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
		Controller: LiveControllerPending,
	}
	var err error
	record.AttestationCiphertext, err = cipher.Encrypt(`{"v":1,"s":0,"t":"v1.retry"}`)
	require.NoError(t, err)
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveRetryConcurrencyCache{ttl: time.Minute, expiresAt: time.Minute}
	upstream := newLiveTestFrameConn()
	upstream.reads <- liveTestFrame{err: coderws.CloseError{Code: coderws.StatusNormalClosure}}
	dialer := &liveRetryDialer{
		failures: 3,
		conn:     upstream,
		advance: func() {
			concurrencyCache.Advance(25 * time.Second)
		},
	}
	service := &OpenAIGatewayService{
		accountRepo:               &liveTestAccountRepo{account: account},
		cache:                     store,
		concurrencyService:        NewConcurrencyService(concurrencyCache),
		openaiWSPassthroughDialer: dialer,
		liveAttestationCipher:     cipher,
		liveTiming: liveRuntimeTiming{
			leaseRefreshInterval:   5 * time.Millisecond,
			controllerPollInterval: time.Millisecond,
			observerRetryInterval:  5 * time.Millisecond,
			sidebandDialTimeout:    5 * time.Millisecond,
		},
	}

	service.observeLiveCall(record.CallHash)

	dialer.mu.Lock()
	require.Equal(t, 4, dialer.attempts)
	dialer.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.GreaterOrEqual(t, concurrencyCache.refreshes, 3)
	require.Greater(t, concurrencyCache.now, time.Minute)
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	store.mu.Lock()
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	store.mu.Unlock()
}

func TestObserveLiveCallFinalizesWhenBlockingDialLosesLease(t *testing.T) {
	account := &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-access-token", "chatgpt_account_id": "acct_test"},
	}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "live-blocking-dial-test-secret"}}
	cipher := newLiveAttestationCipher(cfg)
	record := &LiveCallRecord{
		CallID:     "call_blocking_dial",
		CallHash:   hashLiveCallID("call_blocking_dial"),
		AccountID:  account.ID,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-blocking-dial",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
		Controller: LiveControllerPending,
	}
	var err error
	record.AttestationCiphertext, err = cipher.Encrypt(`{"v":1,"s":0,"t":"v1.blocking"}`)
	require.NoError(t, err)
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveRetryConcurrencyCache{ttl: time.Minute, expiresAt: time.Minute, leaseLost: true}
	dialer := &liveRetryDialer{block: true, canceled: make(chan struct{})}
	service := &OpenAIGatewayService{
		accountRepo:               &liveTestAccountRepo{account: account},
		cache:                     store,
		concurrencyService:        NewConcurrencyService(concurrencyCache),
		openaiWSPassthroughDialer: dialer,
		liveAttestationCipher:     cipher,
		liveTiming: liveRuntimeTiming{
			leaseRefreshInterval:   10 * time.Millisecond,
			controllerPollInterval: time.Millisecond,
			observerRetryInterval:  10 * time.Millisecond,
			sidebandDialTimeout:    10 * time.Millisecond,
		},
	}

	service.observeLiveCall(record.CallHash)

	select {
	case <-dialer.canceled:
	case <-time.After(time.Second):
		t.Fatal("阻塞的 sideband Dial 未在续租窗口内取消")
	}
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	store.mu.Lock()
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	store.mu.Unlock()
	claimed, claimErr := store.ClaimLiveController(context.Background(), record.CallHash, LiveControllerProxy, "proxy-owner")
	require.NoError(t, claimErr)
	require.False(t, claimed, "失去 Live 租约的旧会话不得再被 proxy 接管")
}

func TestFinalizeLiveCallIsIdempotentAndWritesZeroUsage(t *testing.T) {
	record := &LiveCallRecord{
		CallID:          "call_secret",
		CallHash:        hashLiveCallID("call_secret"),
		AccountID:       11,
		APIKeyID:        22,
		UserID:          33,
		GroupID:         44,
		LeaseID:         "lease-1",
		Model:           "gpt-live-test",
		SessionID:       "session-live-123",
		CreatedAt:       time.Now().Add(-time.Second),
		ExpiresAt:       time.Now().Add(time.Hour),
		Controller:      LiveControllerPending,
		InboundEndpoint: "/v1/live",
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	service := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	service.finalizeLiveCall(record)
	service.finalizeLiveCall(record)

	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	log := usageRepo.logs[0]
	usageRepo.mu.Unlock()
	require.Equal(t, RequestTypeLive, log.RequestType)
	require.Equal(t, record.CallHash, log.RequestID)
	require.NotEqual(t, record.CallID, log.RequestID)
	require.NotNil(t, log.DurationMs)
	require.NotNil(t, log.SessionID)
	require.Equal(t, record.SessionID, *log.SessionID)
	require.Zero(t, log.InputTokens)
	require.Zero(t, log.OutputTokens)
	require.Zero(t, log.TotalCost)
	require.Zero(t, log.ActualCost)
}

func TestGetLiveCallForIdentityRejectsMismatchedCaller(t *testing.T) {
	groupID := int64(44)
	record := &LiveCallRecord{
		CallID:     "call_identity",
		CallHash:   hashLiveCallID("call_identity"),
		APIKeyID:   22,
		UserID:     33,
		GroupID:    groupID,
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	service := &OpenAIGatewayService{cache: store}

	_, err := service.GetLiveCallForIdentity(context.Background(), record.CallID, LiveCallIdentity{
		APIKeyID: 99,
		UserID:   record.UserID,
		GroupID:  &groupID,
	})
	require.ErrorIs(t, err, ErrLiveIdentityMismatch)

	loaded, err := service.GetLiveCallForIdentity(context.Background(), record.CallID, LiveCallIdentity{
		APIKeyID: record.APIKeyID,
		UserID:   record.UserID,
		GroupID:  &groupID,
	})
	require.NoError(t, err)
	require.Equal(t, record.AccountID, loaded.AccountID)
}

func TestProxyLiveSidebandForwardsTextAndBinary(t *testing.T) {
	account := &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	record := &LiveCallRecord{
		CallID:     "call_proxy",
		CallHash:   hashLiveCallID("call_proxy"),
		AccountID:  account.ID,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Minute),
		Controller: LiveControllerPending,
	}
	attestationCipher := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "live-sideband-test-secret"},
	})
	var err error
	record.AttestationCiphertext, err = attestationCipher.Encrypt(`{"v":1,"s":0,"t":"v1.sideband"}`)
	require.NoError(t, err)
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	upstream := newLiveTestFrameConn()
	dialer := &liveTestDialer{conn: upstream}
	service := &OpenAIGatewayService{
		accountRepo:               &liveTestAccountRepo{account: account},
		cache:                     store,
		openaiWSPassthroughDialer: dialer,
		liveAttestationCipher:     attestationCipher,
	}
	proxyResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		downstream, err := coderws.Accept(writer, request, nil)
		if err != nil {
			proxyResult <- err
			return
		}
		defer func() { _ = downstream.CloseNow() }()
		proxyResult <- service.ProxyLiveSideband(request.Context(), record, downstream)
	}))
	defer server.Close()

	client, _, err := coderws.Dial(
		context.Background(),
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"client.text"}`)))
	clientText := <-upstream.writes
	require.Equal(t, coderws.MessageText, clientText.messageType)
	require.JSONEq(t, `{"type":"client.text"}`, string(clientText.payload))

	require.NoError(t, client.Write(ctx, coderws.MessageBinary, []byte{1, 2, 3}))
	clientBinary := <-upstream.writes
	require.Equal(t, coderws.MessageBinary, clientBinary.messageType)
	require.Equal(t, []byte{1, 2, 3}, clientBinary.payload)

	upstream.reads <- liveTestFrame{messageType: coderws.MessageText, payload: []byte(`{"type":"server.text"}`)}
	messageType, payload, err := client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, messageType)
	require.JSONEq(t, `{"type":"server.text"}`, string(payload))

	upstream.reads <- liveTestFrame{messageType: coderws.MessageBinary, payload: []byte{4, 5, 6}}
	messageType, payload, err = client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageBinary, messageType)
	require.Equal(t, []byte{4, 5, 6}, payload)

	require.Equal(t, "wss://chatgpt.com/backend-api/codex/call_proxy", dialer.url)
	require.Equal(t, "Bearer test-access-token", dialer.headers.Get("Authorization"))
	require.Equal(t, "acct_test", dialer.headers.Get("Chatgpt-Account-Id"))
	require.Equal(t, `{"v":1,"s":0,"t":"v1.sideband"}`, dialer.headers.Get(liveAttestationHeader))
	upstream.reads <- liveTestFrame{err: coderws.CloseError{Code: coderws.StatusNormalClosure}}
	require.ErrorIs(t, <-proxyResult, ErrLiveCallNotFound)
}

// TestLiveSessionEndedTreatsLeaseLossAsTerminal 锁定：租约续租失败（ErrLiveUnavailable）
// 必须判为会话终结。RefreshLiveLease 的 Lua 在 leaseID 被 GC 后不会重新写入，若把它
// 当临时错误交给 observer 重连，会话会空转到 ExpiresAt 且不计入任何并发限制。
func TestLiveSessionEndedTreatsLeaseLossAsTerminal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"租约丢失", ErrLiveUnavailable, true},
		{"租约丢失（被包装）", fmt.Errorf("refresh live lease: %w", ErrLiveUnavailable), true},
		{"上游报告会话已关闭", ErrLiveCallNotFound, true},
		{"到达会话时长上限", context.DeadlineExceeded, true},
		{"请求上下文被取消", context.Canceled, true},
		{"控制权被他人接管", ErrLiveControllerChanged, false},
		{"临时读错误", errors.New("unexpected EOF"), false},
		{"无错误", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, liveSessionEnded(tc.err))
		})
	}
}

// TestWaitForLiveObserverRetryReportsTerminalState 锁定：重拨等待期发现过期或失去
// controller 时返回明确原因，让 observeLiveCall 区分 finalize 与 proxy handoff。
func TestWaitForLiveObserverRetryReportsTerminalState(t *testing.T) {
	record := &LiveCallRecord{
		CallID:          "call_expired",
		CallHash:        hashLiveCallID("call_expired"),
		Controller:      LiveControllerObserver,
		ControllerOwner: "observer-owner",
		ExpiresAt:       time.Now().Add(-time.Minute),
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	svc := &OpenAIGatewayService{cache: store}

	require.ErrorIs(t, svc.waitForLiveObserverRetry(record, "observer-owner"), context.DeadlineExceeded)
	record.ExpiresAt = time.Now().Add(time.Hour)

	// A stale observer must stop even when the replacement has the same role.
	require.NoError(t, store.SaveLiveCall(context.Background(), &LiveCallRecord{
		CallID:          record.CallID,
		CallHash:        record.CallHash,
		Controller:      LiveControllerObserver,
		ControllerOwner: "replacement-owner",
		ExpiresAt:       time.Now().Add(time.Hour),
	}, time.Hour))
	require.ErrorIs(t, svc.waitForLiveObserverRetry(record, "observer-owner"), ErrLiveControllerChanged)
}
