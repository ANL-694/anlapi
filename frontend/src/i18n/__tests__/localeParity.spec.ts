import { describe, expect, it } from 'vitest'
import en from '../locales/en'
import zh from '../locales/zh'

function collectLeafKeys(value: unknown, prefix = ''): string[] {
  if (value === null || typeof value !== 'object') return prefix ? [prefix] : []
  if (Array.isArray(value)) return prefix ? [prefix] : []

  return Object.entries(value).flatMap(([key, child]) =>
    collectLeafKeys(child, prefix ? `${prefix}.${key}` : key)
  )
}

function normalizeKey(key: string): string {
  // Both locale files intentionally retain the historical nesting used by older admin bundles.
  return key.replace(
    'admin.groups.modelRouting.claudeMaxSimulation',
    'admin.groups.claudeMaxSimulation'
  )
}

describe('locale parity', () => {
  it('keeps English and Chinese message key sets identical', () => {
    const englishKeys = new Set(collectLeafKeys(en).map(normalizeKey))
    const chineseKeys = new Set(collectLeafKeys(zh).map(normalizeKey))
    const missingInEnglish = [...chineseKeys].filter((key) => !englishKeys.has(key)).sort()
    const missingInChinese = [...englishKeys].filter((key) => !chineseKeys.has(key)).sort()

    expect({ missingInEnglish, missingInChinese }).toEqual({ missingInEnglish: [], missingInChinese: [] })
  })

  it('includes the complete user-account locale contract in both languages', () => {
    const requiredKeys = [
      'userAccounts.title',
      'userAccounts.importAccounts',
      'userAccounts.proxyPool',
      'userAccounts.shareValidationTitle',
      'userAccounts.importCompletedWithIssues',
    ]
    const englishKeys = new Set(collectLeafKeys(en))
    const chineseKeys = new Set(collectLeafKeys(zh))

    for (const key of requiredKeys) {
      expect(englishKeys.has(key), 'English locale is missing ' + key).toBe(true)
      expect(chineseKeys.has(key), 'Chinese locale is missing ' + key).toBe(true)
    }
  })
})
