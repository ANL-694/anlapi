import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const {
  getDashboardStats,
  getDashboardTrend,
  getDashboardModels,
  getByDateRange,
  getDashboardAccountSharing,
  getMyPlatformQuotas,
  refreshUser
} = vi.hoisted(() => ({
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  getByDateRange: vi.fn(),
  getDashboardAccountSharing: vi.fn(),
  getMyPlatformQuotas: vi.fn(),
  refreshUser: vi.fn()
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats,
    getDashboardTrend,
    getDashboardModels,
    getByDateRange,
    getDashboardAccountSharing
  }
}))

vi.mock('@/api/user', () => ({ getMyPlatformQuotas }))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { balance: 12.5 },
    isSimpleMode: false,
    refreshUser
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import DashboardView from '../DashboardView.vue'

const errorStateStub = {
  props: { retry: Boolean },
  template: '<button data-testid="dashboard-error" @click="$emit(\'retry\')">error</button>'
}

const viewStub = { template: '<div />' }
const sectionStub = {
  props: { error: Boolean },
  emits: ['retry'],
  template: '<button v-if="error" data-testid="section-error" @click="$emit(\'retry\')">error</button>'
}

const stats = {
  total_api_keys: 1,
  active_api_keys: 1,
  total_requests: 1,
  total_input_tokens: 1,
  total_output_tokens: 1,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 2,
  total_cost: 0.01,
  total_actual_cost: 0.01,
  today_requests: 1,
  today_input_tokens: 1,
  today_output_tokens: 1,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 2,
  today_cost: 0.01,
  today_actual_cost: 0.01,
  average_duration_ms: 10,
  rpm: 1,
  tpm: 2,
  by_platform: []
}

function mountDashboard() {
  return mount(DashboardView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: true,
        ErrorState: errorStateStub,
        UserDashboardHeroActions: viewStub,
        UserDashboardStats: viewStub,
        UserDashboardPlatformUsage: viewStub,
        UserDashboardPlatformQuotas: sectionStub,
        UserDashboardAccountSharing: viewStub,
        UserDashboardCharts: sectionStub,
        UserDashboardRecentUsage: sectionStub
      }
    }
  })
}

describe('user DashboardView', () => {
  beforeEach(() => {
    getDashboardStats.mockReset()
    getDashboardTrend.mockReset()
    getDashboardModels.mockReset()
    getByDateRange.mockReset()
    getDashboardAccountSharing.mockReset()
    getMyPlatformQuotas.mockReset()
    refreshUser.mockReset()

    refreshUser.mockResolvedValue(undefined)
    getDashboardStats.mockResolvedValue(stats)
    getDashboardTrend.mockResolvedValue({ trend: [] })
    getDashboardModels.mockResolvedValue({ models: [] })
    getByDateRange.mockResolvedValue({ items: [] })
    getDashboardAccountSharing.mockResolvedValue(null)
    getMyPlatformQuotas.mockResolvedValue({ platform_quotas: [] })
  })

  it('shows a page error instead of a blank screen when the main stats request fails, then retries', async () => {
    getDashboardStats.mockRejectedValueOnce(new Error('stats unavailable')).mockResolvedValueOnce(stats)

    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.find('[data-testid="dashboard-error"]').exists()).toBe(true)
    await wrapper.find('[data-testid="dashboard-error"]').trigger('click')
    await flushPromises()

    expect(getDashboardStats).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="dashboard-error"]').exists()).toBe(false)
  })

  it('keeps independent section errors visible instead of treating them as empty success', async () => {
    getDashboardTrend.mockRejectedValueOnce(new Error('trend unavailable'))
    getDashboardModels.mockRejectedValueOnce(new Error('models unavailable'))
    getByDateRange.mockRejectedValueOnce(new Error('usage unavailable'))
    getMyPlatformQuotas.mockRejectedValueOnce(new Error('quota unavailable'))

    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.findAll('[data-testid="section-error"]')).toHaveLength(3)
  })
})
