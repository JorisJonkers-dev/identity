import { execFileSync } from 'node:child_process'
import { createHmac } from 'node:crypto'
import AxeBuilder from '@axe-core/playwright'
import { expect, type Page } from '@playwright/test'

const mailpit = 'http://localhost:28025'

export async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

/** The confirmation link Mailpit received for to, waiting for it to arrive. */
export async function confirmationLink(page: Page, to: string): Promise<string> {
  let link = ''
  await expect(async () => {
    const search = await page.request.get(`${mailpit}/api/v1/search?query=${encodeURIComponent(`to:"${to}"`)}`)
    const { messages } = (await search.json()) as { messages: { ID: string }[] }
    expect(messages.length).toBeGreaterThan(0)
    const message = await page.request.get(`${mailpit}/api/v1/message/${messages[0]?.ID ?? ''}`)
    const { Text } = (await message.json()) as { Text: string }
    link = /https?:\/\/\S+\/confirm\?token=\S+/.exec(Text)?.[0] ?? ''
    expect(link).not.toBe('')
  }).toPass({ timeout: 10_000 })
  return link
}

/** Signs a new User up, confirms them through Mailpit and signs them in; returns their email. */
export async function signedUp(page: Page, prefix: string): Promise<string> {
  const email = `${prefix}-${String(Date.now())}-${String(Math.floor(Math.random() * 1e6))}@example.org`
  const password = 'correct horse battery'
  await page.goto('/signup')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Name', { exact: true }).fill('Grace Hopper')
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign up' }).click()
  await expect(page.getByRole('status')).toBeVisible()
  await page.goto(await confirmationLink(page, email))
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Confirm email' }).click()
  await expect(page.getByRole('status')).toContainText('Your email is confirmed.')
  await page.goto('/signin')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL('/')
  return email
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

/** Adds an authenticator app on the Security page and returns its secret. */
export async function enrolTOTP(page: Page): Promise<string> {
  await page.goto('/security')
  await page.getByRole('button', { name: 'Add an authenticator app' }).click()
  const secret = (await page.locator('code').textContent()) ?? ''
  const form = page.getByRole('form', { name: 'Add an authenticator app' })
  await form.getByLabel('Name for this app').fill('Phone')
  await form.getByLabel('6-digit code from your app').fill(totp(secret))
  await form.getByLabel('Your password').fill('correct horse battery')
  await form.getByRole('button', { name: 'Add authenticator app' }).click()
  await expect(page.getByRole('status')).toContainText('Authenticator app added.')
  return secret
}
