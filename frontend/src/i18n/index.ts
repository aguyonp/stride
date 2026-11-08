// Stride i18n (§15.2): fr.ts is the source dictionary, en.ts mirrors it; no external library.
// Components use `useI18n()`; non-React modules import `t` directly. A language switch remounts the
// app tree (I18nBoundary), so module-level labels re-evaluate in the new language.

import { createElement, Fragment, useSyncExternalStore, type ReactNode } from 'react'
import {
  capitalize,
  fmtClock,
  fmtDateLong,
  fmtDateShort,
  fmtDateTime,
  fmtDayMonth,
  fmtDuration,
  fmtHM,
  fmtKm,
  fmtMonthYear,
  fmtNumber,
  fmtPace,
  fmtPct,
  fmtRelative,
  fmtSigned,
  fmtTimeOfDay,
  fmtUnit,
  fmtWeekdayShort,
  fmtWhen,
} from '../lib/format'
import { getLang, LOCALE, setLang, subscribeLang, type Lang } from './lang'
import { t, type MessageKey } from './t'

export { detectLang, getLang, isLang, LANGS, LOCALE, resolveLang, setLang, subscribeLang, type Lang } from './lang'
export { hasKey, lazyRecord, t, tDyn, type MessageKey, type Params } from './t'

/** Locale-aware formatters (same functions as lib/format, grouped for `useI18n().fmt`). */
export const fmt = {
  number: fmtNumber,
  unit: fmtUnit,
  km: fmtKm,
  pct: fmtPct,
  signed: fmtSigned,
  duration: fmtDuration,
  hm: fmtHM,
  pace: fmtPace,
  relative: fmtRelative,
  dateLong: fmtDateLong,
  dateShort: fmtDateShort,
  dayMonth: fmtDayMonth,
  weekdayShort: fmtWeekdayShort,
  monthYear: fmtMonthYear,
  timeOfDay: fmtTimeOfDay,
  dateTime: fmtDateTime,
  clock: fmtClock,
  when: fmtWhen,
  capitalize,
  /** « A, B et C » / "A, B and C". */
  list: (items: string[]) => new Intl.ListFormat(LOCALE[getLang()], { style: 'long', type: 'conjunction' }).format(items),
}

const subscribe = (cb: () => void) => subscribeLang(cb)

export function useI18n(): { t: typeof t; lang: Lang; setLang: typeof setLang; fmt: typeof fmt } {
  const lang = useSyncExternalStore(subscribe, getLang, getLang)
  return { t, lang, setLang, fmt }
}

/** `t` with React nodes as parameters: « Tape {text} pour confirmer » with `text: <strong>…</strong>`. */
export function tx(key: MessageKey, params: Record<string, ReactNode>): ReactNode {
  const raw = t(key, Object.fromEntries(Object.keys(params).map((k) => [k, `\u0000${k}\u0000`])))
  return raw.split('\u0000').map((part, i) => (i % 2 === 1 ? createElement(Fragment, { key: i }, params[part]) : part))
}
