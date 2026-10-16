import { afterEach, describe, expect, it } from 'vitest'
import { mountAt, problem, settle } from '../../app/testing'

const admins = '00000000-0000-4000-8000-0000000000b1'
const family = '00000000-0000-4000-8000-0000000000b2'
const root = '00000000-0000-4000-8000-0000000000a1'
const ada = '00000000-0000-4000-8000-0000000000a2'

function overview(assignments: { groupId: string; clientId: string; role: string }[] = []) {
  return {
    groups: [
      { id: admins, name: 'admins', admins: true, members: [root] },
      { id: family, name: 'Family', admins: false, members: [] },
    ],
    users: [
      { id: root, email: 'root@example.org', name: 'Root' },
      { id: ada, email: 'ada@example.org', name: 'Ada' },
    ],
    clients: [{ id: 'grafana', label: 'Grafana', roles: ['user', 'editor'] }],
    assignments,
  }
}

const ok = () => new Response(null, { status: 204 })

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the access page', () => {
  it('says loading before the overview arrives', async () => {
    const { wrapper } = await mountAt('/access', { 'GET /api/v1/access': () => new Promise<Response>(() => undefined) })
    expect(wrapper.text()).toContain('Loading…')
  })

  it('tells a non-Admin it is not theirs', async () => {
    const { wrapper } = await mountAt('/access', { 'GET /api/v1/access': problem(403, 'Only Admins manage access.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Only Admins manage access.')
  })

  it('shows each Group against each Client, without the admins Group', async () => {
    const { wrapper } = await mountAt('/access', {
      'GET /api/v1/access': Response.json(overview([{ groupId: family, clientId: 'grafana', role: 'editor' }])),
    })
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(1)
    expect(rows[0]?.get('th').text()).toBe('Family')
    expect(wrapper.get<HTMLInputElement>('input[aria-label="Family: editor on Grafana"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('input[aria-label="Family: user on Grafana"]').element.checked).toBe(false)
    expect(wrapper.get('article[aria-label="admins"]').text()).toContain('Root (root@example.org)')
    expect(wrapper.find('article[aria-label="admins"] button[type="button"]').text()).toBe('Remove')
  })

  it('says when there are no Clients', async () => {
    const { wrapper } = await mountAt('/access', { 'GET /api/v1/access': Response.json({ ...overview(), clients: [] }) })
    expect(wrapper.text()).toContain('No Clients are in the cluster yet.')
    expect(wrapper.find('table').exists()).toBe(false)
  })

  it('gives and takes an App Role, and reads the overview again', async () => {
    let held = false
    const { wrapper, requests } = await mountAt('/access', {
      'GET /api/v1/access': () => Response.json(overview(held ? [{ groupId: family, clientId: 'grafana', role: 'user' }] : [])),
      [`PUT /api/v1/access/groups/${family}/roles/grafana/user`]: () => {
        held = true
        return ok()
      },
      [`DELETE /api/v1/access/groups/${family}/roles/grafana/user`]: () => {
        held = false
        return ok()
      },
    })
    const box = () => wrapper.get<HTMLInputElement>('input[aria-label="Family: user on Grafana"]')
    await box().setValue(true)
    await settle()
    expect(requests.map((r) => r.key)).toContain(`PUT /api/v1/access/groups/${family}/roles/grafana/user`)
    expect(box().element.checked).toBe(true)
    await box().setValue(false)
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/access/groups/${family}/roles/grafana/user`)
    expect(box().element.checked).toBe(false)
  })

  it('makes, renames and deletes a Group', async () => {
    const { wrapper, requests } = await mountAt('/access', {
      'GET /api/v1/access': () => Response.json(overview()),
      'POST /api/v1/access/groups': () => Response.json({ id: family, name: 'Cousins', admins: false, members: [] }, { status: 201 }),
      [`PATCH /api/v1/access/groups/${family}`]: ok,
      [`DELETE /api/v1/access/groups/${family}`]: ok,
    })
    const name = wrapper.get<HTMLInputElement>('#new-group')
    await name.setValue('Cousins')
    await wrapper.findAll('form')[0]?.trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/access/groups')?.body).toEqual({ name: 'Cousins' })
    expect(name.element.value).toBe('')

    await wrapper.get(`#rename-${family}`).setValue('Kin')
    await wrapper.get(`article[aria-label="Family"] form`).trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === `PATCH /api/v1/access/groups/${family}`)?.body).toEqual({ name: 'Kin' })

    expect(wrapper.find('article[aria-label="admins"]').text()).not.toContain('Delete')
    await wrapper.get('article[aria-label="Family"] > button').trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/access/groups/${family}`)
  })

  it('sends no rename when nothing was typed', async () => {
    const { wrapper, requests } = await mountAt('/access', { 'GET /api/v1/access': () => Response.json(overview()) })
    await wrapper.get(`article[aria-label="Family"] form`).trigger('submit')
    await settle()
    expect(requests.some((r) => r.key.startsWith('PATCH'))).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  })

  it('adds and removes members, offering only those not in the Group', async () => {
    const { wrapper, requests } = await mountAt('/access', {
      'GET /api/v1/access': () => Response.json(overview()),
      [`PUT /api/v1/access/groups/${family}/members/${ada}`]: ok,
      [`DELETE /api/v1/access/groups/${admins}/members/${root}`]: problem(422, 'The last Admin cannot leave the admins Group.'),
    })
    const options = wrapper.findAll(`#add-${admins} option`).map((o) => o.text())
    expect(options).toEqual(['Ada (ada@example.org)'])

    const addForm = () => wrapper.findAll('article[aria-label="Family"] form')[1]
    await addForm()?.trigger('submit')
    await settle()
    expect(requests.some((r) => r.key.startsWith('PUT'))).toBe(false)

    await wrapper.get(`#add-${family}`).setValue(ada)
    await addForm()?.trigger('submit')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`PUT /api/v1/access/groups/${family}/members/${ada}`)

    await wrapper.get('article[aria-label="admins"] li button').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('The last Admin cannot leave the admins Group.')
  })

  it('shows the problem when a Group is gone', async () => {
    const { wrapper } = await mountAt('/access', { 'GET /api/v1/access': () => Response.json(overview()) })
    await wrapper.get('article[aria-label="Family"] > button').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe(`no answer for DELETE /api/v1/access/groups/${family}`)
  })

  it('names a member it does not know by id', async () => {
    const stranger = '00000000-0000-4000-8000-0000000000ff'
    const o = overview()
    o.groups[1] = { id: family, name: 'Family', admins: false, members: [stranger] }
    const { wrapper } = await mountAt('/access', { 'GET /api/v1/access': Response.json(o) })
    expect(wrapper.get('article[aria-label="Family"] li').text()).toContain(stranger)
  })
})

describe('the no-access page', () => {
  it('says why the app did not open', async () => {
    const { wrapper } = await mountAt('/no-access', {})
    expect(wrapper.get('h1').text()).toBe('No access to this app yet')
    expect(wrapper.get('a').attributes('href')).toBe('/profile')
  })
})
