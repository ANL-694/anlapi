import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const SearchInputStub = {
  props: ['modelValue'],
  template: '<input :value="modelValue" />'
}

const SelectStub = {
  props: ['modelValue', 'options'],
  template: '<select :value="modelValue"><option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option></select>'
}

function mountFilters(type = '') {
  return mount(AccountTableFilters, {
    props: {
      searchQuery: '',
      filters: {
        platform: '',
        type,
        status: '',
        privacy_mode: '',
        proxy_id: '',
        group: ''
      }
    },
    global: {
      stubs: {
        SearchInput: SearchInputStub,
        Select: SelectStub
      }
    }
  })
}

describe('AccountTableFilters account type views', () => {
  it('uses a first-class OAuth view instead of requiring the type dropdown', async () => {
    const wrapper = mountFilters()

    await wrapper.get('[data-test="account-view-oauth"]').trigger('click')

    expect(wrapper.emitted('update:filters')).toEqual([[
      expect.objectContaining({ type: 'oauth' })
    ]])
    expect(wrapper.emitted('change')).toHaveLength(1)
  })

  it('switches to the API Key upstream view and marks it selected', async () => {
    const wrapper = mountFilters('oauth')

    await wrapper.get('[data-test="account-view-apikey"]').trigger('click')

    expect(wrapper.emitted('update:filters')).toEqual([[
      expect.objectContaining({ type: 'apikey' })
    ]])
    await wrapper.setProps({
      filters: {
        platform: '',
        type: 'apikey',
        status: '',
        privacy_mode: '',
        proxy_id: '',
        group: ''
      }
    })
    expect(wrapper.get('[data-test="account-view-apikey"]').attributes('aria-selected')).toBe('true')
  })

  it('returns a non-standard account type to the all accounts view', async () => {
    const wrapper = mountFilters('setup-token')

    expect(wrapper.get('[data-test="account-view-all"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-test="account-view-all"]').trigger('click')

    expect(wrapper.emitted('update:filters')).toEqual([[
      expect.objectContaining({ type: '' })
    ]])
  })
})
