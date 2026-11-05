// Fake WebAuthn ceremonies: never touches navigator.credentials.

import { has } from './flags'

const b64url = (len: number) => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  let s = ''
  for (let i = 0; i < len; i++) s += chars[Math.floor(Math.random() * chars.length)]
  return s
}

export async function mockCeremony(kind: 'create' | 'get'): Promise<unknown> {
  await new Promise((r) => setTimeout(r, 600))
  if (has('pkcancel')) throw new DOMException('The operation either timed out or was not allowed.', 'NotAllowedError')
  const id = b64url(22)
  if (kind === 'create') {
    return {
      id,
      rawId: id,
      type: 'public-key',
      authenticatorAttachment: 'platform',
      clientExtensionResults: { credProps: { rk: true } },
      response: { clientDataJSON: b64url(120), attestationObject: b64url(200), transports: ['internal', 'hybrid'] },
    }
  }
  return {
    id,
    rawId: id,
    type: 'public-key',
    authenticatorAttachment: 'platform',
    clientExtensionResults: {},
    response: { clientDataJSON: b64url(120), authenticatorData: b64url(50), signature: b64url(70), userHandle: b64url(16) },
  }
}
