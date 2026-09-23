<template>
  <div
    class="app-shell"
    :class="{ 'admin-font': isAdminWorkspace }"
    :data-workspace="workspace"
  >
    <AppSidebar />

    <div
      class="app-workspace"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <AppHeader />

      <main class="app-main">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import { resolveAppWorkspace } from '@/utils/workspace'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const route = useRoute()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const workspace = computed(() => resolveAppWorkspace(route, authStore.isSimpleMode, authStore.isAdmin))
const isAdminWorkspace = computed(() => workspace.value === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: () => `${workspace.value}_guide`,
  workspace: () => workspace.value,
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
