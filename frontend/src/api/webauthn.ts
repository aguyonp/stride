import { browserSupportsWebAuthn, startAuthentication, startRegistration, WebAuthnError } from '@simplewebauthn/browser'
import type { PublicKeyCredentialCreationOptionsJSON, PublicKeyCredentialRequestOptionsJSON } from '@simplewebauthn/browser'
import { api, ApiError, IS_MOCK } from './client'
import type { Me, Passkey } from './types'
import { lazyRecord, t } from '../i18n/t'

export type PasskeyIssue = 'insecure' | 'unsupported'

export const ISSUE_MESSAGE: Record<PasskeyIssue, string> = lazyRecord(['insecure', 'unsupported'] as const, (k) => t(`auth.passkey.${k}`))

export function passkeyIssue(): PasskeyIssue | null {
  if (IS_MOCK) return null
  if (typeof window === 'undefined' || !window.isSecureContext) return 'insecure'
  if (!browserSupportsWebAuthn()) return 'unsupported'
  return null
}

class PasskeyCancelled extends Error {
  constructor() {
    super('cancelled')
    this.name = 'PasskeyCancelled'
  }
}

function isCancel(err: unknown): boolean {
  if (err instanceof PasskeyCancelled) return true
  if (err instanceof WebAuthnError) return err.code === 'ERROR_CEREMONY_ABORTED' || (err.cause instanceof Error && err.cause.name === 'NotAllowedError')
  return err instanceof Error && (err.name === 'NotAllowedError' || err.name === 'AbortError')
}

const noAuthenticator = (flow: 'login' | 'register' | 'add') => t(flow === 'login' ? 'auth.passkey.noAuthenticatorLogin' : 'auth.passkey.noAuthenticatorRegister')

export function passkeyErrorMessage(err: unknown, flow: 'login' | 'register' | 'add'): string {
  if (err instanceof ApiError) {
    if (err.code === 'INVITE_REQUIRED') return t('auth.passkey.inviteRequired')
    if (err.status === 0) return err.message
    return err.message
  }
  if (isCancel(err)) return t(flow === 'login' ? 'auth.passkey.loginCancelled' : 'auth.passkey.createCancelled')
  if (err instanceof WebAuthnError) {
    if (err.code === 'ERROR_AUTHENTICATOR_PREVIOUSLY_REGISTERED') return t('auth.passkey.alreadyRegistered')
    if (err.code === 'ERROR_INVALID_DOMAIN' || err.code === 'ERROR_INVALID_RP_ID') return t('auth.passkey.invalidDomain')
    if (err.code === 'ERROR_AUTHENTICATOR_MISSING_DISCOVERABLE_CREDENTIAL_SUPPORT') return t('auth.passkey.noResidentKey')
    if (err.code === 'ERROR_AUTHENTICATOR_MISSING_USER_VERIFICATION_SUPPORT' || err.code === 'ERROR_AUTO_REGISTER_USER_VERIFICATION_FAILURE') {
      return t('auth.passkey.noUserVerification')
    }
    if (err.code === 'ERROR_AUTHENTICATOR_GENERAL_ERROR' || err.code === 'ERROR_AUTHENTICATOR_NO_SUPPORTED_PUBKEYCREDPARAMS_ALG') {
      return noAuthenticator(flow)
    }
  }
  if (err instanceof Error) {
    if (err.name === 'SecurityError') return ISSUE_MESSAGE.insecure
    if (err.name === 'UnknownError' || err.name === 'NotSupportedError') {
      return noAuthenticator(flow)
    }
  }
  return t(flow === 'login' ? 'auth.passkey.loginFailed' : 'auth.passkey.createFailed')
}

function ensureEnvironment() {
  const issue = passkeyIssue()
  if (issue) throw new Error(ISSUE_MESSAGE[issue])
}

async function create(publicKey: unknown): Promise<unknown> {
  if (import.meta.env.VITE_MOCK === '1') {
    const { mockCeremony } = await import('../mocks/webauthn')
    return mockCeremony('create')
  }
  return startRegistration({ optionsJSON: publicKey as PublicKeyCredentialCreationOptionsJSON })
}

async function get(publicKey: unknown): Promise<unknown> {
  if (import.meta.env.VITE_MOCK === '1') {
    const { mockCeremony } = await import('../mocks/webauthn')
    return mockCeremony('get')
  }
  return startAuthentication({ optionsJSON: publicKey as PublicKeyCredentialRequestOptionsJSON })
}

export async function loginWithPasskey(): Promise<Me> {
  ensureEnvironment()
  const { publicKey } = await api.loginBegin()
  const assertion = await get(publicKey)
  return api.loginFinish(assertion)
}

export async function registerWithPasskey(displayName: string, inviteCode?: string): Promise<Me> {
  ensureEnvironment()
  const { publicKey } = await api.registerBegin({ displayName, inviteCode: inviteCode || undefined })
  const attestation = await create(publicKey)
  return api.registerFinish(attestation)
}

export async function addPasskey(): Promise<Passkey> {
  ensureEnvironment()
  const { publicKey } = await api.passkeyBegin()
  const attestation = await create(publicKey)
  return api.passkeyFinish(attestation)
}
