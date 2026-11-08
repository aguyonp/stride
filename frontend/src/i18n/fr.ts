// Source dictionary (§15.2). One namespace per feature under fr/; en.ts mirrors every key.
import admin from './fr/admin'
import auth from './fr/auth'
import coach from './fr/coach'
import common from './fr/common'
import dashboard from './fr/dashboard'
import errors from './fr/errors'
import health from './fr/health'
import labels from './fr/labels'
import layout from './fr/layout'
import onboarding from './fr/onboarding'
import plan from './fr/plan'
import session from './fr/session'
import settings from './fr/settings'
import share from './fr/share'
import shoes from './fr/shoes'

export const fr = { admin, auth, coach, common, dashboard, errors, health, labels, layout, onboarding, plan, session, settings, share, shoes } as const
export type Messages = typeof fr
