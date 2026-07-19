package service

import (
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"ikik-api/internal/config"
)

func newOpenAIImageGenerationControlTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	cfg := &config.Config{}
	return &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
}

func newOpenAIImageGenerationControlTestContext(allowImages bool, userAgent string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", userAgent)
	groupID := int64(4242)
	c.Set("api_key", &APIKey{
		ID:      2424,
		GroupID: &groupID,
		Group: &Group{
			ID:                   groupID,
			AllowImageGeneration: allowImages,
			RateMultiplier:       1,
			ImageRateMultiplier:  1,
		},
	})
	return c, recorder
}

func newOpenAIImageGenerationControlTestAccount() *Account {
	return &Account{
		ID:          5151,
		Name:        "openai-image-controls",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}
}
