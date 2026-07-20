package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"anlapi/internal/pkg/openai"
)

const DefaultOpenAICodexUserAgent = "codex-tui/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.144.1)"

type cachedOpenAICodexUserAgent struct {
	value     string
	expiresAt int64
}

const (
	openAICodexUserAgentCacheTTL  = 60 * time.Second
	openAICodexUserAgentErrorTTL  = 5 * time.Second
	openAICodexUserAgentDBTimeout = 5 * time.Second
)

func (s *SettingService) GetOpenAICodexUserAgent(ctx context.Context) string {
	fallback := DefaultOpenAICodexUserAgent
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, ok := s.openAICodexUACache.Load().(*cachedOpenAICodexUserAgent); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.value
	}

	result, _, _ := s.openAICodexUASF.Do("openai_codex_user_agent", func() (any, error) {
		if cached, ok := s.openAICodexUACache.Load().(*cachedOpenAICodexUserAgent); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached.value, nil
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAICodexUserAgentDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyOpenAICodexUserAgent)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get openai codex user agent setting", "error", err)
			s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
				value:     fallback,
				expiresAt: time.Now().Add(openAICodexUserAgentErrorTTL).UnixNano(),
			})
			return fallback, nil
		}
		userAgent := strings.TrimSpace(value)
		if userAgent == "" {
			userAgent = fallback
		}
		s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
			value:     userAgent,
			expiresAt: time.Now().Add(openAICodexUserAgentCacheTTL).UnixNano(),
		})
		return userAgent, nil
	})
	if userAgent, ok := result.(string); ok && userAgent != "" {
		return userAgent
	}
	return fallback
}

func (s *OpenAIGatewayService) overrideBrowserUserAgent(ctx context.Context, account *Account, req *http.Request) {
	if req == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	if !openai.IsBrowserUserAgent(req.Header.Get("user-agent")) {
		return
	}
	userAgent := DefaultOpenAICodexUserAgent
	if s != nil && s.settingService != nil {
		if configured := strings.TrimSpace(s.settingService.GetOpenAICodexUserAgent(ctx)); configured != "" {
			userAgent = configured
		}
	}
	req.Header.Set("user-agent", userAgent)
}
