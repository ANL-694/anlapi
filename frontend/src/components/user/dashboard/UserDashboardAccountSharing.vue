<template>
  <UiSection
    class="dashboard-section"
    surface="panel"
    :title="t('dashboard.accountSharing.title')"
    :description="`${data?.start_date || '-'} - ${data?.end_date || '-'}`"
  >
    <template #actions>
      <span class="text-xs text-[var(--app-muted)]">{{ t('dashboard.accountSharing.settlement') }}</span>
    </template>

    <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50">
      <LoadingSpinner size="md" />
    </div>

    <div v-if="error" class="mb-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>

    <div v-else-if="summary" class="space-y-4">
      <UiMetricStrip :style="{ '--metric-columns': 5 }">
        <UiMetric
          :label="t('dashboard.accountSharing.ownedAccounts')"
          :value="formatNumber(summary.owned_accounts)"
          :detail="`${t('dashboard.accountSharing.publicAccounts')} ${summary.public_accounts} · ${t('dashboard.accountSharing.publicApproved')} ${summary.public_approved_accounts}`"
        />
        <UiMetric
          :label="t('dashboard.accountSharing.selfAccountCost')"
          :value="`$${formatCost(summary.self_account_cost)}`"
          :detail="`${formatNumber(summary.self_requests)} ${t('dashboard.requests')}`"
        />
        <UiMetric
          :label="t('dashboard.accountSharing.externalConsumerCharge')"
          :value="`$${formatCost(summary.external_consumer_charge)}`"
          :detail="`${formatNumber(summary.external_requests)} ${t('dashboard.requests')}`"
        />
        <UiMetric
          :label="t('dashboard.accountSharing.ownerCredit')"
          :value="`$${formatCost(summary.external_owner_credit)}`"
          :detail="`${t('dashboard.accountSharing.platformFee')} $${formatCost(summary.external_platform_fee)}`"
        />
        <UiMetric
          :label="t('dashboard.accountSharing.balanceNetChange')"
          :value="`${summary.balance_net_change >= 0 ? '+' : '-'}$${formatCost(Math.abs(summary.balance_net_change))}`"
          :tone="summary.balance_net_change >= 0 ? 'success' : 'danger'"
          :detail="`${t('dashboard.accountSharing.selfActualCost')} $${formatCost(summary.self_actual_cost)}`"
        />
      </UiMetricStrip>

      <div class="grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-gray-300 lg:grid-cols-4">
        <div v-for="item in statusItems" :key="item.label" class="sharing-status-item">
          <span>{{ item.label }}</span>
          <span :class="item.className" class="font-semibold">{{ item.value }}</span>
        </div>
      </div>

      <div v-if="trend.length" class="border-t border-gray-100 pt-4 dark:border-gray-800">
        <div class="mb-2 flex items-center justify-between gap-3">
          <h4 class="text-xs font-semibold text-gray-500 dark:text-gray-400">
            {{ t('dashboard.accountSharing.trend') }}
          </h4>
          <span class="text-xs text-gray-400 dark:text-gray-500">{{ data?.granularity }}</span>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[34rem] text-xs">
            <thead>
              <tr class="border-b border-gray-100 text-gray-500 dark:border-gray-800 dark:text-gray-400">
                <th class="pb-2 text-left">{{ t('dashboard.accountSharing.period') }}</th>
                <th class="pb-2 text-right">{{ t('dashboard.accountSharing.selfUsage') }}</th>
                <th class="pb-2 text-right">{{ t('dashboard.accountSharing.externalUsage') }}</th>
                <th class="pb-2 text-right">{{ t('dashboard.accountSharing.ownerCredit') }}</th>
                <th class="pb-2 text-right">{{ t('dashboard.accountSharing.netChange') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="point in trend" :key="point.date" class="border-b border-gray-50 last:border-0 dark:border-gray-800/70">
                <td class="py-2 text-left text-gray-600 dark:text-gray-300">{{ point.date }}</td>
                <td class="py-2 text-right text-gray-700 dark:text-gray-300">${{ formatCost(point.self_account_cost) }}</td>
                <td class="py-2 text-right text-primary-600 dark:text-primary-300">${{ formatCost(point.external_consumer_charge) }}</td>
                <td class="py-2 text-right text-accent-600 dark:text-accent-300">${{ formatCost(point.external_owner_credit) }}</td>
                <td class="py-2 text-right font-semibold" :class="point.external_owner_credit - point.self_actual_cost >= 0 ? 'text-accent-600 dark:text-accent-300' : 'text-rose-600 dark:text-rose-300'">
                  {{ point.external_owner_credit - point.self_actual_cost >= 0 ? '+' : '-' }}${{ formatCost(Math.abs(point.external_owner_credit - point.self_actual_cost)) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div>
        <div class="mb-2 flex items-center justify-between gap-3">
          <h4 class="text-xs font-semibold text-gray-500 dark:text-gray-400">
            {{ t('dashboard.accountSharing.ownedAccountBreakdown') }}
          </h4>
          <span class="text-right text-xs text-gray-400 dark:text-gray-500">
            {{ t('dashboard.accountSharing.totalAccountCost') }} ${{ formatCost(summary.total_account_cost) }}
          </span>
        </div>

        <div v-if="accounts.length" class="space-y-3">
          <div class="hidden overflow-x-auto md:block">
            <table class="w-full text-xs">
              <thead>
                <tr class="border-b border-gray-100 text-gray-500 dark:border-gray-700 dark:text-gray-400">
                  <th class="pb-2 text-left">{{ t('dashboard.accountSharing.account') }}</th>
                  <th class="pb-2 text-left">{{ t('dashboard.accountSharing.shareStatus') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.accountSharing.selfUsage') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.accountSharing.externalUsage') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.accountSharing.ownerCredit') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="account in accounts" :key="account.account_id" class="border-b border-gray-50 dark:border-gray-800">
                  <td class="max-w-[180px] py-2">
                    <div class="truncate font-medium text-gray-900 dark:text-white" :title="account.name">{{ account.name }}</div>
                    <div class="text-gray-400 dark:text-gray-500">{{ account.platform }}</div>
                  </td>
                  <td class="py-2">
                    <span class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium" :class="statusClass(account)">
                      {{ statusLabel(account) }}
                    </span>
                  </td>
                  <td class="py-2 text-right text-gray-700 dark:text-gray-300">
                    ${{ formatCost(account.self_account_cost) }}
                    <div class="text-gray-400 dark:text-gray-500">{{ formatNumber(account.self_requests) }}</div>
                  </td>
                  <td class="py-2 text-right text-primary-600 dark:text-primary-300">
                    ${{ formatCost(account.external_consumer_charge) }}
                    <div class="text-gray-400 dark:text-gray-500">{{ formatNumber(account.external_requests) }}</div>
                  </td>
                  <td class="py-2 text-right font-semibold text-accent-600 dark:text-accent-300">
                    ${{ formatCost(account.external_owner_credit) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="space-y-2 md:hidden">
            <div
              v-for="account in accounts"
              :key="account.account_id"
              class="rounded-lg border border-gray-100 bg-white/70 p-3 dark:border-gray-800 dark:bg-dark-800/40"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="account.name">{{ account.name }}</div>
                  <div class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ account.platform }}</div>
                </div>
                <span class="shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium" :class="statusClass(account)">
                  {{ statusLabel(account) }}
                </span>
              </div>
              <div class="mt-3 grid grid-cols-3 gap-2 text-xs">
                <div class="min-w-0 rounded-md bg-gray-50 p-2 dark:bg-dark-700/60">
                  <div class="truncate text-gray-500 dark:text-gray-400">{{ t('dashboard.accountSharing.selfUsage') }}</div>
                  <div class="mt-1 font-semibold text-gray-900 dark:text-white">${{ formatCost(account.self_account_cost) }}</div>
                  <div class="text-gray-400 dark:text-gray-500">{{ formatNumber(account.self_requests) }}</div>
                </div>
                <div class="min-w-0 rounded-md bg-primary-50 p-2 dark:bg-primary-950/20">
                  <div class="truncate text-gray-500 dark:text-gray-400">{{ t('dashboard.accountSharing.externalUsage') }}</div>
                  <div class="mt-1 font-semibold text-primary-600 dark:text-primary-300">${{ formatCost(account.external_consumer_charge) }}</div>
                  <div class="text-gray-400 dark:text-gray-500">{{ formatNumber(account.external_requests) }}</div>
                </div>
                <div class="min-w-0 rounded-md bg-accent-50 p-2 dark:bg-accent-950/20">
                  <div class="truncate text-gray-500 dark:text-gray-400">{{ t('dashboard.accountSharing.ownerCredit') }}</div>
                  <div class="mt-1 font-semibold text-accent-600 dark:text-accent-300">${{ formatCost(account.external_owner_credit) }}</div>
                </div>
              </div>
            </div>
          </div>

          <Pagination
            v-if="hasAccountPagination"
            :total="accountPagination.total"
            :page="page"
            :page-size="pageSize"
            :page-size-options="[5, 10, 20, 50, 100, 1000]"
            :show-jump="true"
            @update:page="emit('update:page', $event)"
            @update:pageSize="emit('update:pageSize', $event)"
          />
        </div>

        <div v-else class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('dashboard.accountSharing.empty') }}
        </div>
      </div>
    </div>
  </UiSection>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import { UiMetric, UiMetricStrip, UiSection } from '@/ui'
import type { AccountSharingAccountStat, AccountSharingDashboardStats } from '@/api/usage'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber } from '@/utils/format'

const props = withDefaults(defineProps<{
  data: AccountSharingDashboardStats | null
  loading: boolean
  error?: string
  page?: number
  pageSize?: number
}>(), {
  page: 1,
  pageSize: 20
})

const emit = defineEmits<{
  (event: 'update:page', page: number): void
  (event: 'update:pageSize', pageSize: number): void
}>()

const { t } = useI18n()
const summary = computed(() => props.data?.summary ?? null)
const accounts = computed(() => props.data?.accounts ?? [])
const trend = computed(() => props.data?.trend ?? [])
const accountPagination = computed(() => props.data?.accounts_pagination ?? {
  total: accounts.value.length,
  page: props.page,
  page_size: props.pageSize,
  pages: 1
})
const hasAccountPagination = computed(() => accountPagination.value.total > accountPagination.value.page_size)

const statusItems = computed(() => {
  if (!summary.value) return []
  return [
    { label: t('dashboard.accountSharing.privateMode'), value: summary.value.private_accounts, className: 'text-gray-900 dark:text-white' },
    { label: t('dashboard.accountSharing.publicPending'), value: summary.value.public_pending_accounts, className: 'text-primary-600 dark:text-primary-300' },
    { label: t('dashboard.accountSharing.publicApproved'), value: summary.value.public_approved_accounts, className: 'text-accent-600 dark:text-accent-300' },
    { label: t('dashboard.accountSharing.publicSuspended'), value: summary.value.public_suspended_accounts, className: 'text-rose-600 dark:text-rose-400' }
  ]
})

function statusLabel(account: AccountSharingAccountStat): string {
  if (account.share_mode === 'private') return t('dashboard.accountSharing.privateMode')
  if (account.share_status === 'approved') return t('dashboard.accountSharing.publicApproved')
  if (account.share_status === 'suspended') return t('dashboard.accountSharing.publicSuspended')
  return t('dashboard.accountSharing.publicPending')
}

function statusClass(account: AccountSharingAccountStat): string {
  if (account.share_mode === 'private') return 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300'
  if (account.share_status === 'approved') return 'bg-accent-100 text-accent-700 dark:bg-accent-900/30 dark:text-accent-300'
  if (account.share_status === 'suspended') return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
  return 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
}
</script>
