import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { configureApi } from '../infrastructure/http'
import App from './App.vue'
import { createAppRouter } from './router'

type Handler = (request: Request) => Response | Promise<Response>

function json(body: unknown, status = 200, contentType = 'application/json'): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': contentType } })
}

const note = (text: string, id = crypto.randomUUID()) => ({ id, text, createdAt: '2026-10-02T09:30:00Z' })

/** Mounts the whole app at path, with the network answered by handle. */
async function mountApp(handle: Handler, path = '/') {
  const fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
    Promise.resolve(handle(input instanceof Request ? input : new Request(input, init))),
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
  return { wrapper, fetch, router }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the notes page', () => {
  it('lists the notes the API returns', async () => {
    const { wrapper } = await mountApp(() => json({ items: [note('Buy milk'), note('Call home')] }))
    expect(wrapper.findAll('li').map((li) => li.find('span').text())).toEqual(['Buy milk', 'Call home'])
  })

  it('accepts a time with any offset, as RFC 3339 does', async () => {
    const { wrapper } = await mountApp(() => json({ items: [{ ...note('Later'), createdAt: '2026-10-02T11:30:00+02:00' }] }))
    expect(wrapper.get('li time').attributes('datetime')).toBe('2026-10-02T11:30:00+02:00')
  })

  it('says so when there are no notes', async () => {
    const { wrapper } = await mountApp(() => json({ items: [] }))
    expect(wrapper.text()).toContain('No notes yet.')
  })

  it('shows the problem when the list cannot load', async () => {
    const { wrapper } = await mountApp(() =>
      json({ type: 'about:blank', title: 'Internal Server Error', status: 500 }, 500, 'application/problem+json'),
    )
    expect(wrapper.get('[role="alert"]').text()).toBe('Internal Server Error')
  })

  it('refuses a response the contract does not allow', async () => {
    const { wrapper } = await mountApp(() => json({ items: [{ id: 'not-a-uuid', text: '', createdAt: 'yesterday' }] }))
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('adds a note, clears the field and lists it', async () => {
    const stored: ReturnType<typeof note>[] = []
    const { wrapper } = await mountApp(async (request) => {
      if (request.method === 'POST') {
        const body = (await request.json()) as { text: string }
        stored.unshift(note(body.text))
        return json(stored[0], 201)
      }
      return json({ items: stored })
    })
    const input = wrapper.get<HTMLInputElement>('#note-text')
    await input.setValue('  Buy milk ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await flushPromises()
    expect(stored.map((n) => n.text)).toEqual(['Buy milk'])
    expect(input.element.value).toBe('')
    expect(wrapper.get('li span').text()).toBe('Buy milk')
  })

  it('does not send text the domain refuses, and says why', async () => {
    const { wrapper, fetch } = await mountApp(() => json({ items: [] }))
    const button = wrapper.get<HTMLButtonElement>('button[type="submit"]')
    expect(button.element.disabled).toBe(true)
    await wrapper.get('#note-text').setValue('   ')
    expect(wrapper.get('#note-hint').text()).toBe('Write something first.')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(fetch.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0)
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it("shows the server's refusal of a note", async () => {
    const { wrapper } = await mountApp((request) =>
      request.method === 'POST'
        ? json({ type: 'about:blank', title: 'Invalid note', status: 422, detail: 'Too long.' }, 422, 'application/problem+json')
        : json({ items: [] }),
    )
    await wrapper.get('#note-text').setValue('x')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Too long.')
  })

  it('sends unknown paths home', async () => {
    const { router } = await mountApp(() => json({ items: [] }), '/no/such/page')
    expect(router.currentRoute.value.path).toBe('/')
  })
})
