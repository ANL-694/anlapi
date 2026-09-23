<template>
  <section class="dashboard-action-hub" :aria-label="t('dashboard.quickActions')" data-testid="dashboard-hero-actions">
    <router-link
      to="/purchase"
      class="dashboard-action-balance group"
    >
      <span class="dashboard-action-kicker"><span class="dashboard-action-step">01</span>{{ t('dashboard.balance') }}</span>
      <strong>${{ formatBalance(balance) }}</strong>
      <span class="dashboard-action-link">{{ t('dashboard.rechargeBalance') }} <Icon name="arrowRight" size="sm" /></span>
      <span class="dashboard-action-icon dashboard-action-icon--balance" aria-hidden="true">
        <Icon name="dollar" size="lg" />
      </span>
    </router-link>

    <div class="dashboard-action-grid">
      <router-link to="/keys" class="dashboard-action-card group">
        <span class="dashboard-action-card-head"><span class="dashboard-action-step">02</span><span class="dashboard-action-icon"><Icon name="key" size="md" /></span></span>
        <span class="dashboard-action-card-copy"><strong>{{ t('dashboard.createApiKey') }}</strong><small>{{ keySummary }}</small></span>
        <Icon name="arrowRight" size="sm" class="dashboard-action-arrow" aria-hidden="true" />
      </router-link>
      <router-link to="/usage" class="dashboard-action-card group">
        <span class="dashboard-action-card-head"><span class="dashboard-action-step">03</span><span class="dashboard-action-icon"><Icon name="chart" size="md" /></span></span>
        <span class="dashboard-action-card-copy"><strong>{{ t('dashboard.viewUsage') }}</strong><small>{{ t('dashboard.todayRequests') }}: {{ todayRequests }}</small></span>
        <Icon name="arrowRight" size="sm" class="dashboard-action-arrow" aria-hidden="true" />
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ balance: number; totalApiKeys?: number; activeApiKeys?: number; todayRequests?: number }>()
const { t } = useI18n()
const keySummary = computed(() => props.totalApiKeys ? `${t('keys.status.active')}: ${props.activeApiKeys || 0} / ${props.totalApiKeys}` : t('keys.noKeysYet'))
const todayRequests = computed(() => (props.todayRequests || 0).toLocaleString())
const formatBalance = (value: number) => new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
}).format(value)
</script>

<style scoped>
.dashboard-action-hub { display: grid; grid-template-columns: minmax(13.5rem, .9fr) minmax(0, 2fr); gap: .75rem; margin: 0 0 1.5rem; }
.dashboard-action-balance, .dashboard-action-card { position: relative; min-width: 0; overflow: hidden; border: 1px solid theme('colors.gray.200'); border-radius: .5rem; background: white; color: inherit; text-decoration: none; transition: border-color .16s ease, background .16s ease, box-shadow .16s ease, transform .16s ease; }
.dashboard-action-balance { display: flex; min-height: 8.75rem; flex-direction: column; align-items: flex-start; justify-content: space-between; padding: 1rem; border-color: theme('colors.primary.300'); background: theme('colors.primary.50'); }
.dashboard-action-balance:hover, .dashboard-action-card:hover { border-color: theme('colors.primary.400'); background: theme('colors.primary.50'); box-shadow: 0 8px 20px rgba(0,0,0,.08); transform: translateY(-1px); }
.dashboard-action-kicker, .dashboard-action-card-head { display: flex; align-items: center; gap: .5rem; color: #64748b; font-size: .75rem; font-weight: 700; }
.dashboard-action-step { display: inline-flex; min-width: 1.5rem; height: 1.5rem; align-items: center; justify-content: center; border: 1px solid #dbe2ea; border-radius: .375rem; background: white; color: #64748b; font-size: .6875rem; font-variant-numeric: tabular-nums; }
.dashboard-action-balance > strong { margin-top: auto; color: #0f172a; font-size: 1.75rem; font-weight: 750; line-height: 1; font-variant-numeric: tabular-nums; }
.dashboard-action-link { display: inline-flex; align-items: center; gap: .25rem; margin-top: .875rem; color: #0e7490; font-size: .8125rem; font-weight: 700; }
.dashboard-action-icon { display: inline-flex; width: 2rem; height: 2rem; align-items: center; justify-content: center; border-radius: .375rem; background: #f0f9ff; color: #0284c7; }
.dashboard-action-icon--balance { position: absolute; top: 1rem; right: 1rem; background: white; }
.dashboard-action-grid { display: grid; min-width: 0; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .75rem; }
.dashboard-action-card { display: flex; min-height: 8.75rem; flex-direction: column; align-items: flex-start; padding: 1rem; }
.dashboard-action-card-head { width: 100%; justify-content: space-between; }
.dashboard-action-card-copy { display: grid; min-width: 0; gap: .375rem; margin-top: auto; padding-top: .875rem; }
.dashboard-action-card-copy strong { color: #0f172a; font-size: .95rem; }
.dashboard-action-card-copy small { color: #64748b; font-size: .75rem; }
.dashboard-action-arrow { position: absolute; right: 1rem; bottom: 1rem; color: #94a3b8; }
@media (max-width: 768px) { .dashboard-action-hub, .dashboard-action-grid { grid-template-columns: 1fr; } }
:global(.dark) .dashboard-action-balance, :global(.dark) .dashboard-action-card { border-color: #334155; background: #1e293b; }
:global(.dark) .dashboard-action-balance { border-color: #0e7490; background: rgba(8, 47, 73, .35); }
:global(.dark) .dashboard-action-card-copy strong, :global(.dark) .dashboard-action-balance > strong { color: #f8fafc; }
:global(.dark) .dashboard-action-card-copy small, :global(.dark) .dashboard-action-kicker, :global(.dark) .dashboard-action-card-head { color: #94a3b8; }
</style>
