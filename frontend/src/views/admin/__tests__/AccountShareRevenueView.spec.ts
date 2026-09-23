import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const viewSource = readFileSync(resolve(here, '../AccountShareRevenueView.vue'), 'utf8')
const routerSource = readFileSync(resolve(here, '../../../router/index.ts'), 'utf8')
const sidebarSource = readFileSync(resolve(here, '../../../components/layout/AppSidebar.vue'), 'utf8')

describe('account sharing revenue management surface', () => {
  it('keeps policy management and settlement audit as separate tabs', () => {
    expect(viewSource).toContain("activeTab === 'policies'")
    expect(viewSource).toContain("activeTab === 'settlements'")
    expect(viewSource).toContain("accountShareAPI.listPolicies")
    expect(viewSource).toContain("accountShareAPI.listSettlements")
  })

  it('exposes all four policy scopes and validates the combined ratio', () => {
    for (const scope of ['global', 'platform', 'group', 'account']) {
      expect(viewSource).toContain(`value: '${scope}'`)
    }
    expect(viewSource).toContain('owner + invite > 100')
    expect(viewSource).toContain('policyFormError')
  })

  it('is admin-only and remains in the account-management menu', () => {
    expect(routerSource).toContain("path: '/admin/revenue'")
    expect(routerSource).toContain("name: 'AdminRevenue'")
    expect(routerSource).toContain("path: '/admin/account-share-revenue'")
    expect(routerSource).toContain('name: \'AdminAccountShareRevenue\'')
    expect(routerSource).toContain('requiresAdmin: true')
    expect(sidebarSource).toContain("{ path: '/admin/revenue', label: t('nav.revenue')")
    expect(sidebarSource).toContain("{ path: '/admin/account-share-revenue', label: t('nav.accountShareRevenue')")
    expect(sidebarSource).toContain("{ path: '/admin/accounts', label: t('nav.accounts')")
  })
})
