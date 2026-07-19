import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('security settings locale copy', () => {
  it('exposes step-up and session binding settings in both locales', () => {
    expect(zh.admin.settings.security.stepUp).toContain('二次验证')
    expect(zh.admin.settings.security.sessionBindingHint).toContain('默认关闭')
    expect(en.admin.settings.security.stepUp).toContain('Step-up 2FA')
    expect(en.admin.settings.security.sessionBindingHint).toContain('disabled by default')
  })

  it('keeps generic step-up dialog messages available', () => {
    expect(zh.stepUp.adminApiKeyForbidden).toContain('管理 API Key')
    expect(en.stepUp.notEnabled).toContain('enable TOTP')
  })
})
