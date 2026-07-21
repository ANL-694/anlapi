import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountsView from '../AccountsView.vue'

const { listAccounts, listProxies, getBatchTodayStats, getAvailableGroups } = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listProxies: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAvailableGroups: vi.fn()
}))

vi.mock('@/api', () => ({
  accountsAPI: {
    list: listAccounts,
    listProxies,
    getBatchTodayStats,
    getUsage: vi.fn(),
    getStats: vi.fn()
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: '<div data-test="data-table" :data-columns="columns.map(column => column.key).join(\',\')"></div>'
}

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<div data-test="select"></div>'
}

function mountAccountsView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        UiPage: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        DataTable: DataTableStub,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: true,
        UiIconButton: true,
        UiMenu: true,
        Icon: true,
        PlatformTypeBadge: true,
        AccountStatusIndicator: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        AccountTodayStatsCell: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        AccountStatsModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        UserAccountActionMenu: true,
        ImportAccountsModal: true,
        UserProxyPoolModal: true,
        Teleport: true
      }
    }
  })
}

describe('user AccountsView', () => {
  beforeEach(() => {
    listAccounts.mockReset().mockResolvedValue({
      items: [{ id: 1 }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listProxies.mockReset().mockResolvedValue([])
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getAvailableGroups.mockReset().mockResolvedValue([])
  })

  it('does not expose the legacy account capacity column', async () => {
    const wrapper = mountAccountsView()

    await flushPromises()

    expect(wrapper.get('[data-test="data-table"]').attributes('data-columns')?.split(',')).not.toContain('capacity')
  })

  it('filters OAuth and API Key accounts independently', async () => {
    const wrapper = mountAccountsView()
    await flushPromises()

    const selects = wrapper.findAllComponents(SelectStub)
    expect(selects).toHaveLength(4)

    selects[1].vm.$emit('update:modelValue', 'oauth')
    await flushPromises()
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({ type: 'oauth' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )

    selects[1].vm.$emit('update:modelValue', 'apikey')
    await flushPromises()
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({ type: 'apikey' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })
})
