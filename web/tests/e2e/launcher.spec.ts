import { expect, test } from '@playwright/test'
import { addMember, enrolTOTP, expectAccessible, givesRole, makeGroup, promote, signedUp } from './helpers'

test('a new User is told they have no apps yet', async ({ page }, info) => {
  await signedUp(page, `launcher-empty-${info.project.name}`)
  await expect(page.getByText('No apps yet. An Admin adds you to a Group.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Contact an Admin' })).toHaveAttribute('href', /^mailto:/)
  await expectAccessible(page)
})

test('the Launcher shows each app a Group gives the User, with its icon from identity', async ({ page }, info) => {
  const email = await signedUp(page, `launcher-${info.project.name}`)

  await test.step("an Admin gives the User's Group a role on Board and on Notes", async () => {
    promote(email)
    await enrolTOTP(page)
    const group = `Launcher ${info.project.name} ${String(Date.now())}`
    await makeGroup(page, group)
    await addMember(page, email)
    for (const app of ['Board', 'Notes']) await givesRole(page, group, app, 'user')
  })

  await test.step("Board's card opens Board and shows the User's role there", async () => {
    await page.goto('/')
    const board = page.locator('a.app').filter({ hasText: 'Board' })
    await expect(board).toHaveAttribute('href', 'http://board.localhost:18604/today')
    await expect(board.locator('.role')).toHaveText(['user'])
  })

  await test.step("Board's icon comes from identity's own origin, not the Client's host", async () => {
    const icon = page.locator('a.app').filter({ hasText: 'Board' }).locator('img')
    await expect(icon).toHaveAttribute('src', /^\/icons\/board\?v=/)
    await expect.poll(() => icon.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1)
  })

  await test.step("Notes' icon is missing on its host, so its card shows the label's first letter", async () => {
    const notes = page.locator('a.app').filter({ hasText: 'Notes' })
    await expect(notes.locator('.mono-l')).toHaveText('N')
    await expect(notes.locator('img')).toHaveCount(0)
    await expectAccessible(page)
  })

  await test.step('the search finds an app by name', async () => {
    await page.keyboard.press('/')
    await expect(page.getByLabel('Find an app')).toBeFocused()
    await page.keyboard.type('boa')
    await expect(page.locator('a.app')).toHaveCount(1)
  })
})
