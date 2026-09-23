import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

import {
  applyPersonalAccountTemplate,
  sanitizeUserAccountCredentials,
  sanitizeUserAccountExtra
} from '../personalAccountTemplate'

describe('personal account user contract', () => {
  it('keeps provider metadata while removing known admin extra settings', () => {
    const result = applyPersonalAccountTemplate(
      'openai',
      {
        access_token: 'oauth-token',
        pool_mode: true,
        model_mapping: { stale: 'stale' }
      },
      {
        email: 'user@example.test',
        privacy_mode: 'training_off',
        openai_passthrough: true,
        openai_compact_mode: 'force_on',
        quota_limit: 10,
        future_official_metadata: 'preserve-me'
      },
      'user'
    )

    expect(result.credentials.access_token).toBe('oauth-token')
    expect(result.credentials.model_mapping).toBeTruthy()
    expect(result.credentials).not.toHaveProperty('pool_mode')
    expect(result.extra).toMatchObject({
      email: 'user@example.test',
      privacy_mode: 'training_off',
      future_official_metadata: 'preserve-me'
    })
    expect(result.extra).not.toHaveProperty('openai_passthrough')
    expect(result.extra).not.toHaveProperty('openai_compact_mode')
    expect(result.extra).not.toHaveProperty('quota_limit')
  })

  it('does not alter administrator template behavior', () => {
    const result = applyPersonalAccountTemplate(
      'openai',
      { access_token: 'oauth-token' },
      undefined,
      'admin'
    )

    expect(result.extra).toMatchObject({
      openai_passthrough: false,
      openai_compact_mode: 'force_on'
    })
  })

  it('sanitizes non-OAuth user payload helpers', () => {
    expect(sanitizeUserAccountCredentials({
      api_key: 'key',
      base_url: 'https://example.test/v1',
      temp_unschedulable_enabled: true,
      model_mapping: { model: 'model' }
    })).toEqual({
      api_key: 'key',
      base_url: 'https://example.test/v1',
      model_mapping: { model: 'model' }
    })
    expect(sanitizeUserAccountExtra({
      free_model_provider: 'provider',
      free_model_disabled_models: {},
      openai_responses_supported: false,
      quota_limit: 10
    })).toEqual({
      free_model_provider: 'provider',
      free_model_disabled_models: {}
    })
  })

  it('keeps Free Models creation limited to its own extra fields', () => {
    const source = readFileSync(
      resolve(process.cwd(), 'src/views/user/FreeModelsView.vue'),
      'utf8'
    )
    expect(source).toContain('free_model_provider: provider.code')
    expect(source).toContain('free_model_disabled_models: {}')
    expect(source).not.toContain('openai_apikey_responses_websockets_v2_mode:')
    expect(source).not.toContain('openai_passthrough: false')
    expect(source).not.toContain('codex_cli_only: false')
  })
})
