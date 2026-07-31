import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import UsageView from '../UsageView.vue'

const { route, query, getStatsByDateRange, list, showError, showWarning, showSuccess, showInfo } = vi.hoisted(() => ({
  route: {
    query: {} as Record<string, string | null | Array<string | null> | undefined>,
  },
  query: vi.fn(),
  getStatsByDateRange: vi.fn(),
  list: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
}))

const messages: Record<string, string> = {
  'usage.costDetails': 'Cost Breakdown',
  'admin.usage.inputCost': 'Input Cost',
  'admin.usage.outputCost': 'Output Cost',
  'admin.usage.cacheCreationCost': 'Cache Creation Cost',
  'admin.usage.cacheReadCost': 'Cache Read Cost',
  'usage.inputTokenPrice': 'Input price',
  'usage.outputTokenPrice': 'Output price',
  'usage.perMillionTokens': '/ 1M tokens',
  'usage.serviceTier': 'Service tier',
  'usage.serviceTierPriority': 'Fast',
  'usage.serviceTierFlex': 'Flex',
  'usage.serviceTierStandard': 'Standard',
  'usage.rate': 'Rate',
  'usage.original': 'Original',
  'usage.billed': 'Billed',
  'usage.allApiKeys': 'All API Keys',
  'usage.apiKeyFilter': 'API Key',
  'usage.requestTypeFilter': 'Request Type',
  'usage.allRequestTypes': 'All Request Types',
  'usage.sync': 'Sync',
  'usage.stream': 'Stream',
  'usage.cyber': 'Cyber',
  'usage.model': 'Model',
  'usage.reasoningEffort': 'Reasoning Effort',
  'usage.type': 'Type',
  'usage.tokens': 'Tokens',
  'usage.cost': 'Cost',
  'usage.firstToken': 'First Token',
  'usage.duration': 'Duration',
  'usage.time': 'Time',
  'usage.userAgent': 'User Agent',
  'usage.cacheCreation5mHint': '5-minute cache creation tokens',
  'usage.cacheCreation1hHint': '1-hour cache creation tokens',
}

vi.mock('@/api', () => ({
  usageAPI: {
    query,
    getStatsByDateRange,
  },
  keysAPI: {
    list,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess, showInfo }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => route,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot /></div>',
}

describe('user UsageView', () => {
  beforeEach(() => {
    route.query = {}
    query.mockReset()
    getStatsByDateRange.mockReset()
    list.mockReset()
    showError.mockReset()
    showWarning.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()

    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })

    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 20,
      left: 20,
      right: 120,
      bottom: 40,
      width: 100,
      height: 20,
      toJSON: () => ({}),
    } as DOMRect)

    ;(globalThis as any).ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
  })

  it('shows fast service tier and unit prices in user tooltip', async () => {
    query.mockResolvedValue({
      items: [
        {
          request_id: 'req-user-1',
          actual_cost: 0.092883,
          total_cost: 0.092883,
          rate_multiplier: 1,
          service_tier: 'priority',
          input_cost: 0.020285,
          output_cost: 0.00303,
          cache_creation_cost: 0,
          cache_read_cost: 0.069568,
          input_tokens: 4057,
          output_tokens: 101,
          cache_creation_tokens: 0,
          cache_read_tokens: 278272,
          cache_creation_5m_tokens: 0,
          cache_creation_1h_tokens: 0,
          image_count: 0,
          image_size: null,
          first_token_ms: null,
          duration_ms: 1,
          created_at: '2026-03-08T00:00:00Z',
        },
      ],
      total: 1,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 1,
      total_tokens: 100,
      total_cost: 0.1,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()
    await nextTick()

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.tooltipData = {
      request_id: 'req-user-1',
      actual_cost: 0.092883,
      total_cost: 0.092883,
      rate_multiplier: 1,
      service_tier: 'priority',
      input_cost: 0.020285,
      output_cost: 0.00303,
      cache_creation_cost: 0,
      cache_read_cost: 0.069568,
      input_tokens: 4057,
      output_tokens: 101,
    }
    setupState.tooltipVisible = true
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('Service tier')
    expect(text).toContain('Fast')
    expect(text).toContain('Rate')
    expect(text).toContain('1.00x')
    expect(text).toContain('Billed')
    expect(text).toContain('$0.092883')
    expect(text).toContain('$5.0000 / 1M tokens')
    expect(text).toContain('$30.0000 / 1M tokens')
  })

  it('labels cache creation TTL tiers without marking rows that have no TTL breakdown', async () => {
    const usageLog = {
      request_id: 'req-cache-ttl',
      model: 'claude-sonnet-4.6',
      actual_cost: 0.01,
      total_cost: 0.01,
      rate_multiplier: 1,
      input_cost: 0.004,
      output_cost: 0.004,
      cache_creation_cost: 0.002,
      cache_read_cost: 0,
      input_tokens: 100,
      output_tokens: 20,
      cache_creation_tokens: 50,
      cache_read_tokens: 0,
      cache_creation_5m_tokens: 0,
      cache_creation_1h_tokens: 0,
      reasoning_tokens: 0,
      image_count: 0,
      image_size: null,
      image_input_tokens: 0,
      image_output_tokens: 0,
      first_token_ms: null,
      duration_ms: 100,
      stream: false,
      billing_type: 0,
      cache_ttl_overridden: false,
      created_at: '2026-03-08T00:00:00Z',
    }

    query.mockResolvedValue({
      items: [
        { ...usageLog, request_id: 'req-cache-5m', cache_creation_5m_tokens: 50 },
        { ...usageLog, request_id: 'req-cache-1h', cache_creation_1h_tokens: 50 },
        { ...usageLog, request_id: 'req-cache-unspecified' },
      ],
      total: 3,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 3,
      total_tokens: 510,
      total_cost: 0.03,
      average_duration_ms: 100,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    const fiveMinuteBadges = wrapper.findAll('[title="5-minute cache creation tokens"]')
    const oneHourBadges = wrapper.findAll('[title="1-hour cache creation tokens"]')

    expect(fiveMinuteBadges).toHaveLength(1)
    expect(fiveMinuteBadges[0].text()).toBe('5m')
    expect(oneHourBadges).toHaveLength(1)
    expect(oneHourBadges[0].text()).toBe('1h')
  })

  it('exports csv with input and output unit price columns', async () => {
    const exportedLogs = [
      {
        request_id: 'req-user-export',
        actual_cost: 0.092883,
        total_cost: 0.092883,
        rate_multiplier: 1,
        service_tier: 'priority',
        input_cost: 0.020285,
        output_cost: 0.00303,
        cache_creation_cost: 0.000001,
        cache_read_cost: 0.069568,
        input_tokens: 4057,
        output_tokens: 101,
        cache_creation_tokens: 4,
        cache_read_tokens: 278272,
        cache_creation_5m_tokens: 0,
        cache_creation_1h_tokens: 0,
        image_count: 0,
        image_size: null,
        first_token_ms: 12,
        duration_ms: 345,
        created_at: '2026-03-08T00:00:00Z',
        model: 'gpt-5.4',
        reasoning_effort: null,
        api_key: { name: 'demo-key' },
      },
    ]

    query.mockResolvedValue({
      items: exportedLogs,
      total: 1,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 1,
      total_tokens: 100,
      total_cost: 0.1,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    let exportedBlob: Blob | null = null
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn((blob: Blob | MediaSource) => {
      exportedBlob = blob as Blob
      return 'blob:usage-export'
    }) as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState
    await setupState.exportToCSV()

    expect(exportedBlob).not.toBeNull()
    const hasSortedExportQuery = query.mock.calls.some((call) => {
      const params = call[0] as Record<string, unknown> | undefined
      const config = call[1]
      return (
        params?.page_size === 100 &&
        params?.sort_by === 'created_at' &&
        params?.sort_order === 'desc' &&
        config === undefined
      )
    })
    expect(hasSortedExportQuery).toBe(true)
    expect(clickSpy).toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalled()

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    clickSpy.mockRestore()
  })

  it('applies a valid API key ID from the URL query after loading the current user keys', async () => {
    route.query = { api_key_id: '42' }
    query.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 0,
      total_tokens: 0,
      total_cost: 0,
      average_duration_ms: 0,
    })
    list.mockResolvedValue({ items: [{ id: 42 }] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    expect(list).toHaveBeenCalledWith(1, 100)
    expect(query).toHaveBeenCalledTimes(1)
    expect(query.mock.calls[0][0]).toMatchObject({ api_key_id: 42 })
    expect(getStatsByDateRange).toHaveBeenCalledWith(expect.any(String), expect.any(String), 42, undefined)
    expect((wrapper.vm as any).$?.setupState.filters.api_key_id).toBe(42)
  })

  it('applies the request type filter to usage rows and summary stats', async () => {
    query.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 0,
      total_tokens: 0,
      total_cost: 0,
      average_duration_ms: 0,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.filters.request_type = 'cyber'
    setupState.applyFilters()
    await flushPromises()

    expect(query.mock.calls.at(-1)?.[0]).toMatchObject({ request_type: 'cyber' })
    expect(getStatsByDateRange).toHaveBeenLastCalledWith(
      expect.any(String),
      expect.any(String),
      undefined,
      'cyber'
    )
  })

  it.each([
    ['invalid', 'not-a-key-id'],
    ['unknown', '999'],
  ])('ignores an %s URL API key ID without reporting an error', async (_kind, apiKeyId) => {
    route.query = { api_key_id: apiKeyId }
    query.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 0,
      total_tokens: 0,
      total_cost: 0,
      average_duration_ms: 0,
    })
    list.mockResolvedValue({ items: [{ id: 42 }] })

    mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    expect(query).toHaveBeenCalledTimes(1)
    expect(query.mock.calls[0][0]).toMatchObject({ api_key_id: undefined })
    expect(getStatsByDateRange).toHaveBeenCalledWith(expect.any(String), expect.any(String), undefined, undefined)
    expect(showError).not.toHaveBeenCalled()
  })
})
