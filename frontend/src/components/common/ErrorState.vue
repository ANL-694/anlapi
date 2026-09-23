<template>
  <div class="flex min-h-32 flex-col items-center justify-center gap-3 rounded-lg border border-rose-200 bg-rose-50 p-6 text-center dark:border-rose-900/60 dark:bg-rose-950/20">
    <div class="flex h-10 w-10 items-center justify-center rounded-full bg-rose-100 text-rose-600 dark:bg-rose-900/40 dark:text-rose-300" aria-hidden="true">
      <Icon name="exclamationTriangle" size="md" />
    </div>
    <p class="text-sm font-medium text-rose-800 dark:text-rose-200">{{ message || t('common.loadFailed') }}</p>
    <button v-if="retry" type="button" class="btn btn-secondary" @click="$emit('retry')">
      {{ t('common.retry') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ message?: string; retry?: boolean }>()
defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>
