import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { qk, useProfile, useSaveProfile } from '../api/queries'
import type { AthleteProfile } from '../api/types'
import { STORAGE_KEYS, writeStored } from '../lib/storage'
import { detectLang, getLang, isLang, setLang, type Lang } from './lang'

/** Applies `profile.language` once the profile is loaded (signed-in shell only). */
export function useProfileLanguage(enabled: boolean) {
  const profile = useProfile(enabled)
  const lang = profile.data?.language
  useEffect(() => {
    if (isLang(lang)) setLang(lang)
  }, [lang])
}

export type LanguagePref = Lang | 'auto'

/**
 * Language preference for Réglages and Cmd+K: persisted to the profile (PUT /api/profile) when signed
 * in, and to localStorage. `auto` stores `language: null` and follows the browser again.
 */
export function useLanguageSwitch(signedIn = true) {
  const qc = useQueryClient()
  const profile = useProfile(signedIn)
  const save = useSaveProfile()
  const pref: LanguagePref = profile.data ? (profile.data.language ?? 'auto') : getLang()

  const choose = (next: LanguagePref) => {
    const p = profile.data
    if (signedIn && p) {
      const updated: AthleteProfile = { ...p, language: next === 'auto' ? null : next }
      qc.setQueryData(qk.profile, updated)
      save.mutate(updated)
    }
    if (next === 'auto') {
      writeStored(STORAGE_KEYS.lang, null)
      setLang(detectLang(), { persist: false })
    } else setLang(next)
  }
  return { pref, choose, pending: save.isPending }
}
