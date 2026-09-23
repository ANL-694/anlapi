import { mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AuthLayout from '../AuthLayout.vue'

const appStore = vi.hoisted(() => ({
  siteName: 'ANL API',
  siteLogo: '',
  publicSettingsLoaded: true,
  cachedPublicSettings: {
    site_subtitle: '统一 AI API 网关',
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const iconButtonStub = {
  emits: ['click'],
  template: '<button type="button" data-testid="theme-toggle" @click="$emit(\'click\')"><slot /></button>',
}

describe('AuthLayout ANL shell', () => {
  beforeEach(() => {
    appStore.fetchPublicSettings.mockClear()
    appStore.publicSettingsLoaded = true
    document.documentElement.classList.remove('dark')
    localStorage.clear()
  })

  it('renders the ANL brand shell without the legacy decorative background', () => {
    const wrapper = mount(AuthLayout, {
      slots: { default: '<h2>登录</h2>' },
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          LocaleSwitcher: true,
          UiIconButton: iconButtonStub,
          Icon: true,
        },
      },
    })

    expect(wrapper.get('.auth-brand-name').text()).toBe('ANL API')
    expect(wrapper.get('.auth-brand-subtitle').text()).toBe('统一 AI API 网关')
    expect(wrapper.get('.auth-logo img').attributes('src')).toBe('/logo.svg')
    expect(wrapper.find('.bg-gradient-to-br').exists()).toBe(false)
    expect(appStore.fetchPublicSettings).toHaveBeenCalledOnce()
  })

  it('keeps the local ANL logo visible while public settings are unavailable', () => {
    appStore.publicSettingsLoaded = false

    const wrapper = mount(AuthLayout, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          LocaleSwitcher: true,
          UiIconButton: iconButtonStub,
          Icon: true,
        },
      },
    })

    expect(wrapper.get('.auth-logo img').attributes('src')).toBe('/logo.svg')
  })

  it('keeps the login theme toggle usable', async () => {
    const wrapper = mount(AuthLayout, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
          LocaleSwitcher: true,
          UiIconButton: iconButtonStub,
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-testid="theme-toggle"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
  })
})
