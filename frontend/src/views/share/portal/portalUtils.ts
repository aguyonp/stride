import { useCallback, useState } from 'react'
import type { PrescribedBy, PublicShareView, ShareAuthorRole, SharePermissions } from '../../../api/types'
import { readStored, writeStored } from '../../../lib/storage'
import { isNum } from '../../../lib/format'

const NONE: SharePermissions = { comments: false, planning: false, activityDetails: false }

/** 4.1 payloads carry no permissions: read only. */
export const permissionsOf = (v: PublicShareView): SharePermissions => ({ ...NONE, ...v.permissions })

/** Items a link may edit: those it prescribed (the server answers 403 for anything else anyway). */
export function ownedByLink(item: { prescribedBy?: PrescribedBy | null }, v: PublicShareView): boolean {
  return item.prescribedBy?.linkId === v.linkId
}

export type PainTrend = { latest: number; previous: number | null; direction: 'up' | 'down' | 'stable' }

/** Next-morning (J+1) pain: latest value vs the mean of the three before (sessions newest first). */
export function painTrend(v: PublicShareView): PainTrend | null {
  const values = (v.recentSessions ?? []).map((s) => s.painNextMorning).filter(isNum)
  if (values.length === 0) {
    const zones = (v.bodyMap?.zones ?? []).map((z) => z.latestColdNextDayScore).filter(isNum)
    return zones.length ? { latest: Math.max(...zones), previous: null, direction: 'stable' } : null
  }
  const [latest, ...rest] = values
  const prev = rest.slice(0, 3)
  const previous = prev.length ? prev.reduce((a, b) => a + b, 0) / prev.length : null
  const delta = previous === null ? 0 : latest - previous
  return { latest, previous, direction: delta >= 0.75 ? 'up' : delta <= -0.75 ? 'down' : 'stable' }
}

export interface Author {
  name: string
  role: ShareAuthorRole
}
const AUTHOR_KEY = 'stride.share.author'

function readAuthor(): Author {
  try {
    const raw = readStored(AUTHOR_KEY)
    const v = raw ? (JSON.parse(raw) as Partial<Author>) : null
    return { name: typeof v?.name === 'string' ? v.name.slice(0, 80) : '', role: v?.role === 'physio' || v?.role === 'coach' || v?.role === 'other' ? v.role : 'physio' }
  } catch {
    return { name: '', role: 'physio' }
  }
}

/** The recipient's signature, remembered on this device only (1-tap saves afterwards). */
export function useAuthor(): [Author, (a: Author) => void] {
  const [author, setAuthor] = useState<Author>(readAuthor)
  const set = useCallback((a: Author) => {
    setAuthor(a)
    writeStored(AUTHOR_KEY, JSON.stringify(a))
  }, [])
  return [author, set]
}
