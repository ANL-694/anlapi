import { describe, expect, it } from 'vitest'

import router from '@/router'
import { resolveAppWorkspace } from '@/utils/workspace'

describe('workspace route boundaries', () => {
  it('marks every concrete admin route as authenticated and admin-only', () => {
    const adminRoutes = router
      .getRoutes()
      .filter((route) => route.path.startsWith('/admin/') && route.components)

    expect(adminRoutes.length).toBeGreaterThan(10)
    expect(adminRoutes.every((route) => route.meta.requiresAuth === true)).toBe(true)
    expect(adminRoutes.every((route) => route.meta.requiresAdmin === true)).toBe(true)
  })

  it('keeps user workspace routes non-admin while still requiring authentication', () => {
    const userPaths = ['/dashboard', '/keys', '/accounts', '/usage']

    for (const path of userPaths) {
      const route = router.getRoutes().find((candidate) => candidate.path === path)
      expect(route, `missing user route ${path}`).toBeDefined()
      expect(route?.meta.requiresAuth).toBe(true)
      expect(route?.meta.requiresAdmin).toBe(false)
    }
  })

  it('resolves workspace from the route contract, not from the viewer role alone', () => {
    expect(resolveAppWorkspace({ meta: { requiresAdmin: false } }, false, false)).toBe('user')
    expect(resolveAppWorkspace({ meta: { requiresAdmin: false } }, false, true)).toBe('user')
    expect(resolveAppWorkspace({ meta: { requiresAdmin: false } }, true, true)).toBe('user')
    expect(resolveAppWorkspace({ meta: { requiresAdmin: true } }, false, true)).toBe('admin')
    expect(resolveAppWorkspace({ meta: { requiresAdmin: true } }, true, true)).toBe('admin')
  })
})
