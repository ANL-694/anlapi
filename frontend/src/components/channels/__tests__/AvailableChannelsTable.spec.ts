import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AvailableChannelsTable.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AvailableChannelsTable scrolling', () => {
  it('keeps a long channel list in a bounded vertical scroll container', () => {
    expect(componentSource).toContain('class="channel-list channel-list--scrollable"')
    expect(componentSource).toMatch(/\.channel-list--scrollable\s*\{[\s\S]*overflow-y:\s*auto;/)
  })
})
