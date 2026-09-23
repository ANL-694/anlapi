import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const copyToClipboard = vi.fn().mockResolvedValue(true)

const messages: Record<string, string> = {
  'keys.endpoints.title': 'API 端点',
  'keys.endpoints.default': '默认',
  'keys.endpoints.copied': '已复制',
  'keys.endpoints.copiedHint': '已复制到剪贴板',
  'keys.endpoints.clickToCopy': '点击可复制此端点',
  'keys.endpoints.speedTest': '测速',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

import EndpointPopover from '../EndpointPopover.vue'

describe('EndpointPopover', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('将说明提示渲染到 URL 上方而不是旧的 title 图标上', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com/v1',
            description: '自定义说明',
          },
        ],
      },
    })

    expect(wrapper.text()).toContain('自定义说明')
    expect(wrapper.text()).toContain('点击可复制此端点')
    expect(wrapper.find('[role="button"]').attributes('title')).toBeUndefined()
    expect(wrapper.find('[title="自定义说明"]').exists()).toBe(false)
  })

  it('点击 URL 后会复制并切换为已复制提示', async () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [],
      },
    })

    await wrapper.find('[role="button"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('https://default.example.com/v1', '已复制')
    expect(wrapper.text()).toContain('已复制到剪贴板')
    expect(wrapper.find('button[aria-label="已复制到剪贴板"]').exists()).toBe(true)
  })

  it('规范化端点并按 URL 去重，默认端点优先', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1/',
        customEndpoints: [
          { name: '重复默认', endpoint: 'HTTPS://DEFAULT.EXAMPLE.COM/v1', description: '' },
          { name: '备用线路', endpoint: 'backup.example.com/v1///', description: '' },
          { name: '大小写敏感路径', endpoint: 'https://backup.example.com/API/v1', description: '' },
        ],
      },
    })

    expect(wrapper.findAll('code').map((item) => item.text())).toEqual([
      'https://default.example.com/v1',
      'https://backup.example.com/v1',
      'https://backup.example.com/API/v1',
    ])
  })

  it('保留相对 API 路径，不把本地网关拼成无效 HTTPS URL', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: '/api/v1/',
        customEndpoints: [],
      },
    })

    expect(wrapper.find('code').text()).toBe('/api/v1')
  })

  it('测速时不将端点凭据或查询参数发送给第三方', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://user:password@example.com/v1?token=secret#fragment',
        customEndpoints: [],
      },
    })

    const href = wrapper.get('a[title="测速"]').attributes('href')
    expect(decodeURIComponent(href)).toContain('https://example.com/v1')
    expect(decodeURIComponent(href)).not.toContain('user')
    expect(decodeURIComponent(href)).not.toContain('password')
    expect(decodeURIComponent(href)).not.toContain('token=secret')
    expect(decodeURIComponent(href)).not.toContain('fragment')
  })
})
