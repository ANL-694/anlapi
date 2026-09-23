import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

import UserDashboardHeroActions from '../UserDashboardHeroActions.vue'

describe('UserDashboardHeroActions', () => {
  it('renders the balance, key, and usage destinations', () => {
    const wrapper = mount(UserDashboardHeroActions, {
      props: { balance: 2662.52 },
      global: { stubs: { RouterLink: { template: '<a :href="to"><slot /></a>', props: ['to'] } } }
    })

    expect(wrapper.text()).toContain('$2,662.52')
    expect(wrapper.find('a[href="/purchase"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/keys"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/usage"]').exists()).toBe(true)
  })
})
