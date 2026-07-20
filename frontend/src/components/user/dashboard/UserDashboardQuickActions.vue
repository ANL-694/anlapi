<template>
  <section class="dashboard-quick-actions" :aria-label="t('dashboard.quickActions')">
    <span class="dashboard-quick-actions-label">{{ t('dashboard.quickActions') }}</span>
    <div class="dashboard-quick-actions-list">
      <router-link to="/purchase" class="dashboard-quick-action dashboard-quick-action--primary">
        <Icon name="dollar" size="sm" />
        <span>{{ t('home.nav.purchase') }}</span>
      </router-link>
      <router-link to="/keys" class="dashboard-quick-action">
        <Icon name="key" size="sm" />
        <span>{{ t('dashboard.createApiKey') }}</span>
      </router-link>
      <router-link to="/usage" class="dashboard-quick-action">
        <Icon name="chart" size="sm" />
        <span>{{ t('dashboard.viewUsage') }}</span>
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
</script>

<style scoped>
.dashboard-quick-actions {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin: 1rem 0 1.25rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--ui-border);
}

.dashboard-quick-actions-label {
  flex: 0 0 auto;
  color: var(--ui-text-tertiary);
  font-size: 0.75rem;
  font-weight: 600;
}

.dashboard-quick-actions-list {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.5rem;
}

.dashboard-quick-action {
  display: inline-flex;
  min-height: 2rem;
  align-items: center;
  gap: 0.375rem;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-md);
  background: var(--ui-surface);
  color: var(--ui-text-secondary);
  padding: 0.375rem 0.625rem;
  font-size: 0.8125rem;
  font-weight: 600;
  transition:
    background 160ms ease,
    border-color 160ms ease,
    color 160ms ease;
}

.dashboard-quick-action:hover {
  border-color: var(--ui-border-strong);
  background: var(--ui-surface-muted);
  color: var(--ui-text);
}

.dashboard-quick-action:focus-visible {
  outline: 2px solid var(--ui-brand);
  outline-offset: 2px;
}

.dashboard-quick-action--primary {
  border-color: var(--ui-brand);
  background: var(--ui-brand);
  color: var(--ui-brand-contrast);
}

.dashboard-quick-action--primary:hover {
  border-color: var(--ui-brand-hover);
  background: var(--ui-brand-hover);
  color: var(--ui-brand-contrast);
}

@media (max-width: 640px) {
  .dashboard-quick-actions {
    align-items: flex-start;
    flex-direction: column;
  }

  .dashboard-quick-actions-list {
    width: 100%;
    justify-content: flex-start;
  }
}

@media (max-width: 420px) {
  .dashboard-quick-action {
    flex: 1 1 auto;
    justify-content: center;
  }
}
</style>
