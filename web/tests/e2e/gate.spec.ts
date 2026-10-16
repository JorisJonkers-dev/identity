import { request as httpRequest, createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import { expect, test } from '@playwright/test'
import { enrolTOTP, promote, signedUp } from './helpers'

const identity = { host: 'localhost', port: 18573 }

/** Each project's gated host, a Client of its own in tests/e2e/clients.yaml. */
function gatedPort(project: string): number {
  return project === 'phone' ? 18601 : 18600
}

function label(project: string): string {
  return project === 'phone' ? 'Phone stats' : 'Stats'
}

/** Sends a request on to identity as Traefik would, keeping its Host, and relays the answer. */
function toIdentity(path: string, headers: IncomingMessage['headers'], res: ServerResponse, onOK?: (h: IncomingMessage['headers']) => void) {
  const out = httpRequest({ ...identity, path, method: 'GET', headers }, (answer) => {
    if (answer.statusCode === 200 && onOK) {
      answer.resume()
      onOK(answer.headers)
      return
    }
    res.writeHead(answer.statusCode ?? 502, answer.headers)
    answer.pipe(res)
  })
  out.end()
}

/** A stand-in for Traefik on the gated host: /.auth/ goes to identity, everything else through forwardAuth. */
function traefik(port: number): Server {
  return createServer((req, res) => {
    const url = req.url ?? '/'
    if (url.startsWith('/.auth/')) {
      toIdentity(url, req.headers, res)
      return
    }
    const check = { cookie: req.headers.cookie ?? '', 'x-forwarded-host': req.headers.host ?? '', 'x-forwarded-uri': url, 'x-forwarded-proto': 'http' }
    toIdentity('/gate/check', check, res, (h) => {
      res.writeHead(200, { 'content-type': 'text/plain' })
      res.end(`Hello ${String(h['x-auth-request-email'])} with ${String(h['x-auth-request-roles'])}`)
    })
  }).listen(port)
}

let proxy: Server
// Playwright reads the fixtures a hook asks for from its first argument; this hook asks for none.
test.beforeAll(({}, info) => {
  proxy = traefik(gatedPort(info.project.name))
})
test.afterAll(() => {
  proxy.close()
})

test('a gated app signs in through the Gate, and only for a User with a role on it', async ({ page }, info) => {
  const app = `http://stats.localhost:${String(gatedPort(info.project.name))}`
  const name = label(info.project.name)
  const email = await signedUp(page, `gate-${info.project.name}`)

  await test.step('a visitor without the host cookie gets the Gate page with one button', async () => {
    await page.goto(`${app}/week/42`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(`Sign in to ${name}`)
  })

  await test.step('a signed-in User with no role on the app is told so', async () => {
    await page.getByRole('link', { name: 'Sign in with jorisjonkers.dev' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(`You don't have access to ${name}`)
  })

  await test.step("an Admin gives the User's Group the role", async () => {
    promote(email)
    await enrolTOTP(page)
    await page.goto('/access')
    const group = `Stats readers ${info.project.name} ${String(Date.now())}`
    await page.getByLabel('Group name').fill(group)
    await page.getByRole('button', { name: 'Create Group' }).click()
    const article = page.getByRole('article', { name: group })
    await article.getByLabel('Add a member').selectOption({ label: `Grace Hopper (${email})` })
    await article.getByRole('button', { name: 'Add' }).click()
    await page.getByRole('checkbox', { name: `${group}: user on ${name}` }).check()
    await expect(page.getByRole('checkbox', { name: `${group}: user on ${name}` })).toBeChecked()
  })

  await test.step('signing in through the Gate returns to the page asked for', async () => {
    await page.goto(`${app}/week/42`)
    await page.getByRole('link', { name: 'Sign in with jorisjonkers.dev' }).click()
    await expect(page).toHaveURL(`${app}/week/42`)
    await expect(page.locator('body')).toHaveText(`Hello ${email} with user`)
  })

  await test.step("the cookie is the host's alone, and the next visit needs no sign-in", async () => {
    const cookies = await page.context().cookies(app)
    const gate = cookies.find((c) => c.name === '__Host-identity_gate')
    expect(gate?.httpOnly).toBe(true)
    expect(gate?.domain).toBe('stats.localhost')
    expect((await page.context().cookies('http://localhost:18573')).some((c) => c.name === '__Host-identity_gate')).toBe(false)
    await page.goto(`${app}/`)
    await expect(page.locator('body')).toHaveText(`Hello ${email} with user`)
  })

  await test.step('signing out at identity reaches the gated host at once, not when its 30-second cache runs out', async () => {
    await page.goto('/profile')
    await page.getByRole('button', { name: 'Sign out' }).click()
    await expect(page).toHaveURL('/signin')
    await page.goto(`${app}/`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(`Sign in to ${name}`)
  })
})
