import { afterEach, describe, expect, it, vi } from 'vitest'
import { admin, dashboard } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import type { MachinesOverview } from '../../infrastructure/api/types.gen'

const backupId = '00000000-0000-4000-8000-0000000000c1'
const syncId = '00000000-0000-4000-8000-0000000000c2'
const ok = () => new Response(null, { status: 204 })

function machines(over: Partial<MachinesOverview> = {}): MachinesOverview {
  return {
    machines: [
      { id: backupId, name: 'Backup', namespace: 'media', serviceAccount: 'backup', createdAt: '2026-10-19T12:00:00Z' },
      { id: syncId, name: 'Sync', namespace: 'media', serviceAccount: 'sync', createdAt: '2026-10-19T12:00:00Z' },
    ],
    assignments: [{ machineId: backupId, clientId: 'grafana', role: 'viewer' }],
    clients: [
      { id: 'grafana', label: 'Grafana', roles: ['editor', 'viewer'] },
      { id: 'jellyfin', label: 'Jellyfin', roles: ['user'] },
    ],
    ...over,
  }
}

/** An Admin's Machines pages over Machine Identities the answers change, as the server would. */
function server(start: MachinesOverview = machines()) {
  const o = structuredClone(start)
  const answers: Record<string, () => Response> = {
    'GET /api/v1/profile': () => Response.json(admin()),
    'GET /api/v1/dashboard': () => Response.json(dashboard()),
    'GET /api/v1/access/machines': () => Response.json(o),
    'POST /api/v1/access/machines': () => {
      const m = { id: '00000000-0000-4000-8000-0000000000c3', name: 'Mailer', namespace: 'mail', serviceAccount: 'mailer', createdAt: '2026-10-19T12:00:00Z' }
      o.machines.push(m)
      return Response.json(m, { status: 201 })
    },
    [`DELETE /api/v1/access/machines/${backupId}`]: () => {
      o.machines = o.machines.filter((m) => m.id !== backupId)
      return ok()
    },
    [`POST /api/v1/access/machines/${backupId}/secret`]: () => Response.json({ secret: 'shown-once-secret' }),
  }
  for (const c of o.clients) {
    for (const r of c.roles) {
      answers[`PUT /api/v1/access/machines/${backupId}/roles/${c.id}/${r}`] = () => {
        o.assignments.push({ machineId: backupId, clientId: c.id, role: r })
        return ok()
      }
      answers[`DELETE /api/v1/access/machines/${backupId}/roles/${c.id}/${r}`] = () => {
        o.assignments = o.assignments.filter((a) => !(a.machineId === backupId && a.clientId === c.id && a.role === r))
        return ok()
      }
    }
  }
  return answers
}

afterEach(() => vi.unstubAllGlobals())

describe('the Machines tab', () => {
  it('lists each Machine Identity with its ServiceAccount and how many roles it holds', async () => {
    const { wrapper } = await mountAt('/admin/machines', server())
    const rows = wrapper.findAll('.lt-r').map((r) => r.text())
    expect(rows).toEqual(['Backupmedia/backup1 role', 'Syncmedia/sync0 roles'])
    expect(wrapper.get('.head .quiet').text()).toBe('2 Machine Identities')
  })

  it('counts one Machine Identity in the singular', async () => {
    const { wrapper } = await mountAt('/admin/machines', server(machines({ machines: machines().machines.slice(0, 1) })))
    expect(wrapper.get('.head .quiet').text()).toBe('1 Machine Identity')
  })

  it('says when there is none yet', async () => {
    const { wrapper } = await mountAt('/admin/machines', server(machines({ machines: [], assignments: [] })))
    expect(wrapper.get('.empty-state').text()).toBe('No Machine Identities yet. Add one for each workload that calls an app.')
  })

  it('is the Admin view’s tab after People, marked as current', async () => {
    const { wrapper } = await mountAt('/admin/machines', server())
    const tabs = wrapper.findAll('nav.tabs a').map((t) => t.text())
    expect(tabs.slice(tabs.indexOf('People'), tabs.indexOf('People') + 2)).toEqual(['People', 'Machines'])
    expect(wrapper.get('nav.tabs a[aria-current="page"]').text()).toBe('Machines')
  })

  it('adds a Machine Identity bound to a ServiceAccount and opens its page', async () => {
    const { wrapper, requests, router } = await mountAt('/admin/machines', server())
    await wrapper.get('.head .btn.primary').trigger('click')
    await wrapper.get('#mc-name').setValue('Mailer')
    await wrapper.get('#mc-ns').setValue('mail')
    await wrapper.get('#mc-sa').setValue('mailer')
    await wrapper.get('form.mc-add').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/access/machines')?.body).toEqual({ name: 'Mailer', namespace: 'mail', serviceAccount: 'mailer' })
    expect(router.currentRoute.value.path).toBe('/admin/machines/00000000-0000-4000-8000-0000000000c3')
  })

  it('closes the form when adding is cancelled', async () => {
    const { wrapper } = await mountAt('/admin/machines', server())
    await wrapper.get('.head .btn.primary').trigger('click')
    await wrapper.get('form.mc-add .btn.ghost').trigger('click')
    expect(wrapper.find('form.mc-add').exists()).toBe(false)
  })

  it('shows why an add was refused', async () => {
    const answers = { ...server(), 'POST /api/v1/access/machines': () => problem(409, 'Another Machine Identity already uses that ServiceAccount.') }
    const { wrapper } = await mountAt('/admin/machines', answers)
    await wrapper.get('.head .btn.primary').trigger('click')
    await wrapper.get('#mc-name').setValue('Backup two')
    await wrapper.get('#mc-ns').setValue('media')
    await wrapper.get('#mc-sa').setValue('backup')
    await wrapper.get('form.mc-add').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Another Machine Identity already uses that ServiceAccount.')
  })

  it('opens a Machine Identity from its row', async () => {
    const { wrapper, router } = await mountAt('/admin/machines', server())
    await wrapper.get('.lt-r').trigger('click')
    await settle()
    expect(router.currentRoute.value.path).toBe(`/admin/machines/${backupId}`)
  })

  it('shows the problem when Machine Identities cannot load', async () => {
    const answers = { ...server(), 'GET /api/v1/access/machines': () => problem(403, 'Only Admins manage access.') }
    const { wrapper } = await mountAt('/admin/machines', answers)
    expect(wrapper.get('[role="alert"]').text()).toBe('Only Admins manage access.')
  })
})

describe('a Machine Identity’s page', () => {
  it('says which ServiceAccount it signs in as and its client id', async () => {
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, server())
    expect(wrapper.get('.hero-t p').text()).toBe('Signs in as the ServiceAccount media/backup')
    expect(wrapper.get('.pills').text()).toContain(`Client ID ${backupId}`)
  })

  it('lists the apps it can open first, with the roles it holds', async () => {
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, server())
    const rows = wrapper.findAll('.acc-row')
    expect(rows.map((r) => r.get('.acc-n').text())).toEqual(['GrafanaHas access', 'JellyfinNo access'])
  })

  it('gives it a role', async () => {
    const { wrapper, requests } = await mountAt(`/admin/machines/${backupId}`, server())
    const editor = wrapper.findAll('.acc-row')[0]?.findAll('button')
    await editor?.find((b) => b.text() === 'editor')?.trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`PUT /api/v1/access/machines/${backupId}/roles/grafana/editor`)
  })

  it('takes a role it holds', async () => {
    const { wrapper, requests } = await mountAt(`/admin/machines/${backupId}`, server())
    const viewer = wrapper.findAll('.acc-row')[0]?.findAll('button')
    await viewer?.find((b) => b.text() === 'viewer')?.trigger('click')
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/access/machines/${backupId}/roles/grafana/viewer`)
  })

  it('shows a new secret once', async () => {
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, server(), { attach: true })
    await wrapper.findAll('.row button').find((b) => b.text() === 'Make a new secret')?.trigger('click')
    await settle()
    expect(document.querySelector('.secret code')?.textContent).toBe('shown-once-secret')
    ;(document.querySelector('.dialog .btn-row .btn') as HTMLButtonElement).click()
    await settle()
    expect(document.querySelector('.secret')).toBeNull()
    wrapper.unmount()
  })

  it('deletes it after asking, and goes back to the list', async () => {
    const { wrapper, requests, router } = await mountAt(`/admin/machines/${backupId}`, server(), { attach: true })
    await wrapper.findAll('.row button').find((b) => b.text() === 'Delete')?.trigger('click')
    await settle()
    expect(document.querySelector('#mc-dlg')?.textContent).toBe('Delete Backup?')
    ;(document.querySelector('.dialog form, form.dialog') as HTMLFormElement).requestSubmit()
    await settle()
    expect(requests.map((r) => r.key)).toContain(`DELETE /api/v1/access/machines/${backupId}`)
    expect(router.currentRoute.value.path).toBe('/admin/machines')
    wrapper.unmount()
  })

  it('closes the new secret on Escape', async () => {
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, server(), { attach: true })
    await wrapper.findAll('.row button').find((b) => b.text() === 'Make a new secret')?.trigger('click')
    await settle()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await settle()
    expect(document.querySelector('.secret')).toBeNull()
    wrapper.unmount()
  })

  it('copies the new secret', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, server(), { attach: true })
    await wrapper.findAll('.row button').find((b) => b.text() === 'Make a new secret')?.trigger('click')
    await settle()
    ;(document.querySelector('.secret button') as HTMLButtonElement).click()
    await settle()
    expect(writeText).toHaveBeenCalledWith('shown-once-secret')
    expect(document.querySelector('.secret button')?.textContent).toBe('Copied')
    wrapper.unmount()
  })

  it('closes the delete question on Escape', async () => {
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, server(), { attach: true })
    await wrapper.findAll('.row button').find((b) => b.text() === 'Delete')?.trigger('click')
    await settle()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await settle()
    expect(document.querySelector('#mc-dlg')).toBeNull()
    wrapper.unmount()
  })

  it('keeps it when deleting is cancelled', async () => {
    const { wrapper, requests } = await mountAt(`/admin/machines/${backupId}`, server(), { attach: true })
    await wrapper.findAll('.row button').find((b) => b.text() === 'Delete')?.trigger('click')
    await settle()
    ;(document.querySelector('form.dialog .btn-row .btn') as HTMLButtonElement).click()
    await settle()
    expect(document.querySelector('#mc-dlg')).toBeNull()
    expect(requests.map((r) => r.key)).not.toContain(`DELETE /api/v1/access/machines/${backupId}`)
    wrapper.unmount()
  })

  it('shows nothing for a Machine Identity that is gone', async () => {
    const { wrapper } = await mountAt('/admin/machines/00000000-0000-4000-8000-0000000000ff', server())
    expect(wrapper.find('.hero').exists()).toBe(false)
  })

  it('shows why a change was refused', async () => {
    const answers = { ...server(), [`POST /api/v1/access/machines/${backupId}/secret`]: () => problem(403, 'Only Admins can make a new secret for a Machine Identity.') }
    const { wrapper } = await mountAt(`/admin/machines/${backupId}`, answers)
    await wrapper.findAll('.row button').find((b) => b.text() === 'Make a new secret')?.trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Only Admins can make a new secret for a Machine Identity.')
  })
})
