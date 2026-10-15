import { afterEach, describe, expect, it } from 'vitest'
import { client } from './api/client.gen'
import { configureApi } from './http'

describe('configureApi', () => {
  const original = client.getConfig()
  afterEach(() => client.setConfig(original))

  it('targets the page origin with the platform fetch by default', () => {
    configureApi()
    expect(client.getConfig().baseUrl).toBe('')
    expect(client.getConfig().fetch).toBe(original.fetch)
  })

  it('takes a base URL and a fetch when given', () => {
    const fetch: typeof globalThis.fetch = () => Promise.resolve(new Response(null))
    configureApi({ baseUrl: 'http://app.test', fetch })
    expect(client.getConfig().baseUrl).toBe('http://app.test')
    expect(client.getConfig().fetch).toBe(fetch)
  })
})
