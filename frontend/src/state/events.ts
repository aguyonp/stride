import { useCallback, useRef, useSyncExternalStore } from 'react'
import type {
  JobStatus,
  LLMAnalysis,
  PlanAdaptPreview,
  PlanDraftsResponse,
  StrideStreamEventMap,
} from '../api/types'

export interface LLMStreamState {
  jobId: string
  kind: string
  ref: string
  text: string
  done: boolean
  error: string | null
  analysis: LLMAnalysis | null
  planDrafts: PlanDraftsResponse | null
  adaptPreview: PlanAdaptPreview | null
  updatedAt: number
}

export interface EventStoreState {
  connected: boolean
  lastConnectedAt: string | null
  jobs: Record<string, JobStatus>
  llmByJob: Record<string, LLMStreamState>
  llmJobByRef: Record<string, string>
}

const INITIAL_STATE: EventStoreState = {
  connected: false,
  lastConnectedAt: null,
  jobs: {},
  llmByJob: {},
  llmJobByRef: {},
}

let state: EventStoreState = INITIAL_STATE
const storeListeners = new Set<() => void>()

type EventListener<K extends keyof StrideStreamEventMap> = (payload: StrideStreamEventMap[K]) => void
const eventListeners: { [K in keyof StrideStreamEventMap]?: Set<EventListener<K>> } = {}

function notifyStore() {
  storeListeners.forEach((cb) => cb())
}

function patchState(updater: (prev: EventStoreState) => EventStoreState) {
  state = updater(state)
  notifyStore()
}

const subscribeStore = (cb: () => void) => {
  storeListeners.add(cb)
  return () => storeListeners.delete(cb)
}

// The snapshot is memoized per (state, selector): selectors that build arrays or objects would
// otherwise return a new reference on every read and make useSyncExternalStore loop forever.
export function useEventStore<T>(selector: (s: EventStoreState) => T): T {
  const cache = useRef<{ state: EventStoreState; selector: (s: EventStoreState) => T; value: T } | null>(null)
  const getSnapshot = () => {
    const c = cache.current
    if (c && c.state === state && c.selector === selector) return c.value
    const value = selector(state)
    cache.current = { state, selector, value }
    return value
  }
  return useSyncExternalStore(subscribeStore, getSnapshot)
}

export function getEventStoreState(): EventStoreState {
  return state
}

export function setStreamConnected(connected: boolean, serverTime?: string) {
  patchState((prev) => ({
    ...prev,
    connected,
    lastConnectedAt: connected ? (serverTime ?? new Date().toISOString()) : prev.lastConnectedAt,
  }))
}

export function upsertJobInStore(job: JobStatus) {
  patchState((prev) => ({
    ...prev,
    jobs: {
      ...prev.jobs,
      [job.id]: job,
    },
  }))
}

export function seedJobsInStore(items: JobStatus[]) {
  patchState((prev) => {
    const next = { ...prev.jobs }
    for (const item of items) next[item.id] = item
    return { ...prev, jobs: next }
  })
}

export const streamRefKey = (kind: string, ref: string) => `${kind}:${ref}`

export function startLlmStream(jobId: string, kind: string, ref: string) {
  const key = streamRefKey(kind, ref)
  patchState((prev) => ({
    ...prev,
    llmByJob: {
      ...prev.llmByJob,
      [jobId]: prev.llmByJob[jobId] ?? {
        jobId,
        kind,
        ref,
        text: '',
        done: false,
        error: null,
        analysis: null,
        planDrafts: null,
        adaptPreview: null,
        updatedAt: Date.now(),
      },
    },
    llmJobByRef: {
      ...prev.llmJobByRef,
      [key]: jobId,
    },
  }))
}

export function clearLlmStream(jobId: string | null, kind?: string, ref?: string) {
  patchState((prev) => {
    const nextByJob = { ...prev.llmByJob }
    const nextByRef = { ...prev.llmJobByRef }
    if (jobId) delete nextByJob[jobId]
    if (kind !== undefined && ref !== undefined) {
      const k = streamRefKey(kind, ref)
      const mapped = nextByRef[k]
      if (mapped && (!jobId || mapped === jobId)) delete nextByRef[k]
    }
    return { ...prev, llmByJob: nextByJob, llmJobByRef: nextByRef }
  })
}

function recordToken(ev: StrideStreamEventMap['llm.token']) {
  const key = streamRefKey(ev.kind, ev.ref)
  patchState((prev) => {
    const cur = prev.llmByJob[ev.jobId] ?? {
      jobId: ev.jobId,
      kind: ev.kind,
      ref: ev.ref,
      text: '',
      done: false,
      error: null,
      analysis: null,
      planDrafts: null,
      adaptPreview: null,
      updatedAt: Date.now(),
    }
    return {
      ...prev,
      llmByJob: {
        ...prev.llmByJob,
        [ev.jobId]: {
          ...cur,
          text: cur.text + ev.delta,
          updatedAt: Date.now(),
        },
      },
      llmJobByRef: {
        ...prev.llmJobByRef,
        [key]: ev.jobId,
      },
    }
  })
}

function recordDone(ev: StrideStreamEventMap['llm.done']) {
  const key = streamRefKey(ev.kind, ev.ref)
  patchState((prev) => {
    const cur = prev.llmByJob[ev.jobId] ?? {
      jobId: ev.jobId,
      kind: ev.kind,
      ref: ev.ref,
      text: '',
      done: false,
      error: null,
      analysis: null,
      planDrafts: null,
      adaptPreview: null,
      updatedAt: Date.now(),
    }
    return {
      ...prev,
      llmByJob: {
        ...prev.llmByJob,
        [ev.jobId]: {
          ...cur,
          done: true,
          error: ev.error ?? null,
          analysis: ev.analysis ?? cur.analysis,
          planDrafts: ev.planDrafts ?? cur.planDrafts,
          adaptPreview: ev.adaptPreview ?? cur.adaptPreview,
          text: ev.analysis?.markdown && !cur.text ? ev.analysis.markdown : cur.text,
          updatedAt: Date.now(),
        },
      },
      llmJobByRef: {
        ...prev.llmJobByRef,
        [key]: ev.jobId,
      },
    }
  })
}

export function subscribeStreamEvent<K extends keyof StrideStreamEventMap>(
  event: K,
  listener: EventListener<K>,
): () => void {
  let set = eventListeners[event] as Set<EventListener<K>> | undefined
  if (!set) {
    set = new Set<EventListener<K>>()
    ;(eventListeners as Record<string, unknown>)[event] = set
  }
  set.add(listener)
  return () => {
    set?.delete(listener)
  }
}

/**
 * Emits a real or synthetic SSE event into the Stride event pipeline.
 */
export function emitStreamEvent<K extends keyof StrideStreamEventMap>(
  event: K,
  payload: StrideStreamEventMap[K],
): void {
  if (event === 'connected') {
    const p = payload as StrideStreamEventMap['connected']
    setStreamConnected(true, p.serverTime)
  } else if (event === 'job.updated') {
    upsertJobInStore(payload as StrideStreamEventMap['job.updated'])
  } else if (event === 'llm.token') {
    recordToken(payload as StrideStreamEventMap['llm.token'])
  } else if (event === 'llm.done') {
    recordDone(payload as StrideStreamEventMap['llm.done'])
  }

  const listeners = eventListeners[event] as Set<EventListener<K>> | undefined
  if (listeners) {
    listeners.forEach((fn) => fn(payload))
  }
}

export function resetEventStore() {
  state = INITIAL_STATE
  notifyStore()
}

export function isJobStatus(v: unknown): v is JobStatus {
  return (
    typeof v === 'object' &&
    v !== null &&
    'id' in v &&
    'kind' in v &&
    'queue' in v &&
    'state' in v &&
    typeof (v as JobStatus).id === 'string' &&
    typeof (v as JobStatus).state === 'string'
  )
}

function selectActiveJobs(s: EventStoreState): JobStatus[] {
  const active: JobStatus[] = []
  for (const job of Object.values(s.jobs)) {
    if (job.state === 'queued' || job.state === 'running' || job.state === 'retrying') {
      active.push(job)
    }
  }
  active.sort((a, b) => (a.updatedAt > b.updatedAt ? -1 : 1))
  return active
}

export function useActiveJobs(): JobStatus[] {
  return useEventStore(selectActiveJobs)
}

export function useLlmStream(jobId: string | null, kind?: string, ref?: string): LLMStreamState | null {
  const select = useCallback(
    (s: EventStoreState) => {
      if (jobId && s.llmByJob[jobId]) return s.llmByJob[jobId]
      if (kind !== undefined && ref !== undefined) {
        const mappedId = s.llmJobByRef[streamRefKey(kind, ref)]
        if (mappedId && s.llmByJob[mappedId]) return s.llmByJob[mappedId]
      }
      return null
    },
    [jobId, kind, ref],
  )
  return useEventStore(select)
}
