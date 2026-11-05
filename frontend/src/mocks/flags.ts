// Scenario flags: `?mock=a,b,c` in the page URL at first load, persisted in sessionStorage across SPA navigation.

const KEY = 'stride.mock.flags'
let override: string[] | null = null

function storage(): Storage | null {
  try {
    return typeof window !== 'undefined' ? window.sessionStorage : null
  } catch {
    return null
  }
}

function read(): string[] {
  if (override) return override
  if (typeof window === 'undefined') return []
  const fromUrl = new URLSearchParams(window.location.search).get('mock')
  const st = storage()
  if (fromUrl !== null) {
    const list = fromUrl.split(',').map((s) => s.trim().toLowerCase()).filter(Boolean)
    st?.setItem(KEY, list.join(','))
    return list
  }
  const stored = st?.getItem(KEY)
  return stored ? stored.split(',').filter(Boolean) : []
}

let cached: string[] | null = null

function flags(): string[] {
  if (!cached) cached = read()
  return cached
}

export const has = (f: string) => flags().includes(f)
export const flagsKey = () => [...flags()].sort().join(',')

/** Tests only. */
export function setFlags(list: string[]) {
  override = list
  cached = null
}
