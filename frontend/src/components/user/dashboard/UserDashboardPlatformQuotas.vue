<template>
  <UiSection class="dashboard-section" surface="panel" :title="t('dashboard.platformQuotasTitle')">
    <template #actions>
      <span class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('dashboard.platformQuotasHint') }}
      </span>
    </template>

    <div v-if="loading" class="flex justify-center py-8">
      <LoadingSpinner />
    </div>
    <div v-else-if="quotas.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('dashboard.platformQuotasEmpty') }}
    </div>
    <div v-else class="quota-list">
      <div v-for="quota in quotas" :key="quota.platform" class="quota-row">
        <div class="flex min-w-0 items-center gap-2">
          <span :class="['inline-flex rounded-md px-2 py-1 text-xs font-medium', platformBadgeLightClass(quota.platform)]">
            {{ platformLabel(quota.platform) }}
          </span>
        </div>
        <div v-for="window in PLATFORM_QUOTA_WINDOWS" :key="window" class="quota-window">
          <div class="quota-window-heading">
            <span>{{ t(`dashboard.platformQuotaWindows.${window}`) }}</span>
            <span>{{ usageText(quota, window) }}</span>
          </div>
          <div class="quota-progress">
            <div
              :class="platformAccentBarClass(quota.platform)"
              :style="{ width: `${progress(quota, window)}%` }"
            />
          </div>
          <p class="quota-reset">{{ resetText(quota, window) }}</p>
        </div>
      </div>
    </div>
  </UiSection>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { UiSection } from '@/ui'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import {
  PLATFORM_QUOTA_WINDOWS,
  type PlatformQuotaRecord,
  type PlatformQuotaWindow
} from '@/api/platformQuotas'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformLabel
} from '@/utils/platformColors'

defineProps<{ quotas: PlatformQuotaRecord[]; loading: boolean }>()
const { t, locale } = useI18n()

const usage = (quota: PlatformQuotaRecord, window: PlatformQuotaWindow) =>
  quota[`${window}_usage_usd`]
const limit = (quota: PlatformQuotaRecord, window: PlatformQuotaWindow) =>
  quota[`${window}_limit_usd`]
const resetAt = (quota: PlatformQuotaRecord, window: PlatformQuotaWindow) =>
  quota[`${window}_window_resets_at`]

const money = (value: number) => `$${value.toFixed(4).replace(/\.?0+$/, '') || '0'}`
const usageText = (quota: PlatformQuotaRecord, window: PlatformQuotaWindow) => {
  const currentLimit = limit(quota, window)
  return currentLimit === null
    ? `${money(usage(quota, window))} / ${t('dashboard.platformQuotaUnlimited')}`
    : `${money(usage(quota, window))} / ${money(currentLimit)}`
}
const progress = (quota: PlatformQuotaRecord, window: PlatformQuotaWindow) => {
  const currentLimit = limit(quota, window)
  if (currentLimit === null || currentLimit <= 0) return 0
  return Math.min(100, Math.max(0, (usage(quota, window) / currentLimit) * 100))
}
const resetText = (quota: PlatformQuotaRecord, window: PlatformQuotaWindow) => {
  const value = resetAt(quota, window)
  if (!value) return t('dashboard.platformQuotaNotStarted')
  return t('dashboard.platformQuotaResetsAt', {
    time: new Intl.DateTimeFormat(locale.value, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    }).format(new Date(value))
  })
}
</script>

<style scoped>
.quota-list {
  display: grid;
  gap: 0.25rem;
}

.quota-row {
  display: grid;
  grid-template-columns: minmax(7.5rem, 0.55fr) repeat(3, minmax(10rem, 1fr));
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 0;
}

.quota-window {
  min-width: 0;
}

.quota-window-heading {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  color: var(--ui-text-secondary);
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
}

.quota-progress {
  height: 0.3rem;
  margin-top: 0.4rem;
  overflow: hidden;
  border-radius: 999px;
  background: var(--ui-surface-hover);
}

.quota-progress > div {
  height: 100%;
  border-radius: inherit;
}

.quota-reset {
  margin-top: 0.35rem;
  overflow: hidden;
  color: var(--ui-text-tertiary);
  font-size: 0.7rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .quota-row {
    grid-template-columns: 1fr;
    gap: 0.75rem;
  }
}
</style>
