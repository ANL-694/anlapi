import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountStatsModal from '../AccountStatsModal.vue'

const getStats = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getStats } },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const account = {
  id: 42,
  name: 'owned account',
  platform: 'openai',
  type: 'oauth',
} as any

describe('AccountStatsModal', () => {
  it('uses the injected user stats loader instead of the admin API', async () => {
    const userStatsLoader = vi.fn().mockResolvedValue({
      history: [],
      models: [],
      endpoints: [],
      upstream_endpoints: [],
      summary: {
        total_cost: 0,
        total_user_cost: 0,
        total_standard_cost: 0,
        total_requests: 0,
        avg_daily_cost: 0,
        avg_daily_user_cost: 0,
        avg_daily_requests: 0,
        total_tokens: 0,
        avg_daily_tokens: 0,
        avg_duration_ms: 0,
        actual_days_used: 0,
        days: 30,
      },
    })
    const wrapper = mount(AccountStatsModal, {
      props: {
        show: false,
        account,
        statsLoader: userStatsLoader,
      },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
          LoadingSpinner: true,
          ModelDistributionChart: true,
          EndpointDistributionChart: true,
          Icon: true,
          Line: true,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(userStatsLoader).toHaveBeenCalledWith(42, 30)
    expect(getStats).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
