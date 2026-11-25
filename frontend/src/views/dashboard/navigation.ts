import type { ActivitySummary } from '../../api/types'

export interface Neighbours {
  older: ActivitySummary | null
  newer: ActivitySummary | null
}

/**
 * Previous / next session in a list ordered like `GET /api/activities`
 * (`start_local DESC, id DESC`): `older` is the next entry, `newer` the previous one.
 * An unknown id or a list edge yields null, so the caller keeps that arrow disabled.
 */
export function neighboursOf(list: readonly ActivitySummary[], id: number | null): Neighbours {
  const i = id === null ? -1 : list.findIndex((a) => a.id === id)
  if (i < 0) return { older: null, newer: null }
  return { older: list[i + 1] ?? null, newer: list[i - 1] ?? null }
}

/**
 * Neighbours of a session that is not among the loaded pages (opened from the calendar, or from
 * `?s=`). The list endpoint pages newest-first only, so each side has its own exact query:
 * the session immediately below comes from a `before` cursor, the one above from every session
 * dated on or after it (`complete` is false when that listing was truncated, and the arrow then
 * stays disabled rather than jumping to a wrong session — issue #21).
 */
export function outsideNeighbours(
  id: number | null,
  older: ActivitySummary | null,
  since: readonly ActivitySummary[],
  complete: boolean,
): Neighbours {
  return { older, newer: complete ? neighboursOf(since, id).newer : null }
}
