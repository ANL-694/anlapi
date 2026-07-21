import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import sub2En from '../locales/en/index'
import zh from '../locales/zh'
import sub2Zh from '../locales/zh/index'

function extractKeys(value: Record<string, unknown>, prefix = ''): string[] {
  const keys: string[] = []

  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof child === 'object' && child !== null && !Array.isArray(child)) {
      keys.push(...extractKeys(child as Record<string, unknown>, path))
    } else {
      keys.push(path)
    }
  }

  return keys
}

describe('Sub2 locale coverage', () => {
  it.each([
    ['zh', sub2Zh, zh],
    ['en', sub2En, en]
  ])('keeps every official %s locale key in the ANL locale', (_locale, official, merged) => {
    const mergedKeys = new Set(extractKeys(merged))
    const missingKeys = extractKeys(official).filter(key => !mergedKeys.has(key))

    expect(missingKeys).toEqual([])
  })

  it('keeps ANL copy ahead of the official defaults', () => {
    expect(zh.home.redesign.hero.title).toBe('一个 Key 接入你的 AI 工具')
    expect(en.home.redesign.hero.title).toBe('One Key for Your AI Tools')
  })

  it('describes the ops card as user concurrency without queue capacity', () => {
    expect(zh.admin.ops.concurrency.title).toBe('用户实时并发')
    expect(zh.admin.ops.concurrency.currentInUse).toBe('当前真实占用')
    expect(zh.admin.ops.concurrency.userLimit).toBe('用户并发上限')
    expect(en.admin.ops.concurrency.title).toBe('User Concurrency')
    expect(en.admin.ops.concurrency.currentInUse).toBe('Current in use')
    expect(en.admin.ops.concurrency.userLimit).toBe('User limit')
  })
})
