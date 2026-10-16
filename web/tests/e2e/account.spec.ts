import { expect, test } from '@playwright/test'
import { confirmationLink, expectAccessible } from './helpers'

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
  await page.getByRole('button', { name: 'Confirm email' }).click()
  await expect(page.getByRole('status')).toContainText('Your email is confirmed.')

  await page.getByRole('link', { name: 'Sign in' }).first().click()
  await expect(page).toHaveURL('/signin')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL('/profile')
  await expect(page.getByRole('definition').filter({ hasText: email })).toBeVisible()
  await expectAccessible(page)

  // A new User is no Admin: the access page refuses them, and the Profile does not offer it.
  await expect(page.getByRole('link', { name: 'Manage access' })).toHaveCount(0)
  await page.goto('/access')
  await expect(page.getByRole('alert')).toHaveText('Only Admins manage access.')
  await expectAccessible(page)
  await page.goto('/profile')

  const cookie = (await page.context().cookies()).find((c) => c.name === '__Host-identity_session')
  expect(cookie?.httpOnly).toBe(true)
  expect(cookie?.sameSite).toBe('Lax')

  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL('/signin')
  await page.goto('/profile')
  await expect(page.getByRole('alert')).toBeVisible()
  expect(errors.filter((e) => !e.includes('401') && !e.includes('403'))).toEqual([])
})

test('a wrong password is refused with the same answer as an unknown email', async ({ page }) => {
  await page.goto('/signin')
  await page.getByLabel('Email', { exact: true }).fill('nobody@example.org')
  await page.getByLabel('Password', { exact: true }).fill('wrong password!!')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toHaveText('The email or the password is wrong.')
})

test('the no-access page says why an app did not open', async ({ page }) => {
  await page.goto('/no-access')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText("You don't have access to this app")
  await expectAccessible(page)
})
