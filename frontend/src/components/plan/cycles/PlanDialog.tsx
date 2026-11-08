import { Loader2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useCycleMutations } from '../../../api/queries'
import type { CyclePlanStatus, Goal, TrainingPlan, TrainingPlanInput } from '../../../api/types'
import { fmtDateShort } from '../../../lib/format'
import { Dialog } from '../../ui/Dialog'
import { Checkbox, Field } from '../../ui/Field'
import { InlineError } from '../../ui/Misc'
import { useToast } from '../../ui/Toast'
import { parseOptionalNumber, planFormDefaults, validatePlanForm } from './cycleModel'
import { t } from '../../../i18n'

export function PlanDialog({
  open,
  plan,
  goals,
  today,
  onClose,
}: {
  open: boolean
  plan: TrainingPlan | null
  goals: Goal[]
  today: string
  onClose: () => void
}) {
  const ids = useId()
  const mut = useCycleMutations()
  const { toast } = useToast()
  const [init] = useState(() => planFormDefaults(plan, goals, today))

  const [name, setName] = useState(init.name)
  const [sport, setSport] = useState<'run' | 'bike' | 'other'>(init.sport)
  const [goalId, setGoalId] = useState<number | null>(init.goalId)
  const [startDate, setStartDate] = useState(init.startDate)
  const [endDate, setEndDate] = useState(init.endDate)
  const [targetEventLabel, setTargetEventLabel] = useState(init.targetEventLabel)
  const [status, setStatus] = useState<CyclePlanStatus>(init.status)
  const [autoScaffold, setAutoScaffold] = useState(true)
  const [peakVolume, setPeakVolume] = useState('52')
  const [peakLoad, setPeakLoad] = useState('420')
  const [note, setNote] = useState(init.note)
  const [errMsg, setErrMsg] = useState<string | null>(null)

  const busy = mut.createPlan.isPending || mut.updatePlan.isPending

  const onGoalPick = (nextId: number | null) => {
    setGoalId(nextId)
    const g = goals.find((x) => x.id === nextId)
    if (g && !plan) {
      if (g.targetDate) setEndDate(g.targetDate)
      setTargetEventLabel(g.title)
      if (!name.trim() || name.startsWith('Préparation') || name.endsWith('build-up')) setName(t('plan.cycles.prep', { goal: g.title }))
    }
  }

  const submit = () => {
    const pv = parseOptionalNumber(peakVolume)
    const pl = parseOptionalNumber(peakLoad)
    const invalid = validatePlanForm({ name, startDate, endDate, peakVolume: pv, peakLoad: pl })
    setErrMsg(invalid)
    if (invalid) return

    const payload: TrainingPlanInput = {
      name: name.trim(),
      goalId,
      sport,
      startDate,
      endDate,
      targetEventLabel: targetEventLabel.trim(),
      status,
      note: note.trim(),
      ...(plan ? {} : { autoScaffold, peakWeeklyVolumeKm: pv, peakWeeklyLoad: pl }),
    }

    if (plan) {
      mut.updatePlan.mutate(
        { id: plan.id, ...payload },
        {
          onSuccess: () => {
            toast({ title: t('plan.cycleDialogs.planUpdated'), tone: 'success' })
            onClose()
          },
          onError: (e) => setErrMsg(e.message),
        },
      )
    } else {
      mut.createPlan.mutate(payload, {
        onSuccess: () => {
          toast({ title: t('plan.cycleDialogs.planCreated'), description: autoScaffold ? t('plan.cycleDialogs.scaffolded') : undefined, tone: 'success' })
          onClose()
        },
        onError: (e) => setErrMsg(e.message),
      })
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title={plan ? t('plan.cycleDialogs.editPlan') : t('plan.cycleDialogs.newPlan')}
      description={t('plan.cycleDialogs.planHint')}
      initialFocus={`#${CSS.escape(`${ids}-name`)}`}
      footer={
        <div className="flex justify-end gap-2 pb-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {plan ? t('common.actions.save') : t('plan.cycleDialogs.createPlan')}
          </button>
        </div>
      }
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label htmlFor={`${ids}-name`} className="mb-1.5 block text-sm font-medium text-ink-2">
              {t('plan.cycleDialogs.planName')}
            </label>
            <input
              id={`${ids}-name`}
              className="field"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              placeholder={t('plan.cycleDialogs.planNamePlaceholder')}
            />
          </div>
          <Field label={t('plan.cycleDialogs.discipline')}>
            {(p) => (
              <select {...p} className="field" value={sport} onChange={(e) => setSport(e.target.value as 'run' | 'bike' | 'other')}>
                <option value="run">{t('plan.cycleDialogs.run')}</option>
                <option value="bike">{t('plan.cycleDialogs.bike')}</option>
                <option value="other">{t('plan.cycleDialogs.multi')}</option>
              </select>
            )}
          </Field>
          <Field label={t('plan.cycleDialogs.linkedGoal')}>
            {(p) => (
              <select {...p} className="field" value={goalId ?? ''} onChange={(e) => onGoalPick(e.target.value ? Number(e.target.value) : null)}>
                <option value="">{t('plan.cycleDialogs.noGoal')}</option>
                {goals.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title}
                    {g.targetDate ? ` (${fmtDateShort(g.targetDate)})` : ''}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label={t('plan.cycleDialogs.start')}>{(p) => <input {...p} type="date" className="field num" value={startDate} onChange={(e) => setStartDate(e.target.value)} />}</Field>
          <Field label={t('plan.cycleDialogs.end')}>{(p) => <input {...p} type="date" className="field num" value={endDate} onChange={(e) => setEndDate(e.target.value)} />}</Field>
          <Field label={t('plan.cycleDialogs.eventLabel')} className={plan ? undefined : 'sm:col-span-2'}>
            {(p) => (
              <input
                {...p}
                className="field"
                value={targetEventLabel}
                onChange={(e) => setTargetEventLabel(e.target.value)}
                maxLength={100}
                placeholder={t('plan.cycleDialogs.eventPlaceholder')}
              />
            )}
          </Field>
          {plan && (
            <Field label={t('plan.cycleDialogs.status')}>
              {(p) => (
                <select {...p} className="field" value={status} onChange={(e) => setStatus(e.target.value as CyclePlanStatus)}>
                  <option value="active">{t('plan.cycles.planStatus.active')}</option>
                  <option value="completed">{t('plan.cycles.planStatus.completed')}</option>
                  <option value="archived">{t('plan.cycles.planStatus.archived')}</option>
                </select>
              )}
            </Field>
          )}
        </div>

        {!plan && (
          <div className="space-y-3 rounded-2xl border border-line bg-track/35 p-3.5">
            <Checkbox
              checked={autoScaffold}
              onChange={setAutoScaffold}
              label={t('plan.cycleDialogs.autoSplit')}
              description={t('plan.cycleDialogs.autoSplitHint')}
            />
            {autoScaffold && (
              <div className="grid gap-3 pt-1 sm:grid-cols-2">
                <Field label={t('plan.cycleDialogs.peakVolume')}>
                  {(p) => <input {...p} type="number" min={5} max={300} step={1} className="field num" value={peakVolume} onChange={(e) => setPeakVolume(e.target.value)} />}
                </Field>
                <Field label={t('plan.cycleDialogs.peakLoad')}>
                  {(p) => <input {...p} type="number" min={50} max={3000} step={10} className="field num" value={peakLoad} onChange={(e) => setPeakLoad(e.target.value)} />}
                </Field>
              </div>
            )}
          </div>
        )}

        <Field label={t('plan.cycleDialogs.notes')}>
          {(p) => (
            <textarea
              {...p}
              className="field min-h-20 resize-y"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={600}
              placeholder={t('plan.cycleDialogs.notesPlaceholder')}
            />
          )}
        </Field>
        {errMsg && <InlineError>{errMsg}</InlineError>}
      </form>
    </Dialog>
  )
}
