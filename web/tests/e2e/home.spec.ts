import AxeBuilder from '@axe-core/playwright'
import { expect, type Page, test } from '@playwright/test'

/** Fails on any WCAG 2.1 A or AA violation axe finds on the page as it is now. */
async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

test('the home page names the service and shows the running version', async ({ page }) => {
  // A Content-Security-Policy violation surfaces only as a console error. The signed-out
  // Launcher's 401 is one too, and is expected.
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error' && !message.text().includes('401 (Unauthorized)')) errors.push(message.text())
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'jorisjonkers.dev' })).toBeVisible()
  await expect(page.getByText(/^Version /)).toBeVisible()
  await expectAccessible(page)
  expect(errors).toEqual([])
})

test('a client route is served the app, and an unknown one goes home', async ({ page }) => {
  await page.goto('/no/such/page')
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { name: 'jorisjonkers.dev' })).toBeVisible()
})
