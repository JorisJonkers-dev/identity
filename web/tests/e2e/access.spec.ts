import { expect, test } from '@playwright/test'
import { addMember, enrolTOTP, expectAccessible, makeGroup, promote, signedUp, toAdmin } from './helpers'

test('a promoted Admin manages Groups', async ({ page, browser }, info) => {
  const email = await signedUp(page, `admin-${info.project.name}`)
  const group = `Family ${info.project.name} ${String(Date.now())}`

  await test.step('an Admin with no two-factor is sent to set it up before the Admin pages open', async () => {
    await expect(page.getByRole('group', { name: 'View' })).toHaveCount(0)
    expect(promote(email)).toBe(`${email} is now an Admin.\n`)
    await page.reload()
    await toAdmin(page)
    await expect(page).toHaveURL('/security/two-factor?for=admin')
    await expect(page.getByRole('status')).toContainText('Admin pages need two-factor.')
    await expectAccessible(page)
  })

  await test.step('with an authenticator app the Groups page opens', async () => {
    await enrolTOTP(page)
    await page.goto('/admin/groups')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Groups')
    await expectAccessible(page)
  })

  await test.step('an Admin makes a Group and adds someone to it, and the page stays as wide as a phone', async () => {
    const other = await browser.newContext({ ...info.project.use })
    const member = await signedUp(await other.newPage(), `member-${info.project.name}`)
    await other.close()
    await makeGroup(page, group)
    await addMember(page, member)
    await page.getByRole('button', { name: /^Members/ }).click()
    await expectAccessible(page)
    // A long name or email in a menu once pushed the page wider than a phone.
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })

  await test.step('an Admin deletes the Group', async () => {
    await page.getByRole('button', { name: 'Delete' }).click()
    await page.getByRole('button', { name: 'Delete Group' }).click()
    await expect(page).toHaveURL('/admin/groups')
    await expect(page.getByRole('navigation', { name: 'Groups', exact: true }).getByRole('link', { name: group })).toHaveCount(0)
  })
})

test('an Admin signs a User out on every device', async ({ page, browser }, info) => {
  const other = await browser.newContext({ ...info.project.use })
  const theirs = await other.newPage()
  const email = await signedUp(theirs, `signed-out-${info.project.name}`)

  await test.step("an Admin signs the User out from the User's page", async () => {
    promote(await signedUp(page, `admin-out-${info.project.name}`))
    await enrolTOTP(page)
    await page.goto('/admin/people')
    await page.getByLabel('Find a person').fill(email)
    await page.locator('a.who').filter({ hasText: email }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Grace Hopper')
    await page.locator('main').getByRole('button', { name: 'Sign out everywhere' }).click()
    await expect(page.getByText('Signed out on every device')).toBeVisible()
    await expectAccessible(page)
  })

  await test.step("the User's own browser is signed out", async () => {
    await theirs.goto('/account')
    await expect(theirs).toHaveURL('/signin')
    await other.close()
  })
})
