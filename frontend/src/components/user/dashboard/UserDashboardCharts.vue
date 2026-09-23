<template>
  <section class="dashboard-analytics">
    <UiToolbar class="dashboard-analytics-toolbar">
      <div class="dashboard-filter-row">
        <div class="dashboard-filter-control dashboard-filter-control--date">
          <span class="dashboard-filter-label">{{ t('dashboard.timeRange') }}</span>
          <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        </div>
        <div class="dashboard-filter-control">
          <span class="dashboard-filter-label">{{ t('dashboard.granularity') }}</span>
          <div class="w-28">
            <Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" />
          </div>
        </div>
        <UiIconButton :label="t('common.refresh')" :disabled="loading" @click="$emit('refresh')">
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </UiIconButton>
      </div>
    </UiToolbar>

    <div class="dashboard-analytics-grid">
      <div class="dashboard-analytics-pane dashboard-analytics-pane--trend">
        <TokenUsageTrend :trend-data="trend" :loading="loading" />
      </div>

      <div class="dashboard-analytics-pane dashboard-analytics-pane--models">
        <UiSection :title="t('dashboard.modelDistribution')">
          <div class="relative min-h-48 overflow-hidden">
            <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50">
              <LoadingSpinner size="md" />
            </div>
            <div class="model-distribution">
              <div class="h-40 w-40 shrink-0">
                <Doughnut v-if="modelData" :data="modelData" :options="doughnutOptions" />
                <div v-else class="flex h-full items-center justify-center text-sm text-[var(--app-muted)]">{{ t('dashboard.noDataAvailable') }}</div>
              </div>
              <div class="max-h-44 min-w-0 flex-1 overflow-auto">
                <table class="w-full text-xs">
                  <thead>
                    <tr class="text-[var(--app-muted)]">
                      <th class="pb-2 text-left">{{ t('dashboard.model') }}</th>
                      <th class="pb-2 text-right">{{ t('dashboard.requests') }}</th>
                      <th class="pb-2 text-right">{{ t('dashboard.tokens') }}</th>
                      <th class="pb-2 text-right">{{ t('dashboard.actual') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="model in models" :key="model.model">
                      <td class="max-w-[100px] truncate py-1.5 font-medium text-[var(--app-text)]" :title="model.model">{{ model.model }}</td>
                      <td class="py-1.5 text-right text-[var(--app-muted-strong)]">{{ formatNumber(model.requests) }}</td>
                      <td class="py-1.5 text-right text-[var(--app-muted-strong)]">{{ formatTokens(model.total_tokens) }}</td>
                      <td class="py-1.5 text-right text-[var(--app-text)]">${{ formatCost(model.actual_cost) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </UiSection>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { UiIconButton, UiSection, UiToolbar } from '@/ui'
import { useDarkMode } from '@/composables/useDarkMode'
import { Doughnut } from 'vue-chartjs'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'
import { chartPaletteFor } from '@/utils/chartPalette'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler } from 'chart.js'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler)

const props = defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[], models: ModelStat[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()
const isDarkMode = useDarkMode()

const modelData = computed(() => !props.models?.length ? null : {
  labels: props.models.map((m: ModelStat) => m.model),
  datasets: [{
    data: props.models.map((m: ModelStat) => m.total_tokens),
    backgroundColor: chartPaletteFor(props.models.length),
    borderColor: isDarkMode.value ? '#212121' : '#ffffff',
    borderWidth: 3,
    hoverOffset: 3
  }]
})

const doughnutOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.label}: ${formatTokens(context.parsed)} ${t('dashboard.tokens')}`
      }
    }
  }
}
</script>

<style scoped>
.dashboard-analytics {
  min-width: 0;
}

.dashboard-analytics-toolbar {
  padding: 0.75rem 1rem;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-lg);
  background: var(--ui-surface);
  box-shadow: var(--ui-shadow-card);
  transition:
    border-color var(--ui-duration-base) var(--ui-ease-standard),
    box-shadow var(--ui-duration-base) var(--ui-ease-standard);
}

@media (min-width: 768px) {
  .dashboard-analytics-toolbar {
    position: sticky;
    top: 3.75rem;
    z-index: 10;
    background: color-mix(in srgb, var(--ui-surface) 92%, transparent);
    backdrop-filter: blur(14px);
  }
}

.dashboard-filter-row,
.dashboard-filter-control {
  display: flex;
  min-width: 0;
  align-items: center;
}

.dashboard-filter-row {
  flex-wrap: wrap;
  gap: 0.75rem;
}

.dashboard-filter-control {
  gap: 0.5rem;
}

.dashboard-filter-label {
  color: var(--ui-text-secondary);
  font-size: 0.8125rem;
  font-weight: 500;
  white-space: nowrap;
}

.dashboard-analytics-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(24rem, 0.95fr);
  gap: 1rem;
  margin-top: 1rem;
}

.dashboard-analytics-pane {
  min-width: 0;
  position: relative;
  overflow: hidden;
  padding: 1rem 1.125rem 0;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-lg);
  background: var(--ui-surface);
  box-shadow: var(--ui-shadow-card);
  transition:
    border-color var(--ui-duration-base) var(--ui-ease-standard),
    box-shadow var(--ui-duration-base) var(--ui-ease-standard);
}

.dashboard-analytics-pane::before {
  position: absolute;
  top: 0;
  right: 1.125rem;
  left: 1.125rem;
  height: 2px;
  border-radius: 999px;
  background: var(--ui-brand);
  content: "";
  opacity: 0;
  transform: scaleX(0.3);
  transform-origin: left;
  transition:
    opacity var(--ui-duration-base) var(--ui-ease-standard),
    transform var(--ui-duration-slow) var(--ui-ease-emphasized);
}

.dashboard-analytics-toolbar:hover,
.dashboard-analytics-pane:hover {
  border-color: var(--ui-border-strong);
  box-shadow: var(--ui-shadow-card-hover);
}

.dashboard-analytics-pane:hover::before {
  opacity: 1;
  transform: scaleX(1);
}

.dashboard-analytics-pane--models {
  border-left: 1px solid var(--ui-border);
}

.model-distribution {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 1.5rem;
}

.model-distribution tbody tr {
  transition:
    background-color var(--ui-duration-fast) var(--ui-ease-standard),
    transform var(--ui-duration-fast) var(--ui-ease-standard);
}

.model-distribution tbody tr:hover {
  background: var(--ui-surface-hover);
  transform: translateX(2px);
}

@media (max-width: 640px) {
  .dashboard-analytics-toolbar {
    padding: 0.75rem 0.875rem;
  }

  .dashboard-analytics-pane {
    padding: 0.875rem 0.875rem 0;
  }

  .dashboard-filter-row {
    width: 100%;
    flex-wrap: nowrap;
    gap: 0.5rem;
  }

  .dashboard-filter-label {
    display: none;
  }

  .dashboard-filter-control--date {
    flex: 1 1 auto;
  }

  .model-distribution {
    align-items: stretch;
    flex-direction: column;
  }

  .model-distribution > :first-child {
    align-self: center;
  }
}

@media (max-width: 1100px) {
  .dashboard-analytics-grid {
    grid-template-columns: 1fr;
  }

  .dashboard-analytics-pane--models {
    border-left: 1px solid var(--ui-border);
  }
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-analytics-toolbar,
  .dashboard-analytics-pane,
  .dashboard-analytics-pane::before,
  .model-distribution tbody tr {
    transition-duration: 1ms;
  }

  .model-distribution tbody tr:hover {
    transform: none;
  }
}
</style>
