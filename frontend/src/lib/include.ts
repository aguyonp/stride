import type { IncludeKey } from '../api/types'
import { t } from '../i18n'

export interface IncludeChip {
  key: IncludeKey
  label: string
  hint: string
  scope: 'session' | 'all'
}

const chip = (key: IncludeKey, scope: IncludeChip['scope']): IncludeChip => ({
  key,
  scope,
  get label() {
    return t(`coach.prompt.chips.${key}.label`)
  },
  get hint() {
    return t(`coach.prompt.chips.${key}.hint`)
  },
})

export const INCLUDE_CHIPS: IncludeChip[] = [
  chip('pain', 'all'),
  chip('biomech', 'session'),
  chip('health', 'all'),
  chip('plan', 'all'),
  chip('load', 'session'),
  chip('splits', 'session'),
  chip('history', 'all'),
]

export const ALL_INCLUDE: IncludeKey[] = INCLUDE_CHIPS.map((c) => c.key)

export function parseInclude(raw: string | null): IncludeKey[] | undefined {
  if (raw === null) return undefined
  if (raw === '') return []
  const keys = raw.split(',').filter((k): k is IncludeKey => (ALL_INCLUDE as string[]).includes(k))
  return ALL_INCLUDE.filter((k) => keys.includes(k))
}

export const serializeInclude = (keys: IncludeKey[]) => ALL_INCLUDE.filter((k) => keys.includes(k)).join(',')

/** Keys sent for a subject: session-only chips are dropped for the day report. */
export function effectiveInclude(keys: IncludeKey[], subject: 'activity' | 'day' | 'multi'): IncludeKey[] {
  return keys.filter((k) => {
    const chip = INCLUDE_CHIPS.find((c) => c.key === k)
    if (!chip) return false
    if (subject === 'day') return chip.scope === 'all'
    if (subject === 'multi') return k !== 'splits'
    return true
  })
}
