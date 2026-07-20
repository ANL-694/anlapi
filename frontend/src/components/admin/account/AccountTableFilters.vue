<template>
  <div class="flex flex-col gap-3">
    <div
      class="inline-flex w-fit max-w-full flex-wrap gap-1 rounded-lg bg-[var(--app-surface-muted)] p-1"
      role="tablist"
      :aria-label="t('admin.accounts.accountViewLabel')"
    >
      <button
        v-for="option in accountViewOptions"
        :key="option.value || 'all'"
        :data-test="`account-view-${option.testId}`"
        type="button"
        role="tab"
        :aria-selected="activeAccountView === option.value"
        class="min-w-24 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
        :class="activeAccountView === option.value
          ? 'bg-[var(--app-surface)] text-[var(--app-text)] shadow-sm'
          : 'text-[var(--app-muted)] hover:text-[var(--app-text)]'"
        @click="selectAccountView(option.value)"
      >
        {{ option.label }}
      </button>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <SearchInput
        :model-value="searchQuery"
        :placeholder="t('admin.accounts.searchAccounts')"
        class="w-full sm:w-64"
        @update:model-value="$emit('update:searchQuery', $event)"
        @search="$emit('change')"
      />
      <Select :model-value="filters.platform" class="w-40" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
      <Select :model-value="filters.status" class="w-40" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
      <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
      <Select :model-value="filters.proxy_id" class="w-48" :options="proxyOpts" searchable @update:model-value="updateProxy" @change="$emit('change')" />
      <Select :model-value="filters.group" class="w-40" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'; import { useI18n } from 'vue-i18n'; import Select from '@/components/common/Select.vue'; import SearchInput from '@/components/common/SearchInput.vue'
import type { AdminGroup, Proxy } from '@/types'
const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[]; proxies?: Proxy[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change']); const { t } = useI18n()
type AccountView = '' | 'oauth' | 'apikey'

const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value }) }
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateProxy = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, proxy_id: value || '' }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') }, { value: 'anthropic', label: 'Anthropic' }, { value: 'openai', label: 'OpenAI' }, { value: 'gemini', label: 'Gemini' }, { value: 'antigravity', label: 'Antigravity' }, { value: 'grok', label: 'Grok' }, { value: 'kiro', label: 'Kiro' }, { value: 'custom', label: 'Custom' }])
const activeAccountView = computed<AccountView>(() => {
  const type = String(props.filters.type ?? '')
  return type === 'oauth' || type === 'apikey' ? type : ''
})
const accountViewOptions = computed<Array<{ value: AccountView; label: string; testId: string }>>(() => [
  { value: '', label: t('admin.accounts.accountViews.all'), testId: 'all' },
  { value: 'oauth', label: t('admin.accounts.accountViews.oauth'), testId: 'oauth' },
  { value: 'apikey', label: t('admin.accounts.accountViews.apiKey'), testId: 'apikey' }
])
const selectAccountView = (value: AccountView) => {
  if (String(props.filters.type ?? '') === value) return
  emit('update:filters', { ...props.filters, type: value })
  emit('change')
}
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') }, { value: 'active', label: t('admin.accounts.status.active') }, { value: 'inactive', label: t('admin.accounts.status.inactive') }, { value: 'disabled', label: t('admin.accounts.status.disabled') }, { value: 'error', label: t('admin.accounts.status.error') }, { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') }, { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') }, { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') }])
const privacyOpts = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
const ACCOUNT_PROXY_UNASSIGNED_FILTER = -1
const proxyOpts = computed(() => [
  { value: '', label: t('admin.accounts.allProxies') },
  { value: ACCOUNT_PROXY_UNASSIGNED_FILTER, label: t('admin.accounts.noProxy') },
  ...(props.proxies || []).map(proxy => ({
    value: proxy.id,
    label: proxy.status === 'inactive'
      ? `${proxy.name} (${t('common.inactive')})`
      : proxy.name
  }))
])
const gOpts = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map(g => ({ value: String(g.id), label: g.name }))
])
</script>
