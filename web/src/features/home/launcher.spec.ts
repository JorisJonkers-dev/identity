import { afterEach, describe, expect, it } from 'vitest'
import { mountAt, problem } from '../../app/testing'

const grafana = {
  id: 'grafana',
  label: 'Grafana',
  description: 'Metrics and alerts',
  launchUrl: 'https://grafana.test/',
  iconUrl: '/icons/grafana?v=3f2a9c1b7d4e',
  roles: ['editor', 'viewer'],
}
const jellyfin = { id: 'jellyfin', label: 'Jellyfin', launchUrl: 'https://jellyfin.test/web', roles: ['user'] }

const profile = (admin: boolean) =>
  Response.json({ email: 'ada@example.org', name: 'Ada', admin, secondFactor: false, createdAt: '2026-10-01T09:00:00Z' })

function signedIn(apps: unknown[], admin = false) {
  return {
    'GET /api/v1/launcher': () => Response.json({ apps }),
    'GET /api/v1/profile': () => profile(admin),
    'GET /api/v1/version': () => Response.json({ version: '0.4.0' }),
  }
}

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
    expect(wrapper.find('a[href="/signin"]').exists()).toBe(true)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})

describe('the Launcher', () => {
  it('shows one tile per app the User can open', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana, jellyfin]))
    expect(wrapper.get('h1').text()).toBe('Your apps')
    expect(wrapper.findAll('li.app').map((li) => li.get('.label').text())).toEqual(['Grafana', 'Jellyfin'])
  })

  it('opens an app at its launch URL', async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin]))
    expect(wrapper.get('li.app a').attributes('href')).toBe('https://jellyfin.test/web')
  })

  it("shows an app's description and the User's access to it", async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    expect(wrapper.get('li.app .description').text()).toBe('Metrics and alerts')
    expect(wrapper.get('li.app .roles').text()).toBe('Your access: editor, viewer')
  })

  it("shows an app's icon from identity's own origin", async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    expect(wrapper.get('li.app img').attributes('src')).toBe('/icons/grafana?v=3f2a9c1b7d4e')
    expect(wrapper.find('li.app .letter').exists()).toBe(false)
  })

  it("shows the label's first letter for an app without an icon", async () => {
    const { wrapper } = await mountAt('/', signedIn([jellyfin]))
    expect(wrapper.find('li.app img').exists()).toBe(false)
    expect(wrapper.get('li.app .letter').text()).toBe('J')
  })

  it("shows the label's first letter when an icon fails to load", async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    await wrapper.get('li.app img').trigger('error')
    expect(wrapper.find('li.app img').exists()).toBe(false)
    expect(wrapper.get('li.app .letter').text()).toBe('G')
  })

  it('tells a User with no apps how to get some', async () => {
    const { wrapper } = await mountAt('/', signedIn([]))
    expect(wrapper.text()).toContain("You don't have access to any apps yet. Ask an Admin to add you to a Group.")
    expect(wrapper.find('li.app').exists()).toBe(false)
  })

  it('links to the Profile', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana]))
    expect(wrapper.get('nav a[href="/profile"]').text()).toBe('Your profile')
  })

  it('links an Admin to the access page', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana], true))
    expect(wrapper.get('nav a[href="/access"]').text()).toBe('Manage access')
  })

  it('does not link anyone else to the access page', async () => {
    const { wrapper } = await mountAt('/', signedIn([grafana], false))
    expect(wrapper.find('nav a[href="/access"]').exists()).toBe(false)
  })

  it('shows the problem when the apps cannot load', async () => {
    const { wrapper } = await mountAt('/', {
      'GET /api/v1/launcher': problem(500, 'Something went wrong. Try again.'),
      'GET /api/v1/version': Response.json({ version: '0.4.0' }),
    })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})
