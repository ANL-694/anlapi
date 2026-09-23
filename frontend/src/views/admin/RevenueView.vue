<template>
  <AppLayout>
    <UiPage width="wide">
      <div class="flex flex-wrap items-end gap-3 border-y border-[var(--app-border)] py-4">
        <label class="block">
          <span class="input-label">{{ t('admin.revenue.filters.startDate') }}</span>
          <input v-model="startDate" type="date" class="input h-10" @change="loadSummary" />
        </label>
        <label class="block">
          <span class="input-label">{{ t('admin.revenue.filters.endDate') }}</span>
          <input v-model="endDate" type="date" class="input h-10" @change="loadSummary" />
        </label>
        <label class="block min-w-28">
          <span class="input-label">{{ t('admin.revenue.filters.granularity') }}</span>
          <select v-model="granularity" class="input h-10" @change="loadSummary">
            <option value="day">{{ t('admin.revenue.filters.day') }}</option>
            <option value="hour">{{ t('admin.revenue.filters.hour') }}</option>
          </select>
        </label>
        <div class="ml-auto flex items-center gap-2">
          <UiIconButton :label="t('common.refresh')" :disabled="loading" @click="loadSummary">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </UiIconButton>
        </div>
      </div>

      <div v-if="loading && !summary" class="flex items-center justify-center py-16">
        <LoadingSpinner />
      </div>

      <div v-else-if="error" class="border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
        {{ error }}
      </div>

      <template v-else-if="summary">
        <UiMetricStrip :style="{ '--metric-columns': 5 }">
          <UiMetric :label="t('admin.revenue.metrics.netPaid')" :value="formatMoney(summary.cash.net_paid_amount)" :detail="t('admin.revenue.metrics.paidOrders', { count: formatInteger(summary.cash.paid_order_count) })" tone="success" />
          <UiMetric :label="t('admin.revenue.metrics.consumedRevenue')" :value="formatMoney(summary.usage.consumed_revenue)" :detail="t('admin.revenue.metrics.requests', { count: formatInteger(summary.usage.requests) })" />
          <UiMetric :label="t('admin.revenue.metrics.accountCost')" :value="formatMoney(summary.usage.account_cost)" :detail="t('admin.revenue.metrics.tokens', { count: formatInteger(summary.usage.total_tokens) })" />
          <UiMetric :label="t('admin.revenue.metrics.netProfit')" :value="formatMoney(summary.profit.estimated_net_profit)" :detail="t('admin.revenue.metrics.grossProfit', { value: formatMoney(summary.profit.usage_gross_profit) })" :tone="summary.profit.estimated_net_profit >= 0 ? 'success' : 'danger'" />
          <UiMetric :label="t('admin.revenue.metrics.ownerCredit')" :value="formatMoney(summary.adjustments.share_owner_credit)" :detail="t('admin.revenue.metrics.platformFee', { value: formatMoney(summary.adjustments.share_platform_fee) })" />
        </UiMetricStrip>

        <UiSection surface="panel" :title="t('admin.revenue.trend.title')" :description="`${summary.start_date} - ${summary.end_date}`">
          <div v-if="summary.trend.length" class="overflow-x-auto">
            <table class="w-full min-w-[720px] text-sm">
              <thead>
                <tr class="border-b border-[var(--app-border)] text-left text-xs text-[var(--app-muted)]">
                  <th class="px-3 py-2 font-medium">{{ t('admin.revenue.trend.date') }}</th>
                  <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.metrics.netPaid') }}</th>
                  <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.metrics.consumedRevenue') }}</th>
                  <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.metrics.accountCost') }}</th>
                  <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.metrics.netProfit') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="point in summary.trend" :key="point.date" class="border-b border-[var(--app-border)] last:border-0">
                  <td class="px-3 py-2 text-[var(--app-text)]">{{ point.date }}</td>
                  <td class="px-3 py-2 text-right">{{ formatMoney(point.net_paid_amount) }}</td>
                  <td class="px-3 py-2 text-right">{{ formatMoney(point.consumed_revenue) }}</td>
                  <td class="px-3 py-2 text-right">{{ formatMoney(point.account_cost) }}</td>
                  <td class="px-3 py-2 text-right" :class="point.estimated_net_profit >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">{{ formatMoney(point.estimated_net_profit) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="py-8 text-center text-sm text-[var(--app-muted)]">{{ t('admin.revenue.empty') }}</p>
        </UiSection>

        <div class="grid gap-6 xl:grid-cols-2">
          <RevenueBreakdown :title="t('admin.revenue.breakdowns.users')" :items="summary.top_users" />
          <RevenueBreakdown :title="t('admin.revenue.breakdowns.accounts')" :items="summary.top_accounts" />
        </div>
      </template>
    </UiPage>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import RevenueBreakdown from '@/components/admin/revenue/RevenueBreakdown.vue'
import { UiIconButton, UiMetric, UiMetricStrip, UiPage, UiSection } from '@/ui'
import { revenueAPI, type RevenueGranularity, type RevenueSummary } from '@/api/admin/revenue'
import { formatCostFixed, formatNumberLocaleString } from '@/utils/format'

const { t } = useI18n()
const today = new Date()
const start = new Date(today)
start.setDate(today.getDate() - 6)
const dateParam = (value: Date) => {
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
const startDate = ref(dateParam(start))
const endDate = ref(dateParam(today))
const granularity = ref<RevenueGranularity>('day')
const summary = ref<RevenueSummary | null>(null)
const loading = ref(false)
const error = ref('')
let requestID = 0

const formatMoney = (value: number) => `$${formatCostFixed(value)}`
const formatInteger = (value: number) => formatNumberLocaleString(value)

async function loadSummary() {
  if (!startDate.value || !endDate.value || startDate.value > endDate.value) {
    error.value = t('admin.revenue.invalidDateRange')
    return
  }
  const currentRequest = ++requestID
  loading.value = true
  error.value = ''
  try {
    const response = await revenueAPI.getSummary({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      top_limit: 10
    })
    if (currentRequest === requestID) summary.value = response.data
  } catch {
    if (currentRequest === requestID) error.value = t('admin.revenue.loadFailed')
  } finally {
    if (currentRequest === requestID) loading.value = false
  }
}

onMounted(loadSummary)
</script>
