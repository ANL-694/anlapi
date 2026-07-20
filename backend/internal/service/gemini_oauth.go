package service

import (
	"context"

	"anlapi/internal/pkg/geminicli"
)

// GeminiOAuthClient performs Google OAuth token exchange/refresh for Gemini integration.
type GeminiOAuthClient interface {
	ExchangeCode(ctx context.Context, oauthType, code, codeVerifier, redirectURI, proxyURL string) (*geminicli.TokenResponse, error)
	RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*geminicli.TokenResponse, error)
}
