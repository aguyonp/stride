import { CircleCheck, Eye, EyeOff, KeyRound, Loader2, Monitor, Moon, OctagonAlert, Plug, Save, Sun, Trash2, Unplug } from 'lucide-react'
import { useId, useMemo, useState, type ReactNode } from 'react'
import { ApiError, IS_MOCK } from '../../api/client'
import { useHealth, useSettings, useTestConnection, useUpdateSettings } from '../../api/queries'
import type { Profile, Settings } from '../../api/types'
import type { ThemePref } from '../../hooks/useTheme'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtPaceValue, parsePace } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import { SegmentedControl } from '../ui/SegmentedControl'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'

interface SettingsDialogProps {
  open: boolean
  onClose: () => void
  focusApiKey: boolean
  themePref: ThemePref
  onThemePref: (p: ThemePref) => void
}

export function SettingsDialog({ open, onClose, focusApiKey, themePref, onThemePref }: SettingsDialogProps) {
  const settings = useSettings()
  const health = useHealth()
  return (
    <Dialog
      open={open}
      onClose={onClose}
      variant="drawer"
      title="Paramètres"
      description="Connexion Garmin, profil de rééducation, apparence"
      initialFocus={focusApiKey ? '#garmin-api-key' : undefined}
    >
      <div className="space-y-6">
        {settings.isPending ? (
          <div className="space-y-3">
            <Skeleton className="h-24" />
            <Skeleton className="h-11" />
            <Skeleton className="h-64" />
          </div>
        ) : settings.isError ? (
          <p className="rounded-2xl border border-danger/30 bg-danger/10 p-3 text-sm text-danger">Impossible de charger les paramètres : {settings.error.message}</p>
        ) : (
          <>
            <ApiKeySection settings={settings.data} />
            <ProfileSection key={JSON.stringify(settings.data.profile) + settings.data.timezone} settings={settings.data} />
          </>
        )}
        <Section title="Apparence">
          <SegmentedControl<ThemePref>
            ariaLabel="Thème"
            value={themePref}
            onChange={onThemePref}
            options={[
              { value: 'system', label: 'Système', icon: <Monitor /> },
              { value: 'light', label: 'Clair', icon: <Sun /> },
              { value: 'dark', label: 'Sombre', icon: <Moon /> },
            ]}
          />
        </Section>
        <p className="text-center text-[11px] text-ink-3">
          Patella {health.data?.version ? `v${health.data.version}` : ''} · Données Garmin Connect{IS_MOCK ? ' · mode démo (fixtures)' : ''}
        </p>
      </div>
    </Dialog>
  )
}

function Section({ title, children, description }: { title: string; children: ReactNode; description?: ReactNode }) {
  return (
    <section>
      <h3 className="label-caps mb-1">{title}</h3>
      {description && <p className="mb-3 text-xs text-ink-3">{description}</p>}
      <div className={cn(!description && 'mt-2')}>{children}</div>
    </section>
  )
}

/* ------------------------------------------------------------------ */
/* Clé API                                                            */
/* ------------------------------------------------------------------ */

const SOURCE_LABEL: Record<Settings['apiKeySource'], string> = {
  ui: 'saisie dans l’app',
  env: 'fichier .env du serveur',
  none: 'aucune',
}

function ApiKeySection({ settings }: { settings: Settings }) {
  // La clé ne vit que dans cet état local, le temps de la saisie ; vidée après envoi.
  const [key, setKey] = useState('')
  const [show, setShow] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null)
  const update = useUpdateSettings()
  const test = useTestConnection()
  const { toast } = useToast()

  const save = () => {
    const value = key.trim()
    if (!value) return
    setResult(null)
    update.mutate(
      { apiKey: value },
      {
        onSuccess: () => {
          setKey('')
          setShow(false)
          update.reset()
          setResult({ ok: true, message: 'Clé enregistrée et validée par Garmin.' })
          toast({ title: 'Clé Garmin enregistrée', description: 'Rechargement des données…' })
        },
        onError: (e) => {
          update.reset()
          setResult({ ok: false, message: e instanceof ApiError ? e.message : 'Échec de l’enregistrement.' })
        },
      },
    )
  }

  const runTest = () => {
    setResult(null)
    test.mutate(undefined, {
      onSuccess: (r) => setResult({ ok: r.ok, message: r.ok ? `${r.message}${r.tokenExpiresAt ? ` · jeton valide jusqu’au ${fmtDateTime(r.tokenExpiresAt)}` : ''}` : r.message }),
      onError: (e) => setResult({ ok: false, message: e instanceof ApiError ? e.message : 'Test impossible.' }),
    })
  }

  const remove = () => {
    setResult(null)
    update.mutate(
      { apiKey: '' },
      {
        onSuccess: (s) => {
          setConfirmDelete(false)
          setResult({ ok: true, message: s.apiKeySource === 'env' ? 'Clé de l’app supprimée : retour à la clé du fichier .env.' : 'Clé supprimée.' })
        },
        onError: (e) => setResult({ ok: false, message: e instanceof ApiError ? e.message : 'Suppression impossible.' }),
      },
    )
  }

  const configured = settings.apiKeyConfigured
  return (
    <Section title="Connexion Garmin">
      <div className={cn('flex items-start gap-3 rounded-2xl border p-3', configured ? 'border-ok/25 bg-ok/8' : 'border-warn/30 bg-warn/10')}>
        <span className={cn('grid size-9 flex-none place-items-center rounded-xl', configured ? 'bg-ok/15 text-ok' : 'bg-warn/15 text-warn')}>
          {configured ? <Plug className="size-[1.125rem]" aria-hidden="true" /> : <Unplug className="size-[1.125rem]" aria-hidden="true" />}
        </span>
        <div className="min-w-0 text-sm">
          <p className="font-semibold text-ink">{configured ? 'Clé API configurée' : 'Aucune clé API'}</p>
          <p className="text-ink-2">
            Source : {SOURCE_LABEL[settings.apiKeySource]}
            {settings.apiKeyHint && (
              <>
                {' '}
                · <span className="num font-mono text-xs">{settings.apiKeyHint}</span>
              </>
            )}
          </p>
          <p className="text-xs text-ink-3">{settings.tokenExpiresAt ? `Jeton valide jusqu’au ${fmtDateTime(settings.tokenExpiresAt)}` : 'Aucun jeton actif'}</p>
        </div>
      </div>

      <form
        className="mt-3"
        autoComplete="off"
        onSubmit={(e) => {
          e.preventDefault()
          save()
        }}
      >
        <label htmlFor="garmin-api-key" className="mb-1.5 block text-sm font-medium text-ink">
          {configured ? 'Remplacer la clé API' : 'Clé API personnelle Garmin'}
        </label>
        <div className="relative">
          <KeyRound className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-3" aria-hidden="true" />
          <input
            id="garmin-api-key"
            name="garmin-personal-api-key"
            type={show ? 'text' : 'password'}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            data-1p-ignore
            data-lpignore="true"
            className="field num pr-11 pl-9 font-mono"
            placeholder="Colle ta clé ici"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            aria-describedby="api-key-help"
          />
          <button type="button" className="absolute top-1/2 right-1.5 grid size-8 -translate-y-1/2 place-items-center rounded-lg text-ink-3 hover:text-ink" onClick={() => setShow((v) => !v)} aria-label={show ? 'Masquer la clé' : 'Afficher la clé'}>
            {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
          </button>
        </div>
        <p id="api-key-help" className="mt-1.5 text-xs text-ink-3">
          Envoyée une seule fois au serveur Patella, qui la valide auprès de Garmin et la conserve côté serveur. Elle n’est jamais stockée dans ce navigateur.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <button type="submit" className="btn btn-primary" disabled={!key.trim() || update.isPending}>
            {update.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Save className="size-4" aria-hidden="true" />}
            Enregistrer
          </button>
          <button type="button" className="btn btn-ghost" onClick={runTest} disabled={test.isPending || !configured}>
            {test.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plug className="size-4" aria-hidden="true" />}
            Tester la connexion
          </button>
          {settings.apiKeySource === 'ui' &&
            (confirmDelete ? (
              <span className="flex items-center gap-2">
                <button type="button" className="btn btn-danger" onClick={remove} disabled={update.isPending}>
                  <Trash2 className="size-4" aria-hidden="true" /> Confirmer
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => setConfirmDelete(false)}>
                  Annuler
                </button>
              </span>
            ) : (
              <button type="button" className="btn btn-danger" onClick={() => setConfirmDelete(true)}>
                <Trash2 className="size-4" aria-hidden="true" /> Supprimer la clé UI
              </button>
            ))}
        </div>
      </form>
      <div aria-live="polite" className="min-h-0">
        {result && (
          <p className={cn('mt-3 flex items-start gap-2 rounded-xl border px-3 py-2 text-sm', result.ok ? 'border-ok/25 bg-ok/10 text-ink' : 'border-danger/30 bg-danger/10 text-ink')}>
            {result.ok ? <CircleCheck className="mt-0.5 size-4 flex-none text-ok" aria-hidden="true" /> : <OctagonAlert className="mt-0.5 size-4 flex-none text-danger" aria-hidden="true" />}
            {result.message}
          </p>
        )}
      </div>
    </Section>
  )
}

/* ------------------------------------------------------------------ */
/* Profil                                                             */
/* ------------------------------------------------------------------ */

type NumKey = 'cadenceTargetMin' | 'cadenceTargetMax' | 'gctBalanceAlertPct' | 'verticalOscillationAlertCm' | 'descentAlertM' | 'rhrAlertDeltaBpm'
type PaceKey = 'pace10kMinSecPerKm' | 'pace10kMaxSecPerKm' | 'paceMarathonMinSecPerKm' | 'paceMarathonMaxSecPerKm'

const NUM_FIELDS: { key: NumKey; label: string; unit: string; min: number; max: number; decimals: boolean }[] = [
  { key: 'cadenceTargetMin', label: 'Cadence cible min', unit: 'spm', min: 140, max: 220, decimals: false },
  { key: 'cadenceTargetMax', label: 'Cadence cible max', unit: 'spm', min: 140, max: 230, decimals: false },
  { key: 'gctBalanceAlertPct', label: 'Seuil écart G/D', unit: 'pt', min: 0.1, max: 10, decimals: true },
  { key: 'verticalOscillationAlertCm', label: 'Seuil oscillation', unit: 'cm', min: 3, max: 20, decimals: true },
  { key: 'descentAlertM', label: 'Seuil D−', unit: 'm', min: 1, max: 3000, decimals: false },
  { key: 'rhrAlertDeltaBpm', label: 'Seuil FC repos', unit: '+bpm', min: 1, max: 40, decimals: false },
]

function ProfileSection({ settings }: { settings: Settings }) {
  const p = settings.profile
  const [side, setSide] = useState<Profile['injuredSide']>(p.injuredSide)
  const [nums, setNums] = useState<Record<NumKey, string>>(() => {
    const o = {} as Record<NumKey, string>
    for (const f of NUM_FIELDS) o[f.key] = String(p[f.key]).replace('.', ',')
    return o
  })
  const [paces, setPaces] = useState<Record<PaceKey, string>>({
    pace10kMinSecPerKm: fmtPaceValue(p.pace10kMinSecPerKm),
    pace10kMaxSecPerKm: fmtPaceValue(p.pace10kMaxSecPerKm),
    paceMarathonMinSecPerKm: fmtPaceValue(p.paceMarathonMinSecPerKm),
    paceMarathonMaxSecPerKm: fmtPaceValue(p.paceMarathonMaxSecPerKm),
  })
  const [tz, setTz] = useState(settings.timezone)
  const update = useUpdateSettings()
  const { toast } = useToast()
  const uid = useId()

  const timezones = useMemo(() => {
    let list: string[] = []
    try {
      list = Intl.supportedValuesOf('timeZone')
    } catch {
      list = ['Europe/Paris', 'Europe/London', 'Europe/Brussels', 'Europe/Zurich', 'America/Montreal', 'UTC']
    }
    return list.includes(settings.timezone) ? list : [settings.timezone, ...list]
  }, [settings.timezone])

  const { profile, errors } = useMemo(() => {
    const errs: Partial<Record<NumKey | PaceKey, string>> = {}
    const out: Partial<Profile> = { injuredSide: side }
    for (const f of NUM_FIELDS) {
      const v = Number(nums[f.key].replace(',', '.').trim())
      if (!nums[f.key].trim() || !Number.isFinite(v) || v < f.min || v > f.max) errs[f.key] = `Entre ${f.min} et ${f.max}`
      else out[f.key] = f.decimals ? Math.round(v * 10) / 10 : Math.round(v)
    }
    if (out.cadenceTargetMin !== undefined && out.cadenceTargetMax !== undefined && out.cadenceTargetMin >= out.cadenceTargetMax) errs.cadenceTargetMax = 'Doit dépasser le min'
    for (const k of Object.keys(paces) as PaceKey[]) {
      const v = parsePace(paces[k])
      if (v === null) errs[k] = 'Format m:ss'
      else out[k] = v
    }
    if (out.pace10kMinSecPerKm && out.pace10kMaxSecPerKm && out.pace10kMinSecPerKm > out.pace10kMaxSecPerKm) errs.pace10kMaxSecPerKm = 'Plus lente que « de »'
    if (out.paceMarathonMinSecPerKm && out.paceMarathonMaxSecPerKm && out.paceMarathonMinSecPerKm > out.paceMarathonMaxSecPerKm) errs.paceMarathonMaxSecPerKm = 'Plus lente que « de »'
    return { profile: out, errors: errs }
  }, [side, nums, paces])

  const valid = Object.keys(errors).length === 0
  const dirty = tz !== settings.timezone || (Object.keys(profile) as (keyof Profile)[]).some((k) => profile[k] !== p[k])

  const save = () => {
    if (!valid) return
    update.mutate(
      { timezone: tz, profile },
      {
        onSuccess: () => toast({ title: 'Profil enregistré', description: 'Les analyses sont recalculées avec tes seuils.' }),
        onError: (e) => toast({ tone: 'error', title: 'Enregistrement impossible', description: e.message }),
      },
    )
  }

  const numField = (f: (typeof NUM_FIELDS)[number]) => {
    const id = `${uid}-${f.key}`
    return (
      <div key={f.key}>
        <label htmlFor={id} className="mb-1 block text-xs font-medium text-ink-2">
          {f.label}
        </label>
        <div className="relative">
          <input
            id={id}
            inputMode={f.decimals ? 'decimal' : 'numeric'}
            className="field num pr-12"
            value={nums[f.key]}
            onChange={(e) => setNums((s) => ({ ...s, [f.key]: e.target.value }))}
            aria-invalid={!!errors[f.key]}
            aria-describedby={errors[f.key] ? `${id}-err` : undefined}
          />
          <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-ink-3">{f.unit}</span>
        </div>
        {errors[f.key] && (
          <p id={`${id}-err`} className="mt-1 text-xs text-danger">
            {errors[f.key]}
          </p>
        )}
      </div>
    )
  }

  const paceField = (k: PaceKey, label: string) => {
    const id = `${uid}-${k}`
    return (
      <div>
        <label htmlFor={id} className="mb-1 block text-xs font-medium text-ink-2">
          {label}
        </label>
        <div className="relative">
          <input
            id={id}
            inputMode="numeric"
            placeholder="3:35"
            className="field num pr-12"
            value={paces[k]}
            onChange={(e) => setPaces((s) => ({ ...s, [k]: e.target.value }))}
            aria-invalid={!!errors[k]}
            aria-describedby={errors[k] ? `${id}-err` : undefined}
          />
          <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-ink-3">/km</span>
        </div>
        {errors[k] && (
          <p id={`${id}-err`} className="mt-1 text-xs text-danger">
            {errors[k]}
          </p>
        )}
      </div>
    )
  }

  return (
    <>
      <Section title="Profil de rééducation" description="Ces seuils pilotent les alertes et les rapports.">
        <div className="space-y-4">
          <div>
            <p className="mb-1.5 text-xs font-medium text-ink-2" id={`${uid}-side`}>
              Côté blessé
            </p>
            <SegmentedControl
              ariaLabel="Côté blessé"
              size="sm"
              value={side}
              onChange={setSide}
              options={[
                { value: 'left', label: 'Genou gauche' },
                { value: 'right', label: 'Genou droit' },
              ]}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">{NUM_FIELDS.map(numField)}</div>
          <div>
            <p className="mb-2 text-xs font-semibold text-ink">Allures cibles</p>
            <div className="grid grid-cols-2 gap-3">
              {paceField('pace10kMinSecPerKm', '10 km — de')}
              {paceField('pace10kMaxSecPerKm', '10 km — à')}
              {paceField('paceMarathonMinSecPerKm', 'Marathon — de')}
              {paceField('paceMarathonMaxSecPerKm', 'Marathon — à')}
            </div>
          </div>
          <div>
            <label htmlFor={`${uid}-tz`} className="mb-1 block text-xs font-medium text-ink-2">
              Fuseau horaire
            </label>
            <select id={`${uid}-tz`} className="field" value={tz} onChange={(e) => setTz(e.target.value)}>
              {timezones.map((z) => (
                <option key={z} value={z}>
                  {z.replace(/_/g, ' ')}
                </option>
              ))}
            </select>
          </div>
          <button type="button" className="btn btn-primary w-full" onClick={save} disabled={!dirty || !valid || update.isPending}>
            {update.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Save className="size-4" aria-hidden="true" />}
            {dirty ? 'Enregistrer le profil' : 'Profil à jour'}
          </button>
        </div>
      </Section>
    </>
  )
}
