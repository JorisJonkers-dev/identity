import { afterEach, describe, expect, it } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { mountAt, problem, settle } from '../../app/testing'

const root = '00000000-0000-4000-8000-0000000000a1'
const noor = '00000000-0000-4000-8000-0000000000a2'
const hourStart = Date.UTC(2026, 9, 15, 22)

const me = (secondFactor = true) =>
  Response.json({ email: 'joris@example.org', name: 'Joris Jonkers', createdAt: '2026-10-01T09:00:00Z', admin: true, secondFactor })

function dashboard(over: Record<string, unknown> = {}) {
  return Response.json({
    users: 14,
    usersThisWeek: 1,
    unassigned: [{ id: noor, name: 'Noor Bakker', email: 'noor@example.org', confirmedAt: new Date(Date.now() - 2.5 * 3600000).toISOString() }],
    signIns: 186,
    failedSignIns: 4,
    hours: Array.from({ length: 24 }, (_, i) => ({ at: new Date(hourStart + i * 3600000).toISOString(), signIns: i, failed: 0 })),
    activeSessions: 23,
    sessionUsers: 12,
    ...over,
  })
}

const access = () =>
  Response.json({
    groups: [],
    users: [
      { id: root, email: 'joris@example.org', name: 'Joris' },
      { id: noor, email: 'noor@example.org', name: 'Noor Bakker' },
    ],
    clients: [{ id: 'grafana', label: 'Grafana', roles: ['user', 'viewer'] }],
    assignments: [],
  })

const page = (events: unknown[], next?: string) => Response.json(next ? { events, next } : { events })

const gave = {
  id: '00000000-0000-4000-8000-0000000000e1',
  at: '2026-10-16T21:14:02Z',
  kind: 'access',
  action: 'role-assigned',
  actor: { id: root, name: 'Joris', email: 'joris@example.org' },
  client: { id: 'grafana', label: 'Grafana' },
  groupName: 'Family',
  role: 'viewer',
}

function link(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('a').find((a) => a.text() === text)
  if (!found) throw new Error(`no link ${text}`)
  return found
}

function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the dashboard', () => {
  const answers = (over: Record<string, unknown> = {}) => ({
    'GET /api/v1/profile': me(),
    'GET /api/v1/dashboard': () => dashboard(over),
  })

  it('shows the counts of the last day', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    const stats = wrapper.findAll('[aria-label="Counts"] li').map((s) => s.text())
    expect(stats).toEqual([
      'Users14+1 this week',
      'Unassigned1waiting for a Group',
      'Sign-ins1864 failed',
      'Active sessions23across 12 Users',
    ])
    expect(wrapper.get('[data-attention]').text()).toContain('Unassigned')
  })

  it('does not warn when nobody is waiting', async () => {
    const { wrapper } = await mountAt('/admin', answers({ unassigned: [] }))
    expect(wrapper.find('[data-attention]').exists()).toBe(false)
    expect(wrapper.text()).toContain('No Users are waiting for a Group.')
  })

  it('says one User, not one Users', async () => {
    const { wrapper } = await mountAt('/admin', answers({ activeSessions: 1, sessionUsers: 1 }))
    expect(wrapper.text()).toContain('across 1 User')
    expect(wrapper.text()).not.toContain('across 1 Users')
  })

  it('draws the sign-ins per hour, described for a screen reader', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    const chart = wrapper.get('svg[role="img"]')
    expect(chart.attributes('aria-label')).toBe(`276 sign-ins in the last 24 hours. 0 failed sign-ins. The busiest hour had 23 sign-ins, around ${chartHour(23)}.`)
    expect(chart.get('[data-test="sign-ins"]').attributes('points')?.split(' ')).toHaveLength(24)
    expect(chart.findAll('text').map((t) => t.text()).slice(0, 3)).toEqual(['50', '25', '0'])
  })

  it('lists who waits for a Group, with a way to add them', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    const row = wrapper.get('[aria-labelledby="queue-title"] li')
    expect(row.text()).toContain('Noor Bakker')
    expect(row.text()).toContain('noor@example.org · confirmed 2h ago')
    expect(row.get('a').attributes('href')).toBe('/access#waiting-title')
    expect(row.get('a').attributes('aria-label')).toBe('Add Noor Bakker to a Group')
  })

  it('shows the newest five in the queue and links to the rest', async () => {
    const waiting = Array.from({ length: 7 }, (_, i) => ({
      id: `00000000-0000-4000-8000-00000000010${String(i)}`,
      name: `Person ${String(i)}`,
      email: `p${String(i)}@example.org`,
      confirmedAt: '2026-10-16T09:00:00Z',
    }))
    const { wrapper } = await mountAt('/admin', answers({ unassigned: waiting }))
    expect(wrapper.findAll('[aria-labelledby="queue-title"] li')).toHaveLength(5)
    expect(link(wrapper, 'See all 7 on the access page').attributes('href')).toBe('/access#waiting-title')
  })

  it('needs no link to the rest when the whole queue shows', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    expect(wrapper.text()).not.toContain('See all')
  })

  it('links to Grafana only when it is configured', async () => {
    const without = await mountAt('/admin', answers())
    expect(without.wrapper.text()).not.toContain('Open in Grafana')
    expect(without.wrapper.get('[data-test="subtitle"]').text()).toBe('Last 24 hours.')
    document.body.innerHTML = ''
    const { wrapper } = await mountAt('/admin', answers({ grafanaUrl: 'https://grafana.example.org/d/identity' }))
    expect(link(wrapper, 'Open in Grafana ↗').attributes('href')).toBe('https://grafana.example.org/d/identity')
    expect(wrapper.get('[data-test="subtitle"]').text()).toBe('Last 24 hours. Charts over time are in Grafana.')
  })

  it('says loading before the counts arrive', async () => {
    const { wrapper } = await mountAt('/admin', {
      'GET /api/v1/profile': me(),
      'GET /api/v1/dashboard': () => new Promise<Response>(() => undefined),
    })
    expect(wrapper.text()).toContain('Loading…')
  })

  it('tells a non-Admin it is not theirs', async () => {
    const { wrapper } = await mountAt('/admin', {
      'GET /api/v1/profile': me(),
      'GET /api/v1/dashboard': problem(403, 'Only Admins can see this page.'),
    })
    expect(wrapper.get('[role="alert"]').text()).toBe('Only Admins can see this page.')
  })

  it('sends an Admin with no Second Factor to add one', async () => {
    const { router } = await mountAt('/admin', { 'GET /api/v1/profile': me(false), 'GET /api/v1/dashboard': () => dashboard() })
    expect(router.currentRoute.value.fullPath).toBe('/security?for=admin')
  })

  it('names the Admin and counts the queue in the sidebar', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    const side = wrapper.get('nav[aria-label="Admin"]')
    expect(side.get('[data-who]').text()).toBe('JJJoris JonkersAdmin · 12h session')
    expect(side.get('[aria-current="page"]').text()).toBe('Dashboard')
    expect(side.get('[data-count]').text()).toBe('1')
  })

  it('leaves the count out of the sidebar when nobody waits', async () => {
    const { wrapper } = await mountAt('/admin', answers({ unassigned: [] }))
    expect(wrapper.find('nav [data-count]').exists()).toBe(false)
  })

  it('fills the window instead of the narrow column the other pages use', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    expect(wrapper.get('main').classes()).toEqual(['wide'])
  })
})

function chartHour(i: number): string {
  return new Date(hourStart + i * 3600000).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false })
}

describe('the audit page', () => {
  const answers = (events: () => Response) => ({
    'GET /api/v1/profile': me(),
    'GET /api/v1/dashboard': () => dashboard(),
    'GET /api/v1/access': access,
    'GET /api/v1/audit': events,
  })

  it('reads each event as a sentence, with its time and kind', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([gave])))
    const row = wrapper.get('[data-event]')
    expect(row.get('time').attributes('datetime')).toBe('2026-10-16T21:14:02Z')
    expect(row.get('p').text()).toBe('Joris gave Family viewer on Grafana')
    expect(row.get('p b').text()).toBe('Joris')
    expect(row.get('[data-role]').text()).toBe('viewer')
    expect(row.get('[data-tone]').attributes('data-tone')).toBe('a')
    expect(row.get('[data-kind]').text()).toBe('access')
  })

  it('heads each day of events once', async () => {
    const earlier = { ...gave, id: '00000000-0000-4000-8000-0000000000e2', at: '2026-10-16T20:00:00Z' }
    const older = { ...gave, id: '00000000-0000-4000-8000-0000000000e3', at: '2026-10-01T20:00:00Z' }
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([gave, earlier, older])))
    expect(wrapper.findAll('[data-day]').map((d) => d.text())).toEqual([expect.any(String), '1 Oct'])
  })

  it('says when no event matches', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([])))
    expect(wrapper.text()).toContain('No Audit Events match.')
  })

  it('narrows to a kind, and back, through the address', async () => {
    const { wrapper, router, requests } = await mountAt('/admin/audit', answers(() => page([gave])))
    const access = () => wrapper.get('button[aria-pressed]:nth-of-type(3)')
    expect(access().text()).toBe('access')
    await access().trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/admin/audit?kind=access')
    expect(access().attributes('aria-pressed')).toBe('true')
    expect(requests.filter((r) => r.key === 'GET /api/v1/audit')).toHaveLength(2)
    await access().trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/admin/audit')
  })

  it('narrows to a User and to an app', async () => {
    const { wrapper, router } = await mountAt('/admin/audit', answers(() => page([gave])))
    const [users, clients] = wrapper.findAll('select')
    expect(users?.findAll('option').map((o) => o.text())).toEqual(['All Users', 'Joris', 'Noor Bakker'])
    await users?.setValue(noor)
    await settle()
    await clients?.setValue('grafana')
    await settle()
    expect(router.currentRoute.value.query).toEqual({ user: noor, client: 'grafana' })
  })

  it('takes a filter from the address and ignores a kind there is not', async () => {
    const { wrapper } = await mountAt('/admin/audit?kind=teleports&user=' + noor, answers(() => page([gave])))
    expect(wrapper.find('button[aria-pressed="true"]').exists()).toBe(false)
    expect(wrapper.get<HTMLSelectElement>('select').element.value).toBe(noor)
  })

  it('reads older events a page at a time', async () => {
    let pages = 0
    const { wrapper } = await mountAt(
      '/admin/audit',
      answers(() => {
        pages++
        return pages === 1 ? page([gave], 'cursor-1') : page([{ ...gave, id: '00000000-0000-4000-8000-0000000000e9' }])
      }),
    )
    await button(wrapper, 'Show older Audit Events').trigger('click')
    await settle()
    expect(wrapper.findAll('[data-event]')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('Show older Audit Events')
  })

  it('says loading before the events arrive', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => new Promise<Response>(() => undefined) as unknown as Response))
    expect(wrapper.text()).toContain('Loading…')
  })

  it('tells a non-Admin it is not theirs', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => problem(403, 'Only Admins can see this page.')))
    expect(wrapper.get('[role="alert"]').text()).toBe('Only Admins can see this page.')
  })

  it('marks the Audit link as the page open', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([])))
    expect(wrapper.get('nav [aria-current="page"]').text()).toBe('Audit')
  })
})
