import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { invalidateDomain, useIntegrations } from './queries'
import { PLAN_ROOTS } from './queriesV3'
import type { CalendarEventInput, ManualActivityInput, PlanAdaptRequest, PlannedSessionInput, RecordInput, RecordKey } from './types'

const qk31 = {
  records: ['records'] as const,
  events: (from: string, to: string) => ['events', from, to] as const,
  phases: ['phases'] as const,
}

/** No provider connected: Stride runs on manual activities only. */
export function useStandalone(): boolean {
  const integ = useIntegrations()
  const d = integ.data
  return !!d && !d.garmin.connected && !d.strava.connected
}

export function useRecords(enabled = true) {
  return useQuery({ queryKey: qk31.records, queryFn: api.records, enabled, staleTime: 5 * 60_000, select: (r) => r.records })
}

export function useRecordMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['records', 'report'])
  return {
    save: useMutation({ mutationFn: ({ key, ...b }: RecordInput & { key: RecordKey }) => api.saveRecord(key, b), onSuccess: done }),
    reset: useMutation({ mutationFn: (key: RecordKey) => api.deleteRecord(key), onSuccess: done }),
  }
}

export function useManualActivityMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['activities', 'activity', 'day', 'week', 'calendar', 'report', 'records', 'efficiency'])
  return {
    create: useMutation({ mutationFn: (b: ManualActivityInput) => api.createActivity(b), onSuccess: done }),
    update: useMutation({ mutationFn: ({ id, ...b }: Partial<ManualActivityInput> & { id: number }) => api.updateActivity(id, b), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: number) => api.deleteActivity(id), onSuccess: done }),
  }
}

export function useCalendarEvents(from: string, to: string, enabled = true) {
  return useQuery({ queryKey: qk31.events(from, to), queryFn: () => api.events(from, to), enabled, placeholderData: keepPreviousData })
}

export function useEventMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['events', 'calendar', 'report'])
  return {
    create: useMutation({ mutationFn: (b: CalendarEventInput) => api.createEvent(b), onSuccess: done }),
    update: useMutation({ mutationFn: ({ id, ...b }: Partial<CalendarEventInput> & { id: number }) => api.updateEvent(id, b), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: number) => api.deleteEvent(id), onSuccess: done }),
  }
}

export function usePlanPhases(enabled = true) {
  return useQuery({ queryKey: qk31.phases, queryFn: api.planPhases, enabled, staleTime: 5 * 60_000 })
}

export function usePlanV31() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, PLAN_ROOTS)
  return {
    shift: useMutation({ mutationFn: ({ id, ...b }: { id: number; days?: number; toDate?: string }) => api.shiftPlanned(id, b), onSuccess: done }),
    adapt: useMutation({ mutationFn: (b: PlanAdaptRequest) => api.planAdapt(b) }),
    replace: useMutation({ mutationFn: (b: { removeIds: number[]; sessions: PlannedSessionInput[] }) => api.planReplace(b), onSuccess: done }),
  }
}

export function usePlanRange(from: string, to: string, enabled = true) {
  return useQuery({ queryKey: ['plan', from, to] as const, queryFn: () => api.plan(from, to), enabled, placeholderData: keepPreviousData })
}
