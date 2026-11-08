import type fr from '../fr/labels'
import type { Widen } from '../t'

const en: Widen<typeof fr> = {
  hrv: {
    BALANCED: 'Balanced',
    UNBALANCED: 'Unbalanced',
    LOW: 'Low',
    POOR: 'Poor',
  },
  sleep: {
    EXCELLENT: 'Excellent',
    GOOD: 'Good',
    FAIR: 'Fair',
    POOR: 'Poor',
  },
  drift: {
    stable: 'Stable',
    moderate: 'Moderate',
    high: 'High',
  },
  acwr: {
    low: 'Low',
    optimal: 'Optimal',
    high: 'High',
    danger: 'Danger',
  },
  side: {
    left: 'left',
    right: 'right',
    both: 'both',
  },
  te: {
    RECOVERY: 'recovery',
    AEROBIC_BASE: 'aerobic base',
    TEMPO: 'tempo',
    LACTATE_THRESHOLD: 'lactate threshold',
    VO2MAX: 'VO2max',
    ANAEROBIC_CAPACITY: 'anaerobic capacity',
    SPEED: 'speed',
  },
  focus: {
    none: '100% performance – no injury',
    knee_pfps: 'Knee / patellofemoral pain',
    achilles: 'Achilles tendon',
    shin_splints: 'Shin splints',
    other: 'Other',
  },
  contextTag: {
    metronome: 'Metronome on',
    warmup_iso: 'Warm-up / pre-run isometric activation',
    squatting_48h: 'DIY work / squatting positions (48h)',
    desk_sitting: 'Prolonged desk sitting',
  },
  planSport: {
    run: 'Run',
    bike: 'Bike',
    strength: 'Strength',
    mobility: 'Mobility',
    rest: 'Rest',
    other: 'Other',
  },
  match: {
    done: 'Done',
    partial: 'Partial',
    missed: 'Missed',
    upcoming: 'Upcoming',
    extra: 'Unplanned',
  },
  goalKind: {
    macro: 'Macro goal',
    race: 'Target race',
    cadence: 'Cadence',
    weekly_volume: 'Weekly volume',
    other: 'Other',
  },
  shoeType: {
    soft: 'Cushioned',
    tempo: 'Tempo',
    carbon: 'Carbon',
  },
  freshness: {
    db: 'Local database',
    dbHint: 'Data read from the Stride database (no provider call).',
    cache: 'Cache',
    cacheHint: 'Data served from cache (a few minutes old at most).',
    live: 'Live sync',
    liveHint: 'A Garmin / Strava call was made for this response.',
  },
  source: {
    garmin: 'Garmin',
    strava: 'Strava',
    import: 'Import',
    manual: 'Manual',
  },
  sport: {
    run: 'Run',
    bike: 'Bike',
    walk: 'Walk',
    hike: 'Hike',
    swim: 'Swim',
    strength: 'Strength',
    other: 'Other',
  },
  eventKind: {
    fatigue: { label: 'Fatigue / soreness', short: 'Fatigue', hint: 'Heavy legs, short sleep, busy day' },
    pain: { label: 'Pain / injury alert', short: 'Pain', hint: 'Lingering discomfort, sensitive area' },
    illness: { label: 'Illness', short: 'Illness', hint: 'Cold, fever, stomach bug' },
    unavailable: { label: 'Unavailable / travel', short: 'Away', hint: 'Travel, work, family' },
    forced_rest: { label: 'Forced rest', short: 'Forced rest', hint: 'Medical advice, complete stop' },
  },
  severity: {
    '1': 'Mild',
    '2': 'Moderate',
    '3': 'Severe',
  },
  recordSource: {
    computed: 'computed',
    strava: 'Strava',
    manual: 'manual',
  },
}
export default en
