import { expect, type Page } from '@playwright/test'

/**
 * Collects console errors and unhandled page exceptions. The mock-mode app must boot and
 * navigate without a single one: there is no network, no third-party resource.
 */
export function trackConsoleErrors(page: Page): () => string[] {
  const errors: string[] = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text())
  })
  page.on('pageerror', (err) => {
    errors.push(`unhandled exception: ${err.message}`)
  })
  return () => errors
}

/** Fails the test with the collected console errors as the message. */
export function assertNoConsoleErrors(errors: () => string[]) {
  expect(errors(), `console errors: ${errors().join(' | ')}`).toEqual([])
}
