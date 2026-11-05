import { ArrowLeft, Bot, ClipboardPaste, Copy, Loader2, Sparkles, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'wouter'
import { useLlmConfig, usePlanV3 } from '../../api/queriesV3'
import type { PlanDraftsResponse, PlannedSessionInput, PlanPromptResponse } from '../../api/types'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtNumber } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { Dialog } from '../ui/Dialog'
import { Checkbox, Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { useToast } from '../ui/Toast'
import { targetParts } from './planFormat'
import { stepsSummary } from './steps'

type Mode = 'llm' | 'prompt'
type Step = 'config' | 'paste' | 'preview'

function DraftList({ drafts, errors, selected, onToggle }: { drafts: PlannedSessionInput[]; errors: string[]; selected: Set<number>; onToggle: (i: number, on: boolean) => void }) {
  return (
    <div className="space-y-3">
      {errors.length > 0 && (
        <div role="alert" className="rounded-2xl border border-warn/30 bg-warn/[0.07] p-3">
          <p className="flex items-center gap-1.5 text-sm font-semibold text-warn">
            <TriangleAlert className="size-4" aria-hidden="true" /> {errors.length} point{errors.length > 1 ? 's' : ''} à vérifier
          </p>
          <ul className="mt-1 list-disc pl-5 text-sm text-ink-2">
            {errors.map((e) => (
              <li key={e}>{e}</li>
            ))}
          </ul>
        </div>
      )}
      {drafts.length === 0 ? (
        <p className="text-sm text-ink-3">Aucune séance exploitable.</p>
      ) : (
        <ul className="space-y-2" aria-label="Séances proposées">
          {drafts.map((d, i) => (
            <li key={i} className={cn('rounded-2xl border p-3 transition-opacity', selected.has(i) ? 'border-line' : 'border-line opacity-55')}>
              <Checkbox
                checked={selected.has(i)}
                onChange={(on) => onToggle(i, on)}
                label={
                  <span>
                    <span className="block text-xs text-ink-3">
                      {capitalize(fmtDateLong(d.date))} · {PLAN_SPORT_LABEL[d.sport] ?? d.sport}
                    </span>
                    <span className="block font-semibold text-ink">{d.title}</span>
                  </span>
                }
                description={
                  <span className="block space-y-0.5">
                    {targetParts(d).length > 0 && <span className="num block">{targetParts(d).join(' · ')}</span>}
                    {d.steps && d.steps.length > 0 && <span className="num block text-ink-2">{stepsSummary(d.steps)}</span>}
                    {d.instructions && <span className="block">{d.instructions}</span>}
                  </span>
                }
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** « Générer » : connected LLM → preview → confirm, or prompt → copy → paste answer → preview → confirm bulk create. */
export function GenerateDialog({ open, onClose, today }: { open: boolean; onClose: () => void; today: string }) {
  const llm = useLlmConfig()
  const plan = usePlanV3()
  const { toast } = useToast()
  const llmOn = llm.data?.configured === true
  const [mode, setMode] = useState<Mode | null>(null)
  const [step, setStep] = useState<Step>('config')
  const [horizon, setHorizon] = useState<'session' | 'week'>('week')
  const [startDate, setStartDate] = useState(addDays(today, 1))
  const [note, setNote] = useState('')
  const [prompt, setPrompt] = useState<PlanPromptResponse | null>(null)
  const [answer, setAnswer] = useState('')
  const [result, setResult] = useState<PlanDraftsResponse | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const effMode: Mode = llmOn ? (mode ?? 'llm') : 'prompt'

  const showDrafts = (r: PlanDraftsResponse) => {
    setResult(r)
    setSelected(new Set(r.drafts.map((_, i) => i)))
    setStep('preview')
  }
  const generate = () => {
    plan.generate.mutate(
      { mode: effMode, horizon, startDate, note: note.trim() || undefined },
      {
        onSuccess: (r) => {
          if ('prompt' in r) {
            setPrompt(r)
            setStep('paste')
          } else showDrafts(r)
        },
      },
    )
  }
  const parse = () => plan.importText.mutate(answer, { onSuccess: showDrafts })
  const confirm = () => {
    const chosen = (result?.drafts ?? []).filter((_, i) => selected.has(i)).map((d) => ({ ...d, source: d.source ?? (effMode === 'llm' ? 'llm' : 'import') }))
    plan.bulk.mutate(chosen, {
      onSuccess: (r) => {
        toast({ title: `${r.length} séance${r.length > 1 ? 's' : ''} ajoutée${r.length > 1 ? 's' : ''} au planning`, tone: 'success' })
        onClose()
      },
    })
  }
  const busy = plan.generate.isPending || plan.importText.isPending || plan.bulk.isPending
  const error = plan.generate.error ?? plan.importText.error ?? plan.bulk.error

  const footer =
    step === 'config' ? (
      <div className="flex justify-end gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          Annuler
        </button>
        <button type="button" className="btn btn-primary" onClick={generate} disabled={busy || !startDate}>
          {plan.generate.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : effMode === 'llm' ? <Sparkles className="size-4" aria-hidden="true" /> : <Copy className="size-4" aria-hidden="true" />}
          {effMode === 'llm' ? 'Générer' : 'Obtenir le prompt'}
        </button>
      </div>
    ) : step === 'paste' ? (
      <div className="flex justify-between gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setStep('config')}>
          <ArrowLeft className="size-4" aria-hidden="true" /> Retour
        </button>
        <button type="button" className="btn btn-primary" onClick={parse} disabled={busy || !answer.trim()}>
          {plan.importText.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <ClipboardPaste className="size-4" aria-hidden="true" />}
          Lire la réponse
        </button>
      </div>
    ) : (
      <div className="flex justify-between gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setStep(effMode === 'llm' ? 'config' : 'paste')}>
          <ArrowLeft className="size-4" aria-hidden="true" /> Retour
        </button>
        <button type="button" className="btn btn-primary" onClick={confirm} disabled={busy || selected.size === 0}>
          {plan.bulk.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          Ajouter {selected.size} séance{selected.size > 1 ? 's' : ''}
        </button>
      </div>
    )

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title="Générer un plan"
      description={step === 'config' ? 'Stride prépare le contexte (séances récentes, préparation, charge, objectifs) et impose le format de réponse.' : step === 'paste' ? 'Étape 2 : copie le prompt dans ton IA, puis colle sa réponse.' : 'Vérifie les séances avant de les ajouter.'}
      footer={footer}
    >
      <div className="space-y-4" aria-busy={busy}>
        {step === 'config' && (
          <>
            {llmOn && (
              <SegmentedControl<Mode>
                ariaLabel="Mode"
                value={effMode}
                onChange={setMode}
                options={[
                  { value: 'llm', label: 'Mon IA connectée', icon: <Bot /> },
                  { value: 'prompt', label: 'Copier-coller', icon: <ClipboardPaste /> },
                ]}
              />
            )}
            {!llmOn && llm.isSuccess && (
              <p className="text-xs text-ink-3">
                Aucune IA connectée :{' '}
                <Link href="/settings#ia" className="underline underline-offset-2" onClick={onClose}>
                  brancher mon IA
                </Link>{' '}
                ou utilise le copier-coller (ChatGPT, Claude, Gemini…).
              </p>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Horizon">
                {(p) => (
                  <select {...p} className="field" value={horizon} onChange={(e) => setHorizon(e.target.value as 'session' | 'week')}>
                    <option value="session">Prochaine séance</option>
                    <option value="week">Semaine (7 jours)</option>
                  </select>
                )}
              </Field>
              <Field label="À partir du">{(p) => <input {...p} type="date" className="field" value={startDate} min={today} onChange={(e) => setStartDate(e.target.value)} />}</Field>
              <Field label="Précision pour le coach (facultatif)" className="sm:col-span-2">
                {(p) => <textarea {...p} className="field min-h-20 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder="Ex. pas de séance le jeudi, sortie longue le dimanche" />}
              </Field>
            </div>
          </>
        )}

        {step === 'paste' && prompt && (
          <>
            <div>
              <div className="mb-1.5 flex items-center justify-between gap-2">
                <p className="text-sm font-medium text-ink-2">1. Prompt à copier</p>
                <span className="num text-xs text-ink-3">≈ {fmtNumber(prompt.tokensEstimate)} tokens</span>
              </div>
              <textarea readOnly className="field num min-h-36 resize-y font-mono text-xs" value={prompt.prompt} aria-label="Prompt de génération du plan" onFocus={(e) => e.currentTarget.select()} />
              <button type="button" className="btn btn-ghost mt-2" onClick={async () => toast((await copyText(prompt.prompt)) ? { title: 'Prompt copié', tone: 'success' } : { title: 'Copie impossible', tone: 'error' })}>
                <Copy className="size-4" aria-hidden="true" /> Copier le prompt
              </button>
            </div>
            <Field label="2. Réponse de l’IA" hint="Colle la réponse complète : Stride lit le premier bloc ```json (format stride-plan).">
              {(p) => <textarea {...p} className="field min-h-36 resize-y font-mono text-xs" value={answer} onChange={(e) => setAnswer(e.target.value)} placeholder={'```json\n{"format":"stride-plan","version":1,"sessions":[…]}\n```'} />}
            </Field>
          </>
        )}

        {step === 'preview' && result && (
          <DraftList
            drafts={result.drafts}
            errors={result.errors}
            selected={selected}
            onToggle={(i, on) => {
              const next = new Set(selected)
              if (on) next.add(i)
              else next.delete(i)
              setSelected(next)
            }}
          />
        )}

        {plan.generate.isPending && effMode === 'llm' && <p className="text-sm text-ink-3">Ton IA prépare le plan… (jusqu’à 2 minutes)</p>}
        {error && <InlineError>{error.message}</InlineError>}
      </div>
    </Dialog>
  )
}
