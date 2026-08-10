package openai_compat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOfficialDeepSeekBaseURL(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{baseURL: "https://api.deepseek.com", want: true},
		{baseURL: "https://api.deepseek.com/v1", want: true},
		{baseURL: "https://API.DEEPSEEK.COM/chat/completions", want: true},
		{baseURL: "https://api.deepseek.com.evil.example/v1", want: false},
		{baseURL: "https://deepseek.com", want: false},
		{baseURL: "api.deepseek.com", want: false},
		{baseURL: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			require.Equal(t, tt.want, IsOfficialDeepSeekBaseURL(tt.baseURL))
		})
	}
}

func TestShouldUseResponsesAPIForModel(t *testing.T) {
	tests := []struct {
		name    string
		extra   map[string]any
		baseURL string
		model   string
		want    bool
	}{
		{
			name:    "official flash overrides stale negative probe",
			extra:   map[string]any{ExtraKeyResponsesSupported: false},
			baseURL: "https://api.deepseek.com/v1",
			model:   " deepseek-v4-flash ",
			want:    true,
		},
		{
			name:    "official pro overrides positive account probe",
			extra:   map[string]any{ExtraKeyResponsesSupported: true},
			baseURL: "https://api.deepseek.com",
			model:   "deepseek-v4-pro",
			want:    false,
		},
		{
			name:    "force responses wins for pro",
			extra:   map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses)},
			baseURL: "https://api.deepseek.com",
			model:   "deepseek-v4-pro",
			want:    true,
		},
		{
			name:    "force chat wins for flash",
			extra:   map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions)},
			baseURL: "https://api.deepseek.com",
			model:   "deepseek-v4-flash",
			want:    false,
		},
		{
			name:    "third party host keeps probe result",
			extra:   map[string]any{ExtraKeyResponsesSupported: true},
			baseURL: "https://deepseek-compatible.example/v1",
			model:   "deepseek-v4-pro",
			want:    true,
		},
		{
			name:    "suffix confusion host is not official",
			extra:   map[string]any{ExtraKeyResponsesSupported: false},
			baseURL: "https://api.deepseek.com.evil.example/v1",
			model:   "deepseek-v4-flash",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ShouldUseResponsesAPIForModel(tt.extra, tt.baseURL, tt.model))
		})
	}
}
