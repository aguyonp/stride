import type fr from '../fr/onboarding'
import type { Widen } from '../t'

const en: Widen<typeof fr> = {
  view: {
    title: 'Welcome',
    steps: {
      health: { title: 'Your health status', short: 'Health' },
      garmin: { title: 'Garmin Connect', short: 'Garmin' },
      strava: { title: 'Strava', short: 'Strava' },
      done: { title: 'You’re all set', short: 'Done' },
    },
    progress: 'Progress',
    stepDone: ' (done)',
    stepCurrent: ' (current step)',
    next: 'Next',
    skip: 'Skip',
    hello: 'Hi {name}',
    skipAll: 'Skip all',
    stepOf: 'Step {n} of {total}',
    healthIntro: 'Stride tailors its alerts and AI prompts to your condition. You can change this later in Health & fitness.',
    garminIntro: 'With your Personal API key, Stride fetches your activities, biomechanics (FIT), sleep, HRV and Body Battery.',
    garminConnected: 'Garmin is connected (key {key}). Syncing starts automatically.',
    replaceKey: 'Replace the key',
    stravaIntro: 'Optional: Strava adds the title, description, RPE, Relative Effort and your shoes. Sessions are merged with Garmin when both exist.',
    stravaJustConnected: 'Strava is connected. Your sessions will be enriched automatically.',
    stravaFailed: 'Strava connection failed. You can try again or skip this step.',
    stravaFailedReason: 'Strava connection failed: {reason}. You can try again or skip this step.',
    stravaConnected: 'Strava is connected{name}.',
    readyIntro: 'Your space is ready. Here’s what’s set up:',
    health: 'Health',
    healthDefault: 'Healthy · performance mode',
    connectedKey: 'Connected · {key}',
    connected: 'Connected{name}',
    notConnected: 'Not connected',
    unavailable: 'Unavailable on this server',
    noSource: 'No source connected: the dashboard will stay empty until Garmin or Strava is linked. You can do it anytime in Settings → Integrations.',
    review: 'Review the steps',
    open: 'Open the dashboard',
    previous: '← Previous step',
  },
}
export default en
