import { afterEach, describe, expect, it } from 'vitest'
import { en } from './en'
import { fr } from './fr'
import { getLang, resolveLang, setLang } from './lang'
import { t } from './t'
import { fmtDateTime, fmtKm, fmtPct, fmtRelative, ND } from '../lib/format'

type Tree = { [k: string]: unknown }

/** Leaf paths → message (plural objects flattened as `key#form`). */
function flatten(node: unknown, prefix = '', out = new Map<string, string>()): Map<string, string> {
  for (const [k, v] of Object.entries(node as Tree)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out.set(path, v)
    else if (v && typeof v === 'object' && 'one' in v && 'other' in v) for (const [form, msg] of Object.entries(v)) out.set(`${path}#${form}`, String(msg))
    else if (v && typeof v === 'object') flatten(v, path, out)
    else out.set(path, String(v))
  }
  return out
}

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort()

describe('i18n dictionaries (§15.2)', () => {
  const frMap = flatten(fr)
  const enMap = flatten(en)

  it('has 100 % key parity between fr and en', () => {
    const missingInEn = [...frMap.keys()].filter((k) => !enMap.has(k))
    const extraInEn = [...enMap.keys()].filter((k) => !frMap.has(k))
    expect(missingInEn).toEqual([])
    expect(extraInEn).toEqual([])
    expect(frMap.size).toBeGreaterThan(500)
  })

  it('has no empty message', () => {
    expect([...frMap].filter(([, v]) => !v.trim()).map(([k]) => k)).toEqual([])
    expect([...enMap].filter(([, v]) => !v.trim()).map(([k]) => k)).toEqual([])
  })

  it('uses the same {placeholders} in both languages', () => {
    const mismatched = [...frMap.keys()].filter((k) => {
      const base = k.split('#')[0]
      const enMsg = enMap.get(k) ?? ''
      // Plural forms may drop {count} in one language ("un", "one").
      const strip = (xs: string[]) => xs.filter((x) => x !== 'count')
      const a = placeholders(frMap.get(k) ?? '')
      const b = placeholders(enMsg)
      return base === k ? a.join() !== b.join() : strip(a).join() !== strip(b).join()
    })
    expect(mismatched).toEqual([])
  })
})

describe('t / language resolution', () => {
  const initial = getLang()
  afterEach(() => setLang(initial, { persist: false }))

  it('interpolates and follows the active language', () => {
    setLang('fr', { persist: false })
    expect(t('errors.http.status', { status: 503 })).toBe('Erreur 503 du serveur Stride.')
    setLang('en', { persist: false })
    expect(t('errors.http.status', { status: 503 })).toBe('Stride server error 503.')
    expect(t('common.actions.save')).toBe('Save')
  })

  it('resolves profile → stored → navigator → en', () => {
    expect(resolveLang('fr')).toBe('fr')
    expect(resolveLang('en')).toBe('en')
    expect(['fr', 'en']).toContain(resolveLang(null))
  })

  it('formats numbers and dates per language', () => {
    setLang('fr', { persist: false })
    expect(fmtKm(7020)).toBe('7,02 km')
    expect(fmtPct(49.2)).toBe('49,2 %')
    expect(ND).toBe('N/D')
    expect(fmtRelative('2026-11-02T18:30:00', new Date(2026, 10, 2, 20))).toBe("aujourd'hui 18:30")
    setLang('en', { persist: false })
    expect(fmtKm(7020)).toBe('7.02 km')
    expect(fmtPct(49.2)).toBe('49.2%')
    expect(ND).toBe('N/A')
    expect(fmtRelative('2026-11-01T18:30:00', new Date(2026, 10, 2, 20))).toBe('yesterday 18:30')
    expect(fmtDateTime('2026-11-04T18:00:00')).toMatch(/^4 Nov at 18:00$/)
  })
})
