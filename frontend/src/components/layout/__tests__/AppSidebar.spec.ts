import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const routerSource = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../../router/index.ts'), 'utf8')
const statusViewSource = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../../views/user/HttpStatusCodesView.vue'), 'utf8')
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

describe('AppSidebar collapsed groups', () => {
  it('expands the sidebar instead of ignoring a grouped navigation click', () => {
    expect(componentSource).toContain('appStore.setSidebarCollapsed(false)')
    expect(componentSource).toContain('expandedGroups.value.add(item.path)')
  })
})

describe('HTTP status code navigation', () => {
  it('exposes the user-facing menu item and route', () => {
    expect(componentSource).toContain("path: '/http-status-codes'")
    expect(componentSource).toContain("label: t('nav.httpStatusCodes')")
    expect(routerSource).toContain("path: '/http-status-codes'")
    expect(routerSource).toContain("name: 'HttpStatusCodes'")
    expect(routerSource).toContain("requiresAdmin: false")
  })

  it('keeps status code categories collapsed until clicked', () => {
    expect(statusViewSource).toContain('const expandedGroups = ref<Set<string>>(new Set())')
    expect(statusViewSource).toContain('@click="toggleGroup(group.range)"')
    expect(statusViewSource).toContain('v-if="isGroupExpanded(group.range)"')
    expect(statusViewSource).toContain(':aria-expanded="isGroupExpanded(group.range)"')
  })
})
