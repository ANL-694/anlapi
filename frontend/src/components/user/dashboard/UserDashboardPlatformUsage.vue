<template>
  <section class="card p-5">
    <header class="mb-4 flex items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('dashboard.platformUsageTitle') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('dashboard.platformUsageHint') }}</p>
      </div>
      <span class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
        {{ t('dashboard.platformCount', { count: rows.length }) }}
      </span>
    </header>

    <div v-if="rows.length" class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      <article v-for="item in rows" :key="item.platform" class="border border-gray-200 p-4 dark:border-dark-600">
        <div class="flex items-center justify-between gap-3">
          <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ platformLabel(item.platform) }}</h3>
          <span class="font-mono text-sm text-emerald-600 dark:text-emerald-400">${{ formatCost(todayActualCost(item)) }}</span>
        </div>
        <dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
          <div>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.todayRequests') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatNumber(todayRequests(item)) }}</dd>
          </div>
          <div>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.todayTokens') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatTokens(todayTokens(item)) }}</dd>
          </div>
          <div v-if="hasDetailedRows">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.input') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatTokens(inputTokens(item)) }}</dd>
          </div>
          <div v-if="hasDetailedRows">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.output') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatTokens(outputTokens(item)) }}</dd>
          </div>
          <div v-if="hasDetailedRows">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.cacheCreation') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatTokens(cacheCreationTokens(item)) }}</dd>
          </div>
          <div v-if="hasDetailedRows">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.cacheRead') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatTokens(cacheReadTokens(item)) }}</dd>
          </div>
          <div v-if="!hasDetailedRows">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.requests') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatNumber(totalRequests(item)) }}</dd>
          </div>
          <div v-if="!hasDetailedRows">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('dashboard.tokens') }}</dt>
            <dd class="mt-0.5 font-mono text-gray-900 dark:text-white">{{ formatTokens(totalTokens(item)) }}</dd>
          </div>
        </dl>
        <div class="mt-3 flex items-center justify-between border-t border-gray-100 pt-3 text-xs dark:border-dark-700">
          <span class="text-gray-500 dark:text-gray-400">{{ hasDetailedRows ? t('dashboard.standardCost') : t('dashboard.totalCost') }}</span>
          <span class="font-mono font-medium text-gray-900 dark:text-white">${{ formatCost(standardCost(item)) }}</span>
        </div>
        <div v-if="hasDetailedRows" class="mt-1 flex items-center justify-between text-xs">
          <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.actual') }}</span>
          <span class="font-mono font-medium text-emerald-600 dark:text-emerald-400">${{ formatCost(actualCost(item)) }}</span>
        </div>
      </article>
    </div>
    <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('dashboard.platformBreakdownEmpty') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlatformDashboardStats, UserDashboardPlatformUsage, UserDashboardStats } from '@/api/usage'

const props = defineProps<{ stats: UserDashboardStats }>()
const { t } = useI18n()

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok'
}

type PlatformUsageRow = PlatformDashboardStats | UserDashboardPlatformUsage

const hasDetailedRows = computed(() => (props.stats.today_platforms?.length ?? 0) > 0)
const rows = computed<PlatformUsageRow[]>(() => {
  if (hasDetailedRows.value) {
    return [...(props.stats.today_platforms ?? [])].sort((a, b) => b.total_tokens - a.total_tokens)
  }
  return [...(props.stats.by_platform ?? [])].sort((a, b) => b.today_actual_cost - a.today_actual_cost)
})

const todayRequests = (item: PlatformUsageRow) => 'requests' in item ? item.requests : item.today_requests
const todayTokens = (item: PlatformUsageRow) => 'requests' in item ? item.total_tokens : item.today_tokens
const todayActualCost = (item: PlatformUsageRow) => 'requests' in item ? item.actual_cost : item.today_actual_cost
const inputTokens = (item: PlatformUsageRow) => 'requests' in item ? item.input_tokens : 0
const outputTokens = (item: PlatformUsageRow) => 'requests' in item ? item.output_tokens : 0
const cacheCreationTokens = (item: PlatformUsageRow) => 'requests' in item ? item.cache_creation_tokens : 0
const cacheReadTokens = (item: PlatformUsageRow) => 'requests' in item ? item.cache_read_tokens : 0
const totalRequests = (item: PlatformUsageRow) => 'requests' in item ? item.requests : item.total_requests
const totalTokens = (item: PlatformUsageRow) => item.total_tokens
const standardCost = (item: PlatformUsageRow) => 'requests' in item ? item.cost : item.total_actual_cost
const actualCost = (item: PlatformUsageRow) => 'requests' in item ? item.actual_cost : item.today_actual_cost

const platformLabel = (platform: string) => PLATFORM_LABELS[platform] ?? platform
const formatNumber = (value: number) => value.toLocaleString()
const formatCost = (value: number) => value.toFixed(4)
const formatTokens = (value: number) => {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`
  return value.toString()
}
</script>
