<template>
  <BaseDialog
    :show="show"
    :title="t('keys.testModelModal.title')"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div
        v-if="apiKey"
        class="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-700"
      >
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-500">
            <Icon name="beaker" size="md" class="text-white" :stroke-width="2" />
          </div>
          <div class="min-w-0">
            <div class="truncate font-semibold text-gray-900 dark:text-gray-100">{{ apiKey.name }}</div>
            <div class="font-mono text-xs text-gray-500 dark:text-gray-400">{{ maskApiKey(apiKey.key) }}</div>
          </div>
        </div>
        <span
          :class="[
            'shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold',
            status === 'success'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : status === 'error'
                ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'
                : 'bg-gray-100 text-gray-600 dark:bg-gray-600 dark:text-gray-300'
          ]"
        >
          {{ statusLabel }}
        </span>
      </div>

      <p class="text-sm text-gray-600 dark:text-gray-300">
        {{ t('keys.testModelModal.description') }}
      </p>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('keys.testModelModal.modelLabel') }}
        </label>
        <Select
          v-model="selectedModelId"
          :options="availableModels"
          :disabled="status === 'loading-models' || status === 'testing' || availableModels.length === 0"
          value-key="id"
          label-key="display_name"
          :placeholder="
            status === 'loading-models'
              ? t('keys.testModelModal.loadingModels')
              : t('keys.testModelModal.modelPlaceholder')
          "
        />
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('keys.testModelModal.usageHint') }}
        </p>
      </div>

      <div
        ref="terminalRef"
        class="max-h-[260px] min-h-[140px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
      >
        <div v-if="status === 'loading-models'" class="flex items-center gap-2 text-yellow-400">
          <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <span>{{ t('keys.testModelModal.loadingModels') }}</span>
        </div>
        <div v-else-if="status === 'idle'" class="flex items-center gap-2 text-gray-500">
          <Icon name="play" size="sm" :stroke-width="2" />
          <span>{{ t('keys.testModelModal.ready') }}</span>
        </div>
        <div v-else-if="status === 'ready'" class="flex items-center gap-2 text-green-400">
          <Icon name="checkCircle" size="sm" :stroke-width="2" />
          <span>{{ t('keys.testModelModal.ready') }}</span>
        </div>
        <div v-else-if="status === 'testing'" class="flex items-center gap-2 text-yellow-400">
          <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <span>{{ t('keys.testModelModal.testing') }}</span>
        </div>

        <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
          {{ line.text }}
        </div>

        <div v-if="streamingContent" class="whitespace-pre-wrap text-green-300">
          {{ streamingContent }}<span class="animate-pulse">_</span>
        </div>

        <div
          v-if="status === 'success'"
          class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-green-400"
        >
          <Icon name="check" size="sm" :stroke-width="2" />
          <span>{{ t('keys.testModelModal.completed') }}</span>
        </div>
        <div
          v-else-if="status === 'error'"
          class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-red-400"
        >
          <Icon name="x" size="sm" :stroke-width="2" />
          <span>{{ errorMessage }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          type="button"
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
        <button
          type="button"
          @click="startTest"
          :disabled="status === 'loading-models' || status === 'testing' || !selectedModelId"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            status === 'loading-models' || status === 'testing' || !selectedModelId
              ? 'cursor-not-allowed bg-primary-400 text-white'
              : status === 'success'
                ? 'bg-green-500 text-white hover:bg-green-600'
                : status === 'error'
                  ? 'bg-orange-500 text-white hover:bg-orange-600'
                  : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <Icon
            v-if="status === 'loading-models' || status === 'testing'"
            name="refresh"
            size="sm"
            class="animate-spin"
            :stroke-width="2"
          />
          <Icon v-else-if="status === 'idle' || status === 'ready'" name="play" size="sm" :stroke-width="2" />
          <Icon v-else name="refresh" size="sm" :stroke-width="2" />
          <span>
            {{ status === 'testing' ? t('keys.testModelModal.testing') : status === 'success' || status === 'error' ? t('keys.testModelModal.retry') : t('keys.testModelModal.start') }}
          </span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { buildGatewayUrl } from '@/api/client'
import type { ApiKey } from '@/types'
import { maskApiKey } from '@/utils/maskApiKey'

const { t } = useI18n()

type TestStatus = 'idle' | 'loading-models' | 'ready' | 'testing' | 'success' | 'error'

interface ModelOption {
  id: string
  display_name: string
}

interface OutputLine {
  text: string
  class: string
}

const props = defineProps<{
  show: boolean
  apiKey: ApiKey | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const status = ref<TestStatus>('idle')
const availableModels = ref<ModelOption[]>([])
const selectedModelId = ref('')
const outputLines = ref<OutputLine[]>([])
const streamingContent = ref('')
const errorMessage = ref('')
const terminalRef = ref<HTMLElement | null>(null)
let requestController: AbortController | null = null

const statusLabel = computed(() => {
  switch (status.value) {
    case 'loading-models':
      return t('keys.testModelModal.status.loading')
    case 'testing':
      return t('keys.testModelModal.status.testing')
    case 'success':
      return t('keys.testModelModal.status.success')
    case 'error':
      return t('keys.testModelModal.status.error')
    case 'ready':
      return t('keys.testModelModal.status.ready')
    default:
      return t('keys.testModelModal.status.idle')
  }
})

const abortRequest = () => {
  requestController?.abort()
  requestController = null
}

const resetState = () => {
  status.value = 'idle'
  availableModels.value = []
  selectedModelId.value = ''
  outputLines.value = []
  streamingContent.value = ''
  errorMessage.value = ''
}

const resetTestOutput = () => {
  outputLines.value = []
  streamingContent.value = ''
  errorMessage.value = ''
  status.value = 'ready'
}

const isAbortError = (error: unknown): boolean => {
  return error instanceof DOMException && error.name === 'AbortError'
}

const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  }
}

const addLine = (text: string, className = 'text-gray-300') => {
  outputLines.value.push({ text, class: className })
  void scrollToBottom()
}

const readResponsePayload = async (response: Response): Promise<unknown> => {
  const text = await response.text()
  if (!text.trim()) return null
  try {
    return JSON.parse(text) as unknown
  } catch {
    return { message: text.slice(0, 240) }
  }
}

const asRecord = (value: unknown): Record<string, unknown> | null => {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

const responseErrorDetail = (payload: unknown): string => {
  const root = asRecord(payload)
  const error = asRecord(root?.error)
  const candidate = error?.message ?? root?.message ?? root?.detail
  return typeof candidate === 'string' ? candidate.trim().slice(0, 240) : ''
}

const requestError = (response: Response, payload: unknown): Error => {
  const detail = responseErrorDetail(payload)
  const suffix = detail ? `: ${detail}` : ''
  return new Error(`${t('keys.testModelModal.requestFailed')} (${response.status})${suffix}`)
}

const isNonChatModel = (modelId: string): boolean => {
  const normalized = modelId.toLowerCase()
  return (
    normalized.startsWith('gpt-image-') ||
    normalized.includes('embedding') ||
    normalized.includes('moderation') ||
    normalized.startsWith('cogview') ||
    (normalized.startsWith('gemini-') && normalized.includes('-image')) ||
    (normalized.startsWith('grok-') && normalized.includes('-image'))
  )
}

const modelOptionsFromPayload = (payload: unknown): ModelOption[] => {
  const root = asRecord(payload)
  const rawModels = Array.isArray(root?.data)
    ? root.data
    : Array.isArray(root?.models)
      ? root.models
      : Array.isArray(payload)
        ? payload
        : []
  const seen = new Set<string>()

  return rawModels
    .map((model): ModelOption | null => {
      if (typeof model === 'string') {
        const id = model.trim()
        return id ? { id, display_name: id } : null
      }
      const record = asRecord(model)
      const id = typeof record?.id === 'string' ? record.id.trim() : ''
      if (!id) return null
      const displayName = typeof record?.display_name === 'string' && record.display_name.trim()
        ? record.display_name.trim()
        : id
      return { id, display_name: displayName }
    })
    .filter((model): model is ModelOption => {
      if (!model || seen.has(model.id)) return false
      seen.add(model.id)
      return !isNonChatModel(model.id)
    })
}

const loadAvailableModels = async () => {
  abortRequest()
  resetState()

  if (!props.apiKey?.key) {
    status.value = 'error'
    errorMessage.value = t('keys.testModelModal.missingKey')
    addLine(errorMessage.value, 'text-red-400')
    return
  }

  status.value = 'loading-models'
  const controller = new AbortController()
  requestController = controller

  try {
    const response = await fetch(buildGatewayUrl('/v1/models'), {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${props.apiKey.key}`,
        Accept: 'application/json'
      },
      signal: controller.signal
    })
    const payload = await readResponsePayload(response)
    if (!response.ok) throw requestError(response, payload)

    const models = modelOptionsFromPayload(payload)
    if (models.length === 0) {
      throw new Error(t('keys.testModelModal.noModels'))
    }
    availableModels.value = models
    selectedModelId.value = models[0].id
    status.value = 'ready'
    addLine(t('keys.testModelModal.modelsLoaded', { count: models.length }), 'text-green-400')
  } catch (error: unknown) {
    if (isAbortError(error)) return
    status.value = 'error'
    errorMessage.value = error instanceof Error ? error.message : t('keys.testModelModal.loadFailed')
    addLine(errorMessage.value, 'text-red-400')
  } finally {
    if (requestController === controller) requestController = null
  }
}

const contentText = (value: unknown): string => {
  if (typeof value === 'string') return value
  if (Array.isArray(value)) {
    return value.map((item) => contentText(asRecord(item)?.text ?? item)).join('')
  }
  const record = asRecord(value)
  return typeof record?.text === 'string' ? record.text : ''
}

const chatContentFromPayload = (payload: unknown): string => {
  const root = asRecord(payload)
  const choices = Array.isArray(root?.choices) ? root.choices : []
  const first = asRecord(choices[0])
  const delta = asRecord(first?.delta)
  const message = asRecord(first?.message)
  return contentText(delta?.content ?? message?.content ?? first?.text)
}

const streamChatResponse = async (response: Response): Promise<void> => {
  if (response.headers.get('content-type')?.includes('application/json')) {
    const payload = await readResponsePayload(response)
    const content = chatContentFromPayload(payload)
    if (content) streamingContent.value += content
    return
  }

  const reader = response.body?.getReader()
  if (!reader) throw new Error(t('keys.testModelModal.noResponseBody'))

  const decoder = new TextDecoder()
  let buffer = ''
  let finished = false

  const processLine = (line: string) => {
    if (!line.startsWith('data:')) return
    const raw = line.slice(5).trim()
    if (!raw) return
    if (raw === '[DONE]') {
      finished = true
      return
    }
    let payload: unknown
    try {
      payload = JSON.parse(raw) as unknown
    } catch {
      return
    }
    const root = asRecord(payload)
    const detail = responseErrorDetail(payload)
    if (root?.error || root?.type === 'error') {
      throw new Error(detail || t('keys.testModelModal.requestFailed'))
    }
    const content = chatContentFromPayload(payload)
    if (content) {
      streamingContent.value += content
      void scrollToBottom()
    }
  }

  while (!finished) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() || ''
    for (const line of lines) processLine(line)
  }
  if (buffer) processLine(buffer)
}

const startTest = async () => {
  if (!props.apiKey?.key || !selectedModelId.value) return

  abortRequest()
  resetTestOutput()
  status.value = 'testing'
  addLine(t('keys.testModelModal.starting', { model: selectedModelId.value }), 'text-blue-400')
  addLine(t('keys.testModelModal.promptHint'), 'text-gray-400')

  const controller = new AbortController()
  requestController = controller

  try {
    const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${props.apiKey.key}`,
        'Content-Type': 'application/json',
        Accept: 'text/event-stream, application/json'
      },
      body: JSON.stringify({
        model: selectedModelId.value,
        messages: [{ role: 'user', content: 'Reply with exactly OK.' }],
        max_tokens: 8,
        stream: true
      }),
      signal: controller.signal
    })

    if (!response.ok) {
      const payload = await readResponsePayload(response)
      throw requestError(response, payload)
    }

    await streamChatResponse(response)
    if (!streamingContent.value) {
      addLine(t('keys.testModelModal.emptyResponse'), 'text-yellow-400')
    } else {
      addLine(streamingContent.value, 'text-green-300')
      streamingContent.value = ''
    }
    status.value = 'success'
  } catch (error: unknown) {
    if (isAbortError(error)) return
    status.value = 'error'
    errorMessage.value = error instanceof Error ? error.message : t('keys.testModelModal.testFailed')
    if (streamingContent.value) {
      addLine(streamingContent.value, 'text-green-300')
      streamingContent.value = ''
    }
    addLine(errorMessage.value, 'text-red-400')
  } finally {
    if (requestController === controller) requestController = null
  }
}

const handleClose = () => {
  abortRequest()
  emit('close')
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      void loadAvailableModels()
    } else {
      abortRequest()
    }
  },
  { immediate: true }
)

onUnmounted(abortRequest)
</script>
