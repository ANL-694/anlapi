import { apiClient } from '../client'

export type RevenueGranularity = 'day' | 'hour'

export interface RevenueSummaryParams {
  start_date?: string
  end_date?: string
  granularity?: RevenueGranularity
  top_limit?: number
  user_id?: number
}

export interface RevenueBreakdownItem {
  id?: number
  name: string
  secondary?: string
  requests: number
  total_tokens: number
  consumed_revenue: number
  account_cost: number
  gross_profit: number
  net_profit: number
}

export interface RevenueTrendPoint {
  date: string
  net_paid_amount: number
  consumed_revenue: number
  account_cost: number
  estimated_net_profit: number
}

export interface RevenueSummary {
  generated_at: string
  start_date: string
  end_date: string
  granularity: RevenueGranularity
  cash: {
    net_paid_amount: number
    pending_amount: number
    paid_order_count: number
  }
  usage: {
    requests: number
    total_tokens: number
    consumed_revenue: number
    account_cost: number
  }
  adjustments: {
    share_owner_credit: number
    share_platform_fee: number
  }
  profit: {
    usage_gross_profit: number
    estimated_net_profit: number
  }
  trend: RevenueTrendPoint[]
  top_users: RevenueBreakdownItem[]
  top_accounts: RevenueBreakdownItem[]
}

export const revenueAPI = {
  getSummary(params?: RevenueSummaryParams) {
    return apiClient.get<RevenueSummary>('/admin/revenue/summary', { params })
  }
}

export default revenueAPI
