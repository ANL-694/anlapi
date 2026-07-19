export const PLATFORM_QUOTA_PLATFORMS = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
  'kiro'
] as const

export const PLATFORM_QUOTA_WINDOWS = ['daily', 'weekly', 'monthly'] as const

export type PlatformQuotaPlatform = (typeof PLATFORM_QUOTA_PLATFORMS)[number]
export type PlatformQuotaWindow = (typeof PLATFORM_QUOTA_WINDOWS)[number]

export interface PlatformQuotaLimitSetting {
  daily: number | null
  weekly: number | null
  monthly: number | null
}
export type PlatformQuotaLimitSettings = Record<
  PlatformQuotaPlatform,
  PlatformQuotaLimitSetting
>

export interface PlatformQuotaRecord {
  platform: PlatformQuotaPlatform
  daily_usage_usd: number
  daily_limit_usd: number | null
  daily_window_resets_at: string | null
  weekly_usage_usd: number
  weekly_limit_usd: number | null
  weekly_window_resets_at: string | null
  monthly_usage_usd: number
  monthly_limit_usd: number | null
  monthly_window_resets_at: string | null
  daily_window_start?: string | null
  weekly_window_start?: string | null
  monthly_window_start?: string | null
}

export interface PlatformQuotaResponse {
  platform_quotas: PlatformQuotaRecord[]
}

export interface PlatformQuotaUpdateInput {
  platform: PlatformQuotaPlatform
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
}

export function createEmptyPlatformQuotaLimitSettings(): PlatformQuotaLimitSettings {
  return PLATFORM_QUOTA_PLATFORMS.reduce((settings, platform) => {
    settings[platform] = { daily: null, weekly: null, monthly: null }
    return settings
  }, {} as PlatformQuotaLimitSettings)
}

function normalizeLimit(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const numeric = Number(value)
  return Number.isFinite(numeric) && numeric >= 0 ? numeric : null
}

export function normalizePlatformQuotaLimitSettings(
  input: unknown
): PlatformQuotaLimitSettings {
  const normalized = createEmptyPlatformQuotaLimitSettings()
  if (!input || typeof input !== 'object') return normalized

  const source = input as Record<string, unknown>
  for (const platform of PLATFORM_QUOTA_PLATFORMS) {
    const raw = source[platform]
    if (!raw || typeof raw !== 'object') continue
    const limits = raw as Record<string, unknown>
    normalized[platform] = {
      daily: normalizeLimit(limits.daily),
      weekly: normalizeLimit(limits.weekly),
      monthly: normalizeLimit(limits.monthly)
    }
  }
  return normalized
}

export function platformQuotaRecordsToUpdateInputs(
  settings: PlatformQuotaLimitSettings
): PlatformQuotaUpdateInput[] {
  const normalized = normalizePlatformQuotaLimitSettings(settings)
  return PLATFORM_QUOTA_PLATFORMS.map((platform) => ({
    platform,
    daily_limit_usd: normalized[platform].daily,
    weekly_limit_usd: normalized[platform].weekly,
    monthly_limit_usd: normalized[platform].monthly
  }))
}
