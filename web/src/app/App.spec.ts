import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { configureApi } from '../infrastructure/http'
import App from './App.vue'
import { createAppRouter } from './router'

const signedOut = () =>
  new Response(JSON.stringify({ type: 'about:blank', title: 'Unauthorized', status: 401 }), {
    status: 401,
    headers: { 'Content-Type': 'application/problem+json' },
  })

/** Mounts the whole app at path, signed out, with the version answered by respond. */
async function mountApp(respond: () => Response, path = '/') {
  const fetch = vi.fn((input: RequestInfo | URL) =>
    Promise.resolve(new Request(input).url.endsWith('/api/v1/launcher') ? signedOut() : respond()),
  )
  configureApi({ baseUrl: 'http://app.test', fetch })
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = mount(App, { global: { plugins: [router, [VueQueryPlugin, { queryClient }]] } })
  await router.isReady()
  await flushPromises()
  await vi.dynamicImportSettled()
  await flushPromises()
  return { wrapper, router }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the home page', () => {
  it('names the service and shows the running version', async () => {
    const { wrapper } = await mountApp(() => Response.json({ version: '0.4.0' }))
    expect(wrapper.get('h1').text()).toBe('jorisjonkers.dev')
    expect(wrapper.get('footer').text()).toBe('Version 0.4.0')
  })

  it('shows the problem when the version cannot load', async () => {
    const { wrapper } = await mountApp(
      () =>
        new Response(JSON.stringify({ type: 'about:blank', title: 'Internal Server Error', status: 500 }), {
          status: 500,
          headers: { 'Content-Type': 'application/problem+json' },
        }),
    )
    expect(wrapper.get('[role="alert"]').text()).toBe('Internal Server Error')
  })

  it('sends an unknown path home', async () => {
    const { router } = await mountApp(() => Response.json({ version: '0.4.0' }), '/no/such/page')
    expect(router.currentRoute.value.path).toBe('/')
  })
})
