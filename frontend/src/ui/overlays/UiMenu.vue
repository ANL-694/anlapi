<template>
  <div ref="rootRef" class="ui-menu">
    <slot name="trigger" :open="open" :toggle="toggle">
      <button
        type="button"
        class="btn btn-secondary"
        aria-haspopup="menu"
        :aria-expanded="open"
        @click="toggle"
      >
        {{ label }}
      </button>
    </slot>
  </div>
  <Teleport to="body">
    <Transition name="ui-menu-fade">
      <div
        v-if="open"
        ref="menuRef"
        class="ui-menu-content"
        :style="menuStyle"
        role="menu"
      >
        <slot :close="close" />
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'

defineProps<{ label: string }>()

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLElement | null>(null)
const menuPosition = ref({ top: 0, left: 0 })
const menuStyle = computed(() => ({
  top: `${menuPosition.value.top}px`,
  left: `${menuPosition.value.left}px`
}))

const viewportPadding = 8

async function updatePosition() {
  const trigger = triggerRef.value
  const menu = menuRef.value
  if (!trigger || !menu) return

  const triggerRect = trigger.getBoundingClientRect()
  const menuRect = menu.getBoundingClientRect()
  const menuWidth = menuRect.width
  const menuHeight = menuRect.height

  let left = triggerRect.right - menuWidth
  let top = triggerRect.bottom + 6

  if (left < viewportPadding) {
    left = viewportPadding
  } else if (left + menuWidth > window.innerWidth - viewportPadding) {
    left = window.innerWidth - menuWidth - viewportPadding
  }

  if (top + menuHeight > window.innerHeight - viewportPadding) {
    top = triggerRect.top - menuHeight - 6
  }
  if (top < viewportPadding) {
    top = viewportPadding
  }

  menuPosition.value = {
    top: Math.round(top),
    left: Math.round(left)
  }
}

function close() {
  open.value = false
  triggerRef.value = null
}

function toggle(event?: MouseEvent) {
  if (open.value) {
    close()
    return
  }

  triggerRef.value = (event?.currentTarget as HTMLElement | null) ?? rootRef.value
  open.value = !open.value
  void nextTick(updatePosition)
}

function handlePointerDown(event: MouseEvent) {
  const target = event.target as Node
  if (!rootRef.value?.contains(target) && !menuRef.value?.contains(target)) close()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}

function handleViewportChange() {
  if (open.value) close()
}

onMounted(() => {
  document.addEventListener('mousedown', handlePointerDown)
  document.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', handleViewportChange)
  window.addEventListener('scroll', handleViewportChange, true)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handlePointerDown)
  document.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleViewportChange)
  window.removeEventListener('scroll', handleViewportChange, true)
})
</script>

<style scoped>
.ui-menu {
  position: relative;
}

.ui-menu-content {
  position: fixed;
  z-index: 1000;
  display: flex;
  width: max-content;
  min-width: 12rem;
  max-width: min(20rem, calc(100vw - 1rem));
  flex-direction: column;
  padding: 0.375rem;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-lg);
  background: var(--ui-surface);
  box-shadow: var(--ui-shadow-popover);
  transform-origin: top right;
}

.ui-menu-content :deep(button),
.ui-menu-content :deep(a) {
  display: flex;
  width: 100%;
  min-height: 2.25rem;
  align-items: center;
  border-radius: var(--ui-radius-md);
  padding: 0.5rem 0.625rem;
  color: var(--ui-text-secondary);
  font-size: 0.875rem;
  text-align: left;
  transition:
    background-color var(--ui-duration-fast) var(--ui-ease-standard),
    color var(--ui-duration-fast) var(--ui-ease-standard),
    transform var(--ui-duration-fast) var(--ui-ease-standard);
}

.ui-menu-content :deep(.btn) {
  justify-content: flex-start;
  border: 0;
  background: transparent;
  box-shadow: none;
}

.ui-menu-content :deep(button:hover),
.ui-menu-content :deep(a:hover) {
  background: var(--ui-surface-hover);
  color: var(--ui-text);
  transform: translateX(1px);
}

.ui-menu-content :deep(button:disabled) {
  cursor: not-allowed;
  opacity: 0.45;
}

.ui-menu-fade-enter-active,
.ui-menu-fade-leave-active {
  transition:
    opacity var(--ui-duration-fast) var(--ui-ease-standard),
    transform var(--ui-duration-fast) var(--ui-ease-emphasized);
}

.ui-menu-fade-enter-from,
.ui-menu-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}

@media (prefers-reduced-motion: reduce) {
  .ui-menu-fade-enter-active,
  .ui-menu-fade-leave-active,
  .ui-menu-content :deep(button),
  .ui-menu-content :deep(a) {
    transition-duration: 1ms;
  }

  .ui-menu-content :deep(button:hover),
  .ui-menu-content :deep(a:hover) {
    transform: none;
  }
}
</style>
