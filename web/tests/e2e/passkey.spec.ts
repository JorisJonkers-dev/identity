import { expect, test } from '@playwright/test'
import { expectAccessible, signedUp } from './helpers'

test('a User adds a passkey, and signing in then uses it', async ({ page }, info) => {
  // Chromium's virtual authenticator stands in for Touch ID or a security key.
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true },
  })
  const email = await signedUp(page, `passkey-${info.project.name}`)

  await test.step('a User adds a passkey with their password', async () => {
    await page.goto('/security')
    const form = page.getByRole('form', { name: 'Add a passkey' })
    await form.getByLabel('Name for this passkey').fill('This laptop')
    await form.getByLabel('Your password').fill('correct horse battery')
    await form.getByRole('button', { name: 'Add passkey' }).click()
    await expect(page.getByRole('status')).toHaveText('Passkey added. Every other device has been signed out. Next time, choose Use your passkey after your password.')
    await expect(page.getByRole('listitem').filter({ hasText: 'This laptop' })).toContainText('Passkey')
    await expectAccessible(page)
  })

  await test.step('signing in again asks for the passkey after the password', async () => {
    await page.goto('/profile')
    await page.getByRole('button', { name: 'Sign out' }).click()
    await page.getByLabel('Email', { exact: true }).fill(email)
    await page.getByLabel('Password', { exact: true }).fill('correct horse battery')
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText("Confirm it's you")
    await expectAccessible(page)
  })

  await test.step('the passkey finishes signing in', async () => {
    await page.getByRole('button', { name: 'Use your passkey' }).click()
    await expect(page).toHaveURL('/')
  })
})
