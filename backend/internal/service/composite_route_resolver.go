package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type CompositeRouteResolver struct {
	repo CompositeModelRouteRepository
}

func NewCompositeRouteResolver(repo CompositeModelRouteRepository) *CompositeRouteResolver {
	return &CompositeRouteResolver{repo: repo}
}

func (r *CompositeRouteResolver) Resolve(ctx context.Context, groupID int64, model, endpoint string) (CompositeRouteDecision, error) {
	model = strings.TrimSpace(model)
	endpoint = normalizeCompositeRouteEndpoint(endpoint)
	decision := CompositeRouteDecision{GroupID: groupID, PublicModel: model, Endpoint: endpoint}
	if model == "" {
		decision.Reason = "model is required"
		return decision, nil
	}

	if r != nil && r.repo != nil && groupID > 0 {
		routes, err := r.repo.ListByGroup(ctx, groupID, false)
		if err != nil {
			return decision, fmt.Errorf("list composite routes: %w", err)
		}
		if route, ok := matchCompositeRoute(routes, model, endpoint); ok {
			if !isConcreteRequestPlatform(route.TargetPlatform) {
				return decision, fmt.Errorf("composite route %d has invalid target platform %q", route.ID, route.TargetPlatform)
			}
			upstreamModel := strings.TrimSpace(route.UpstreamModel)
			if upstreamModel == "" {
				upstreamModel = model
			}
			return CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceExplicit,
				GroupID:        groupID,
				PublicModel:    model,
				TargetPlatform: route.TargetPlatform,
				UpstreamModel:  upstreamModel,
				Endpoint:       endpoint,
				Route:          &route,
			}, nil
		}
	}

	if platform, ok := DetectModelPlatform(model); ok {
		return CompositeRouteDecision{
			Matched:        true,
			Source:         CompositeRouteSourceDetector,
			GroupID:        groupID,
			PublicModel:    model,
			TargetPlatform: platform,
			UpstreamModel:  model,
			Endpoint:       endpoint,
		}, nil
	}
	decision.Reason = "no explicit route or built-in detector match"
	return decision, nil
}

func (r *CompositeRouteResolver) ResolveForAPIKey(ctx context.Context, apiKey *APIKey, model, endpoint string) (*APIKey, CompositeRouteDecision, error) {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != PlatformComposite {
		return apiKey, CompositeRouteDecision{}, nil
	}
	decision, err := r.Resolve(ctx, apiKey.Group.ID, model, endpoint)
	if err != nil || !decision.Matched {
		return apiKey, decision, err
	}
	resolvedKey, err := r.FilterGroupRoutes(ctx, apiKey, decision, endpoint)
	if err != nil {
		return nil, decision, err
	}
	return resolvedKey, decision, nil
}

func (r *CompositeRouteResolver) FilterGroupRoutes(ctx context.Context, apiKey *APIKey, decision CompositeRouteDecision, endpoint string) (*APIKey, error) {
	if apiKey == nil || len(apiKey.GroupRoutes) == 0 {
		return apiKey, nil
	}
	filtered := make([]APIKeyGroupRoute, 0, len(apiKey.GroupRoutes))
	for _, route := range apiKey.GroupRoutes {
		if !route.Enabled || route.Group == nil {
			continue
		}
		if route.GroupID == decision.GroupID {
			filtered = append(filtered, route)
			continue
		}
		if route.Group.Platform != PlatformComposite {
			if route.Group.Platform == decision.TargetPlatform {
				filtered = append(filtered, route)
			}
			continue
		}
		other, err := r.Resolve(ctx, route.GroupID, decision.PublicModel, endpoint)
		if err != nil {
			return nil, err
		}
		if other.Matched && other.TargetPlatform == decision.TargetPlatform && other.UpstreamModel == decision.UpstreamModel {
			filtered = append(filtered, route)
		}
	}
	resolved := *apiKey
	if len(filtered) == 0 {
		resolved.GroupRoutes = nil
		return &resolved, nil
	}
	resolved.GroupRoutes = filtered
	return &resolved, nil
}

func matchCompositeRoute(routes []CompositeModelRoute, model, endpoint string) (CompositeModelRoute, bool) {
	type candidate struct {
		route          CompositeModelRoute
		matchStrength  int
		endpointWeight int
		prefixLen      int
	}
	candidates := make([]candidate, 0, len(routes))
	for _, route := range routes {
		route.Endpoint = normalizeCompositeRouteEndpoint(route.Endpoint)
		if route.Endpoint != endpoint && route.Endpoint != CompositeRouteEndpointAny {
			continue
		}
		route.MatchType = normalizeCompositeRouteMatchType(route.MatchType)
		publicModel := strings.TrimSpace(route.PublicModel)
		if publicModel == "" {
			continue
		}

		matchStrength := 0
		switch route.MatchType {
		case CompositeRouteMatchExact:
			if publicModel != model {
				continue
			}
			matchStrength = 2
		case CompositeRouteMatchPrefix:
			if !strings.HasPrefix(model, publicModel) {
				continue
			}
			matchStrength = 1
		default:
			continue
		}
		endpointWeight := 0
		if route.Endpoint == endpoint {
			endpointWeight = 1
		}
		candidates = append(candidates, candidate{
			route:          route,
			matchStrength:  matchStrength,
			endpointWeight: endpointWeight,
			prefixLen:      len(publicModel),
		})
	}
	if len(candidates) == 0 {
		return CompositeModelRoute{}, false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.matchStrength != b.matchStrength {
			return a.matchStrength > b.matchStrength
		}
		if a.endpointWeight != b.endpointWeight {
			return a.endpointWeight > b.endpointWeight
		}
		if a.prefixLen != b.prefixLen {
			return a.prefixLen > b.prefixLen
		}
		if a.route.Priority != b.route.Priority {
			return a.route.Priority < b.route.Priority
		}
		return a.route.ID < b.route.ID
	})
	return candidates[0].route, true
}
