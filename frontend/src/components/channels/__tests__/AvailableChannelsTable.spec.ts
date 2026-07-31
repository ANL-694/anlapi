import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AvailableChannelsTable from '../AvailableChannelsTable.vue'
import type { UserAvailableChannel } from '@/api/channels'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AvailableChannelsTable.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AvailableChannelsTable scrolling', () => {
  it('keeps a long channel list in a bounded vertical scroll container', () => {
    expect(componentSource).toContain('class="channel-list channel-list--scrollable"')
    expect(componentSource).toMatch(/\.channel-list--scrollable\s*\{[\s\S]*overflow-y:\s*auto;/)
  })
})

const rows: UserAvailableChannel[] = [
  {
    name: 'Primary channel',
    description: 'Fast and reliable access',
    platforms: [
      {
        platform: 'anthropic',
        groups: [
          {
            id: 1,
            name: 'Exclusive Pro',
            platform: 'anthropic',
            subscription_type: 'standard',
            rate_multiplier: 1.2,
            peak_rate_enabled: true,
            peak_start: '08:00',
            peak_end: '10:00',
            peak_rate_multiplier: 1.5,
            is_exclusive: true,
          },
          {
            id: 2,
            name: 'Public',
            platform: 'anthropic',
            subscription_type: 'standard',
            rate_multiplier: 1,
            peak_rate_enabled: false,
            peak_start: '',
            peak_end: '',
            peak_rate_multiplier: 1,
            is_exclusive: false,
          },
        ],
        supported_models: [{ name: 'claude-test', platform: 'anthropic', pricing: null }],
      },
    ],
  },
]

const baseProps = {
  columns: {
    name: 'Channel',
    description: 'Description',
    platform: 'Platform',
    groups: 'Groups and rates',
    supportedModels: 'Models and pricing',
  },
  rows,
  loading: false,
  pricingKeyPrefix: 'availableChannels.pricing',
  noPricingLabel: 'No pricing',
  noModelsLabel: 'No models',
  emptyLabel: 'No channels',
  userGroupRates: { 1: 0.8 },
}

function mountTable(props = {}) {
  return mount(AvailableChannelsTable, {
    props: { ...baseProps, ...props },
    global: {
      plugins: [createPinia()],
      stubs: {
        Icon: { props: ['name'], template: '<i :data-icon="name" />' },
        PlatformIcon: { template: '<i data-platform-icon />' },
        GroupBadge: {
          props: ['name', 'rateMultiplier', 'userRateMultiplier'],
          template:
            '<span data-group-badge>{{ name }}:{{ rateMultiplier }}:{{ userRateMultiplier }}</span>',
        },
        SupportedModelChip: {
          props: ['model', 'noPricingLabel'],
          template: '<span data-model-chip>{{ model.name }}:{{ noPricingLabel }}</span>',
        },
      },
    },
  })
}

describe('AvailableChannelsTable adaptive surface', () => {
  it('renders channel panels with groups and model pricing', () => {
    const wrapper = mountTable()

    expect(wrapper.get('.channel-list').classes()).toContain('channel-list--scrollable')
    expect(wrapper.get('.channel-panel').text()).toContain('Primary channel')
    expect(wrapper.get('.channel-panel').text()).toContain('Fast and reliable access')
    expect(wrapper.findAll('.channel-platform-row')).toHaveLength(1)
    expect(wrapper.findAll('[data-group-badge]')).toHaveLength(2)
    expect(wrapper.get('.channel-model-name').text()).toBe('claude-test')
    expect(wrapper.get('.channel-model-price').text()).toBe('availableChannels.noPricing')
  })

  it('adapts the same panel surface for compact breakpoints', () => {
    expect(componentSource).toMatch(/@media \(max-width: 900px\)\s*\{[\s\S]*?\.channel-platform-row\s*\{[\s\S]*?grid-template-columns:\s*1fr;/)
    expect(componentSource).toMatch(/@media \(max-width: 640px\)\s*\{[\s\S]*?\.channel-header\s*\{[\s\S]*?flex-direction:\s*column;/)
    expect(componentSource).toMatch(/\.channel-model-main\s*\{[\s\S]*?grid-template-columns:\s*1fr;/)
  })

  it('keeps placeholders when a platform has no groups or models', () => {
    const wrapper = mountTable({
      rows: [
        {
          name: 'Fallback channel',
          description: '',
          platforms: [{ platform: 'openai', groups: [], supported_models: [] }],
        },
      ],
    })

    expect(wrapper.text()).toContain('Fallback channel')
    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('No models')
    expect(wrapper.findAll('.channel-empty-value')[0].text()).toBe('-')
  })

  it('provides loading and empty states', async () => {
    const wrapper = mountTable({ loading: true, rows: [] })

    expect(wrapper.get('.channel-state [data-icon="refresh"]')).toBeTruthy()

    await wrapper.setProps({ loading: false })

    expect(wrapper.get('.channel-state--empty').text()).toContain('No channels')
  })
})
