import { afterEach, describe, expect, it } from 'vitest'
import { admin, dashboard, profile, token } from '../../app/fixtures'
import { mountAt, problem } from '../../app/testing'
import type { PersonalToken } from '../../infrastructure/api/types.gen'

const grafana = {
  id: 'grafana',
  label: 'Grafana',
  description: 'Metrics and alerts',
  launchUrl: 'https://grafana.test/',
  iconUrl: '/icons/grafana?v=3f2a9c1b7d4e',
  roles: ['editor', 'viewer'],
}
const jellyfin = { id: 'jellyfin', label: 'Jellyfin', launchUrl: 'https://jellyfin.test/web', roles: ['user'] }
const oldWiki = { id: 'old-wiki', label: 'Old wiki', launchUrl: 'https://wiki.test/', roles: ['reader'] }

function signedIn(apps: unknown[], tokens: PersonalToken[] = [], asAdmin = false, heldBack = 0) {
  return {
    'GET /api/v1/launcher': () => Response.json({ apps, heldBack }),
    'GET /api/v1/profile': () => Response.json(asAdmin ? admin() : profile()),
    'GET /api/v1/version': () => Response.json({ version: '0.4.0' }),
    'GET /api/v1/profile/tokens': () => Response.json({ tokens, clients: [] }),
    'GET /api/v1/dashboard': () => Response.json(dashboard()),
  }
}

const names = (wrapper: Awaited<ReturnType<typeof mountAt>>['wrapper']) => wrapper.findAll('a.app > div > b').map((b) => b.text())

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the home page, signed out', () => {
  it('offers to sign in or sign up', async () => {
    const { wrapper } = await mountAt('/', {
      'GET /api/v1/launcher': problem(401, 'Sign in first.'),
      'GET /api/v1/version': Response.json({ version: '0.4.0' }),
    })
    expect(wrapper.get('h1').text()).toBe('jorisjonkers.dev')
    expect(wrapper.findAll('.land-acts a').map((a) => [a.text(), a.attributes('href')])).toEqual([
      ['Sign in', '/signin'],
      ['Sign up', '/signup'],
    ])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})

describe('the Launcher', () => {
  it('shows one card per app the User can open', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana, jellyfin]))
    expect(names(wrapper)).toEqual(['Grafana', 'Jellyfin'])
  })

  it('opens an app at its launch URL', async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin]))
    expect(wrapper.get('a.app').attributes('href')).toBe('https://jellyfin.test/web')
  })

  it("shows the User's roles on each app", async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    expect(wrapper.findAll('a.app .role').map((r) => r.text())).toEqual(['editor', 'viewer'])
  })

  it("shows an app's icon from identity's own origin", async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    expect(wrapper.get('a.app img').attributes('src')).toBe('/icons/grafana?v=3f2a9c1b7d4e')
  })

  it("shows the label's first letter for an app without an icon", async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin]))
    expect(wrapper.find('a.app img').exists()).toBe(false)
    expect(wrapper.get('a.app .mono-l').text()).toBe('J')
  })

  it('shows the first letters of a two-word label', async () => {
    const { wrapper } = await mountAt('/', signedIn([oldWiki]))
    expect(wrapper.get('a.app .mono-l').text()).toBe('OW')
  })

  it("shows the label's first letter when an icon fails to load", async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    await wrapper.get('a.app img').trigger('error')
    expect(wrapper.find('a.app img').exists()).toBe(false)
    expect(wrapper.get('a.app .mono-l').text()).toBe('G')
  })

  it('tells a User with no apps who to ask', async () => {
    const { wrapper } = await mountAt('/', signedIn([]))
    expect(wrapper.get('.apps-empty p').text()).toBe('No apps yet. An Admin adds you to a Group.')
    expect(wrapper.get('.apps-empty a').attributes('href')).toBe('mailto:admin@jorisjonkers.dev')
  })

  it('finds an app by its name or description', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana, jellyfin]))
    await wrapper.get('input[type="search"]').setValue('alerts')
    expect(names(wrapper)).toEqual(['Grafana'])
    await wrapper.get('input[type="search"]').setValue('JELLY')
    expect(names(wrapper)).toEqual(['Jellyfin'])
  })

  it('says when no app matches', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana, jellyfin]))
    await wrapper.get('input[type="search"]').setValue('overleaf')
    expect(wrapper.get('p.soft').text()).toBe('No app matches “overleaf”.')
    expect(wrapper.find('a.app').exists()).toBe(false)
  })

  it('puts the cursor in the search when the User types /', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]), { attach: true })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: '/' }))
    expect(document.activeElement).toBe(wrapper.get('input[type="search"]').element)
  })

  it('leaves a / typed into a field alone', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    const other = document.createElement('input')
    document.body.appendChild(other)
    other.focus()
    const slash = new KeyboardEvent('keydown', { key: '/', cancelable: true })
    document.dispatchEvent(slash)
    expect(slash.defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(other)
    wrapper.unmount()
  })

  it('stops listening for / once the page is gone', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    wrapper.unmount()
    const slash = new KeyboardEvent('keydown', { key: '/', cancelable: true })
    document.dispatchEvent(slash)
    expect(slash.defaultPrevented).toBe(false)
  })

  it('shows the running version', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    expect(wrapper.get('footer').text()).toBe('Version 0.4.0')
  })

  it('shows the problem when the apps cannot load', async () => {
    const { wrapper } = await mountAt('/', {
      'GET /api/v1/launcher': problem(500, 'Something went wrong. Try again.'),
      'GET /api/v1/version': Response.json({ version: '0.4.0' }),
    })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})

describe('the expiring tokens strip', () => {
  it('names the one token that expires this week', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana], [token({ name: 'backup' })]))
    expect(wrapper.get('.strip b').text()).toBe('backup')
    expect(wrapper.get('.strip a').attributes('href')).toBe('/tokens')
  })

  it('counts several tokens that expire this week', async () => {
    const tokens = [token({ name: 'backup' }), token({ id: '00000000-0000-4000-8000-0000000000c2', name: 'ci' }), token({ id: '00000000-0000-4000-8000-0000000000c3', name: 'later', state: 'active', expiresAt: '2027-02-02T09:00:00Z' })]
    const { wrapper } = await mountAt('/', signedIn([grafana], tokens))
    expect(wrapper.get('.strip').text()).toContain('2 tokens expire within 7 days.')
  })

  it('stays away while no token expires this week', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana], [token({ state: 'active', expiresAt: '2027-02-02T09:00:00Z' })]))
    expect(wrapper.find('.strip').exists()).toBe(false)
  })
})

describe('the Launcher, for someone a Two-factor Group holds back', () => {
  it('says how many apps two-factor gives back, and links to set it up', async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin], [], false, 2))
    expect(wrapper.get('.note.warn').text()).toBe('Add two-factor to open 2 more apps.Set up two-factor')
    expect(wrapper.get('.note.warn a').attributes('href')).toBe('/security/two-factor')
  })

  it('says it for one app', async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin], [], false, 1))
    expect(wrapper.get('.note.warn').text()).toBe('Add two-factor to open 1 more app.Set up two-factor')
  })

  it('says nothing once nothing is held back', async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin]))
    expect(wrapper.find('.note.warn').exists()).toBe(false)
  })
})
