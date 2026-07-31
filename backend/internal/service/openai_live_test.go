package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"anlapi/internal/config"
	"anlapi/internal/pkg/tlsfingerprint"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type liveHTTPUpstreamStub struct {
	request *http.Request
	body    []byte
	calls   int
}

type liveAttestationStub struct {
	header string
	err    error
}

func (s liveAttestationStub) Check(context.Context) error {
	return s.err
}

func (s liveAttestationStub) Generate(context.Context) (string, error) {
	return s.header, s.err
}

func (s *liveHTTPUpstreamStub) Do(
	request *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	s.calls++
	s.request = request
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	s.body = body
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Location": {"/backend-api/codex/call_test"},
		},
		Body: io.NopCloser(strings.NewReader("v=0\r\n")),
	}, nil
}

type liveCreateConcurrencyCache struct {
	ConcurrencyCache
	mu                    sync.Mutex
	accountLeases         map[int64]map[string]struct{}
	userLeases            map[int64]map[string]struct{}
	replacingUserSlotArgs []bool
}

func (c *liveCreateConcurrencyCache) GetAccountsLoadBatch(
	_ context.Context,
	accounts []AccountWithConcurrency,
) (map[int64]*AccountLoadInfo, error) {
	result := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		result[account.ID] = &AccountLoadInfo{AccountID: account.ID}
	}
	return result, nil
}

func (c *liveCreateConcurrencyCache) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}

func (c *liveCreateConcurrencyCache) AcquireLiveLease(
	_ context.Context,
	accountID int64,
	accountMax int,
	userID int64,
	userMax int,
	_ int64,
	leaseID string,
	replacingRegularUserSlot bool,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replacingUserSlotArgs = append(c.replacingUserSlotArgs, replacingRegularUserSlot)
	if c.accountLeases == nil {
		c.accountLeases = make(map[int64]map[string]struct{})
	}
	if c.userLeases == nil {
		c.userLeases = make(map[int64]map[string]struct{})
	}
	if accountMax > 0 && len(c.accountLeases[accountID]) >= accountMax {
		return false, nil
	}
	if userMax > 0 && len(c.userLeases[userID]) >= userMax {
		return false, nil
	}
	if c.accountLeases[accountID] == nil {
		c.accountLeases[accountID] = make(map[string]struct{})
	}
	if c.userLeases[userID] == nil {
		c.userLeases[userID] = make(map[string]struct{})
	}
	c.accountLeases[accountID][leaseID] = struct{}{}
	c.userLeases[userID][leaseID] = struct{}{}
	return true, nil
}

func (c *liveCreateConcurrencyCache) RefreshLiveLease(
	_ context.Context,
	accountID int64,
	userID int64,
	_ int64,
	leaseID string,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, accountFound := c.accountLeases[accountID][leaseID]
	_, userFound := c.userLeases[userID][leaseID]
	return accountFound && userFound, nil
}

func (c *liveCreateConcurrencyCache) ReleaseLiveLease(
	_ context.Context,
	accountID int64,
	userID int64,
	_ int64,
	leaseID string,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.accountLeases[accountID], leaseID)
	delete(c.userLeases[userID], leaseID)
	return nil
}

type liveCreateStore struct {
	*liveTestStore
}

func (s *liveCreateStore) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, errors.New("session account not found")
}

func (s *liveCreateStore) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (s *liveCreateStore) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *liveCreateStore) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

func (s *liveCreateStore) ClaimLiveController(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (s *liveHTTPUpstreamStub) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, accountConcurrency)
}

func TestLiveCapabilityOnlyAllowsOpenAIOAuth(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{Platform: PlatformGrok, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModePersonalAccessToken,
		},
	}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModeAgentIdentity,
		},
	}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
}

func TestValidateLiveCallRequestDoesNotRequireDelegation(t *testing.T) {
	request := &LiveCallRequest{
		SDP:     "v=0\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test","instructions":"hello"}`),
	}
	require.NoError(t, ValidateLiveCallRequest(request))
	require.NotContains(t, string(request.Session), "delegation")
}

func TestCreateUpstreamLiveCallPreservesSession(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	service := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	session := json.RawMessage(`{
		"model":"gpt-live-test",
		"delegation":{"type":"client"},
		"custom":{"keep":true}
	}`)

	created, err := service.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: session,
	}, `{"v":1,"s":0,"t":"v1.test"}`)
	require.NoError(t, err)
	require.Equal(t, "call_test", created.CallID)
	require.Equal(t, []byte("v=0\r\n"), created.SDP)

	var forwarded struct {
		SDP     string          `json:"sdp"`
		Session json.RawMessage `json:"session"`
	}
	require.NoError(t, json.Unmarshal(upstream.body, &forwarded))
	require.Equal(t, "v=offer\r\n", forwarded.SDP)
	require.JSONEq(t, string(session), string(forwarded.Session))
	require.Equal(t, "Bearer test-access-token", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "acct_test", upstream.request.Header.Get("Chatgpt-Account-Id"))
	require.Equal(t, "quicksilver=v2", upstream.request.Header.Get("OpenAI-Alpha"))
	require.Equal(t, `{"v":1,"s":0,"t":"v1.test"}`, upstream.request.Header.Get(liveAttestationHeader))
	require.NotEmpty(t, upstream.request.Header.Get("Session-Id"))
	require.NotEmpty(t, upstream.request.Header.Get("Thread-Id"))
	require.Empty(t, upstream.request.Header.Get("OpenAI-Beta"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.request.Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.request.Context()))
}

func TestCreateLiveCallEnforcesAccountConcurrencyWithoutRegularAccountAllowance(t *testing.T) {
	account := Account{
		ID:          71,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	upstream := &liveHTTPUpstreamStub{}
	store := &liveCreateStore{liveTestStore: &liveTestStore{}}
	concurrencyCache := &liveCreateConcurrencyCache{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.JWT.Secret = "live-create-concurrency-test"
	service := &OpenAIGatewayService{
		accountRepo:           schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache:                 store,
		cfg:                   cfg,
		concurrencyService:    NewConcurrencyService(concurrencyCache),
		httpUpstream:          upstream,
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: newLiveAttestationCipher(cfg),
	}
	request := &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test"}`),
	}
	identity := LiveCallIdentity{APIKeyID: 72, UserID: 73}

	first, err := service.CreateLiveCall(context.Background(), request, identity, 0)
	require.NoError(t, err)
	require.NotNil(t, first)
	store.mu.Lock()
	firstLeaseID := store.record.LeaseID
	store.mu.Unlock()

	second, err := service.CreateLiveCall(context.Background(), request, identity, 0)
	require.ErrorIs(t, err, ErrLiveConcurrencyFull)
	require.Nil(t, second)
	require.Equal(t, 1, upstream.calls)

	service.releaseLiveLease(account.ID, identity.UserID, identity.APIKeyID, firstLeaseID)
	third, err := service.CreateLiveCall(context.Background(), request, identity, 0)
	require.NoError(t, err)
	require.NotNil(t, third)
	require.Equal(t, 2, upstream.calls)
	concurrencyCache.mu.Lock()
	require.Equal(t, []bool{true, true, true}, concurrencyCache.replacingUserSlotArgs)
	concurrencyCache.mu.Unlock()
}

func TestLiveAttestationCipherRoundTripAndRejectsOtherInstanceKey(t *testing.T) {
	first := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "first-live-secret"},
	})
	second := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "second-live-secret"},
	})
	require.NotNil(t, first)
	require.NotNil(t, second)

	ciphertext, err := first.Encrypt(`{"v":1,"s":0,"t":"v1.opaque"}`)
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "opaque")

	plaintext, err := first.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, `{"v":1,"s":0,"t":"v1.opaque"}`, plaintext)

	_, err = second.Decrypt(ciphertext)
	require.Error(t, err)
}

func TestPrepareLiveAttestationEncryptsHeaderAndReturnsExplicitProviderError(t *testing.T) {
	cipher := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "live-attestation-test-secret"},
	})
	service := &OpenAIGatewayService{
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: cipher,
	}
	header, ciphertext, err := service.prepareLiveAttestation(context.Background())
	require.NoError(t, err)
	require.Equal(t, `{"v":1,"s":0,"t":"v1.test"}`, header)
	require.NotContains(t, ciphertext, "v1.test")
	decrypted, err := cipher.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, header, decrypted)

	service.liveAttestation = liveAttestationStub{err: errors.New("macOS app missing")}
	_, _, err = service.prepareLiveAttestation(context.Background())
	var unavailable *LiveAttestationUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Contains(t, unavailable.Error(), "macOS app missing")
}

func TestLiveMaxSessionDurationDefaultsAndOverrides(t *testing.T) {
	require.Equal(t, defaultLiveMaxSessionDuration, (&OpenAIGatewayService{}).liveMaxSessionDuration())
	require.Equal(
		t,
		90*time.Second,
		(&OpenAIGatewayService{cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Live: config.GatewayLiveConfig{MaxSessionDurationSeconds: 90},
			},
		}}).liveMaxSessionDuration(),
	)
}

func TestLiveSidebandNormalCloseEndsCall(t *testing.T) {
	normalClose := coderws.CloseError{Code: coderws.StatusNormalClosure}
	require.ErrorIs(t, liveSidebandReadError(normalClose), ErrLiveCallNotFound)

	abnormalClose := coderws.CloseError{Code: coderws.StatusInternalError}
	require.Equal(t, abnormalClose, liveSidebandReadError(abnormalClose))
}

func TestLiveCreateFailoverUsesExistingOpenAIPolicy(t *testing.T) {
	service := &OpenAIGatewayService{}
	require.False(t, service.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"message":"invalid session"}}`),
	}))
	require.True(t, service.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode: http.StatusForbidden,
	}))
	require.True(t, service.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}))
	require.True(t, service.shouldFailoverLiveCreateError(errors.New("transport failed")))
}

func TestLiveCallIDFromLocation(t *testing.T) {
	callID, err := liveCallIDFromLocation("https://chatgpt.com/backend-api/codex/call_123?intent=quicksilver")
	require.NoError(t, err)
	require.Equal(t, "call_123", callID)

	callID, err = liveCallIDFromLocation("/backend-api/codex/call_456")
	require.NoError(t, err)
	require.Equal(t, "call_456", callID)
}

func TestRequestTypeLive(t *testing.T) {
	require.True(t, RequestTypeLive.IsValid())
	require.Equal(t, "live", RequestTypeLive.String())
	parsed, err := ParseUsageRequestType("live")
	require.NoError(t, err)
	require.Equal(t, RequestTypeLive, parsed)
}
