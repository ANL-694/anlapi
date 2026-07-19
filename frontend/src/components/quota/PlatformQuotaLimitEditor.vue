<template>
  <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
    <table class="w-full min-w-[680px] table-fixed text-sm">
      <thead class="bg-gray-50 text-left text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-400">
        <tr>
          <th class="w-36 px-4 py-3">{{ t('admin.settings.platformQuotas.platform') }}</th>
          <th v-for="window in PLATFORM_QUOTA_WINDOWS" :key="window" class="px-3 py-3">
            {{ t(`admin.settings.platformQuotas.${window}`) }}
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
        <tr v-for="platform in PLATFORM_QUOTA_PLATFORMS" :key="platform">
          <td class="px-4 py-3">
            <span :class="['inline-flex rounded-md px-2 py-1 text-xs font-medium', platformBadgeLightClass(platform)]">
              {{ platformLabel(platform) }}
            </span>
          </td>
          <td v-for="window in PLATFORM_QUOTA_WINDOWS" :key="window" class="px-3 py-2.5">
            <div class="relative">
              <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400">$</span>
              <input
                :value="displayValue(platform, window)"
                type="number"
                min="0"
                step="any"
                inputmode="decimal"
                class="input h-9 pl-7"
                :placeholder="t('admin.settings.platformQuotas.unlimited')"
                @input="updateLimit(platform, window, $event)"
              />
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  PLATFORM_QUOTA_PLATFORMS,
  PLATFORM_QUOTA_WINDOWS,
  normalizePlatformQuotaLimitSettings,
  type PlatformQuotaLimitSettings,
  type PlatformQuotaPlatform,
  type PlatformQuotaWindow
} from '@/api/platformQuotas'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'

const props = defineProps<{ modelValue: PlatformQuotaLimitSettings }>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: PlatformQuotaLimitSettings): void
}>()
const { t } = useI18n()

const displayValue = (platform: PlatformQuotaPlatform, window: PlatformQuotaWindow) =>
  props.modelValue?.[platform]?.[window] ?? ''

const updateLimit = (
  platform: PlatformQuotaPlatform,
  window: PlatformQuotaWindow,
  event: Event
) => {
  const input = event.target as HTMLInputElement
  const raw = input.value.trim()
  const next = normalizePlatformQuotaLimitSettings(props.modelValue)
  next[platform][window] = raw === '' ? null : Math.max(0, Number(raw) || 0)
  emit('update:modelValue', next)
}
</script>
