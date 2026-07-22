package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAccountCredentialImportContentsEnrichesOpenAIJWTIdentity(t *testing.T) {
	token := importTestJWT(t, map[string]any{
		"email": "seat@example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "team-account",
			"chatgpt_user_id":    "user-seat",
			"chatgpt_plan_type":  "team",
			"organizations": []map[string]any{
				{"id": "org-team", "is_default": true},
			},
		},
	})
	content := `{"tokens":{"access_token":"` + token + `"}}`

	sources, errs := ParseAccountCredentialImportContents([]string{content})

	require.Empty(t, errs)
	require.Len(t, sources, 1)
	require.Equal(t, PlatformOpenAI, sources[0].Platform)
	require.Equal(t, AccountCredentialImportKindOAuthCredentials, sources[0].Kind)
	require.Equal(t, "seat@example.com", sources[0].Credentials["email"])
	require.Equal(t, "team-account", sources[0].Credentials["chatgpt_account_id"])
	require.Equal(t, "user-seat", sources[0].Credentials["chatgpt_user_id"])
	require.Equal(t, "org-team", sources[0].Credentials["organization_id"])
	require.Equal(t, "team", sources[0].Credentials["plan_type"])
}

func TestParseAccountCredentialImportContentsAcceptsOwnedAgentIdentityWhenEnabled(t *testing.T) {
	privateKey := importTestAgentIdentityPrivateKey(t)
	content, err := json.Marshal(map[string]any{
		"name":     "personal-agent",
		"platform": PlatformOpenAI,
		"type":     AccountTypeOAuth,
		"credentials": map[string]any{
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   "runtime-import",
			"agent_private_key":  privateKey,
			"task_id":            "task-import",
			"chatgpt_account_id": "team-import",
			"chatgpt_user_id":    "user-import",
			"email":              "agent@example.invalid",
			"plan_type":          "pro",
		},
	})
	require.NoError(t, err)

	sources, errs := ParseAccountCredentialImportContentsWithOptions([]string{string(content)}, AccountCredentialImportOptions{
		OpenAIAgentIdentityImport: true,
	})

	require.Empty(t, errs)
	require.Len(t, sources, 1)
	require.Equal(t, AccountCredentialImportKindOpenAIAgentIdentity, sources[0].Kind)
	require.Equal(t, PlatformOpenAI, sources[0].Platform)
	require.Equal(t, OpenAIAuthModeAgentIdentity, sources[0].Credentials["auth_mode"])
	require.Equal(t, "team-import", sources[0].Credentials["chatgpt_account_id"])
	require.NotContains(t, sources[0].Credentials, "access_token")
	require.NotContains(t, sources[0].Credentials, "refresh_token")
}

func TestParseAccountCredentialImportContentsAcceptsRawAgentIdentityAuthJSON(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity,
		"agent_identity": map[string]any{
			"agent_runtime_id":  "runtime-import",
			"agent_private_key": importTestAgentIdentityPrivateKey(t),
			"account_id":        "team-import",
			"chatgpt_user_id":   "user-import",
		},
	})
	require.NoError(t, err)

	sources, errs := ParseAccountCredentialImportContentsWithOptions([]string{string(content)}, AccountCredentialImportOptions{
		OpenAIAgentIdentityImport: true,
	})

	require.Empty(t, errs)
	require.Len(t, sources, 1)
	require.Equal(t, AccountCredentialImportKindOpenAIAgentIdentity, sources[0].Kind)
	require.Equal(t, "runtime-import", sources[0].Credentials["agent_runtime_id"])
	require.Equal(t, "team-import", sources[0].Credentials["chatgpt_account_id"])
}

func TestParseAccountCredentialImportContentsKeepsAgentIdentityDisabledByDefault(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity,
		"agent_identity": map[string]any{
			"agent_runtime_id":  "runtime-import",
			"agent_private_key": importTestAgentIdentityPrivateKey(t),
			"account_id":        "team-import",
			"chatgpt_user_id":   "user-import",
		},
	})
	require.NoError(t, err)

	sources, errs := ParseAccountCredentialImportContents([]string{string(content)})

	require.Empty(t, sources)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, "disallowed credential field: auth_mode")
}

func TestParseAccountCredentialImportContentsRejectsAgentIdentityCustomUpstream(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity,
		"base_url":  "https://example.invalid/v1",
		"agent_identity": map[string]any{
			"agent_runtime_id":  "runtime-import",
			"agent_private_key": importTestAgentIdentityPrivateKey(t),
			"account_id":        "team-import",
			"chatgpt_user_id":   "user-import",
		},
	})
	require.NoError(t, err)

	sources, errs := ParseAccountCredentialImportContentsWithOptions([]string{string(content)}, AccountCredentialImportOptions{
		OpenAIAgentIdentityImport: true,
	})

	require.Empty(t, sources)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, "disallowed credential field: base_url")
}

func TestParseAccountCredentialImportContentsRejectsAgentIdentityMixedWithOAuthToken(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity,
		"agent_identity": map[string]any{
			"agent_runtime_id":  "runtime-import",
			"agent_private_key": importTestAgentIdentityPrivateKey(t),
			"account_id":        "team-import",
			"chatgpt_user_id":   "user-import",
			"access_token":      "unexpected-token",
		},
	})
	require.NoError(t, err)

	sources, errs := ParseAccountCredentialImportContentsWithOptions([]string{string(content)}, AccountCredentialImportOptions{
		OpenAIAgentIdentityImport: true,
	})

	require.Empty(t, sources)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, "agent identity must not include access_token")
}

func TestParseAccountCredentialImportContentsRejectsUnknownAuthMode(t *testing.T) {
	content := `{"auth_mode":"personalAccessToken","access_token":"token"}`

	sources, errs := ParseAccountCredentialImportContentsWithOptions([]string{content}, AccountCredentialImportOptions{
		OpenAIAgentIdentityImport: true,
	})

	require.Empty(t, sources)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, "disallowed credential field: auth_mode")
}

func TestParseAccountCredentialImportContentsRejectsLegacyOpenAIAuthMode(t *testing.T) {
	content := `{"openai_auth_mode":"personal_access_token","access_token":"token"}`

	sources, errs := ParseAccountCredentialImportContents([]string{content})

	require.Empty(t, sources)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Message, "disallowed credential field: openai_auth_mode")
}

func TestParseAccountCredentialImportContentsKiroConfigRequiresSwitch(t *testing.T) {
	content := `{
		"clientId":"client-id",
		"clientSecret":"client-secret",
		"refreshToken":"refresh-token",
		"email":"kiro@example.com",
		"provider":"BuilderId",
		"region":"us-east-1",
		"subscription":"KIRO FREE",
		"creditLimit":50,
		"creditUsed":1
	}`

	sources, errs := ParseAccountCredentialImportContents([]string{content})
	require.Empty(t, errs)
	require.Len(t, sources, 1)
	require.NotEqual(t, AccountCredentialImportKindKiroConfig, sources[0].Kind)

	sources, errs = ParseAccountCredentialImportContentsWithOptions([]string{content}, AccountCredentialImportOptions{
		KiroConfigImport: true,
	})
	require.Empty(t, errs)
	require.Len(t, sources, 1)
	require.Equal(t, AccountCredentialImportKindKiroConfig, sources[0].Kind)
	require.Equal(t, PlatformKiro, sources[0].Platform)
	require.Equal(t, "kiro@example.com", sources[0].Name)
	require.Equal(t, "refresh-token", sources[0].Token)
	require.Equal(t, "client-id", sources[0].ClientID)
	require.Equal(t, "client-secret", sources[0].ClientSecret)
	require.Equal(t, "idc", sources[0].AuthMethod)
	require.Equal(t, "BuilderId", sources[0].Provider)
	require.Equal(t, "us-east-1", sources[0].Region)
	require.Equal(t, "KIRO FREE", sources[0].Credentials["plan_type"])
	require.Equal(t, json.Number("50"), sources[0].Credentials["credit_limit"])
	require.Equal(t, json.Number("1"), sources[0].Credentials["credit_used"])
	require.Equal(t, "kiro_config", sources[0].Extra["import_source"])
}

func TestParseAccountCredentialImportContentsClaudeWebNestedArrayRequiresSwitch(t *testing.T) {
	content := `[[
		{
			"email":"first@example.com",
			"uuid":"account-1",
			"org_uuid":"org-1",
			"org_name":"First Organization",
			"cookies":{
				"sessionKey":"sk-ant-sid02-first",
				"sessionKeyLC":"123",
				"routingHint":"routing-1",
				"__cf_bm":"cf-1",
				"_cfuvid":"cfuvid-1"
			},
			"saved_at":"2026-07-09T03:37:14Z"
		},
		{
			"email_address":"second@example.com",
			"uuid":"account-2",
			"org_uuid":"org-2",
			"cookies":{"sessionKey":"sk-ant-sid02-second"}
		}
	]]`

	sources, errs := ParseAccountCredentialImportContents([]string{content})
	require.Empty(t, sources)
	require.NotEmpty(t, errs)

	sources, errs = ParseAccountCredentialImportContentsWithOptions([]string{content}, AccountCredentialImportOptions{
		ClaudeWebImport: true,
	})
	require.Empty(t, errs)
	require.Len(t, sources, 2)
	require.Equal(t, AccountCredentialImportKindClaudeWebSession, sources[0].Kind)
	require.Equal(t, PlatformAnthropic, sources[0].Platform)
	require.Equal(t, "first@example.com", sources[0].Name)
	require.Equal(t, "sk-ant-sid02-first", sources[0].Credentials[ClaudeWebSessionKeyCredential])
	require.Equal(t, "org-1", sources[0].Credentials[ClaudeWebOrganizationCredential])
	require.NotContains(t, sources[0].Credentials, ClaudeWebBrowserCookieCredential)
	require.NotContains(t, sources[0].Credentials, ClaudeWebRoutingHintCredential)
	require.NotContains(t, sources[0].Credentials, ClaudeWebCFBMCredential)
	require.Equal(t, true, sources[0].Extra[ClaudeWebSessionExtraKey])
	require.Equal(t, "2026-07-09T03:37:14Z", sources[0].Extra["saved_at"])
	require.Equal(t, "second@example.com", sources[1].Name)
}

func importTestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func importTestAgentIdentityPrivateKey(t *testing.T) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(der)
}
