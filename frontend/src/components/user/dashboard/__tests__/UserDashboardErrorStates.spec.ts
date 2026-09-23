import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: { value: 'en-US' } })
  }
})

import UserDashboardCharts from '../UserDashboardCharts.vue'
import UserDashboardPlatformQuotas from '../UserDashboardPlatformQuotas.vue'
import UserDashboardRecentUsage from '../UserDashboardRecentUsage.vue'

const errorStateStub = {
  props: { retry: Boolean },
  template: '<button data-testid="error-state" @click="$emit(\'retry\')">error</button>'
}

describe('user dashboard error states', () => {
  it('renders chart errors instead of an empty chart and exposes retry', async () => {
    const wrapper = mount(UserDashboardCharts, {
      props: {
        loading: false,
        error: true,
        startDate: '2026-09-01',
        endDate: '2026-09-09',
        granularity: 'day',
        trend: [],
        models: []
      },
      global: {
        stubs: {
          DateRangePicker: true,
          Select: true,
          LoadingSpinner: true,
          TokenUsageTrend: true,
          Doughnut: true,
          ErrorState: errorStateStub
        }
      }
    })

    expect(wrapper.findAll('[data-testid="error-state"]')).toHaveLength(2)
    await wrapper.find('[data-testid="error-state"]').trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()
  })

  it('distinguishes quota request failure from a successful empty result', async () => {
    const failed = mount(UserDashboardPlatformQuotas, {
      props: { loading: false, error: true, quotas: null },
      global: { stubs: { LoadingSpinner: true, ErrorState: errorStateStub } }
    })
    expect(failed.find('[data-testid="error-state"]').exists()).toBe(true)

    const empty = mount(UserDashboardPlatformQuotas, {
      props: { loading: false, error: false, quotas: [] },
      global: { stubs: { LoadingSpinner: true, ErrorState: errorStateStub } }
    })
    expect(empty.text()).toContain('dashboard.platformQuota.empty')
    expect(empty.find('[data-testid="error-state"]').exists()).toBe(false)
  })

  it('renders recent usage errors with a retry action', async () => {
    const wrapper = mount(UserDashboardRecentUsage, {
      props: { loading: false, error: true, data: [] },
      global: {
        stubs: {
          LoadingSpinner: true,
          EmptyState: true,
          Icon: true,
          ErrorState: errorStateStub,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })

    await wrapper.find('[data-testid="error-state"]').trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()
  })
})
