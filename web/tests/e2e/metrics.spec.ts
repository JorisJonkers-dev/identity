import { expect, test } from '@playwright/test'

test('the binary serves the Unassigned count on its own metrics port, and not on the public one', async ({ request }) => {
  const metrics = await request.get('http://127.0.0.1:18574/metrics')
  expect(metrics.status()).toBe(200)
  expect(await metrics.text()).toMatch(/^identity_unassigned_users \d+$/m)

  const publicPort = await request.get('/metrics')
  expect(await publicPort.text()).not.toContain('identity_unassigned_users')
})
