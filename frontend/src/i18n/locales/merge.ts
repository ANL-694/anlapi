type LocaleMessages = Record<string, unknown>

function isLocaleMessages(value: unknown): value is LocaleMessages {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function mergeLocaleMessages<
  TBase extends LocaleMessages,
  TOverrides extends LocaleMessages
>(base: TBase, overrides: TOverrides): TBase & TOverrides {
  const merged: LocaleMessages = { ...base }

  for (const [key, value] of Object.entries(overrides)) {
    const baseValue = merged[key]
    merged[key] =
      isLocaleMessages(baseValue) && isLocaleMessages(value)
        ? mergeLocaleMessages(baseValue, value)
        : value
  }

  return merged as TBase & TOverrides
}
