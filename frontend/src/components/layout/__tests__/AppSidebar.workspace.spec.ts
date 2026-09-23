import { mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppSidebar from '../AppSidebar.vue'

const state = vi.hoisted(() => ({
  route: {
    path: '/dashboard',
    fullPath: '/dashboard',
    meta: { requiresAdmin: false },
  },
  push: vi.fn(),
  appStore: {
    sidebarCollapsed: false,
    mobileOpen: false,
    backendModeEnabled: false,
    cachedPublicSettings: {
      payment_enabled: true,
      purchase_subscription_enabled: false,
      purchase_subscription_url: '',
      custom_menu_items: [],
    },
    siteName: 'anl-api',
    siteLogo: '',
    siteVersion: '0.1.176',
    publicSettingsLoaded: true,
    sidebarScrollTop: 0,
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn(),
  },
  authStore: {
    isAdmin: false,
    isSimpleMode: false,
    user: { id: 1, role: 'user' },
    token: 'test-token',
  },
  adminSettingsStore: {
    opsMonitoringEnabled: true,
    paymentEnabled: true,
    customMenuItems: [],
    fetch: vi.fn(),
  },
  onboardingStore: {
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  },
  refreshBatchImageAccess: vi.fn(),
  featureFlags: {
    channelMonitor: true,
    payment: true,
    availableChannels: true,
    freeModels: true,
    carpool: true,
    affiliate: true,
    riskControl: true,
  } as Record<string, boolean>,
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
      locale: { value: 'zh-CN' },
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => state.appStore,
  useAuthStore: () => state.authStore,
  useAdminSettingsStore: () => state.adminSettingsStore,
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => state.onboardingStore,
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({
    canUseBatchImage: { value: true },
    refreshBatchImageAccess: state.refreshBatchImageAccess,
  }),
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    channelMonitor: 'channelMonitor',
    payment: 'payment',
    availableChannels: 'availableChannels',
    freeModels: 'freeModels',
    carpool: 'carpool',
    affiliate: 'affiliate',
    riskControl: 'riskControl',
  },
  makeSidebarFlag: (flag: string) => () => state.featureFlags[flag] !== false,
}))

function mountSidebar() {
  return mount(AppSidebar, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        VersionBadge: true,
      },
    },
  })
}

function linkTargets(wrapper: ReturnType<typeof mountSidebar>): string[] {
  return wrapper
    .findAllComponents(RouterLinkStub)
    .map((link) => link.props('to'))
    .filter((target): target is string => typeof target === 'string')
}

async function expandGroup(wrapper: ReturnType<typeof mountSidebar>, label: string): Promise<void> {
  const button = wrapper.findAll('button').find((candidate) => candidate.text().includes(label))
  if (!button) throw new Error(`Missing sidebar group: ${label}`)
  await button.trigger('click')
}

describe('AppSidebar user/admin workspace behavior', () => {
  beforeEach(() => {
    state.route.path = '/dashboard'
    state.route.fullPath = '/dashboard'
    state.route.meta.requiresAdmin = false
    state.push.mockReset()
    state.appStore.sidebarCollapsed = false
    state.appStore.mobileOpen = false
    state.authStore.isAdmin = false
    state.authStore.isSimpleMode = false
    state.authStore.user = { id: 1, role: 'user' }
    state.adminSettingsStore.fetch.mockClear()
    state.refreshBatchImageAccess.mockClear()
    for (const key of Object.keys(state.featureFlags)) state.featureFlags[key] = true
    localStorage.clear()
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })

  it('shows only user navigation to a regular user', async () => {
    const wrapper = mountSidebar()
    await expandGroup(wrapper, 'nav.accountManagement')
    const targets = linkTargets(wrapper)

    expect(wrapper.find('[data-testid="sidebar-workspace-switch"]').exists()).toBe(false)
    expect(targets).toContain('/dashboard')
    expect(targets).toContain('/accounts')
    expect(targets).not.toContain('/admin/dashboard')
    expect(targets).not.toContain('/admin/users')
  })

  it('does not fetch administrator settings for a regular user', () => {
    mountSidebar()

    expect(state.adminSettingsStore.fetch).not.toHaveBeenCalled()
  })

  it('shows the same user navigation when an administrator opens the user workspace', async () => {
    state.authStore.isAdmin = true
    state.authStore.user = { id: 1, role: 'admin' }

    const wrapper = mountSidebar()
    await expandGroup(wrapper, 'nav.accountManagement')
    const targets = linkTargets(wrapper)

    expect(wrapper.get('[data-workspace="user"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-workspace="admin"]').attributes('aria-selected')).toBe('false')
    expect(targets).toContain('/dashboard')
    expect(targets).toContain('/accounts')
    expect(targets).not.toContain('/admin/users')
  })

  it('keeps a simple-mode administrator on user navigation when opening a user route', async () => {
    state.authStore.isAdmin = true
    state.authStore.isSimpleMode = true
    state.authStore.user = { id: 1, role: 'admin' }

    const wrapper = mountSidebar()
    const targets = linkTargets(wrapper)

    expect(wrapper.find('[data-testid="sidebar-workspace-switch"]').exists()).toBe(false)
    expect(targets).toContain('/dashboard')
    expect(targets).toContain('/keys')
    expect(targets).not.toContain('/admin/users')
    expect(targets).not.toContain('/admin/dashboard')
  })

  it('shows only management navigation when an administrator opens the admin workspace', async () => {
    state.authStore.isAdmin = true
    state.authStore.user = { id: 1, role: 'admin' }
    state.route.path = '/admin/dashboard'
    state.route.fullPath = '/admin/dashboard'
    state.route.meta.requiresAdmin = true

    const wrapper = mountSidebar()
    await expandGroup(wrapper, 'nav.accountManagement')
    const targets = linkTargets(wrapper)

    expect(wrapper.get('[data-workspace="user"]').attributes('aria-selected')).toBe('false')
    expect(wrapper.get('[data-workspace="admin"]').attributes('aria-selected')).toBe('true')
    expect(targets).toContain('/admin/dashboard')
    expect(targets).toContain('/admin/users')
    expect(targets).toContain('/admin/accounts')
    expect(targets).not.toContain('/accounts')
  })

  it('keeps both workspace controls usable while the desktop sidebar is collapsed', () => {
    state.authStore.isAdmin = true
    state.authStore.user = { id: 1, role: 'admin' }
    state.appStore.sidebarCollapsed = true

    const wrapper = mountSidebar()
    const switcher = wrapper.get('[data-testid="sidebar-workspace-switch"]')

    expect(switcher.classes()).toContain('sidebar-workspace-switch-collapsed')
    expect(wrapper.get('[data-workspace="user"]').attributes('title')).toBe('nav.userWorkspace')
    expect(wrapper.get('[data-workspace="admin"]').attributes('title')).toBe('nav.adminWorkspace')
  })

  it('routes workspace clicks to the matching dashboard', async () => {
    state.authStore.isAdmin = true
    state.authStore.user = { id: 1, role: 'admin' }
    state.route.path = '/admin/dashboard'
    state.route.fullPath = '/admin/dashboard'
    state.route.meta.requiresAdmin = true

    const wrapper = mountSidebar()
    await wrapper.get('[data-workspace="user"]').trigger('click')
    expect(state.push).toHaveBeenCalledWith('/dashboard')

    state.route.path = '/dashboard'
    state.route.fullPath = '/dashboard'
    state.route.meta.requiresAdmin = false
    await wrapper.get('[data-workspace="admin"]').trigger('click')
    expect(state.push).toHaveBeenCalledWith('/admin/dashboard')
  })

  it('reprojects the navigation after a workspace route change without remounting', async () => {
    state.authStore.isAdmin = true
    state.authStore.user = { id: 1, role: 'admin' }
    state.route.path = '/admin/dashboard'
    state.route.fullPath = '/admin/dashboard'
    state.route.meta.requiresAdmin = true

    const wrapper = mountSidebar()
    await expandGroup(wrapper, 'nav.accountManagement')
    expect(linkTargets(wrapper)).toContain('/admin/users')
    expect(linkTargets(wrapper)).not.toContain('/accounts')

    wrapper.unmount()
    state.route.path = '/dashboard'
    state.route.fullPath = '/dashboard'
    state.route.meta.requiresAdmin = false
    const refreshedWrapper = mountSidebar()
    await expandGroup(refreshedWrapper, 'nav.accountManagement')

    expect(refreshedWrapper.get('[data-workspace="user"]').attributes('aria-selected')).toBe('true')
    expect(refreshedWrapper.get('[data-workspace="admin"]').attributes('aria-selected')).toBe('false')
    expect(linkTargets(refreshedWrapper)).toContain('/accounts')
    expect(linkTargets(refreshedWrapper)).not.toContain('/admin/users')
  })

  it('closes the mobile sidebar after selecting a navigation item', async () => {
    state.authStore.isAdmin = true
    state.authStore.user = { id: 1, role: 'admin' }
    state.route.path = '/admin/dashboard'
    state.route.fullPath = '/admin/dashboard'
    state.route.meta.requiresAdmin = true
    state.appStore.mobileOpen = true

    vi.useFakeTimers()
    try {
      const wrapper = mountSidebar()
      await expandGroup(wrapper, 'nav.accountManagement')
      const usersLink = wrapper
        .findAllComponents(RouterLinkStub)
        .find((link) => link.props('to') === '/admin/users')
      expect(usersLink).toBeDefined()

      await usersLink!.trigger('click')
      vi.advanceTimersByTime(150)

      expect(state.appStore.setMobileOpen).toHaveBeenCalledWith(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('removes a navigation group when all of its feature-gated children are hidden', () => {
    state.featureFlags.availableChannels = false
    state.featureFlags.channelMonitor = false

    const wrapper = mountSidebar()

    expect(wrapper.text()).not.toContain('nav.modelLobby')
    expect(wrapper.text()).not.toContain('nav.availableChannels')
    expect(wrapper.text()).not.toContain('nav.channelStatus')
  })
})
