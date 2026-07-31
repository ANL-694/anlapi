import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
  },
}))

import { usageAPI } from '@/api/usage'

describe('usage api', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: {} })
  })

  it('includes the request type in date-range stats queries', async () => {
    await usageAPI.getStatsByDateRange('2026-03-01', '2026-03-08', 42, 'cyber')

    expect(get).toHaveBeenCalledWith('/usage/stats', {
      params: {
        start_date: '2026-03-01',
        end_date: '2026-03-08',
        api_key_id: 42,
        request_type: 'cyber',
      },
    })
  })
})
