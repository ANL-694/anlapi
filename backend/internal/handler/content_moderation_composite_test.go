package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "anlapi/internal/server/middleware"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationInputUsesCompositeTargetPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(12)
	decision := service.CompositeRouteDecision{
		Matched: true, GroupID: groupID, PublicModel: "public-model",
		TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-5.4",
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), decision))

	input := buildContentModerationInput(c, &service.APIKey{
		ID: 1, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: service.PlatformComposite},
	}, middleware2.AuthSubject{UserID: 2}, service.ContentModerationProtocolOpenAIResponses, "gpt-5.4", []byte(`{"input":"hello"}`))

	require.Equal(t, service.PlatformOpenAI, input.Provider)
}
