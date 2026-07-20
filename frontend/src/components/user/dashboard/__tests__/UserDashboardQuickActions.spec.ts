import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../UserDashboardQuickActions.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('UserDashboardQuickActions', () => {
  it('keeps the recharge, key creation, and usage routes reachable from the dashboard', () => {
    expect(componentSource).toContain('to="/purchase"')
    expect(componentSource).toContain('to="/keys"')
    expect(componentSource).toContain('to="/usage"')
  })

  it('uses theme contrast tokens for the primary recharge action', () => {
    expect(componentSource).toContain('background: var(--ui-brand)')
    expect(componentSource).toContain('color: var(--ui-brand-contrast)')
  })
})
