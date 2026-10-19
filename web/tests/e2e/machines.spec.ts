import { expect, test } from '@playwright/test'
import { enrolTOTP, expectAccessible, promote, signedUp } from './helpers'

test('an Admin adds a Machine Identity, gives it a role and a secret, and deletes it', async ({ page }, info) => {
  const email = await signedUp(page, `machines-${info.project.name}`)
  promote(email)
  await enrolTOTP(page)
  const name = `Backup ${info.project.name} ${String(Date.now())}`
  const serviceAccount = `backup-${info.project.name}-${String(Date.now())}`

  await test.step('the Machines tab adds one bound to a ServiceAccount', async () => {
    await page.goto('/admin/machines')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Machines')
    await page.getByRole('button', { name: 'Add Machine Identity' }).click()
    await page.getByLabel('Name', { exact: true }).fill(name)
    await page.getByLabel('Namespace').fill('media')
    await page.getByLabel('ServiceAccount').fill(serviceAccount)
    await expectAccessible(page)
    await page.getByRole('button', { name: 'Add', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(name)
    await expect(page.getByText(`Signs in as the ServiceAccount media/${serviceAccount}`)).toBeVisible()
  })

  await test.step('its page gives it a role', async () => {
    const firstApp = page.locator('.acc-row').first()
    await firstApp.getByRole('button').first().click()
    await expect(firstApp.locator('.acc-n small')).toHaveText('Has access')
    await expectAccessible(page)
  })

  await test.step('a new secret is shown once', async () => {
    await page.getByRole('button', { name: 'Make a new secret' }).click()
    await expect(page.getByRole('dialog')).toContainText('Shown once.')
    await expect(page.locator('.secret code')).not.toBeEmpty()
    await expectAccessible(page)
    await page.getByRole('button', { name: 'Done' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
  })

  await test.step('it is deleted after a question naming it', async () => {
    await page.getByRole('button', { name: 'Delete', exact: true }).click()
    await expect(page.getByRole('dialog')).toContainText(`Delete ${name}?`)
    await page.getByRole('button', { name: `Delete ${name}` }).click()
    await expect(page).toHaveURL('/admin/machines')
    await expect(page.getByText(name)).toHaveCount(0)
  })
})
