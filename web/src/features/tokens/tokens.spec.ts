import type { VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { admin, dashboard, ids, overview, profile, token } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import type { PersonalToken } from '../../infrastructure/api/types.gen'
import { expiring, expiry } from './expiry'

const backup = token({
  id: '00000000-0000-4000-8000-0000000000c2',
  name: 'backup-script',
  client: { id: 'vault', label: 'Vault' },
  roles: ['user'],
  expiresAt: '2027-02-02T09:00:00Z',
  state: 'active',
})
const ok = () => new Response(null, { status: 204 })
const grafana = [{ id: 'grafana', label: 'Grafana' }]

function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

const rows = (wrapper: VueWrapper) => wrapper.findAll('.lt-r')

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
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
  const answers = (tokens: PersonalToken[], clients = grafana) => ({
    'GET /api/v1/profile': () => Response.json(profile()),
    'GET /api/v1/profile/tokens': () => Response.json({ tokens, clients }),
  })
  const made = (secret = 'idpat_s3cret') => () => Response.json({ token: token(), secret }, { status: 201 })

  async function make(wrapper: VueWrapper, name = 'grafana-cli') {
    await button(wrapper, 'New token').trigger('click')
    await wrapper.get('#nt-name').setValue(name)
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
  }

  it('lists each token with its app, roles, end and state', async () => {
    const { wrapper } = await mountAt('/tokens', answers([backup]))
    const row = rows(wrapper)[0]
    expect(row?.get('.name').text()).toBe('backup-script')
    expect(row?.get('.app-c').text()).toBe('VVault')
    expect(row?.findAll('.role').map((r) => r.text())).toEqual(['user'])
    expect(row?.get('.muted').text()).toBe('expires 2 Feb 2027')
    expect(row?.get('.state').text()).toBe('Active')
  })

  it('marks a token ending this week', async () => {
    const { wrapper } = await mountAt('/tokens', answers([token()]))
    expect(rows(wrapper)[0]?.get('.state').classes()).toContain('warn')
  })

  it('says a token with no role left has none', async () => {
    const { wrapper } = await mountAt('/tokens', answers([token({ roles: [] })]))
    expect(rows(wrapper)[0]?.get('.roles').text()).toBe('No role')
  })

  it('offers no revoke for a token that is already over', async () => {
    const { wrapper } = await mountAt('/tokens', answers([token({ state: 'revoked' }), token({ id: '00000000-0000-4000-8000-0000000000c9', state: 'expired' })]))
    expect(wrapper.findAll('.lt-r button')).toHaveLength(0)
    expect(rows(wrapper).every((r) => r.classes('off'))).toBe(true)
  })

  it('says when there are no tokens', async () => {
    const { wrapper } = await mountAt('/tokens', answers([]))
    expect(wrapper.get('.empty p').text()).toBe('No tokens yet.')
    expect(wrapper.find('.empty .soft').exists()).toBe(false)
  })

  it('explains that a token needs an app, when the User has none', async () => {
    const { wrapper } = await mountAt('/tokens', answers([], []))
    expect(wrapper.get('.empty .soft').text()).toBe('A token opens one app you have a role in. You have none yet.')
    expect(button(wrapper, 'New token').attributes('disabled')).toBeDefined()
  })

  it('makes a token for the chosen app and length, and shows its secret once', async () => {
    let minted = false
    const { wrapper, requests } = await mountAt('/tokens', {
      ...answers([], [...grafana, { id: 'vault', label: 'Vault' }]),
      'GET /api/v1/profile/tokens': () => Response.json({ tokens: minted ? [token()] : [], clients: [...grafana, { id: 'vault', label: 'Vault' }] }),
      'POST /api/v1/profile/tokens': () => {
        minted = true
        return made()()
      },
    })
    await button(wrapper, 'New token').trigger('click')
    await wrapper.get('#nt-name').setValue('grafana-cli')
    await wrapper.get('.dd-btn[aria-label="App"]').trigger('click')
    await wrapper.findAll('[role="option"]').find((o) => o.text() === 'Vault')?.trigger('click')
    await button(wrapper, '30 days').trigger('click')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/tokens')?.body).toEqual({ name: 'grafana-cli', clientId: 'vault', days: 30 })
    expect(wrapper.get('#sc-h').text()).toBe('Your new token')
    expect(wrapper.get('.secret code').text()).toBe('idpat_s3cret')
    expect(rows(wrapper)).toHaveLength(1)
    await button(wrapper, 'Done').trigger('click')
    expect(wrapper.find('.secret').exists()).toBe(false)
  })

  it('starts a new token on the first app and 90 days', async () => {
    const { wrapper, requests } = await mountAt('/tokens', { ...answers([]), 'POST /api/v1/profile/tokens': made() })
    await make(wrapper)
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/tokens')?.body).toEqual({ name: 'grafana-cli', clientId: 'grafana', days: 90 })
  })

  it('copies the secret', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { wrapper } = await mountAt('/tokens', { ...answers([]), 'POST /api/v1/profile/tokens': made() })
    await make(wrapper)
    await button(wrapper, 'Copy').trigger('click')
    await settle()
    expect(writeText).toHaveBeenCalledWith('idpat_s3cret')
    expect(button(wrapper, 'Copied').exists()).toBe(true)
  })

  it('says Copy again a moment after copying', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout'] })
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: () => Promise.resolve() }, configurable: true })
    const { wrapper } = await mountAt('/tokens', { ...answers([]), 'POST /api/v1/profile/tokens': made() })
    await make(wrapper)
    await button(wrapper, 'Copy').trigger('click')
    await settle()
    vi.advanceTimersByTime(1500)
    await settle()
    expect(button(wrapper, 'Copy').exists()).toBe(true)
    vi.useRealTimers()
  })

  it('leaves the secret to be copied by hand when the browser refuses', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: () => Promise.reject(new Error('denied')) }, configurable: true })
    const { wrapper } = await mountAt('/tokens', { ...answers([]), 'POST /api/v1/profile/tokens': made() })
    await make(wrapper)
    await button(wrapper, 'Copy').trigger('click')
    await settle()
    expect(button(wrapper, 'Copy').exists()).toBe(true)
    expect(wrapper.get('.secret code').text()).toBe('idpat_s3cret')
  })

  it('says why a token was not made', async () => {
    const { wrapper } = await mountAt('/tokens', {
      ...answers([]),
      'POST /api/v1/profile/tokens': problem(422, 'You have no role in that app, so a token for it could open nothing.'),
    })
    await make(wrapper)
    expect(wrapper.get('[role="alert"]').text()).toBe('You have no role in that app, so a token for it could open nothing.')
  })

  it('puts the form away on cancel', async () => {
    const { wrapper, requests } = await mountAt('/tokens', answers([]))
    await button(wrapper, 'New token').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('form[role="dialog"]').exists()).toBe(false)
    expect(requests.some((r) => r.key.startsWith('POST'))).toBe(false)
  })

  it('revokes a token after asking once', async () => {
    let revoked = false
    const { wrapper, requests } = await mountAt('/tokens', {
      ...answers([]),
      'GET /api/v1/profile/tokens': () => Response.json({ tokens: [revoked ? { ...backup, state: 'revoked' } : backup], clients: [] }),
      [`DELETE /api/v1/profile/tokens/${backup.id}`]: () => {
        revoked = true
        return ok()
      },
    })
    await wrapper.get('button[aria-label="Revoke backup-script"]').trigger('click')
    expect(wrapper.get('.lt-confirm span').text()).toBe('Revoke backup-script? Scripts using it stop working.')
    await wrapper.get('.lt-confirm .danger-solid').trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/profile/tokens/${backup.id}`)
    expect(rows(wrapper)[0]?.get('.state').text()).toBe('Revoked')
  })

  it('keeps a token when the revoke is cancelled', async () => {
    const { wrapper, requests } = await mountAt('/tokens', answers([backup]))
    await wrapper.get('button[aria-label="Revoke backup-script"]').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(requests.some((r) => r.key.startsWith('DELETE'))).toBe(false)
    expect(wrapper.find('button[aria-label="Revoke backup-script"]').exists()).toBe(true)
  })

  it('shows the problem when the tokens cannot load', async () => {
    const { wrapper } = await mountAt('/tokens', { ...answers([]), 'GET /api/v1/profile/tokens': problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('puts the form away on Escape', async () => {
    const { wrapper } = await mountAt('/tokens', answers([]))
    await button(wrapper, 'New token').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await settle()
    expect(wrapper.find('form[role="dialog"]').exists()).toBe(false)
  })
})

describe("an Admin's view of a User's tokens", () => {
  const answers = (tokens: PersonalToken[]) => ({
    'GET /api/v1/profile': () => Response.json(admin()),
    'GET /api/v1/dashboard': () => Response.json(dashboard()),
    'GET /api/v1/access': () => Response.json(overview()),
    [`GET /api/v1/access/users/${ids.ada}/tokens`]: () => Response.json({ tokens }),
  })
  const at = `/admin/people/${ids.ada}`
  const list = (wrapper: VueWrapper) => wrapper.get('[aria-label="Ada Lovelace\'s tokens"]')

  it("lists the User's tokens", async () => {
    const { wrapper } = await mountAt(at, answers([backup]))
    expect(list(wrapper).get('.ut-row .grow b').text()).toBe('backup-script')
    expect(list(wrapper).get('.ut-row small').text()).toBe('Vault · expires 2 Feb 2027')
  })

  it('says when the User has none', async () => {
    const { wrapper } = await mountAt(at, answers([]))
    expect(list(wrapper).get('.soft').text()).toBe('No tokens')
  })

  it("revokes a User's token after asking once", async () => {
    const { wrapper, requests } = await mountAt(at, { ...answers([backup]), [`DELETE /api/v1/access/users/${ids.ada}/tokens/${backup.id}`]: ok() })
    await list(wrapper).get('button[aria-label="Revoke backup-script"]').trigger('click')
    await list(wrapper).get('.danger-solid').trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/access/users/${ids.ada}/tokens/${backup.id}`)
  })

  it('keeps the token when the revoke is cancelled', async () => {
    const { wrapper, requests } = await mountAt(at, answers([backup]))
    await list(wrapper).get('button[aria-label="Revoke backup-script"]').trigger('click')
    await list(wrapper).findAll('button').find((b) => b.text() === 'Cancel')?.trigger('click')
    expect(requests.some((r) => r.key.startsWith('DELETE'))).toBe(false)
    expect(list(wrapper).find('button[aria-label="Revoke backup-script"]').exists()).toBe(true)
  })

  it('offers no revoke for a token that is already over', async () => {
    const { wrapper } = await mountAt(at, answers([{ ...backup, state: 'revoked' }]))
    expect(list(wrapper).findAll('button')).toHaveLength(0)
  })

  it('says why the tokens could not be read', async () => {
    const { wrapper } = await mountAt(at, { ...answers([]), [`GET /api/v1/access/users/${ids.ada}/tokens`]: problem(500, 'Something went wrong. Try again.') })
    expect(list(wrapper).get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})
