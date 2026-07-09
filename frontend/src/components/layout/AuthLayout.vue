<template>
  <div class="auth-shell">
    <div class="auth-backdrop"></div>

    <!-- Content Container -->
    <div class="auth-content">
      <!-- Logo/Brand -->
      <div class="auth-brand">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div class="auth-logo">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="auth-title">
            {{ siteName }}
          </h1>
          <p class="auth-subtitle">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="auth-card">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="auth-footer">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="auth-copyright">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'ikik-api')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API 接入与用量管理平台')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-shell {
  position: relative;
  display: flex;
  min-height: 100vh;
  align-items: center;
  justify-content: center;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 2rem;
  color: var(--app-text);
  background:
    linear-gradient(90deg, rgba(255, 255, 255, 0.96) 0%, rgba(255, 255, 255, 0.96) 42%, rgba(247, 247, 248, 0.98) 42%, rgba(247, 247, 248, 0.98) 100%),
    var(--app-bg);
  font-family: var(--font-app);
}

.auth-shell::before {
  position: absolute;
  inset: 0;
  z-index: 0;
  background:
    repeating-linear-gradient(0deg, transparent 0, transparent 39px, rgba(32, 33, 35, 0.026) 40px),
    repeating-linear-gradient(90deg, transparent 0, transparent 39px, rgba(32, 33, 35, 0.018) 40px);
  opacity: 0.54;
  content: "";
  pointer-events: none;
}

.auth-shell::after {
  position: absolute;
  inset: 0;
  z-index: 0;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.46), rgba(255, 255, 255, 0.12) 44%, rgba(236, 236, 241, 0.42));
  content: "";
  pointer-events: none;
}

.auth-backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;
  background: linear-gradient(115deg, rgba(16, 163, 127, 0.08) 0%, rgba(16, 163, 127, 0.02) 28%, transparent 56%);
  pointer-events: none;
}

.auth-content {
  position: relative;
  z-index: 1;
  display: grid;
  width: min(100%, 72rem);
  min-height: min(42rem, calc(100vh - 4rem));
  grid-template-columns: minmax(0, 1fr) minmax(22rem, 30rem);
  align-items: center;
  gap: 4rem;
  animation: auth-rise 620ms ease both;
}

.auth-brand {
  grid-row: 1 / span 3;
  align-self: center;
  max-width: 28rem;
  text-align: left;
}

.auth-brand::after {
  display: block;
  width: 4.5rem;
  height: 2px;
  margin-top: 1.5rem;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--app-primary), rgba(59, 130, 246, 0.46));
  content: "";
}

.auth-logo {
  display: flex;
  width: 3.5rem;
  height: 3.5rem;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid var(--app-border);
  border-radius: 0.875rem;
  background: var(--app-surface);
  box-shadow: 0 14px 32px rgba(0, 0, 0, 0.06);
}

.auth-title {
  margin: 0.875rem 0 0;
  color: var(--app-text);
  font-family: var(--font-home-display);
  font-size: 2.25rem;
  font-weight: 700;
  line-height: 1.12;
  letter-spacing: 0;
}

.auth-subtitle {
  max-width: 25rem;
  margin-top: 0.625rem;
  color: var(--app-muted);
  font-size: 0.95rem;
  line-height: 1.6;
}

.auth-card {
  grid-column: 2;
  border: 1px solid var(--app-border);
  border-radius: 1rem;
  background: color-mix(in srgb, var(--app-surface) 94%, transparent);
  padding: 2.125rem;
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.09);
  backdrop-filter: blur(12px);
}

.auth-footer {
  grid-column: 2;
  margin-top: 1.25rem;
  color: var(--app-muted);
  text-align: center;
  font-size: 0.875rem;
}

.auth-copyright {
  grid-column: 2;
  margin-top: 1.5rem;
  color: var(--app-muted);
  text-align: center;
  font-size: 0.75rem;
}

.auth-card :deep(h2) {
  color: var(--app-text);
  font-family: var(--font-home-display);
  font-size: 1.625rem;
  line-height: 1.2;
  letter-spacing: 0;
}

.auth-card :deep(p),
.auth-card :deep(.text-gray-500),
.auth-card :deep(.dark\:text-dark-400) {
  color: var(--app-muted);
}

.auth-card :deep(.input-label) {
  color: var(--app-muted-strong);
}

.auth-card :deep(.input) {
  min-height: 2.875rem;
  border-color: var(--app-border);
  background: var(--app-surface);
  color: var(--app-text);
  box-shadow: none;
}

.auth-card :deep(.input::placeholder) {
  color: var(--app-muted);
}

.auth-card :deep(.input:focus) {
  border-color: var(--app-primary);
  box-shadow: 0 0 0 3px rgba(16, 163, 127, 0.14);
}

.auth-card :deep(.text-gray-400),
.auth-card :deep(.dark\:text-dark-500),
.auth-card :deep(.auth-password-toggle) {
  color: var(--app-muted);
}

.auth-card :deep(.auth-password-toggle:hover) {
  color: var(--app-text);
}

.auth-card :deep(.btn-primary) {
  min-height: 2.875rem;
  border-radius: 0.75rem;
  background: var(--app-primary);
  color: #ffffff;
  box-shadow: none;
}

.auth-card :deep(.btn-primary:hover) {
  background: var(--app-primary-hover);
  box-shadow: none;
}

.auth-card :deep(.text-primary-600),
.auth-footer :deep(.text-primary-600),
.auth-card :deep(.dark\:text-primary-400),
.auth-footer :deep(.dark\:text-primary-400) {
  color: var(--app-primary);
}

.auth-card :deep(.hover\:text-primary-500:hover),
.auth-footer :deep(.hover\:text-primary-500:hover),
.auth-card :deep(.dark\:hover\:text-primary-300:hover),
.auth-footer :deep(.dark\:hover\:text-primary-300:hover) {
  color: var(--app-primary-hover);
}

.auth-card :deep(.bg-gray-200),
.auth-card :deep(.dark\:bg-dark-700) {
  background-color: rgba(16, 163, 127, 0.12);
}

:global(html.dark .auth-shell) {
  color: var(--app-text);
  background:
    linear-gradient(90deg, rgba(33, 33, 33, 0.98) 0%, rgba(33, 33, 33, 0.98) 42%, rgba(23, 23, 23, 0.96) 42%, rgba(23, 23, 23, 0.96) 100%),
    var(--app-bg);
}

:global(html.dark .auth-shell::before) {
  background:
    repeating-linear-gradient(0deg, transparent 0, transparent 39px, rgba(236, 236, 241, 0.035) 40px),
    repeating-linear-gradient(90deg, transparent 0, transparent 39px, rgba(236, 236, 241, 0.024) 40px);
  opacity: 0.44;
}

:global(html.dark .auth-shell::after) {
  background: linear-gradient(180deg, rgba(236, 236, 241, 0.05), rgba(23, 23, 23, 0.16) 48%, rgba(0, 0, 0, 0.28));
}

:global(html.dark .auth-backdrop) {
  background: linear-gradient(115deg, rgba(16, 163, 127, 0.12) 0%, rgba(16, 163, 127, 0.03) 28%, transparent 56%);
}

:global(html.dark .auth-logo) {
  border-color: var(--app-border);
  background: var(--app-surface);
  box-shadow: 0 14px 32px rgba(0, 0, 0, 0.26);
}

:global(html.dark .auth-title) {
  color: var(--app-text);
}

:global(html.dark .auth-brand::after) {
  background: linear-gradient(90deg, var(--app-primary), rgba(59, 130, 246, 0.56));
}

:global(html.dark .auth-subtitle),
:global(html.dark .auth-footer),
:global(html.dark .auth-copyright) {
  color: var(--app-muted);
}

:global(html.dark .auth-card) {
  border-color: var(--app-border);
  background: color-mix(in srgb, var(--app-surface) 92%, transparent);
  box-shadow: 0 28px 78px rgba(0, 0, 0, 0.32);
}

:global(html.dark .auth-card h2) {
  color: var(--app-text);
}

:global(html.dark .auth-card p),
:global(html.dark .auth-card .text-gray-500),
:global(html.dark .auth-card .dark\:text-dark-400) {
  color: var(--app-muted);
}

:global(html.dark .auth-card .input-label) {
  color: var(--app-muted-strong);
}

:global(html.dark .auth-card .input) {
  border-color: var(--app-border);
  background: var(--app-surface);
  color: var(--app-text);
  box-shadow: none;
}

:global(html.dark .auth-card .input::placeholder) {
  color: var(--app-muted);
}

:global(html.dark .auth-card .input:focus) {
  border-color: var(--app-primary);
  box-shadow: 0 0 0 3px rgba(16, 163, 127, 0.18);
}

:global(html.dark .auth-card .text-gray-400),
:global(html.dark .auth-card .dark\:text-dark-500),
:global(html.dark .auth-card .auth-password-toggle) {
  color: var(--app-muted);
}

:global(html.dark .auth-card .auth-password-toggle:hover) {
  color: var(--app-text);
}

:global(html.dark .auth-card .btn-primary) {
  background: var(--app-primary);
  color: #ffffff;
  box-shadow: none;
}

:global(html.dark .auth-card .btn-primary:hover) {
  background: #19c694;
  box-shadow: none;
}

:global(html.dark .auth-card .text-primary-600),
:global(html.dark .auth-footer .text-primary-600),
:global(html.dark .auth-card .dark\:text-primary-400),
:global(html.dark .auth-footer .dark\:text-primary-400) {
  color: var(--app-primary);
}

:global(html.dark .auth-card .hover\:text-primary-500:hover),
:global(html.dark .auth-footer .hover\:text-primary-500:hover),
:global(html.dark .auth-card .dark\:hover\:text-primary-300:hover),
:global(html.dark .auth-footer .dark\:hover\:text-primary-300:hover) {
  color: #19c694;
}

:global(html.dark .auth-card .bg-gray-200),
:global(html.dark .auth-card .dark\:bg-dark-700) {
  background-color: rgba(16, 163, 127, 0.16);
}

@keyframes auth-rise {
  from {
    opacity: 0;
    transform: translateY(14px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 860px) {
  .auth-shell {
    align-items: flex-start;
    padding: 1.25rem;
    background: var(--app-bg);
  }

  .auth-content {
    min-height: auto;
    grid-template-columns: 1fr;
    gap: 1.25rem;
  }

  .auth-brand,
  .auth-card,
  .auth-footer,
  .auth-copyright {
    grid-column: 1;
  }

  .auth-brand {
    grid-row: auto;
    max-width: none;
    margin-top: 0.25rem;
    text-align: center;
  }

  .auth-brand::after {
    margin-right: auto;
    margin-left: auto;
  }

  .auth-logo {
    margin-right: auto;
    margin-left: auto;
  }

  .auth-subtitle {
    max-width: 22rem;
    margin-right: auto;
    margin-left: auto;
  }

  :global(html.dark .auth-shell) {
    background: var(--app-bg);
  }
}

@media (max-width: 430px) {
  .auth-shell {
    padding: 0.875rem;
  }

  .auth-card {
    padding: 1.25rem;
  }

  .auth-title {
    font-size: 1.625rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .auth-content {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
  }
}
</style>
