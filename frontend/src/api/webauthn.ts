import { browserSupportsWebAuthn, startAuthentication, startRegistration, WebAuthnError } from '@simplewebauthn/browser'
import type { PublicKeyCredentialCreationOptionsJSON, PublicKeyCredentialRequestOptionsJSON } from '@simplewebauthn/browser'
import { api, ApiError, IS_MOCK } from './client'
import type { Me, Passkey } from './types'

export type PasskeyIssue = 'insecure' | 'unsupported'

export const ISSUE_MESSAGE: Record<PasskeyIssue, string> = {
  insecure: 'Les passkeys exigent une connexion sécurisée : ouvre Stride via son adresse https:// (http://localhost est accepté en local).',
  unsupported: 'Ce navigateur ne prend pas en charge les passkeys (WebAuthn). Mets-le à jour ou utilise un Safari, Chrome, Edge ou Firefox récent.',
}

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

export function passkeyErrorMessage(err: unknown, flow: 'login' | 'register' | 'add'): string {
  if (err instanceof ApiError) {
    if (err.code === 'INVITE_REQUIRED') return 'Une invitation valide est nécessaire pour créer un compte. Utilise le lien reçu de l’administrateur ou saisis ton code.'
    if (err.status === 0) return err.message
    return err.message
  }
  if (isCancel(err)) return flow === 'login' ? 'Connexion annulée (ou délai dépassé). Réessaie quand tu es prêt.' : 'Création de la passkey annulée (ou délai dépassé).'
  if (err instanceof WebAuthnError) {
    if (err.code === 'ERROR_AUTHENTICATOR_PREVIOUSLY_REGISTERED') return 'Cet appareil possède déjà une passkey pour ce compte.'
    if (err.code === 'ERROR_INVALID_DOMAIN' || err.code === 'ERROR_INVALID_RP_ID') return 'Ce domaine n’est pas autorisé pour les passkeys (HTTPS et domaine configuré requis).'
    if (err.code === 'ERROR_AUTHENTICATOR_MISSING_DISCOVERABLE_CREDENTIAL_SUPPORT') return 'Cet authentificateur ne sait pas stocker de passkey (clé résidente requise).'
  }
  if (err instanceof Error && err.name === 'SecurityError') return ISSUE_MESSAGE.insecure
  return flow === 'login' ? 'Impossible d’utiliser la passkey. Vérifie que tu en as une pour ce site.' : 'La passkey n’a pas pu être créée.'
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
