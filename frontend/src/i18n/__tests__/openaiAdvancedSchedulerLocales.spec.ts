import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const requiredKeys = [
  'stickyWeightedTitle',
  'stickyWeightedDescription',
  'subscriptionPriorityTitle',
  'subscriptionPriorityDescription',
  'weightsTitle',
  'weightsDescription',
  'defaultPlaceholder',
  'topKLabel',
  'priorityWeight',
  'loadWeight',
  'queueWeight',
  'errorRateWeight',
  'ttftWeight',
  'resetWeight',
  'quotaHeadroomWeight',
  'upstreamCostWeight',
  'previousResponseWeight',
  'sessionStickyWeight'
] as const

describe('OpenAI advanced scheduler locale keys', () => {
  it.each([
    ['zh', zh.admin.settings.openaiExperimentalScheduler],
    ['en', en.admin.settings.openaiExperimentalScheduler]
  ])('exposes every advanced scheduler label in %s', (_locale, messages) => {
    for (const key of requiredKeys) {
      expect(messages[key]).toBeTruthy()
    }
  })
})
