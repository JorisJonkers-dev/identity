import AxeBuilder from '@axe-core/playwright'
import { expect, type Page, test } from '@playwright/test'

/** Fails on any WCAG 2.1 A or AA violation axe finds on the page as it is now. */
async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

test('a note added in the browser is stored and listed', async ({ page }, info) => {
  // A Content-Security-Policy violation surfaces only as a console error, so any error fails.
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  const text = `Buy milk ${info.project.name} ${String(Date.now())}`
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible()
  await expectAccessible(page)

  await page.getByLabel('New note').fill(text)
  await page.getByRole('button', { name: 'Add note' }).click()
  await expect(page.getByLabel('New note')).toHaveValue('')
  await expect(page.getByRole('list', { name: 'Notes' })).toContainText(text)
  await expectAccessible(page)

  await page.reload()
  await expect(page.getByRole('list', { name: 'Notes' })).toContainText(text)
  expect(errors).toEqual([])
})

test('text the domain refuses is never sent', async ({ page }) => {
  await page.goto('/')
  const add = page.getByRole('button', { name: 'Add note' })
  await expect(add).toBeDisabled()
  await page.getByLabel('New note').fill('   ')
  await expect(page.getByText('Write something first.')).toBeVisible()
  await expect(add).toBeDisabled()
})

test('a client route is served the app, and an unknown one goes home', async ({ page }) => {
  await page.goto('/no/such/page')
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible()
})
