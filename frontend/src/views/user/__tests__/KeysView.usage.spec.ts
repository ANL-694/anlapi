import { flushPromises, mount } from '@vue/test-utils'
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
    data: {
      type: Array,
      default: () => []
    }
  },
  template: '<div><slot v-if="data.length" name="cell-actions" :row="data[0]" /></div>'
}

const ApiKeyTestModalStub = {
  name: 'ApiKeyTestModal',
  props: ['show', 'apiKey'],
  template: '<div data-test="api-key-test-modal" />'
}

describe('KeysView model testing entry', () => {
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
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailable.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    getPublicSettings.mockResolvedValue({ hide_ccs_import_button: true })
  })

  it('opens the API key model test modal for the selected key', async () => {
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
          ApiKeyTestModal: ApiKeyTestModalStub,
          EndpointPopover: true,
          EndpointCards: true,
          GroupBadge: true,
          GroupOptionItem: true
        }
      }
    })

    try {
      await flushPromises()

      const testButton = wrapper.get('button[aria-label="keys.testModel"]')
      expect(testButton.element.parentElement?.querySelector('[role="tooltip"]')?.textContent).toBe('keys.testModel')

      await testButton.trigger('click')

      const testModal = wrapper.getComponent(ApiKeyTestModalStub)
      expect(testModal.props('show')).toBe(true)
      expect(testModal.props('apiKey')).toMatchObject({ id: 42, name: 'Usage test key' })
      expect(wrapper.find('button[aria-label="keys.usage"]').exists()).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })
})
