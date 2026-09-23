<template>
  <UiSection surface="panel" :title="title">
    <div v-if="items.length" class="overflow-x-auto">
      <table class="w-full min-w-[520px] text-sm">
        <thead>
          <tr class="border-b border-[var(--app-border)] text-left text-xs text-[var(--app-muted)]">
            <th class="px-3 py-2 font-medium">{{ t('admin.revenue.table.name') }}</th>
            <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.table.requests') }}</th>
            <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.table.tokens') }}</th>
            <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.table.revenue') }}</th>
            <th class="px-3 py-2 text-right font-medium">{{ t('admin.revenue.table.net') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in items" :key="item.id ?? item.name" class="border-b border-[var(--app-border)] last:border-0">
            <td class="max-w-52 px-3 py-2">
              <div class="truncate font-medium text-[var(--app-text)]">{{ item.name }}</div>
              <div v-if="item.secondary" class="truncate text-xs text-[var(--app-muted)]">{{ item.secondary }}</div>
            </td>
            <td class="px-3 py-2 text-right">{{ formatNumberLocaleString(item.requests) }}</td>
            <td class="px-3 py-2 text-right">{{ formatNumberLocaleString(item.total_tokens) }}</td>
            <td class="px-3 py-2 text-right">${{ formatCostFixed(item.consumed_revenue) }}</td>
            <td class="px-3 py-2 text-right" :class="item.net_profit >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
              ${{ formatCostFixed(item.net_profit) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else class="py-8 text-center text-sm text-[var(--app-muted)]">{{ t('admin.revenue.empty') }}</p>
  </UiSection>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { UiSection } from '@/ui'
import type { RevenueBreakdownItem } from '@/api/admin/revenue'
import { formatCostFixed, formatNumberLocaleString } from '@/utils/format'

defineProps<{
  title: string
  items: RevenueBreakdownItem[]
}>()

const { t } = useI18n()
</script>
