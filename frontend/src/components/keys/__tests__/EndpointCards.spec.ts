import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const copyToClipboard = vi.fn().mockResolvedValue(true)

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

import EndpointCards from '../EndpointCards.vue'

describe('EndpointCards', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the default route first and removes duplicate normalized URLs', () => {
    const wrapper = mount(EndpointCards, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1/',
        customEndpoints: [
          { name: '重复默认', endpoint: 'HTTPS://DEFAULT.EXAMPLE.COM/v1', description: '' },
          { name: '备用线路', endpoint: 'backup.example.com/v1///', description: '备用说明' },
        ],
      },
      global: {
        stubs: { Icon: { template: '<span />' } },
      },
    })

    expect(wrapper.findAll('code').map((code) => code.text())).toEqual([
      'https://default.example.com/v1',
      'https://backup.example.com/v1',
    ])
    expect(wrapper.text()).toContain('备用说明')
    expect(wrapper.text()).not.toContain('重复默认')
  })

  it('copies the displayed normalized endpoint and shows the copied state', async () => {
    const wrapper = mount(EndpointCards, {
      props: {
        apiBaseUrl: '/api/v1/',
        customEndpoints: [],
      },
      global: {
        stubs: { Icon: { template: '<span />' } },
      },
    })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('/api/v1', 'keys.endpoints.copied')
    expect(wrapper.find('button').attributes('title')).toBe('keys.endpoints.copiedHint')
  })
})
