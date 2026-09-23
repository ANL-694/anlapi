import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export type AccountSharePolicyScope = 'global' | 'platform' | 'group' | 'account'

export interface AccountSharePolicy {
  id: number
  scope_type: AccountSharePolicyScope | string
  scope_id?: number | null
  platform?: string | null
  owner_share_ratio: number
  invite_share_ratio: number
  version: number
  enabled: boolean
  effective_at: string
  created_by_admin_id?: number | null
  created_at: string
  updated_at: string
}

export interface AccountSharePolicyInput {
  scope_type: AccountSharePolicyScope
  scope_id?: number | null
  platform?: string | null
  owner_share_ratio: number
  invite_share_ratio?: number
  enabled?: boolean
  effective_at?: string
}

export interface AccountShareSettlement {
  id: number
  usage_log_id?: number | null
  request_id: string
  api_key_id: number
  consumer_user_id: number
  consumer_email: string
  owner_user_id: number
  owner_email: string
  inviter_user_id?: number | null
  inviter_email?: string | null
  account_id: number
  account_name: string
  platform: string
  group_id?: number | null
  group_name?: string | null
  model?: string | null
  policy_id?: number | null
  policy_version: number
  share_mode_snapshot: string
  share_status_snapshot: string
  consumer_charge: number
  account_cost: number
  owner_share_ratio: number
  owner_credit: number
  invite_share_ratio: number
  invite_credit: number
  platform_share_ratio: number
  platform_fee: number
  invite_bound_at_snapshot?: string | null
  invite_expires_at_snapshot?: string | null
  status: string
  created_at: string
}

export interface AccountShareListParams {
  page?: number
  page_size?: number
  scope_type?: string
  platform?: string
  enabled?: boolean
}

export interface AccountShareSettlementParams {
  page?: number
  page_size?: number
  start_date?: string
  end_date?: string
  search?: string
  status?: string
  timezone?: string
}

export async function listPolicies(params: AccountShareListParams = {}) {
  const { data } = await apiClient.get<PaginatedResponse<AccountSharePolicy>>('/admin/account-share-revenue/share-policies', { params })
  return data
}

export async function createPolicy(payload: AccountSharePolicyInput) {
  const { data } = await apiClient.post<AccountSharePolicy>('/admin/account-share-revenue/share-policies', payload)
  return data
}

export async function updatePolicy(id: number, payload: Partial<AccountSharePolicyInput>) {
  const { data } = await apiClient.put<AccountSharePolicy>(`/admin/account-share-revenue/share-policies/${id}`, payload)
  return data
}

export async function deletePolicy(id: number) {
  const { data } = await apiClient.delete<{ id: number }>(`/admin/account-share-revenue/share-policies/${id}`)
  return data
}

export async function listSettlements(params: AccountShareSettlementParams = {}) {
  const { data } = await apiClient.get<PaginatedResponse<AccountShareSettlement>>('/admin/account-share-revenue/share-settlements', { params })
  return data
}

export const accountShareAPI = {
  listPolicies,
  createPolicy,
  updatePolicy,
  deletePolicy,
  listSettlements,
}

export default accountShareAPI
