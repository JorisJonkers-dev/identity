import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { configureApi } from '../../../infrastructure/http'
import { describeError, useVersion } from './useVersion'

function mountWith(fetch: typeof globalThis.fetch) {
  configureApi({ baseUrl: 'http://app.test', fetch })
  let version: ReturnType<typeof useVersion> | undefined
  mount(defineComponent({ setup: () => ((version = useVersion()), () => h('div')) }), {
    global: { plugins: [[VueQueryPlugin, { queryClient: new QueryClient({ defaultOptions: { queries: { retry: false } } }) }]] },
  })
  if (!version) throw new Error('useVersion did not run')
  return version
}

describe('useVersion', () => {
  it('has no version before the first response', () => {
    const version = mountWith(() => new Promise<Response>(() => undefined))
    expect(version.version.value).toBeNull()
    expect(version.loadError.value).toBeNull()
  })

  it('reads the version the API returns', async () => {
    const version = mountWith(() => Promise.resolve(Response.json({ version: '0.4.0' })))
    await flushPromises()
    expect(version.version.value).toBe('0.4.0')
  })

  it('says what went wrong when the version cannot load', async () => {
    const version = mountWith(() =>
      Promise.resolve(
        new Response(JSON.stringify({ type: 'about:blank', title: 'Not Found', status: 404 }), {
          status: 404,
          headers: { 'Content-Type': 'application/problem+json' },
        }),
      ),
    )
    await flushPromises()
    expect(version.loadError.value).toBe('Not Found')
  })
})

describe('describeError', () => {
  it("prefers a problem's detail, then its title", () => {
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400, detail: 'Fix it.' })).toBe('Fix it.')
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400 })).toBe('Bad Request')
  })

  it('falls back for anything that is not a problem', () => {
    expect(describeError(new TypeError('Failed to fetch'))).toBe('Something went wrong. Try again.')
  })
})
