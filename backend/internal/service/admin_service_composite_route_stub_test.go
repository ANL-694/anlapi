//go:build unit

package service

import "context"

type compositeRouteRepoStubForAdmin struct {
	routes  []CompositeModelRoute
	nextID  int64
	created *CompositeModelRoute
	updated *CompositeModelRoute
	deleted []int64
}

func (r *compositeRouteRepoStubForAdmin) ListByGroup(_ context.Context, groupID int64, _ bool) ([]CompositeModelRoute, error) {
	out := make([]CompositeModelRoute, 0, len(r.routes))
	for _, route := range r.routes {
		if route.GroupID == groupID {
			out = append(out, route)
		}
	}
	return out, nil
}

func (r *compositeRouteRepoStubForAdmin) Create(_ context.Context, route *CompositeModelRoute) error {
	if r.nextID > 0 {
		route.ID = r.nextID
	}
	r.created = cloneCompositeRouteForAdminTest(route)
	r.routes = append(r.routes, *route)
	return nil
}

func (r *compositeRouteRepoStubForAdmin) Update(_ context.Context, route *CompositeModelRoute) error {
	r.updated = cloneCompositeRouteForAdminTest(route)
	for i := range r.routes {
		if r.routes[i].ID == route.ID {
			r.routes[i] = *route
			break
		}
	}
	return nil
}

func (r *compositeRouteRepoStubForAdmin) Delete(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (*compositeRouteRepoStubForAdmin) DeleteByGroup(context.Context, int64) error { return nil }

func cloneCompositeRouteForAdminTest(route *CompositeModelRoute) *CompositeModelRoute {
	if route == nil {
		return nil
	}
	clone := *route
	return &clone
}
