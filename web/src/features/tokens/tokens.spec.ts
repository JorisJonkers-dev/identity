import type { VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mountAt, problem, settle } from '../../app/testing'
import type { PersonalToken } from '../../infrastructure/api/types.gen'
import { expiring, expiry } from './expiry'

const ada = '00000000-0000-4000-8000-0000000000a2'
const day = 24 * 3600 * 1000

function token(over: Partial<PersonalToken> = {}): PersonalToken {
  return {
    id: '00000000-0000-4000-8000-0000000000c1',
    name: 'grafana-cli',
    client: { id: 'grafana', label: 'Grafana' },
    roles: ['viewer'],
    createdAt: '2026-10-01T09:00:00Z',
    expiresAt: new Date(Date.now() + 5.5 * day).toISOString(),
    state: 'expiring',
    ...over,
  }
}

const backup = token({
  id: '00000000-0000-4000-8000-0000000000c2',
  name: 'backup-script',
  client: { id: 'vault', label: 'Vault' },
  roles: ['user'],
  expiresAt: '2027-02-02T09:00:00Z',
  state: 'active',
})

const me = (admin = false) =>
  Response.json({ email: 'ada@example.org', name: 'Ada Lovelace', createdAt: '2026-10-01T09:00:00Z', admin, secondFactor: true })

const ok = () => new Response(null, { status: 204 })

function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

async function fillAndSubmit(wrapper: VueWrapper) {
  await button(wrapper, 'New token').trigger('click')
  const form = wrapper.get('form[aria-label="New token"]')
  await form.get('input').setValue('grafana-cli')
  await form.get('select').setValue('grafana')
  await form.trigger('submit')
  await settle()
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('when a token ends', () => {
  const now = new Date('2026-10-17T09:00:00Z')
  it.each<[string, Partial<PersonalToken>, string]>([
    ['an active token gives its date', { state: 'active', expiresAt: '2027-02-02T09:00:00Z' }, 'expires 2 Feb 2027'],
    ['an expiring token counts its days', { state: 'expiring', expiresAt: '2026-10-22T09:00:00Z' }, 'expires in 5 days'],
    ['a token ending within a day says so', { state: 'expiring', expiresAt: '2026-10-17T20:00:00Z' }, 'expires within a day'],
    ['an expired token gives its date', { state: 'expired', expiresAt: '2026-10-10T09:00:00Z' }, 'expired 10 Oct 2026'],
    ['a revoked token says so', { state: 'revoked' }, 'revoked'],
  ])('%s', (_, over, want) => {
    expect(expiry(token(over), now)).toBe(want)
  })

  it('picks out only the tokens ending soon', () => {
    expect(expiring([token(), backup]).map((t) => t.name)).toEqual(['grafana-cli'])
  })
})

describe('the Tokens page', () => {
  const answers = (tokens: PersonalToken[], clients = [{ id: 'grafana', label: 'Grafana' }]) => ({
    'GET /api/v1/profile': me(),
    'GET /api/v1/profile/tokens': () => Response.json({ tokens, clients }),
  })

  it('lists each token with its app, roles, end and state', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([backup, token()]))
    const rows = wrapper.findAll('[aria-labelledby="tokens-title"] li')
    expect(rows.map((r) => r.text())).toEqual([
      expect.stringMatching(/^backup-script\s*Vault · user · expires 2 Feb 2027\s*active\s*Revoke$/),
      expect.stringMatching(/^grafana-cli\s*Grafana · viewer · expires in \d days\s*expiring\s*Revoke$/),
    ])
  })

  it('says loading before the tokens arrive', async () => {
    const { wrapper } = await mountAt('/profile/tokens', {
      'GET /api/v1/profile': me(),
      'GET /api/v1/profile/tokens': () => new Promise<Response>(() => undefined),
    })
    expect(wrapper.text()).toContain('Loading…')
  })

  it('says why the tokens could not be read', async () => {
    const { wrapper } = await mountAt('/profile/tokens', { 'GET /api/v1/profile': me(), 'GET /api/v1/profile/tokens': problem(401, 'Sign in first.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Sign in first.')
  })

  it('says when there are no tokens', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([]))
    expect(wrapper.text()).toContain('You have no tokens.')
    expect(wrapper.text()).not.toContain('you have none yet')
  })

  it('explains that a token needs an app, when the User has none', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([], []))
    expect(wrapper.text()).toContain('A token is for an app you have a role in, and you have none yet.')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('New token')
  })

  it('shows a token with no role left as having none', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([token({ roles: [] })]))
    expect(wrapper.get('[aria-labelledby="tokens-title"] li').text()).toContain('Grafana · no role ·')
  })

  it('offers no revoke for a token that is already over', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([token({ state: 'revoked' }), token({ id: '00000000-0000-4000-8000-0000000000c9', state: 'expired' })]))
    expect(wrapper.findAll('[aria-labelledby="tokens-title"] button')).toHaveLength(0)
  })

  it('makes a token and shows its secret once', async () => {
    let made = false
    const { wrapper, requests } = await mountAt('/profile/tokens', {
      ...answers([]),
      'GET /api/v1/profile/tokens': () => Response.json({ tokens: made ? [token()] : [], clients: [{ id: 'grafana', label: 'Grafana' }] }),
      'POST /api/v1/profile/tokens': () => {
        made = true
        return Response.json({ token: token(), secret: 'idpat_s3cret' }, { status: 201 })
      },
    })
    await button(wrapper, 'New token').trigger('click')
    const form = wrapper.get('form[aria-label="New token"]')
    await form.get('input').setValue('grafana-cli')
    const [app, lasts] = form.findAll('select')
    await app?.setValue('grafana')
    await lasts?.setValue('30')
    await form.trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/tokens')?.body).toEqual({ name: 'grafana-cli', clientId: 'grafana', days: 30 })
    expect(wrapper.get('[data-secret]').text()).toBe('idpat_s3cret')
    expect(wrapper.text()).toContain("Copy your token now. You won't see it again.")
    expect(wrapper.findAll('[aria-labelledby="tokens-title"] li')).toHaveLength(1)
    await button(wrapper, 'Done').trigger('click')
    expect(wrapper.find('[data-secret]').exists()).toBe(false)
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('copies the secret', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { wrapper } = await mountAt('/profile/tokens', {
      ...answers([]),
      'POST /api/v1/profile/tokens': () => Response.json({ token: token(), secret: 'idpat_s3cret' }, { status: 201 }),
    })
    await fillAndSubmit(wrapper)
    await button(wrapper, 'Copy').trigger('click')
    await settle()
    expect(writeText).toHaveBeenCalledWith('idpat_s3cret')
    expect(button(wrapper, 'Copied').exists()).toBe(true)
  })

  it('leaves the secret to be copied by hand when the browser refuses', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: () => Promise.reject(new Error('denied')) }, configurable: true })
    const { wrapper } = await mountAt('/profile/tokens', {
      ...answers([]),
      'POST /api/v1/profile/tokens': () => Response.json({ token: token(), secret: 'idpat_s3cret' }, { status: 201 }),
    })
    await fillAndSubmit(wrapper)
    await button(wrapper, 'Copy').trigger('click')
    await settle()
    expect(button(wrapper, 'Copy').exists()).toBe(true)
  })

  it('says why a token was not made', async () => {
    const { wrapper } = await mountAt('/profile/tokens', {
      ...answers([]),
      'POST /api/v1/profile/tokens': problem(422, 'You have no role in that app, so a token for it could open nothing.'),
    })
    await fillAndSubmit(wrapper)
    expect(wrapper.get('[role="alert"]').text()).toBe('You have no role in that app, so a token for it could open nothing.')
  })

  it('puts the form away on cancel', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([]))
    await button(wrapper, 'New token').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('revokes a token after asking once', async () => {
    let revoked = false
    const { wrapper, requests } = await mountAt('/profile/tokens', {
      ...answers([]),
      'GET /api/v1/profile/tokens': () => Response.json({ tokens: [revoked ? backup : backup].map((t) => (revoked ? { ...t, state: 'revoked' } : t)), clients: [] }),
      [`DELETE /api/v1/profile/tokens/${backup.id}`]: () => {
        revoked = true
        return ok()
      },
    })
    await wrapper.get('button[aria-label="Revoke backup-script"]').trigger('click')
    expect(wrapper.text()).toContain('Revoke backup-script? Scripts using it stop working.')
    await button(wrapper, 'Yes, revoke').trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/profile/tokens/${backup.id}`)
    expect(wrapper.get('[aria-labelledby="tokens-title"] li').text()).toContain('revoked')
  })

  it('keeps a token when the revoke is cancelled', async () => {
    const { wrapper, requests } = await mountAt('/profile/tokens', answers([backup]))
    await wrapper.get('button[aria-label="Revoke backup-script"]').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(requests.some((r) => r.key.startsWith('DELETE'))).toBe(false)
    expect(wrapper.find('button[aria-label="Revoke backup-script"]').exists()).toBe(true)
  })

  it('names the User and their pages in the sidebar', async () => {
    const { wrapper } = await mountAt('/profile/tokens', answers([]))
    const nav = wrapper.get('nav')
    expect(nav.get('[data-who]').text()).toBe('ALAda Lovelaceada@example.org')
    expect(nav.findAll('a').map((a) => a.text())).toEqual(['Apps', 'Profile', 'Security', 'Tokens'])
    expect(nav.get('[aria-current="page"]').text()).toBe('Tokens')
  })

  it('gives an Admin a way to their dashboard', async () => {
    const { wrapper } = await mountAt('/profile/tokens', { ...answers([]), 'GET /api/v1/profile': me(true) })
    expect(wrapper.get('nav').findAll('a').map((a) => a.text())).toContain('Dashboard')
  })
})

describe('the Launcher', () => {
  const signedIn = (tokens: PersonalToken[]) => ({
    'GET /api/v1/launcher': () => Response.json({ apps: [] }),
    'GET /api/v1/profile': () => me(),
    'GET /api/v1/version': () => Response.json({ version: '0.4.0' }),
    'GET /api/v1/profile/tokens': () => Response.json({ tokens, clients: [] }),
  })

  it('warns of one token ending soon, by name', async () => {
    const { wrapper } = await mountAt('/', signedIn([token(), backup]))
    expect(wrapper.get('[role="status"]').text()).toMatch(/^Your token grafana-cli expires in \d days\. Make a new one on the Tokens page\.$/)
    expect(wrapper.get('[role="status"] a').attributes('href')).toBe('/profile/tokens')
  })

  it('warns of several tokens ending soon, by count', async () => {
    const { wrapper } = await mountAt('/', signedIn([token(), token({ id: '00000000-0000-4000-8000-0000000000c3', name: 'deploy' })]))
    expect(wrapper.get('[role="status"]').text()).toContain('2 of your tokens expire within 7 days: grafana-cli, deploy.')
  })

  it('says nothing when no token ends soon', async () => {
    const { wrapper } = await mountAt('/', signedIn([backup]))
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('links to the Tokens page', async () => {
    const { wrapper } = await mountAt('/', signedIn([]))
    expect(wrapper.find('nav a[href="/profile/tokens"]').exists()).toBe(true)
  })
})

describe("an Admin's view of a User's tokens", () => {
  const overview = {
    groups: [{ id: '00000000-0000-4000-8000-0000000000b1', name: 'admins', admins: true, members: [] }],
    users: [{ id: ada, email: 'ada@example.org', name: 'Ada' }],
    clients: [],
    assignments: [],
  }
  const answers = (tokens: () => Response) => ({
    'GET /api/v1/profile': me(true),
    'GET /api/v1/access': () => Response.json(overview),
    [`GET /api/v1/access/users/${ada}/tokens`]: tokens,
  })

  async function open(tokens: () => Response) {
    const mounted = await mountAt('/access', answers(tokens))
    await button(mounted.wrapper, 'Tokens').trigger('click')
    await settle()
    return mounted
  }

  it('opens and closes a User’s tokens', async () => {
    const { wrapper } = await open(() => Response.json({ tokens: [backup] }))
    expect(wrapper.get('[role="group"][aria-label="Ada\'s tokens"]').text()).toContain('backup-script · Vault · expires 2 Feb 2027')
    await button(wrapper, 'Hide tokens').trigger('click')
    expect(wrapper.find('[aria-label="Ada\'s tokens"]').exists()).toBe(false)
  })

  it('says when the User has none', async () => {
    const { wrapper } = await open(() => Response.json({ tokens: [] }))
    expect(wrapper.text()).toContain('Ada has no tokens.')
  })

  it('revokes a User’s token after asking once', async () => {
    let revoked = false
    const { wrapper, requests } = await mountAt('/access', {
      ...answers(() => Response.json({ tokens: [revoked ? { ...backup, state: 'revoked' } : backup] })),
      [`DELETE /api/v1/access/users/${ada}/tokens/${backup.id}`]: () => {
        revoked = true
        return ok()
      },
    })
    await button(wrapper, 'Tokens').trigger('click')
    await settle()
    await wrapper.get('button[aria-label="Revoke backup-script"]').trigger('click')
    await button(wrapper, 'Yes, revoke').trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/access/users/${ada}/tokens/${backup.id}`)
    expect(wrapper.find('button[aria-label="Revoke backup-script"]').exists()).toBe(false)
  })

  it('keeps the token when the revoke is cancelled', async () => {
    const { wrapper } = await open(() => Response.json({ tokens: [backup] }))
    await wrapper.get('button[aria-label="Revoke backup-script"]').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('button[aria-label="Revoke backup-script"]').exists()).toBe(true)
  })

  it('offers no revoke for a token that is already over', async () => {
    const { wrapper } = await open(() => Response.json({ tokens: [{ ...backup, state: 'expired' }] }))
    expect(wrapper.find('button[aria-label="Revoke backup-script"]').exists()).toBe(false)
  })

  it('says why the tokens could not be read', async () => {
    const { wrapper } = await open(() => problem(403, 'Only Admins manage other people’s tokens.'))
    expect(wrapper.get('[aria-label="Ada\'s tokens"] [role="alert"]').text()).toBe('Only Admins manage other people’s tokens.')
  })
})
