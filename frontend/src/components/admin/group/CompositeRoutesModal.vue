<template>
  <BaseDialog
    :show="show"
    :title="t('admin.groups.compositeRoutes.title')"
    width="extra-wide"
    @close="handleClose"
  >
    <div v-if="group" class="space-y-5">
      <div class="flex flex-wrap items-center gap-3 rounded-lg bg-cyan-50 px-4 py-2.5 text-sm text-cyan-900 dark:bg-cyan-950/30 dark:text-cyan-100">
        <span class="inline-flex items-center gap-1.5 font-medium">
          <PlatformIcon platform="composite" size="sm" />
          {{ group.name }}
        </span>
        <span class="text-cyan-500 dark:text-cyan-500">|</span>
        <span>{{ t('admin.groups.compositeRoutes.description') }}</span>
      </div>

      <div class="grid gap-5 lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.65fr)]">
        <form class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-600" @submit.prevent="saveRoute">
          <div class="flex items-center justify-between gap-3">
            <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">
              {{ editingRouteID === null ? t('admin.groups.compositeRoutes.addRoute') : t('admin.groups.compositeRoutes.editRoute') }}
            </h4>
            <button
              v-if="editingRouteID !== null"
              type="button"
              class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
              @click="resetForm"
            >
              {{ t('admin.groups.compositeRoutes.reset') }}
            </button>
          </div>

          <div class="grid gap-3 md:grid-cols-2">
            <div>
              <label class="input-label" for="composite-route-public-model">
                {{ t('admin.groups.compositeRoutes.publicModel') }}
              </label>
              <input
                id="composite-route-public-model"
                v-model="form.public_model"
                data-testid="composite-route-public-model"
                type="text"
                required
                class="input"
                :placeholder="t('admin.groups.compositeRoutes.publicModelPlaceholder')"
              />
            </div>
            <div>
              <label class="input-label" for="composite-route-upstream-model">
                {{ t('admin.groups.compositeRoutes.upstreamModel') }}
              </label>
              <input
                id="composite-route-upstream-model"
                v-model="form.upstream_model"
                type="text"
                class="input"
                :placeholder="t('admin.groups.compositeRoutes.upstreamModelPlaceholder')"
              />
            </div>
          </div>

          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <label class="input-label" for="composite-route-match-type">
                {{ t('admin.groups.compositeRoutes.matchType') }}
              </label>
              <select id="composite-route-match-type" v-model="form.match_type" class="input">
                <option v-for="option in matchTypeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="input-label" for="composite-route-target-platform">
                {{ t('admin.groups.compositeRoutes.targetPlatform') }}
              </label>
              <select id="composite-route-target-platform" v-model="form.target_platform" class="input">
                <option v-for="option in targetPlatformOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="input-label" for="composite-route-endpoint">
                {{ t('admin.groups.compositeRoutes.endpoint') }}
              </label>
              <select id="composite-route-endpoint" v-model="form.endpoint" class="input">
                <option v-for="option in endpointOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="input-label" for="composite-route-priority">
                {{ t('admin.groups.compositeRoutes.priority') }}
              </label>
              <input
                id="composite-route-priority"
                v-model.number="form.priority"
                type="number"
                min="0"
                step="1"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="input-label" for="composite-route-notes">
              {{ t('admin.groups.compositeRoutes.notes') }}
            </label>
            <textarea
              id="composite-route-notes"
              v-model="form.notes"
              rows="2"
              class="input min-h-16 resize-y"
              :placeholder="t('admin.groups.compositeRoutes.notesPlaceholder')"
            />
          </div>

          <label class="inline-flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            {{ t('admin.groups.compositeRoutes.enabled') }}
          </label>

          <div class="flex justify-end gap-2 border-t border-gray-100 pt-4 dark:border-dark-600">
            <button type="button" class="btn btn-secondary" @click="resetForm">
              {{ t('common.cancel') }}
            </button>
            <button
              data-testid="composite-route-save"
              type="submit"
              class="btn btn-primary"
              :disabled="saving || !form.public_model.trim()"
            >
              <Icon v-if="saving" name="refresh" size="sm" class="mr-1 inline animate-spin" />
              {{ editingRouteID === null ? t('common.create') : t('common.update') }}
            </button>
          </div>
        </form>

        <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div>
            <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">
              {{ t('admin.groups.compositeRoutes.previewTitle') }}
            </h4>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.groups.compositeRoutes.previewHint') }}
            </p>
          </div>
          <div>
            <label class="input-label" for="composite-route-preview-model">
              {{ t('admin.groups.compositeRoutes.previewModel') }}
            </label>
            <input
              id="composite-route-preview-model"
              v-model="previewModel"
              data-testid="composite-route-preview-model"
              type="text"
              class="input"
              :placeholder="t('admin.groups.compositeRoutes.previewModelPlaceholder')"
              @keyup.enter="runPreview"
            />
          </div>
          <div>
            <label class="input-label" for="composite-route-preview-endpoint">
              {{ t('admin.groups.compositeRoutes.endpoint') }}
            </label>
            <select id="composite-route-preview-endpoint" v-model="previewEndpoint" class="input">
              <option v-for="option in endpointOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </div>
          <button
            data-testid="composite-route-preview"
            type="button"
            class="btn btn-secondary w-full"
            :disabled="previewing || !previewModel.trim()"
            @click="runPreview"
          >
            <Icon v-if="previewing" name="refresh" size="sm" class="mr-1 inline animate-spin" />
            <Icon v-else name="search" size="sm" class="mr-1 inline" />
            {{ t('admin.groups.compositeRoutes.preview') }}
          </button>

          <div v-if="previewResult" class="space-y-2 rounded-md bg-gray-50 p-3 text-xs dark:bg-dark-700">
            <p :class="previewResult.matched ? 'font-medium text-emerald-700 dark:text-emerald-400' : 'font-medium text-amber-700 dark:text-amber-400'">
              {{ previewResult.matched ? t('admin.groups.compositeRoutes.previewMatched') : t('admin.groups.compositeRoutes.previewNoMatch') }}
            </p>
            <template v-if="previewResult.matched">
              <p><span class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.compositeRoutes.previewSource') }}:</span> {{ previewResult.source }}</p>
              <p><span class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.compositeRoutes.previewTarget') }}:</span> {{ t(`admin.groups.platforms.${previewResult.target_platform}`) }}</p>
              <p><span class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.compositeRoutes.previewUpstream') }}:</span> {{ previewResult.upstream_model }}</p>
            </template>
            <p v-else-if="previewResult.reason" class="text-gray-500 dark:text-gray-400">
              {{ previewResult.reason }}
            </p>
          </div>
        </section>
      </div>

      <section class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
        <div class="flex items-center justify-between gap-3 border-b border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-700">
          <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">
            {{ t('admin.groups.compositeRoutes.listTitle', { count: routes.length }) }}
          </h4>
          <button type="button" class="rounded p-1 text-gray-500 hover:bg-gray-200 hover:text-gray-800 dark:hover:bg-dark-600 dark:hover:text-gray-100" :title="t('common.refresh')" @click="loadRoutes">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          </button>
        </div>
        <div v-if="loading" class="p-6 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.compositeRoutes.loading') }}
        </div>
        <div v-else-if="routes.length === 0" class="p-6 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.compositeRoutes.noRoutes') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[780px] text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2 text-left">{{ t('admin.groups.compositeRoutes.publicModel') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.groups.compositeRoutes.matchType') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.groups.compositeRoutes.targetPlatform') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.groups.compositeRoutes.upstreamModel') }}</th>
                <th class="px-3 py-2 text-left">{{ t('admin.groups.compositeRoutes.endpoint') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.groups.compositeRoutes.priority') }}</th>
                <th class="px-3 py-2 text-center">{{ t('admin.groups.compositeRoutes.enabled') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.groups.columns.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
              <tr v-for="route in routes" :key="route.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                <td class="max-w-48 truncate px-3 py-2 font-medium text-gray-900 dark:text-gray-100" :title="route.public_model">{{ route.public_model }}</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ t(`admin.groups.compositeRoutes.matchTypes.${route.match_type}`) }}</td>
                <td class="px-3 py-2"><span class="inline-flex items-center gap-1.5"><PlatformIcon :platform="route.target_platform" size="xs" />{{ t(`admin.groups.platforms.${route.target_platform}`) }}</span></td>
                <td class="max-w-48 truncate px-3 py-2 text-gray-600 dark:text-gray-300" :title="route.upstream_model || route.public_model">{{ route.upstream_model || route.public_model }}</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ t(`admin.groups.compositeRoutes.endpoints.${route.endpoint}`) }}</td>
                <td class="px-3 py-2 text-right text-gray-600 dark:text-gray-300">{{ route.priority }}</td>
                <td class="px-3 py-2 text-center"><Icon :name="route.enabled ? 'checkCircle' : 'ban'" size="sm" :class="route.enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'" /></td>
                <td class="px-3 py-2 text-right">
                  <button :data-testid="`composite-route-edit-${route.id}`" type="button" class="rounded p-1 text-gray-500 hover:bg-primary-50 hover:text-primary-600 dark:hover:bg-primary-900/20 dark:hover:text-primary-400" :title="t('common.edit')" @click="startEdit(route)"><Icon name="edit" size="sm" /></button>
                  <button :data-testid="`composite-route-delete-${route.id}`" type="button" class="ml-1 rounded p-1 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" :title="t('common.delete')" @click="pendingDelete = route"><Icon name="trash" size="sm" /></button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </BaseDialog>

  <ConfirmDialog
    :show="pendingDelete !== null"
    :title="t('admin.groups.compositeRoutes.deleteRoute')"
    :message="t('admin.groups.compositeRoutes.deleteConfirm', { model: pendingDelete?.public_model ?? '' })"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="deleteRoute"
    @cancel="pendingDelete = null"
  />
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CompositeModelRoute,
  CompositeRouteDecision,
  CompositeRouteEndpoint,
  CompositeRouteInput,
  CompositeRouteMatchType,
  CompositeTargetPlatform
} from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const routes = ref<CompositeModelRoute[]>([])
const loading = ref(false)
const saving = ref(false)
const previewing = ref(false)
const editingRouteID = ref<number | null>(null)
const pendingDelete = ref<CompositeModelRoute | null>(null)
const previewModel = ref('')
const previewEndpoint = ref<CompositeRouteEndpoint>('any')
const previewResult = ref<CompositeRouteDecision | null>(null)

const defaultForm = (): CompositeRouteInput => ({
  public_model: '',
  match_type: 'exact',
  target_platform: 'openai',
  upstream_model: '',
  endpoint: 'any',
  priority: 100,
  enabled: true,
  notes: ''
})

const form = reactive<CompositeRouteInput>(defaultForm())

const matchTypeOptions = computed(() => [
  { value: 'exact' as CompositeRouteMatchType, label: t('admin.groups.compositeRoutes.matchTypes.exact') },
  { value: 'prefix' as CompositeRouteMatchType, label: t('admin.groups.compositeRoutes.matchTypes.prefix') }
])

const targetPlatformOptions = computed(() =>
  (['anthropic', 'openai', 'gemini', 'antigravity', 'grok'] as CompositeTargetPlatform[]).map((value) => ({
    value,
    label: t(`admin.groups.platforms.${value}`)
  }))
)

const endpointOptions = computed(() =>
  (['any', 'messages', 'count_tokens', 'responses', 'chat_completions', 'embeddings', 'images', 'gemini'] as CompositeRouteEndpoint[]).map((value) => ({
    value,
    label: t(`admin.groups.compositeRoutes.endpoints.${value}`)
  }))
)

const resetForm = () => {
  Object.assign(form, defaultForm())
  editingRouteID.value = null
}

const loadRoutes = async () => {
  if (!props.group || props.group.platform !== 'composite') return
  loading.value = true
  try {
    routes.value = await adminAPI.groups.listCompositeRoutes(props.group.id)
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.groups.compositeRoutes.loadFailed')))
  } finally {
    loading.value = false
  }
}

const startEdit = (route: CompositeModelRoute) => {
  editingRouteID.value = route.id
  Object.assign(form, {
    public_model: route.public_model,
    match_type: route.match_type,
    target_platform: route.target_platform,
    upstream_model: route.upstream_model,
    endpoint: route.endpoint,
    priority: route.priority,
    enabled: route.enabled,
    notes: route.notes || ''
  })
}

const routePayload = (): CompositeRouteInput => ({
  public_model: form.public_model.trim(),
  match_type: form.match_type,
  target_platform: form.target_platform,
  upstream_model: form.upstream_model?.trim() || undefined,
  endpoint: form.endpoint,
  priority: Number.isFinite(form.priority) ? form.priority : 100,
  enabled: form.enabled,
  notes: form.notes?.trim() || undefined
})

const saveRoute = async () => {
  if (!props.group || !form.public_model.trim()) return
  saving.value = true
  try {
    const payload = routePayload()
    if (editingRouteID.value === null) {
      await adminAPI.groups.createCompositeRoute(props.group.id, payload)
      appStore.showSuccess(t('admin.groups.compositeRoutes.created'))
    } else {
      await adminAPI.groups.updateCompositeRoute(props.group.id, editingRouteID.value, payload)
      appStore.showSuccess(t('admin.groups.compositeRoutes.updated'))
    }
    resetForm()
    await loadRoutes()
    emit('success')
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.groups.compositeRoutes.saveFailed')))
  } finally {
    saving.value = false
  }
}

const deleteRoute = async () => {
  if (!props.group || !pendingDelete.value) return
  const routeID = pendingDelete.value.id
  try {
    await adminAPI.groups.deleteCompositeRoute(props.group.id, routeID)
    if (editingRouteID.value === routeID) resetForm()
    pendingDelete.value = null
    await loadRoutes()
    appStore.showSuccess(t('admin.groups.compositeRoutes.deleted'))
    emit('success')
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.groups.compositeRoutes.deleteFailed')))
  }
}

const runPreview = async () => {
  if (!props.group || !previewModel.value.trim()) return
  previewing.value = true
  try {
    previewResult.value = await adminAPI.groups.previewCompositeRoute(props.group.id, {
      model: previewModel.value.trim(),
      endpoint: previewEndpoint.value
    })
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.groups.compositeRoutes.previewFailed')))
  } finally {
    previewing.value = false
  }
}

const handleClose = () => {
  pendingDelete.value = null
  previewResult.value = null
  resetForm()
  emit('close')
}

watch(
  () => props.show,
  (show) => {
    if (!show) return
    resetForm()
    previewModel.value = ''
    previewEndpoint.value = 'any'
    previewResult.value = null
    void loadRoutes()
  },
  { immediate: true }
)
</script>
