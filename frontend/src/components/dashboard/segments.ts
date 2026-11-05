import type { Segment } from '../../api/types'

const PAUSE_KINDS = new Set(['walk', 'stand', 'rest'])
export const isPause = (s: Segment) => PAUSE_KINDS.has(s.kind)
