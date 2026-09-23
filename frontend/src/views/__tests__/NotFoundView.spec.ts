import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import NotFoundView from '../NotFoundView.vue'

const state = vi.hoisted(() => ({
  route: { fullPath: '/admin/missing' },
  push: vi.fn(),
  appStore: { contactInfo: '' },
  authStore: { isAdmin: true },
}))

vi.mock('vue-router', () => ({
  useRoute: () => state.route,
  useRouter: () => ({ back: state.push }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => state.appStore,
  useAuthStore: () => state.authStore,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('NotFoundView', () => {
  it('uses the admin home for an administrator on a missing admin route', () => {
    const wrapper = mount(NotFoundView, {
      global: {
        stubs: {
          Icon: true,
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="to"><slot /></a>',
          },
        },
      },
    })

    expect(wrapper.get('[data-to]').attributes('data-to')).toBe('/admin/dashboard')
    expect(wrapper.text()).toContain('common.pageNotFoundDescription')
  })
})
