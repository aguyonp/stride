import type fr from '../fr/errors'
import type { Widen } from '../t'

const en: Widen<typeof fr> = {
  code: {
    BAD_REQUEST: 'Invalid request.',
    UNAUTHORIZED: 'Session expired: please sign in again.',
    FORBIDDEN: 'Access denied.',
    NOT_FOUND: 'Not found.',
    GONE: 'This content is no longer available.',
    CONFLICT: 'Conflicts with the current state: refresh and try again.',
    PRECONDITION_FAILED: 'A prerequisite for this action is missing.',
    RATE_LIMITED: 'Too many requests, try again in a moment.',
    PROVIDER_UNAUTHORIZED: 'The provider denied access: reconnect the integration.',
    PROVIDER_UNAVAILABLE: 'Provider unavailable, try again later.',
    PROVIDER_NOT_CONNECTED: 'Integration not connected.',
    INVITE_REQUIRED: 'An invitation is required to create an account.',
    SHARE_FORBIDDEN: 'This share link does not allow this action.',
    INTERNAL: 'Internal Stride server error.',
  },
  http: {
    network: 'Can’t reach the Stride server. Check your connection.',
    unauthorized: 'Session expired: please sign in again.',
    forbidden: 'Access denied.',
    rateLimited: 'Too many requests, try again in a moment.',
    status: 'Stride server error {status}.',
  },
}
export default en
