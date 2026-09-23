<template>
  <section class="card p-5">
    <header class="mb-4 flex items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('dashboard.platformQuota.title') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('dashboard.platformQuota.hint') }}</p>
      </div>
    </header>

    <div v-if="loading" class="flex justify-center py-8"><LoadingSpinner /></div>
    <ErrorState v-else-if="error" :retry="true" @retry="$emit('retry')" />
    <p v-else-if="configuredQuotas.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('dashboard.platformQuota.empty') }}
    </p>
    <div v-else class="space-y-3">
      <article v-for="quota in configuredQuotas" :key="quota.platform" class="border border-gray-200 p-4 dark:border-dark-600">
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ platformLabel(quota.platform) }}</h3>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div v-for="window in windows" :key="window" class="min-w-0">
            <div class="flex items-center justify-between gap-2 text-xs">
              <span class="text-gray-500 dark:text-gray-400">{{ t(`dashboard.platformQuota.${window}`) }}</span>
              <span :class="limit(quota, window) === 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-200'" class="font-mono">
                {{ usageText(quota, window) }}
              </span>
            </div>
            <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
              <div class="h-full rounded-full transition-all" :class="barClass(quota, window)" :style="{ width: `${progress(quota, window)}%` }" />
            </div>
            <p class="mt-1 h-4 truncate text-[11px] text-gray-400 dark:text-gray-500">{{ resetText(quota, window) }}</p>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import type { PlatformQuotaItem, PlatformQuotaWindow } from '@/types'

const props = defineProps<{ quotas: PlatformQuotaItem[] | null; loading: boolean; error?: boolean }>()
defineEmits(['retry'])
const { t, locale } = useI18n()

const windows: PlatformQuotaWindow[] = ['daily', 'weekly', 'monthly']
const platformOrder = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']
const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok'
}

const limit = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => quota[`${window}_limit_usd`]
const usage = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => quota[`${window}_usage_usd`]
const resetAt = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => quota[`${window}_window_resets_at`]
const configuredQuotas = computed(() =>
  [...(props.quotas ?? [])]
    .filter((quota) => windows.some((window) => limit(quota, window) !== null))
    .sort((a, b) => platformOrder.indexOf(a.platform) - platformOrder.indexOf(b.platform))
)

const platformLabel = (platform: string) => PLATFORM_LABELS[platform] ?? platform
const money = (value: number) => value.toFixed(4).replace(/\.?0+$/, '') || '0'
const usageText = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => {
  const currentLimit = limit(quota, window)
  if (currentLimit === null) return t('dashboard.platformQuota.noLimit')
  if (currentLimit === 0) return t('dashboard.platformQuota.disabled')
  return `$${money(usage(quota, window))} / $${money(currentLimit)}`
}
const progress = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => {
  const currentLimit = limit(quota, window)
  if (currentLimit === 0) return 100
  if (currentLimit === null || currentLimit <= 0) return 0
  return Math.min(100, Math.max(0, (usage(quota, window) / currentLimit) * 100))
}
const barClass = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => {
  const value = progress(quota, window)
  if (limit(quota, window) === 0 || value >= 95) return 'bg-red-500'
  if (value >= 75) return 'bg-amber-500'
  return 'bg-emerald-500'
}
const resetText = (quota: PlatformQuotaItem, window: PlatformQuotaWindow) => {
  const value = resetAt(quota, window)
  if (!value) return t('dashboard.platformQuota.notStarted')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return t('dashboard.platformQuota.resetsAt', {
    time: new Intl.DateTimeFormat(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
  })
}
</script>
