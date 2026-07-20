<template>
  <section class="dashboard-action-hub" :aria-label="t('dashboard.quickActions')">
    <router-link to="/purchase" class="dashboard-action-balance">
      <span class="dashboard-action-kicker">
        <span class="dashboard-action-step">01</span>
        {{ t('dashboard.balance') }}
      </span>
      <strong>${{ formattedBalance }}</strong>
      <span class="dashboard-action-link">
        {{ t('dashboard.rechargeBalance') }}
        <Icon name="arrowRight" size="sm" />
      </span>
      <span class="dashboard-action-icon dashboard-action-icon--balance" aria-hidden="true">
        <Icon name="dollar" size="lg" />
      </span>
    </router-link>

    <div class="dashboard-action-grid">
      <router-link to="/keys" class="dashboard-action-card">
        <span class="dashboard-action-card-head">
          <span class="dashboard-action-step">02</span>
          <span class="dashboard-action-icon">
            <Icon name="key" size="md" />
          </span>
        </span>
        <span class="dashboard-action-card-copy">
          <strong>{{ t('dashboard.createApiKey') }}</strong>
          <small>{{ keySummary }}</small>
        </span>
        <Icon name="arrowRight" size="sm" class="dashboard-action-arrow" aria-hidden="true" />
      </router-link>

      <router-link to="/usage" class="dashboard-action-card">
        <span class="dashboard-action-card-head">
          <span class="dashboard-action-step">03</span>
          <span class="dashboard-action-icon">
            <Icon name="chart" size="md" />
          </span>
        </span>
        <span class="dashboard-action-card-copy">
          <strong>{{ t('dashboard.viewUsage') }}</strong>
          <small>{{ t('dashboard.todayRequests') }}: {{ formattedTodayRequests }}</small>
        </span>
        <Icon name="arrowRight" size="sm" class="dashboard-action-arrow" aria-hidden="true" />
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserDashboardStats } from '@/api/usage'
import Icon from '@/components/icons/Icon.vue'
import type { User } from '@/types'

const props = defineProps<{
  user: User | null | undefined
  stats: UserDashboardStats
}>()

const { t } = useI18n()

const formattedBalance = computed(() => {
  const balance = Number(props.user?.balance || 0)
  return Number.isFinite(balance) ? balance.toFixed(2) : '0.00'
})

const formattedTodayRequests = computed(() => props.stats.today_requests.toLocaleString())

const keySummary = computed(() => {
  const total = props.stats.total_api_keys || 0
  if (total === 0) return t('keys.noKeysYet')

  return `${t('keys.status.active')}: ${props.stats.active_api_keys || 0} / ${total}`
})
</script>

<style scoped>
.dashboard-action-hub {
  display: grid;
  grid-template-columns: minmax(13.5rem, 0.9fr) minmax(0, 2fr);
  gap: 0.75rem;
  margin: 1rem 0 1.5rem;
}

.dashboard-action-balance,
.dashboard-action-card {
  position: relative;
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-md);
  background: var(--ui-surface);
  color: var(--ui-text);
  text-decoration: none;
  transition:
    border-color 160ms ease,
    background 160ms ease,
    box-shadow 160ms ease,
    transform 160ms ease;
}

.dashboard-action-balance {
  display: flex;
  min-height: 8.75rem;
  flex-direction: column;
  align-items: flex-start;
  justify-content: space-between;
  padding: 1rem;
  border-color: var(--ui-brand);
  background: color-mix(in srgb, var(--ui-brand) 6%, var(--ui-surface));
}

.dashboard-action-balance:hover,
.dashboard-action-card:hover {
  border-color: var(--ui-border-strong);
  background: var(--ui-surface-subtle);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08);
  transform: translateY(-1px);
}

.dashboard-action-balance:hover {
  border-color: var(--ui-brand);
}

.dashboard-action-balance:focus-visible,
.dashboard-action-card:focus-visible {
  outline: 2px solid var(--ui-brand);
  outline-offset: 2px;
}

.dashboard-action-kicker,
.dashboard-action-card-head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--ui-text-secondary);
  font-size: 0.75rem;
  font-weight: 700;
}

.dashboard-action-step {
  display: inline-flex;
  min-width: 1.5rem;
  height: 1.5rem;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-sm);
  background: var(--ui-surface);
  color: var(--ui-text-secondary);
  font-size: 0.6875rem;
  font-variant-numeric: tabular-nums;
}

.dashboard-action-balance > strong {
  margin-top: auto;
  color: var(--ui-text);
  font-size: 1.75rem;
  font-weight: 750;
  line-height: 1;
  font-variant-numeric: tabular-nums;
}

.dashboard-action-link {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.875rem;
  color: var(--ui-brand);
  font-size: 0.8125rem;
  font-weight: 700;
}

.dashboard-action-icon {
  display: inline-flex;
  width: 2rem;
  height: 2rem;
  align-items: center;
  justify-content: center;
  border-radius: var(--ui-radius-sm);
  background: var(--ui-surface-subtle);
  color: var(--ui-brand);
}

.dashboard-action-icon--balance {
  position: absolute;
  top: 1rem;
  right: 1rem;
  background: var(--ui-surface);
}

.dashboard-action-grid {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.dashboard-action-card {
  display: flex;
  min-height: 8.75rem;
  flex-direction: column;
  align-items: flex-start;
  padding: 1rem;
}

.dashboard-action-card-head {
  width: 100%;
  justify-content: space-between;
}

.dashboard-action-card-copy {
  display: grid;
  min-width: 0;
  gap: 0.375rem;
  margin-top: auto;
  padding-top: 0.875rem;
}

.dashboard-action-card-copy strong {
  color: var(--ui-text);
  font-size: 0.9375rem;
  font-weight: 700;
}

.dashboard-action-card-copy small {
  overflow: hidden;
  color: var(--ui-text-tertiary);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dashboard-action-arrow {
  position: absolute;
  right: 1rem;
  bottom: 1rem;
  color: var(--ui-text-tertiary);
}

@media (max-width: 860px) {
  .dashboard-action-hub {
    grid-template-columns: 1fr;
  }

  .dashboard-action-balance {
    min-height: 7.5rem;
  }
}

@media (max-width: 520px) {
  .dashboard-action-grid {
    grid-template-columns: 1fr;
  }

  .dashboard-action-card {
    min-height: 7.5rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-action-balance,
  .dashboard-action-card {
    transition: none;
  }
}
</style>
