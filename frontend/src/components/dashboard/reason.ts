import type { ActivityView } from '../../api/types'

/** Reason shown next to N/D for Garmin-only metrics when the session has no Garmin source. */
export const garminReason = (v: ActivityView) => (v.activity.sources.includes('garmin') ? null : 'Garmin non connecté')
