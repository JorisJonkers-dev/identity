import type { DOMWrapper, VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { admin, dashboard, ids, overview } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import type { AccessOverview } from '../../infrastructure/api/types.gen'

const ok = () => new Response(null, { status: 204 })
const roleAt = (groupId: string, clientId: string, role: string) => `/api/v1/access/groups/${groupId}/roles/${clientId}/${role}`
const memberAt = (groupId: string, userId: string) => `/api/v1/access/groups/${groupId}/members/${userId}`
const userRoleAt = (userId: string, clientId: string, role: string) => `/api/v1/access/users/${userId}/roles/${clientId}/${role}`

/** An Admin's pages over an overview the answers change, as the server would. */
function server(start: AccessOverview = overview()) {
  const o = structuredClone(start)
  const group = (id: string) => o.groups.find((g) => g.id === id)
  const answers: Record<string, () => Response> = {
    'GET /api/v1/profile': () => Response.json(admin()),
    'GET /api/v1/dashboard': () => Response.json(dashboard()),
    'GET /api/v1/access': () => Response.json(o),
  }
  let stepUp = 0
  for (const c of o.clients) {
    answers[`GET /api/v1/apps/${c.id}/step-up`] = () => Response.json({ minutes: stepUp })
    answers[`PUT /api/v1/apps/${c.id}/step-up`] = () => {
      stepUp = 15
      return ok()
    }
  }
  for (const g of o.groups) {
    for (const c of o.clients) {
      for (const r of c.roles) {
        answers[`PUT ${roleAt(g.id, c.id, r)}`] = () => {
          o.assignments.push({ groupId: g.id, clientId: c.id, role: r })
          return ok()
        }
        answers[`DELETE ${roleAt(g.id, c.id, r)}`] = () => {
          o.assignments = o.assignments.filter((a) => !(a.groupId === g.id && a.clientId === c.id && a.role === r))
          return ok()
        }
      }
    }
    for (const u of o.users) {
      answers[`PUT ${memberAt(g.id, u.id)}`] = () => {
        group(g.id)?.members.push(u.id)
        return ok()
      }
      answers[`DELETE ${memberAt(g.id, u.id)}`] = () => {
        const it = group(g.id)
        if (it) it.members = it.members.filter((m) => m !== u.id)
        return ok()
      }
    }
  }
  for (const u of o.users) {
    for (const c of o.clients) {
      for (const r of c.roles) {
        answers[`PUT ${userRoleAt(u.id, c.id, r)}`] = () => {
          o.userAssignments.push({ userId: u.id, clientId: c.id, role: r })
          return ok()
        }
        answers[`DELETE ${userRoleAt(u.id, c.id, r)}`] = () => {
          o.userAssignments = o.userAssignments.filter((a) => !(a.userId === u.id && a.clientId === c.id && a.role === r))
          return ok()
        }
      }
    }
  }
  return { o, answers }
}

function button(wrapper: { findAll: (selector: string) => DOMWrapper<Element>[] }, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

async function pick(wrapper: VueWrapper, label: string, option: string) {
  await wrapper.get(`.dd-btn[aria-label="${label}"]`).trigger('click')
  await wrapper
    .get(`.dd-menu[aria-label="${label}"]`)
    .findAll('[role="option"]')
    .find((o) => o.text() === option)
    ?.trigger('click')
  await settle()
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.useRealTimers()
})

describe('the Groups page', () => {
  const appNames = (wrapper: VueWrapper) => wrapper.findAll('.acc-row .acc-n b').map((b) => b.text())
  const chip = (wrapper: VueWrapper, app: string, role: string) => {
    const row = wrapper.findAll('.acc-row').find((r) => r.get('.acc-n b').text() === app)
    const found = row?.findAll('.chip').find((c) => c.text() === role)
    if (!found) throw new Error(`no ${role} on ${app}`)
    return found
  }

  it('lists the Groups and opens the first that is not the admins', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    expect(wrapper.findAll('.g-item b').map((b) => b.text())).toEqual(['Admins', 'Family', 'Friends'])
    expect(wrapper.get('.g-item[aria-current="page"] b').text()).toBe('Family')
    expect(wrapper.get('.g-detail h2').text()).toBe('Family')
  })

  it('counts each Group’s people and apps', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    expect(wrapper.findAll('.g-item small').map((s) => s.text())).toEqual(['1 people · Admin console', '2 people · 2 apps', '0 people · 0 apps'])
  })

  it('opens the Group the address names', async () => {
    const { wrapper } = await mountAt(`/admin/groups/${ids.friends}`, server().answers)
    expect(wrapper.get('.g-detail h2').text()).toBe('Friends')
  })

  it('lists every app, those the Group has first, with the roles it holds pressed', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    expect(appNames(wrapper)).toEqual(['Grafana', 'Jellyfin', 'Vault'])
    expect(chip(wrapper, 'Grafana', 'viewer').attributes('aria-pressed')).toBe('true')
    expect(chip(wrapper, 'Grafana', 'editor').attributes('aria-pressed')).toBe('false')
  })

  it('gives the Group a role, says so, and undoes it', async () => {
    const { answers } = server()
    const { wrapper, requests } = await mountAt('/admin/groups', answers)
    await chip(wrapper, 'Vault', 'user').trigger('click')
    await settle()
    expect(requests.at(-2)?.key).toBe(`PUT ${roleAt(ids.family, 'vault', 'user')}`)
    expect(chip(wrapper, 'Vault', 'user').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('.toast').text()).toContain('Family now holds user on Vault')
    await button(wrapper, 'Undo').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE ${roleAt(ids.family, 'vault', 'user')}`)).toBe(true)
    expect(chip(wrapper, 'Vault', 'user').attributes('aria-pressed')).toBe('false')
    expect(wrapper.find('.toast').exists()).toBe(false)
  })

  it('takes a role from the Group', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    await chip(wrapper, 'Grafana', 'viewer').trigger('click')
    await settle()
    expect(chip(wrapper, 'Grafana', 'viewer').attributes('aria-pressed')).toBe('false')
    expect(wrapper.get('.toast').text()).toContain('Family no longer holds viewer on Grafana')
  })

  it('puts the toast away after a few seconds', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    await chip(wrapper, 'Vault', 'user').trigger('click')
    await settle()
    vi.advanceTimersByTime(4100)
    await settle()
    expect(wrapper.find('.toast').exists()).toBe(false)
  })

  it('finds an app by name', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    await wrapper.get('input[aria-label="Find an app"]').setValue('VAU')
    expect(appNames(wrapper)).toEqual(['Vault'])
    await wrapper.get('input[aria-label="Find an app"]').setValue('overleaf')
    expect(wrapper.get('.acc .empty-state').text()).toBe('No app matches.')
  })

  it('narrows to the apps with a role', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    await pick(wrapper, 'Role', 'editor')
    expect(appNames(wrapper)).toEqual(['Grafana'])
  })

  it('narrows to the apps the Group has', async () => {
    const { wrapper } = await mountAt(`/admin/groups/${ids.family}`, server().answers)
    await button(wrapper, 'With access 2').trigger('click')
    expect(appNames(wrapper)).toEqual(['Grafana', 'Jellyfin'])
    await button(wrapper, 'All 3').trigger('click')
    expect(appNames(wrapper)).toHaveLength(3)
  })

  it('sorts the apps by name', async () => {
    const { o, answers } = server()
    o.assignments = [{ groupId: ids.family, clientId: 'vault', role: 'user' }]
    const { wrapper } = await mountAt('/admin/groups', answers)
    expect(appNames(wrapper)).toEqual(['Vault', 'Grafana', 'Jellyfin'])
    await pick(wrapper, 'Sort', 'A to Z')
    expect(appNames(wrapper)).toEqual(['Grafana', 'Jellyfin', 'Vault'])
  })

  it('gives the admins Group no app roles, and keeps it from being renamed or deleted', async () => {
    const { wrapper } = await mountAt(`/admin/groups/${ids.admins}`, server().answers)
    expect(wrapper.get('.admin-note').text()).toBe('Admins manage identity: Groups, apps, people and the audit log. No app roles.')
    expect(wrapper.find('.acc').exists()).toBe(false)
    expect(wrapper.findAll('.g-head button')).toHaveLength(0)
  })

  it('switches to the members, through the address', async () => {
    const { wrapper, router } = await mountAt('/admin/groups', server().answers)
    await wrapper.findAll('.g-tab')[1]?.trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe(`/admin/groups/${ids.family}?tab=members`)
    expect(wrapper.findAll('.m-row b').map((b) => b.text())).toEqual(['Joris Jonkers', 'Ada Lovelace'])
  })

  it('names the other Groups each member is in', async () => {
    const { wrapper } = await mountAt(`/admin/groups/${ids.family}?tab=members`, server().answers)
    expect(wrapper.findAll('.m-row .also').map((a) => a.text())).toEqual(['Admins', ''])
  })

  it('removes a member', async () => {
    const { wrapper, requests } = await mountAt(`/admin/groups/${ids.family}?tab=members`, server().answers)
    await wrapper.get('[aria-label="Remove Ada Lovelace from Family"]').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE ${memberAt(ids.family, ids.ada)}`)).toBe(true)
    expect(wrapper.findAll('.m-row b').map((b) => b.text())).toEqual(['Joris Jonkers'])
  })

  it('adds people, those without a Group first', async () => {
    const { wrapper, requests } = await mountAt(`/admin/groups/${ids.friends}?tab=members`, server().answers)
    expect(wrapper.get('.m-list .empty-state').text()).toBe('No one yet.')
    await wrapper.get('input[aria-label="Add people"]').trigger('focus')
    const offered = wrapper.findAll('.sugg')
    expect(offered.map((s) => s.get('b').text())).toEqual(['Noor Bakker', 'Ada Lovelace', 'Joris Jonkers'])
    expect(offered[0]?.find('.gtag.warn').exists()).toBe(true)
    await offered[0]?.trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `PUT ${memberAt(ids.friends, ids.noor)}`)).toBe(true)
    expect(wrapper.findAll('.m-row b').map((b) => b.text())).toEqual(['Noor Bakker'])
  })

  it('finds a person to add by name or email', async () => {
    const { wrapper } = await mountAt(`/admin/groups/${ids.friends}?tab=members`, server().answers)
    await wrapper.get('input[aria-label="Add people"]').trigger('focus')
    await wrapper.get('input[aria-label="Add people"]').setValue('ada@')
    expect(wrapper.findAll('.sugg b').map((b) => b.text())).toEqual(['Ada Lovelace'])
  })

  it('says when everyone is in the Group already', async () => {
    const { o, answers } = server()
    o.groups = o.groups.map((g) => (g.id === ids.friends ? { ...g, members: [ids.joris, ids.ada, ids.noor] } : g))
    const { wrapper } = await mountAt(`/admin/groups/${ids.friends}?tab=members`, answers)
    await wrapper.get('input[aria-label="Add people"]').trigger('focus')
    expect(wrapper.get('.sugg-list').text()).toContain('Everyone is in Friends.')
  })

  it('closes the list on Escape and on Done', async () => {
    const { wrapper } = await mountAt(`/admin/groups/${ids.friends}?tab=members`, server().answers)
    const input = wrapper.get('input[aria-label="Add people"]')
    await input.trigger('focus')
    await input.trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('.sugg-list').exists()).toBe(false)
    await input.trigger('focus')
    await button(wrapper, 'Done').trigger('click')
    expect(wrapper.find('.sugg-list').exists()).toBe(false)
  })

  it('makes a new Group', async () => {
    const { answers } = server()
    const { wrapper, requests } = await mountAt('/admin/groups', {
      ...answers,
      'POST /api/v1/access/groups': () => Response.json({ id: '00000000-0000-4000-8000-0000000000b9', name: 'Neighbours', admins: false, members: [] }, { status: 201 }),
    })
    await button(wrapper, 'New Group').trigger('click')
    await wrapper.get('#g-name').setValue('Neighbours')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/access/groups')?.body).toEqual({ name: 'Neighbours' })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('renames a Group, starting from its name', async () => {
    const { wrapper, requests } = await mountAt('/admin/groups', { ...server().answers, [`PATCH /api/v1/access/groups/${ids.family}`]: ok })
    await button(wrapper, 'Rename').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#g-name').element.value).toBe('Family')
    await wrapper.get('#g-name').setValue('Household')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === `PATCH /api/v1/access/groups/${ids.family}`)?.body).toEqual({ name: 'Household' })
  })

  it('deletes a Group after saying what it costs', async () => {
    const { o, answers } = server()
    const gone = () => {
      o.groups = o.groups.filter((g) => g.id !== ids.family)
      return ok()
    }
    const { wrapper, router, requests } = await mountAt(`/admin/groups/${ids.family}`, { ...answers, [`DELETE /api/v1/access/groups/${ids.family}`]: gone })
    await button(wrapper, 'Delete').trigger('click')
    expect(wrapper.get('[role="dialog"] p').text()).toBe('2 people lose the access it gives them.')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(requests.some((r) => r.key === `DELETE /api/v1/access/groups/${ids.family}`)).toBe(true)
    expect(router.currentRoute.value.path).toBe('/admin/groups')
    expect(wrapper.findAll('.g-item b').map((b) => b.text())).toEqual(['Admins', 'Friends'])
  })

  it('keeps a Group open, and says why, when its delete is refused', async () => {
    const { wrapper, router } = await mountAt(`/admin/groups/${ids.family}`, {
      ...server().answers,
      [`DELETE /api/v1/access/groups/${ids.family}`]: () => problem(409, 'Family still gives roles.'),
    })
    await button(wrapper, 'Delete').trigger('click')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(router.currentRoute.value.path).toBe(`/admin/groups/${ids.family}`)
    expect(wrapper.get('[role="alert"]').text()).toBe('Family still gives roles.')
  })

  it('cancels a dialog without a change', async () => {
    const { wrapper, requests } = await mountAt('/admin/groups', server().answers)
    await button(wrapper, 'Delete').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(requests.some((r) => r.key.startsWith('DELETE'))).toBe(false)
  })

  it('shows a refused change', async () => {
    const { wrapper } = await mountAt('/admin/groups', { ...server().answers, [`PUT ${roleAt(ids.family, 'vault', 'user')}`]: () => problem(409, 'That role is no longer on Vault.') })
    await chip(wrapper, 'Vault', 'user').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That role is no longer on Vault.')
  })

  it('shows the problem when the Groups cannot load', async () => {
    const { wrapper } = await mountAt('/admin/groups', { ...server().answers, 'GET /api/v1/access': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('closes a dialog on a click beside it', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    await button(wrapper, 'New Group').trigger('click')
    await wrapper.get('.scrim').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('opens and closes a dropdown from its button', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    const role = () => wrapper.get('.dd-btn[aria-label="Role"]')
    await role().trigger('click')
    expect(role().attributes('aria-expanded')).toBe('true')
    await role().trigger('click')
    expect(role().attributes('aria-expanded')).toBe('false')
  })

  it('closes a dropdown on Escape or a click elsewhere, not a click inside', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers, { attach: true })
    const role = () => wrapper.get('.dd-btn[aria-label="Role"]')
    await role().trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await settle()
    expect(role().attributes('aria-expanded')).toBe('false')
    await role().trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    await settle()
    expect(role().attributes('aria-expanded')).toBe('true')
    wrapper.get('.dd-menu[aria-label="Role"]').element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await settle()
    expect(role().attributes('aria-expanded')).toBe('true')
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await settle()
    expect(role().attributes('aria-expanded')).toBe('false')
  })

  it('opens the admins Group when it is the only one', async () => {
    const { o, answers } = server()
    o.groups = o.groups.filter((g) => g.admins)
    const { wrapper } = await mountAt('/admin/groups', answers)
    expect(wrapper.get('.g-detail h2').text()).toBe('Admins')
  })

  it('switches back to the apps', async () => {
    const { wrapper, router } = await mountAt(`/admin/groups/${ids.family}?tab=members`, server().answers)
    await wrapper.findAll('.g-tab')[0]?.trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe(`/admin/groups/${ids.family}`)
    expect(wrapper.find('.acc').exists()).toBe(true)
  })

  it('keeps the name when the rename is cancelled', async () => {
    const { wrapper, requests } = await mountAt('/admin/groups', server().answers)
    await button(wrapper, 'Rename').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(requests.some((r) => r.key.startsWith('PATCH'))).toBe(false)
  })

  it('shows a refused new Group', async () => {
    const { wrapper } = await mountAt('/admin/groups', { ...server().answers, 'POST /api/v1/access/groups': () => problem(409, 'A Group with that name exists.') })
    await button(wrapper, 'New Group').trigger('click')
    await wrapper.get('#g-name').setValue('Family')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('A Group with that name exists.')
  })

  it('closes the rename and delete dialogs on Escape', async () => {
    const { wrapper } = await mountAt('/admin/groups', server().answers)
    for (const name of ['Rename', 'Delete']) {
      await button(wrapper, name).trigger('click')
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await settle()
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    }
  })

  it('puts the new Group form away on cancel', async () => {
    const { wrapper, requests } = await mountAt('/admin/groups', server().answers)
    await button(wrapper, 'New Group').trigger('click')
    await button(wrapper.get('[role="dialog"]'), 'Cancel').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(requests.some((r) => r.key === 'POST /api/v1/access/groups')).toBe(false)
  })
})

describe('the Apps page', () => {
  const names = (wrapper: VueWrapper) => wrapper.findAll('.lt-r .app-c').map((a) => a.text().slice(1))

  it('lists each app with its id, roles and how many Groups hold any', async () => {
    const { wrapper } = await mountAt('/admin/apps', server().answers)
    expect(wrapper.get('.head .quiet').text()).toBe('3 apps')
    const row = wrapper.findAll('.lt-r')[0]
    expect(row?.findAll('.muted').map((m) => m.text())).toEqual(['grafana', '1 Group'])
    expect(row?.findAll('.role').map((r) => r.text())).toEqual(['editor', 'viewer'])
    expect(wrapper.findAll('.lt-r')[2]?.findAll('.muted')[1]?.text()).toBe('0 Groups')
  })

  it('opens an app', async () => {
    const { wrapper, router } = await mountAt('/admin/apps', server().answers)
    await wrapper.findAll('.lt-r')[1]?.trigger('click')
    await settle()
    expect(router.currentRoute.value.path).toBe('/admin/apps/jellyfin')
  })

  it('finds an app by name or id, and says when none matches', async () => {
    const { wrapper } = await mountAt('/admin/apps', server().answers)
    await wrapper.get('input[aria-label="Find an app"]').setValue('jelly')
    expect(names(wrapper)).toEqual(['Jellyfin'])
    await wrapper.get('input[aria-label="Find an app"]').setValue('overleaf')
    expect(wrapper.get('.empty-state').text()).toBe('No app matches.')
  })

  it('narrows to the apps with a role, and sorts by Groups', async () => {
    const { o, answers } = server()
    o.assignments.push({ groupId: ids.friends, clientId: 'vault', role: 'user' }, { groupId: ids.family, clientId: 'vault', role: 'user' })
    const { wrapper } = await mountAt('/admin/apps', answers)
    await pick(wrapper, 'Sort', 'Most Groups')
    expect(names(wrapper)).toEqual(['Vault', 'Grafana', 'Jellyfin'])
    await pick(wrapper, 'Role', 'user')
    expect(names(wrapper)).toEqual(['Vault', 'Jellyfin'])
  })

  it('shows the problem when the apps cannot load', async () => {
    const { wrapper } = await mountAt(`/admin/apps`, { ...server().answers, 'GET /api/v1/access': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})

describe('an app’s page', () => {
  const stepUpRow = (wrapper: VueWrapper) => wrapper.get('.row.step-up')
  const block = (wrapper: VueWrapper, role: string) => {
    const found = wrapper.findAll('.role-block').find((b) => b.get('.role').text() === role)
    if (!found) throw new Error(`no role ${role}`)
    return found
  }

  it('lists the Groups holding each role', async () => {
    const { wrapper } = await mountAt('/admin/apps/grafana', server().answers)
    expect(wrapper.get('h1').text()).toBe('Grafana')
    expect(block(wrapper, 'viewer').findAll('.gchip').map((g) => g.text())).toEqual(['Family'])
    expect(block(wrapper, 'viewer').get('.rb-h .soft').text()).toBe('1 Groups')
    expect(block(wrapper, 'editor').get('.gchips .soft').text()).toBe('None')
  })

  it('takes a role from a Group', async () => {
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', server().answers)
    await wrapper.get('[aria-label="Take viewer from Family"]').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE ${roleAt(ids.family, 'grafana', 'viewer')}`)).toBe(true)
    expect(block(wrapper, 'viewer').findAll('.gchip')).toHaveLength(0)
  })

  it('gives a role to a Group, offering only those without it and never the admins', async () => {
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', server().answers)
    expect(block(wrapper, 'viewer').get('.rb-line.groups').findAll('[role="option"]').map((o) => o.text())).toEqual(['Friends'])
    await pick(wrapper, 'Give editor to a Group', 'Family')
    expect(requests.some((r) => r.key === `PUT ${roleAt(ids.family, 'grafana', 'editor')}`)).toBe(true)
    expect(block(wrapper, 'editor').findAll('.gchip').map((g) => g.text())).toEqual(['Family'])
  })

  it('writes a new secret once asked twice', async () => {
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', { ...server().answers, 'POST /api/v1/apps/grafana/secret': ok })
    await button(wrapper, 'Replace secret').trigger('click')
    expect(wrapper.get('.confirm .ask').text()).toBe('Replace the secret for Grafana?The old one works until Grafana uses the new one.')
    await button(wrapper.get('.confirm'), 'Replace secret').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === 'POST /api/v1/apps/grafana/secret')).toBe(true)
    expect(wrapper.get('.row small').text()).toBe('Secret replaced')
  })

  it('keeps the secret when asked to', async () => {
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', server().answers)
    await button(wrapper, 'Replace secret').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('.confirm').exists()).toBe(false)
    expect(requests.some((r) => r.key.endsWith('/secret'))).toBe(false)
  })

  it('shows a refused secret', async () => {
    const { wrapper } = await mountAt('/admin/apps/grafana', { ...server().answers, 'POST /api/v1/apps/grafana/secret': () => problem(409, 'Grafana signs in without a secret.') })
    await button(wrapper, 'Replace secret').trigger('click')
    await button(wrapper.get('.confirm'), 'Replace secret').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Grafana signs in without a secret.')
  })

  it('lists the people holding each role directly', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.ada, clientId: 'grafana', role: 'editor' })
    const { wrapper } = await mountAt('/admin/apps/grafana', answers)
    expect(block(wrapper, 'editor').findAll('.rb-line.people .gchip').map((g) => g.text())).toEqual(['Ada Lovelace'])
    expect(block(wrapper, 'viewer').get('.rb-line.people .soft').text()).toBe('None')
  })

  it('gives a role to a person, offering only those without it', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.ada, clientId: 'grafana', role: 'editor' })
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', answers)
    expect(block(wrapper, 'editor').get('.rb-line.people').findAll('[role="option"]').map((x) => x.text())).toEqual(['Joris Jonkers', 'Noor Bakker'])
    await pick(wrapper, 'Give viewer to a person', 'Noor Bakker')
    expect(requests.some((r) => r.key === `PUT ${userRoleAt(ids.noor, 'grafana', 'viewer')}`)).toBe(true)
    expect(block(wrapper, 'viewer').findAll('.rb-line.people .gchip').map((g) => g.text())).toEqual(['Noor Bakker'])
  })

  it('takes a role from a person', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.ada, clientId: 'grafana', role: 'editor' })
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', answers)
    await wrapper.get('[aria-label="Take editor from Ada Lovelace"]').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE ${userRoleAt(ids.ada, 'grafana', 'editor')}`)).toBe(true)
    expect(block(wrapper, 'editor').findAll('.rb-line.people .gchip')).toHaveLength(0)
  })

  it('says step-up is off until an Admin sets it', async () => {
    const { wrapper } = await mountAt('/admin/apps/grafana', server().answers)
    expect(stepUpRow(wrapper).get('small').text()).toBe('Off. Signed-in people open Grafana without a new check.')
  })

  it('sets how fresh two-factor must be to open the app', async () => {
    const { wrapper, requests } = await mountAt('/admin/apps/grafana', server().answers)
    await pick(wrapper, 'Ask for two-factor again in Grafana', '15 minutes')
    expect(requests.find((r) => r.key === 'PUT /api/v1/apps/grafana/step-up')?.body).toEqual({ minutes: 15 })
    expect(stepUpRow(wrapper).get('small').text()).toBe('Asks for two-factor again after 15 minutes.')
  })

  it('shows a refused step-up', async () => {
    const { wrapper } = await mountAt('/admin/apps/grafana', { ...server().answers, 'PUT /api/v1/apps/grafana/step-up': () => problem(422, 'Choose Off, 5 minutes, 15 minutes or 60 minutes.') })
    await pick(wrapper, 'Ask for two-factor again in Grafana', '5 minutes')
    expect(wrapper.get('[role="alert"]').text()).toBe('Choose Off, 5 minutes, 15 minutes or 60 minutes.')
  })

  it('goes back to the apps', async () => {
    const { wrapper } = await mountAt('/admin/apps/grafana', server().answers)
    expect(wrapper.get('a.crumb').attributes('href')).toBe('/admin/apps')
  })

  it('shows the problem when the app cannot load', async () => {
    const { wrapper } = await mountAt(`/admin/apps/grafana`, { ...server().answers, 'GET /api/v1/access': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('shows a refused role change', async () => {
    const { wrapper } = await mountAt('/admin/apps/grafana', { ...server().answers, [`DELETE ${roleAt(ids.family, 'grafana', 'viewer')}`]: () => problem(409, 'That role is no longer on Grafana.') })
    await wrapper.get('[aria-label="Take viewer from Family"]').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That role is no longer on Grafana.')
  })
})

describe('the People page', () => {
  const names = (wrapper: VueWrapper) => wrapper.findAll('.lt-r .who b').map((b) => b.text())

  it('lists everyone, those without a Group first, with their Groups', async () => {
    const { wrapper } = await mountAt('/admin/people', server().answers)
    expect(wrapper.get('.head .quiet').text()).toBe('3 people')
    expect(names(wrapper)).toEqual(['Noor Bakker', 'Ada Lovelace', 'Joris Jonkers'])
    expect(wrapper.findAll('.lt-r')[0]?.get('.gtag.warn').text()).toBe('Unassigned')
    expect(wrapper.findAll('.lt-r')[2]?.findAll('.gtag').map((t) => t.text())).toEqual(['Admins', 'Family'])
    expect(wrapper.findAll('.lt-r')[0]?.get('a.who').attributes('href')).toBe(`/admin/people/${ids.noor}`)
  })

  it('finds a person by name or email, and says when nobody matches', async () => {
    const { wrapper } = await mountAt('/admin/people', server().answers)
    await wrapper.get('input[aria-label="Find a person"]').setValue('joris@')
    expect(names(wrapper)).toEqual(['Joris Jonkers'])
    await wrapper.get('input[aria-label="Find a person"]').setValue('nobody')
    expect(wrapper.get('.empty-state').text()).toBe('No one matches.')
  })

  it('narrows to a Group, or to those in none', async () => {
    const { wrapper } = await mountAt('/admin/people', server().answers)
    await pick(wrapper, 'Group', 'Family')
    expect(names(wrapper)).toEqual(['Ada Lovelace', 'Joris Jonkers'])
    await pick(wrapper, 'Group', 'Unassigned')
    expect(names(wrapper)).toEqual(['Noor Bakker'])
  })

  it('counts a person with only a direct role as placed', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.noor, clientId: 'vault', role: 'user' })
    const { wrapper } = await mountAt('/admin/people', answers)
    expect(wrapper.findAll('.gtag.warn')).toHaveLength(0)
    await pick(wrapper, 'Group', 'Unassigned')
    expect(wrapper.get('.empty-state').text()).toBe('No one matches.')
  })

  it('never counts an Admin as Unassigned', async () => {
    const { o, answers } = server()
    o.groups = o.groups.map((g) => (g.id === ids.family ? { ...g, members: [ids.ada] } : g))
    const { wrapper } = await mountAt('/admin/people', answers)
    expect(wrapper.findAll('.lt-r').find((r) => r.get('.who b').text() === 'Joris Jonkers')?.find('.gtag.warn').exists()).toBe(false)
  })

  it('counts a person in a Group that gives no role as Unassigned', async () => {
    const { o, answers } = server()
    o.groups = o.groups.map((g) => (g.id === ids.friends ? { ...g, members: [ids.noor] } : g))
    const { wrapper } = await mountAt('/admin/people', answers)
    expect(names(wrapper)[0]).toBe('Noor Bakker')
    expect(wrapper.findAll('.lt-r')[0]?.get('.gtag.warn').text()).toBe('Unassigned')
  })

  it('narrows to the people holding a role directly', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.noor, clientId: 'vault', role: 'user' })
    const { wrapper } = await mountAt('/admin/people', answers)
    await pick(wrapper, 'Group', 'With a direct role')
    expect(names(wrapper)).toEqual(['Noor Bakker'])
  })

  it('sorts by name', async () => {
    const { wrapper } = await mountAt('/admin/people', server().answers)
    await pick(wrapper, 'Sort', 'A to Z')
    expect(names(wrapper)).toEqual(['Ada Lovelace', 'Joris Jonkers', 'Noor Bakker'])
  })

  it('adds the people picked to a Group at once', async () => {
    const { wrapper, requests } = await mountAt('/admin/people', server().answers)
    await wrapper.get('[aria-label="Select Noor Bakker"]').setValue(true)
    await wrapper.get('[aria-label="Select Ada Lovelace"]').setValue(true)
    expect(wrapper.get('.bulk').classes()).toContain('shown')
    expect(wrapper.get('.bulk b').text()).toBe('2 selected')
    expect(wrapper.get('.bulk').findAll('[role="option"]').map((o) => o.text())).toEqual(['Family', 'Friends'])
    await pick(wrapper, 'Add selected to a Group', 'Friends')
    expect(requests.filter((r) => r.key.startsWith('PUT')).map((r) => r.key).sort()).toEqual([`PUT ${memberAt(ids.friends, ids.ada)}`, `PUT ${memberAt(ids.friends, ids.noor)}`].sort())
    expect(wrapper.get('.bulk').classes()).not.toContain('shown')
  })

  it('lets go of a pick, or of all of them', async () => {
    const { wrapper } = await mountAt('/admin/people', server().answers)
    await wrapper.get('[aria-label="Select Noor Bakker"]').setValue(true)
    await wrapper.get('[aria-label="Select Ada Lovelace"]').setValue(true)
    await wrapper.get('[aria-label="Select Noor Bakker"]').setValue(false)
    expect(wrapper.get('.bulk b').text()).toBe('1 selected')
    await button(wrapper.get('.bulk'), 'Clear').trigger('click')
    expect(wrapper.get('.bulk').classes()).not.toContain('shown')
  })

  it('shows the problem when the people cannot load', async () => {
    const { wrapper } = await mountAt(`/admin/people`, { ...server().answers, 'GET /api/v1/access': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('shows a refused add', async () => {
    const { wrapper } = await mountAt('/admin/people', { ...server().answers, [`PUT ${memberAt(ids.friends, ids.noor)}`]: () => problem(404, 'That person is gone.') })
    await wrapper.get('[aria-label="Select Noor Bakker"]').setValue(true)
    await pick(wrapper, 'Add selected to a Group', 'Friends')
    expect(wrapper.get('[role="alert"]').text()).toBe('That person is gone.')
  })
})

describe('a person’s page', () => {
  it('shows who they are, their Groups and their apps', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.joris}`, server().answers)
    expect(wrapper.get('h1').text()).toBe('Joris Jonkers')
    expect(wrapper.get('.hero-t p').text()).toBe('joris@example.org')
    expect(wrapper.findAll('.pills .pill').map((p) => p.text())).toEqual(['Admin', '2 Groups', '2 apps'])
    expect(wrapper.findAll('.gchip').map((g) => g.text())).toEqual(['Admins', 'Family'])
  })

  it('shows each app’s roles as coming through Groups', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.ada}`, server().answers)
    const grafana = wrapper.findAll('.acc-row')[0]
    expect(grafana?.get('.acc-n small').text()).toBe('Via Family')
    expect(grafana?.classes()).toContain('held')
    const viewer = grafana?.findAll('.chip').find((c) => c.text() === 'viewer')
    expect(viewer?.attributes('disabled')).toBeDefined()
    expect(viewer?.attributes('title')).toBe('From Family. Change it on the Group.')
    expect(grafana?.findAll('.chip').find((c) => c.text() === 'editor')?.attributes('aria-pressed')).toBe('false')
  })

  it('names every Group that gives a role', async () => {
    const { o, answers } = server()
    o.groups = o.groups.map((g) => (g.id === ids.friends ? { ...g, members: [ids.ada] } : g))
    o.assignments.push({ groupId: ids.friends, clientId: 'grafana', role: 'viewer' })
    const { wrapper } = await mountAt(`/admin/people/${ids.ada}`, answers)
    expect(wrapper.findAll('.acc-row')[0]?.findAll('.chip').find((c) => c.text() === 'viewer')?.attributes('title')).toBe('From Family, Friends. Change it on the Group.')
  })

  it('says when someone has no Group, and offers every app with none held', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.noor}`, server().answers)
    expect(wrapper.get('.gchips .soft').text()).toBe('No Group yet')
    expect(wrapper.findAll('.acc-row .acc-n b').map((b) => b.text())).toEqual(['Grafana', 'Jellyfin', 'Vault'])
    expect(wrapper.findAll('.acc-row.held')).toHaveLength(0)
    expect(wrapper.findAll('.acc-row .acc-n small').map((s) => s.text())).toEqual(['No access', 'No access', 'No access'])
  })

  it('lists the apps they hold a role on first', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.noor, clientId: 'vault', role: 'user' })
    const { wrapper } = await mountAt(`/admin/people/${ids.noor}`, answers)
    expect(wrapper.findAll('.acc-row .acc-n b').map((b) => b.text())).toEqual(['Vault', 'Grafana', 'Jellyfin'])
  })

  it('gives them a role on an app directly', async () => {
    const { wrapper, requests } = await mountAt(`/admin/people/${ids.noor}`, server().answers)
    const vault = () => wrapper.findAll('.acc-row').find((r) => r.get('.acc-n b').text() === 'Vault')
    await vault()?.get('.chip').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `PUT ${userRoleAt(ids.noor, 'vault', 'user')}`)).toBe(true)
    expect(vault()?.get('.chip').attributes('aria-pressed')).toBe('true')
    expect(vault()?.get('.acc-n small').text()).toBe('Direct')
    expect(wrapper.findAll('.pills .pill').map((p) => p.text())).toEqual(['0 Groups', '1 apps'])
  })

  it('takes a direct role away', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.ada, clientId: 'grafana', role: 'editor' })
    const { wrapper, requests } = await mountAt(`/admin/people/${ids.ada}`, answers)
    const editor = () => wrapper.findAll('.acc-row')[0]?.findAll('.chip').find((c) => c.text() === 'editor')
    expect(wrapper.findAll('.acc-row')[0]?.get('.acc-n small').text()).toBe('Direct, also via Family')
    expect(editor()?.attributes('aria-pressed')).toBe('true')
    await editor()?.trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE ${userRoleAt(ids.ada, 'grafana', 'editor')}`)).toBe(true)
    expect(editor()?.attributes('aria-pressed')).toBe('false')
  })

  it('keeps a role held directly and through a Group toggleable, naming the Group', async () => {
    const { o, answers } = server()
    o.userAssignments.push({ userId: ids.ada, clientId: 'grafana', role: 'viewer' })
    const { wrapper } = await mountAt(`/admin/people/${ids.ada}`, answers)
    const viewer = wrapper.findAll('.acc-row')[0]?.findAll('.chip').find((c) => c.text() === 'viewer')
    expect(viewer?.attributes('disabled')).toBeUndefined()
    expect(viewer?.attributes('aria-pressed')).toBe('true')
    expect(viewer?.attributes('title')).toBe('Also via Family')
  })

  it('shows a refused role', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.noor}`, { ...server().answers, [`PUT ${userRoleAt(ids.noor, 'vault', 'user')}`]: () => problem(422, 'That role is no longer on Vault.') })
    await wrapper.findAll('.acc-row').find((r) => r.get('.acc-n b').text() === 'Vault')?.get('.chip').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That role is no longer on Vault.')
  })

  it('adds them to a Group', async () => {
    const { wrapper, requests } = await mountAt(`/admin/people/${ids.noor}`, server().answers)
    await pick(wrapper, 'Add Noor Bakker to a Group', 'Family')
    expect(requests.some((r) => r.key === `PUT ${memberAt(ids.family, ids.noor)}`)).toBe(true)
    expect(wrapper.findAll('.gchip').map((g) => g.text())).toEqual(['Family'])
  })

  it('takes them out of a Group', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.ada}`, server().answers)
    await wrapper.get('[aria-label="Remove Ada Lovelace from Family"]').trigger('click')
    await settle()
    expect(wrapper.findAll('.gchip')).toHaveLength(0)
  })

  it('signs them out everywhere', async () => {
    const { wrapper, requests } = await mountAt(`/admin/people/${ids.ada}`, {
      ...server().answers,
      [`POST /api/v1/access/users/${ids.ada}/sign-out`]: ok,
      [`GET /api/v1/access/users/${ids.ada}/tokens`]: () => Response.json({ tokens: [] }),
    })
    const sessions = () => wrapper.get('main .cols-2 .group')
    await button(sessions(), 'Sign out everywhere').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `POST /api/v1/access/users/${ids.ada}/sign-out`)).toBe(true)
    expect(sessions().get('.row small').text()).toBe('Signed out on every device')
    expect(button(sessions(), 'Sign out everywhere').attributes('disabled')).toBeDefined()
  })

  it('shows the problem when the person cannot load', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.ada}`, { ...server().answers, 'GET /api/v1/access': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('shows a refused add', async () => {
    const { wrapper } = await mountAt(`/admin/people/${ids.noor}`, { ...server().answers, [`PUT ${memberAt(ids.family, ids.noor)}`]: () => problem(404, 'That person is gone.') })
    await pick(wrapper, 'Add Noor Bakker to a Group', 'Family')
    expect(wrapper.get('[role="alert"]').text()).toBe('That person is gone.')
  })
})

describe('the Unassigned page', () => {
  const answers = () => server().answers

  it('lists who waits for a Group, and when they signed up', async () => {
    const { wrapper } = await mountAt('/admin/unassigned', answers())
    expect(wrapper.get('.head .quiet').text()).toBe('1 waiting')
    expect(wrapper.get('.un-who b').text()).toBe('Noor Bakker')
    expect(wrapper.get('.un-who .mono').text()).toBe('signed up 2h ago')
    expect(wrapper.get('.un-acts a').attributes('href')).toBe(`/admin/people/${ids.noor}`)
  })

  it('offers every Group but the admins', async () => {
    const { wrapper } = await mountAt('/admin/unassigned', answers())
    expect(wrapper.findAll('.un-groups .chip').map((c) => c.text())).toEqual(['Family', 'Friends'])
  })

  it('adds them to a Group, and takes them out again', async () => {
    const { wrapper, requests } = await mountAt('/admin/unassigned', answers())
    const family = () => wrapper.findAll('.un-groups .chip')[0]
    await family()?.trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `PUT ${memberAt(ids.family, ids.noor)}`)).toBe(true)
    expect(family()?.attributes('aria-pressed')).toBe('true')
    await family()?.trigger('click')
    await settle()
    expect(requests.some((r) => r.key === `DELETE ${memberAt(ids.family, ids.noor)}`)).toBe(true)
  })

  it('says when nobody waits', async () => {
    const { wrapper } = await mountAt('/admin/unassigned', { ...answers(), 'GET /api/v1/dashboard': () => Response.json(dashboard({ unassigned: [] })) })
    expect(wrapper.get('.empty p').text()).toBe('No one waiting.')
  })

  it('shows the problem when the queue cannot load', async () => {
    const { wrapper } = await mountAt('/admin/unassigned', { ...answers(), 'GET /api/v1/dashboard': () => problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
    expect(wrapper.get('.head .quiet').text()).toBe('0 waiting')
  })

  it('shows the problem when the Groups cannot load, and offers none', async () => {
    const { wrapper } = await mountAt('/admin/unassigned', { ...answers(), 'GET /api/v1/access': () => problem(500, 'The Groups could not be read.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('The Groups could not be read.')
    expect(wrapper.find('.un-groups .chip').exists()).toBe(false)
  })

  it('shows a refused add', async () => {
    const { wrapper } = await mountAt('/admin/unassigned', { ...answers(), [`PUT ${memberAt(ids.family, ids.noor)}`]: () => problem(404, 'That person is gone.') })
    await wrapper.findAll('.un-groups .chip')[0]?.trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That person is gone.')
  })
})
