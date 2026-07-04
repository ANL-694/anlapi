<template>
  <div
    :class="[
      'group relative flex flex-col overflow-hidden rounded-lg border transition-all duration-200',
      'border-[#d9d9e3] bg-[#ffffff] shadow-[0_10px_28px_rgba(0,0,0,0.06)]',
      'hover:-translate-y-0.5 hover:border-[#cfbda8] hover:shadow-[0_18px_40px_rgba(0,0,0,0.10)]',
      'dark:border-[#3f3f46] dark:bg-[#212121] dark:shadow-[0_14px_34px_rgba(0,0,0,0.28)] dark:hover:border-[#565869]',
    ]"
  >
    <div class="h-1 bg-[#10a37f] dark:bg-[#45d09a]" />

    <div class="flex flex-1 flex-col p-4 sm:p-5">
      <div class="mb-4 flex items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-semibold text-[#171717] dark:text-[#ececf1]">{{ plan.name }}</h3>
            <span class="shrink-0 rounded-md border border-[#d9d9e3] bg-[#f3f3f6] px-2 py-0.5 text-[11px] font-medium text-[#0d8f70] dark:border-[#565869] dark:bg-[#2f2f2f] dark:text-[#45d09a]">
              {{ pLabel }}
            </span>
          </div>
          <p
            v-if="plan.description"
            :title="plan.description"
            class="mt-1 whitespace-pre-line break-words text-xs leading-relaxed text-[#6e6e80] [overflow-wrap:anywhere] dark:text-[#c5c5d2]"
          >
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div class="flex items-baseline gap-1">
            <span class="text-xs text-[#9c8d7f] dark:text-[#958578]">¥</span>
            <span class="text-2xl font-semibold tracking-tight text-[#171717] dark:text-[#ececf1]">{{ plan.price }}</span>
          </div>
          <span class="text-[11px] text-[#6e6e80] dark:text-[#acacbe]">/ {{ validitySuffix }}</span>
          <div v-if="plan.original_price" class="mt-0.5 flex items-center justify-end gap-1.5">
            <span class="text-xs text-[#9b9ba7] line-through dark:text-[#8e8ea0]">¥{{ plan.original_price }}</span>
            <span class="rounded-md bg-[#e6f6f1] px-1.5 py-0.5 text-[10px] font-semibold text-[#0d8f70] dark:bg-[#2f2f2f] dark:text-[#45d09a]">{{ discountText }}</span>
          </div>
        </div>
      </div>

      <div class="mb-4 grid grid-cols-2 gap-x-3 gap-y-2 rounded-lg border border-[#d9d9e3] bg-[#f3f3f6] px-3 py-3 text-xs dark:border-[#3f3f46] dark:bg-[#171717]">
        <div class="flex items-center justify-between">
          <span class="text-[#6e6e80] dark:text-[#acacbe]">{{ t('payment.planCard.rate') }}</span>
          <span class="font-medium text-[#3f3f46] dark:text-[#ececf1]">{{ rateDisplay }}</span>
        </div>
        <div v-if="plan.daily_limit_usd != null" class="flex items-center justify-between">
          <span class="text-[#6e6e80] dark:text-[#acacbe]">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-[#3f3f46] dark:text-[#ececf1]">${{ plan.daily_limit_usd }}</span>
        </div>
        <div v-if="plan.weekly_limit_usd != null" class="flex items-center justify-between">
          <span class="text-[#6e6e80] dark:text-[#acacbe]">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-[#3f3f46] dark:text-[#ececf1]">${{ plan.weekly_limit_usd }}</span>
        </div>
        <div v-if="plan.monthly_limit_usd != null" class="flex items-center justify-between">
          <span class="text-[#6e6e80] dark:text-[#acacbe]">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-[#3f3f46] dark:text-[#ececf1]">${{ plan.monthly_limit_usd }}</span>
        </div>
        <div v-if="plan.daily_limit_usd == null && plan.weekly_limit_usd == null && plan.monthly_limit_usd == null" class="flex items-center justify-between">
          <span class="text-[#6e6e80] dark:text-[#acacbe]">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-[#3f3f46] dark:text-[#ececf1]">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="col-span-2 flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-start sm:justify-between">
          <span class="shrink-0 text-[#6e6e80] dark:text-[#acacbe]">{{ t('payment.planCard.models') }}</span>
          <div class="flex min-w-0 flex-wrap gap-1 sm:justify-end" :title="modelScopeTitle">
            <span v-for="scope in visibleModelScopeLabels" :key="scope"
              class="max-w-full rounded-md border border-[#d9d9e3] bg-[#ffffff] px-1.5 py-0.5 text-[10px] font-medium text-[#565869] [overflow-wrap:anywhere] dark:border-[#565869] dark:bg-[#2f2f2f] dark:text-[#d8c7b7]">
              {{ scope }}
            </span>
            <span v-if="!expandedDetails && hiddenModelScopeCount > 0"
              class="rounded-md border border-[#e0c7b2] bg-[#e6f6f1] px-1.5 py-0.5 text-[10px] font-semibold text-[#0d8f70] dark:border-[#565869] dark:bg-[#2f2f2f] dark:text-[#45d09a]">
              +{{ hiddenModelScopeCount }}
            </span>
          </div>
        </div>
      </div>

      <div v-if="plan.features.length > 0" class="mb-4 space-y-1.5">
        <div v-for="feature in visibleFeatures" :key="feature" class="flex min-w-0 items-start gap-1.5" :title="feature">
          <svg class="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-[#10a37f] dark:text-[#45d09a]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span
            :class="[
              'min-w-0 break-words text-xs leading-relaxed text-[#565869] [overflow-wrap:anywhere] dark:text-[#d8c7b7]',
              expandedDetails ? '' : 'line-clamp-2',
            ]"
          >{{ feature }}</span>
        </div>
        <div v-if="!expandedDetails && hiddenFeatureCount > 0" class="flex items-center gap-1.5 text-[11px] font-medium text-[#0d8f70] dark:text-[#45d09a]" :title="featuresTitle">
          <span class="h-1.5 w-1.5 rounded-full bg-[#10a37f] dark:bg-[#45d09a]" />
          <span>+{{ hiddenFeatureCount }}</span>
        </div>
      </div>

      <button
        v-if="hasExpandableDetails"
        type="button"
        class="mb-4 self-start text-xs font-medium text-[#0d8f70] transition-colors hover:text-[#0a5c4b] dark:text-[#45d09a] dark:hover:text-[#82e7bd]"
        @click="expandedDetails = !expandedDetails"
      >
        {{ expandedDetails ? t('common.collapse') : t('common.expand') }}
      </button>

      <div class="flex-1" />

      <!-- Subscribe Button -->
      <button
        type="button"
        class="w-full rounded-lg bg-[#171717] py-2.5 text-sm font-medium text-[#ffffff] shadow-[0_10px_24px_rgba(0,0,0,0.18)] transition-all hover:bg-black hover:shadow-[0_14px_28px_rgba(0,0,0,0.24)] active:scale-[0.98] dark:bg-[#ececf1] dark:text-[#171717] dark:hover:bg-white"
        @click="emit('select', plan)"
      >
        {{ isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'
import type { UserSubscription } from '@/types'
import {
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[] }>()
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t } = useI18n()
const expandedDetails = ref(false)

const platform = computed(() => props.plan.group_platform || '')
const isRenewal = computed(() =>
  props.activeSubscriptions?.some(s => s.group_id === props.plan.group_id && s.status === 'active') ?? false
)

const pLabel = computed(() => platformLabel(platform.value))

const MAX_VISIBLE_MODEL_SCOPES = 6
const MAX_VISIBLE_FEATURES = 4

const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `×${Number(rate.toPrecision(10))}`
})

const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const modelScopeLabels = computed(() => {
  const scopes = props.plan.supported_model_scopes
  if (!scopes || scopes.length === 0) return []
  return scopes.map(s => MODEL_SCOPE_LABELS[s] || s)
})
const visibleModelScopeLabels = computed(() =>
  expandedDetails.value ? modelScopeLabels.value : modelScopeLabels.value.slice(0, MAX_VISIBLE_MODEL_SCOPES)
)
const hiddenModelScopeCount = computed(() => Math.max(0, modelScopeLabels.value.length - visibleModelScopeLabels.value.length))
const modelScopeTitle = computed(() => modelScopeLabels.value.join(', '))

const visibleFeatures = computed(() =>
  expandedDetails.value ? props.plan.features : props.plan.features.slice(0, MAX_VISIBLE_FEATURES)
)
const hiddenFeatureCount = computed(() => Math.max(0, props.plan.features.length - visibleFeatures.value.length))
const featuresTitle = computed(() => props.plan.features.join('\n'))
const hasExpandableDetails = computed(() => {
  return modelScopeLabels.value.length > MAX_VISIBLE_MODEL_SCOPES || props.plan.features.length > MAX_VISIBLE_FEATURES
})

const validitySuffix = computed(() => {
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${props.plan.validity_days}${t('payment.days')}`
})
</script>
