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
    r.onerror = () => reject(r.error ?? new Error('Lecture impossible'))
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
    return { ok: false, error: 'Ce fichier n’est pas un JSON valide.' }
  }
  if (!data || typeof data !== 'object' || (data as { format?: unknown }).format !== 'stride-backup') {
    return { ok: false, error: 'Ce fichier n’est pas une sauvegarde Stride (format « stride-backup » attendu).' }
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

const F_DAY = new Intl.DateTimeFormat('fr-FR', { day: 'numeric', month: 'short', year: 'numeric' })

/** RFC3339 → « 4 nov. 2026 ». */
export function fmtDay(rfc3339: string | null | undefined): string {
  if (!rfc3339) return 'N/D'
  const d = new Date(rfc3339)
  return Number.isNaN(d.getTime()) ? 'N/D' : F_DAY.format(d)
}
