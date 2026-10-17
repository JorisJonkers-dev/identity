import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, givesRole, promote, signedUp } from './helpers'

test('a User makes a Personal Token for an app, uses it through the Gate, and revokes it', async ({ page }, info) => {
  const host = info.project.name === 'phone' ? 'stats.localhost:18601' : 'stats.localhost:18600'
  const app = info.project.name === 'phone' ? 'Phone stats' : 'Stats'
  const name = `script-${info.project.name}-${String(Date.now())}`
  let secret = ''

  await test.step("an Admin gives the User's Group a role on the app", async () => {
    const email = await signedUp(page, `tokens-${info.project.name}`)
    promote(email)
    await enrolTOTP(page)
    await page.goto('/access')
    const group = `Tokens ${info.project.name} ${String(Date.now())}`
    await page.getByLabel('Group name').fill(group)
    await page.getByRole('button', { name: 'Create Group' }).click()
    const article = page.getByRole('article', { name: group })
    await article.getByLabel('Add a member').selectOption({ label: `Grace Hopper (${email})` })
    await article.getByRole('button', { name: 'Add' }).click()
    await givesRole(page, page.getByRole('checkbox', { name: `${group}: user on ${app}` }))
  })

  await test.step('the User makes a token from the Launcher and sees its secret once', async () => {
    await page.goto('/')
    await page.getByRole('link', { name: 'Tokens' }).click()
    await expect(page).toHaveURL('/profile/tokens')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Tokens')
    await page.getByRole('button', { name: 'New token' }).click()
    const form = page.getByRole('form', { name: 'New token' })
    await form.getByLabel('Name').fill(name)
    await form.getByLabel('App').selectOption({ label: app })
    await form.getByLabel('Lasts').selectOption({ label: '30 days' })
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(page.getByText("Copy your token now. You won't see it again.")).toBeVisible()
    secret = (await page.locator('[data-secret]').textContent()) ?? ''
    expect(secret).toMatch(/^idpat_/)
    await expectAccessible(page)
    await page.getByRole('button', { name: 'Done' }).click()
    await expect(page.getByRole('listitem', { name })).toContainText(`${app} · user ·`)
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
    const row = page.getByRole('listitem', { name })
    await row.getByRole('button', { name: `Revoke ${name}` }).click()
    await row.getByRole('button', { name: 'Yes, revoke' }).click()
    await expect(row).toContainText('revoked')
    // The Gate kept the token's answer from its use above; revoking it drops that at once.
    const check = await page.request.get('/gate/check', {
      headers: { 'X-Forwarded-Host': host, 'X-Forwarded-Uri': '/', Authorization: `Bearer ${secret}` },
    })
    expect(check.status()).toBe(401)
  })
})
