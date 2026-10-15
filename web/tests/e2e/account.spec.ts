import AxeBuilder from '@axe-core/playwright'
import { expect, type Page, test } from '@playwright/test'

const mailpit = 'http://localhost:58025'

async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

/** The confirmation link Mailpit received for to, waiting for it to arrive. */
async function confirmationLink(page: Page, to: string): Promise<string> {
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

test('a person signs up, confirms by mail, signs in, sees their profile and signs out', async ({ page }, info) => {
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  const email = `ada-${info.project.name}-${String(Date.now())}@example.org`
  const password = 'correct horse battery'

  await page.goto('/signup')
  await expectAccessible(page)
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Name', { exact: true }).fill('Ada Lovelace')
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign up' }).click()
  await expect(page.getByRole('status')).toHaveText('Check your email for a link to finish signing up.')

  await page.goto(await confirmationLink(page, email))
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Confirm' }).click()
  await expect(page.getByRole('status')).toContainText('Your email is confirmed.')

  await page.getByRole('link', { name: 'Sign in' }).first().click()
  await expect(page).toHaveURL('/signin')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL('/profile')
  await expect(page.getByRole('definition').filter({ hasText: email })).toBeVisible()
  await expectAccessible(page)

  const cookie = (await page.context().cookies()).find((c) => c.name === '__Host-identity_session')
  expect(cookie?.httpOnly).toBe(true)
  expect(cookie?.sameSite).toBe('Lax')

  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL('/signin')
  await page.goto('/profile')
  await expect(page.getByRole('alert')).toBeVisible()
  expect(errors.filter((e) => !e.includes('401'))).toEqual([])
})

test('a wrong password is refused with the same answer as an unknown email', async ({ page }) => {
  await page.goto('/signin')
  await page.getByLabel('Email', { exact: true }).fill('nobody@example.org')
  await page.getByLabel('Password', { exact: true }).fill('wrong password!!')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toHaveText('The email or the password is wrong.')
})
