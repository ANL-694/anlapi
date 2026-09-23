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
  name: 'SelectControl',
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
    const fetchMock = global.fetch as any
    fetchMock
      .mockResolvedValueOnce(jsonResponse({
        object: 'list',
        data: [
          { id: 'gpt-test', display_name: 'GPT Test' },
          { id: 'gpt-image-1', display_name: 'Image model' },
          { id: 'grok-imagine-video-1.5', display_name: 'Video model' },
          { id: 'dall-e-3', display_name: 'DALL-E' },
          { id: 'imagen-4', display_name: 'Imagen' },
          { id: 'sora-2', display_name: 'Sora' },
          { id: 'chatgpt-image-latest', display_name: 'ChatGPT Image' },
          { id: 'tts-1', display_name: 'TTS' },
          { id: 'speech-preview', display_name: 'Speech' },
          { id: 'gpt-4o-mini-tts', display_name: 'GPT TTS' },
          { id: 'custom-image', display_name: 'Custom image', modalities: ['image'] },
          { id: 'custom-audio', display_name: 'Custom audio', modalities: { input: ['audio'], output: ['speech'] } },
          { id: 'custom-image-output', display_name: 'Custom image output', modalities: { input: ['text'], output: ['image'] } },
          { id: 'vision-chat', display_name: 'Vision chat', modalities: ['text', 'image'] },
          { id: 'capability-only-media', display_name: 'Capability media', capabilities: { chat_completions: false } }
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
      { id: 'gpt-test', display_name: 'GPT Test' },
      { id: 'vision-chat', display_name: 'Vision chat' }
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

  it('fails when an SSE response ends without the DONE marker', async () => {
    const fetchMock = global.fetch as any
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'gpt-test' }] }))
      .mockResolvedValueOnce(streamResponse(['data: {"choices":[{"delta":{"content":"OK"}}]}\n\n']))

    const wrapper = mount(ApiKeyTestModal, {
      props: { show: true, apiKey },
      global: { stubs: { BaseDialog: BaseDialogStub, Select: SelectStub, Icon: true } }
    })
    await flushPromises()
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect((wrapper.vm as any).status).toBe('error')
    expect(wrapper.text()).toContain('keys.testModelModal.incompleteResponse')
  })

  it('fails when an SSE event contains malformed JSON', async () => {
    const fetchMock = global.fetch as any
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'gpt-test' }] }))
      .mockResolvedValueOnce(streamResponse(['data: {not-json}\n\n', 'data: [DONE]\n\n']))

    const wrapper = mount(ApiKeyTestModal, {
      props: { show: true, apiKey },
      global: { stubs: { BaseDialog: BaseDialogStub, Select: SelectStub, Icon: true } }
    })
    await flushPromises()
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect((wrapper.vm as any).status).toBe('error')
    expect(wrapper.text()).toContain('keys.testModelModal.invalidResponse')
  })

  it('accepts a legal SSE event split across multiple data lines', async () => {
    const fetchMock = global.fetch as any
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'gpt-test' }] }))
      .mockResolvedValueOnce(streamResponse([
        'event: message\ndata: {"choices":[{"delta":\ndata: {"content":"OK"}}]}\n\n',
        ': keepalive\n\ndata: [DONE]\n\n'
      ]))

    const wrapper = mount(ApiKeyTestModal, {
      props: { show: true, apiKey },
      global: { stubs: { BaseDialog: BaseDialogStub, Select: SelectStub, Icon: true } }
    })
    await flushPromises()
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect((wrapper.vm as any).status).toBe('success')
    expect(wrapper.text()).toContain('OK')
  })

  it('fails on an empty JSON response or an error payload', async () => {
    const fetchMock = global.fetch as any
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'gpt-test' }] }))
      .mockResolvedValueOnce(jsonResponse({ choices: [] }))

    const wrapper = mount(ApiKeyTestModal, {
      props: { show: true, apiKey },
      global: { stubs: { BaseDialog: BaseDialogStub, Select: SelectStub, Icon: true } }
    })
    await flushPromises()
    await (wrapper.vm as any).startTest()
    await flushPromises()
    expect((wrapper.vm as any).status).toBe('error')

    fetchMock
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'gpt-test' }] }))
      .mockResolvedValueOnce(jsonResponse({ error: { message: 'synthetic failure' } }))
    await (wrapper.vm as any).loadAvailableModels()
    await flushPromises()
    await (wrapper.vm as any).startTest()
    await flushPromises()
    expect((wrapper.vm as any).status).toBe('error')
    expect(wrapper.text()).toContain('synthetic failure')
  })

})
