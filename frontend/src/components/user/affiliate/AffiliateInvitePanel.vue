<template>
  <section class="affiliate-panel">
    <header class="affiliate-panel-header">
      <h2>{{ t('affiliate.title') }}</h2>
    </header>

    <div class="affiliate-invite-grid">
      <div class="affiliate-invite-field">
        <span class="affiliate-field-label">{{ t('affiliate.yourCode') }}</span>
        <div class="affiliate-field-value flex flex-col items-stretch sm:flex-row sm:items-center">
          <code class="min-w-0 break-all sm:flex-1 sm:truncate">{{ code }}</code>
          <button class="affiliate-copy-button w-full sm:w-auto sm:shrink-0" :title="t('affiliate.copyCode')" @click="emit('copy-code')">
            <Icon name="copy" size="sm" />
            <span>{{ t('affiliate.copyCode') }}</span>
          </button>
        </div>
      </div>

      <div class="affiliate-invite-field">
        <span class="affiliate-field-label">{{ t('affiliate.inviteLink') }}</span>
        <div class="affiliate-field-value flex flex-col items-stretch sm:flex-row sm:items-center">
          <code class="min-w-0 break-all sm:flex-1 sm:truncate">{{ inviteLink }}</code>
          <button class="affiliate-copy-button w-full sm:w-auto sm:shrink-0" :title="t('affiliate.copyLink')" @click="emit('copy-link')">
            <Icon name="copy" size="sm" />
            <span>{{ t('affiliate.copyLink') }}</span>
          </button>
        </div>
      </div>
    </div>

    <div class="affiliate-rules">
      <span>{{ t('affiliate.tips.line1') }}</span>
      <span>{{ t('affiliate.tips.line2', { rate: `${rebateRate}%` }) }}</span>
      <span>{{ t('affiliate.tips.line3') }}</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  code: string
  inviteLink: string
  rebateRate: string
}>()

const emit = defineEmits<{
  'copy-code': []
  'copy-link': []
}>()

const { t } = useI18n()
</script>

<style scoped>
.affiliate-panel {
  overflow: hidden;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-lg);
  background: var(--ui-surface);
}

.affiliate-panel-header {
  padding: 0.875rem 1.25rem;
  border-bottom: 1px solid var(--ui-border);
}

.affiliate-panel-header h2 {
  color: var(--ui-text);
  font-size: 0.9375rem;
  font-weight: 600;
}

.affiliate-invite-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1.25rem;
  padding: 1.25rem;
}

.affiliate-invite-field {
  min-width: 0;
}

.affiliate-field-label {
  display: block;
  margin-bottom: 0.45rem;
  color: var(--ui-text-secondary);
  font-size: 0.75rem;
  font-weight: 500;
}

.affiliate-field-value {
  display: flex;
  min-width: 0;
  min-height: 2.75rem;
  flex-direction: column;
  align-items: stretch;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-md);
}

.affiliate-field-value code {
  min-width: 0;
  overflow: visible;
  overflow-wrap: anywhere;
  color: var(--ui-text);
  font-size: 0.8125rem;
  text-overflow: clip;
  white-space: normal;
}

.affiliate-copy-button {
  display: inline-flex;
  width: 100%;
  height: 2rem;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.375rem;
  justify-content: center;
  padding-inline: 0.625rem;
  border-radius: var(--ui-radius-md);
  color: var(--ui-text-secondary);
}

.affiliate-copy-button:hover {
  background: var(--ui-surface-subtle);
  color: var(--ui-text);
}

.affiliate-rules {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1rem;
  padding: 0.875rem 1.25rem;
  border-top: 1px solid var(--ui-border);
  color: var(--ui-text-tertiary);
  font-size: 0.75rem;
  line-height: 1.45;
}

@media (max-width: 900px) {
  .affiliate-invite-grid,
  .affiliate-rules {
    grid-template-columns: 1fr;
  }

  .affiliate-rules {
    gap: 0.35rem;
  }
}

@media (min-width: 640px) {
  .affiliate-field-value {
    height: 2.75rem;
    flex-direction: row;
    align-items: center;
    padding: 0 0.4rem 0 0.75rem;
  }

  .affiliate-field-value code {
    flex: 1;
    overflow: hidden;
    overflow-wrap: normal;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .affiliate-copy-button {
    width: auto;
  }
}

@media (max-width: 640px) {
  .affiliate-invite-grid {
    gap: 1rem;
    padding: 1rem;
  }

  .affiliate-rules {
    padding: 0.75rem 1rem;
  }
}
</style>
