import { execFileSync } from 'node:child_process'
import AxeBuilder from '@axe-core/playwright'
import { expect, type Page } from '@playwright/test'

const mailpit = 'http://localhost:58025'

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
  await expect(page).toHaveURL('/profile')
  return email
}

/** Runs the binary's admin command, as an operator would in the pod. */
export function promote(email: string): string {
  return execFileSync('../bin/identity', ['admin', 'promote', email, '--force'], {
    env: { PATH: process.env.PATH ?? '', DATABASE_URL: process.env.DATABASE_URL ?? 'postgres://app:app@localhost:55432/app?sslmode=disable' },
    encoding: 'utf8',
  })
}
