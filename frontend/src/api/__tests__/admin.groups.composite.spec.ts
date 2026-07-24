import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: del }
}))

import {
  createCompositeRoute,
  deleteCompositeRoute,
  listCompositeRoutes,
  previewCompositeRoute,
  updateCompositeRoute,
  type CompositeRouteInput
} from '@/api/admin/groups'

const routeInput: CompositeRouteInput = {
  public_model: 'team-gpt',
  match_type: 'exact',
  target_platform: 'openai',
  upstream_model: 'gpt-5.4',
  endpoint: 'responses',
  priority: 10,
  enabled: true,
  notes: 'primary route'
}

describe('admin composite group routes API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    del.mockReset()
  })

  it('uses the group-scoped CRUD and preview endpoints', async () => {
    get.mockResolvedValueOnce({ data: [{ id: 5 }] })
    post
      .mockResolvedValueOnce({ data: { id: 6 } })
      .mockResolvedValueOnce({ data: { matched: true } })
    put.mockResolvedValueOnce({ data: { id: 6, priority: 20 } })
    del.mockResolvedValueOnce({ data: { message: 'deleted' } })

    await expect(listCompositeRoutes(12)).resolves.toEqual([{ id: 5 }])
    await expect(createCompositeRoute(12, routeInput)).resolves.toEqual({ id: 6 })
    await expect(updateCompositeRoute(12, 6, { ...routeInput, priority: 20 })).resolves.toEqual({ id: 6, priority: 20 })
    await expect(previewCompositeRoute(12, { model: 'team-gpt', endpoint: 'responses' })).resolves.toEqual({ matched: true })
    await expect(deleteCompositeRoute(12, 6)).resolves.toEqual({ message: 'deleted' })

    expect(get).toHaveBeenCalledWith('/admin/groups/12/composite-routes')
    expect(post).toHaveBeenNthCalledWith(1, '/admin/groups/12/composite-routes', routeInput)
    expect(put).toHaveBeenCalledWith('/admin/groups/12/composite-routes/6', { ...routeInput, priority: 20 })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/groups/12/composite-routes/preview', {
      model: 'team-gpt',
      endpoint: 'responses'
    })
    expect(del).toHaveBeenCalledWith('/admin/groups/12/composite-routes/6')
  })
})
