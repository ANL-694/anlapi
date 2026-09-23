import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get, put }
}))

import { getGroupRateSchedules, replaceGroupRateSchedules } from '../groups'

describe('group rate schedule API', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
    get.mockResolvedValue({ data: [] })
    put.mockResolvedValue({ data: [] })
  })

  it('uses the admin group schedule routes', async () => {
    await getGroupRateSchedules(7)
    await replaceGroupRateSchedules(7, [{ start_minute: 60, end_minute: 120, rate_multiplier: 1.5, enabled: true }])

    expect(get).toHaveBeenCalledWith('/admin/groups/7/rate-schedules')
    expect(put).toHaveBeenCalledWith('/admin/groups/7/rate-schedules', {
      entries: [{ start_minute: 60, end_minute: 120, rate_multiplier: 1.5, enabled: true }]
    })
  })
})
