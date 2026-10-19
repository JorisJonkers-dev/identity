import { expect, test } from '@playwright/test'
import { expectAccessible } from './helpers'

test('the home page names the service and shows the running version', async ({ page }) => {
  // A Content-Security-Policy violation surfaces only as a console error. The signed-out
  // Launcher's 401 is one too, and is expected.
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error' && !message.text().includes('401 (Unauthorized)')) errors.push(message.text())
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'jorisjonkers.dev' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/signin')
  await expect(page.getByRole('link', { name: 'Sign up' })).toHaveAttribute('href', '/signup')
  await expect(page.getByText(/^Version /)).toBeVisible()
  await expectAccessible(page)
  expect(errors).toEqual([])
})

test('a client route is served the app, and an unknown one goes home', async ({ page }) => {
  await page.goto('/no/such/page')
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { name: 'jorisjonkers.dev' })).toBeVisible()
})
