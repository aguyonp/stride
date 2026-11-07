import type { ShareExpiryDays, ShareLink, ShareLinkInput, SharePeriodWeeks, ShareSection } from '../../api/types'

export const SHARE_SECTIONS: { key: ShareSection; label: string }[] = [
  { key: 'activities', label: 'Séances' },
  { key: 'load', label: 'Charge' },
  { key: 'recovery', label: 'Récupération' },
  { key: 'health', label: 'Santé & douleurs' },
  { key: 'plan', label: 'Planning' },
  { key: 'records', label: 'Records' },
  { key: 'shoes', label: 'Chaussures' },
  { key: 'notes', label: 'Notes & ressenti' },
]
export const ALL_SHARE_SECTIONS = SHARE_SECTIONS.map((s) => s.key)

export const EXPIRY_CHOICES: { value: ShareExpiryDays | null; label: string }[] = [
  { value: 1, label: '1 jour' },
  { value: 7, label: '7 jours' },
  { value: 30, label: '30 jours' },
  { value: 90, label: '90 jours' },
  { value: null, label: 'Sans limite' },
]

export const PERIOD_CHOICES: { value: SharePeriodWeeks; label: string }[] = [
  { value: 4, label: '4 sem.' },
  { value: 8, label: '8 sem.' },
  { value: 12, label: '12 sem.' },
  { value: 26, label: '6 mois' },
  { value: 52, label: '1 an' },
]

export const DEFAULT_SHARE_INPUT: ShareLinkInput = {
  label: 'Kiné du sport',
  expiresInDays: 30,
  periodWeeks: 12,
  sections: ALL_SHARE_SECTIONS,
  anonymize: false,
}

/** Edit form: `keep` leaves the current expiry untouched, a number or null restarts it from now. */
export type ShareForm = Omit<ShareLinkInput, 'expiresInDays'> & { expiresInDays: ShareExpiryDays | null | 'keep' }

export function formFromLink(link: ShareLink | null): ShareForm {
  if (!link) return DEFAULT_SHARE_INPUT
  return { label: link.label, expiresInDays: 'keep', periodWeeks: link.periodWeeks, sections: link.sections, anonymize: link.anonymize }
}

export function toShareInput(f: ShareForm): Partial<ShareLinkInput> {
  const sections = ALL_SHARE_SECTIONS.filter((k) => f.sections.includes(k))
  const base = { label: f.label.trim(), periodWeeks: f.periodWeeks, sections, anonymize: f.anonymize }
  return f.expiresInDays === 'keep' ? base : { ...base, expiresInDays: f.expiresInDays }
}

export function validateShareForm(f: ShareForm): string | null {
  const n = f.label.trim().length
  if (n < 2 || n > 80) return 'Indique un intitulé de 2 à 80 caractères.'
  if (f.sections.length === 0) return 'Choisis au moins une rubrique à partager.'
  return null
}

export function linkStatus(link: ShareLink, nowMs: number): { label: string; cls: string; active: boolean } {
  if (link.revoked) return { label: 'Révoqué', cls: 'border-line bg-track text-ink-3', active: false }
  if (link.expiresAt && new Date(link.expiresAt).getTime() <= nowMs) return { label: 'Expiré', cls: 'border-warn/35 bg-warn/12 text-warn', active: false }
  return { label: 'Actif', cls: 'border-ok/35 bg-ok/12 text-ok', active: true }
}

export function scopeSummary(link: Pick<ShareLink, 'periodWeeks' | 'sections' | 'anonymize'>): string {
  const period = PERIOD_CHOICES.find((p) => p.value === link.periodWeeks)?.label ?? `${link.periodWeeks} sem.`
  const sections =
    link.sections.length === ALL_SHARE_SECTIONS.length
      ? 'tout le dossier'
      : SHARE_SECTIONS.filter((s) => link.sections.includes(s.key))
          .map((s) => s.label)
          .join(', ')
  return [period, sections, link.anonymize ? 'anonymisé' : null].filter(Boolean).join(' · ')
}
