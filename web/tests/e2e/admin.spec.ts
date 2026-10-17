import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, promote, signedUp } from './helpers'

test('an Admin reads the dashboard and the audit log', async ({ page, browser }, info) => {
  const group = `Audited ${info.project.name} ${String(Date.now())}`

  await test.step('an Admin opens the dashboard from the Launcher', async () => {
    promote(await signedUp(page, `dashboard-${info.project.name}`))
    await enrolTOTP(page)
    await page.goto('/')
    await page.getByRole('link', { name: 'Dashboard' }).click()
    await expect(page).toHaveURL('/admin')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Estate sign-in')
    await expect(page.getByRole('list', { name: 'Counts' }).getByRole('listitem')).toHaveCount(5)
    await expect(page.getByRole('img', { name: /sign-ins in the last 24 hours/ })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Open in Grafana ↗' })).toHaveAttribute('href', 'https://grafana.example.org/d/identity')
    await expectAccessible(page)
    // The sidebar and the counts fit a phone's width.
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })

  await test.step('someone who just signed up waits in the queue, one step from a Group', async () => {
    const other = await browser.newContext({ ...info.project.use })
    const email = await signedUp(await other.newPage(), `queued-${info.project.name}`)
    await other.close()
    await page.reload()
    const queue = page.getByRole('list', { name: 'Unassigned queue new sign-ups' })
    await expect(queue.getByText(email)).toBeVisible()
    await queue.getByRole('link', { name: 'Add Grace Hopper to a Group' }).first().click()
    await expect(page).toHaveURL('/access#waiting-title')
  })

  await test.step('a change to access shows in the audit log, narrowed to access', async () => {
    await page.getByLabel('Group name').fill(group)
    await page.getByRole('button', { name: 'Create Group' }).click()
    await expect(page.getByRole('article', { name: group })).toBeVisible()
    await page.goto('/admin/audit')
    await page.getByRole('button', { name: 'access', exact: true }).click()
    await expect(page).toHaveURL('/admin/audit?kind=access')
    await expect(page.getByRole('list', { name: 'Audit Events, newest first' })).toContainText(`made the Group ${group}`)
    await expectAccessible(page)
  })
})
