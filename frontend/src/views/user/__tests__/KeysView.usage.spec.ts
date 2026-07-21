import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import KeysView from '../KeysView.vue'

const {
  routerPush,
  list,
  getDashboardApiKeysUsage,
  getAvailable,
  getUserGroupRates,
  getPublicSettings
} = vi.hoisted(() => ({
  routerPush: vi.fn(),
  list: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailable: vi.fn(),
  getUserGroupRates: vi.fn(),
  getPublicSettings: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush })
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
    data: {
      type: Array,
      default: () => []
    }
  },
  template: '<div><slot v-if="data.length" name="cell-actions" :row="data[0]" /></div>'
}

describe('KeysView usage navigation', () => {
  const apiKey = {
    id: 42,
    key: 'sk-plaintext-must-not-be-routed',
    name: 'Usage test key',
    status: 'active',
    is_system_managed: false
  }

  beforeEach(() => {
    routerPush.mockReset()
    list.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailable.mockReset()
    getUserGroupRates.mockReset()
    getPublicSettings.mockReset()

    list.mockResolvedValue({ items: [apiKey], total: 1, pages: 1 })
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailable.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    getPublicSettings.mockResolvedValue({ hide_ccs_import_button: true })
  })

  it('opens usage with only the API key database id', async () => {
    const wrapper = mount(KeysView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          UiPage: { template: '<div><slot /></div>' },
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

      const usageButton = wrapper.get('button[aria-label="keys.usage"]')
      expect(usageButton.element.parentElement?.querySelector('[role="tooltip"]')?.textContent).toBe('keys.usage')

      await usageButton.trigger('click')

      expect(routerPush).toHaveBeenCalledWith({
        path: '/usage',
        query: { api_key_id: '42' }
      })
      expect(JSON.stringify(routerPush.mock.calls)).not.toContain(apiKey.key)
    } finally {
      wrapper.unmount()
    }
  })
})
