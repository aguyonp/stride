import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { invalidateDomain } from './queries'
import type { LLMAnalysis, LLMAnalyzeRequest, LLMConfigInput, NotificationInput, PlanGenerateRequest, PlannedSessionInput } from './types'

export const qk3 = {
  llm: ['llm'] as const,
  analyses: (kind: string, ref: string) => ['llm', 'analyses', kind, ref] as const,
  calendar: (from: string, to: string) => ['calendar', from, to] as const,
  efficiency: (weeks: number) => ['efficiency', weeks] as const,
  track: (id: number) => ['track', id] as const,
  notifications: ['notifications'] as const,
}

const PLAN_ROOTS = ['plan', 'week', 'calendar', 'activity', 'activities', 'day', 'report']

export function useLlmConfig() {
  return useQuery({ queryKey: qk3.llm, queryFn: api.llm, staleTime: 5 * 60_000 })
}

export function useLlmMutations() {
  const qc = useQueryClient()
  const set = () => void qc.invalidateQueries({ queryKey: qk3.llm })
  return {
    save: useMutation({ mutationFn: (b: LLMConfigInput) => api.saveLlm(b), onSuccess: (c) => qc.setQueryData(qk3.llm, c) }),
    remove: useMutation({ mutationFn: api.deleteLlm, onSuccess: set }),
    test: useMutation({ mutationFn: api.testLlm }),
  }
}

export function useAnalyses(kind: string, ref: string | null, enabled = true) {
  return useQuery({ queryKey: qk3.analyses(kind, ref ?? ''), queryFn: () => api.analyses(kind, ref as string), enabled: enabled && ref !== null })
}

export function useAnalyze() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (b: LLMAnalyzeRequest) => api.analyze(b),
    onSuccess: (a: LLMAnalysis) => {
      qc.setQueryData<LLMAnalysis[]>(qk3.analyses(a.kind, a.ref), (list) => [a, ...(list ?? []).filter((x) => x.id !== a.id)].slice(0, 20))
    },
  })
}

export function useCalendar(from: string, to: string) {
  return useQuery({ queryKey: qk3.calendar(from, to), queryFn: () => api.calendar(from, to), placeholderData: keepPreviousData })
}

export function usePlanV3() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, PLAN_ROOTS)
  return {
    generate: useMutation({ mutationFn: (b: PlanGenerateRequest) => api.planGenerate(b) }),
    importText: useMutation({ mutationFn: (text: string) => api.planImport(text) }),
    bulk: useMutation({ mutationFn: (s: PlannedSessionInput[]) => api.planBulk(s), onSuccess: done }),
    push: useMutation({ mutationFn: (id: number) => api.pushGarmin(id), onSettled: done }),
  }
}

export function useEfficiencyTrend(weeks = 12, enabled = true) {
  return useQuery({ queryKey: qk3.efficiency(weeks), queryFn: () => api.efficiencyTrend(weeks), enabled, staleTime: 5 * 60_000 })
}

export function useTrack(id: number | null, enabled = true) {
  return useQuery({ queryKey: qk3.track(id ?? 0), queryFn: () => api.track(id as number), enabled: enabled && id !== null, staleTime: Infinity })
}

export function useNotifications() {
  return useQuery({ queryKey: qk3.notifications, queryFn: api.notifications })
}

export function useNotificationMutations() {
  const qc = useQueryClient()
  const done = () => void qc.invalidateQueries({ queryKey: qk3.notifications })
  return {
    create: useMutation({ mutationFn: (b: NotificationInput) => api.createNotification(b), onSuccess: done }),
    update: useMutation({ mutationFn: ({ id, ...b }: Partial<NotificationInput> & { id: number }) => api.updateNotification(id, b), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: number) => api.deleteNotification(id), onSuccess: done }),
    test: useMutation({ mutationFn: (id: number) => api.testNotification(id), onSettled: done }),
  }
}

export function useImportFile() {
  const qc = useQueryClient()
  return useMutation({ mutationFn: (f: File) => api.importFile(f), onSuccess: () => void invalidateDomain(qc) })
}

export function useMergeShoe() {
  const qc = useQueryClient()
  return useMutation({ mutationFn: ({ id, intoId }: { id: number; intoId: number }) => api.mergeShoe(id, intoId), onSuccess: () => void invalidateDomain(qc, ['shoes', 'activity', 'report']) })
}
