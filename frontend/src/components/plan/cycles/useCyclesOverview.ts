import { useMemo } from 'react'
import { useCycles } from '../../../api/queries'
import { mergeWeeks, resolveCurrentPlan } from './cycleModel'

const EMPTY: never[] = []

/** Cycles query plus the plan, phases and merged week list the timeline renders. */
export function useCyclesOverview(selectedPlanId: number | null) {
  const query = useCycles(selectedPlanId)
  const overview = query.data
  const plans = overview?.plans ?? EMPTY
  const phases = overview?.phases ?? EMPTY
  const currentPlan = resolveCurrentPlan(plans, selectedPlanId, overview?.activePlan ?? null)
  const allWeeks = useMemo(() => mergeWeeks(phases, overview?.standaloneWeeks ?? EMPTY), [phases, overview?.standaloneWeeks])
  return { query, overview, plans, phases, currentPlan, allWeeks }
}
