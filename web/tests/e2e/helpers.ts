import { execFileSync } from 'node:child_process'
import { createHmac } from 'node:crypto'
import AxeBuilder from '@axe-core/playwright'
import { expect, type Page } from '@playwright/test'

const mailpit = 'http://localhost:28025'

// axe cannot measure contrast over a gradient and passes it silently, so the glow behind the page
// and the glass's sheen give way to their flat colours while it looks, and motion stops so nothing is
// caught mid-fade. The styles go on through the CSSOM, which the page's style-src 'self' allows.
const flat: [string, Record<string, string>][] = [
  ['*', { animation: 'none', transition: 'none' }],
  ['html, body', { 'background-image': 'none' }],
  [
    '.glass, .app, .bar, .from, .crumb, .pick-sel, .search-sm, .seg, .btn:not(.primary):not(.ghost):not(.danger-solid):not(.danger)',
    { 'background-image': 'none', 'background-color': 'var(--lg-top)' },
  ],
  ['.glass.strong', { 'background-color': 'var(--lg-strong)' }],
  ['.btn.primary', { 'background-image': 'none', 'background-color': 'var(--accent-solid)' }],
]

/** Fails on any WCAG 2.1 A or AA violation axe finds on the page as it is now. */
export async function expectAccessible(page: Page) {
  const saved = await page.evaluateHandle((rules) => {
    const before = new Map<HTMLElement, string>()
    for (const [selector, props] of rules) {
      for (const el of document.querySelectorAll<HTMLElement>(selector)) {
        if (!before.has(el)) before.set(el, el.style.cssText)
        for (const [name, value] of Object.entries(props)) el.style.setProperty(name, value, 'important')
      }
    }
    return before
  }, flat)
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  await saved.evaluate((before) => {
    for (const [el, css] of before) el.style.cssText = css
  })
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

/** Opens the Admin view: the switch in the bar, or on a phone the menu. */
export async function toAdmin(page: Page) {
  const view = page.getByRole('group', { name: 'View' }).getByRole('link', { name: 'Admin' })
  await expect(page.locator('summary.me-btn')).toBeVisible()
  if (await view.isVisible()) await view.click()
  else {
    await page.getByLabel('Menu').click()
    await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Dashboard' }).click()
  }
}

/** The confirmation link Mailpit received for to, waiting for it to arrive. */
export function confirmationLink(page: Page, to: string): Promise<string> {
  return mailedLink(page, to, '/confirm')
}

/** The newest link to path in the mail Mailpit received for to, waiting for it to arrive. */
export async function mailedLink(page: Page, to: string, path: string): Promise<string> {
  let link = ''
  await expect(async () => {
    const search = await page.request.get(`${mailpit}/api/v1/search?query=${encodeURIComponent(`to:"${to}"`)}`)
    const { messages } = (await search.json()) as { messages: { ID: string }[] }
    expect(messages.length).toBeGreaterThan(0)
    const message = await page.request.get(`${mailpit}/api/v1/message/${messages[0]?.ID ?? ''}`)
    const { Text } = (await message.json()) as { Text: string }
    link = new RegExp(`https?://\\S+${path}\\?token=\\S+`).exec(Text)?.[0] ?? ''
    expect(link).not.toBe('')
  }).toPass({ timeout: 10_000 })
  return link
}

/** Signs a new User up, confirms them through Mailpit and signs them in; returns their email. */
export async function signedUp(page: Page, prefix: string): Promise<string> {
  const email = `${prefix}-${String(Date.now())}-${String(Math.floor(Math.random() * 1e6))}@example.org`
  const password = 'correct horse battery'
  await page.goto('/signup')
  await page.getByLabel('Name', { exact: true }).fill('Grace Hopper')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Create account' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Check your email')
  await page.goto(await confirmationLink(page, email))
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Confirm email' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Email confirmed')
  await signIn(page, email)
  await expect(page).toHaveURL('/')
  return email
}

/** Signs in with the password every test User has; a second factor, if any, is left to the test. */
export async function signIn(page: Page, email: string) {
  await page.goto('/signin')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill('correct horse battery')
  await page.getByRole('button', { name: 'Sign in' }).click()
}

/** Signs out from the menu under the User's avatar. */
export async function signOut(page: Page) {
  await page.locator('summary.me-btn').click()
  // Wait for the session to end: the page is often at / already, so the URL alone proves nothing.
  const ended = page.waitForResponse((r) => r.request().method() === 'DELETE' && new URL(r.url()).pathname === '/api/v1/session')
  await page.getByRole('button', { name: 'Sign out' }).click()
  expect((await ended).status()).toBe(204)
  await expect(page).toHaveURL('/')
}

/** Runs the binary's admin command, as an operator would in the pod. */
export function promote(email: string): string {
  return execFileSync('../bin/identity', ['admin', 'promote', email, '--force'], {
    env: { PATH: process.env.PATH ?? '', DATABASE_URL: process.env.DATABASE_URL ?? 'postgres://app:app@localhost:25432/app?sslmode=disable' },
    encoding: 'utf8',
  })
}

/** The RFC 6238 code an authenticator app shows for a base32 secret, a step away from now if asked. */
export function totp(secret: string, step = 0): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/=+$/, '')) bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  const key = Buffer.from(bits.match(/.{8}/g)?.map((b) => parseInt(b, 2)) ?? [])
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30_000) + step))
  const mac = createHmac('sha1', key).update(counter).digest()
  const offset = (mac[mac.length - 1] ?? 0) & 0xf
  return String((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

/** Sets up an authenticator app, step by step, and returns its secret. */
export async function enrolTOTP(page: Page): Promise<string> {
  await page.goto('/security/two-factor/setup')
  const secret = (await page.locator('.secret code').textContent()) ?? ''
  await page.getByRole('button', { name: 'I have scanned it' }).click()
  await page.getByLabel('Digit 1 of 6').fill(totp(secret))
  await page.getByLabel('Name', { exact: true }).fill('Phone')
  await page.getByRole('button', { name: 'Continue' }).click()
  await page.getByLabel('Password', { exact: true }).fill('correct horse battery')
  await page.getByRole('button', { name: 'Turn on two-factor' }).click()
  await expect(page.getByText('Two-factor is on')).toBeVisible()
  return secret
}

/** Makes a Group on the Groups page and leaves it open. */
export async function makeGroup(page: Page, name: string) {
  await page.goto('/admin/groups')
  await page.getByRole('button', { name: 'New Group' }).click()
  await page.getByLabel('Name', { exact: true }).fill(name)
  await page.getByRole('button', { name: 'Create Group' }).click()
  await page.getByRole('navigation', { name: 'Groups', exact: true }).getByRole('link', { name }).click()
  await expect(page.getByRole('heading', { level: 2, name })).toBeVisible()
}

/** Adds the person with this email to the Group that is open. */
export async function addMember(page: Page, email: string) {
  await page.getByRole('button', { name: /^Members/ }).click()
  const find = page.getByLabel('Add people')
  await find.click()
  await find.fill(email)
  const added = page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().includes('/members/') && r.status() === 204)
  await page.locator('.sugg').filter({ hasText: email }).click()
  await added
  await expect(page.locator('.m-row').filter({ hasText: email })).toBeVisible()
  await page.getByRole('button', { name: /^Access/ }).click()
}

/**
 * Gives the open Group a role on an app and waits for identity to keep it. The chip lights at once,
 * so leaving the page before the answer can cancel the request.
 */
export async function givesRole(page: Page, group: string, app: string, role: string) {
  const chip = page.getByRole('group', { name: `${group} on ${app}` }).getByRole('button', { name: role, exact: true })
  const kept = page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().includes('/roles/') && r.status() === 204)
  await chip.click()
  await kept
  await expect(chip).toHaveAttribute('aria-pressed', 'true')
}
