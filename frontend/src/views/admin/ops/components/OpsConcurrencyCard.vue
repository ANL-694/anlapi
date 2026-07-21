<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { opsAPI, type OpsUserConcurrencyStatsResponse } from '@/api/admin/ops'

interface Props {
  refreshToken: number
}

const props = defineProps<Props>()

const { t } = useI18n()
const loading = ref(false)
const errorMessage = ref('')
const userConcurrency = ref<OpsUserConcurrencyStatsResponse | null>(null)

interface UserRow {
  key: string
  userEmail: string
  username: string
  current: number
  max: number
}

function safeNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

const realtimeEnabled = computed(() => userConcurrency.value?.enabled ?? true)

const rows = computed<UserRow[]>(() => {
  const stats = userConcurrency.value?.user || {}

  return Object.entries(stats)
    .map(([userID, item]) => ({
      key: userID,
      userEmail: item.user_email || `User ${userID}`,
      username: item.username || '',
      current: safeNumber(item.current_in_use),
      max: safeNumber(item.max_capacity)
    }))
    .sort((a, b) => b.current - a.current || a.key.localeCompare(b.key))
})

function resolveErrorMessage(error: unknown): string {
  const detail = (error as { response?: { data?: { detail?: unknown } } })?.response?.data?.detail
  return typeof detail === 'string' && detail ? detail : t('admin.ops.concurrency.loadFailed')
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    userConcurrency.value = await opsAPI.getUserConcurrencyStats()
  } catch (error: unknown) {
    console.error('[OpsConcurrencyCard] Failed to load user concurrency', error)
    errorMessage.value = resolveErrorMessage(error)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.refreshToken,
  () => {
    if (realtimeEnabled.value) loadData()
  }
)

watch(
  () => realtimeEnabled.value,
  enabled => {
    if (enabled) loadData()
  },
  { immediate: true }
)
</script>

<template>
  <div class="ops-panel flex h-full flex-col">
    <div class="mb-4 flex shrink-0 items-center justify-between gap-3">
      <h3 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
        <svg class="h-4 w-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
        {{ t('admin.ops.concurrency.title') }}
      </h3>
      <button
        class="flex items-center gap-1 rounded-lg bg-gray-100 px-2 py-1 text-[11px] font-semibold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
        :disabled="loading"
        :title="t('common.refresh')"
        @click="loadData"
      >
        <svg class="h-3 w-3" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
      </button>
    </div>

    <div v-if="errorMessage" class="mb-3 shrink-0 rounded-lg bg-red-50 p-2.5 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div
      v-if="!realtimeEnabled"
      class="flex flex-1 items-center justify-center rounded-lg border border-dashed border-gray-200 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
    >
      {{ t('admin.ops.concurrency.disabledHint') }}
    </div>

    <div v-else class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
      <div class="grid shrink-0 grid-cols-[minmax(0,1fr)_5rem_5rem] items-center gap-2 border-b border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900">
        <span class="text-[10px] font-bold uppercase text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.concurrency.byUser') }}
        </span>
        <span class="text-center text-[10px] font-bold text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.concurrency.currentInUse') }}
        </span>
        <span class="text-center text-[10px] font-bold text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.concurrency.userLimit') }}
        </span>
      </div>

      <div v-if="rows.length === 0" class="flex flex-1 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.concurrency.empty') }}
      </div>

      <div v-else class="custom-scrollbar max-h-[360px] flex-1 space-y-2 overflow-y-auto p-3">
        <div v-for="row in rows" :key="row.key" class="grid grid-cols-[minmax(0,1fr)_5rem_5rem] items-center gap-2 rounded-lg bg-gray-50 p-2.5 dark:bg-dark-900">
          <div class="flex min-w-0 items-center gap-1.5">
            <span class="truncate text-[11px] font-bold text-gray-900 dark:text-white" :title="row.username || row.userEmail">
              {{ row.username || row.userEmail }}
            </span>
            <span v-if="row.username" class="truncate text-[10px] text-gray-400 dark:text-gray-500" :title="row.userEmail">
              {{ row.userEmail }}
            </span>
          </div>
          <span class="text-center font-mono text-xs font-bold text-gray-900 dark:text-white">{{ row.current }}</span>
          <span class="text-center font-mono text-xs font-bold text-gray-900 dark:text-white">{{ row.max }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.custom-scrollbar {
  scrollbar-width: thin;
  scrollbar-color: rgba(156, 163, 175, 0.3) transparent;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background-color: rgba(156, 163, 175, 0.3);
  border-radius: 3px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background-color: rgba(156, 163, 175, 0.5);
}
</style>
