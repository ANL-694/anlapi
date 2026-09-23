import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post },
}))

import { adminPaymentAPI } from '../payment'

describe('admin payment manual fulfillment API', () => {
  beforeEach(() => post.mockReset())

  it('sends only the administrator-confirmed details to the protected endpoint', async () => {
    const payload = {
      reason: 'bank transfer reconciled',
      paid_amount: 81.13,
      trade_no: 'manual-trade-001',
    }

    await adminPaymentAPI.manualFulfillOrder(42, payload)

    expect(post).toHaveBeenCalledWith('/admin/payment/orders/42/manual-fulfill', payload)
  })
})
