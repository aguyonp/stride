import type { DirectiveRestrictions, PrescribedBy, ShareAuthorRole, ShareNote } from '../../api/types'
import { fmtDateShort, isNum } from '../../lib/format'
import { lazyRecord, t } from '../../i18n'

export const AUTHOR_ROLE_LABEL: Record<ShareAuthorRole, string> = lazyRecord(['physio', 'coach', 'other'] as const, (k) => t(`share.scope.roles.${k}`))
export const AUTHOR_ROLES: ShareAuthorRole[] = ['physio', 'coach', 'other']

export const NO_RESTRICTIONS: DirectiveRestrictions = { noIntervals: false, maxIntensity: null, maxElevLossM: null, cadenceTargetSpm: null }

/** Readable restriction chips: « Pas de fractionné », « Intensité facile », « D− ≤ 150 m », « Cadence 176 spm ». */
export function restrictionChips(r: DirectiveRestrictions | null | undefined): string[] {
  if (!r) return []
  const out: string[] = []
  if (r.noIntervals) out.push(t('share.scope.noIntervals'))
  if (r.maxIntensity === 'easy') out.push(t('share.scope.easy'))
  if (r.maxIntensity === 'moderate') out.push(t('share.scope.moderate'))
  if (isNum(r.maxElevLossM)) out.push(t('share.scope.maxLoss', { value: r.maxElevLossM }))
  if (isNum(r.cadenceTargetSpm)) out.push(t('share.scope.cadence', { value: r.cadenceTargetSpm }))
  return out
}

/** « Claire Martin · Kiné » or « Kiné du sport » when unsigned. */
export function authorLine(n: Pick<ShareNote, 'authorName' | 'authorRole' | 'linkLabel'>): string {
  const role = AUTHOR_ROLE_LABEL[n.authorRole] ?? AUTHOR_ROLE_LABEL.other
  return n.authorName ? `${n.authorName} · ${role}` : `${n.linkLabel} · ${role}`
}

export function validityLine(n: Pick<ShareNote, 'validUntil'>): string {
  return n.validUntil ? t('share.scope.validUntil', { date: fmtDateShort(n.validUntil) }) : t('share.scope.noEnd')
}

/** « Prescrit par Kiné du sport (Claire Martin) ». */
export function prescribedByLine(p: PrescribedBy): string {
  return p.authorName ? t('share.scope.prescribedByAuthor', { label: p.label, author: p.authorName }) : t('share.scope.prescribedBy', { label: p.label })
}

/** A directive stays active while its link lives and `validUntil` is not past (the server filters with `active=1`). */
export function isDirectiveActive(n: ShareNote, today: string): boolean {
  return n.kind === 'directive' && (!n.validUntil || n.validUntil >= today)
}
