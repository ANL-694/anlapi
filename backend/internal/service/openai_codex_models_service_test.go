package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"anlapi/internal/config"
	"github.com/stretchr/testify/require"
)

type anlCodexModelsUpstreamStub struct {
	HTTPUpstream
	do func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)
}

func (s *anlCodexModelsUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.do(req, proxyURL, accountID, accountConcurrency)
}

func newANLCodexModelsAPIKeyTestService(upstream HTTPUpstream) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled: false,
		}}},
		httpUpstream: upstream,
	}
}

func newANLCodexModelsAPIKeyTestAccount(baseURL string) *Account {
	credentials := map[string]any{"api_key": "test-upstream-key"}
	if baseURL != "" {
		credentials["base_url"] = baseURL
	}
	return &Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: credentials,
		Concurrency: 3,
	}
}

func newANLCodexModelsOAuthTestAccount() *Account {
	return &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "test-account",
		},
	}
}

func TestFetchCodexModelsManifestAPIKeyConvertsStandardOpenAIModelList(t *testing.T) {
	upstreamBody := `{"object":"list","data":[{"id":"gpt-5.6","object":"model"},{"id":"  ","object":"model"},{"id":"gpt-5.6-codex","object":"model"}]}`
	upstream := &anlCodexModelsUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		header := make(http.Header)
		header.Set("ETag", `W/"openai-list"`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}, nil
	}}

	s := newANLCodexModelsAPIKeyTestService(upstream)
	manifest, err := s.FetchCodexModelsManifest(
		context.Background(),
		newANLCodexModelsAPIKeyTestAccount("https://upstream.example/v1"),
		"0.144.0",
		"",
	)
	require.NoError(t, err)
	require.Equal(t, `{"models":[{"slug":"gpt-5.6"},{"slug":"gpt-5.6-codex"}]}`, string(manifest.Body))
	require.Equal(t, `W/"openai-list"`, manifest.ETag)
}

func TestConvertOpenAIModelListToCodexManifest(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "standard list",
			body: `{"object":"list","data":[{"id":"m-1"},{"id":"m-2"}]}`,
			want: `{"models":[{"slug":"m-1"},{"slug":"m-2"}]}`,
		},
		{
			name: "codex manifest unchanged",
			body: `{"models":[{"slug":"m-1"}]}`,
			want: `{"models":[{"slug":"m-1"}]}`,
		},
		{
			name: "empty data unchanged",
			body: `{"object":"list","data":[]}`,
			want: `{"object":"list","data":[]}`,
		},
		{
			name: "invalid JSON unchanged",
			body: `{"data":`,
			want: `{"data":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, string(convertOpenAIModelListToCodexManifest([]byte(tt.body))))
		})
	}
}

type anlCodexModelsAccountStateRepo struct {
	AccountRepository
	mu                  sync.Mutex
	setErrorCalls       int
	lastErrorMsg        string
	setTempUnschedCalls int
	lastTempReason      string
}

func (r *anlCodexModelsAccountStateRepo) SetError(_ context.Context, _ int64, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	r.lastErrorMsg = errorMsg
	return nil
}

func (r *anlCodexModelsAccountStateRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempUnschedCalls++
	r.lastTempReason = reason
	return nil
}

func newANLCodexModels401TestService(repo AccountRepository) *OpenAIGatewayService {
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	s := &OpenAIGatewayService{rateLimitService: rateLimitService}
	rateLimitService.SetAccountRuntimeBlocker(s)
	return s
}

func TestFetchCodexModelsManifestOAuth401MarksAccountUnschedulable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":{"message":"invalid token"}}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	repo := &anlCodexModelsAccountStateRepo{}
	s := newANLCodexModels401TestService(repo)
	account := newANLCodexModelsOAuthTestAccount()
	account.Credentials["refresh_token"] = "test-refresh-token"

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.True(t, IsRetryableCodexModelsManifestError(err), "manifest 401 should allow account failover")
	require.Equal(t, 1, repo.setTempUnschedCalls, "OAuth 401 should temp-unschedule the account")
	require.Equal(t, 0, repo.setErrorCalls)
	require.True(t, s.isOpenAIAccountRuntimeBlocked(account), "account should be runtime-blocked after manifest 401")
}

func TestFetchCodexModelsManifestOAuth401TokenRevokedDisablesAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"token_revoked","message":"token has been revoked"}}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	repo := &anlCodexModelsAccountStateRepo{}
	s := newANLCodexModels401TestService(repo)
	account := newANLCodexModelsOAuthTestAccount()
	account.Credentials["refresh_token"] = "test-refresh-token"

	_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.True(t, IsRetryableCodexModelsManifestError(err))
	require.Equal(t, 1, repo.setErrorCalls, "revoked token should permanently disable the account")
	require.Contains(t, repo.lastErrorMsg, "Token revoked")
	require.Equal(t, 0, repo.setTempUnschedCalls)
}

func TestFetchCodexModelsManifestAgentIdentity401DoesNotDisableAccount(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	account := &Account{
		ID:       6,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   "runtime-401",
			"agent_private_key":  base64.StdEncoding.EncodeToString(der),
			"task_id":            "task-401",
			"chatgpt_account_id": "account-401",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"some non-task 401"}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	repo := &anlCodexModelsAccountStateRepo{}
	s := newANLCodexModels401TestService(repo)

	_, err = s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	require.Error(t, err)
	require.Equal(t, 0, repo.setErrorCalls, "agent identity 401s must not disable the account")
	require.Equal(t, 0, repo.setTempUnschedCalls)
}

func TestFetchCodexModelsManifestAPIKey401KeepsNoFailoverAndNoDisable(t *testing.T) {
	upstream := &anlCodexModelsUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"invalid api key"}`)),
		}, nil
	}}

	repo := &anlCodexModelsAccountStateRepo{}
	s := newANLCodexModelsAPIKeyTestService(upstream)
	s.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newANLCodexModelsAPIKeyTestAccount("https://upstream.example"),
		"0.144.0",
		"",
	)
	require.Error(t, err)
	require.False(t, IsRetryableCodexModelsManifestError(err), "custom upstream manifest 401 keeps the no-failover behavior")
	require.Equal(t, 0, repo.setErrorCalls, "custom upstream manifest 401 must not disable the account")
	require.Equal(t, 0, repo.setTempUnschedCalls)
}
