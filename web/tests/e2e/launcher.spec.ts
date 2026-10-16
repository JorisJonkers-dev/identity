import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, givesRole, promote, signedUp } from './helpers'

test('a new User is told they have no apps yet', async ({ page }, info) => {
  await signedUp(page, `launcher-empty-${info.project.name}`)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Your apps')
  await expect(page.getByText("You don't have access to any apps yet. Ask an Admin to add you to a Group.")).toBeVisible()
})

test('the Launcher shows each app a Group gives the User, with its icon from identity', async ({ page }, info) => {
  const email = await signedUp(page, `launcher-${info.project.name}`)

  await test.step("an Admin gives the User's Group a role on Board and on Notes", async () => {
    promote(email)
    await enrolTOTP(page)
    await page.goto('/access')
    const group = `Launcher ${info.project.name} ${String(Date.now())}`
    await page.getByLabel('Group name').fill(group)
    await page.getByRole('button', { name: 'Create Group' }).click()
    const article = page.getByRole('article', { name: group })
    await article.getByLabel('Add a member').selectOption({ label: `Grace Hopper (${email})` })
    await article.getByRole('button', { name: 'Add' }).click()
    for (const app of ['Board', 'Notes']) {
      await givesRole(page, page.getByRole('checkbox', { name: `${group}: user on ${app}` }))
    }
  })

  await test.step("Board's tile opens Board and says what the User can do there", async () => {
    await page.goto('/')
    const board = page.getByRole('listitem').filter({ hasText: 'Board' })
    await expect(board.getByRole('link')).toHaveAttribute('href', 'http://board.localhost:18604/today')
    await expect(board).toContainText('The family board')
    await expect(board).toContainText('Your access: user')
  })

  await test.step("Board's icon comes from identity's own origin, not the Client's host", async () => {
    const icon = page.getByRole('listitem').filter({ hasText: 'Board' }).locator('img')
    await expect(icon).toHaveAttribute('src', /^\/icons\/board\?v=/)
    await expect.poll(() => icon.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1)
  })

  await test.step("Notes' icon is missing on its host, so its tile shows the label's first letter", async () => {
    const notes = page.getByRole('listitem').filter({ hasText: 'Notes' })
    await expect(notes.locator('.letter')).toHaveText('N')
    await expect(notes.locator('img')).toHaveCount(0)
    await expectAccessible(page)
  })
})
