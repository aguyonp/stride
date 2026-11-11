import { getLang, LOCALE, t } from '../../i18n'
import { ND } from '../../lib/format'

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export function readFileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result ?? ''))
    r.onerror = () => reject(r.error ?? new Error(t('settings.providers.readFailed')))
    r.readAsText(file)
  })
}

export interface BackupSummary {
  exportedAt: string | null
  goals: number
  plan: number
  shoes: number
  checkins: number
  notes: number
}

/** Parses and validates a Stride backup file; never sends anything. */
export function inspectBackup(text: string): { ok: true; summary: BackupSummary } | { ok: false; error: string } {
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch {
    return { ok: false, error: t('settings.providers.invalidJson') }
  }
  if (!data || typeof data !== 'object' || (data as { format?: unknown }).format !== 'stride-backup') {
    return { ok: false, error: t('settings.providers.notBackup') }
  }
  const d = data as Record<string, unknown>
  const len = (k: string) => (Array.isArray(d[k]) ? (d[k] as unknown[]).length : 0)
  return {
    ok: true,
    summary: {
      exportedAt: typeof d.exportedAt === 'string' ? d.exportedAt : null,
      goals: len('goals'),
      plan: len('plan'),
      shoes: len('shoes'),
      checkins: len('checkins'),
      notes: len('activityNotes'),
    },
  }
}

/** RFC3339 → « 4 nov. 2026 » (browser time, active language). */
export function fmtDay(rfc3339: string | null | undefined): string {
  if (!rfc3339) return ND
  const d = new Date(rfc3339)
  return Number.isNaN(d.getTime()) ? ND : new Intl.DateTimeFormat(LOCALE[getLang()], { day: 'numeric', month: 'short', year: 'numeric' }).format(d)
}
