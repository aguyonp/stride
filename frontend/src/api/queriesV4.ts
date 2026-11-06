import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { invalidateDomain } from './queries'
import { seedJobsInStore } from '../state/events'
import type {
  BodyStressInput,
  BodyZone,
  ShareLinkInput,
  TrainingPhaseInput,
  TrainingPlanInput,
  TrainingWeekInput,
} from './types'

export const qk4 = {
  publicVersion: ['version', 'public'] as const,
  jobs: ['jobs'] as const,
  job: (id: string) => ['jobs', id] as const,
  cycles: (planId?: number | null) => ['cycles', planId ?? 'active'] as const,
  bodymap: (days: number, zone?: BodyZone | null) => ['bodymap', days, zone ?? 'all'] as const,
  share: ['share'] as const,
  publicShare: (token: string) => ['publicShare', token] as const,
}

export function usePublicVersion(enabled = true) {
  return useQuery({ queryKey: qk4.publicVersion, queryFn: api.publicVersion, staleTime: Infinity, enabled })
}

export function useJobs(limit = 20, enabled = true) {
  return useQuery({
    queryKey: qk4.jobs,
    queryFn: async () => {
      const res = await api.jobs(limit)
      seedJobsInStore(res.items)
      return res
    },
    enabled,
    staleTime: 60_000,
  })
}

export function useCycles(planId?: number | null, enabled = true) {
  return useQuery({
    queryKey: qk4.cycles(planId),
    queryFn: () => api.cycles(planId),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useCycleMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['cycles', 'plan', 'week', 'calendar', 'phases', 'goals'])
  return {
    createPlan: useMutation({ mutationFn: (b: TrainingPlanInput) => api.createCyclePlan(b), onSuccess: done }),
    updatePlan: useMutation({
      mutationFn: ({ id, ...b }: Partial<TrainingPlanInput> & { id: number }) => api.updateCyclePlan(id, b),
      onSuccess: done,
    }),
    removePlan: useMutation({ mutationFn: (id: number) => api.deleteCyclePlan(id), onSuccess: done }),
    createPhase: useMutation({ mutationFn: (b: TrainingPhaseInput) => api.createCyclePhase(b), onSuccess: done }),
    updatePhase: useMutation({
      mutationFn: ({ id, ...b }: Partial<TrainingPhaseInput> & { id: number }) => api.updateCyclePhase(id, b),
      onSuccess: done,
    }),
    removePhase: useMutation({ mutationFn: (id: number) => api.deleteCyclePhase(id), onSuccess: done }),
    upsertWeek: useMutation({ mutationFn: (b: TrainingWeekInput) => api.upsertCycleWeek(b), onSuccess: done }),
  }
}

export function useBodyMap(days = 90, zone?: BodyZone | null, enabled = true) {
  return useQuery({
    queryKey: qk4.bodymap(days, zone),
    queryFn: () => api.bodyMap(days, zone),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useBodyMapMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['bodymap', 'activity', 'day', 'report'])
  return {
    createEntry: useMutation({ mutationFn: (b: BodyStressInput) => api.createBodyEntry(b), onSuccess: done }),
    updateEntry: useMutation({
      mutationFn: ({ id, ...b }: Partial<BodyStressInput> & { id: number }) => api.updateBodyEntry(id, b),
      onSuccess: done,
    }),
    removeEntry: useMutation({ mutationFn: (id: number) => api.deleteBodyEntry(id), onSuccess: done }),
  }
}

export function useShareLinks(enabled = true) {
  return useQuery({ queryKey: qk4.share, queryFn: api.shareLinks, enabled })
}

export function useShareMutations() {
  const qc = useQueryClient()
  const done = () => void qc.invalidateQueries({ queryKey: qk4.share })
  return {
    create: useMutation({ mutationFn: (b: ShareLinkInput) => api.createShareLink(b), onSuccess: done }),
    revoke: useMutation({ mutationFn: (id: number) => api.revokeShareLink(id), onSuccess: done }),
  }
}

export function usePublicShareView(token: string | null) {
  return useQuery({
    queryKey: qk4.publicShare(token ?? ''),
    queryFn: () => api.publicShare(token as string),
    enabled: Boolean(token),
    retry: false,
    staleTime: 30_000,
  })
}
