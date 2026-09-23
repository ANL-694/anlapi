import { describe, expect, it } from 'vitest'
import type { SubscriptionPlan } from '@/types/payment'
import { groupSubscriptionPlans, normalizeSubscriptionPlans } from '../subscriptionCatalog'

function plan(overrides: Partial<SubscriptionPlan> = {}): SubscriptionPlan {
  return {
    id: 1,
    group_id: 10,
    group_name: 'OpenAI',
    group_platform: 'openai',
    name: 'Monthly',
    description: '  starter  ',
    price: 10,
    original_price: 12,
    validity_days: 30,
    validity_unit: 'day',
    features: ['  fast  ', '', 'tools'],
    for_sale: true,
    sort_order: 1,
    ...overrides,
  }
}

describe('subscriptionCatalog', () => {
  it('removes malformed, explicitly hidden, and duplicate plans without hiding valid plans', () => {
    const result = normalizeSubscriptionPlans([
      plan({ id: 1 }),
      plan({ id: 2, sort_order: 2, name: 'Annual', price: 90 }),
      plan({ id: 7 }),
      plan({ id: 3, for_sale: false, name: 'Hidden' }),
      plan({ id: 4, name: '', price: 1 }),
      plan({ id: 5, price: 0 }),
      plan({ id: 6, group_id: 0 }),
    ])

    expect(result.map(item => item.id)).toEqual([1, 2])
    expect(result[0].description).toBe('starter')
    expect(result[0].features).toEqual(['  fast  ', 'tools'])
  })

  it('deduplicates equivalent displayed products even when legacy rows have different IDs', () => {
    const result = normalizeSubscriptionPlans([
      plan({ id: 20 }),
      plan({ id: 21 }),
      plan({ id: 22, price: 12 }),
      plan({ id: 23, validity_days: 90 }),
    ])

    expect(result.map(item => item.id)).toEqual([20, 23, 22])
  })

  it('deduplicates equivalent products when a legacy sync created a second group ID', () => {
    const result = normalizeSubscriptionPlans([
      plan({ id: 30, group_id: 10 }),
      plan({ id: 31, group_id: 11 }),
      plan({ id: 32, group_id: 11, rate_multiplier: 2 }),
    ])

    expect(result.map(item => item.id)).toEqual([30, 32])
  })

  it('sorts plans deterministically and groups valid plans by provider group', () => {
    const normalized = normalizeSubscriptionPlans([
      plan({ id: 12, group_id: 20, group_name: 'Anthropic', group_platform: 'anthropic', sort_order: 3, price: 12 }),
      plan({ id: 11, group_id: 10, sort_order: 2, price: 11 }),
      plan({ id: 10, group_id: 10, sort_order: 1, price: 10 }),
    ])

    expect(normalized.map(item => item.id)).toEqual([10, 11, 12])
    expect(groupSubscriptionPlans(normalized)).toEqual([
      {
        key: 'subscription-group-10',
        groupId: 10,
        name: 'OpenAI',
        platform: 'openai',
        plans: [normalized[0], normalized[1]],
      },
      {
        key: 'subscription-group-20',
        groupId: 20,
        name: 'Anthropic',
        platform: 'anthropic',
        plans: [normalized[2]],
      },
    ])
  })
})
