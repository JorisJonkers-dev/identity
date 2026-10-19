import { expect, test } from '@playwright/test'
import { confirmationLink, expectAccessible, signedUp, signIn, signOut } from './helpers'

test('a person signs up, confirms by mail, signs in, sees their account and signs out', async ({ page }, info) => {
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  const email = `ada-${info.project.name}-${String(Date.now())}@example.org`
  const password = 'correct horse battery'

  await test.step('a person signs up and is told to check their mail', async () => {
    await page.goto('/signup')
    await expect(page.getByLabel('Name', { exact: true })).toBeFocused()
    await expectAccessible(page)
    await page.getByLabel('Name', { exact: true }).fill('Ada Lovelace')
    await page.getByLabel('Email', { exact: true }).fill(email)
    await page.getByLabel('Password', { exact: true }).fill(password)
    await page.getByRole('button', { name: 'Create account' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Check your email')
    await expect(page.getByText(`Link sent to ${email}.`)).toBeVisible()
  })

  await test.step('the mailed link confirms the email with the password', async () => {
    await page.goto(await confirmationLink(page, email))
    await expectAccessible(page)
    await page.getByLabel('Password', { exact: true }).fill(password)
    await page.getByRole('button', { name: 'Confirm email' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Email confirmed')
  })

  await test.step('signing in lands on the apps, and the Account page shows the email', async () => {
    await page.getByRole('link', { name: 'Sign in' }).click()
    await expect(page).toHaveURL('/signin')
    await expect(page.getByLabel('Email', { exact: true })).toBeFocused()
    await expectAccessible(page)
    await signIn(page, email)
    await expect(page).toHaveURL('/')
    await page.goto('/account')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Ada Lovelace')
    await expect(page.locator('.hero-t p')).toHaveText(email)
    await expectAccessible(page)
  })

  await test.step('the session cookie is HttpOnly and SameSite Lax', async () => {
    const cookie = (await page.context().cookies()).find((c) => c.name === '__Host-identity_session')
    expect(cookie?.httpOnly).toBe(true)
    expect(cookie?.sameSite).toBe('Lax')
  })

  await test.step('signing out ends the session', async () => {
    await signOut(page)
    await page.goto('/account')
    await expect(page).toHaveURL('/signin')
    expect(errors.filter((e) => !e.includes('401') && !e.includes('403'))).toEqual([])
  })
})

test('a new User is no Admin: no Admin view is offered, and the Admin pages send them home', async ({ page }, info) => {
  await signedUp(page, `not-admin-${info.project.name}`)
  await page.goto('/account')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Grace Hopper')
  await expect(page.getByRole('group', { name: 'View' })).toHaveCount(0)
  await page.goto('/admin/groups')
  await expect(page).toHaveURL('/')
  await expectAccessible(page)
})

test('a wrong password is refused with the same answer as an unknown email', async ({ page }) => {
  await signIn(page, 'nobody@example.org')
  await expect(page.getByRole('alert')).toHaveText('The email or the password is wrong.')
  await expectAccessible(page)
})

test('the no-access page sends the person back to their apps', async ({ page }) => {
  await page.goto('/no-access')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('No access to this app')
  await expect(page.getByRole('link', { name: 'Your apps' })).toHaveAttribute('href', '/')
  await expectAccessible(page)
})
