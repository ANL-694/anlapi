import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import ApiKeyTestModal from '../ApiKeyTestModal.vue'

const { buildGatewayUrlMock } = vi.hoisted(() => ({
  buildGatewayUrlMock: vi.fn((path: string) => path)
}))

vi.mock('@/api/client', () => ({
  buildGatewayUrl: buildGatewayUrlMock
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      values ? `${key}:${Object.values(values).join(',')}` : key
  })
}))

vi.mock('@/utils/maskApiKey', () => ({
  maskApiKey: (value: string) => `masked:${value.slice(-4)}`
}))

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const SelectStub = defineComponent({
  name: 'Select',
  props: {
    modelValue: { type: String, default: '' },
    options: { type: Array, default: () => [] },
    valueKey: { type: String, default: 'value' },
    labelKey: { type: String, default: 'label' }
  },
  emits: ['update:modelValue'],
  template: `
    <select
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option[valueKey]" :value="option[valueKey]">
        {{ option[labelKey] }}
      </option>
    </select>
  `
})

const apiKey = {
  id: 42,
  user_id: 7,
  key: 'sk-synthetic-test-key',
  name: 'Synthetic test key',
  group_id: 9,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '',
  updated_at: '',
  current_concurrency: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null
} as any

function jsonResponse(payload: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: vi.fn().mockResolvedValue(JSON.stringify(payload)),
    headers: { get: vi.fn(() => 'application/json') }
  }
}

function streamResponse(chunks: string[]) {
  let index = 0
  return {
    ok: true,
    status: 200,
    headers: { get: vi.fn(() => 'text/event-stream') },
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index >= chunks.length) return { done: true, value: undefined }
          const value = new TextEncoder().encode(chunks[index])
          index += 1
          return { done: false, value }
        })
      })
    }
  }
}

describe('ApiKeyTestModal', () => {
  beforeEach(() => {
    buildGatewayUrlMock.mockClear()
    global.fetch = vi.fn()
  })

  it('discovers models and tests the selected model through the API key gateway', async () => {
    ;(global.fetch as any)
      .mockResolvedValueOnce(jsonResponse({
        object: 'list',
        data: [
          { id: 'gpt-test', display_name: 'GPT Test' },
          { id: 'gpt-image-1', display_name: 'Image model' }
        ]
      }))
      .mockResolvedValueOnce(streamResponse([
        'data: {"choices":[{"delta":{"content":"OK"}}]}\n\n',
        'data: [DONE]\n\n'
      ]))

    const wrapper = mount(ApiKeyTestModal, {
      props: { show: true, apiKey },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          Icon: true
        }
      }
    })

    await flushPromises()

    expect(global.fetch).toHaveBeenCalledWith('/v1/models', expect.objectContaining({
      headers: expect.objectContaining({ Authorization: `Bearer ${apiKey.key}` })
    }))
    expect((wrapper.vm as any).availableModels).toEqual([
      { id: 'gpt-test', display_name: 'GPT Test' }
    ])
    expect((wrapper.vm as any).selectedModelId).toBe('gpt-test')

    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledWith('/v1/chat/completions', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ Authorization: `Bearer ${apiKey.key}` })
    }))
    const [, request] = (global.fetch as any).mock.calls[1]
    expect(JSON.parse(request.body)).toMatchObject({
      model: 'gpt-test',
      messages: [{ role: 'user', content: 'Reply with exactly OK.' }],
      max_tokens: 8,
      stream: true
    })
    expect((wrapper.vm as any).status).toBe('success')
    expect(wrapper.text()).toContain('keys.testModelModal.completed')
  })
})
