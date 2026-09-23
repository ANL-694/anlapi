import type { RouteLocationNormalizedLoaded } from 'vue-router'

export type AppWorkspace = 'user' | 'admin'

type WorkspaceRoute = Partial<Pick<RouteLocationNormalizedLoaded, 'meta'>>

export function resolveAppWorkspace(
  route: WorkspaceRoute,
  _simpleAdminMode = false,
  _isAdmin = false,
): AppWorkspace {
  return route.meta?.requiresAdmin === true ? 'admin' : 'user'
}
