import { expect, test } from '@playwright/test'
import { addMember, enrolTOTP, expectAccessible, givesRole, makeGroup, promote, signedUp } from './helpers'

test('a User makes a Personal Token for an app, uses it through the Gate, and revokes it', async ({ page }, info) => {
  const host = info.project.name === 'phone' ? 'stats.localhost:18601' : 'stats.localhost:18600'
  const app = info.project.name === 'phone' ? 'Phone stats' : 'Stats'
  const name = `script-${info.project.name}-${String(Date.now())}`
  let secret = ''

  await test.step("an Admin gives the User's Group a role on the app", async () => {
    const email = await signedUp(page, `tokens-${info.project.name}`)
    promote(email)
    await enrolTOTP(page)
    const group = `Tokens ${info.project.name} ${String(Date.now())}`
    await makeGroup(page, group)
    await addMember(page, email)
    await givesRole(page, group, app, 'user')
  })

  await test.step('the User makes a token and sees its secret once', async () => {
    await page.goto('/tokens')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Tokens')
    await page.getByRole('button', { name: 'New token' }).click()
    const dialog = page.getByRole('dialog', { name: 'New token' })
    await dialog.getByLabel('Name', { exact: true }).fill(name)
    await dialog.getByRole('button', { name: 'App' }).click()
    await dialog.getByRole('option', { name: app, exact: true }).click()
    await dialog.getByRole('button', { name: '30 days' }).click()
    await dialog.getByRole('button', { name: 'Create token' }).click()
    const shown = page.getByRole('dialog', { name: 'Your new token' })
    await expect(shown.getByText('Shown once.')).toBeVisible()
    secret = (await shown.locator('code').textContent()) ?? ''
    expect(secret).toMatch(/^idpat_/)
    await expectAccessible(page)
    await shown.getByRole('button', { name: 'Done' }).click()
    const row = page.locator('.lt-r').filter({ hasText: name })
    await expect(row.locator('.app-c')).toContainText(app)
    await expect(row.locator('.role')).toHaveText(['user'])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })

  await test.step('the Gate lets a script carrying the token through', async () => {
    const check = await page.request.get('/gate/check', {
      headers: { 'X-Forwarded-Host': host, 'X-Forwarded-Uri': '/', Authorization: `Bearer ${secret}` },
    })
    expect(check.status()).toBe(200)
    expect(check.headers()['x-auth-request-roles']).toBe('user')
  })

  await test.step('once revoked, the token opens nothing', async () => {
    const row = page.locator('.lt-r').filter({ hasText: name })
    await row.getByRole('button', { name: `Revoke ${name}` }).click()
    await row.locator('.lt-confirm').getByRole('button', { name: 'Revoke' }).click()
    await expect(row.locator('.state')).toHaveText('Revoked')
    // The Gate kept the token's answer from its use above; revoking it drops that at once.
    const check = await page.request.get('/gate/check', {
      headers: { 'X-Forwarded-Host': host, 'X-Forwarded-Uri': '/', Authorization: `Bearer ${secret}` },
    })
    expect(check.status()).toBe(401)
  })
})
