import { WebAuthnError } from '@simplewebauthn/browser'
import type { WebAuthnErrorCode } from '@simplewebauthn/browser'
import { describe, expect, it } from 'vitest'
import { ApiError } from './client'
import { ISSUE_MESSAGE, passkeyErrorMessage } from './webauthn'

function domErr(name: string, message = name): Error {
  const err = new Error(message)
  err.name = name
  return err
}

function webauthnErr(code: WebAuthnErrorCode, causeName = 'Error'): WebAuthnError {
  return new WebAuthnError({
    message: code,
    code,
    cause: domErr(causeName),
  })
}

describe('passkeyErrorMessage', () => {
  it('explique clairement une invitation requise', () => {
    const err = new ApiError('invite required', 403, 'INVITE_REQUIRED')
    expect(passkeyErrorMessage(err, 'register')).toContain('Une invitation valide est nécessaire')
  })

  it('distingue une annulation utilisateur en connexion et en création', () => {
    const aborted = webauthnErr('ERROR_CEREMONY_ABORTED', 'AbortError')
    const notAllowed = webauthnErr('ERROR_PASSTHROUGH_SEE_CAUSE_PROPERTY', 'NotAllowedError')
    expect(passkeyErrorMessage(aborted, 'login')).toContain('Connexion annulée')
    expect(passkeyErrorMessage(notAllowed, 'register')).toContain('Création de la passkey annulée')
  })

  it('explique l’absence d’authentificateur disponible (UnknownError / NotSupportedError)', () => {
    const general = webauthnErr('ERROR_AUTHENTICATOR_GENERAL_ERROR', 'UnknownError')
    const noAlg = webauthnErr('ERROR_AUTHENTICATOR_NO_SUPPORTED_PUBKEYCREDPARAMS_ALG', 'NotSupportedError')
    expect(passkeyErrorMessage(general, 'register')).toContain('Aucun authentificateur de passkey disponible')
    expect(passkeyErrorMessage(general, 'register')).toContain('Windows Hello')
    expect(passkeyErrorMessage(noAlg, 'add')).toContain('FIDO2')
    expect(passkeyErrorMessage(general, 'login')).toContain('Aucun authentificateur de passkey disponible sur cet appareil')
    expect(passkeyErrorMessage(domErr('UnknownError'), 'register')).toContain('Aucun authentificateur de passkey disponible')
    expect(passkeyErrorMessage(domErr('NotSupportedError'), 'login')).toContain('Aucun authentificateur de passkey disponible sur cet appareil')
  })

  it('détaille les contraintes WebAuthn courantes (clé résidente, UV, doublon, domaine)', () => {
    expect(passkeyErrorMessage(webauthnErr('ERROR_AUTHENTICATOR_PREVIOUSLY_REGISTERED'), 'add')).toContain('possède déjà une passkey')
    expect(passkeyErrorMessage(webauthnErr('ERROR_AUTHENTICATOR_MISSING_DISCOVERABLE_CREDENTIAL_SUPPORT'), 'register')).toContain('clé résidente requise')
    expect(passkeyErrorMessage(webauthnErr('ERROR_AUTHENTICATOR_MISSING_USER_VERIFICATION_SUPPORT'), 'register')).toContain('code PIN ou biométrie')
    expect(passkeyErrorMessage(webauthnErr('ERROR_INVALID_RP_ID'), 'login')).toContain('domaine n’est pas autorisé')
    expect(passkeyErrorMessage(domErr('SecurityError'), 'register')).toBe(ISSUE_MESSAGE.insecure)
  })
})
