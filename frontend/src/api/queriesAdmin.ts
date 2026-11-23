// 4.1 admin control center (§14.6): overview, cross-user jobs and share links, maintenance actions.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import { ADMIN_METER_POLL_MS } from './polling'
import type { AdminJobFilter, FeedbackCategory, FeedbackInput, FeedbackStatus, McpQuota, McpUsage } from './types'

/** Active jobs move fast: poll them more often than the overview. */
export const ADMIN_JOBS_POLL_MS = 5_000

export const qkAdmin = {
  all: ['admin'] as const,
  overview: ['admin', 'overview'] as const,
  jobs: (state: AdminJobFilter) => ['admin', 'jobs', state] as const,
  shares: (state: 'active' | 'all') => ['admin', 'shares', state] as const,
  feedback: (status: FeedbackStatus | 'all', category: FeedbackCategory | 'all') => ['admin', 'feedback', status, category] as const,
  mcpUsage: ['admin', 'mcp', 'usage'] as const,
  mcpJournal: ['admin', 'mcp', 'journal'] as const,
}

/** Instance health at a glance, refreshed every 15 s while the page is visible. */
export function useAdminOverview() {
  return useQuery({ queryKey: qkAdmin.overview, queryFn: api.adminOverview, refetchInterval: ADMIN_METER_POLL_MS })
}

export function useAdminJobs(state: AdminJobFilter) {
  return useQuery({
    queryKey: qkAdmin.jobs(state),
    queryFn: () => api.adminJobs(state),
    refetchInterval: (q) => (state === 'active' || q.state.data?.items.some((j) => j.cancellable) ? ADMIN_JOBS_POLL_MS : ADMIN_METER_POLL_MS),
  })
}

export function useAdminShares(state: 'active' | 'all') {
  return useQuery({ queryKey: qkAdmin.shares(state), queryFn: () => api.adminShares(state) })
}

export function useAdminFeedback(status: FeedbackStatus | 'all', category: FeedbackCategory | 'all') {
  return useQuery({ queryKey: qkAdmin.feedback(status, category), queryFn: () => api.adminFeedback(status, category) })
}

/** Any signed-in user: send an in-app feedback. */
export function useSendFeedback() {
  return useMutation({ mutationFn: (body: FeedbackInput) => api.sendFeedback(body) })
}

/** MCP usage on the public route: the counters move fast, poll them like the meter. */
export function useAdminMcpUsage() {
  return useQuery({ queryKey: qkAdmin.mcpUsage, queryFn: api.adminMcpUsage, refetchInterval: ADMIN_METER_POLL_MS })
}

/** MCP call journal (4.5.0, issue #135): the in-memory ring of the last public-route calls. */
export function useAdminMcpJournal() {
  return useQuery({ queryKey: qkAdmin.mcpJournal, queryFn: api.adminMcpJournal, refetchInterval: ADMIN_METER_POLL_MS })
}

/** Control center actions; each one refreshes every admin view (counts live in the overview). */
export function useAdminControl() {
  const qc = useQueryClient()
  const refresh = () => void qc.invalidateQueries({ queryKey: qkAdmin.all })
  return {
    revokeSessions: useMutation({ mutationFn: (id: number) => api.adminRevokeSessions(id), onSuccess: refresh }),
    syncUser: useMutation({ mutationFn: (id: number) => api.adminSyncUser(id), onSuccess: refresh }),
    retryJob: useMutation({ mutationFn: (id: string) => api.adminRetryJob(id), onSuccess: refresh }),
    cancelJob: useMutation({ mutationFn: (id: string) => api.adminCancelJob(id), onSuccess: refresh }),
    revokeShare: useMutation({ mutationFn: (id: number) => api.adminRevokeShare(id), onSuccess: refresh }),
    purge: useMutation({ mutationFn: api.adminPurge, onSuccess: refresh }),
    optimize: useMutation({ mutationFn: (vacuum: boolean) => api.adminOptimize(vacuum), onSuccess: refresh }),
    check: useMutation({ mutationFn: api.adminCheck }),
    updateFeedback: useMutation({
      mutationFn: ({ id, ...b }: { id: number; status?: FeedbackStatus; adminNote?: string }) => api.adminUpdateFeedback(id, b),
      onSuccess: refresh,
    }),
    updateMcpQuota: useMutation({
      mutationFn: (q: McpQuota) => api.adminMcpQuota(q),
      onSuccess: (q) => {
        qc.setQueryData(qkAdmin.mcpUsage, (old: McpUsage | undefined) => (old ? { ...old, quota: q } : old))
      },
    }),
  }
}
