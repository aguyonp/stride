// Mock of GET /api/search matching: activity names plus French dates (« marathon », « 14 avril », « avril 2025 », « 14/04 »).

const MONTHS = ['janvier', 'fevrier', 'mars', 'avril', 'mai', 'juin', 'juillet', 'aout', 'septembre', 'octobre', 'novembre', 'decembre']

const normalizeQuery = (s: string) =>
  s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .trim()

export interface ParsedQuery {
  text: string[]
  day: number | null
  month: number | null
  year: number | null
}

function monthOf(token: string): number | null {
  if (token.length < 3) return null
  const i = MONTHS.findIndex((m) => m.startsWith(token.replace(/\.$/, '')))
  return i >= 0 ? i + 1 : null
}

export function parseQuery(q: string): ParsedQuery {
  const out: ParsedQuery = { text: [], day: null, month: null, year: null }
  const tokens = normalizeQuery(q).split(/\s+/).filter(Boolean)
  tokens.forEach((t, i) => {
    const iso = /^(\d{4})-(\d{2})-(\d{2})$/.exec(t)
    const slash = /^(\d{1,2})\/(\d{1,2})(?:\/(\d{2,4}))?$/.exec(t)
    const month = monthOf(t)
    if (iso) {
      out.year = Number(iso[1])
      out.month = Number(iso[2])
      out.day = Number(iso[3])
    } else if (slash) {
      out.day = Number(slash[1])
      out.month = Number(slash[2])
      if (slash[3]) out.year = slash[3].length === 2 ? 2000 + Number(slash[3]) : Number(slash[3])
    } else if (month !== null) {
      out.month = month
    } else if (/^(19|20)\d{2}$/.test(t)) {
      out.year = Number(t)
    } else if (/^\d{1,2}$/.test(t) && Number(t) <= 31 && i + 1 < tokens.length && monthOf(tokens[i + 1]) !== null) {
      out.day = Number(t)
    } else {
      out.text.push(t)
    }
  })
  return out
}

export function matchesActivity(a: { name: string; typeLabel: string; date: string }, q: ParsedQuery): boolean {
  const [y, m, d] = a.date.split('-').map(Number)
  if (q.year !== null && y !== q.year) return false
  if (q.month !== null && m !== q.month) return false
  if (q.day !== null && d !== q.day) return false
  const hay = normalizeQuery(`${a.name} ${a.typeLabel}`)
  return q.text.every((t) => hay.includes(t))
}

export function isEmptyQuery(q: ParsedQuery): boolean {
  return q.text.length === 0 && q.day === null && q.month === null && q.year === null
}
