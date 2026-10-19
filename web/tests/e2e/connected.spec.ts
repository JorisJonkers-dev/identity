import { createHash } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { addMember, enrolTOTP, expectAccessible, givesRole, makeGroup, promote, signedUp } from './helpers'

const callback = 'http://127.0.0.1:33418/callback'
const verifier = 'an-e2e-tool-verifier-long-enough-for-pkce-0123456789'

test('a User connects a tool to an app, the tool reaches it through the Gate, and the User disconnects it', async ({ page }, info) => {
  const phone = info.project.name === 'phone'
  const host = phone ? 'stats.localhost:18601' : 'stats.localhost:18600'
  const clientId = phone ? 'stats-phone' : 'stats'
  const app = phone ? 'Phone stats' : 'Stats'
  const tool = `E2E tool ${info.project.name}`
  let toolId = ''
  let access = ''

  await test.step('an Admin gives the User a role on the app and lets tools connect to it', async () => {
    const email = await signedUp(page, `connected-${info.project.name}`)
    promote(email)
    await enrolTOTP(page)
    const group = `Connected ${info.project.name} ${String(Date.now())}`
    await makeGroup(page, group)
    await addMember(page, email)
    await givesRole(page, group, app, 'user')
    await page.goto(`/admin/apps/${clientId}`)
    const toggle = page.getByLabel(`Let tools use ${app} as the people who connect them`)
    if (!(await toggle.isChecked())) {
      const kept = page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().endsWith('/mcp') && r.status() === 204)
      await toggle.click({ force: true })
      await kept
    }
    await expect(toggle).toBeChecked()
  })

  await test.step('the tool registers itself', async () => {
    const res = await page.request.post('/oidc/register', { data: { client_name: tool, redirect_uris: [callback] } })
    expect(res.status()).toBe(201)
    toolId = ((await res.json()) as { client_id: string }).client_id
  })

  await test.step('the User agrees on the consent page, and the tool gets its tokens', async () => {
    const challenge = createHash('sha256').update(verifier).digest('base64url')
    const query = new URLSearchParams({
      client_id: toolId, redirect_uri: callback, response_type: 'code', state: 'st',
      code_challenge: challenge, code_challenge_method: 'S256', resource: `https://${host}/mcp`,
    })
    await page.goto(`/oidc/authorize?${query.toString()}`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(`Connect ${tool} to ${app}?`)
    await expectAccessible(page)
    // Nothing listens where the tool would, so the browser's last hop is read as it leaves.
    const [back] = await Promise.all([
      page.waitForRequest((r) => r.url().startsWith(callback)),
      page.getByRole('button', { name: 'Connect' }).click(),
    ])
    const code = new URL(back.url()).searchParams.get('code') ?? ''
    const token = await page.request.post('/oidc/token', {
      form: { grant_type: 'authorization_code', code, client_id: toolId, redirect_uri: callback, code_verifier: verifier },
    })
    expect(token.status()).toBe(200)
    access = ((await token.json()) as { access_token: string }).access_token
    expect(access).toMatch(/^idapp_/)
  })

  await test.step('the Gate lets the tool through as the User', async () => {
    const check = await page.request.get('/gate/check', { headers: { 'X-Forwarded-Host': host, 'X-Forwarded-Uri': '/mcp', Authorization: `Bearer ${access}` } })
    expect(check.status()).toBe(200)
    expect(check.headers()['x-auth-request-roles']).toBe('user')
  })

  await test.step('the User disconnects the tool under Security, and it opens nothing', async () => {
    // The first tab is still failing to reach the tool; the same browser opens a tab of its own.
    const security = await page.context().newPage()
    await security.goto('/security/connected-apps')
    const row = security.locator('.group .row').filter({ hasText: tool })
    await expect(row).toContainText(`Uses ${app} as you`)
    await expectAccessible(security)
    await row.getByRole('button', { name: 'Disconnect' }).click()
    await expect(row).toHaveCount(0)
    const check = await page.request.get('/gate/check', { headers: { 'X-Forwarded-Host': host, 'X-Forwarded-Uri': '/mcp', Authorization: `Bearer ${access}` } })
    expect(check.status()).toBe(401)
    expect(check.headers()['www-authenticate']).toContain(`resource_metadata="`)
  })
})
