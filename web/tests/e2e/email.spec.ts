import { expect, test } from '@playwright/test'
import { expectAccessible, mailedLink, signedUp, signIn, signOut } from './helpers'

test('a person changes their sign-in email through the link mailed to the new address', async ({ page }, info) => {
  const email = await signedUp(page, `old-${info.project.name}`)
  const next = email.replace('old-', 'new-')

  await test.step('the Account page asks for the new address and the password', async () => {
    await page.goto('/account')
    await page.getByRole('button', { name: 'Change' }).click()
    await page.getByLabel('New email').fill(next)
    await page.getByLabel('Password', { exact: true }).fill('correct horse battery')
    await page.getByRole('button', { name: 'Send link' }).click()
    await expect(page.getByText(`Waiting for you to open the link sent to ${next}`)).toBeVisible()
    await expectAccessible(page)
  })

  await test.step('the link mailed to the new address makes it the sign-in email', async () => {
    await page.goto(await mailedLink(page, next, '/account/email'))
    await expectAccessible(page)
    await page.getByRole('button', { name: 'Change my sign-in email' }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Email changed')
  })

  await test.step('the Account page shows the new address, and it signs in', async () => {
    await page.getByRole('link', { name: 'Go to your account' }).click()
    await expect(page.locator('.hero-t p')).toHaveText(next)
    await signOut(page)
    await signIn(page, next)
    await expect(page).toHaveURL('/')
  })
})
