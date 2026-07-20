package service

import (
	"net/url"
	"strings"
)

type freeModelProviderPolicy struct {
	baseURL             string
	cloudflareAccountID bool
}

// freeModelProviderPolicies is the server-side authority for user-supplied API
// keys. User accounts may target these official endpoints only; administrators
// retain the existing unrestricted upstream account flow.
var freeModelProviderPolicies = map[string]freeModelProviderPolicy{
	"groq":                  {baseURL: "https://api.groq.com/openai/v1"},
	"cerebras":              {baseURL: "https://api.cerebras.ai/v1"},
	"openrouter":            {baseURL: "https://openrouter.ai/api/v1"},
	"github":                {baseURL: "https://models.github.ai/inference"},
	"gemini_openai":         {baseURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
	"cloudflare_workers_ai": {cloudflareAccountID: true},
	"cohere":                {baseURL: "https://api.cohere.ai/compatibility/v1"},
	"ovh_ai_endpoints":      {baseURL: "https://oai.endpoints.kepler.ai.cloud.ovh.net/v1"},
	"mistral":               {baseURL: "https://api.mistral.ai/v1"},
	"huggingface":           {baseURL: "https://router.huggingface.co/v1"},
	"zhipu":                 {baseURL: "https://api.z.ai/api/paas/v4"},
	"qwen_intl":             {baseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"},
	"siliconflow_global":    {baseURL: "https://api.siliconflow.com/v1"},
	"nvidia_nim":            {baseURL: "https://integrate.api.nvidia.com/v1"},
	"ollama":                {baseURL: "https://ollama.com/v1"},
	"opencode":              {baseURL: "https://opencode.ai/zen/v1"},
}

func validateOwnedFreeModelAPIKey(platform string, credentials, extra map[string]any) error {
	if strings.ToLower(strings.TrimSpace(platform)) != PlatformOpenAI {
		return ownedFreeModelSourceError("platform")
	}
	providerCode, ok := nonEmptyStringValue(extra, "free_model_provider")
	if !ok {
		return ownedFreeModelSourceError("free_model_provider")
	}
	policy, ok := freeModelProviderPolicies[providerCode]
	if !ok {
		return ownedFreeModelSourceError("free_model_provider")
	}
	baseURL, ok := nonEmptyStringValue(credentials, "base_url")
	if !ok || !policy.matchesBaseURL(baseURL) {
		return ownedFreeModelSourceError("base_url")
	}
	return nil
}

func (p freeModelProviderPolicy) matchesBaseURL(raw string) bool {
	parsed, ok := parseStrictHTTPSURL(raw)
	if !ok {
		return false
	}
	if p.cloudflareAccountID {
		return isOfficialCloudflareWorkersAIURL(parsed)
	}
	expected, ok := parseStrictHTTPSURL(p.baseURL)
	if !ok {
		return false
	}
	return strings.EqualFold(parsed.Host, expected.Host) && normalizedURLPath(parsed) == normalizedURLPath(expected)
}

func parseStrictHTTPSURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, false
	}
	if parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return nil, false
	}
	return parsed, true
}

func normalizedURLPath(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		return "/"
	}
	return path
}

func isOfficialCloudflareWorkersAIURL(parsed *url.URL) bool {
	if parsed == nil || !strings.EqualFold(parsed.Host, "api.cloudflare.com") {
		return false
	}
	parts := strings.Split(strings.Trim(normalizedURLPath(parsed), "/"), "/")
	if len(parts) != 6 || parts[0] != "client" || parts[1] != "v4" || parts[2] != "accounts" || parts[4] != "ai" || parts[5] != "v1" {
		return false
	}
	return isCloudflareAccountID(parts[3])
}

func isCloudflareAccountID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func nonEmptyStringValue(values map[string]any, key string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	value, ok := values[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

func ownedFreeModelSourceError(field string) error {
	return ErrOwnedAccountAPIKeySourceNotAllowed.WithMetadata(map[string]string{"field": field})
}
