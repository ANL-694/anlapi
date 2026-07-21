import { describe, expect, it } from 'vitest'

import { currencySymbol, normalizePaymentCurrency } from '../currency'

describe('payment currency', () => {
  it('keeps the ANL CNY default for legacy plans', () => {
    expect(normalizePaymentCurrency('')).toBe('CNY')
    expect(currencySymbol('', 'zh-CN')).toBe('¥')
  })

  it('uses the configured currency instead of a hardcoded USD symbol', () => {
    expect(currencySymbol('CNY', 'zh-CN')).toBe('¥')
    expect(currencySymbol('USD', 'en-US')).toBe('$')
    expect(currencySymbol('EUR', 'de-DE')).toBe('€')
  })
})
