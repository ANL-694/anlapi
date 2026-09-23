<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="pb-1">
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-72">
            <input v-model="keyword" type="text" :placeholder="t('admin.withdrawals.searchPlaceholder')" class="input" @input="debounceLoad" />
          </div>
          <Select v-model="filters.status" :options="statusOptions" class="w-36" @change="load" />
          <Select v-model="filters.payment_method" :options="methodOptions" class="w-36" @change="load" />
          <div class="ml-auto flex items-center gap-2">
            <UiIconButton :label="t('common.refresh')" :disabled="loading" @click="load">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </UiIconButton>
          </div>
        </div>
      </div>

      <DataTable :columns="columns" :data="items" :loading="loading">
        <template #cell-id="{ value }">
          <span class="font-mono text-sm">#{{ value }}</span>
        </template>
        <template #cell-user_email="{ value, row }">
          <div class="text-sm">
            <p class="font-medium text-gray-900 dark:text-white">{{ value }}</p>
            <p class="text-xs text-gray-500 dark:text-gray-400">UID {{ row.user_id }}</p>
          </div>
        </template>
        <template #cell-amount="{ value, row }">
          <div class="text-sm">
            <p class="font-semibold text-gray-900 dark:text-white">${{ value.toFixed(2) }}</p>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.withdrawals.deducted') }} ${{ row.total_deducted.toFixed(2) }}</p>
          </div>
        </template>
        <template #cell-payment_method="{ value }">
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ methodLabel(value) }}</span>
        </template>
        <template #cell-receipt_code_url="{ row }">
          <button class="btn btn-secondary btn-sm" @click="openReceipt(row)">{{ t('admin.withdrawals.viewReceipt') }}</button>
        </template>
        <template #cell-status="{ value }">
          <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="statusClass(value)">
            {{ statusLabel(value) }}
          </span>
        </template>
        <template #cell-created_at="{ value }">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex flex-wrap items-center gap-1">
            <button class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-600" @click="openDetail(row)">
              <Icon name="eye" size="sm" />
              {{ t('admin.withdrawals.view') }}
            </button>
            <button v-if="row.status === 'PENDING'" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-green-600 hover:bg-green-50 dark:text-green-400 dark:hover:bg-green-900/20" @click="openProcess(row, 'settle')">
              <Icon name="check" size="sm" />
              {{ t('admin.withdrawals.settle') }}
            </button>
            <button v-if="row.status === 'PENDING'" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="openProcess(row, 'reject')">
              <Icon name="x" size="sm" />
              {{ t('admin.withdrawals.reject') }}
            </button>
          </div>
        </template>
      </DataTable>

      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </div>

    <BaseDialog :show="!!receiptTarget" :title="t('admin.withdrawals.receiptSnapshot')" width="narrow" @close="closeReceipt">
      <div v-if="receiptTarget" class="space-y-4">
        <div class="flex min-h-64 items-center justify-center overflow-hidden rounded-xl border border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-900/50">
          <div v-if="receiptLoading" class="flex flex-col items-center gap-2 text-gray-400 dark:text-gray-500">
            <Icon name="refresh" size="lg" class="animate-spin" />
            <span class="text-sm">{{ t('admin.withdrawals.loadingImage') }}</span>
          </div>
          <img
            v-else-if="receiptImageUrl && !receiptImageFailed"
            :src="receiptImageUrl"
            class="h-auto max-h-[70vh] w-full object-contain"
            :alt="t('admin.withdrawals.receipt')"
            @error="receiptImageFailed = true"
          >
          <div v-else class="flex flex-col items-center gap-2 px-6 py-10 text-center text-gray-400 dark:text-gray-500">
            <Icon name="exclamationCircle" size="lg" />
            <span class="text-sm">{{ t('admin.withdrawals.unavailableImage') }}</span>
          </div>
        </div>
        <div class="text-xs text-gray-500 dark:text-gray-400">
          <p>{{ t('admin.withdrawals.methodLabel') }}: {{ methodLabel(receiptTarget.payment_method) }}</p>
          <p class="break-all">SHA256：{{ receiptTarget.receipt_code_sha256 }}</p>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="!!detailTarget" :title="t('admin.withdrawals.detail')" width="wide" @close="detailTarget = null">
      <div v-if="detailTarget" class="grid gap-4 sm:grid-cols-2">
        <InfoItem :label="t('admin.withdrawals.orderNo')" :value="'#' + detailTarget.id" />
        <InfoItem :label="t('common.status')" :value="statusLabel(detailTarget.status)" />
        <InfoItem :label="t('admin.withdrawals.userEmail')" :value="detailTarget.user_email" />
        <InfoItem :label="t('admin.withdrawals.userId')" :value="String(detailTarget.user_id)" />
        <InfoItem :label="t('admin.withdrawals.amount')" :value="'$' + detailTarget.amount.toFixed(2)" />
        <InfoItem :label="t('admin.withdrawals.fee')" :value="'$' + detailTarget.fee_amount.toFixed(2)" />
        <InfoItem :label="t('admin.withdrawals.balanceBefore')" :value="'$' + detailTarget.balance_before.toFixed(2)" />
        <InfoItem :label="t('admin.withdrawals.balanceAfter')" :value="'$' + detailTarget.balance_after.toFixed(2)" />
        <InfoItem :label="t('admin.withdrawals.createdAt')" :value="formatDate(detailTarget.created_at)" />
        <InfoItem :label="t('admin.withdrawals.processedAt')" :value="detailTarget.processed_at ? formatDate(detailTarget.processed_at) : '-'" />
        <InfoItem class="sm:col-span-2" :label="t('admin.withdrawals.note')" :value="detailTarget.admin_note || detailTarget.user_cancel_reason || '-'" />
      </div>
    </BaseDialog>

    <BaseDialog :show="!!processTarget" :title="processAction === 'settle' ? t('admin.withdrawals.confirmSettle') : t('admin.withdrawals.confirmReject')" width="narrow" @close="processTarget = null">
      <div v-if="processTarget" class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ processAction === 'settle' ? t('admin.withdrawals.settleHint') : t('admin.withdrawals.rejectHint') }}
        </p>
        <textarea v-model="processNote" rows="3" class="input" :placeholder="t('admin.withdrawals.notePlaceholder')"></textarea>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="processTarget = null">{{ t('admin.withdrawals.cancel') }}</button>
          <button class="btn" :class="processAction === 'settle' ? 'btn-primary' : 'btn-danger'" :disabled="processing" @click="submitProcess">
            {{ processing ? t('admin.withdrawals.processing') : t('admin.withdrawals.confirm') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { ReceiptCodePaymentMethod, WithdrawalRequest, WithdrawalStatus } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import type { Column } from '@/components/common/types'
import Icon from '@/components/icons/Icon.vue'
import { UiIconButton } from '@/ui'

const InfoItem = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(props) {
    return () => h('div', [
      h('p', { class: 'text-xs text-gray-500 dark:text-gray-400' }, props.label),
      h('p', { class: 'mt-1 break-words text-sm font-medium text-gray-900 dark:text-white' }, props.value),
    ])
  },
})

const { t } = useI18n()
const appStore = useAppStore()
const items = ref<WithdrawalRequest[]>([])
const loading = ref(false)
const processing = ref(false)
const keyword = ref('')
const filters = reactive({ status: '', payment_method: '' as ReceiptCodePaymentMethod | '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const receiptTarget = ref<WithdrawalRequest | null>(null)
const receiptLoading = ref(false)
const receiptImageFailed = ref(false)
const detailTarget = ref<WithdrawalRequest | null>(null)
const processTarget = ref<WithdrawalRequest | null>(null)
const processAction = ref<'settle' | 'reject'>('settle')
const processNote = ref('')
let debounceTimer: ReturnType<typeof setTimeout> | null = null
let receiptRequestSeq = 0

const columns = computed<Column[]>(() => [
  { key: 'id', label: t('admin.withdrawals.orderNo') },
  { key: 'user_email', label: t('admin.withdrawals.user') },
  { key: 'amount', label: t('admin.withdrawals.amount') },
  { key: 'payment_method', label: t('admin.withdrawals.method') },
  { key: 'receipt_code_url', label: t('admin.withdrawals.receipt') },
  { key: 'status', label: t('common.status') },
  { key: 'created_at', label: t('admin.withdrawals.createdAt') },
  { key: 'actions', label: t('admin.withdrawals.actions') },
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.withdrawals.allStatus') },
  { value: 'PENDING', label: t('withdrawal.statuses.pending') },
  { value: 'SETTLED', label: t('withdrawal.statuses.settled') },
  { value: 'CANCELLED', label: t('withdrawal.statuses.cancelled') },
  { value: 'REJECTED', label: t('withdrawal.statuses.rejected') },
])

const methodOptions = computed(() => [
  { value: '', label: t('admin.withdrawals.allMethods') },
  { value: 'alipay', label: t('withdrawal.methods.alipay') },
  { value: 'wechat', label: t('withdrawal.methods.wechat') },
])

const receiptImageUrl = computed(() => receiptTarget.value?.receipt_code_url?.trim() || '')

function debounceLoad() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => load(), 300)
}

async function load() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getWithdrawals({
      page: pagination.page,
      page_size: pagination.page_size,
      keyword: keyword.value || undefined,
      status: filters.status || undefined,
      payment_method: filters.payment_method || undefined,
    })
    items.value = res.data.items || []
    pagination.total = res.data.total || 0
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.withdrawals.loaded')))
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  void load()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void load()
}

async function openReceipt(row: WithdrawalRequest) {
  const requestSeq = ++receiptRequestSeq
  receiptTarget.value = row
  receiptImageFailed.value = false
  receiptLoading.value = true
  try {
    const res = await adminPaymentAPI.getWithdrawal(row.id)
    if (requestSeq !== receiptRequestSeq) {
      return
    }
    receiptTarget.value = res.data
    const index = items.value.findIndex(item => item.id === res.data.id)
    if (index >= 0) {
      items.value.splice(index, 1, res.data)
    }
  } catch (error: unknown) {
    if (requestSeq === receiptRequestSeq) {
      appStore.showError(extractApiErrorMessage(error, t('admin.withdrawals.receiptLoadFailed')))
    }
  } finally {
    if (requestSeq === receiptRequestSeq) {
      receiptLoading.value = false
    }
  }
}

function closeReceipt() {
  receiptRequestSeq++
  receiptTarget.value = null
  receiptLoading.value = false
  receiptImageFailed.value = false
}

function openDetail(row: WithdrawalRequest) {
  detailTarget.value = row
}

function openProcess(row: WithdrawalRequest, action: 'settle' | 'reject') {
  processTarget.value = row
  processAction.value = action
  processNote.value = ''
}

async function submitProcess() {
  if (!processTarget.value) return
  processing.value = true
  try {
    if (processAction.value === 'settle') {
      await adminPaymentAPI.settleWithdrawal(processTarget.value.id, { note: processNote.value })
      appStore.showSuccess(t('admin.withdrawals.settled'))
    } else {
      await adminPaymentAPI.rejectWithdrawal(processTarget.value.id, { note: processNote.value })
      appStore.showSuccess(t('admin.withdrawals.rejected'))
    }
    processTarget.value = null
    await load()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.withdrawals.processFailed')))
  } finally {
    processing.value = false
  }
}

function statusLabel(status: WithdrawalStatus): string {
  const map: Record<WithdrawalStatus, string> = {
    PENDING: t('withdrawal.statuses.pending'),
    SETTLED: t('withdrawal.statuses.settled'),
    CANCELLED: t('withdrawal.statuses.cancelled'),
    REJECTED: t('withdrawal.statuses.rejected'),
  }
  return map[status]
}

function statusClass(status: WithdrawalStatus): string {
  const map: Record<WithdrawalStatus, string> = {
    PENDING: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300',
    SETTLED: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
    CANCELLED: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
    REJECTED: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  }
  return map[status]
}

function methodLabel(method: ReceiptCodePaymentMethod): string {
  return method === 'alipay' ? t('withdrawal.methods.alipay') : t('withdrawal.methods.wechat')
}

function formatDate(raw: string): string {
  return new Date(raw).toLocaleString()
}

void load()
</script>
