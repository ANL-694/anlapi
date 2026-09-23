import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: { value: 'en-US' } })
  }
})

import UserDashboardPlatformUsage from '../UserDashboardPlatformUsage.vue'
import UserDashboardPlatformQuotas from '../UserDashboardPlatformQuotas.vue'
import UserDashboardAccountSharing from '../UserDashboardAccountSharing.vue'
import type { UserDashboardStats } from '@/api/usage'
import type { PlatformQuotaItem } from '@/types'

const stats = (byPlatform: UserDashboardStats['by_platform']): UserDashboardStats => ({
  total_api_keys: 0,
  active_api_keys: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  average_duration_ms: 0,
  rpm: 0,
  tpm: 0,
  by_platform: byPlatform
})

const quota = (overrides: Partial<PlatformQuotaItem> & Pick<PlatformQuotaItem, 'platform'>): PlatformQuotaItem => ({
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  daily_usage_usd: 0,
  weekly_usage_usd: 0,
  monthly_usage_usd: 0,
  ...overrides
})

describe('UserDashboard platform sections', () => {
  it('orders platform usage by today actual cost and renders an empty state', () => {
    const populated = mount(UserDashboardPlatformUsage, {
      props: { stats: stats([
        { platform: 'openai', total_requests: 9, total_tokens: 900, total_actual_cost: 1, today_requests: 3, today_tokens: 300, today_actual_cost: 0.2 },
        { platform: 'anthropic', total_requests: 8, total_tokens: 800, total_actual_cost: 2, today_requests: 4, today_tokens: 400, today_actual_cost: 0.5 }
      ]) }
    })
    expect(populated.text().indexOf('Claude')).toBeLessThan(populated.text().indexOf('OpenAI'))

    const empty = mount(UserDashboardPlatformUsage, { props: { stats: stats([]) } })
    expect(empty.text()).toContain('dashboard.platformBreakdownEmpty')
  })

  it('prefers detailed today platform usage and renders token dimensions', () => {
    const wrapper = mount(UserDashboardPlatformUsage, {
      props: {
        stats: {
          ...stats([
            { platform: 'openai', total_requests: 99, total_tokens: 999, total_actual_cost: 9, today_requests: 99, today_tokens: 999, today_actual_cost: 9 }
          ]),
          today_platforms: [
            {
              platform: 'anthropic',
              requests: 2,
              input_tokens: 100,
              output_tokens: 50,
              cache_creation_tokens: 25,
              cache_read_tokens: 10,
              total_tokens: 185,
              cost: 0.12,
              actual_cost: 0.15
            }
          ]
        }
      }
    })
    expect(wrapper.text()).toContain('Claude')
    expect(wrapper.text()).not.toContain('OpenAI')
    expect(wrapper.text()).toContain('dashboard.cacheCreation')
    expect(wrapper.text()).toContain('dashboard.cacheRead')
  })

  it('only shows configured quotas and marks a zero limit as disabled', () => {
    const wrapper = mount(UserDashboardPlatformQuotas, {
      props: {
        loading: false,
        quotas: [
          quota({ platform: 'openai', daily_limit_usd: 0 }),
          quota({ platform: 'anthropic', monthly_limit_usd: 5, monthly_usage_usd: 1 }),
          quota({ platform: 'gemini' })
        ]
      },
      global: { stubs: { LoadingSpinner: true } }
    })
    expect(wrapper.text()).toContain('dashboard.platformQuota.disabled')
    expect(wrapper.text()).toContain('Claude')
    expect(wrapper.text()).not.toContain('Gemini')
  })

  it('renders account sharing summary and hides private-only detail empty state', () => {
    const wrapper = mount(UserDashboardAccountSharing, {
      props: {
        loading: false,
        data: {
          summary: {
            owned_accounts: 2,
            private_accounts: 1,
            public_pending_accounts: 0,
            public_approved_accounts: 1,
            public_suspended_accounts: 0,
            self_requests: 4,
            self_tokens: 400,
            self_actual_cost: 0.8,
            self_account_cost: 0.7,
            external_requests: 3,
            external_consumer_charge: 1.2,
            external_account_cost: 1,
            external_owner_credit: 0.8,
            external_platform_fee: 0.2,
            total_account_cost: 1.7,
            balance_net_change: 0
          },
          accounts: [],
          accounts_pagination: { total: 0, page: 1, page_size: 20, pages: 0 },
          trend: [{
            date: '2026-08-10',
            self_requests: 2,
            self_tokens: 100,
            self_actual_cost: 0.4,
            self_account_cost: 0.35,
            external_requests: 3,
            external_consumer_charge: 1.2,
            external_account_cost: 1,
            external_owner_credit: 0.8,
            external_platform_fee: 0.2
          }],
          start_date: '2026-08-10',
          end_date: '2026-08-17',
          granularity: 'day'
        }
      },
      global: { stubs: { LoadingSpinner: true } }
    })
    expect(wrapper.text()).toContain('dashboard.accountSharing.title')
    expect(wrapper.text()).toContain('dashboard.accountSharing.externalConsumerCharge')
    expect(wrapper.text()).toContain('dashboard.accountSharing.trend')
    expect(wrapper.text()).toContain('2026-08-10')
    expect(wrapper.text()).toContain('dashboard.accountSharing.empty')
  })
})
