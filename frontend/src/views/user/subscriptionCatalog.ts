import type { SubscriptionPlan } from '@/types/payment'

/**
 * The checkout endpoint already filters `for_sale` plans on the server. The
 * client still needs a defensive normalizer because older gateways may omit
 * that field, return duplicate rows after a join, or include malformed data.
 */
export function normalizeSubscriptionPlans(
  plans: readonly SubscriptionPlan[] | null | undefined,
): SubscriptionPlan[] {
  if (!Array.isArray(plans)) return []

  const candidates = plans
    .filter((plan): plan is SubscriptionPlan => Boolean(plan))
    .filter((plan) => plan.for_sale !== false)
    .filter((plan) => Number.isInteger(plan.id) && plan.id > 0)
    .filter((plan) => Number.isInteger(plan.group_id) && plan.group_id > 0)
    .filter((plan) => typeof plan.name === 'string' && plan.name.trim().length > 0)
    .filter((plan) => Number.isFinite(plan.price) && plan.price > 0)
    .filter((plan) => Number.isInteger(plan.validity_days) && plan.validity_days > 0)
    .map((plan) => ({
      ...plan,
      name: plan.name.trim(),
      description: typeof plan.description === 'string' ? plan.description.trim() : '',
      features: Array.isArray(plan.features)
        ? plan.features.filter((feature): feature is string => typeof feature === 'string' && feature.trim().length > 0)
        : [],
    }))
    .sort(comparePlans)

  const seen = new Set<string>()
  return candidates.filter((plan) => {
    const key = subscriptionPlanDisplayKey(plan)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export interface SubscriptionPlanGroup {
  key: string
  groupId: number
  name: string
  platform: string
  plans: SubscriptionPlan[]
}

/** Keep multiple valid plans, but make their group boundary visible. */
export function groupSubscriptionPlans(
  plans: readonly SubscriptionPlan[],
): SubscriptionPlanGroup[] {
  const groups = new Map<number, SubscriptionPlanGroup>()

  for (const plan of plans) {
    let group = groups.get(plan.group_id)
    if (!group) {
      group = {
        key: `subscription-group-${plan.group_id}`,
        groupId: plan.group_id,
        name: plan.group_name?.trim() || plan.group_platform?.trim() || `Group ${plan.group_id}`,
        platform: plan.group_platform?.trim() || '',
        plans: [],
      }
      groups.set(plan.group_id, group)
    }
    group.plans.push(plan)
  }

  return [...groups.values()]
}

function comparePlans(a: SubscriptionPlan, b: SubscriptionPlan): number {
  const sortOrder = (a.sort_order ?? 0) - (b.sort_order ?? 0)
  if (sortOrder !== 0) return sortOrder

  const groupOrder = a.group_id - b.group_id
  if (groupOrder !== 0) return groupOrder

  const priceOrder = a.price - b.price
  if (priceOrder !== 0) return priceOrder

  return a.id - b.id
}

/**
 * Use the user-visible contract rather than the database ID for deduplication.
 * A join or legacy migration can create multiple IDs for the same displayed
 * product; materially different price, term, or features remain separate.
 */
function subscriptionPlanDisplayKey(plan: SubscriptionPlan): string {
  return JSON.stringify([
    // The database group ID is an internal identity. Legacy syncs can create
    // two groups for the same user-visible product, so it must not prevent
    // deduplication. Keep the fields that materially change the entitlement
    // or the displayed purchase offer in the key instead.
    (plan.group_platform || '').trim().toLocaleLowerCase(),
    (plan.group_name || '').trim().toLocaleLowerCase(),
    plan.name.trim().toLocaleLowerCase(),
    plan.description.trim(),
    plan.price,
    plan.original_price ?? null,
    (plan.currency || '').trim().toLocaleUpperCase(),
    plan.validity_days,
    plan.validity_unit.trim().toLocaleLowerCase(),
    plan.product_name?.trim() || '',
    plan.features,
    plan.rate_multiplier ?? 1,
    plan.peak_rate_enabled ?? false,
    plan.peak_start || '',
    plan.peak_end || '',
    plan.peak_rate_multiplier ?? 1,
    plan.daily_limit_usd ?? null,
    plan.weekly_limit_usd ?? null,
    plan.monthly_limit_usd ?? null,
    plan.supported_model_scopes || [],
  ])
}
