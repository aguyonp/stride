import { ArrowRight, Loader2, Save, UserRound } from 'lucide-react'
import { Link } from 'wouter'
import { useId, useMemo, useState } from 'react'
import { useProfile, usePresets, useSaveProfile, useUpdateMe } from '../../api/queries'
import type { AthleteProfile, FocusPreset, Me } from '../../api/types'
import { PaceTargetsEditor } from '../../components/profile/PaceTargetsEditor'
import {
  draftToPaces,
  draftToThresholds,
  focusTitle,
  isValidTimezone,
  paceToDraft,
  presetFor,
  thresholdsToDraft,
  TIMEZONES,
} from '../../components/profile/profileUtils'
import { ThresholdsEditor } from '../../components/profile/ThresholdsEditor'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { ErrorView } from '../StateViews'
import { Section } from './Section'

export function ProfileSection({ me }: { me: Me }) {
  const profile = useProfile()
  const presets = usePresets()
  return (
    <Section id="profil" title="Profil & seuils" icon={<UserRound />} description="Seuils d’alerte et allures cibles utilisés dans les analyses et les prompts.">
      {profile.isError ? (
        <ErrorView error={profile.error} onRetry={() => void profile.refetch()} title="Profil indisponible" />
      ) : !profile.data ? (
        <div className="space-y-3">
          <Skeleton className="h-11" />
          <Skeleton className="h-40" />
          <Skeleton className="h-56" />
        </div>
      ) : (
        <ProfileForm key={profile.dataUpdatedAt} me={me} profile={profile.data} presets={presets.data} />
      )}
    </Section>
  )
}

function ProfileForm({ me, profile, presets }: { me: Me; profile: AthleteProfile; presets: FocusPreset[] | undefined }) {
  const ids = useId()
  const { toast } = useToast()
  const save = useSaveProfile()
  const updateMe = useUpdateMe()
  const [name, setName] = useState(me.displayName)
  const [tz, setTz] = useState(profile.timezone)
  const initialTh = useMemo(() => thresholdsToDraft(profile.thresholds), [profile.thresholds])
  const initialPaces = useMemo(() => profile.paceTargets.map(paceToDraft), [profile.paceTargets])
  const [th, setTh] = useState(initialTh)
  const [paces, setPaces] = useState(initialPaces)
  const [showErrors, setShowErrors] = useState(false)

  const area = profile.focusArea
  const preset = presetFor(presets, area)
  const thCheck = draftToThresholds(th)
  const paceCheck = draftToPaces(paces)
  const nameError = name.trim() === '' ? 'Nom requis' : null
  const tzError = !isValidTimezone(tz.trim()) ? 'Fuseau horaire inconnu (ex. Europe/Paris)' : null

  const dirty =
    name.trim() !== me.displayName ||
    tz.trim() !== profile.timezone ||
    JSON.stringify(th) !== JSON.stringify(initialTh) ||
    JSON.stringify(paces) !== JSON.stringify(initialPaces)

  const busy = save.isPending || updateMe.isPending



  const submit = async () => {
    setShowErrors(true)
    if (nameError || tzError || !thCheck.value || !paceCheck.value) return
    try {
      if (name.trim() !== me.displayName) await updateMe.mutateAsync({ displayName: name.trim() })
      await save.mutateAsync({
        focusArea: profile.focusArea,
        focusLabel: profile.focusLabel,
        injuredSide: profile.injuredSide,
        timezone: tz.trim(),
        thresholds: thCheck.value,
        paceTargets: paceCheck.value,
      })
      toast({ title: 'Profil enregistré', description: 'Les analyses et prompts utilisent désormais ces réglages.' })
    } catch (e) {
      toast({ tone: 'error', title: 'Enregistrement impossible', description: e instanceof Error ? e.message : undefined })
    }
  }

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
      noValidate
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="min-w-0">
          <label htmlFor={`${ids}-name`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Nom affiché
          </label>
          <input id={`${ids}-name`} className="field" value={name} maxLength={60} autoComplete="nickname" aria-invalid={showErrors && nameError ? true : undefined} onChange={(e) => setName(e.target.value)} />
          {showErrors && nameError && <p className="mt-1 text-xs text-danger">{nameError}</p>}
        </div>
        <div className="min-w-0">
          <label htmlFor={`${ids}-tz`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Fuseau horaire
          </label>
          <input id={`${ids}-tz`} className="field" list={`${ids}-tzl`} value={tz} autoComplete="off" spellCheck={false} aria-invalid={tzError ? true : undefined} onChange={(e) => setTz(e.target.value)} />
          <datalist id={`${ids}-tzl`}>
            {TIMEZONES.map((z) => (
              <option key={z} value={z} />
            ))}
          </datalist>
          <p className={`mt-1 text-xs ${tzError ? 'text-danger' : 'text-ink-3'}`}>{tzError ?? 'Sert à définir « aujourd’hui » et les journées de récupération.'}</p>
        </div>
      </div>

      <p className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-line bg-track/35 px-3.5 py-2.5 text-sm text-ink-2">
        <span>
          Zone suivie : <span className="font-medium text-ink">{profile.focusArea === 'none' ? 'aucune (bonne santé)' : profile.focusLabel}</span>
        </span>
        <Link href="/health" className="inline-flex items-center gap-1 text-xs font-semibold text-accent hover:underline">
          Gérer dans Santé & forme <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
      </p>

      <div>
        <h3 className="mb-1 text-sm font-semibold text-ink">Seuils d’alerte</h3>
        <p className="mb-3 text-xs text-ink-3">Valeurs par défaut issues du preset « {focusTitle(presets, area)} », modifiables. Les champs personnalisés sont soulignés.</p>
        <ThresholdsEditor draft={th} defaults={preset?.thresholds ?? null} errors={showErrors ? thCheck.errors : {}} onChange={setTh} onReset={preset ? () => setTh(thresholdsToDraft(preset.thresholds)) : undefined} />
      </div>

      <div>
        <h3 className="mb-1 text-sm font-semibold text-ink">Allures cibles</h3>
        <p className="mb-3 text-xs text-ink-3">Fourchettes au format m:ss par km, utilisées pour le temps passé à allure cible.</p>
        <PaceTargetsEditor rows={paces} errors={showErrors ? paceCheck.errors : {}} onChange={setPaces} />
      </div>

      <div className="flex flex-col-reverse gap-3 border-t border-line pt-4 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-xs text-ink-3" aria-live="polite">
          {dirty ? 'Modifications non enregistrées.' : 'Tout est à jour.'}
        </p>
        <button type="submit" className="btn btn-primary" disabled={!dirty || busy}>
          {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Save className="size-4" aria-hidden="true" />}
          Enregistrer le profil
        </button>
      </div>
    </form>
  )
}
