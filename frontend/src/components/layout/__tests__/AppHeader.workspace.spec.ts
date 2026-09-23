import { mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppHeader from '../AppHeader.vue'

const state = vi.hoisted(() => ({
  route: {
    name: 'Dashboard',
    path: '/dashboard',
    params: {} as Record<string, string>,
    meta: {
      requiresAdmin: false,
      title: 'Dashboard',
      titleKey: 'dashboard.title',
      description: '',
      descriptionKey: '',
    },
  },
  push: vi.fn(),
  appStore: {
    contactInfo: '',
    docUrl: 'https://docs.example.test',
    cachedPublicSettings: {
      custom_menu_items: [],
    },
    toggleMobileSidebar: vi.fn(),
  },
  authStore: {
    user: {
      id: 1,
      role: 'user',
      username: 'member',
      email: 'member@example.test',
      balance: 12,
      frozen_balance: 3,
      avatar_url: '',
    },
    isAdmin: false,
    isSimpleMode: false,
    logout: vi.fn(),
  },
  adminSettingsStore: {
    customMenuItems: [] as Array<{ id: string; label: string }>,
  },
  onboardingStore: {
    replay: vi.fn(),
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => state.route,
  useRouter: () => ({ push: state.push }),
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

vi.mock('@/stores', () => ({
  useAppStore: () => state.appStore,
  useAuthStore: () => state.authStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => state.adminSettingsStore,
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => state.onboardingStore,
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: { modelPlaza: 'modelPlaza' },
  isFeatureFlagEnabled: () => true,
}))

function mountHeader() {
  return mount(AppHeader, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: true,
        SubscriptionProgressMini: true,
        AnnouncementBell: true,
        Icon: true,
      },
    },
  })
}

describe('AppHeader workspace context', () => {
  beforeEach(() => {
    state.route.name = 'Dashboard'
    state.route.path = '/dashboard'
    state.route.params = {}
    state.route.meta.requiresAdmin = false
    state.route.meta.title = 'Dashboard'
    state.route.meta.titleKey = 'dashboard.title'
    state.route.meta.description = ''
    state.route.meta.descriptionKey = ''
    state.authStore.user.role = 'user'
    state.authStore.isAdmin = false
    state.authStore.isSimpleMode = false
    state.adminSettingsStore.customMenuItems = []
    state.push.mockReset()
  })

  it('labels a regular user route as the user workspace', () => {
    const wrapper = mountHeader()
    const badge = wrapper.get('[data-testid="header-workspace-badge"]')

    expect(badge.text()).toContain('nav.userWorkspace')
    expect(badge.classes()).not.toContain('app-header-workspace-admin')
  })

  it('keeps an administrator on a user route in the same user workspace', () => {
    state.authStore.user.role = 'admin'
    state.authStore.isAdmin = true

    const wrapper = mountHeader()
    expect(wrapper.get('[data-testid="header-workspace-badge"]').text()).toContain('nav.userWorkspace')
  })

  it('keeps a simple-mode administrator on a user route in the user workspace', () => {
    state.authStore.user.role = 'admin'
    state.authStore.isAdmin = true
    state.authStore.isSimpleMode = true

    const wrapper = mountHeader()
    expect(wrapper.get('[data-testid="header-workspace-badge"]').text()).toContain('nav.userWorkspace')
    expect(wrapper.find('[data-testid="header-workspace-switch"]').exists()).toBe(false)
  })

  it('does not render a workspace switch for a regular user', () => {
    const wrapper = mountHeader()

    expect(wrapper.find('[data-testid="header-workspace-switch"]').exists()).toBe(false)
  })

  it('lets an administrator switch from the user workspace to the management workspace', async () => {
    state.authStore.user.role = 'admin'
    state.authStore.isAdmin = true

    const wrapper = mountHeader()
    const switcher = wrapper.get('[data-testid="header-workspace-switch"]')

    expect(switcher.get('[data-workspace="user"]').attributes('aria-selected')).toBe('true')
    expect(switcher.get('[data-workspace="admin"]').attributes('aria-selected')).toBe('false')

    await switcher.get('[data-workspace="admin"]').trigger('click')

    expect(state.push).toHaveBeenCalledWith('/admin/dashboard')
  })

  it('lets an administrator switch from the management workspace to the real user workspace', async () => {
    state.authStore.user.role = 'admin'
    state.authStore.isAdmin = true
    state.route.name = 'AdminDashboard'
    state.route.path = '/admin/dashboard'
    state.route.meta.requiresAdmin = true
    state.route.meta.titleKey = 'admin.dashboard.title'

    const wrapper = mountHeader()
    const switcher = wrapper.get('[data-testid="header-workspace-switch"]')

    expect(switcher.get('[data-workspace="user"]').attributes('aria-selected')).toBe('false')
    expect(switcher.get('[data-workspace="admin"]').attributes('aria-selected')).toBe('true')

    await switcher.get('[data-workspace="user"]').trigger('click')

    expect(state.push).toHaveBeenCalledWith('/dashboard')
  })

  it('labels routes protected by requiresAdmin as the management workspace', () => {
    state.authStore.user.role = 'admin'
    state.authStore.isAdmin = true
    state.route.name = 'AdminDashboard'
    state.route.path = '/admin/dashboard'
    state.route.meta.requiresAdmin = true
    state.route.meta.titleKey = 'admin.dashboard.title'

    const wrapper = mountHeader()
    const badge = wrapper.get('[data-testid="header-workspace-badge"]')

    expect(badge.text()).toContain('nav.adminWorkspace')
    expect(badge.classes()).toContain('app-header-workspace-admin')
  })

  it('uses only the admin menu label on an admin custom page', () => {
    state.authStore.user.role = 'admin'
    state.authStore.isAdmin = true
    state.route.name = 'AdminCustomPage'
    state.route.path = '/admin/custom/shared'
    state.route.params = { id: 'shared' }
    state.route.meta.requiresAdmin = true
    state.appStore.cachedPublicSettings.custom_menu_items = [{ id: 'shared', label: '用户页面' }]
    state.adminSettingsStore.customMenuItems = [{ id: 'shared', label: '管理页面' }]

    const wrapper = mountHeader()
    expect(wrapper.find('.app-header-title').text()).toBe('管理页面')
  })
})
