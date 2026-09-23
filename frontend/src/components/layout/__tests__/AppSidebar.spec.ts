import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar mobile drawer transition', () => {
  it('defines removable fade classes for the mobile overlay', () => {
    expect(componentSource).toContain('.fade-enter-active,')
    expect(componentSource).toContain('transition: opacity 0.18s ease;')
    expect(componentSource).toContain('.fade-leave-to {')
    expect(componentSource).toContain('opacity: 0;')
  })

  it('keeps the desktop collapse preference control out of the mobile drawer', () => {
    expect(componentSource).toContain('class="sidebar-link hidden w-full lg:flex"')
  })
})

describe('AppSidebar workspace separation', () => {
  it('provides an admin-only user/admin workspace switch', () => {
    expect(componentSource).toContain("v-if=\"isAdmin && !authStore.isSimpleMode\"")
    expect(componentSource).toContain("'sidebar-workspace-switch-collapsed': sidebarCollapsed")
    expect(componentSource).toContain("@click=\"switchWorkspace('user')\"")
    expect(componentSource).toContain("@click=\"switchWorkspace('admin')\"")
    expect(componentSource).toContain('const workspace = computed(() => resolveAppWorkspace(route))')
    expect(componentSource).toContain("const isAdminWorkspace = computed(() => workspace.value === 'admin')")
  })

  it('keeps the workspace switch visible and identifiable when the sidebar is collapsed', () => {
    expect(componentSource).toContain('data-testid="sidebar-workspace-switch"')
    expect(componentSource).toContain('data-workspace="user"')
    expect(componentSource).toContain('data-workspace="admin"')
    expect(componentSource).toContain(':title="sidebarCollapsed ? t(\'nav.userWorkspace\') : undefined"')
    expect(componentSource).toContain(':title="sidebarCollapsed ? t(\'nav.adminWorkspace\') : undefined"')
    expect(componentSource).toContain('.sidebar-workspace-switch-collapsed {')
  })

  it('renders only the active workspace navigation for admins', () => {
    expect(componentSource).toContain('v-for="item in visibleNavItems"')
    expect(componentSource).toContain('const visibleNavItems = computed')
    expect(componentSource).toContain('if (!isAdmin.value) return userNavItems.value')
    expect(componentSource).toContain('return isAdminWorkspace.value ? adminNavItems.value : userNavItems.value')
    expect(componentSource).toContain("const homePath = computed(() => (isAdminWorkspace.value ? '/admin/dashboard' : '/dashboard'))")
  })

  it('keeps the user workspace grouped into user-facing areas', () => {
    expect(componentSource).toContain("path: '/self/accounts'")
    expect(componentSource).toContain("path: '/self/models'")
    expect(componentSource).toContain("path: '/self/profile-center'")
    expect(componentSource).toContain("path: '/self/extras'")
    expect(componentSource).toContain("path: '/admin/account-management'")
    expect(componentSource).toContain("path: '/available-channels'")
    expect(componentSource).toContain("path: '/accounts'")
    expect(componentSource).toContain("path: '/accounts/free-models'")
    expect(componentSource).toContain("path: '/accounts/carpools'")
    expect(componentSource).toContain("path: '/models'")
    expect(componentSource).toContain("path: '/profile'")
    expect(componentSource).toContain('item.children?.length')
    expect(componentSource).toContain('handleGroupClick(item)')
  })

  it('keeps custom menu visibility and external-link behavior role-scoped', () => {
    expect(componentSource).toContain(".filter((item) => item.visibility === 'user')")
    expect(componentSource).toContain(".filter((item) => item.visibility === 'admin')")
    expect(componentSource).toContain('openInNewWindow?: boolean')
    expect(componentSource).toContain('function buildNewWindowUrl(item: NavItem)')
    expect(componentSource).toContain("window.open(buildNewWindowUrl(item), '_blank', 'noopener,noreferrer')")
    expect(componentSource).toContain('path: `/admin/custom/${cm.id}`')
  })

  it('preserves official onboarding and clear child-menu selection behavior', () => {
    expect(componentSource).toContain('const onboardingStore = useOnboardingStore()')
    expect(componentSource).toContain("'/admin/groups': '#sidebar-group-manage'")
    expect(componentSource).toContain("'/admin/accounts': '#sidebar-channel-manage'")
    expect(componentSource).toContain("if (path === '/accounts') return route.path === path")
    expect(componentSource).toContain('const collapsedGroups = ref<Set<string>>(new Set())')
  })

  it('keeps the ANL external purchase entry visible when the internal payment switch is off', () => {
    expect(componentSource).toContain('settings.purchase_subscription_enabled === true')
    expect(componentSource).toContain('settings.purchase_subscription_url?.trim() || \'\'')
    expect(componentSource).toContain('return settings.payment_enabled === true || externalPurchaseEnabled')
  })
})

describe('AppSidebar brand fallback', () => {
  it('renders the local ANL logo while public settings are loading or unavailable', () => {
    expect(componentSource).toContain(':src="siteLogo || DEFAULT_FAVICON"')
    expect(componentSource).not.toContain('v-if="settingsLoaded"')
    expect(componentSource).toContain("import { DEFAULT_FAVICON } from '@/utils/branding'")
  })
})
