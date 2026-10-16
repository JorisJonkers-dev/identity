import { expect, test } from '@playwright/test'
import { expectAccessible, promote, signedUp } from './helpers'

test('a promoted Admin manages Groups on the access page', async ({ page }, info) => {
  const email = await signedUp(page, `admin-${info.project.name}`)
  await expect(page.getByRole('link', { name: 'Manage access' })).toHaveCount(0)

  expect(promote(email)).toBe(`${email} is now an Admin.\n`)
  await page.reload()
  await page.getByRole('link', { name: 'Manage access' }).click()
  await expect(page).toHaveURL('/access')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Access')
  await expectAccessible(page)

  const group = `Family ${info.project.name} ${String(Date.now())}`
  await page.getByLabel('Group name').fill(group)
  await page.getByRole('button', { name: 'Create Group' }).click()
  await expect(page.getByRole('article', { name: group })).toBeVisible()
  // A long name or email in a menu once pushed the page wider than a phone.
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.getByRole('article', { name: group }).getByRole('button', { name: `Delete ${group}` }).click()
  await expect(page.getByRole('article', { name: group })).toHaveCount(0)
})
