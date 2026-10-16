import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, signedUp, totp } from './helpers'

test('a User adds an authenticator app, and signing in then asks for its code', async ({ page }, info) => {
  const email = await signedUp(page, `totp-${info.project.name}`)
  let secret = ''

  await test.step('a User adds an authenticator app', async () => {
    secret = await enrolTOTP(page)
    await expect(page.getByRole('listitem').filter({ hasText: 'Phone' })).toBeVisible()
    await expectAccessible(page)
  })

  await test.step('signing in again asks for the code after the password', async () => {
    await page.goto('/profile')
    await page.getByRole('button', { name: 'Sign out' }).click()
    await page.getByLabel('Email', { exact: true }).fill(email)
    await page.getByLabel('Password', { exact: true }).fill('correct horse battery')
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText("Confirm it's you")
    await expectAccessible(page)
  })

  await test.step('a wrong code is refused', async () => {
    await page.getByLabel('6-digit code').fill('000000')
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByRole('alert')).toContainText('That code is wrong')
  })

  await test.step("the app's next code signs in; the one used to add it was spent", async () => {
    await page.getByLabel('6-digit code').fill(totp(secret, 1))
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page).toHaveURL('/')
  })
})
