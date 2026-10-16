import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { configureApi } from '../infrastructure/http'
import App from './App.vue'
import { createAppRouter } from './router'

/** What a test answers a request with: a response, or a function making one per request. */
type Answer = Response | (() => Response) | (() => Promise<Response>)

export function problem(status: number, detail: string): Response {
  return new Response(JSON.stringify({ type: 'about:blank', title: 'Problem', status, detail }), {
    status,
    headers: { 'Content-Type': 'application/problem+json' },
  })
}

/** Mounts the app at path, answering each "METHOD /path" from answers and recording the requests. */
export async function mountAt(path: string, answers: Record<string, Answer>) {
  const requests: { key: string; body: unknown }[] = []
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init)
    const key = `${request.method} ${new URL(request.url).pathname}`
    const text = await request.text()
    requests.push({ key, body: text ? (JSON.parse(text) as unknown) : null })
    const answer = answers[key]
    if (!answer) return problem(404, `no answer for ${key}`)
    return typeof answer === 'function' ? answer() : answer.clone()
  })
  configureApi({ baseUrl: 'http://app.test', fetch })
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = mount(App, { global: { plugins: [router, [VueQueryPlugin, { queryClient }]] } })
  await router.isReady()
  await settle()
  return { wrapper, router, requests }
}

export async function settle() {
  await flushPromises()
  await vi.dynamicImportSettled()
  await flushPromises()
}
