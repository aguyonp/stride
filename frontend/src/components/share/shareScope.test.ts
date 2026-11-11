import { describe, expect, it } from 'vitest'
import type { ShareLink } from '../../api/types'
import { ALL_SHARE_SECTIONS, DEFAULT_SHARE_INPUT, formFromLink, linkStatus, NO_PERMISSIONS, permissionsSummary, scopeSummary, toShareInput, validateShareForm } from './shareScope'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const link = (over: Partial<ShareLink> = {}): ShareLink => ({
  id: 1,
  label: 'Kiné du sport',
  tokenPrefix: 'sh_abc12',
  url: '/share/sh_abc12345',
  expiresAt: null,
  revoked: false,
  revokedAt: null,
  accessCount: 0,
  lastAccessedAt: null,
  createdAt: '2026-11-01T10:00:00Z',
  periodWeeks: 12,
  sections: ALL_SHARE_SECTIONS,
  anonymize: false,
  permissions: NO_PERMISSIONS,
  ...over,
})

describe('share scope', () => {
  it('creates with full defaults and edits without touching the expiry unless asked', () => {
    expect(toShareInput(formFromLink(null))).toEqual(DEFAULT_SHARE_INPUT)
    const edit = formFromLink(link({ periodWeeks: 4, sections: ['shoes', 'health'], anonymize: true }))
    expect(toShareInput(edit)).toEqual({ label: 'Kiné du sport', periodWeeks: 4, sections: ['health', 'shoes'], anonymize: true, permissions: NO_PERMISSIONS })
    expect(toShareInput({ ...edit, expiresInDays: null })).toMatchObject({ expiresInDays: null })
    expect(toShareInput({ ...edit, expiresInDays: 7 })).toMatchObject({ expiresInDays: 7 })
  })

  it('keeps the three portal permissions off by default and round-trips them on edit', () => {
    expect(DEFAULT_SHARE_INPUT.permissions).toEqual({ comments: false, planning: false, activityDetails: false })
    const perms = { comments: true, planning: false, activityDetails: true }
    expect(toShareInput(formFromLink(link({ permissions: perms }))).permissions).toEqual(perms)
    // 4.1 links (no permissions in the payload) stay read only
    expect(formFromLink(link({ permissions: undefined as never })).permissions).toEqual(NO_PERMISSIONS)
    expect(permissionsSummary(NO_PERMISSIONS)).toBeNull()
    expect(permissionsSummary(perms)).toBe('consignes et remarques, détail des séances')
  })

  it('validates the label and requires at least one section', () => {
    expect(validateShareForm(DEFAULT_SHARE_INPUT)).toBeNull()
    expect(validateShareForm({ ...DEFAULT_SHARE_INPUT, label: ' a ' })).toMatch(/intitulé/)
    expect(validateShareForm({ ...DEFAULT_SHARE_INPUT, sections: [] })).toMatch(/rubrique/)
  })

  it('summarises scope and status in plain French', () => {
    expect(scopeSummary(link())).toBe('12 sem. · tout le dossier')
    expect(scopeSummary(link({ periodWeeks: 52, sections: ['shoes', 'activities'], anonymize: true }))).toBe('1 an · Séances, Chaussures · anonymisé')
    const now = Date.parse('2026-11-07T12:00:00Z')
    expect(linkStatus(link(), now).label).toBe('Actif')
    expect(linkStatus(link({ expiresAt: '2026-11-06T00:00:00Z' }), now).active).toBe(false)
    expect(linkStatus(link({ revoked: true }), now).label).toBe('Révoqué')
  })
})
