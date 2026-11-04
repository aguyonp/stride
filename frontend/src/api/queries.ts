import {
  keepPreviousData,
  QueryClient,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api, ApiError } from './client'
import type { Mode, SettingsUpdate } from './types'

export const PAGE_SIZE = 30

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2 * 60_000,
      refetchOnWindowFocus: false,
      // Pas de retry sur les erreurs « métier » (clé absente, 404…) : seulement réseau / 5xx.
      retry: (count, err) => {
        if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false
        return count < 2
      },
    },
  },
})

export const qk = {
  settings: ['settings'] as const,
  health: ['health'] as const,
  activities: ['activities'] as const,
  month: (from: string, to: string) => ['activities', 'range', from, to] as const,
  analysis: (id: number, mode: Mode, history: number) => ['analysis', id, mode, history] as const,
  day: (date: string, history: number) => ['day', date, history] as const,
}

export function useSettings() {
  return useQuery({ queryKey: qk.settings, queryFn: api.getSettings, staleTime: 5 * 60_000 })
}

export function useHealth() {
  return useQuery({ queryKey: qk.health, queryFn: api.health, staleTime: 10 * 60_000 })
}

/** Profil / fuseau : invalide les analyses (les seuils d'alerte changent). */
export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SettingsUpdate) => api.updateSettings(body),
    // gcTime 0 : les variables (qui peuvent contenir la clé API) ne restent pas en cache mémoire.
    gcTime: 0,
    onSuccess: (settings, vars) => {
      qc.setQueryData(qk.settings, settings)
      if (vars.apiKey !== undefined) {
        // Nouvelle clé (ou suppression) : tout recharger.
        void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== 'settings' })
      } else {
        void qc.invalidateQueries({ queryKey: ['analysis'] })
        void qc.invalidateQueries({ queryKey: ['day'] })
      }
    },
  })
}

export function useTestConnection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.testConnection,
    gcTime: 0,
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.settings }),
  })
}

export function useActivities() {
  return useInfiniteQuery({
    queryKey: qk.activities,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api.listActivities({ limit: PAGE_SIZE, start: pageParam }),
    getNextPageParam: (last, pages) => (last.hasMore ? pages.reduce((n, p) => n + p.items.length, 0) : undefined),
  })
}

export function useActivitiesInRange(from: string, to: string, enabled = true) {
  return useQuery({
    queryKey: qk.month(from, to),
    queryFn: () => api.listActivities({ from, to, limit: 100, start: 0 }),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useActivityAnalysis(id: number | null, mode: Mode, history: number) {
  return useQuery({
    queryKey: qk.analysis(id ?? 0, mode, history),
    queryFn: () => api.activityAnalysis(id as number, mode, history),
    enabled: id !== null,
    staleTime: 5 * 60_000,
    placeholderData: keepPreviousData,
  })
}

export function useDayAnalysis(date: string | null, history: number) {
  return useQuery({
    queryKey: qk.day(date ?? '', history),
    queryFn: () => api.dayAnalysis(date as string, history),
    enabled: date !== null,
    staleTime: 5 * 60_000,
    placeholderData: keepPreviousData,
  })
}
