import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, signedUp, totp } from './helpers'

test('a User adds an authenticator app, and signing in then asks for its code', async ({ page }, info) => {
  const email = await signedUp(page, `totp-${info.project.name}`)
  const secret = await enrolTOTP(page)
  await expect(page.getByRole('listitem').filter({ hasText: 'Phone' })).toBeVisible()
  await expectAccessible(page)

  await page.goto('/profile')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill('correct horse battery')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText("Confirm it's you")
  await expectAccessible(page)

  await page.getByLabel('6-digit code').fill('000000')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toContainText('That code is wrong')

  // The code used to add the app was spent; the next step's code signs in.
  await page.getByLabel('6-digit code').fill(totp(secret, 1))
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL('/profile')
})
