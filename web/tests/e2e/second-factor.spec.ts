import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, signedUp, signIn, signOut, totp } from './helpers'

test('a User adds an authenticator app, and signing in then asks for its code', async ({ page }, info) => {
  const email = await signedUp(page, `totp-${info.project.name}`)
  let secret = ''

  await test.step('a User sets up an authenticator app', async () => {
    secret = await enrolTOTP(page)
    await page.goto('/security/two-factor')
    await expect(page.locator('.group .row').filter({ hasText: 'Phone' })).toContainText('Authenticator app')
    await expectAccessible(page)
  })

  await test.step('signing in again asks for the code after the password, with the cursor in the first box', async () => {
    await signOut(page)
    await signIn(page, email)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Enter your code')
    await expect(page.getByLabel('Digit 1 of 6')).toBeFocused()
    await expectAccessible(page)
  })

  await test.step('a wrong code is refused', async () => {
    await page.keyboard.type('000000')
    await page.getByRole('button', { name: 'Verify' }).click()
    await expect(page.getByRole('alert')).toContainText('That code is wrong')
  })

  await test.step("the app's next code signs in; the one used to add it was spent", async () => {
    await page.getByLabel('Digit 1 of 6').fill(totp(secret, 1))
    await page.getByRole('button', { name: 'Verify' }).click()
    await expect(page).toHaveURL('/')
  })
})
