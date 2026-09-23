<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.accountShare.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.accountShare.description') }}</p>
        </div>
        <div class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-800" role="tablist">
          <button
            type="button"
            role="tab"
            :aria-selected="activeTab === 'policies'"
            :class="tabClass(activeTab === 'policies')"
            @click="switchTab('policies')"
          >
            {{ t('admin.accountShare.tabs.policies') }}
          </button>
          <button
            type="button"
            role="tab"
            :aria-selected="activeTab === 'settlements'"
            :class="tabClass(activeTab === 'settlements')"
            @click="switchTab('settlements')"
          >
            {{ t('admin.accountShare.tabs.settlements') }}
          </button>
        </div>
      </div>

      <TablePageLayout v-if="activeTab === 'policies'">
        <template #filters>
          <div class="flex flex-wrap items-center gap-3">
            <Select v-model="policyFilters.scope_type" :options="scopeFilterOptions" class="w-44" @change="reloadPolicies" />
            <input
              v-model="policyFilters.platform"
              type="text"
              class="input w-full sm:w-48"
              :placeholder="t('admin.accountShare.filters.platform')"
              @keyup.enter="reloadPolicies"
            />
            <Select v-model="policyFilters.enabled" :options="enabledFilterOptions" class="w-36" @change="reloadPolicies" />
            <button type="button" class="btn btn-secondary" :disabled="policiesLoading" :title="t('common.refresh')" @click="loadPolicies">
              <Icon name="refresh" size="md" :class="policiesLoading ? 'animate-spin' : ''" />
            </button>
            <button type="button" class="btn btn-primary" @click="openCreatePolicy">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.accountShare.actions.createPolicy') }}
            </button>
          </div>
        </template>

        <template #table>
          <DataTable :columns="policyColumns" :data="policies" :loading="policiesLoading" :server-side-sort="true" default-sort-key="effective_at" default-sort-order="desc">
            <template #cell-scope_type="{ row }">
              <div class="font-medium text-gray-900 dark:text-white">{{ scopeLabel(row.scope_type) }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ policyTarget(row) }}</div>
            </template>
            <template #cell-platform="{ value }">
              <span class="font-mono text-sm text-gray-700 dark:text-gray-300">{{ value || '-' }}</span>
            </template>
            <template #cell-owner_share_ratio="{ value }">
              <span class="font-medium text-emerald-700 dark:text-emerald-400">{{ formatRatio(value) }}</span>
            </template>
            <template #cell-invite_share_ratio="{ value }">
              <span class="font-medium text-blue-700 dark:text-blue-400">{{ formatRatio(value) }}</span>
            </template>
            <template #cell-total_share_ratio="{ row }">
              <span>{{ formatRatio(Number(row.owner_share_ratio) + Number(row.invite_share_ratio)) }}</span>
            </template>
            <template #cell-enabled="{ value }">
              <span :class="['badge', value ? 'badge-success' : 'badge-gray']">
                {{ value ? t('admin.accountShare.status.enabled') : t('admin.accountShare.status.disabled') }}
              </span>
            </template>
            <template #cell-effective_at="{ value }">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(value) }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="flex items-center justify-end gap-1">
                <button type="button" class="btn btn-ghost btn-sm" :title="t('admin.accountShare.actions.editPolicy')" @click="openEditPolicy(row)">
                  <Icon name="edit" size="sm" />
                </button>
                <button type="button" class="btn btn-ghost btn-sm" :title="row.enabled ? t('admin.accountShare.actions.disablePolicy') : t('admin.accountShare.actions.enablePolicy')" @click="togglePolicy(row)">
                  <Icon :name="row.enabled ? 'ban' : 'checkCircle'" size="sm" />
                </button>
                <button type="button" class="btn btn-ghost btn-sm text-red-600 hover:text-red-700" :title="t('common.delete')" @click="askDeletePolicy(row)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </template>
          </DataTable>
        </template>

        <template #pagination>
          <Pagination
            v-if="policyPagination.total > 0"
            :page="policyPagination.page"
            :total="policyPagination.total"
            :page-size="policyPagination.page_size"
            @update:page="handlePolicyPage"
            @update:pageSize="handlePolicyPageSize"
          />
        </template>
      </TablePageLayout>

      <TablePageLayout v-else>
        <template #filters>
          <div class="flex flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-72">
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input v-model="settlementFilters.search" type="text" class="input pl-10" :placeholder="t('admin.accountShare.filters.search')" @input="debouncedSettlementReload" />
            </div>
            <input v-model="settlementFilters.start_date" type="date" class="input w-full sm:w-44" :title="t('admin.accountShare.filters.startDate')" @change="reloadSettlements" />
            <input v-model="settlementFilters.end_date" type="date" class="input w-full sm:w-44" :title="t('admin.accountShare.filters.endDate')" @change="reloadSettlements" />
            <Select v-model="settlementFilters.status" :options="settlementStatusOptions" class="w-36" @change="reloadSettlements" />
            <button type="button" class="btn btn-secondary" :disabled="settlementsLoading" :title="t('common.refresh')" @click="loadSettlements">
              <Icon name="refresh" size="md" :class="settlementsLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </template>

        <template #table>
          <DataTable :columns="settlementColumns" :data="settlements" :loading="settlementsLoading" :server-side-sort="false">
            <template #cell-request_id="{ row }">
              <div class="max-w-48 truncate font-mono text-xs text-gray-700 dark:text-gray-300" :title="row.request_id">{{ row.request_id }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ formatDateTime(row.created_at) }}</div>
            </template>
            <template #cell-consumer="{ row }">
              <div class="text-sm text-gray-900 dark:text-white">{{ row.consumer_email || row.consumer_user_id }}</div>
              <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accountShare.settlement.consumer') }}</div>
            </template>
            <template #cell-owner="{ row }">
              <div class="text-sm text-gray-900 dark:text-white">{{ row.owner_email || row.owner_user_id }}</div>
              <div class="text-xs text-emerald-700 dark:text-emerald-400">{{ formatCurrency(row.owner_credit) }}</div>
            </template>
            <template #cell-inviter="{ row }">
              <div v-if="row.inviter_email" class="text-sm text-gray-900 dark:text-white">{{ row.inviter_email }}</div>
              <div v-else class="text-sm text-gray-400">-</div>
              <div class="text-xs text-blue-700 dark:text-blue-400">{{ formatCurrency(row.invite_credit) }}</div>
            </template>
            <template #cell-account="{ row }">
              <div class="text-sm text-gray-900 dark:text-white">{{ row.account_name }}</div>
              <div class="text-xs text-gray-500 dark:text-dark-400">{{ row.platform }}<span v-if="row.model"> · {{ row.model }}</span></div>
            </template>
            <template #cell-consumer_charge="{ value }">{{ formatCurrency(value) }}</template>
            <template #cell-account_cost="{ value }">{{ formatCurrency(value) }}</template>
            <template #cell-status="{ value }">
              <span :class="['badge', value === 'applied' ? 'badge-success' : value === 'frozen' ? 'badge-warning' : 'badge-gray']">{{ settlementStatusLabel(value) }}</span>
            </template>
          </DataTable>
        </template>

        <template #pagination>
          <Pagination
            v-if="settlementPagination.total > 0"
            :page="settlementPagination.page"
            :total="settlementPagination.total"
            :page-size="settlementPagination.page_size"
            @update:page="handleSettlementPage"
            @update:pageSize="handleSettlementPageSize"
          />
        </template>
      </TablePageLayout>
    </div>

    <BaseDialog :show="policyDialogOpen" :title="editingPolicy ? t('admin.accountShare.dialog.editTitle') : t('admin.accountShare.dialog.createTitle')" width="wide" @close="closePolicyDialog">
      <form id="account-share-policy-form" class="space-y-4" @submit.prevent="savePolicy">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.accountShare.form.scope') }}</label>
            <Select v-model="policyForm.scope_type" :options="scopeOptions" :disabled="!!editingPolicy" />
          </div>
          <div v-if="policyForm.scope_type === 'platform'">
            <label class="input-label">{{ t('admin.accountShare.form.platform') }}</label>
            <input v-model="policyForm.platform" type="text" class="input" :disabled="!!editingPolicy" required />
          </div>
          <div v-if="policyForm.scope_type === 'group' || policyForm.scope_type === 'account'">
            <label class="input-label">{{ t('admin.accountShare.form.scopeId') }}</label>
            <input v-model.number="policyForm.scope_id" type="number" min="1" class="input" :disabled="!!editingPolicy" required />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.accountShare.form.ownerRatio') }}</label>
            <div class="flex items-center gap-2"><input v-model.number="policyForm.owner_percent" type="number" min="0" max="100" step="0.01" class="input" required /><span class="text-sm text-gray-500">%</span></div>
          </div>
          <div>
            <label class="input-label">{{ t('admin.accountShare.form.inviteRatio') }}</label>
            <div class="flex items-center gap-2"><input v-model.number="policyForm.invite_percent" type="number" min="0" max="100" step="0.01" class="input" required /><span class="text-sm text-gray-500">%</span></div>
          </div>
        </div>
        <p class="input-hint">{{ t('admin.accountShare.form.ratioHint') }}</p>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.accountShare.form.effectiveAt') }}</label>
            <input v-model="policyForm.effective_at" type="datetime-local" class="input" />
          </div>
          <label class="flex items-center gap-2 pt-7 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="policyForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            {{ t('admin.accountShare.form.enabled') }}
          </label>
        </div>
        <p v-if="policyFormError" class="text-sm text-red-600 dark:text-red-400">{{ policyFormError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closePolicyDialog">{{ t('common.cancel') }}</button>
          <button type="submit" form="account-share-policy-form" class="btn btn-primary" :disabled="policySaving">{{ policySaving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="deleteDialogOpen"
      :title="t('admin.accountShare.dialog.deleteTitle')"
      :message="t('admin.accountShare.dialog.deleteMessage')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="deletePolicy"
      @cancel="deleteDialogOpen = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { accountShareAPI, type AccountSharePolicy, type AccountSharePolicyInput, type AccountShareSettlement } from '@/api/admin/accountShare'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatCurrency, formatDateTime } from '@/utils/format'

type Tab = 'policies' | 'settlements'

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<Tab>('policies')

const policies = ref<AccountSharePolicy[]>([])
const policiesLoading = ref(false)
const policyFilters = reactive({ scope_type: '', platform: '', enabled: '' })
const policyPagination = reactive({ page: 1, page_size: 20, total: 0 })

const settlements = ref<AccountShareSettlement[]>([])
const settlementsLoading = ref(false)
const settlementFilters = reactive({ search: '', start_date: '', end_date: '', status: '' })
const settlementPagination = reactive({ page: 1, page_size: 20, total: 0 })

const policyDialogOpen = ref(false)
const policySaving = ref(false)
const policyFormError = ref('')
const editingPolicy = ref<AccountSharePolicy | null>(null)
const deleteDialogOpen = ref(false)
const deletingPolicy = ref<AccountSharePolicy | null>(null)
const policyForm = reactive({
  scope_type: 'global' as 'global' | 'platform' | 'group' | 'account',
  scope_id: undefined as number | undefined,
  platform: '',
  owner_percent: 0,
  invite_percent: 0,
  enabled: true,
  effective_at: ''
})
let settlementSearchTimer: ReturnType<typeof setTimeout> | null = null

const scopeOptions = computed(() => [
  { value: 'global', label: t('admin.accountShare.scopes.global') },
  { value: 'platform', label: t('admin.accountShare.scopes.platform') },
  { value: 'group', label: t('admin.accountShare.scopes.group') },
  { value: 'account', label: t('admin.accountShare.scopes.account') }
])
const scopeFilterOptions = computed(() => [{ value: '', label: t('admin.accountShare.filters.allScopes') }, ...scopeOptions.value])
const enabledFilterOptions = computed(() => [
  { value: '', label: t('admin.accountShare.filters.allStatus') },
  { value: 'true', label: t('admin.accountShare.status.enabled') },
  { value: 'false', label: t('admin.accountShare.status.disabled') }
])
const settlementStatusOptions = computed(() => [
  { value: '', label: t('admin.accountShare.filters.allStatus') },
  { value: 'applied', label: t('admin.accountShare.status.applied') },
  { value: 'frozen', label: t('admin.accountShare.status.frozen') },
  { value: 'reversed', label: t('admin.accountShare.status.reversed') }
])

const policyColumns = computed<Column[]>(() => [
  { key: 'scope_type', label: t('admin.accountShare.columns.scope'), sortable: true },
  { key: 'platform', label: t('admin.accountShare.columns.platform'), sortable: true },
  { key: 'owner_share_ratio', label: t('admin.accountShare.columns.ownerRatio'), sortable: true },
  { key: 'invite_share_ratio', label: t('admin.accountShare.columns.inviteRatio'), sortable: true },
  { key: 'total_share_ratio', label: t('admin.accountShare.columns.totalRatio') },
  { key: 'version', label: t('admin.accountShare.columns.version'), sortable: true },
  { key: 'effective_at', label: t('admin.accountShare.columns.effectiveAt'), sortable: true },
  { key: 'enabled', label: t('admin.accountShare.columns.status'), sortable: true },
  { key: 'actions', label: t('admin.accountShare.columns.actions'), class: 'text-right' }
])

const settlementColumns = computed<Column[]>(() => [
  { key: 'request_id', label: t('admin.accountShare.columns.request') },
  { key: 'consumer', label: t('admin.accountShare.columns.consumer') },
  { key: 'owner', label: t('admin.accountShare.columns.owner') },
  { key: 'inviter', label: t('admin.accountShare.columns.inviter') },
  { key: 'account', label: t('admin.accountShare.columns.account') },
  { key: 'consumer_charge', label: t('admin.accountShare.columns.consumerCharge'), sortable: true },
  { key: 'account_cost', label: t('admin.accountShare.columns.accountCost'), sortable: true },
  { key: 'status', label: t('admin.accountShare.columns.status'), sortable: true }
])

function tabClass(active: boolean): string {
  return active
    ? 'rounded-md bg-primary-600 px-3 py-1.5 text-sm font-medium text-white'
    : 'rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'
}

function userTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

function errorMessage(error: unknown): string {
  return extractI18nErrorMessage(error, t, 'admin.accountShare.errors', t('common.error'))
}

async function loadPolicies() {
  policiesLoading.value = true
  try {
    const result = await accountShareAPI.listPolicies({
      page: policyPagination.page,
      page_size: policyPagination.page_size,
      scope_type: policyFilters.scope_type || undefined,
      platform: policyFilters.platform.trim() || undefined,
      enabled: policyFilters.enabled === '' ? undefined : policyFilters.enabled === 'true'
    })
    policies.value = result.items || []
    policyPagination.total = result.total || 0
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    policiesLoading.value = false
  }
}

async function loadSettlements() {
  settlementsLoading.value = true
  try {
    const result = await accountShareAPI.listSettlements({
      page: settlementPagination.page,
      page_size: settlementPagination.page_size,
      start_date: settlementFilters.start_date || undefined,
      end_date: settlementFilters.end_date || undefined,
      search: settlementFilters.search.trim() || undefined,
      status: settlementFilters.status || undefined,
      timezone: userTimezone()
    })
    settlements.value = result.items || []
    settlementPagination.total = result.total || 0
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    settlementsLoading.value = false
  }
}

function switchTab(tab: Tab) {
  activeTab.value = tab
  if (tab === 'settlements' && settlements.value.length === 0) void loadSettlements()
  if (tab === 'policies' && policies.value.length === 0) void loadPolicies()
}

function reloadPolicies() {
  policyPagination.page = 1
  void loadPolicies()
}

function reloadSettlements() {
  settlementPagination.page = 1
  void loadSettlements()
}

function debouncedSettlementReload() {
  if (settlementSearchTimer) clearTimeout(settlementSearchTimer)
  settlementSearchTimer = setTimeout(reloadSettlements, 300)
}

function handlePolicyPage(page: number) {
  policyPagination.page = page
  void loadPolicies()
}
function handlePolicyPageSize(pageSize: number) {
  policyPagination.page_size = pageSize
  policyPagination.page = 1
  void loadPolicies()
}
function handleSettlementPage(page: number) {
  settlementPagination.page = page
  void loadSettlements()
}
function handleSettlementPageSize(pageSize: number) {
  settlementPagination.page_size = pageSize
  settlementPagination.page = 1
  void loadSettlements()
}

function resetPolicyForm() {
  policyForm.scope_type = 'global'
  policyForm.scope_id = undefined
  policyForm.platform = ''
  policyForm.owner_percent = 0
  policyForm.invite_percent = 0
  policyForm.enabled = true
  policyForm.effective_at = ''
  policyFormError.value = ''
}

function openCreatePolicy() {
  editingPolicy.value = null
  resetPolicyForm()
  policyDialogOpen.value = true
}

function openEditPolicy(policy: AccountSharePolicy) {
  editingPolicy.value = policy
  policyForm.scope_type = policy.scope_type as typeof policyForm.scope_type
  policyForm.scope_id = policy.scope_id ?? undefined
  policyForm.platform = policy.platform ?? ''
  policyForm.owner_percent = Number(policy.owner_share_ratio) * 100
  policyForm.invite_percent = Number(policy.invite_share_ratio) * 100
  policyForm.enabled = policy.enabled
  policyForm.effective_at = toLocalDateTime(policy.effective_at)
  policyFormError.value = ''
  policyDialogOpen.value = true
}

function closePolicyDialog() {
  policyDialogOpen.value = false
  editingPolicy.value = null
  policyFormError.value = ''
}

function toLocalDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function toEffectiveAt(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
}

function policyPayload(): AccountSharePolicyInput {
  const payload: AccountSharePolicyInput = {
    scope_type: policyForm.scope_type,
    owner_share_ratio: Number(policyForm.owner_percent) / 100,
    invite_share_ratio: Number(policyForm.invite_percent) / 100,
    enabled: policyForm.enabled,
    effective_at: toEffectiveAt(policyForm.effective_at)
  }
  if (policyForm.scope_type === 'platform') payload.platform = policyForm.platform.trim()
  if (policyForm.scope_type === 'group' || policyForm.scope_type === 'account') payload.scope_id = policyForm.scope_id
  return payload
}

async function savePolicy() {
  const owner = Number(policyForm.owner_percent)
  const invite = Number(policyForm.invite_percent)
  if (!Number.isFinite(owner) || !Number.isFinite(invite) || owner < 0 || invite < 0 || owner > 100 || invite > 100 || owner + invite > 100) {
    policyFormError.value = t('admin.accountShare.form.ratioError')
    return
  }
  if (policyForm.scope_type === 'platform' && !policyForm.platform.trim()) {
    policyFormError.value = t('admin.accountShare.form.platformRequired')
    return
  }
  if ((policyForm.scope_type === 'group' || policyForm.scope_type === 'account') && (!policyForm.scope_id || policyForm.scope_id < 1)) {
    policyFormError.value = t('admin.accountShare.form.scopeIdRequired')
    return
  }

  policySaving.value = true
  policyFormError.value = ''
  try {
    const payload = policyPayload()
    if (editingPolicy.value) {
      await accountShareAPI.updatePolicy(editingPolicy.value.id, payload)
    } else {
      await accountShareAPI.createPolicy(payload)
    }
    appStore.showSuccess(t('admin.accountShare.messages.saved'))
    closePolicyDialog()
    await loadPolicies()
  } catch (error) {
    policyFormError.value = errorMessage(error)
  } finally {
    policySaving.value = false
  }
}

async function togglePolicy(policy: AccountSharePolicy) {
  try {
    await accountShareAPI.updatePolicy(policy.id, { enabled: !policy.enabled })
    appStore.showSuccess(t('admin.accountShare.messages.updated'))
    await loadPolicies()
  } catch (error) {
    appStore.showError(errorMessage(error))
  }
}

function askDeletePolicy(policy: AccountSharePolicy) {
  deletingPolicy.value = policy
  deleteDialogOpen.value = true
}

async function deletePolicy() {
  if (!deletingPolicy.value) return
  try {
    await accountShareAPI.deletePolicy(deletingPolicy.value.id)
    appStore.showSuccess(t('admin.accountShare.messages.deleted'))
    deleteDialogOpen.value = false
    deletingPolicy.value = null
    await loadPolicies()
  } catch (error) {
    appStore.showError(errorMessage(error))
  }
}

function scopeLabel(scope: string): string {
  return t(`admin.accountShare.scopes.${scope}`)
}

function policyTarget(policy: AccountSharePolicy): string {
  if (policy.scope_type === 'platform') return policy.platform || '-'
  if (policy.scope_type === 'global') return t('admin.accountShare.targets.all')
  return policy.scope_id ? `#${policy.scope_id}` : '-'
}

function formatRatio(value: number | null | undefined): string {
  return `${(Number(value || 0) * 100).toFixed(2)}%`
}

function settlementStatusLabel(status: string): string {
  const key = `admin.accountShare.status.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

onMounted(() => {
  void loadPolicies()
})
</script>
