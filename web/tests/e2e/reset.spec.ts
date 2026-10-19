import { expect, test } from '@playwright/test'
import { expectAccessible, mailedLink, signedUp, signOut } from './helpers'

test('a person who forgot their password chooses a new one through the mailed link', async ({ page }, info) => {
  const email = await signedUp(page, `forgot-${info.project.name}`)
  await signOut(page)

  await test.step('the sign-in page offers a reset, which mails a link', async () => {
    await page.goto('/signin')
    await page.getByRole('link', { name: 'Forgot password?' }).click()
    await expect(page.getByLabel('Email', { exact: true })).toBeFocused()
    await page.getByLabel('Email', { exact: true }).fill(email)
    await page.getByRole('button', { name: 'Send link' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Check your email')
    await expectAccessible(page)
  })

  await test.step('the link takes a new password', async () => {
    await page.goto(await mailedLink(page, email, '/reset'))
    await page.getByLabel('New password').fill('a brand new passphrase')
    await expectAccessible(page)
    await page.getByRole('button', { name: 'Save password' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Password changed')
  })

  await test.step('the new password signs in', async () => {
    await page.getByRole('link', { name: 'Sign in' }).click()
    await page.getByLabel('Email', { exact: true }).fill(email)
    await page.getByLabel('Password', { exact: true }).fill('a brand new passphrase')
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page).toHaveURL('/')
  })
})
