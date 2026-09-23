import { describe, expect, it } from 'vitest'

import { maskApiKey } from '@/utils/maskApiKey'

describe('maskApiKey', () => {
  it('masks long and short keys', () => {
    expect(maskApiKey('fixture-test-1234567890-example')).toBe('fixtur...mple')
    expect(maskApiKey('short-key')).toBe('shor***')
    expect(maskApiKey('abc')).toBe('abc***')
  })

  it('keeps an already masked value unchanged', () => {
    expect(maskApiKey('sk-tes...mple')).toBe('sk-tes...mple')
    expect(maskApiKey('shor***')).toBe('shor***')
  })
})
