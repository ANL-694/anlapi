<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.platformQuotas.title')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div v-if="user" class="space-y-5">
      <div class="flex items-center justify-between gap-4 rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-800">
        <div class="min-w-0">
          <p class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.users.platformQuotas.hint') }}
          </p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadQuotas">
          <Icon name="refresh" size="sm" />
          <span>{{ t('common.refresh') }}</span>
        </button>
      </div>

      <div v-if="loading" class="flex justify-center py-12">
        <LoadingSpinner />
      </div>
      <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[980px] table-fixed text-sm">
          <thead class="bg-gray-50 text-left text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            <tr>
              <th class="w-36 px-4 py-3">{{ t('admin.users.platformQuotas.platform') }}</th>
              <th v-for="window in PLATFORM_QUOTA_WINDOWS" :key="window" class="px-3 py-3">
                {{ t(`admin.users.platformQuotas.windows.${window}`) }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="platform in PLATFORM_QUOTA_PLATFORMS" :key="platform">
              <td class="px-4 py-4 align-top">
                <span :class="['inline-flex rounded-md px-2 py-1 text-xs font-medium', platformBadgeLightClass(platform)]">
                  {{ platformLabel(platform) }}
                </span>
              </td>
              <td v-for="window in PLATFORM_QUOTA_WINDOWS" :key="window" class="px-3 py-3 align-top">
                <div class="space-y-2">
                  <div class="flex items-center gap-2">
                    <div class="relative min-w-0 flex-1">
                      <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400">$</span>
                      <input
                        v-model.number="limits[platform][window]"
                        type="number"
                        min="0"
                        step="any"
                        inputmode="decimal"
                        class="input h-9 pl-7"
                        :placeholder="t('admin.users.platformQuotas.unlimited')"
                      />
                    </div>
                    <button
                      type="button"
                      class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-gray-200 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:hover:bg-dark-700"
                      :disabled="resetting === `${platform}:${window}` || usage(platform, window) <= 0"
                      :title="t('admin.users.platformQuotas.resetWindow')"
                      :aria-label="t('admin.users.platformQuotas.resetWindow')"
                      @click="resetWindow(platform, window)"
                    >
                      <Icon name="refresh" size="sm" />
                    </button>
                  </div>
                  <div class="flex items-center justify-between gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>{{ t('admin.users.platformQuotas.used') }} ${{ formatAmount(usage(platform, window)) }}</span>
                    <span class="truncate">{{ resetAt(platform, window) }}</span>
                  </div>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="loading || saving" @click="saveQuotas">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'
import {
  PLATFORM_QUOTA_PLATFORMS,
  PLATFORM_QUOTA_WINDOWS,
  createEmptyPlatformQuotaLimitSettings,
  platformQuotaRecordsToUpdateInputs,
  type PlatformQuotaLimitSettings,
  type PlatformQuotaPlatform,
  type PlatformQuotaRecord,
  type PlatformQuotaWindow
} from '@/api/platformQuotas'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'success'): void }>()
const { t, locale } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const resetting = ref('')
const records = ref<PlatformQuotaRecord[]>([])
const limits = reactive<PlatformQuotaLimitSettings>(createEmptyPlatformQuotaLimitSettings())

const applyRecords = (nextRecords: PlatformQuotaRecord[]) => {
  records.value = nextRecords
  const empty = createEmptyPlatformQuotaLimitSettings()
  for (const record of nextRecords) {
    empty[record.platform] = {
      daily: record.daily_limit_usd,
      weekly: record.weekly_limit_usd,
      monthly: record.monthly_limit_usd
    }
  }
  Object.assign(limits, empty)
}

const loadQuotas = async () => {
  if (!props.user) return
  loading.value = true
  try {
    const response = await adminAPI.users.getPlatformQuotas(props.user.id)
    applyRecords(response.platform_quotas || [])
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.platformQuotas.loadFailed'))
  } finally {
    loading.value = false
  }
}

const saveQuotas = async () => {
  if (!props.user) return
  saving.value = true
  try {
    const response = await adminAPI.users.updatePlatformQuotas(
      props.user.id,
      platformQuotaRecordsToUpdateInputs(limits)
    )
    applyRecords(response.platform_quotas || [])
    appStore.showSuccess(t('admin.users.platformQuotas.saved'))
    emit('success')
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.platformQuotas.saveFailed'))
  } finally {
    saving.value = false
  }
}

const recordFor = (platform: PlatformQuotaPlatform) =>
  records.value.find((record) => record.platform === platform)
const usage = (platform: PlatformQuotaPlatform, window: PlatformQuotaWindow) =>
  recordFor(platform)?.[`${window}_usage_usd`] ?? 0
const resetAt = (platform: PlatformQuotaPlatform, window: PlatformQuotaWindow) => {
  const value = recordFor(platform)?.[`${window}_window_resets_at`]
  if (!value) return t('admin.users.platformQuotas.notStarted')
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}
const formatAmount = (value: number) => value.toFixed(4).replace(/\.?0+$/, '') || '0'

const resetWindow = async (platform: PlatformQuotaPlatform, window: PlatformQuotaWindow) => {
  if (!props.user) return
  resetting.value = `${platform}:${window}`
  try {
    const response = await adminAPI.users.resetPlatformQuotaWindow(props.user.id, platform, window)
    applyRecords(response.platform_quotas || [])
    appStore.showSuccess(t('admin.users.platformQuotas.resetDone'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.platformQuotas.resetFailed'))
  } finally {
    resetting.value = ''
  }
}

watch(
  () => props.show,
  (show) => {
    if (show) void loadQuotas()
  }
)
</script>
