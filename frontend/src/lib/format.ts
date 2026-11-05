// French formatting helpers. null/undefined/NaN → "N/D" (never 0).

export const ND = 'N/D'
const NBSP = '\u00a0'
const MINUS = '−'

type Num = number | null | undefined

export function isNum(v: Num): v is number {
  return typeof v === 'number' && Number.isFinite(v)
}

const nfCache = new Map<string, Intl.NumberFormat>()
function nf(min: number, max: number): Intl.NumberFormat {
  const key = `${min}-${max}`
  let f = nfCache.get(key)
  if (!f) {
    f = new Intl.NumberFormat('fr-FR', { minimumFractionDigits: min, maximumFractionDigits: max })
    nfCache.set(key, f)
  }
  return f
}

/** `fmtNumber(8432) = "8 432"`, decimal comma. */
export function fmtNumber(v: Num, decimals = 0, opts: { trim?: boolean } = {}): string {
  if (!isNum(v)) return ND
  const out = nf(opts.trim ? 0 : decimals, decimals).format(v)
  return out.replace('-', MINUS)
}

/** `fmtUnit(248, 'ms') = "248 ms"`. */
export function fmtUnit(v: Num, unit: string, decimals = 0): string {
  if (!isNum(v)) return ND
  return `${fmtNumber(v, decimals)}${NBSP}${unit}`
}

/** `fmtKm(7020) = "7,02 km"`. */
export function fmtKm(meters: Num, decimals = 2, withUnit = true): string {
  if (!isNum(meters)) return ND
  const s = fmtNumber(meters / 1000, decimals)
  return withUnit ? `${s}${NBSP}km` : s
}

/** `42:10`, `1:02:10`. */
export function fmtDuration(seconds: Num): string {
  if (!isNum(seconds)) return ND
  const total = Math.round(Math.abs(seconds))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const sign = seconds < 0 ? MINUS : ''
  if (h > 0) return `${sign}${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${sign}${m}:${String(s).padStart(2, '0')}`
}

/** `1h22`, `45 min`. */
export function fmtHM(seconds: Num): string {
  if (!isNum(seconds)) return ND
  const totalMin = Math.round(seconds / 60)
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  if (h === 0) return `${m}${NBSP}min`
  return `${h}h${String(m).padStart(2, '0')}`
}

/** `fmtPaceValue(343) = "5:43"`. */
export function fmtPaceValue(secPerKm: Num): string {
  if (!isNum(secPerKm) || secPerKm <= 0) return ND
  let m = Math.floor(secPerKm / 60)
  let s = Math.round(secPerKm - m * 60)
  if (s === 60) {
    m += 1
    s = 0
  }
  return `${m}:${String(s).padStart(2, '0')}`
}

/** `fmtPace(343) = "5:43/km"`. */
export function fmtPace(secPerKm: Num): string {
  const v = fmtPaceValue(secPerKm)
  return v === ND ? ND : `${v}/km`
}

/** Parses `m:ss` → seconds per km, null when invalid. */
export function parsePace(input: string): number | null {
  const m = /^\s*(\d{1,2})\s*[:'’]\s*([0-5]\d)\s*$/.exec(input)
  if (!m) return null
  const v = Number(m[1]) * 60 + Number(m[2])
  return v > 0 ? v : null
}

/** Parses `h:mm:ss`, `mm:ss` or plain minutes (`45`, `45,5`) → seconds, null when invalid. */
export function parseDuration(input: string): number | null {
  const t = input.trim()
  if (!t) return null
  const hms = /^(\d{1,2})[:h](\d{1,2})[:'’](\d{1,2})$/.exec(t)
  if (hms) {
    const [h, m, s] = hms.slice(1).map(Number)
    return m < 60 && s < 60 && h * 3600 + m * 60 + s > 0 ? h * 3600 + m * 60 + s : null
  }
  const ms = /^(\d{1,3})[:'’](\d{1,2})$/.exec(t)
  if (ms) {
    const [m, s] = ms.slice(1).map(Number)
    return s < 60 && m * 60 + s > 0 ? m * 60 + s : null
  }
  const n = Number(t.replace(',', '.'))
  return Number.isFinite(n) && n > 0 ? Math.round(n * 60) : null
}

/** `durationInput(6080) = "1:41:20"`, `durationInput(1361) = "22:41"`. */
export function durationInput(seconds: number | null | undefined): string {
  return isNum(seconds) && seconds > 0 ? fmtDuration(seconds) : ''
}

/** `fmtPct(49.2) = "49,2 %"`. */
export function fmtPct(v: Num, decimals = 1): string {
  if (!isNum(v)) return ND
  return `${fmtNumber(v, decimals)}${NBSP}%`
}

/** `fmtSigned(4) = "+4"`, `fmtSigned(-3.2, 1) = "−3,2"`. */
export function fmtSigned(v: Num, decimals = 0): string {
  if (!isNum(v)) return ND
  const abs = fmtNumber(Math.abs(v), decimals)
  if (Math.abs(v) < 10 ** -decimals / 2) return abs
  return `${v > 0 ? '+' : MINUS}${abs}`
}


interface LocalParts {
  y: number
  mo: number // 1-12
  d: number
  h: number
  mi: number
}

/** Parses "2026-11-02T18:30:12" or "2026-11-02" without time zone conversion. */
export function parseLocal(s: string): LocalParts | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2}))?/.exec(s)
  if (!m) return null
  return { y: +m[1], mo: +m[2], d: +m[3], h: m[4] ? +m[4] : 0, mi: m[5] ? +m[5] : 0 }
}

function partsToDate(p: LocalParts): Date {
  return new Date(p.y, p.mo - 1, p.d, p.h, p.mi)
}

export function toISODate(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** Today's ISO date in the given time zone (browser's otherwise). */
export function todayISO(timezone?: string, now: Date = new Date()): string {
  if (timezone) {
    try {
      return new Intl.DateTimeFormat('en-CA', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(now)
    } catch {
    }
  }
  return toISODate(now)
}

export function addDays(iso: string, n: number): string {
  const p = parseLocal(iso)
  if (!p) return iso
  const d = new Date(p.y, p.mo - 1, p.d + n)
  return toISODate(d)
}

/** Calendar days b − a. */
export function diffDays(aIso: string, bIso: string): number {
  const a = parseLocal(aIso)
  const b = parseLocal(bIso)
  if (!a || !b) return 0
  return Math.round((Date.UTC(b.y, b.mo - 1, b.d) - Date.UTC(a.y, a.mo - 1, a.d)) / 86_400_000)
}

const dtf = (o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat('fr-FR', o)
const F_WEEKDAY = dtf({ weekday: 'long' })
const F_DAY_MONTH = dtf({ day: 'numeric', month: 'short' })
const F_DAY_MONTH_YEAR = dtf({ day: 'numeric', month: 'short', year: 'numeric' })
const F_LONG = dtf({ weekday: 'long', day: 'numeric', month: 'long' })
const F_LONG_YEAR = dtf({ weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
const F_MONTH_YEAR = dtf({ month: 'long', year: 'numeric' })
const F_SHORT_WD = dtf({ weekday: 'short' })

function hhmm(p: LocalParts): string {
  return `${String(p.h).padStart(2, '0')}:${String(p.mi).padStart(2, '0')}`
}

/** « aujourd'hui 18:30 », « hier 18:30 », « lundi 18:30 », « 28 oct. 18:30 », « 28 oct. 2025 ». */
export function fmtRelative(startLocal: string, now: Date = new Date()): string {
  const p = parseLocal(startLocal)
  if (!p) return ND
  const days = diffDays(`${p.y}-${String(p.mo).padStart(2, '0')}-${String(p.d).padStart(2, '0')}`, toISODate(now))
  const time = hhmm(p)
  const hasTime = /[T ]\d{2}:\d{2}/.test(startLocal)
  const suffix = hasTime ? ` ${time}` : ''
  if (days === 0) return `aujourd'hui${suffix}`
  if (days === 1) return `hier${suffix}`
  if (days > 1 && days < 7) return `${F_WEEKDAY.format(partsToDate(p))}${suffix}`
  if (p.y === now.getFullYear()) return `${F_DAY_MONTH.format(partsToDate(p))}${suffix}`
  return F_DAY_MONTH_YEAR.format(partsToDate(p))
}

/** « lundi 2 novembre » (+ year when not the current one). */
export function fmtDateLong(iso: string, now: Date = new Date()): string {
  const p = parseLocal(iso)
  if (!p) return ND
  const d = partsToDate(p)
  return p.y === now.getFullYear() ? F_LONG.format(d) : F_LONG_YEAR.format(d)
}

/** « 2 nov. » */
export function fmtDateShort(iso: string): string {
  const p = parseLocal(iso)
  return p ? F_DAY_MONTH.format(partsToDate(p)) : ND
}

/** « 02/11 » */
export function fmtDayMonth(iso: string): string {
  const p = parseLocal(iso)
  return p ? `${String(p.d).padStart(2, '0')}/${String(p.mo).padStart(2, '0')}` : ND
}

/** « lun. » */
export function fmtWeekdayShort(iso: string): string {
  const p = parseLocal(iso)
  return p ? F_SHORT_WD.format(partsToDate(p)) : ND
}

/** « novembre 2026 » */
export function fmtMonthYear(year: number, month0: number): string {
  return F_MONTH_YEAR.format(new Date(year, month0, 1))
}

/** « 18:30 » from startLocal. */
export function fmtTimeOfDay(startLocal: string): string {
  const p = parseLocal(startLocal)
  return p ? hhmm(p) : ND
}

/** RFC3339 → « 4 nov. à 18:00 » (browser time). */
export function fmtDateTime(rfc3339: string | null | undefined): string {
  if (!rfc3339) return ND
  const d = new Date(rfc3339)
  if (Number.isNaN(d.getTime())) return ND
  return `${F_DAY_MONTH.format(d)} à ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}


export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

const pad2 = (n: number) => String(n).padStart(2, '0')

/** RFC3339 → "18:42" (browser time zone). */
export function fmtClock(rfc3339: string | null | undefined): string {
  if (!rfc3339) return ND
  const d = new Date(rfc3339)
  if (Number.isNaN(d.getTime())) return ND
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

/** RFC3339 → "18:42" today, "hier 18:42", "4 nov. 18:42". */
export function fmtWhen(rfc3339: string | null | undefined, now: Date = new Date()): string {
  if (!rfc3339) return ND
  const d = new Date(rfc3339)
  if (Number.isNaN(d.getTime())) return ND
  const days = diffDays(toISODate(d), toISODate(now))
  const t = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  if (days === 0) return t
  if (days === 1) return `hier ${t}`
  return `${F_DAY_MONTH.format(d)} ${t}`
}

/** Monday (ISO) of the week containing `iso`. */
export function mondayOf(iso: string): string {
  const p = parseLocal(iso)
  if (!p) return iso
  const d = new Date(p.y, p.mo - 1, p.d)
  const offset = (d.getDay() + 6) % 7
  return toISODate(new Date(p.y, p.mo - 1, p.d - offset))
}

/** ≈ chars / 4, the rule of thumb used by the backend. */
export function estimateTokens(text: string | null | undefined): number {
  return text ? Math.ceil(text.length / 4) : 0
}

/** Seconds → "m:ss" input value, empty when null. */
export function paceInput(secPerKm: number | null | undefined): string {
  return isNum(secPerKm) && secPerKm > 0 ? fmtPaceValue(secPerKm) : ''
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (!parts.length) return '?'
  return (parts[0][0] + (parts.length > 1 ? parts[parts.length - 1][0] : '')).toUpperCase()
}
