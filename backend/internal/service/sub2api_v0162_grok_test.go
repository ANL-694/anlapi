package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSub2APIV0162GrokVideoRelayAndSignedURLPolicy(t *testing.T) {
	relay, err := grokMediaSignedVideoContentURL([]byte(`{"video":{"url":"/v1/videos/task-1/content"}}`), "task-1")
	require.NoError(t, err)
	require.Empty(t, relay)

	_, err = grokMediaSignedVideoContentURL([]byte(`{"video":{"url":"/v1/videos/task-2/content"}}`), "task-1")
	require.ErrorContains(t, err, "unsupported video content URL")

	signed, err := grokMediaSignedVideoContentURL([]byte(`{"video":{"url":"https://vidgen.x.ai/video.mp4"}}`), "task-1")
	require.NoError(t, err)
	require.Equal(t, "https://vidgen.x.ai/video.mp4", signed)
}

func TestSub2APIV0162GrokCachePrefersClaudeCodeSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set("api_key", &APIKey{ID: 701, Group: &Group{Platform: PlatformGrok}})
	c.Request.Header.Set(claudeCodeSessionHeader, "cc-session-abc")
	c.Request.Header.Set("session_id", "fallback-session")

	first := resolveGrokCacheIdentity(c, []byte(`{"input":"turn one"}`), "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, []byte(`{"input":"turn two"}`), "different-key", "grok-4.5")
	require.NotEmpty(t, first)
	require.Equal(t, first, second)

	meta := []byte(`{"metadata":{"user_id":"{\"session_id\":\"meta-session\"}"}}`)
	require.Equal(t, "meta-session", extractClaudeCodeSessionIDFromPayload(meta))
}

func TestSub2APIV0162GrokManualTestBypassesSchedulingGate(t *testing.T) {
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusError,
		Schedulable: false,
		Credentials: map[string]any{
			"access_token":  "manual-test-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	provider := NewGrokTokenProvider(nil, nil)

	_, productionErr := provider.GetAccessToken(context.Background(), account)
	require.ErrorIs(t, productionErr, errOAuthRefreshAccountStateChanged)

	token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "manual-test-token", token)
}

func TestSub2APIV0162GrokCountTokensAndRetryPolicy(t *testing.T) {
	count, err := EstimateGrokCountTokens([]byte(`{"model":"grok-4","messages":[{"role":"user","content":"hello"}]}`))
	require.NoError(t, err)
	require.Positive(t, count)

	_, err = EstimateGrokCountTokens([]byte(`{"messages":[]}`))
	require.Error(t, err)

	require.True(t, isRetryableGrokBillingStatus(http.StatusBadGateway))
	require.True(t, isRetryableGrokBillingStatus(http.StatusServiceUnavailable))
	require.True(t, isRetryableGrokBillingStatus(http.StatusGatewayTimeout))
	require.False(t, isRetryableGrokBillingStatus(http.StatusUnauthorized))
	require.False(t, isRetryableGrokBillingStatus(http.StatusTooManyRequests))
}

func TestSub2APIV0162OpsReportUsesANLShellAndEscapesTitle(t *testing.T) {
	html := buildOpsSummaryEmailHTML(`<script>alert(1)</script>`, time.Unix(0, 0), time.Unix(60, 0), nil)
	require.Contains(t, html, "ANL API 运维中心")
	require.Contains(t, html, "当前周期暂无可用数据")
	require.Contains(t, html, "&lt;script&gt;alert(1)&lt;/script&gt;")
	require.NotContains(t, html, `<script>alert(1)</script>`)
}
