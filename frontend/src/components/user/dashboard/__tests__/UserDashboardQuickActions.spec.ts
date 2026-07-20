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

  it('shows live balance, key, and today usage context instead of a passive button row', () => {
    expect(componentSource).toContain('formattedBalance')
    expect(componentSource).toContain('props.stats.total_api_keys')
    expect(componentSource).toContain('props.stats.today_requests')
    expect(componentSource).toContain("t('dashboard.rechargeBalance')")
  })

  it('keeps keyboard focus and reduced-motion states visible', () => {
    expect(componentSource).toContain('.dashboard-action-balance:focus-visible')
    expect(componentSource).toContain('@media (prefers-reduced-motion: reduce)')
  })
})
