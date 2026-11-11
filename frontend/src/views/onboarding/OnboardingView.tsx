import { ArrowRight, Check, CircleCheck, Info, Loader2, PartyPopper, Plug, UserRound, Watch, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useLocation } from 'wouter'
import { useIntegrations, useProfile, useUpdateMe } from '../../api/queries'
import { useHealthStatus, useSaveHealthStatus } from '../../api/queriesV41'
import type { HealthStatusInput, Me } from '../../api/types'
import { HealthStatusForm } from '../../components/health/HealthStatusForm'
import { draftFrom, healthSummary, toHealthInput, validateHealthDraft } from '../../components/health/healthStatus'
import { GarminGuide, GarminKeyForm } from '../../components/profile/GarminKeyForm'
import { clearStravaResult, readStravaResult, StravaConnectButton } from '../../components/profile/StravaConnect'
import { StrideMark } from '../../components/ui/icons'
import { InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { todayISO } from '../../lib/format'
import { t, tx } from '../../i18n'

type Step = 0 | 1 | 2 | 3

const STEP_DEFS: { key: 'health' | 'garmin' | 'strava' | 'done'; icon: LucideIcon }[] = [
  { key: 'health', icon: UserRound },
  { key: 'garmin', icon: Watch },
  { key: 'strava', icon: Plug },
  { key: 'done', icon: PartyPopper },
]
const steps = () => STEP_DEFS.map((s) => ({ ...s, title: t(`onboarding.view.steps.${s.key}.title`), short: t(`onboarding.view.steps.${s.key}.short`) }))

function Progress({ step }: { step: Step }) {
  return (
    <ol className="grid grid-cols-4 gap-1.5" aria-label={t('onboarding.view.progress')}>
      {steps().map((s, i) => {
        const done = i < step
        const current = i === step
        return (
          <li key={s.key} className="min-w-0" aria-current={current ? 'step' : undefined}>
            <div className={cn('h-1.5 rounded-full transition-colors', done || current ? 'bg-accent' : 'bg-track')} />
            <p className={cn('mt-1.5 flex items-center gap-1 truncate text-[11px] font-semibold', current ? 'text-ink' : done ? 'text-ink-2' : 'text-ink-3')}>
              {done && <Check className="size-3 flex-none text-accent" aria-hidden="true" />}
              <span className="truncate">{s.short}</span>
              <span className="sr-only">{done ? t('onboarding.view.stepDone') : current ? t('onboarding.view.stepCurrent') : ''}</span>
            </p>
          </li>
        )
      })}
    </ol>
  )
}

function Footer({ onSkip, onNext, nextLabel = t('onboarding.view.next'), busy, nextDisabled }: { onSkip?: () => void; onNext: () => void; nextLabel?: string; busy?: boolean; nextDisabled?: boolean }) {
  return (
    <div className="mt-6 flex flex-col-reverse gap-2 border-t border-line pt-4 sm:flex-row sm:items-center sm:justify-between">
      {onSkip ? (
        <button type="button" className="btn btn-ghost" onClick={onSkip} disabled={busy}>
          {t('onboarding.view.skip')}
        </button>
      ) : (
        <span />
      )}
      <button type="button" className="btn btn-primary min-h-11" onClick={onNext} disabled={busy || nextDisabled}>
        {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
        {nextLabel}
        {!busy && <ArrowRight className="size-4" aria-hidden="true" />}
      </button>
    </div>
  )
}

function Done({ children }: { children: ReactNode }) {
  return (
    <p role="status" className="flex items-start gap-2 rounded-xl border border-ok/30 bg-ok/10 px-3 py-2.5 text-sm text-ink">
      <CircleCheck className="mt-0.5 size-4 flex-none text-ok" aria-hidden="true" />
      <span className="min-w-0">{children}</span>
    </p>
  )
}

export default function OnboardingView({ me }: { me: Me }) {
  useDocumentTitle(t('onboarding.view.title'))
  const [, navigate] = useLocation()
  const [strava] = useState(readStravaResult)
  const [step, setStep] = useState<Step>(strava ? 2 : 0)
  const profile = useProfile()
  const health = useHealthStatus()
  const integ = useIntegrations()
  const saveHealth = useSaveHealthStatus()
  const updateMe = useUpdateMe()

  const [draft, setDraft] = useState<HealthStatusInput>(() => draftFrom(null))
  const [seeded, setSeeded] = useState(false)
  const [profileError, setProfileError] = useState<string | null>(null)
  const [finishError, setFinishError] = useState<string | null>(null)
  const firstRender = useRef(true)

  if (!seeded && health.data) {
    setSeeded(true)
    setDraft(draftFrom(health.data))
  }

  useEffect(() => clearStravaResult(), [])

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    document.getElementById('onboarding-step-title')?.focus()
  }, [step])

  const next = () => setStep((s) => Math.min(3, s + 1) as Step)

  const finish = () => {
    setFinishError(null)
    updateMe.mutate(
      { onboardingCompleted: true },
      {
        onSuccess: () => navigate('/', { replace: true }),
        onError: (e) => setFinishError(e.message),
      },
    )
  }

  const saveFocus = () => {
    setProfileError(null)
    const invalid = validateHealthDraft(draft)
    if (invalid) return setProfileError(invalid)
    saveHealth.mutate(toHealthInput(draft), { onSuccess: next, onError: (e) => setProfileError(e.message) })
  }

  const g = integ.data?.garmin
  const s = integ.data?.strava
  const meta = steps()[step]

  return (
    <div className="mx-auto max-w-2xl py-2 sm:py-6">
      <div className="mb-5 flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold tracking-tight text-ink">{t('onboarding.view.hello', { name: me.displayName })}</h1>
        </div>
        {step < 3 && (
          <button type="button" className="flex-none text-sm font-medium text-ink-3 underline-offset-4 hover:text-ink hover:underline" onClick={finish} disabled={updateMe.isPending}>
            {t('onboarding.view.skipAll')}
          </button>
        )}
      </div>

      <section className="glass animate-in p-4 sm:p-7" aria-labelledby="onboarding-step-title">
        <Progress step={step} />
        <div className="mt-6 flex items-start gap-3">
          <span className="grid size-10 flex-none place-items-center rounded-2xl bg-accent/12 text-accent" aria-hidden="true">
            <meta.icon className="size-5" />
          </span>
          <div className="min-w-0">
            <p className="text-xs font-medium text-ink-3">
              {t('onboarding.view.stepOf', { n: step + 1, total: STEP_DEFS.length })}
            </p>
            <h2 id="onboarding-step-title" tabIndex={-1} className="text-xl font-semibold tracking-tight outline-none sm:text-2xl">
              {meta.title}
            </h2>
          </div>
        </div>

        {step === 0 && (
          <div className="mt-5">
            <p className="mb-4 text-sm text-ink-2">{t('onboarding.view.healthIntro')}</p>
            {health.isPending && !health.isError ? (
              <div className="space-y-2">
                <Skeleton className="h-16" />
                <Skeleton className="h-16" />
                <Skeleton className="h-16" />
              </div>
            ) : (
              <HealthStatusForm value={draft} onChange={setDraft} today={todayISO(profile.data?.timezone)} />
            )}
            {profileError && <InlineError className="mt-3">{profileError}</InlineError>}
            <Footer onSkip={next} onNext={saveFocus} busy={saveHealth.isPending} nextDisabled={health.isPending} />
          </div>
        )}

        {step === 1 && (
          <div className="mt-5">
            <p className="mb-4 text-sm text-ink-2">{t('onboarding.view.garminIntro')}</p>
            {!integ.data ? (
              <Skeleton className="h-40" />
            ) : g?.connected ? (
              <div className="space-y-4">
                <Done>
                  {tx('onboarding.view.garminConnected', { key: <span className="font-mono">{g.keyHint ?? '••••'}</span> })}
                </Done>
                <details className="rounded-xl border border-line px-3 py-2">
                  <summary className="text-sm font-medium text-ink-2">{t('onboarding.view.replaceKey')}</summary>
                  <div className="mt-3">
                    <GarminKeyForm submitLabel={t('onboarding.view.replaceKey')} />
                  </div>
                </details>
              </div>
            ) : (
              <div className="space-y-5">
                <GarminGuide />
                <GarminKeyForm />
              </div>
            )}
            <Footer onSkip={g?.connected ? undefined : next} onNext={next} />
          </div>
        )}

        {step === 2 && (
          <div className="mt-5">
            <p className="mb-4 text-sm text-ink-2">{t('onboarding.view.stravaIntro')}</p>
            {strava?.status === 'connected' && <Done>{t('onboarding.view.stravaJustConnected')}</Done>}
            {strava?.status === 'error' && <InlineError>{strava.reason ? t('onboarding.view.stravaFailedReason', { reason: strava.reason }) : t('onboarding.view.stravaFailed')}</InlineError>}
            <div className="mt-3">
              {!integ.data ? (
                <Skeleton className="h-12 max-w-xs" />
              ) : s?.connected ? (
                strava?.status !== 'connected' && <Done>{t('onboarding.view.stravaConnected', { name: s.athleteName ? ` (${s.athleteName})` : '' })}</Done>
              ) : (
                <StravaConnectButton strava={s} returnPath="/onboarding" />
              )}
            </div>
            <Footer onSkip={s?.connected ? undefined : next} onNext={next} />
          </div>
        )}

        {step === 3 && (
          <div className="mt-5">
            <p className="mb-4 text-sm text-ink-2">{t('onboarding.view.readyIntro')}</p>
            <ul className="space-y-2">
              <SummaryRow ok label={t('onboarding.view.health')} value={health.data ? healthSummary(health.data) : t('onboarding.view.healthDefault')} />
              <SummaryRow ok={!!g?.connected} label="Garmin Connect" value={g?.connected ? t('onboarding.view.connectedKey', { key: g.keyHint ?? '' }) : t('onboarding.view.notConnected')} />
              <SummaryRow
                ok={!!s?.connected}
                label="Strava"
                value={s?.connected ? t('onboarding.view.connected', { name: s.athleteName ? ` · ${s.athleteName}` : '' }) : s?.available === false ? t('onboarding.view.unavailable') : t('onboarding.view.notConnected')}
              />
            </ul>
            {!g?.connected && !s?.connected && (
              <p className="mt-4 flex items-start gap-2 rounded-xl border border-info/30 bg-info/10 px-3 py-2.5 text-sm text-ink-2">
                <Info className="mt-0.5 size-4 flex-none text-info" aria-hidden="true" />
                {t('onboarding.view.noSource')}
              </p>
            )}
            {finishError && <InlineError className="mt-3">{finishError}</InlineError>}
            <div className="mt-6 flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:items-center sm:justify-between">
              <button type="button" className="btn btn-ghost" onClick={() => setStep(0)}>
                {t('onboarding.view.review')}
              </button>
              <button type="button" className="btn btn-primary min-h-12 text-[0.95rem]" onClick={finish} disabled={updateMe.isPending}>
                {updateMe.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <StrideMark className="w-5" />}
                {t('onboarding.view.open')}
              </button>
            </div>
          </div>
        )}
      </section>
      {step > 0 && step < 3 && (
        <button type="button" className="mt-3 text-sm font-medium text-ink-3 hover:text-ink" onClick={() => setStep((s) => Math.max(0, s - 1) as Step)}>
          {t('onboarding.view.previous')}
        </button>
      )}
    </div>
  )
}

function SummaryRow({ ok, label, value }: { ok: boolean; label: string; value: string }) {
  return (
    <li className="glass-inset flex items-center gap-3 px-3 py-2.5">
      <span className={cn('grid size-7 flex-none place-items-center rounded-full', ok ? 'bg-ok/15 text-ok' : 'bg-track text-ink-3')} aria-hidden="true">
        {ok ? <Check className="size-4" /> : <span className="size-1.5 rounded-full bg-current" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-ink">{label}</span>
        <span className="block truncate text-xs text-ink-3">{value}</span>
      </span>
    </li>
  )
}
