package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateOwnedFreeModelAPIKeyAcceptsOfficialProviderEndpoints(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"groq":               "https://api.groq.com/openai/v1",
		"cerebras":           "https://api.cerebras.ai/v1",
		"openrouter":         "https://openrouter.ai/api/v1",
		"github":             "https://models.github.ai/inference",
		"gemini_openai":      "https://generativelanguage.googleapis.com/v1beta/openai",
		"cohere":             "https://api.cohere.ai/compatibility/v1",
		"ovh_ai_endpoints":   "https://oai.endpoints.kepler.ai.cloud.ovh.net/v1",
		"mistral":            "https://api.mistral.ai/v1",
		"huggingface":        "https://router.huggingface.co/v1",
		"zhipu":              "https://api.z.ai/api/paas/v4",
		"qwen_intl":          "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		"siliconflow_global": "https://api.siliconflow.com/v1",
		"nvidia_nim":         "https://integrate.api.nvidia.com/v1",
		"ollama":             "https://ollama.com/v1",
		"opencode":           "https://opencode.ai/zen/v1",
	}

	for provider, baseURL := range tests {
		provider := provider
		baseURL := baseURL
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			err := validateOwnedFreeModelAPIKey(
				PlatformOpenAI,
				map[string]any{"api_key": "test-key", "base_url": baseURL + "/"},
				map[string]any{"free_model_provider": provider},
			)
			require.NoError(t, err)
		})
	}
}

func TestValidateOwnedFreeModelAPIKeyAcceptsCloudflareAccountIDOnly(t *testing.T) {
	t.Parallel()

	err := validateOwnedFreeModelAPIKey(
		PlatformOpenAI,
		map[string]any{
			"api_key":  "test-key",
			"base_url": "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1",
		},
		map[string]any{"free_model_provider": "cloudflare_workers_ai"},
	)
	require.NoError(t, err)
}

func TestValidateOwnedFreeModelAPIKeyRejectsUntrustedSources(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		platform string
		provider any
		baseURL  any
	}{
		"non OpenAI platform": {
			platform: PlatformAnthropic,
			provider: "groq",
			baseURL:  "https://api.groq.com/openai/v1",
		},
		"missing provider": {
			platform: PlatformOpenAI,
			baseURL:  "https://api.groq.com/openai/v1",
		},
		"unknown provider": {
			platform: PlatformOpenAI,
			provider: "custom",
			baseURL:  "https://api.groq.com/openai/v1",
		},
		"provider endpoint mismatch": {
			platform: PlatformOpenAI,
			provider: "groq",
			baseURL:  "https://openrouter.ai/api/v1",
		},
		"arbitrary endpoint": {
			platform: PlatformOpenAI,
			provider: "groq",
			baseURL:  "https://third-party.example.com/v1",
		},
		"endpoint query": {
			platform: PlatformOpenAI,
			provider: "groq",
			baseURL:  "https://api.groq.com/openai/v1?target=third-party.example.com",
		},
		"endpoint user info": {
			platform: PlatformOpenAI,
			provider: "groq",
			baseURL:  "https://api.groq.com@third-party.example.com/openai/v1",
		},
		"Cloudflare placeholder": {
			platform: PlatformOpenAI,
			provider: "cloudflare_workers_ai",
			baseURL:  "https://api.cloudflare.com/client/v4/accounts/YOUR_ACCOUNT_ID/ai/v1",
		},
		"Cloudflare invalid account ID": {
			platform: PlatformOpenAI,
			provider: "cloudflare_workers_ai",
			baseURL:  "https://api.cloudflare.com/client/v4/accounts/not-an-account-id/ai/v1",
		},
		"Cloudflare extra path": {
			platform: PlatformOpenAI,
			provider: "cloudflare_workers_ai",
			baseURL:  "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/v1/proxy",
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			extra := map[string]any{}
			if test.provider != nil {
				extra["free_model_provider"] = test.provider
			}
			credentials := map[string]any{"api_key": "test-key"}
			if test.baseURL != nil {
				credentials["base_url"] = test.baseURL
			}
			err := validateOwnedFreeModelAPIKey(test.platform, credentials, extra)
			require.ErrorIs(t, err, ErrOwnedAccountAPIKeySourceNotAllowed)
		})
	}
}

func TestAccountServiceCreateOwnedRejectsArbitraryAPIKeyUpstreamBeforeWrite(t *testing.T) {
	t.Parallel()

	repo := &ownedAccountDuplicateRepoStub{}
	svc := &AccountService{accountRepo: repo}

	account, err := svc.CreateOwned(context.Background(), 101, CreateAccountRequest{
		Name:         "untrusted-upstream",
		Platform:     PlatformOpenAI,
		Type:         AccountTypeAPIKey,
		AccountLevel: AccountLevelUnknown,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": "https://third-party.example.com/v1",
		},
		Extra: map[string]any{"free_model_provider": "groq"},
	})

	require.Nil(t, account)
	require.ErrorIs(t, err, ErrOwnedAccountAPIKeySourceNotAllowed)
	require.Empty(t, repo.createdAccounts)
}
