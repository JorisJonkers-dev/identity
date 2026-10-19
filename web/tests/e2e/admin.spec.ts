import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, makeGroup, promote, signedUp, toAdmin } from './helpers'

test('an Admin reads the dashboard and the audit log', async ({ page, browser }, info) => {
  const group = `Audited ${info.project.name} ${String(Date.now())}`

  await test.step('an Admin switches to the Admin view and finds the dashboard', async () => {
    promote(await signedUp(page, `dashboard-${info.project.name}`))
    await enrolTOTP(page)
    await page.goto('/')
    await toAdmin(page)
    await expect(page).toHaveURL('/admin')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Dashboard')
    await expect(page.locator('.stat')).toHaveCount(5)
    await expect(page.getByRole('img', { name: /^Successful sign-ins per hour over the last 24 hours/ })).toBeVisible()
    await expect(page.getByRole('img', { name: /^Failed sign-ins per hour over the last 24 hours/ })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Grafana' })).toHaveAttribute('href', 'https://grafana.example.org/d/identity')
    await expectAccessible(page)
    // The bar and the counts fit a phone's width.
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })

  await test.step('someone who just signed up waits on the Unassigned page, one click from a Group', async () => {
    const other = await browser.newContext({ ...info.project.use })
    const email = await signedUp(await other.newPage(), `queued-${info.project.name}`)
    await other.close()
    await page.goto('/admin/unassigned')
    const card = page.locator('.un-card').filter({ hasText: email })
    await expect(card).toBeVisible()
    await expectAccessible(page)
  })

  await test.step('a change to access shows in the audit log, narrowed to access', async () => {
    await makeGroup(page, group)
    await page.goto('/admin/audit')
    await page.getByRole('button', { name: 'Access', exact: true }).click()
    await expect(page).toHaveURL('/admin/audit?kind=access')
    await expect(page.locator('.audit-list')).toContainText(group)
    await expectAccessible(page)
  })
})
