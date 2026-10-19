import { expect, test } from '@playwright/test'
import { expectAccessible, signedUp, signIn, signOut } from './helpers'

test('a User adds a passkey, and signing in then uses it', async ({ page }, info) => {
  // Chromium's virtual authenticator stands in for Touch ID or a security key.
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true },
  })
  const email = await signedUp(page, `passkey-${info.project.name}`)

  await test.step('a User adds a passkey with their password', async () => {
    await page.goto('/security/two-factor')
    await page.getByRole('button', { name: 'Add a passkey' }).click()
    const dialog = page.getByRole('dialog', { name: 'Add a passkey' })
    await dialog.getByLabel('Name', { exact: true }).fill('This laptop')
    await dialog.getByLabel('Password', { exact: true }).fill('correct horse battery')
    await dialog.getByRole('button', { name: 'Add passkey' }).click()
    await expect(page.locator('.group .row').filter({ hasText: 'This laptop' })).toContainText('Passkey')
    await expectAccessible(page)
  })

  await test.step('signing in again asks for the passkey after the password', async () => {
    await signOut(page)
    await signIn(page, email)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Use your passkey')
    await expectAccessible(page)
  })

  await test.step('the passkey finishes signing in', async () => {
    await page.getByRole('button', { name: 'Use your passkey' }).click()
    await expect(page).toHaveURL('/')
  })
})
