// Translation function (§15.2): dotted keys typed from fr.ts, `{name}` interpolation, plural
// messages ({ one, other }) picked with Intl.PluralRules on `count`. No React dependency.

import { en } from './en'
import { fr } from './fr'
import { getLang, type Lang } from './lang'

type PluralForm = 'zero' | 'one' | 'two' | 'few' | 'many' | 'other'
/** Plural message: `{ one, other }` (+ optional CLDR forms), selected on `params.count`. */
export type PluralMessage = { readonly one: string; readonly other: string } & { readonly [F in Exclude<PluralForm, 'one' | 'other'>]?: string }
type Message = string | PluralMessage

type Leaves<T, P extends string = ''> = {
  [K in keyof T & string]: T[K] extends string ? `${P}${K}` : T[K] extends { readonly one: string; readonly other: string } ? `${P}${K}` : Leaves<T[K], `${P}${K}.`>
}[keyof T & string]

/** Every translatable key, e.g. `'nav.dashboard'`. */
export type MessageKey = Leaves<typeof fr>
export type Params = Record<string, string | number | null | undefined>

/** en.ts (and each en/ namespace) mirrors fr with plain strings: same keys, same plural forms. */
export type Widen<T> = T extends string ? string : { -readonly [K in keyof T]: Widen<T[K]> }

const DICTS: Record<Lang, unknown> = { fr, en }

function lookup(lang: Lang, key: string): Message | undefined {
  let node: unknown = DICTS[lang]
  for (const part of key.split('.')) {
    if (typeof node !== 'object' || node === null) return undefined
    node = (node as Record<string, unknown>)[part]
  }
  if (typeof node === 'string') return node
  if (typeof node === 'object' && node !== null && typeof (node as PluralMessage).one === 'string' && typeof (node as PluralMessage).other === 'string') return node as PluralMessage
  return undefined
}

const pluralCache = new Map<Lang, Intl.PluralRules>()
function plural(lang: Lang, n: number): PluralForm {
  let pr = pluralCache.get(lang)
  if (!pr) {
    pr = new Intl.PluralRules(lang)
    pluralCache.set(lang, pr)
  }
  return pr.select(n) as PluralForm
}

export function interpolate(msg: string, params?: Params): string {
  if (!params) return msg
  return msg.replace(/\{(\w+)\}/g, (all, name: string) => {
    const v = params[name]
    return v === undefined || v === null ? all : String(v)
  })
}

/** True when `key` is a message in the active dictionary (dynamic keys built from API values). */
export function hasKey(key: string): key is MessageKey {
  return lookup(getLang(), key) !== undefined
}

/** Translates `key` in the active language (`lang` overrides it). */
export function t(key: MessageKey, params?: Params, lang: Lang = getLang()): string {
  const msg = lookup(lang, key) ?? lookup('fr', key)
  if (msg === undefined) return key
  if (typeof msg === 'string') return interpolate(msg, params)
  const n = Number(params?.count ?? 0)
  return interpolate(msg[plural(lang, n)] ?? msg.other, params)
}

/** `t` for keys built at runtime; returns `fallback` (or the key) when unknown. */
export function tDyn(key: string, params?: Params, fallback?: string): string {
  return hasKey(key) ? t(key, params) : (fallback ?? key)
}

/**
 * Record whose values are computed on each read, so module-level label tables (`SPORT_LABEL[s]`,
 * `Object.entries(…)`) follow the active language without changing their call sites.
 */
export function lazyRecord<K extends PropertyKey, V>(keys: readonly K[], get: (k: K) => V): Record<K, V> {
  const out = {} as Record<K, V>
  for (const k of keys) Object.defineProperty(out, k, { get: () => get(k), enumerable: true })
  return out
}
