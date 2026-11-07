import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { upsertJobInStore } from '../state/events'
import { diffDays } from '../lib/format'
import { api } from './client'
import { invalidateDomain, useBodyMap } from './queries'
import type { HealthStatus, HealthStatusInput } from './types'

const qk41 = {
  health: ['health', 'status'] as const,
  conditions: ['health', 'conditions'] as const,
  search: (q: string) => ['search', q] as const,
}

export function useHealthStatus(enabled = true) {
  return useQuery({ queryKey: qk41.health, queryFn: api.healthStatus, enabled })
}

/** Pathology catalog per zone (static for a release: fetched once per session). */
export function useHealthConditions(enabled = true) {
  return useQuery({ queryKey: qk41.conditions, queryFn: api.healthConditions, enabled, staleTime: Infinity, gcTime: Infinity })
}

export function useSaveHealthStatus() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (b: HealthStatusInput) => api.saveHealthStatus(b),
    onSuccess: (s: HealthStatus) => {
      qc.setQueryData(qk41.health, s)
      void invalidateDomain(qc, ['profile', 'bodymap', 'report', 'day', 'activity'])
    },
  })
}

export function useSyncHistory() {
  return useMutation({ mutationFn: (from: string | null) => api.syncHistory({ from }), onSuccess: upsertJobInStore })
}

export function useActivitySearch(q: string) {
  const term = q.trim()
  return useQuery({
    queryKey: qk41.search(term),
    queryFn: () => api.search(term),
    enabled: term.length >= 2,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })
}

/** Body-map window wide enough to include an activity's date (90 days minimum, ten years maximum). */
export function symptomWindowDays(date: string | null, today: string): number {
  const age = date ? diffDays(date, today) + 1 : 0
  return Math.min(3650, Math.max(90, age))
}

/** Body entries linked to one activity: the single source of its declared symptoms. */
export function useActivitySymptoms(activityId: number, date: string, today: string) {
  const q = useBodyMap(symptomWindowDays(date, today), null)
  const entries = useMemo(() => (q.data?.entries ?? []).filter((e) => e.activityId === activityId), [q.data, activityId])
  return { entries, ready: q.isSuccess, isError: q.isError }
}
