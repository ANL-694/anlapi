import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import adminComplianceAPI, { type AdminComplianceStatus } from '@/api/admin/compliance'
import { useAdminComplianceStore } from '@/stores/adminCompliance'

vi.mock('@/api/admin/compliance', () => ({
  default: {
    getStatus: vi.fn(),
    accept: vi.fn(),
  },
}))

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh',
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createStatus(required: boolean): AdminComplianceStatus {
  return {
    required,
    version: 'v2026.06.10',
    document_path_zh: 'docs/legal/admin-compliance.zh.md',
    document_path_en: 'docs/legal/admin-compliance.en.md',
    document_url_zh: 'https://example.test/admin-compliance.zh.md',
    document_url_en: 'https://example.test/admin-compliance.en.md',
    ack_phrase_zh: '中文确认短语',
    ack_phrase_en: 'English acknowledgement phrase',
  }
}

describe('useAdminComplianceStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(adminComplianceAPI.getStatus).mockReset()
    vi.mocked(adminComplianceAPI.accept).mockReset()
  })

  it('加载状态后按服务端结果显示强制确认', async () => {
    vi.mocked(adminComplianceAPI.getStatus).mockResolvedValue(createStatus(true))
    const store = useAdminComplianceStore()

    await store.fetchStatus()

    expect(store.initialized).toBe(true)
    expect(store.shouldShow).toBe(true)
    expect(store.expectedPhrase).toBe('中文确认短语')
  })

  it('reset 后忽略旧会话迟到的状态响应', async () => {
    const deferred = createDeferred<AdminComplianceStatus>()
    vi.mocked(adminComplianceAPI.getStatus).mockReturnValue(deferred.promise)
    const store = useAdminComplianceStore()

    const pending = store.fetchStatus()
    expect(store.loading).toBe(true)

    store.reset()
    deferred.resolve(createStatus(true))
    await pending

    expect(store.status).toBeNull()
    expect(store.initialized).toBe(false)
    expect(store.shouldShow).toBe(false)
    expect(store.loading).toBe(false)
  })

  it('423 事件会使旧请求失效并保留事件中的版本信息', async () => {
    const deferred = createDeferred<AdminComplianceStatus>()
    vi.mocked(adminComplianceAPI.getStatus).mockReturnValue(deferred.promise)
    const store = useAdminComplianceStore()

    const pending = store.fetchStatus()
    store.requireAcknowledgement({ version: 'v-next' })
    deferred.resolve(createStatus(false))
    await pending

    expect(store.status?.version).toBe('v-next')
    expect(store.shouldShow).toBe(true)
    expect(store.loading).toBe(false)
  })

  it('确认成功后忽略同版本旧请求迟到的 423', async () => {
    const store = useAdminComplianceStore()
    store.requireAcknowledgement({ version: 'v2026.06.10' })
    vi.mocked(adminComplianceAPI.accept).mockResolvedValue(createStatus(false))

    await store.accept('中文确认短语')
    store.requireAcknowledgement({ version: 'v2026.06.10' })

    expect(store.status?.required).toBe(false)
    expect(store.shouldShow).toBe(false)
  })
})
