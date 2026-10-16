import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, promote, signedUp } from './helpers'

test('a promoted Admin manages Groups on the access page', async ({ page }, info) => {
  const email = await signedUp(page, `admin-${info.project.name}`)
  await expect(page.getByRole('link', { name: 'Manage access' })).toHaveCount(0)

  expect(promote(email)).toBe(`${email} is now an Admin.\n`)
  await page.reload()

  // An Admin with no Second Factor is sent to add one before the access page opens.
  await page.getByRole('link', { name: 'Manage access' }).click()
  await expect(page).toHaveURL('/security?for=admin')
  await expectAccessible(page)
  await enrolTOTP(page)
  await page.goto('/profile')
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

test('an Admin signs a User out on every device', async ({ page, browser }, info) => {
  const other = await browser.newContext({ ...info.project.use })
  const theirs = await other.newPage()
  const email = await signedUp(theirs, `signed-out-${info.project.name}`)

  promote(await signedUp(page, `admin-out-${info.project.name}`))
  await enrolTOTP(page)
  await page.goto('/access')
  const row = page.getByRole('listitem', { name: `Grace Hopper (${email})` })
  await row.getByRole('button', { name: 'Sign out on every device' }).click()
  await row.getByRole('button', { name: 'Yes, sign out' }).click()
  await expect(row.getByRole('status')).toHaveText('Grace Hopper is signed out on every device.')
  await expectAccessible(page)

  await theirs.goto('/profile')
  await expect(theirs.getByRole('alert').getByRole('link', { name: 'Sign in' })).toBeVisible()
  await other.close()
})
