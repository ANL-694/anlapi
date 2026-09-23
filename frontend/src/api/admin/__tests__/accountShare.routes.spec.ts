import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: del },
}))

import { accountShareAPI } from '../accountShare'

describe('account sharing revenue API routes', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    del.mockReset()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
    del.mockResolvedValue({ data: {} })
  })

  it('uses the dedicated account-sharing revenue contract', async () => {
    await accountShareAPI.listPolicies({ page: 2 })
    await accountShareAPI.createPolicy({ scope_type: 'global', owner_share_ratio: 0.2 })
    await accountShareAPI.updatePolicy(7, { enabled: false })
    await accountShareAPI.deletePolicy(7)
    await accountShareAPI.listSettlements({ page: 3 })

    expect(get).toHaveBeenNthCalledWith(1, '/admin/account-share-revenue/share-policies', { params: { page: 2 } })
    expect(post).toHaveBeenCalledWith('/admin/account-share-revenue/share-policies', { scope_type: 'global', owner_share_ratio: 0.2 })
    expect(put).toHaveBeenCalledWith('/admin/account-share-revenue/share-policies/7', { enabled: false })
    expect(del).toHaveBeenCalledWith('/admin/account-share-revenue/share-policies/7')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/account-share-revenue/share-settlements', { params: { page: 3 } })
  })
})
