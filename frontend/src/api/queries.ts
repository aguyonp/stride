import {
  keepPreviousData,
  MutationCache,
  QueryCache,
  QueryClient,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryKey,
} from '@tanstack/react-query'
import { api, ApiError, type ListActivitiesParams } from './client'
import { ADMIN_METER_POLL_MS, fallbackPollInterval } from './polling'
import { resetEventStore, useHasActiveJob, useStreamConnected } from '../state/events'
import type {
  InboxPage,
  LogFilter,
  ActivityListResponse,
  ActivityNotesInput,
  ActivityView,
  AdminSettings,
  AdminUser,
  AthleteProfile,
  AuthState,
  DailyCheckinInput,
  GoalInput,
  IncludeKey,
  Integrations,
  Me,
  MultiExportRequest,
  PlannedSessionInput,
  ShoeInput,
  SyncStatus,
} from './types'

const PAGE_SIZE = 30

export const qk = {
  auth: ['auth'] as const,
  passkeys: ['passkeys'] as const,
  recovery: ['recovery'] as const,
  mcp: ['mcp'] as const,
  integrations: ['integrations'] as const,
  sync: ['sync', 'status'] as const,
  activities: (p: ListActivitiesParams = {}) => ['activities', 'list', p] as const,
  activitiesRange: (from: string, to: string) => ['activities', 'range', from, to] as const,
  activityOlder: (date: string, before: string) => ['activities', 'older', date, before] as const,
  inbox: ['inbox'] as const,
  updates: ['updates'] as const,
  garminBridge: ['garmin', 'bridge'] as const,
  activity: (id: number) => ['activity', id] as const,
  report: (kind: 'activity' | 'day', ref: number | string, include: string) => ['report', kind, ref, include] as const,
  day: (date: string) => ['day', date] as const,
  profile: ['profile'] as const,
  presets: ['presets'] as const,
  goals: ['goals'] as const,
  plan: (from: string, to: string) => ['plan', from, to] as const,
  week: (date: string) => ['week', date] as const,
  shoes: ['shoes'] as const,
  admin: {
    users: ['admin', 'users'] as const,
    invitations: ['admin', 'invitations'] as const,
    settings: ['admin', 'settings'] as const,
    meter: ['admin', 'meter'] as const,
    connections: ['admin', 'connections'] as const,
  },
}

const DOMAIN_ROOTS = new Set([
  'activities',
  'activity',
  'report',
  'day',
  'plan',
  'week',
  'calendar',
  'events',
  'phases',
  'records',
  'efficiency',
  'goals',
  'shoes',
  'integrations',
  'profile',
  'sync',
  'cycles',
  'bodymap',
  'share',
  'jobs',
  'load',
  'readiness',
])

export function invalidateDomain(qc: QueryClient, roots?: string[]) {
  const set = roots ? new Set(roots) : DOMAIN_ROOTS
  return qc.invalidateQueries({ predicate: (q) => set.has(String(q.queryKey[0])) })
}

function onUnauthorized(err: unknown) {
  if (err instanceof ApiError && err.status === 401) {
    queryClient.setQueryData<AuthState>(qk.auth, (s) => (s ? { ...s, authenticated: false, user: null } : s))
  }
}

export const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError: onUnauthorized }),
  mutationCache: new MutationCache({ onError: onUnauthorized }),
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      refetchOnWindowFocus: false,
      retry: (count, err) => {
        if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false
        return count < 2
      },
    },
  },
})

/* ---------------------------------- Auth & account ---------------------------------- */

export function useAuthState(enabled = true) {
  return useQuery({ queryKey: qk.auth, queryFn: api.authState, staleTime: 5 * 60_000, retry: 1, enabled })
}

export function setMe(qc: QueryClient, me: Me | null) {
  qc.setQueryData<AuthState>(qk.auth, (s) => (s ? { ...s, authenticated: me !== null, user: me, firstUser: me ? false : s.firstUser } : s))
}

export function useUpdateMe() {
  const qc = useQueryClient()
  return useMutation({ mutationFn: api.updateMe, onSuccess: (me) => setMe(qc, me) })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.logout,
    onSettled: () => {
      resetEventStore()
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'auth' })
      setMe(qc, null)
    },
  })
}

export function useDeleteAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.deleteMe,
    onSuccess: () => {
      qc.clear()
      void qc.invalidateQueries({ queryKey: qk.auth })
    },
  })
}

export function usePasskeys() {
  return useQuery({ queryKey: qk.passkeys, queryFn: api.passkeys })
}

export function usePasskeyMutations() {
  const qc = useQueryClient()
  const done = () => void qc.invalidateQueries({ queryKey: qk.passkeys })
  return {
    rename: useMutation({ mutationFn: ({ id, name }: { id: string; name: string }) => api.renamePasskey(id, name), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: string) => api.deletePasskey(id), onSuccess: done }),
  }
}

/* ---------------------------------- Recovery codes ---------------------------------- */

export function useRecoveryStatus() {
  return useQuery({ queryKey: qk.recovery, queryFn: api.recoveryStatus })
}

export function useRecoveryMutations() {
  const qc = useQueryClient()
  return {
    generate: useMutation({ mutationFn: api.recoveryGenerate, onSuccess: () => void qc.invalidateQueries({ queryKey: qk.recovery }) }),
  }
}

/* ---------------------------------- MCP server ---------------------------------- */

export function useMcpStatus() {
  return useQuery({ queryKey: qk.mcp, queryFn: api.mcpStatus })
}

export function useMcpMutations() {
  const qc = useQueryClient()
  const done = () => void qc.invalidateQueries({ queryKey: qk.mcp })
  return {
    setEnabled: useMutation({ mutationFn: (enabled: boolean) => api.mcpSetEnabled(enabled), onSuccess: done }),
    generateToken: useMutation({ mutationFn: api.mcpGenerateToken, onSuccess: done }),
    revokeToken: useMutation({ mutationFn: api.mcpRevokeToken, onSuccess: done }),
  }
}

/* ---------------------------------- Integrations & sync ---------------------------------- */

export function useIntegrations(enabled = true) {
  return useQuery({ queryKey: qk.integrations, queryFn: api.integrations, enabled })
}

function setIntegrations(qc: QueryClient, v: unknown) {
  if (v && typeof v === 'object' && 'garmin' in v && 'strava' in v) qc.setQueryData<Integrations>(qk.integrations, v as Integrations)
  void qc.invalidateQueries({ queryKey: qk.integrations })
}

export function useIntegrationMutations() {
  const qc = useQueryClient()
  return {
    connectGarmin: useMutation({
      mutationFn: (apiKey: string) => api.connectGarmin(apiKey),
      gcTime: 0,
      onSuccess: (v) => {
        setIntegrations(qc, v)
        void invalidateDomain(qc)
      },
    }),
    testGarmin: useMutation({ mutationFn: api.testGarmin, onSuccess: (v) => setIntegrations(qc, v) }),
    disconnectGarmin: useMutation({ mutationFn: api.disconnectGarmin, onSuccess: (v) => (setIntegrations(qc, v), void invalidateDomain(qc)) }),
    disconnectStrava: useMutation({ mutationFn: api.disconnectStrava, onSuccess: (v) => (setIntegrations(qc, v), void invalidateDomain(qc)) }),
  }
}

/** Polling interval factory for queries the event stream normally keeps fresh. */
export function useFallbackPoll() {
  const streamConnected = useStreamConnected()
  const jobsBusy = useHasActiveJob()
  return (busy = false) => fallbackPollInterval({ streamConnected, busy: busy || jobsBusy })
}

export function useSyncStatus(enabled = true) {
  const poll = useFallbackPoll()
  return useQuery({
    queryKey: qk.sync,
    queryFn: api.syncStatus,
    enabled,
    staleTime: 15_000,
    refetchInterval: (q) => poll(q.state.data?.running === true),
  })
}

export function useStartSync() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.startSync,
    onSuccess: (s: SyncStatus) => qc.setQueryData(qk.sync, s),
    onError: () => void qc.invalidateQueries({ queryKey: qk.sync }),
  })
}

/* ---------------------------------- Activities & days ---------------------------------- */

export function useActivities(params: Omit<ListActivitiesParams, 'before' | 'limit'> = {}, enabled = true) {
  return useInfiniteQuery({
    queryKey: qk.activities(params),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => api.activities({ ...params, limit: PAGE_SIZE, before: pageParam }),
    getNextPageParam: (last) => (last.hasMore && last.items.length ? last.items[last.items.length - 1].startLocal : undefined),
    enabled,
  })
}

/** Server cap of `GET /api/activities?limit=` (1..100); a larger value is rejected with a 400. */
export const ACTIVITIES_MAX_LIMIT = 100
const RANGE_MAX_PAGES = 20

/** Every activity dated within [from, to], paging with `before` past the server cap. */
export async function fetchActivitiesInRange(from: string, to: string): Promise<ActivityListResponse> {
  let res = await api.activities({ from, to, limit: ACTIVITIES_MAX_LIMIT })
  const items = [...res.items]
  for (let page = 1; res.hasMore && res.items.length > 0 && page < RANGE_MAX_PAGES; page++) {
    res = await api.activities({ from, to, limit: ACTIVITIES_MAX_LIMIT, before: res.items[res.items.length - 1].startLocal })
    items.push(...res.items)
  }
  return { items, hasMore: res.hasMore, freshness: res.freshness }
}

export function useActivitiesInRange(from: string, to: string, enabled = true) {
  return useQuery({
    queryKey: qk.activitiesRange(from, to),
    queryFn: () => fetchActivitiesInRange(from, to),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useActivityView(id: number | null) {
  return useQuery({
    queryKey: qk.activity(id ?? 0),
    queryFn: () => api.activity(id as number),
    enabled: id !== null,
    staleTime: 5 * 60_000,
    placeholderData: keepPreviousData,
  })
}

/**
 * The session immediately older than `before` (a `start_local` cursor on `date`): the list pages
 * newest-first only, so this is the only exact way to step back from an arbitrary session.
 */
export function useOlderActivity(date: string, before: string, enabled = true) {
  return useQuery({
    queryKey: qk.activityOlder(date, before),
    queryFn: () => api.activities({ to: date, before, limit: 1 }),
    enabled: enabled && date !== '' && before !== '',
  })
}

/**
 * Release check (§22, issue #143), administrators only: the server caches the GitHub answer for
 * six hours, so a page load costs nothing.
 */
export function useUpdates(enabled = true) {
  return useQuery({ queryKey: qk.updates, queryFn: api.updates, enabled, staleTime: 10 * 60_000 })
}

/** Experimental Garmin bridge (§23, issue #147): state only, no network call server-side. */
export function useGarminBridge() {
  // A 404 means the gateway is not built (bridge switched off at the instance level): no retry.
  return useQuery({
    queryKey: qk.garminBridge,
    queryFn: api.garminBridge,
    staleTime: 30_000,
    retry: (count, err) => !(err instanceof ApiError && err.status === 404) && count < 2,
  })
}

/** Login, MFA and disconnect of the bridge; each refreshes the state and the plan. */
export function useGarminBridgeMutations() {
  const qc = useQueryClient()
  const done = () => {
    void qc.invalidateQueries({ queryKey: qk.garminBridge })
    void qc.invalidateQueries({ queryKey: ['plan'] })
  }
  return {
    login: useMutation({ mutationFn: ({ email, password }: { email: string; password: string }) => api.garminBridgeLogin(email, password), onSuccess: done }),
    mfa: useMutation({ mutationFn: (code: string) => api.garminBridgeMFA(code), onSuccess: done }),
    disconnect: useMutation({ mutationFn: () => api.garminBridgeDisconnect(), onSuccess: done }),
  }
}

/** Notification centre (§21, issue #142): 50 newest entries and the unread badge. */
export function useInbox(enabled = true) {
  return useQuery({ queryKey: qk.inbox, queryFn: () => api.inbox(50), enabled })
}

export function useInboxMutations() {
  const qc = useQueryClient()
  // The badge follows the answer at once; the list is refetched so the read flags come from the
  // server (no optimistic drift on a 50-row page).
  const done = (r: { unread: number }) => {
    qc.setQueryData<InboxPage>(qk.inbox, (old) => (old ? { ...old, unread: r.unread } : old))
    void qc.invalidateQueries({ queryKey: qk.inbox })
  }
  return {
    markRead: useMutation({ mutationFn: (ids: number[]) => api.markInboxRead(ids), onSuccess: done }),
    markAll: useMutation({ mutationFn: () => api.markInboxAllRead(), onSuccess: done }),
  }
}

export function useDayView(date: string | null) {
  return useQuery({ queryKey: qk.day(date ?? ''), queryFn: () => api.day(date as string), enabled: date !== null, placeholderData: keepPreviousData })
}

export function useReport(subject: { kind: 'activity'; id: number } | { kind: 'day'; date: string } | null, include: IncludeKey[]) {
  const key = include.join(',')
  return useQuery({
    queryKey: subject ? qk.report(subject.kind, subject.kind === 'activity' ? subject.id : subject.date, key) : ['report', 'none'],
    queryFn: () => (subject!.kind === 'activity' ? api.activityReport(subject!.id, include) : api.dayReport(subject!.date, include)),
    enabled: subject !== null,
    placeholderData: keepPreviousData,
    staleTime: 2 * 60_000,
  })
}

export function useSaveNotes(activityId: number | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ActivityNotesInput) => api.saveNotes(activityId as number, body),
    onSuccess: (notes) => {
      if (activityId !== null) qc.setQueryData<ActivityView>(qk.activity(activityId), (v) => (v ? { ...v, notes } : v))
      void invalidateDomain(qc, ['activities', 'activity', 'report', 'day', 'week'])
    },
  })
}

export function useSaveCheckin(date: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: DailyCheckinInput) => api.saveCheckin(date, body),
    onSuccess: () => void invalidateDomain(qc, ['day', 'activity', 'report']),
  })
}

/* ---------------------------------- Profile, goals, plan, shoes ---------------------------------- */

export function useProfile(enabled = true) {
  return useQuery({ queryKey: qk.profile, queryFn: api.profile, staleTime: 10 * 60_000, enabled })
}

export function usePresets() {
  return useQuery({ queryKey: qk.presets, queryFn: api.presets, staleTime: Infinity })
}

export function useSaveProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (p: AthleteProfile) => api.saveProfile(p),
    onSuccess: (p) => {
      qc.setQueryData(qk.profile, p)
      void invalidateDomain(qc, ['activity', 'report', 'day', 'week'])
    },
  })
}

export function useGoals() {
  return useQuery({ queryKey: qk.goals, queryFn: api.goals })
}

export function useGoalMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['goals', 'activity', 'day', 'report'])
  return {
    create: useMutation({ mutationFn: (g: GoalInput) => api.createGoal(g), onSuccess: done }),
    update: useMutation({ mutationFn: ({ id, ...g }: Partial<GoalInput> & { id: number }) => api.updateGoal(id, g), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: number) => api.deleteGoal(id), onSuccess: done }),
  }
}

export function useWeekPlan(date: string | null) {
  return useQuery({ queryKey: qk.week(date ?? ''), queryFn: () => api.week(date as string), enabled: date !== null, placeholderData: keepPreviousData })
}

export function usePlanMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['plan', 'week', 'calendar', 'activity', 'activities', 'day', 'report'])
  return {
    create: useMutation({ mutationFn: (p: PlannedSessionInput) => api.createPlanned(p), onSuccess: done }),
    update: useMutation({ mutationFn: ({ id, ...p }: Partial<PlannedSessionInput> & { id: number }) => api.updatePlanned(id, p), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: number) => api.deletePlanned(id), onSuccess: done }),
  }
}

export function useShoes() {
  return useQuery({ queryKey: qk.shoes, queryFn: api.shoes })
}

export function useShoeMutations() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, ['shoes', 'activity', 'report'])
  return {
    create: useMutation({ mutationFn: (s: ShoeInput) => api.createShoe(s), onSuccess: done }),
    update: useMutation({ mutationFn: ({ id, ...s }: Partial<ShoeInput> & { id: number }) => api.updateShoe(id, s), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: number) => api.deleteShoe(id), onSuccess: done }),
    importStrava: useMutation({ mutationFn: api.importStravaShoes, onSuccess: done }),
  }
}

export function useMultiExport() {
  return useMutation({ mutationFn: (body: MultiExportRequest) => api.multiExport(body) })
}

export function useRestore() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ mode, backup }: { mode: 'merge' | 'replace'; backup: string }) => api.restore(mode, backup),
    onSuccess: () => void invalidateDomain(qc),
  })
}

/* ---------------------------------- Admin ---------------------------------- */

export function useAdminUsers() {
  return useQuery({ queryKey: qk.admin.users, queryFn: api.adminUsers })
}
export function useAdminInvitations() {
  return useQuery({ queryKey: qk.admin.invitations, queryFn: api.adminInvitations })
}
export function useAdminSettings() {
  return useQuery({ queryKey: qk.admin.settings, queryFn: api.adminSettings })
}
export function useAdminMeter() {
  return useQuery({ queryKey: qk.admin.meter, queryFn: api.adminMeter, refetchInterval: ADMIN_METER_POLL_MS })
}
export function useAdminLogs(f: LogFilter) {
  return useInfiniteQuery({
    queryKey: ['admin', 'logs', f.category, f.level, f.userId ?? 0],
    queryFn: ({ pageParam }) => api.adminLogs(f, pageParam || undefined),
    initialPageParam: 0,
    getNextPageParam: (last) => last.nextBefore ?? undefined,
  })
}
export function useVersion() {
  return useQuery({ queryKey: ['version'], queryFn: api.version, staleTime: Infinity })
}
export function useAdminConnections() {
  return useQuery({ queryKey: qk.admin.connections, queryFn: api.adminConnections })
}

export function useAdminMutations() {
  const qc = useQueryClient()
  const inv = (key: QueryKey) => () => void qc.invalidateQueries({ queryKey: key })
  return {
    updateUser: useMutation({ mutationFn: ({ id, ...b }: { id: number; status?: AdminUser['status']; role?: AdminUser['role'] }) => api.adminUpdateUser(id, b), onSuccess: inv(qk.admin.users) }),
    deleteUser: useMutation({ mutationFn: (id: number) => api.adminDeleteUser(id), onSuccess: inv(['admin']) }),
    createInvitation: useMutation({ mutationFn: api.adminCreateInvitation, onSuccess: inv(qk.admin.invitations) }),
    revokeInvitation: useMutation({ mutationFn: (id: number) => api.adminRevokeInvitation(id), onSuccess: inv(qk.admin.invitations) }),
    updateSettings: useMutation({
      mutationFn: (s: Partial<AdminSettings>) => api.adminUpdateSettings(s),
      onSuccess: (s) => {
        qc.setQueryData(qk.admin.settings, s)
        void qc.invalidateQueries({ queryKey: qk.auth })
      },
    }),
    flushCache: useMutation({ mutationFn: api.adminFlushCache, onSuccess: inv(qk.admin.meter) }),
    resync: useMutation({ mutationFn: (id: number) => api.adminResync(id), onSuccess: inv(['admin']) }),
    restore: useMutation({ mutationFn: ({ id, mode, backup }: { id: number; mode: 'merge' | 'replace'; backup: string }) => api.adminRestore(id, mode, backup) }),
  }
}

export * from './queriesV4'
export * from './queriesAdmin'
