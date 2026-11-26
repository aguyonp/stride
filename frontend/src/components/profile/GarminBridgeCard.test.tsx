// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { db, resetDb, userData } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { clickEl, installDomPolyfills, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { GarminBridgeCard } from './GarminBridgeCard'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const text = () => document.body.textContent ?? ''
const btn = (label: string) => [...document.querySelectorAll('button')].find((b) => b.textContent?.includes(label)) ?? null
const inputs = () => Array.from(document.querySelectorAll('input')) as HTMLInputElement[]

describe('GarminBridgeCard (pont expérimental)', () => {
  let mounted: MountedApp | null = null
  let requests: string[] = []

  beforeEach(() => {
    requests = []
    installDomPolyfills()
    setFlags([])
    resetDb()
    // The bridge is off by default in the demo: these tests exercise the flow once an
    // administrator has opened it (the « hidden » test turns it off again).
    db().garminBridgeEnabled = true
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string, init?: RequestInit) => {
        requests.push(`${init?.method ?? 'GET'} ${String(input)}`)
        return mockFetch(String(input), init)
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
    resetDb()
  })

  it('shows nothing when the administrator has not opened the bridge', async () => {
    db().garminBridgeEnabled = false
    mounted = await mountWithProviders(<GarminBridgeCard />)
    // The answer lands asynchronously: wait for the card to stay empty rather than for the request.
    await waitFor(() => expect(requests.some((r) => r.includes('/api/garmin/bridge'))).toBe(true))
    await waitFor(() => expect(text()).toBe(''))
  })

  it('warns before any credential is typed, then connects and disconnects', async () => {
    mounted = await mountWithProviders(<GarminBridgeCard />)
    await waitFor(() => expect(text()).toContain('Déconnecté'))
    // The warning is readable before the form exists.
    expect(text()).toContain('non officiel, instable')
    expect(inputs().length).toBe(0)

    await clickEl(btn('Activer le pont'))
    await waitFor(() => expect(text()).toContain('Avant de continuer'))
    expect(text()).toContain('peut suspendre un compte')
    expect(text()).toContain('ni stocké, ni journalisé')

    // A wrong password is refused and the field is cleared: nothing stays in the DOM.
    await setInputValue(inputs()[0], 'antoine@example.com')
    await setInputValue(inputs()[1], 'faux')
    await clickEl(btn('Se connecter'))
    // Refused: the password is dropped at once and the state does not move.
    await waitFor(() => expect(inputs()[1]?.value).toBe(''))
    expect(text()).toContain('Déconnecté')
    expect(userData(1).garminBridge).toBeUndefined()

    await setInputValue(inputs()[1], 'motdepasse')
    await clickEl(btn('Se connecter'))
    await waitFor(() => expect(text()).toContain('Connecté'))
    expect(inputs().length).toBe(0)
    expect(userData(1).garminBridge?.state).toBe('connected')

    await clickEl(btn('Déconnecter'))
    await waitFor(() => expect(text()).toContain('Déconnecté'))
    expect(userData(1).garminBridge).toBeUndefined()
  })

  it('walks the MFA step', async () => {
    mounted = await mountWithProviders(<GarminBridgeCard />)
    await waitFor(() => expect(text()).toContain('Déconnecté'))
    await clickEl(btn('Activer le pont'))
    await waitFor(() => expect(inputs().length).toBe(2))

    await setInputValue(inputs()[0], 'mfa@example.com')
    await setInputValue(inputs()[1], 'motdepasse')
    await clickEl(btn('Se connecter'))
    // A code is required: the form switches to the code field.
    await waitFor(() => expect(text()).toContain('Code requis'))
    expect(text()).toContain('Garmin a envoyé un code')

    await setInputValue(inputs()[0], '000000')
    await clickEl(btn('Vérifier'))
    // A wrong code keeps the challenge open and clears the field.
    await waitFor(() => expect(inputs()[0]?.value).toBe(''))
    expect(text()).toContain('Code requis')

    await setInputValue(inputs()[0], '123456')
    await clickEl(btn('Vérifier'))
    await waitFor(() => expect(text()).toContain('Connecté'))
  })

  it('offers a reconnect when the session expired or Garmin blocks', async () => {
    userData(1).garminBridge = { state: 'expired' }
    db().data['1'] = userData(1)
    mounted = await mountWithProviders(<GarminBridgeCard />)
    await waitFor(() => expect(text()).toContain('Expiré'))
    expect(text()).toContain('session Garmin a expiré')
    expect(btn('Reconnecter')).not.toBeNull()
    // The fallback is always stated.
    expect(text()).toContain('importez-le dans Garmin Connect Web')
  })
})
