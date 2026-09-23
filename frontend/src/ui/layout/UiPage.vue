<template>
  <div class="ui-page ui-page--animated" :class="[`ui-page--${width}`, `ui-page--${density}`]">
    <slot />
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  width?: 'narrow' | 'standard' | 'wide' | 'full'
  density?: 'comfortable' | 'compact'
}>(), {
  width: 'standard',
  density: 'comfortable'
})
</script>

<style scoped>
.ui-page {
  width: 100%;
  min-width: 0;
  margin-inline: auto;
}

.ui-page--narrow {
  max-width: var(--ui-page-narrow);
}

.ui-page--standard {
  max-width: var(--ui-page-standard);
}

.ui-page--wide {
  max-width: var(--ui-page-wide);
}

.ui-page--full {
  max-width: none;
}

.ui-page--comfortable {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.ui-page--compact {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.ui-page--animated > :deep(*) {
  animation: ui-page-section-in 220ms ease-out both;
}

.ui-page--animated > :deep(*:nth-child(2)) {
  animation-delay: 35ms;
}

.ui-page--animated > :deep(*:nth-child(3)) {
  animation-delay: 70ms;
}

.ui-page--animated > :deep(*:nth-child(4)) {
  animation-delay: 105ms;
}

.ui-page--animated > :deep(*:nth-child(n + 5)) {
  animation-delay: 140ms;
}

@keyframes ui-page-section-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-page--animated > :deep(*) {
    animation: none;
  }
}

@media (max-width: 640px) {
  .ui-page--comfortable,
  .ui-page--compact {
    gap: 1rem;
  }
}
</style>
