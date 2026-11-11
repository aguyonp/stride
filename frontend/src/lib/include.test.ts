import { describe, expect, it } from 'vitest'
import { ALL_INCLUDE, effectiveInclude, parseInclude, serializeInclude } from './include'
import { setLang } from '../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

describe('include chips', () => {
  it('round-trips in canonical order and drops unknown keys', () => {
    expect(parseInclude('history,pain,bogus')).toEqual(['pain', 'history'])
    expect(serializeInclude(['history', 'pain'])).toBe('pain,history')
    expect(parseInclude('')).toEqual([])
    expect(parseInclude(null)).toBeUndefined()
  })
  it('drops session-only chips for the day report and splits for multi export', () => {
    expect(effectiveInclude(ALL_INCLUDE, 'day')).toEqual(['pain', 'health', 'plan', 'history'])
    expect(effectiveInclude(ALL_INCLUDE, 'multi')).not.toContain('splits')
    expect(effectiveInclude(ALL_INCLUDE, 'activity')).toEqual(ALL_INCLUDE)
  })
})
