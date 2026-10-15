import { expect, test } from '@playwright/test'

test('the Provider publishes discovery at the Issuer and its signing key', async ({ request, baseURL }) => {
  const discovery = await request.get('/.well-known/openid-configuration')
  expect(discovery.ok()).toBe(true)
  const config = (await discovery.json()) as { issuer: string; jwks_uri: string; authorization_endpoint: string }
  expect(config.issuer).toBe(baseURL)
  expect(config.authorization_endpoint).toBe(config.issuer + '/oidc/authorize')

  const jwks = (await (await request.get(config.jwks_uri)).json()) as { keys: { alg: string; use: string }[] }
  expect(jwks.keys).toEqual([expect.objectContaining({ alg: 'RS256', use: 'sig' })])
})

test('a sign-in the Provider never started is refused', async ({ request }) => {
  const res = await request.get('/oidc/login?authRequestID=00000000-0000-4000-8000-000000000000', { maxRedirects: 0 })
  expect(res.status()).toBe(400)
})
