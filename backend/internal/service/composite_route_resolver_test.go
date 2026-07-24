package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type compositeRouteRepositoryFixture struct {
	routes map[int64][]CompositeModelRoute
}

func (r *compositeRouteRepositoryFixture) ListByGroup(_ context.Context, groupID int64, _ bool) ([]CompositeModelRoute, error) {
	return append([]CompositeModelRoute(nil), r.routes[groupID]...), nil
}

func (r *compositeRouteRepositoryFixture) Create(context.Context, *CompositeModelRoute) error {
	return nil
}

func (r *compositeRouteRepositoryFixture) Update(context.Context, *CompositeModelRoute) error {
	return nil
}

func (r *compositeRouteRepositoryFixture) Delete(context.Context, int64) error {
	return nil
}

func (r *compositeRouteRepositoryFixture) DeleteByGroup(context.Context, int64) error {
	return nil
}

func TestCompositeRouteResolverPrefersExactEndpointRoute(t *testing.T) {
	resolver := NewCompositeRouteResolver(&compositeRouteRepositoryFixture{routes: map[int64][]CompositeModelRoute{
		7: {
			{ID: 1, GroupID: 7, PublicModel: "team-", MatchType: CompositeRouteMatchPrefix, TargetPlatform: PlatformAnthropic, UpstreamModel: "claude-sonnet-4-5", Endpoint: CompositeRouteEndpointAny, Priority: 10, Enabled: true},
			{ID: 2, GroupID: 7, PublicModel: "team-gpt", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI, UpstreamModel: "gpt-5", Endpoint: CompositeRouteEndpointAny, Priority: 20, Enabled: true},
			{ID: 3, GroupID: 7, PublicModel: "team-gpt", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformGrok, UpstreamModel: "grok-4", Endpoint: CompositeRouteEndpointResponses, Priority: 100, Enabled: true},
		},
	}})

	decision, err := resolver.Resolve(context.Background(), 7, "team-gpt", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, int64(3), decision.Route.ID)
	require.Equal(t, PlatformGrok, decision.TargetPlatform)
	require.Equal(t, "grok-4", decision.UpstreamModel)

	decision, err = resolver.Resolve(context.Background(), 7, "team-gpt", CompositeRouteEndpointChatCompletions)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, int64(2), decision.Route.ID)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
}

func TestCompositeRouteResolverFallsBackToBuiltInDetector(t *testing.T) {
	resolver := NewCompositeRouteResolver(&compositeRouteRepositoryFixture{})

	decision, err := resolver.Resolve(context.Background(), 9, "gpt-5.4", CompositeRouteEndpointAny)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceDetector, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-5.4", decision.UpstreamModel)

	decision, err = resolver.Resolve(context.Background(), 9, "company-alias", CompositeRouteEndpointAny)
	require.NoError(t, err)
	require.False(t, decision.Matched)
	require.NotEmpty(t, decision.Reason)
}

func TestCompositeRouteContextIsScopedToResolvedGroup(t *testing.T) {
	ctx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched:        true,
		Source:         CompositeRouteSourceExplicit,
		GroupID:        12,
		PublicModel:    "public-model",
		TargetPlatform: PlatformGemini,
		UpstreamModel:  "gemini-2.5-pro",
	})

	platform, ok := ResolvedTargetPlatformForGroup(ctx, 12)
	require.True(t, ok)
	require.Equal(t, PlatformGemini, platform)
	upstream, ok := ResolvedUpstreamModelForGroup(ctx, 12)
	require.True(t, ok)
	require.Equal(t, "gemini-2.5-pro", upstream)

	_, ok = ResolvedTargetPlatformForGroup(ctx, 13)
	require.False(t, ok)
	_, ok = ResolvedUpstreamModelForGroup(ctx, 13)
	require.False(t, ok)
}

func TestCompositeGroupCompatibilityPolicies(t *testing.T) {
	require.True(t, canCopyAccountsFromGroupPlatform(PlatformComposite, PlatformOpenAI))
	require.True(t, canCopyAccountsFromGroupPlatform(PlatformComposite, PlatformComposite))
	require.False(t, canCopyAccountsFromGroupPlatform(PlatformOpenAI, PlatformComposite))
	require.True(t, groupSupportsOAuthOnlyFilter(PlatformComposite))
	require.False(t, isConcreteRequestPlatform(PlatformComposite))
}
