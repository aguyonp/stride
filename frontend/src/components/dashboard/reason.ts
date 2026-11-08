import type { ActivityView } from '../../api/types'
import { t } from '../../i18n'

/** Reason shown next to N/D for Garmin-only metrics when the session has no Garmin source. */
export const garminReason = (v: ActivityView) => (v.activity.sources.includes('garmin') ? null : t('dashboard.session.garminOff'))
