// 4.2 (§15): physio/coach portal notes, public portal writes, atomic backups, Smart Workout Morphing.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { upsertJobInStore } from '../state/events'
import { api } from './client'
import { invalidateDomain, qk4 } from './queries'
import { PLAN_ROOTS } from './queriesV3'
import type {
  CalendarEventInput,
  PlannedSession,
  PlannedSessionInput,
  S3ConfigInput,
  ShareDirectiveInput,
  ShareRemarkInput,
} from './types'

export const qk42 = {
  shareNotes: (active: boolean) => ['share', 'notes', active ? 'active' : 'all'] as const,
  publicActivity: (token: string, id: number) => ['publicShare', token, 'activity', id] as const,
  backups: ['admin', 'backups'] as const,
  morph: (id: number) => ['plan', 'morph', id] as const,
}

/** Roots refreshed when a share link writes something for the athlete (SSE `share.updated`). */
export const SHARE_UPDATE_ROOTS: Record<'directive' | 'remark' | 'plan' | 'event', string[]> = {
  directive: ['share', 'plan', 'report', 'day'],
  remark: ['share', 'activities', 'activity', 'report', 'plan'],
  plan: ['share', ...PLAN_ROOTS],
  event: ['share', 'events', 'calendar', 'plan', 'report'],
}

/* ---------------------------------- Athlete side ---------------------------------- */

/** Physio/coach notes: active directives and remarks (`active`), or every note. */
export function useShareNotes(active = true, enabled = true) {
  return useQuery({ queryKey: qk42.shareNotes(active), queryFn: () => api.shareNotes(active), enabled, staleTime: 5 * 60_000 })
}

export function useDeleteShareNote() {
  const qc = useQueryClient()
  return useMutation({ mutationFn: (id: number) => api.deleteShareNote(id), onSuccess: () => void invalidateDomain(qc, ['share', 'plan', 'activities', 'activity', 'report']) })
}

/* ---------------------------------- Public portal ---------------------------------- */

export function usePublicActivity(token: string, id: number | null) {
  return useQuery({
    queryKey: qk42.publicActivity(token, id ?? 0),
    queryFn: () => api.publicActivity(token, id as number),
    enabled: id !== null,
    retry: false,
    staleTime: 60_000,
  })
}

/** Writes from the share portal; each one refetches the public view (the server drops its 5-min cache). */
export function usePublicShareMutations(token: string) {
  const qc = useQueryClient()
  const done = () => void qc.invalidateQueries({ queryKey: qk4.publicShare(token) })
  return {
    putDirective: useMutation({ mutationFn: (b: ShareDirectiveInput) => api.publicPutDirective(token, b), onSuccess: done }),
    deleteDirective: useMutation({ mutationFn: () => api.publicDeleteDirective(token), onSuccess: done }),
    putRemark: useMutation({ mutationFn: ({ activityId, ...b }: ShareRemarkInput & { activityId: number }) => api.publicPutRemark(token, activityId, b), onSuccess: done }),
    deleteRemark: useMutation({ mutationFn: (activityId: number) => api.publicDeleteRemark(token, activityId), onSuccess: done }),
    createPlanned: useMutation({ mutationFn: (b: PlannedSessionInput) => api.publicCreatePlanned(token, b), onSuccess: done }),
    updatePlanned: useMutation({ mutationFn: ({ id, ...b }: Partial<PlannedSessionInput> & { id: number }) => api.publicUpdatePlanned(token, id, b), onSuccess: done }),
    deletePlanned: useMutation({ mutationFn: (id: number) => api.publicDeletePlanned(token, id), onSuccess: done }),
    createEvent: useMutation({ mutationFn: (b: CalendarEventInput) => api.publicCreateEvent(token, b), onSuccess: done }),
    updateEvent: useMutation({ mutationFn: ({ id, ...b }: Partial<CalendarEventInput> & { id: number }) => api.publicUpdateEvent(token, id, b), onSuccess: done }),
    deleteEvent: useMutation({ mutationFn: (id: number) => api.publicDeleteEvent(token, id), onSuccess: done }),
  }
}

/* ---------------------------------- Admin backups ---------------------------------- */

export function useAdminBackups() {
  return useQuery({ queryKey: qk42.backups, queryFn: api.adminBackups })
}

/** Backup jobs report progress through the event store; the list refreshes when they finish (EventStreamController). */
export function useBackupActions() {
  const qc = useQueryClient()
  const refresh = () => void qc.invalidateQueries({ queryKey: qk42.backups })
  return {
    create: useMutation({ mutationFn: (upload: boolean) => api.adminCreateBackup(upload), onSuccess: (job) => (upsertJobInStore(job), refresh()) }),
    upload: useMutation({ mutationFn: (name: string) => api.adminUploadBackup(name), onSuccess: (job) => (upsertJobInStore(job), refresh()) }),
    remove: useMutation({ mutationFn: ({ name, remote }: { name: string; remote: boolean }) => api.adminDeleteBackup(name, remote), onSuccess: refresh }),
    saveS3: useMutation({ mutationFn: (b: S3ConfigInput) => api.adminSaveS3Config(b), onSuccess: refresh }),
    testS3: useMutation({ mutationFn: (b: S3ConfigInput) => api.adminTestS3(b) }),
  }
}

/* ---------------------------------- Smart Workout Morphing ---------------------------------- */

/** Constraints and deterministic proposal for one planned session (J/J+1 only on the server side). */
export function useMorphPreview(id: number | null, enabled = true) {
  return useQuery({
    queryKey: qk42.morph(id ?? 0),
    queryFn: () => api.morphPreview(id as number),
    enabled: enabled && id !== null,
    retry: false,
    staleTime: 5 * 60_000,
  })
}

export function useMorphActions() {
  const qc = useQueryClient()
  const done = () => void invalidateDomain(qc, PLAN_ROOTS)
  return {
    refine: useMutation({ mutationFn: ({ id, note }: { id: number; note?: string }) => api.morphRefine(id, note) }),
    apply: useMutation({ mutationFn: ({ id, proposal }: { id: number; proposal: PlannedSession }) => api.morphApply(id, proposal), onSuccess: done }),
    revert: useMutation({ mutationFn: (id: number) => api.morphRevert(id), onSuccess: done }),
  }
}
