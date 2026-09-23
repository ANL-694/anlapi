import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import EmailBroadcastDialog from '../EmailBroadcastDialog.vue'

const api = vi.hoisted(() => ({
  preview: vi.fn().mockResolvedValue({ html: '<p>preview</p>' }),
  create: vi.fn().mockResolvedValue({ id: 17, status: 'pending' }),
  list: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 }),
  getById: vi.fn(),
  searchRecipients: vi.fn(),
  delete: vi.fn()
}))

vi.mock('@/api/admin', () => ({ adminAPI: { emailBroadcasts: api } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})
const ConfirmDialogStub = defineComponent({ template: '<div />' })

function mountDialog(show = true) {
  return mount(EmailBroadcastDialog, {
    props: { show },
    global: { stubs: { BaseDialog: BaseDialogStub, ConfirmDialog: ConfirmDialogStub, Icon: true } }
  })
}

describe('EmailBroadcastDialog', () => {
  it('refreshes the preview when opened', async () => {
    api.preview.mockClear()
    const wrapper = mountDialog(false)
    await wrapper.setProps({ show: true })
    await new Promise(resolve => setTimeout(resolve, 5))
    await flushPromises()
    expect(api.preview).toHaveBeenCalledWith(
      expect.objectContaining({ body_format: 'html' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect((wrapper.find('iframe').element as HTMLIFrameElement).srcdoc).toContain('preview')
  })

  it('sends a selected-recipient broadcast and emits the id', async () => {
    api.create.mockClear()
    const wrapper = mountDialog()
    const vm = wrapper.vm as any
    vm.form.subject = 'Maintenance'
    vm.form.body = '<p>Back soon</p>'
    vm.selectedRecipients = [{ id: 9, email: 'user@example.test' }]
    await vm.handleSend()
    expect(api.create).toHaveBeenCalledWith({
      subject: 'Maintenance',
      body: '<p>Back soon</p>',
      body_format: 'html',
      recipients_mode: 'selected',
      recipient_user_ids: [9]
    })
    expect(wrapper.emitted('sent')).toEqual([[17]])
    expect(wrapper.emitted('close')).toEqual([[]])
  })
})
