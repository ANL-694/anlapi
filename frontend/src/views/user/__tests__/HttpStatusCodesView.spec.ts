import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../HttpStatusCodesView.vue'),
  'utf8'
)
const routerSource = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../../router/index.ts'), 'utf8')
const sidebarSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../../../components/layout/AppSidebar.vue'),
  'utf8'
)

describe('HTTP status codes page', () => {
  it('is grouped, keyboard-expandable, bilingual, and reachable from navigation', () => {
    expect(viewSource).toContain('http-status-group-header')
    expect(viewSource).toContain(':aria-expanded="isGroupExpanded(group.range)"')
    expect(viewSource).toContain("locale.value.startsWith('zh')")
    expect(viewSource).toContain('status(425')
    expect(routerSource).toContain("name: 'HttpStatusCodes'")
    expect(sidebarSource).toContain("path: '/http-status-codes'")
    expect(sidebarSource).toContain("t('nav.httpStatusCodes')")
  })
})
