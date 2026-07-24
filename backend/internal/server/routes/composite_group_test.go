package routes

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"testing"

	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compositeRoutesRepositoryFixture struct {
	routes map[int64][]service.CompositeModelRoute
}

func (r *compositeRoutesRepositoryFixture) ListByGroup(_ context.Context, groupID int64, _ bool) ([]service.CompositeModelRoute, error) {
	return append([]service.CompositeModelRoute(nil), r.routes[groupID]...), nil
}

func (r *compositeRoutesRepositoryFixture) Create(context.Context, *service.CompositeModelRoute) error {
	return nil
}

func (r *compositeRoutesRepositoryFixture) Update(context.Context, *service.CompositeModelRoute) error {
	return nil
}

func (r *compositeRoutesRepositoryFixture) Delete(context.Context, int64) error {
	return nil
}

func (r *compositeRoutesRepositoryFixture) DeleteByGroup(context.Context, int64) error {
	return nil
}

func TestFilterCompositeGroupRoutesKeepsOnlyCompatibleCandidates(t *testing.T) {
	primary := &service.Group{ID: 1, Platform: service.PlatformComposite}
	openAI := &service.Group{ID: 2, Platform: service.PlatformOpenAI}
	anthropic := &service.Group{ID: 3, Platform: service.PlatformAnthropic}
	matchingComposite := &service.Group{ID: 4, Platform: service.PlatformComposite}
	differentComposite := &service.Group{ID: 5, Platform: service.PlatformComposite}
	apiKey := &service.APIKey{
		Group: primary,
		GroupRoutes: []service.APIKeyGroupRoute{
			{GroupID: 1, Enabled: true, Group: primary},
			{GroupID: 2, Enabled: true, Group: openAI},
			{GroupID: 3, Enabled: true, Group: anthropic},
			{GroupID: 4, Enabled: true, Group: matchingComposite},
			{GroupID: 5, Enabled: true, Group: differentComposite},
		},
	}
	resolver := service.NewCompositeRouteResolver(&compositeRoutesRepositoryFixture{routes: map[int64][]service.CompositeModelRoute{
		4: {{ID: 40, GroupID: 4, PublicModel: "alias", MatchType: service.CompositeRouteMatchExact, TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-5", Endpoint: service.CompositeRouteEndpointAny, Enabled: true}},
		5: {{ID: 50, GroupID: 5, PublicModel: "alias", MatchType: service.CompositeRouteMatchExact, TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-4.1", Endpoint: service.CompositeRouteEndpointAny, Enabled: true}},
	}})
	decision := service.CompositeRouteDecision{
		Matched: true, GroupID: 1, PublicModel: "alias", TargetPlatform: service.PlatformOpenAI,
		UpstreamModel: "gpt-5", Endpoint: service.CompositeRouteEndpointResponses,
	}

	resolved, err := filterCompositeGroupRoutes(context.Background(), apiKey, decision, resolver, decision.Endpoint)
	require.NoError(t, err)
	require.NotSame(t, apiKey, resolved)
	require.Equal(t, []int64{1, 2, 4}, compositeRouteGroupIDs(resolved.GroupRoutes))
	require.Len(t, apiKey.GroupRoutes, 5)
}

func TestFilterCompositeGroupRoutesClearsIncompatibleFallbacks(t *testing.T) {
	primary := &service.Group{ID: 1, Platform: service.PlatformComposite}
	apiKey := &service.APIKey{
		Group: primary,
		GroupRoutes: []service.APIKeyGroupRoute{
			{GroupID: 2, Enabled: true, Group: &service.Group{ID: 2, Platform: service.PlatformAnthropic}},
		},
	}
	decision := service.CompositeRouteDecision{
		Matched: true, GroupID: 1, PublicModel: "alias", TargetPlatform: service.PlatformOpenAI,
		UpstreamModel: "gpt-5", Endpoint: service.CompositeRouteEndpointResponses,
	}

	resolved, err := filterCompositeGroupRoutes(context.Background(), apiKey, decision, service.NewCompositeRouteResolver(nil), decision.Endpoint)
	require.NoError(t, err)
	require.Empty(t, resolved.GroupRoutes)
	require.Len(t, apiKey.GroupRoutes, 1)
}

func TestRewriteCompositeGeminiModelParams(t *testing.T) {
	c := &gin.Context{Params: gin.Params{
		{Key: "model", Value: "public-gemini"},
		{Key: "modelAction", Value: "/public-gemini:generateContent"},
	}}

	rewriteCompositeGeminiModelParams(c, "public-gemini", "gemini-2.5-pro")
	require.Equal(t, "gemini-2.5-pro", c.Param("model"))
	require.Equal(t, "/gemini-2.5-pro:generateContent", c.Param("modelAction"))
	require.Equal(t, "gemini-2.5-pro/streamGenerateContent", rewriteCompositeGeminiModelAction("public-gemini/streamGenerateContent", "public-gemini", "gemini-2.5-pro"))
}

func TestRewriteCompositeRequestModelPreservesMultipartFiles(t *testing.T) {
	var input bytes.Buffer
	writer := multipart.NewWriter(&input)
	require.NoError(t, writer.SetBoundary("anl-composite-boundary"))
	require.NoError(t, writer.WriteField("model", "public-image"))
	file, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02}
	_, err = file.Write(imageBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	rewritten, err := rewriteCompositeRequestModel(writer.FormDataContentType(), input.Bytes(), "gpt-image-1")
	require.NoError(t, err)

	reader := multipart.NewReader(bytes.NewReader(rewritten), writer.Boundary())
	parts := make(map[string][]byte)
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		parts[part.FormName()] = data
	}
	require.Equal(t, []byte("gpt-image-1"), parts["model"])
	require.Equal(t, imageBytes, parts["image"])
}

func compositeRouteGroupIDs(routes []service.APIKeyGroupRoute) []int64 {
	ids := make([]int64, 0, len(routes))
	for _, route := range routes {
		ids = append(ids, route.GroupID)
	}
	return ids
}
