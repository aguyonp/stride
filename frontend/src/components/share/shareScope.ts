import type { ShareExpiryDays, ShareLink, ShareLinkInput, SharePeriodWeeks, SharePermissions, ShareSection } from '../../api/types'
import { t } from '../../i18n'

export const SHARE_SECTIONS: { key: ShareSection; label: string }[] = (['activities', 'load', 'recovery', 'health', 'plan', 'records', 'shoes', 'notes'] as const).map((key) => ({
  key,
  get label() {
    return t(`share.scope.sections.${key}`)
  },
}))
export const ALL_SHARE_SECTIONS = SHARE_SECTIONS.map((s) => s.key)

const choice = <V>(value: V, label: () => string) => ({
  value,
  get label() {
    return label()
  },
})

export const EXPIRY_CHOICES: { value: ShareExpiryDays | null; label: string }[] = [
  ...([1, 7, 30, 90] as const).map((n) => choice<ShareExpiryDays | null>(n, () => t('share.scope.days', { count: n }))),
  choice<ShareExpiryDays | null>(null, () => t('share.scope.noLimit')),
]

export const PERIOD_CHOICES: { value: SharePeriodWeeks; label: string }[] = [
  ...([4, 8, 12] as const).map((n) => choice<SharePeriodWeeks>(n, () => t('share.scope.weeks', { n }))),
  choice<SharePeriodWeeks>(26, () => t('share.scope.sixMonths')),
  choice<SharePeriodWeeks>(52, () => t('share.scope.oneYear')),
]

/** What the recipient may write (§15.1): everything off by default, the link stays read only. */
export const NO_PERMISSIONS: SharePermissions = { comments: false, planning: false, activityDetails: false }

export const SHARE_PERMISSIONS: { key: keyof SharePermissions; label: string; description: string }[] = (['comments', 'planning', 'activityDetails'] as const).map((key) => ({
  key,
  get label() {
    return t(`share.scope.permissions.${key}.label`)
  },
  get description() {
    return t(`share.scope.permissions.${key}.description`)
  },
}))

export const DEFAULT_SHARE_INPUT: ShareLinkInput = {
  get label() {
    return t('share.scope.defaultLabel')
  },
  expiresInDays: 30,
  periodWeeks: 12,
  sections: ALL_SHARE_SECTIONS,
  anonymize: false,
  permissions: NO_PERMISSIONS,
}

/** Edit form: `keep` leaves the current expiry untouched, a number or null restarts it from now. */
export type ShareForm = Omit<ShareLinkInput, 'expiresInDays'> & { expiresInDays: ShareExpiryDays | null | 'keep' }

export function formFromLink(link: ShareLink | null): ShareForm {
  if (!link) return DEFAULT_SHARE_INPUT
  return { label: link.label, expiresInDays: 'keep', periodWeeks: link.periodWeeks, sections: link.sections, anonymize: link.anonymize, permissions: { ...NO_PERMISSIONS, ...link.permissions } }
}

export function toShareInput(f: ShareForm): Partial<ShareLinkInput> {
  const sections = ALL_SHARE_SECTIONS.filter((k) => f.sections.includes(k))
  const base = { label: f.label.trim(), periodWeeks: f.periodWeeks, sections, anonymize: f.anonymize, permissions: f.permissions }
  return f.expiresInDays === 'keep' ? base : { ...base, expiresInDays: f.expiresInDays }
}

export function validateShareForm(f: ShareForm): string | null {
  const n = f.label.trim().length
  if (n < 2 || n > 80) return t('share.scope.labelLength')
  if (f.sections.length === 0) return t('share.scope.sectionRequired')
  return null
}

export function linkStatus(link: ShareLink, nowMs: number): { label: string; cls: string; active: boolean } {
  if (link.revoked) return { label: t('share.scope.revoked'), cls: 'border-line bg-track text-ink-3', active: false }
  if (link.expiresAt && new Date(link.expiresAt).getTime() <= nowMs) return { label: t('share.scope.expired'), cls: 'border-warn/35 bg-warn/12 text-warn', active: false }
  return { label: t('share.scope.active'), cls: 'border-ok/35 bg-ok/12 text-ok', active: true }
}

/** Short list of what the link lets its recipient write, null when read only. */
export function permissionsSummary(p: SharePermissions | undefined): string | null {
  const on = SHARE_PERMISSIONS.filter((x) => p?.[x.key])
  if (on.length === 0) return null
  return on.map((x) => x.label.toLowerCase()).join(', ')
}

export function scopeSummary(link: Pick<ShareLink, 'periodWeeks' | 'sections' | 'anonymize'>): string {
  const period = PERIOD_CHOICES.find((p) => p.value === link.periodWeeks)?.label ?? t('share.scope.weeks', { n: link.periodWeeks })
  const sections =
    link.sections.length === ALL_SHARE_SECTIONS.length
      ? t('share.scope.wholeFile')
      : SHARE_SECTIONS.filter((s) => link.sections.includes(s.key))
          .map((s) => s.label)
          .join(', ')
  return [period, sections, link.anonymize ? t('share.scope.anonymized') : null].filter(Boolean).join(' · ')
}
