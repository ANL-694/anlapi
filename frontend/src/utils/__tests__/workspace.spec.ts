import { describe, expect, it } from 'vitest'
import { resolveAppWorkspace } from '@/utils/workspace'

describe('resolveAppWorkspace', () => {
  it('treats public and user routes as the user workspace regardless of admin role', () => {
    expect(resolveAppWorkspace({ meta: { requiresAdmin: false } })).toBe('user')
  })

  it('uses route authorization metadata for the admin workspace', () => {
    expect(resolveAppWorkspace({ meta: { requiresAdmin: true } })).toBe('admin')
  })

  it('keeps simple admin mode in the route workspace on shared user routes', () => {
    expect(resolveAppWorkspace({ meta: { requiresAdmin: false } }, true, true)).toBe('user')
  })

  it('keeps a regular user in the user workspace in simple mode', () => {
    expect(resolveAppWorkspace({ meta: { requiresAdmin: false } }, true, false)).toBe('user')
  })

  it('defaults transitional routes without meta to the user workspace', () => {
    expect(resolveAppWorkspace({})).toBe('user')
  })
})
