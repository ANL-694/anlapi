import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import type { Account } from '@/types'

vi.mock('@/api/admin/accounts', () => ({ queryOpenAIQuota: vi.fn(), resetOpenAIQuota: vi.fn() }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const makeAccount = (parentAccountID: number | null): Account =>
  ({
    id: 1,
    name: 'account',
    platform: 'openai',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    concurrency: 3,
    priority: 50,
    parent_account_id: parentAccountID
  }) as Account

describe('OpenAIQuotaResetCell spark shadow', () => {
  it('disables reset on a shadow account', () => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount(42) } })
    const reset = wrapper.findAll('button')[1]
    expect(reset.attributes('disabled')).toBeDefined()
    expect(reset.attributes('title')).toBe('admin.accounts.openaiQuotaReset.resetTooltipShadow')
  })

  it('keeps the normal pre-query hint for a parent account', () => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount(null) } })
    expect(wrapper.findAll('button')[1].attributes('title')).toBe(
      'admin.accounts.openaiQuotaReset.resetTooltipNeedQuery'
    )
  })
})
