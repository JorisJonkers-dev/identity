import type { DOMWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { profile } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import type { ConnectedApp } from '../../infrastructure/api/types.gen'
import { navigation } from '../../infrastructure/navigation'

const request = '0b6f4a52-6a8f-4b8e-9d55-6f0b0f2d6e11'
const prompt = { app: 'Claude Code', client: 'Knowledge', roles: ['user', 'editor'], redirectHost: '127.0.0.1:33418' }
const asking = (over: Record<string, () => Response> = {}) => ({
  'GET /oidc/consent': () => Response.json(prompt),
  ...over,
})

const claude: ConnectedApp = {
  id: '00000000-0000-4000-8000-0000000000c1', name: 'Claude Code', client: { id: 'kb', label: 'Knowledge' },
  createdAt: '2026-10-18T09:00:00Z', lastUsedAt: '2026-10-19T11:00:00Z',
}
const cursor: ConnectedApp = { id: '00000000-0000-4000-8000-0000000000c2', name: 'Cursor', client: { id: 'kb', label: 'Knowledge' }, createdAt: '2026-10-19T09:00:00Z' }

function button(wrapper: { findAll: (selector: string) => DOMWrapper<Element>[] }, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the consent page', () => {
  it('asks whether to connect the app, naming the service and the roles it acts with', async () => {
    const { wrapper } = await mountAt(`/connect?authRequestID=${request}`, asking())
    expect(wrapper.get('h1').text()).toBe('Connect Claude Code to Knowledge?')
    expect(wrapper.findAll('.role').map((r) => r.text())).toEqual(['user', 'editor'])
    expect(wrapper.text()).toContain('After you connect, it continues at 127.0.0.1:33418.')
  })

  it('asks the server about the request the link names', async () => {
    const { requests } = await mountAt(`/connect?authRequestID=${request}`, asking())
    expect(requests.some((r) => r.key === 'GET /oidc/consent')).toBe(true)
  })

  it('connects the app and sends the browser on to it', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, requests } = await mountAt(`/connect?authRequestID=${request}`, asking({ 'POST /oidc/consent': () => Response.json({ redirect: '/oidc/authorize/callback?id=x' }) }))
    await button(wrapper, 'Connect').trigger('click')
    await settle()
    expect(requests.find((r) => r.key === 'POST /oidc/consent')?.body).toEqual({ authRequestID: request, allow: true })
    expect(leave).toHaveBeenCalledWith('/oidc/authorize/callback?id=x')
  })

  it('cancels and tells the app no', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, requests } = await mountAt(`/connect?authRequestID=${request}`, asking({ 'POST /oidc/consent': () => Response.json({ redirect: 'http://127.0.0.1:33418/cb?error=access_denied' }) }))
    await button(wrapper, 'Cancel').trigger('click')
    await settle()
    expect(requests.find((r) => r.key === 'POST /oidc/consent')?.body).toEqual({ authRequestID: request, allow: false })
    expect(leave).toHaveBeenCalledWith('http://127.0.0.1:33418/cb?error=access_denied')
  })

  it('shows why an answer was refused', async () => {
    const { wrapper } = await mountAt(`/connect?authRequestID=${request}`, asking({ 'POST /oidc/consent': () => problem(403, 'This tool cannot connect to that app.') }))
    await button(wrapper, 'Connect').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('This tool cannot connect to that app.')
  })

  it('sends someone who is not signed in to sign in first, keeping the request', async () => {
    const { router } = await mountAt(`/connect?authRequestID=${request}`, {
      'GET /oidc/consent': () => problem(401, 'Sign in first.'),
      'GET /api/v1/profile': () => problem(401, 'Sign in first.'),
    })
    expect(router.currentRoute.value.path).toBe('/signin')
    expect(router.currentRoute.value.query.authRequestID).toBe(request)
  })

  it('says the link is broken when it names no request', async () => {
    const { wrapper } = await mountAt('/connect', { 'GET /oidc/consent': () => problem(400, 'This link is broken. Start again from the app.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('This link is broken. Start again from the app.')
  })

  it('says the app cannot connect when the request is refused', async () => {
    const { wrapper } = await mountAt(`/connect?authRequestID=${request}`, { 'GET /oidc/consent': () => problem(404, 'This request has expired or was already answered. Start again from the tool.') })
    expect(wrapper.get('h1').text()).toBe('This tool cannot connect')
    expect(wrapper.get('[role="alert"]').text()).toBe('This request has expired or was already answered. Start again from the tool.')
  })
})

describe('the Connected apps page', () => {
  const page = (apps: ConnectedApp[]) => ({
    'GET /api/v1/profile': () => Response.json(profile()),
    'GET /api/v1/profile/connected-apps': () => Response.json({ apps }),
  })

  it('lists each app with the service it uses and when it last did', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-10-19T12:00:00Z'))
    const { wrapper } = await mountAt('/security/connected-apps', page([claude, cursor]))
    vi.useRealTimers()
    const rows = wrapper.findAll('.group .row')
    expect(rows[0]?.get('b').text()).toBe('Claude Code')
    expect(rows[0]?.get('small').text()).toBe('Uses Knowledge as you · Last used 1h ago')
    expect(rows[1]?.get('small').text()).toBe('Uses Knowledge as you · Not used yet')
  })

  it('says when no app acts for the User', async () => {
    const { wrapper } = await mountAt('/security/connected-apps', page([]))
    expect(wrapper.get('.empty-state').text()).toBe('No tool acts for you. A tool asks before it connects.')
  })

  it('disconnects an app and lists what is left', async () => {
    let apps = [claude, cursor]
    const { wrapper, requests } = await mountAt('/security/connected-apps', {
      'GET /api/v1/profile': () => Response.json(profile()),
      'GET /api/v1/profile/connected-apps': () => Response.json({ apps }),
      [`DELETE /api/v1/profile/connected-apps/${claude.id}`]: () => {
        apps = [cursor]
        return new Response(null, { status: 204 })
      },
    })
    await button(wrapper, 'Disconnect').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE /api/v1/profile/connected-apps/${claude.id}`)).toBe(true)
    expect(wrapper.findAll('.group .row b').map((b) => b.text())).toEqual(['Cursor'])
  })

  it('shows a refused disconnect', async () => {
    const { wrapper } = await mountAt('/security/connected-apps', {
      ...page([claude]),
      [`DELETE /api/v1/profile/connected-apps/${claude.id}`]: () => problem(404, 'That app is already disconnected. Reload the page.'),
    })
    await button(wrapper, 'Disconnect').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That app is already disconnected. Reload the page.')
  })

  it('shows the problem when the list cannot load', async () => {
    const { wrapper } = await mountAt('/security/connected-apps', { ...page([]), 'GET /api/v1/profile/connected-apps': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('is linked from the Security page', async () => {
    const { wrapper } = await mountAt('/security', { 'GET /api/v1/profile': () => Response.json(profile()), 'GET /api/v1/profile/two-factor-methods': () => Response.json({ factors: [] }) })
    expect(wrapper.get('a[href="/security/connected-apps"] b').text()).toBe('Connected apps')
  })
})
