import { getModelsByPlatform, getPresetMappingsByPlatform } from '@/composables/useModelWhitelist'
import { OPENAI_WS_MODE_OFF } from '@/utils/openaiWsMode'
import type { AccountPlatform, OpenAICompactMode } from '@/types'

export const PERSONAL_ACCOUNT_DEFAULT_CONCURRENCY = 3
export const PERSONAL_ACCOUNT_DEFAULT_PRIORITY = 1
export const PERSONAL_ACCOUNT_DEFAULT_AUTO_PAUSE_ON_EXPIRED = true

export const PERSONAL_ACCOUNT_DEFAULT_OPENAI_COMPACT_MODE: OpenAICompactMode = 'force_on'
export const PERSONAL_ACCOUNT_DEFAULT_OPENAI_WS_MODE = OPENAI_WS_MODE_OFF

export type PersonalAccountScope = 'admin' | 'user'

// These settings are administrator-owned operational knobs. User-created
// accounts still retain provider metadata and future official fields, but the
// known admin controls are removed before the user API request is sent.
const USER_ADMIN_EXTRA_KEYS = new Set([
  'openai_responses_supported',
  'openai_oauth_responses_websockets_v2_mode',
  'openai_oauth_responses_websockets_v2_enabled',
  'openai_apikey_responses_websockets_v2_mode',
  'openai_apikey_responses_websockets_v2_enabled',
  'responses_websockets_v2_enabled',
  'openai_ws_enabled',
  'openai_passthrough',
  'openai_oauth_passthrough',
  'openai_responses_flatten_namespaces',
  'openai_long_context_billing_enabled',
  'codex_cli_only',
  'codex_cli_only_allowed_clients',
  'codex_cli_only_allow_app_server',
  'codex_fingerprint_mode',
  'openai_compact_mode',
  'openai_responses_mode',
  'anthropic_passthrough',
  'anthropic_apikey_auth_scheme',
  'web_search_emulation',
  'window_cost_limit',
  'window_cost_sticky_reserve',
  'max_sessions',
  'session_idle_timeout_minutes',
  'base_rpm',
  'rpm_strategy',
  'rpm_sticky_buffer',
  'user_msg_queue_mode',
  'enable_tls_fingerprint',
  'tls_fingerprint_profile_id',
  'session_id_masking_enabled',
  'cache_ttl_override_enabled',
  'cache_ttl_override_target',
  'custom_base_url_enabled',
  'custom_base_url'
])

const USER_ADMIN_CREDENTIAL_KEYS = new Set([
  'pool_mode',
  'pool_mode_retry_count',
  'pool_mode_retry_status_codes',
  'custom_error_codes_enabled',
  'custom_error_codes',
  'intercept_warmup_requests',
  'temp_unschedulable_enabled',
  'temp_unschedulable_rules',
  'openai_capabilities',
  'header_overrides_enabled',
  'header_overrides'
])

export function sanitizeUserAccountExtra(
  extra?: Record<string, unknown>
): Record<string, unknown> | undefined {
  if (!extra) return undefined
  const next = { ...extra }
  for (const key of USER_ADMIN_EXTRA_KEYS) {
    delete next[key]
  }
  for (const key of Object.keys(next)) {
    if (key.startsWith('quota_')) {
      delete next[key]
    }
  }
  return Object.keys(next).length > 0 ? next : undefined
}

export function sanitizeUserAccountCredentials(credentials: Record<string, unknown>): Record<string, unknown> {
  const next = { ...credentials }
  for (const key of USER_ADMIN_CREDENTIAL_KEYS) {
    delete next[key]
  }
  return next
}

export function buildPersonalAccountModelMapping(platform: AccountPlatform | string): Record<string, string> {
  const mapping: Record<string, string> = {}
  if (platform === 'kiro') {
    for (const { from, to } of getPresetMappingsByPlatform('kiro')) {
      if (from && to) {
        mapping[from] = to
      }
    }
    return mapping
  }
  for (const model of getModelsByPlatform(platform)) {
    if (!model.includes('*')) {
      mapping[model] = model
    }
  }
  return mapping
}

export function applyPersonalAccountTemplate(
  platform: AccountPlatform | string,
  credentials: Record<string, unknown>,
  extra?: Record<string, unknown>,
  scope: PersonalAccountScope = 'admin'
): { credentials: Record<string, unknown>; extra?: Record<string, unknown> } {
  const nextCredentials: Record<string, unknown> = {
    ...credentials,
    model_mapping: buildPersonalAccountModelMapping(platform)
  }

  const nextExtra: Record<string, unknown> = {
    ...(scope === 'user' ? sanitizeUserAccountExtra(extra) : extra || {})
  }
  if (scope === 'user') {
    return {
      credentials: sanitizeUserAccountCredentials(nextCredentials),
      extra: Object.keys(nextExtra).length > 0 ? nextExtra : undefined
    }
  }

  if (platform === 'openai') {
    nextExtra.openai_oauth_responses_websockets_v2_mode = PERSONAL_ACCOUNT_DEFAULT_OPENAI_WS_MODE
    nextExtra.openai_oauth_responses_websockets_v2_enabled = false
    nextExtra.openai_passthrough = false
    nextExtra.openai_oauth_passthrough = false
    nextExtra.codex_cli_only = false
    nextExtra.openai_compact_mode = PERSONAL_ACCOUNT_DEFAULT_OPENAI_COMPACT_MODE
    delete nextCredentials.compact_model_mapping
  }
  if (platform === 'kiro') {
    nextExtra.openai_responses_supported = false
  }

  return {
    credentials: nextCredentials,
    extra: Object.keys(nextExtra).length > 0 ? nextExtra : undefined
  }
}
