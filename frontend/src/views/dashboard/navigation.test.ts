import { describe, expect, it } from 'vitest'
import type { ActivitySummary } from '../../api/types'
import { neighboursOf, outsideNeighbours } from './navigation'

/** Only the fields the navigation reads matter here. */
function act(id: number, startLocal: string): ActivitySummary {
  return { id, startLocal, date: startLocal.slice(0, 10), name: `Séance ${id}` } as ActivitySummary
}

// Newest first, as GET /api/activities orders them (start_local DESC, id DESC).
const list = [
  act(40, '2026-11-24T07:00:00'),
  act(39, '2026-11-22T07:00:00'),
  act(38, '2026-11-22T06:00:00'),
  act(37, '2026-11-20T18:30:00'),
]

describe('neighboursOf', () => {
  it('gives both neighbours inside the list', () => {
    expect(neighboursOf(list, 39)).toEqual({ older: list[2], newer: list[0] })
  })

  it('leaves the missing side null at the edges', () => {
    expect(neighboursOf(list, 40)).toEqual({ older: list[1], newer: null })
    expect(neighboursOf(list, 37)).toEqual({ older: null, newer: list[2] })
  })

  it('returns nothing for an unknown or absent selection', () => {
    expect(neighboursOf(list, 999)).toEqual({ older: null, newer: null })
    expect(neighboursOf(list, null)).toEqual({ older: null, newer: null })
  })

  it('handles an empty list and a single session', () => {
    expect(neighboursOf([], 40)).toEqual({ older: null, newer: null })
    expect(neighboursOf([act(1, '2026-11-24T07:00:00')], 1)).toEqual({ older: null, newer: null })
  })

  it('keeps two sessions of the same day in the server order (start_local DESC, id DESC)', () => {
    // Same day: the later start comes first, so the earlier one is the older neighbour.
    expect(neighboursOf(list, 38)).toEqual({ older: list[3], newer: list[1] })
  })
})

describe('outsideNeighbours', () => {
  // Every session dated on or after the selected one, newest first; the oldest of them is 38.
  const since = [act(50, '2026-11-30T07:00:00'), act(40, '2026-11-24T07:00:00'), act(38, '2026-11-24T06:00:00')]

  it('takes the newer side from the listing and the older side from the cursor', () => {
    const older = act(31, '2026-10-02T07:00:00')
    expect(outsideNeighbours(40, older, since, true)).toEqual({ older, newer: since[0] })
  })

  it('has no newer neighbour when the session is the most recent of the listing', () => {
    expect(outsideNeighbours(50, act(49, '2026-11-29T07:00:00'), since, true)).toEqual({
      older: act(49, '2026-11-29T07:00:00'),
      newer: null,
    })
  })

  it('keeps the newer arrow disabled when the listing was truncated', () => {
    // The cursor is exact whatever the listing: only the newer side depends on its completeness.
    const older = act(31, '2026-10-02T07:00:00')
    expect(outsideNeighbours(40, older, since, false)).toEqual({ older, newer: null })
  })

  it('reports the oldest session of the history without a single neighbour', () => {
    expect(outsideNeighbours(7, null, [act(7, '2025-04-14T08:45:00')], true)).toEqual({ older: null, newer: null })
  })
})
