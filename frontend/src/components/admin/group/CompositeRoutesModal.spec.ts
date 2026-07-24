import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CompositeModelRoute } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import CompositeRoutesModal from './CompositeRoutesModal.vue'

const {
  listCompositeRoutes,
  createCompositeRoute,
  updateCompositeRoute,
  deleteCompositeRoute,
  previewCompositeRoute,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listCompositeRoutes: vi.fn(),
  createCompositeRoute: vi.fn(),
  updateCompositeRoute: vi.fn(),
  deleteCompositeRoute: vi.fn(),
  previewCompositeRoute: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      listCompositeRoutes,
      createCompositeRoute,
      updateCompositeRoute,
      deleteCompositeRoute,
      previewCompositeRoute
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const group = {
  id: 12,
  name: 'Composite',
  platform: 'composite'
} as AdminGroup

const route: CompositeModelRoute = {
  id: 5,
  group_id: 12,
  public_model: 'team-gpt',
  match_type: 'exact',
  target_platform: 'openai',
  upstream_model: 'gpt-5.4',
  endpoint: 'responses',
  priority: 10,
  enabled: true,
  notes: '',
  created_at: '2026-07-24T00:00:00Z',
  updated_at: '2026-07-24T00:00:00Z'
}

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<section v-if="show"><slot /></section>'
})

const ConfirmDialogStub = defineComponent({
  props: { show: Boolean },
  emits: ['confirm', 'cancel'],
  template: '<button v-if="show" data-testid="confirm-composite-route-delete" @click="$emit(\'confirm\')">confirm</button>'
})

function mountModal() {
  return mount(CompositeRoutesModal, {
    props: { show: true, group },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        Icon: true,
        PlatformIcon: true
      }
    }
  })
}

describe('CompositeRoutesModal', () => {
  beforeEach(() => {
    for (const fn of [
      listCompositeRoutes,
      createCompositeRoute,
      updateCompositeRoute,
      deleteCompositeRoute,
      previewCompositeRoute,
      showSuccess,
      showError
    ]) {
      fn.mockReset()
    }
    listCompositeRoutes.mockResolvedValue([route])
    createCompositeRoute.mockResolvedValue({ ...route, id: 6 })
    updateCompositeRoute.mockResolvedValue(route)
    deleteCompositeRoute.mockResolvedValue({ message: 'deleted' })
    previewCompositeRoute.mockResolvedValue({
      matched: true,
      source: 'route',
      group_id: 12,
      public_model: 'team-gpt',
      target_platform: 'openai',
      upstream_model: 'gpt-5.4',
      endpoint: 'responses'
    })
  })

  it('loads and creates a route with safe defaults', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-testid="composite-route-public-model"]').setValue('new-alias')
    await wrapper.get('[data-testid="composite-route-save"]').trigger('submit')
    await flushPromises()

    expect(listCompositeRoutes).toHaveBeenCalledWith(12)
    expect(createCompositeRoute).toHaveBeenCalledWith(12, {
      public_model: 'new-alias',
      match_type: 'exact',
      target_platform: 'openai',
      upstream_model: undefined,
      endpoint: 'any',
      priority: 100,
      enabled: true,
      notes: undefined
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.groups.compositeRoutes.created')
  })

  it('edits and deletes the selected route', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-testid="composite-route-edit-5"]').trigger('click')
    await wrapper.get('[data-testid="composite-route-public-model"]').setValue('renamed-alias')
    await wrapper.get('[data-testid="composite-route-save"]').trigger('submit')
    await flushPromises()

    expect(updateCompositeRoute).toHaveBeenCalledWith(12, 5, expect.objectContaining({
      public_model: 'renamed-alias',
      target_platform: 'openai',
      upstream_model: 'gpt-5.4'
    }))

    await wrapper.get('[data-testid="composite-route-delete-5"]').trigger('click')
    await wrapper.get('[data-testid="confirm-composite-route-delete"]').trigger('click')
    await flushPromises()
    expect(deleteCompositeRoute).toHaveBeenCalledWith(12, 5)
  })

  it('previews a model resolution through the backend', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-testid="composite-route-preview-model"]').setValue('team-gpt')
    await wrapper.get('[data-testid="composite-route-preview"]').trigger('click')
    await flushPromises()

    expect(previewCompositeRoute).toHaveBeenCalledWith(12, {
      model: 'team-gpt',
      endpoint: 'any'
    })
    expect(wrapper.text()).toContain('admin.groups.compositeRoutes.previewMatched')
  })
})
