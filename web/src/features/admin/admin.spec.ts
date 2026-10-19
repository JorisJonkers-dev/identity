import type { VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { admin, dashboard, ids, overview } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import type { AuditEvent, Dashboard } from '../../infrastructure/api/types.gen'

const page = (events: unknown[], next?: string) => Response.json(next ? { events, next } : { events })

const gave: AuditEvent = {
  id: '00000000-0000-4000-8000-0000000000e1',
  at: '2026-10-16T21:14:02Z',
  kind: 'access',
  action: 'role-assigned',
  actor: { id: ids.joris, name: 'Joris', email: 'joris@example.org' },
  client: { id: 'grafana', label: 'Grafana' },
  groupName: 'Family',
  role: 'viewer',
}
const failed: AuditEvent = { id: '00000000-0000-4000-8000-0000000000e4', at: '2026-10-16T21:10:00Z', kind: 'sign-in', action: 'sign-in-failed', email: 'eve@example.org', reason: 'password' }

function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the dashboard', () => {
  const answers = (over: Partial<Dashboard> = {}) => ({
    'GET /api/v1/profile': () => Response.json(admin()),
    'GET /api/v1/dashboard': () => Response.json(dashboard(over)),
  })
  const stat = (wrapper: VueWrapper, label: string) => {
    const found = wrapper.findAll('.stat').find((s) => s.get('.lbl').text() === label)
    if (!found) throw new Error(`no stat ${label}`)
    return found
  }

  it('shows the counts of the last day, each linking to where it comes from', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    expect(wrapper.findAll('.stat').map((s) => [s.get('.lbl').text(), s.get('.num').text(), s.attributes('href')])).toEqual([
      ['People', '14', '/admin/people'],
      ['Unassigned', '1', '/admin/unassigned'],
      ['Sign-ins', '186', '/admin/audit'],
      ['Active sessions', '23', '/admin/people'],
      ['Tokens expiring', '3', '/admin/people'],
    ])
    expect(stat(wrapper, 'People').get('.meta').text()).toBe('+1 this week')
    expect(stat(wrapper, 'Sign-ins').get('.meta').text()).toBe('4 failed')
    expect(stat(wrapper, 'Active sessions').get('.meta').text()).toBe('12 people')
  })

  it('says how long the newest has waited for a Group', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    expect(stat(wrapper, 'Unassigned').get('.meta').text()).toBe('newest 2h ago')
    expect(stat(wrapper, 'Unassigned').find('.dot').exists()).toBe(true)
  })

  it('does not warn when nobody is waiting', async () => {
    const { wrapper } = await mountAt('/admin', answers({ unassigned: [], tokensExpiring: 0 }))
    expect(stat(wrapper, 'Unassigned').get('.meta').text()).toBe('none waiting')
    expect(wrapper.findAll('.stat .dot')).toHaveLength(0)
    expect(wrapper.get('.quiet-line').text()).toBe('No one waiting.')
  })

  it('says one person, not one people', async () => {
    const { wrapper } = await mountAt('/admin', answers({ sessionUsers: 1 }))
    expect(stat(wrapper, 'Active sessions').get('.meta').text()).toBe('1 person')
  })

  it('draws the successful and the failed sign-ins apart, each with its peak', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    const [ok, bad] = wrapper.findAll('.chart-card')
    expect(ok?.get('.chart-h b').text()).toBe('182')
    expect(ok?.get('.chart-h small').text()).toMatch(/^peak 23 at \d\d:00$/)
    expect(bad?.get('.chart-h b').text()).toBe('4')
    expect(bad?.get('.chart-h small').text()).toMatch(/^peak 3 at \d\d:00$/)
    expect(bad?.get('svg').attributes('aria-label')).toMatch(/^Failed sign-ins per hour over the last 24 hours, peak 3 at \d\d:00\.$/)
  })

  it('says none for a chart with nothing in it', async () => {
    const { wrapper } = await mountAt('/admin', answers({ failedSignIns: 0, hours: dashboard().hours.map((h) => ({ ...h, failed: 0 })) as Dashboard['hours'] }))
    expect(wrapper.findAll('.chart-card')[1]?.get('.chart-h small').text()).toBe('none')
  })

  it('lists who waits for a Group, each linking to their page', async () => {
    const { wrapper } = await mountAt('/admin', answers())
    const q = wrapper.get('.queue .q')
    expect(q.attributes('href')).toBe(`/admin/people/${ids.noor}`)
    expect(q.get('b').text()).toBe('Noor Bakker')
    expect(q.get('small').text()).toBe('noor@example.org · 2h ago')
    expect(wrapper.get('.un-panel a.btn').attributes('href')).toBe('/admin/unassigned')
  })

  it('shows six of a longer queue', async () => {
    const waiting = Array.from({ length: 8 }, (_, i) => ({ id: `00000000-0000-4000-8000-00000000010${String(i)}`, name: `Person ${String(i)}`, email: `p${String(i)}@example.org`, confirmedAt: '2026-10-16T09:00:00Z' }))
    const { wrapper } = await mountAt('/admin', answers({ unassigned: waiting }))
    expect(wrapper.findAll('.queue .q')).toHaveLength(6)
    expect(wrapper.get('.un-panel .count').text()).toBe('8')
  })

  it('links to Grafana only when it is configured', async () => {
    const without = await mountAt('/admin', answers())
    expect(without.wrapper.findAll('a').some((a) => a.text() === 'Grafana')).toBe(false)
    const withIt = await mountAt('/admin', answers({ grafanaUrl: 'https://grafana.test/d/identity' }))
    expect(withIt.wrapper.findAll('a').find((a) => a.text() === 'Grafana')?.attributes('href')).toBe('https://grafana.test/d/identity')
  })

  it('shows the problem when the counts cannot load', async () => {
    const { wrapper } = await mountAt('/admin', { ...answers(), 'GET /api/v1/dashboard': problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})

describe('the Audit page', () => {
  const answers = (events: () => Response) => ({
    'GET /api/v1/profile': () => Response.json(admin()),
    'GET /api/v1/dashboard': () => Response.json(dashboard()),
    'GET /api/v1/access': () => Response.json(overview()),
    'GET /api/v1/audit': events,
  })
  const pick = async (wrapper: VueWrapper, label: string, option: string) => {
    await wrapper.get(`.dd-btn[aria-label="${label}"]`).trigger('click')
    await wrapper
      .get(`.dd-menu[aria-label="${label}"]`)
      .findAll('[role="option"]')
      .find((o) => o.text() === option)
      ?.trigger('click')
    await settle()
  }

  it('reads each event as a sentence, with its time and kind', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([gave])))
    const row = wrapper.get('.ev')
    expect(row.get('time').attributes('datetime')).toBe('2026-10-16T21:14:02Z')
    expect(row.get('p').text()).toBe('Joris gave Family viewer on Grafana')
    expect(row.get('p b').text()).toBe('Joris')
    expect(row.get('p code').text()).toBe('viewer')
    expect(row.get('.tag').text()).toBe('Access')
  })

  it('marks a failed sign-in', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([failed])))
    expect(wrapper.get('.ev').classes()).toContain('fail')
    expect(wrapper.get('.ev .tag').classes()).toContain('bad')
  })

  it('heads each day of events once', async () => {
    const earlier = { ...gave, id: '00000000-0000-4000-8000-0000000000e2', at: '2026-10-16T20:00:00Z' }
    const older = { ...gave, id: '00000000-0000-4000-8000-0000000000e3', at: '2026-10-01T20:00:00Z' }
    const { wrapper } = await mountAt('/admin/audit', answers(() => page([gave, earlier, older])))
    expect(wrapper.findAll('.day h2').map((d) => d.text())).toEqual([expect.any(String), '1 Oct'])
  })

  it('says when no event matches, and clears the filters', async () => {
    const { wrapper, router } = await mountAt('/admin/audit?kind=tokens', answers(() => page([])))
    expect(wrapper.get('.empty p').text()).toBe('No events match.')
    await button(wrapper, 'Clear filters').trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/admin/audit')
  })

  it('narrows to a kind, and back, through the address', async () => {
    const { wrapper, router, requests } = await mountAt('/admin/audit', answers(() => page([gave])))
    await button(wrapper, 'Access').trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/admin/audit?kind=access')
    expect(button(wrapper, 'Access').attributes('aria-pressed')).toBe('true')
    expect(requests.filter((r) => r.key === 'GET /api/v1/audit')).toHaveLength(2)
    await button(wrapper, 'All').trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/admin/audit')
  })

  it('narrows to a person and to an app', async () => {
    const { wrapper, router } = await mountAt('/admin/audit', answers(() => page([gave])))
    expect(wrapper.get('.dd-menu[aria-label="Person"]').findAll('[role="option"]').map((o) => o.text())).toEqual(['Everyone', 'Joris Jonkers', 'Ada Lovelace', 'Noor Bakker'])
    await pick(wrapper, 'Person', 'Noor Bakker')
    await pick(wrapper, 'App', 'Grafana')
    expect(router.currentRoute.value.query).toEqual({ user: ids.noor, client: 'grafana' })
    expect(wrapper.get('.dd-btn[aria-label="Person"]').text()).toBe('Noor Bakker')
  })

  it('takes a filter from the address and ignores a kind there is not', async () => {
    const { wrapper } = await mountAt(`/admin/audit?kind=teleports&user=${ids.noor}`, answers(() => page([gave])))
    expect(button(wrapper, 'All').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('.dd-btn[aria-label="Person"]').text()).toBe('Noor Bakker')
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
    await button(wrapper, 'Load more').trigger('click')
    await settle()
    expect(wrapper.findAll('.ev')).toHaveLength(2)
    expect(wrapper.findAll('button').some((b) => b.text() === 'Load more')).toBe(false)
  })

  it('shows the problem when the events cannot load', async () => {
    const { wrapper } = await mountAt('/admin/audit', answers(() => problem(500, 'Something went wrong. Try again.')))
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('leaves the person blank when the address names someone it does not know', async () => {
    const { wrapper } = await mountAt('/admin/audit?user=00000000-0000-4000-8000-0000000000ff', answers(() => page([])))
    expect(wrapper.get('.dd-btn[aria-label="Person"] .dd-l').text()).toBe('')
  })
})
