import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// Experimental Garmin bridge (§23, issue #147): the in-app sign-in (password, then code), the FIT
// fallback without a cable, and the bulk push of the day. The mock answers 'motdepasse' and the
// code '123456'; an address starting with « mfa » asks for a code.

test('connects the bridge with a code and sends the sessions of the day', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  // The bridge is off by default: an administrator opens it in Admin › Instance (not in the
  // invitations tab, where those settings used to be buried).
  await page.goto('/admin?tab=system')
  const bridgeSwitch = page.getByRole('switch', { name: 'Allow the Garmin bridge' })
  await expect(bridgeSwitch).toHaveAttribute('aria-checked', 'false')
  await bridgeSwitch.click()
  await expect(bridgeSwitch).toHaveAttribute('aria-checked', 'true')

  await page.goto('/settings?tab=integrations')
  // The warning is the first thing read, before any field exists.
  await expect(page.getByText('Workout push (experimental)')).toBeVisible()
  await expect(page.getByText(/unofficial, unstable/)).toBeVisible()
  const card = page.getByRole('group', { name: 'Workout push (experimental)' })
  await card.getByRole('button', { name: 'Enable the bridge' }).click()
  await expect(page.getByText('Before you continue')).toBeVisible()
  await expect(page.getByText(/Garmin may suspend an account/)).toBeVisible()

  await card.getByLabel('Garmin e-mail').fill('mfa@example.com')
  await card.getByLabel('Garmin password').fill('motdepasse')
  await card.getByRole('button', { name: 'Sign in' }).click()
  // The code step appears in the same card: no external redirect, no helper to install.
  await expect(card.getByText('Code required')).toBeVisible()
  await card.getByLabel('Verification code').fill('123456')
  await card.getByRole('button', { name: 'Verify' }).click()
  await expect(card.getByText('Connected', { exact: true })).toBeVisible()

  // The planning now offers the bulk send, and reports each session.
  await page.goto('/plan')
  await page.getByRole('button', { name: 'Send today’s sessions' }).click()
  const dialog = page.getByRole('dialog', { name: 'Send today’s sessions' })
  // The native <dialog> only wraps a position:fixed panel: assert its open state, then the content.
  await expect(dialog).toHaveAttribute('open', '')
  await expect(dialog.getByText(/sessions of .* that have steps/)).toBeVisible()
  await dialog.getByRole('button', { name: 'Send', exact: true }).click()
  // The outcome is a toast (the dialog keeps the per-session list); both branches are valid.
  await expect(page.getByText(/Send complete|No session to send/).first()).toBeVisible()

  // Disconnecting puts the card back to its warning state.
  await page.goto('/settings?tab=integrations')
  await card.getByRole('button', { name: 'Disconnect' }).click()
  await expect(card.getByText('Disconnected', { exact: true })).toBeVisible()

  assertNoConsoleErrors(errors)
})

test('says a wrong password is wrong, in the card and not only in a toast', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/admin?tab=system')
  const bridgeSwitch = page.getByRole('switch', { name: 'Allow the Garmin bridge' })
  await bridgeSwitch.click()
  await expect(bridgeSwitch).toHaveAttribute('aria-checked', 'true')

  await page.goto('/settings?tab=integrations')
  const card = page.getByRole('group', { name: 'Workout push (experimental)' })
  await card.getByRole('button', { name: 'Enable the bridge' }).click()
  await card.getByLabel('Garmin e-mail').fill('antoine@example.com')
  // The demo only accepts « motdepasse »: anything else is Garmin's refusal.
  await card.getByLabel('Garmin password').fill('faux')
  await card.getByRole('button', { name: 'Sign in' }).click()

  // The refusal lands in the card, where the field is; the password is dropped at once.
  await expect(card.getByRole('alert').last()).toContainText('Garmin refused this e-mail or password.')
  await expect(card.getByLabel('Garmin password')).toHaveValue('')
  await expect(card.getByText('Disconnected', { exact: true })).toBeVisible()

  assertNoConsoleErrors(errors)
})

test('reconnects an expired session from the notification centre', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  // Open the bridge, then age the session: the mock database is the server's state in this build,
  // so patching it is how a real 401 on the API would look.
  await page.goto('/admin?tab=system')
  const bridgeSwitch = page.getByRole('switch', { name: 'Allow the Garmin bridge' })
  await bridgeSwitch.click()
  await expect(bridgeSwitch).toHaveAttribute('aria-checked', 'true')
  await page.goto('/settings?tab=integrations')
  await page.evaluate(() => {
    const raw = window.sessionStorage.getItem('stride.mock.v2')
    if (!raw) return
    const db = JSON.parse(raw) as { data?: Record<string, { garminBridge?: unknown }> }
    const me = db.data?.['1']
    if (me) me.garminBridge = { state: 'expired' }
    window.sessionStorage.setItem('stride.mock.v2', JSON.stringify(db))
  })

  // The notification centre links to the card that can sign in again…
  await page.goto('/settings#integrations')
  const card = page.getByRole('group', { name: 'Workout push (experimental)' })
  await expect(card.getByText('Expired', { exact: true })).toBeVisible()
  // …and the flow opens there, with the reason, without replaying the consent.
  await expect(page.getByRole('dialog')).toHaveAttribute('open', '')
  await expect(page.getByLabel('Garmin e-mail')).toBeVisible()
  await expect(card.getByText(/The Garmin session expired/).first()).toBeVisible()
  await expect(page.getByText('Before you continue')).toHaveCount(0)

  assertNoConsoleErrors(errors)
})

test('the FIT fallback no longer mentions the USB cable', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/plan')
  // Open the first session with a structure: the watch section states the web import checklist.
  await page.getByText('38 min continu').first().click()
  await expect(page.getByText('Without the bridge, the .FIT stays the safe path')).toBeVisible()
  await expect(page.getByText(/Garmin Connect Web › Training › Import/)).toBeVisible()
  const link = page.getByRole('link', { name: 'Open Garmin Connect' })
  await expect(link).toHaveAttribute('href', 'https://connect.garmin.com/modern/workouts')
  await expect(link).toHaveAttribute('target', '_blank')
  await expect(link).toHaveAttribute('rel', /noopener/)
  // The old cable instructions are gone from the interface.
  await expect(page.getByText(/GARMIN\/NewFiles/)).toHaveCount(0)

  assertNoConsoleErrors(errors)
})
