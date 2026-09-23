import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn() })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      refreshOpenAIToken: vi.fn()
    },
    gemini: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      getCapabilities: vi.fn()
    },
    antigravity: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      refreshAntigravityToken: vi.fn()
    }
  }
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn()
  }
}))

import { apiClient } from '@/api/client'
import { adminAPI } from '@/api/admin'
import { useAccountOAuth } from '@/composables/useAccountOAuth'
import { useOpenAIOAuth } from '@/composables/useOpenAIOAuth'
import { useGeminiOAuth } from '@/composables/useGeminiOAuth'
import { useAntigravityOAuth } from '@/composables/useAntigravityOAuth'

describe('user account OAuth scope', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('routes Anthropic authorization through the user endpoint', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({
      data: { auth_url: 'https://example.test/auth', session_id: 'session-1' }
    } as never)

    const oauth = useAccountOAuth('user')
    await expect(oauth.generateAuthUrl('oauth', 7)).resolves.toBe(true)

    expect(apiClient.post).toHaveBeenCalledWith(
      '/account-oauth/anthropic/auth-url',
      { proxy_id: 7 }
    )
    expect(adminAPI.accounts.generateAuthUrl).not.toHaveBeenCalled()
  })

  it('routes OpenAI authorization through the user endpoint', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({
      data: {
        auth_url: 'https://example.test/auth?state=user-state',
        session_id: 'session-2'
      }
    } as never)

    const oauth = useOpenAIOAuth('user')
    await expect(oauth.generateAuthUrl(8)).resolves.toBe(true)

    expect(apiClient.post).toHaveBeenCalledWith(
      '/account-oauth/openai/auth-url',
      { proxy_id: 8 }
    )
    expect(adminAPI.accounts.generateAuthUrl).not.toHaveBeenCalled()
  })

  it('routes Gemini authorization through the user endpoint', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({
      data: {
        auth_url: 'https://example.test/auth',
        session_id: 'session-3',
        state: 'gemini-state'
      }
    } as never)

    const oauth = useGeminiOAuth('user')
    await expect(oauth.generateAuthUrl(9, 'project-1', 'code_assist')).resolves.toBe(true)

    expect(apiClient.post).toHaveBeenCalledWith(
      '/account-oauth/gemini/auth-url',
      { proxy_id: 9, project_id: 'project-1', oauth_type: 'code_assist' }
    )
    expect(adminAPI.gemini.generateAuthUrl).not.toHaveBeenCalled()
  })

  it('routes Antigravity authorization through the user endpoint', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({
      data: {
        auth_url: 'https://example.test/auth',
        session_id: 'session-4',
        state: 'antigravity-state'
      }
    } as never)

    const oauth = useAntigravityOAuth('user')
    await expect(oauth.generateAuthUrl(10)).resolves.toBe(true)

    expect(apiClient.post).toHaveBeenCalledWith(
      '/account-oauth/antigravity/auth-url',
      { proxy_id: 10 }
    )
    expect(adminAPI.antigravity.generateAuthUrl).not.toHaveBeenCalled()
  })
})
