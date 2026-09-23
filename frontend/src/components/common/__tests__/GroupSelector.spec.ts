import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GroupSelector from '../GroupSelector.vue'

const authState = { isSimpleMode: false }

vi.mock('@/stores', () => ({ useAuthStore: () => authState }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const groups = [
  { id: 1, name: 'Basic', platform: 'anthropic', status: 'active' },
  { id: 2, name: 'Composite', platform: 'composite', status: 'active' },
  { id: 3, name: 'Private other', platform: 'anthropic', status: 'active', scope: 'user_private', owner_user_id: 8 },
  { id: 4, name: 'Private mine', platform: 'anthropic', status: 'active', scope: 'user_private', owner_user_id: 7 }
] as any

const mountSelector = (modelValue: number[] = []) => mount(GroupSelector, {
  props: { modelValue, groups },
  global: { stubs: { GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' }, Icon: true } }
})

describe('GroupSelector simple-mode binding policy', () => {
  beforeEach(() => { authState.isSimpleMode = false })

  it('hides composite groups in simple mode and preserves basic groups', () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector()
    expect(wrapper.text()).toContain('Basic')
    expect(wrapper.text()).not.toContain('Composite')
  })

  it('keeps composite groups available in advanced mode', () => {
    const wrapper = mountSelector()
    expect(wrapper.text()).toContain('Composite')
  })

  it('cleans hidden historical composite IDs while preserving visible selections', () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector([1, 2])
    expect(wrapper.emitted('update:modelValue')).toEqual([[[1]]])
  })

  it('limits public account forms to public groups', () => {
    const wrapper = mount(GroupSelector, {
      props: { modelValue: [], groups, groupScope: 'public' },
      global: { stubs: { GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' }, Icon: true } }
    })
    expect(wrapper.text()).toContain('Basic')
    expect(wrapper.text()).toContain('Composite')
    expect(wrapper.text()).not.toContain('Private other')
    expect(wrapper.text()).not.toContain('Private mine')
  })

  it('shows only the selected user private groups plus public groups for owned accounts', () => {
    const wrapper = mount(GroupSelector, {
      props: { modelValue: [], groups, groupScope: 'user_private', ownerUserId: 7 },
      global: { stubs: { GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' }, Icon: true } }
    })
    expect(wrapper.text()).toContain('Basic')
    expect(wrapper.text()).toContain('Composite')
    expect(wrapper.text()).toContain('Private mine')
    expect(wrapper.text()).not.toContain('Private other')
  })

  it('keeps an already-bound private group visible for safe editing', () => {
    const wrapper = mount(GroupSelector, {
      props: { modelValue: [3], groups, groupScope: 'user_private', ownerUserId: 7 },
      global: { stubs: { GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' }, Icon: true } }
    })
    expect(wrapper.text()).toContain('Private other')
  })
})
