<template>
  <section class="ui-section" :class="`ui-section--${surface}`">
    <header v-if="title || description || $slots.actions" class="ui-section-header">
      <div class="min-w-0">
        <h2 v-if="title" class="ui-section-title">{{ title }}</h2>
        <p v-if="description" class="ui-section-description">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="ui-section-actions">
        <slot name="actions" />
      </div>
    </header>
    <div class="ui-section-body">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  title?: string
  description?: string
  surface?: 'plain' | 'panel'
}>(), {
  surface: 'plain'
})
</script>

<style scoped>
.ui-section {
  min-width: 0;
}

.ui-section--panel {
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

.ui-section--panel::before {
  position: absolute;
  top: 0;
  right: 1.125rem;
  left: 1.125rem;
  height: 1px;
  background: color-mix(in srgb, var(--ui-brand) 38%, transparent);
  content: "";
  opacity: 0;
  transform: scaleX(0.25);
  transform-origin: left;
  transition:
    opacity var(--ui-duration-base) var(--ui-ease-standard),
    transform var(--ui-duration-slow) var(--ui-ease-emphasized);
}

.ui-section--panel:hover {
  border-color: var(--ui-border-strong);
  box-shadow: var(--ui-shadow-card-hover);
}

.ui-section--panel:hover::before {
  opacity: 1;
  transform: scaleX(1);
}

.ui-section-header {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  padding: 0 0 0.875rem;
}

.ui-section-title {
  color: var(--ui-text);
  font-size: 0.9375rem;
  font-weight: 600;
  line-height: 1.4;
}

.ui-section-description {
  margin-top: 0.2rem;
  color: var(--ui-text-tertiary);
  font-size: 0.75rem;
  line-height: 1.45;
}

.ui-section-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.5rem;
}

.ui-section-body {
  min-width: 0;
  padding-bottom: 1rem;
}

@media (max-width: 640px) {
  .ui-section--panel {
    padding: 0.875rem 0.875rem 0;
  }

  .ui-section--panel::before {
    right: 0.875rem;
    left: 0.875rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-section--panel,
  .ui-section--panel::before {
    transition-duration: 1ms;
  }
}
</style>
