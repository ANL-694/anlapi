import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppHeader.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppHeader balance display', () => {
  it('keeps the signed-in user balance visible before the avatar menu', () => {
    const announcementIndex = componentSource.indexOf('<AnnouncementBell v-if="user"')
    const balanceIndex = componentSource.indexOf('class="app-header-balance"')
    const dropdownIndex = componentSource.indexOf('ref="dropdownRef"')

    expect(announcementIndex).toBeGreaterThanOrEqual(0)
    expect(balanceIndex).toBeGreaterThan(announcementIndex)
    expect(dropdownIndex).toBeGreaterThan(balanceIndex)
    expect(componentSource).toContain('to="/purchase"')
    expect(componentSource).toContain('<Icon name="dollar" size="sm"')
    expect(componentSource).toContain("{{ t('common.balance') }}")
    expect(componentSource).toContain('${{ formattedBalance }}')
    expect(componentSource).toContain('Number.isFinite(balance)')
    expect(componentSource).toContain('app-header-announcement')
  })

  it('keeps the compact mobile header inside narrow viewports', () => {
    expect(componentSource).toContain('app-header-subscription-progress')
    expect(styleSource).toMatch(/@media \(max-width: 420px\)[\s\S]*?\.app-header-leading[\s\S]*?min-width: 5rem/)
    expect(styleSource).toMatch(/@media \(max-width: 420px\)[\s\S]*?\.app-header-balance-icon[\s\S]*?display: none/)
    expect(styleSource).toMatch(/@media \(max-width: 360px\)[\s\S]*?\.app-header-subscription-progress[\s\S]*?display: none/)
  })
})
