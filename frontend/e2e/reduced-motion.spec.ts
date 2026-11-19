import { expect, test } from '@playwright/test'

// The 4.2.1 regression (issue #91): the global `prefers-reduced-motion: reduce` reset in
// src/index.css capped every animation at one 0.001 ms iteration, freezing the loaders.
// The 4.2.2 fix keeps `.animate-spin` turning, just slower (1.5 s, infinite).
//
// `?mock=syncrunning`: the sync never ends → a persistent `.animate-spin` in the header.
test('loaders keep spinning under prefers-reduced-motion: reduce', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })

  // Control assertion: the emulation must really be active.
  expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)

  await page.goto('/?mock=syncrunning')
  await expect(page).toHaveTitle('Dashboard · Stride')

  // The sync indicator in the header never settles: a .animate-spin stays on screen.
  const spinner = page.locator('.animate-spin').first()
  await expect(spinner).toBeVisible()

  const style = await spinner.evaluate((el) => {
    const css = getComputedStyle(el)
    return { name: css.animationName, duration: css.animationDuration, iterations: css.animationIterationCount }
  })

  // With the 4.2.2 exception the loader turns forever, at 1.5 s — not the 1 s of the
  // animation outside the media query, so this also proves the query is in effect.
  expect(style.name).not.toBe('none')
  expect(style.iterations).toBe('infinite')
  expect(style.duration).toBe('1.5s')
})
