import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountActionMenu from '../AccountActionMenu.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const makeAccount = (overrides: Partial<Account>): Account =>
  ({
    id: 1,
    name: 'account',
    platform: 'openai',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    concurrency: 3,
    priority: 50,
    ...overrides
  }) as Account

const mountMenu = (account: Account) =>
  mount(AccountActionMenu, {
    props: { show: true, account, position: { top: 100, left: 100 } },
    attachTo: document.body
  })

describe('AccountActionMenu spark shadow', () => {
  it('shows creation only for an OpenAI OAuth parent', () => {
    const parent = mountMenu(makeAccount({ parent_account_id: null }))
    expect(document.body.textContent).toContain('admin.accounts.createSparkShadow')
    parent.unmount()

    const shadow = mountMenu(makeAccount({ parent_account_id: 42 }))
    expect(document.body.textContent).not.toContain('admin.accounts.createSparkShadow')
    expect(document.body.textContent).not.toContain('admin.accounts.reAuthorize')
    expect(document.body.textContent).not.toContain('admin.accounts.refreshToken')
    expect(document.body.textContent).not.toContain('admin.accounts.setPrivacy')
    shadow.unmount()
  })

  it('emits create-spark-shadow with the parent account', async () => {
    const account = makeAccount({ parent_account_id: null })
    const wrapper = mountMenu(account)
    const button = Array.from(document.body.querySelectorAll('button')).find((item) =>
      item.textContent?.includes('admin.accounts.createSparkShadow')
    )
    expect(button).toBeDefined()
    button!.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('create-spark-shadow')?.[0]?.[0]).toMatchObject({ id: account.id })
    wrapper.unmount()
  })
})
