import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// 4.7.0 notification centre (§21, issue #142): the bell, the badge and the panel fed by the mock
// state (one unread alert, one unread directive, one read remark), then « Mark all as read ».

test('opens the notification centre, navigates an entry and marks everything read', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/')
  const bell = page.getByRole('button', { name: 'Notifications, 2 unread' })
  await expect(bell).toBeVisible()

  await bell.click()
  // The native <dialog> only wraps a position:fixed panel, so the element itself has no box:
  // assert its open state, then the content inside.
  const panel = page.getByRole('dialog', { name: 'Notifications' })
  await expect(panel).toHaveAttribute('open', '')
  await expect(panel.getByText('2 unread')).toBeVisible()
  // The alert keeps the analysis wording, the portal writes are localized from their code.
  await expect(panel.getByText('Cadence basse sur la dernière sortie')).toBeVisible()
  await expect(panel.getByText('Physio/coach instruction updated')).toBeVisible()
  await expect(panel.getByText('New remark on a session')).toBeVisible()
  // The history is grouped by day.
  await expect(panel.getByRole('region', { name: 'Today' })).toBeVisible()

  // A remark names its session: the entry opens it.
  await panel.getByText('New remark on a session').click()
  await expect(page).toHaveURL(/\/activities\/\d+/)
  // Opening an entry closes the panel and the read remark changed nothing in the count.
  await expect(panel).toBeHidden()
  await expect(page.getByRole('button', { name: 'Notifications, 2 unread' })).toBeVisible()

  // Mark all as read: the badge disappears, the history stays.
  await page.getByRole('button', { name: 'Notifications, 2 unread' }).click()
  await page.getByRole('button', { name: 'Mark all as read' }).click()
  await expect(page.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'Notifications' }).getByText('Cadence basse sur la dernière sortie')).toBeVisible()
  await page.keyboard.press('Escape')

  assertNoConsoleErrors(errors)
})

test('shows the retention settings in the admin invitations panel', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/admin?tab=invitations')
  const card = page.getByRole('heading', { name: 'Notification centre' })
  await expect(card).toBeVisible()
  const nums = page.locator('input[type="number"]')
  // Registration mode has no number field: the two retention inputs come first.
  await expect(nums.nth(0)).toHaveValue('10')
  await expect(nums.nth(1)).toHaveValue('50')

  await nums.nth(0).fill('30')
  const save = page.getByRole('button', { name: 'Save' }).first()
  await expect(save).toBeEnabled()
  await save.click()
  await expect(page.getByText('Retention saved')).toBeVisible()

  assertNoConsoleErrors(errors)
})
