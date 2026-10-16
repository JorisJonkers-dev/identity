import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, promote, signedUp } from './helpers'

test('a promoted Admin manages Groups on the access page', async ({ page }, info) => {
  const email = await signedUp(page, `admin-${info.project.name}`)
  const group = `Family ${info.project.name} ${String(Date.now())}`

  await test.step('an Admin with no Second Factor is sent to add one before the access page opens', async () => {
    await expect(page.getByRole('link', { name: 'Manage access' })).toHaveCount(0)
    expect(promote(email)).toBe(`${email} is now an Admin.\n`)
    await page.reload()
    await page.getByRole('link', { name: 'Manage access' }).click()
    await expect(page).toHaveURL('/security?for=admin')
    await expectAccessible(page)
  })

  await test.step('with an authenticator app the access page opens', async () => {
    await enrolTOTP(page)
    await page.goto('/profile')
    await page.getByRole('link', { name: 'Manage access' }).click()
    await expect(page).toHaveURL('/access')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Access')
    await expectAccessible(page)
  })

  await test.step('an Admin creates a Group, and the page stays as wide as a phone', async () => {
    await page.getByLabel('Group name').fill(group)
    await page.getByRole('button', { name: 'Create Group' }).click()
    await expect(page.getByRole('article', { name: group })).toBeVisible()
    // A long name or email in a menu once pushed the page wider than a phone.
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })

  await test.step('an Admin deletes the Group', async () => {
    await page.getByRole('article', { name: group }).getByRole('button', { name: `Delete ${group}` }).click()
    await expect(page.getByRole('article', { name: group })).toHaveCount(0)
  })
})

test('an Admin signs a User out on every device', async ({ page, browser }, info) => {
  const other = await browser.newContext({ ...info.project.use })
  const theirs = await other.newPage()
  const email = await signedUp(theirs, `signed-out-${info.project.name}`)

  await test.step('an Admin signs the User out from the access page, after confirming', async () => {
    promote(await signedUp(page, `admin-out-${info.project.name}`))
    await enrolTOTP(page)
    await page.goto('/access')
    const row = page.getByRole('listitem', { name: `Grace Hopper (${email})` })
    await row.getByRole('button', { name: 'Sign out on every device' }).click()
    await row.getByRole('button', { name: 'Yes, sign out' }).click()
    await expect(row.getByRole('status')).toHaveText('Grace Hopper is signed out on every device.')
    await expectAccessible(page)
  })

  await test.step("the User's own browser is signed out", async () => {
    await theirs.goto('/profile')
    await expect(theirs.getByRole('alert').getByRole('link', { name: 'Sign in' })).toBeVisible()
    await other.close()
  })
})
