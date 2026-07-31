import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import KeysView from '../KeysView.vue'

const {
  list,
  getDashboardApiKeysUsage,
  getAvailable,
  getUserGroupRates,
  getPublicSettings
} = vi.hoisted(() => ({
  list: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailable: vi.fn(),
  getUserGroupRates: vi.fn(),
  getPublicSettings: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/api', () => ({
  keysAPI: { list },
  authAPI: { getPublicSettings },
  usageAPI: { getDashboardApiKeysUsage },
  userGroupsAPI: { getAvailable, getUserGroupRates }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('@/composables/usePersistedPageSize', () => ({
  getPersistedPageSize: () => 20
}))

const DataTableStub = {
  props: {
    columns: {
      type: Array,
      default: () => []
    },
    data: {
      type: Array,
      default: () => []
    },
    cardRows: {
      type: Boolean,
      default: false
    }
  },
  template: '<div data-test="keys-table" :data-card-rows="String(cardRows)" :data-column-keys="columns.map((column) => column.key).join(\',\')"><slot v-if="data.length" name="cell-actions" :row="data[0]" /></div>'
}

describe('KeysView list layout', () => {
  const apiKey = {
    id: 42,
    key: 'sk-plaintext-must-not-be-routed',
    name: 'Usage test key',
    status: 'active',
    is_system_managed: false
  }

  beforeEach(() => {
    list.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailable.mockReset()
    getUserGroupRates.mockReset()
    getPublicSettings.mockReset()

    list.mockResolvedValue({ items: [apiKey], total: 1, pages: 1 })
    getDashboardApiKeysUsage.mockResolvedValue({
      stats: { 42: { today_actual_cost: 0, total_actual_cost: 0 } }
    })
    getAvailable.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    getPublicSettings.mockResolvedValue({ hide_ccs_import_button: true })
  })

  it('keeps usage in a full-width card table but out of row actions', async () => {
    const wrapper = mount(KeysView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          UiPage: {
            props: ['width'],
            template: '<div data-test="keys-page" :data-width="width"><slot /></div>'
          },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="actions" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          ConfirmDialog: true,
          EmptyState: true,
          Select: true,
          SearchInput: true,
          Icon: true,
          UseKeyModal: true,
          EndpointPopover: true,
          EndpointCards: true,
          GroupBadge: true,
          GroupOptionItem: true
        }
      }
    })

    try {
      await flushPromises()
      await nextTick()

      const keysTable = wrapper.get('[data-test="keys-table"]')
      expect(wrapper.get('[data-test="keys-page"]').attributes('data-width')).toBe('full')
      expect(keysTable.attributes('data-card-rows')).toBe('true')
      expect(keysTable.attributes('data-column-keys')).toContain('usage')
      expect(getDashboardApiKeysUsage).toHaveBeenCalledWith([42], expect.objectContaining({ signal: expect.any(AbortSignal) }))

      expect(wrapper.find('button[aria-label="keys.usage"]').exists()).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })
})
