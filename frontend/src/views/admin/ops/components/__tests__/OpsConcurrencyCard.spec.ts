import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsConcurrencyCard from '../OpsConcurrencyCard.vue'

const mockGetUserConcurrencyStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getUserConcurrencyStats: (...args: unknown[]) => mockGetUserConcurrencyStats(...args)
  }
}))

vi.mock('vue-i18n', async importOriginal => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.count === undefined ? key : `${key}:${String(params.count)}`
    })
  }
})

describe('OpsConcurrencyCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetUserConcurrencyStats.mockResolvedValue({
      enabled: true,
      user: {
        '42': {
          user_id: 42,
          user_email: 'user@example.com',
          username: 'ANL User',
          current_in_use: 2,
          max_capacity: 6
        },
        '43': {
          user_id: 43,
          user_email: 'idle@example.com',
          username: 'Idle User',
          current_in_use: 0,
          max_capacity: 4
        }
      }
    })
  })

  it('只请求并展示真实用户并发', async () => {
    const wrapper = mount(OpsConcurrencyCard, { props: { refreshToken: 0 } })

    await flushPromises()

    expect(mockGetUserConcurrencyStats).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('ANL User')
    expect(wrapper.text()).toContain('Idle User')
    expect(wrapper.text()).toContain('admin.ops.concurrency.currentInUse')
    expect(wrapper.text()).toContain('admin.ops.concurrency.userLimit')
    expect(wrapper.text()).toContain('2')
    expect(wrapper.text()).toContain('6')
    expect(wrapper.text()).toContain('admin.ops.concurrency.byUser')
    expect(wrapper.text()).not.toContain('admin.ops.concurrency.queued')
    expect(wrapper.text()).not.toContain('33%')
    expect(wrapper.html()).not.toContain('transition-all duration-300')
    expect(wrapper.findAll('button')).toHaveLength(1)
  })

  it('刷新令牌变化时重新读取实时并发', async () => {
    const wrapper = mount(OpsConcurrencyCard, { props: { refreshToken: 0 } })
    await flushPromises()

    await wrapper.setProps({ refreshToken: 1 })
    await flushPromises()

    expect(mockGetUserConcurrencyStats).toHaveBeenCalledTimes(2)
  })

  it('用户并发功能关闭时显示禁用状态', async () => {
    mockGetUserConcurrencyStats.mockResolvedValue({ enabled: false, user: {} })

    const wrapper = mount(OpsConcurrencyCard, { props: { refreshToken: 0 } })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.ops.concurrency.disabledHint')
  })
})
