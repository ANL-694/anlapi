import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const getStatus = vi.fn()
const accept = vi.fn()

vi.mock('@/api/admin/compliance', () => ({
  default: { getStatus, accept }
}))

vi.mock('@/i18n', () => ({
  getLocale: vi.fn(() => 'zh')
}))

describe('admin compliance fallback branding', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getStatus.mockReset()
    accept.mockReset()
  })

  it('uses ANL wording when the backend does not provide an acknowledgement phrase', async () => {
    const { useAdminComplianceStore } = await import('../adminCompliance')
    const store = useAdminComplianceStore()

    store.requireAcknowledgement({ required: true })

    expect(store.expectedPhrase).toContain('ANL API')
    expect(store.expectedPhrase).not.toContain('Sub2API')
  })
})
