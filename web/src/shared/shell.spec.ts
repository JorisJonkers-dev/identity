import { afterEach, describe, expect, it, vi } from 'vitest'
import { admin, dashboard, overview, profile } from '../app/fixtures'
import { mountAt, problem, settle } from '../app/testing'
import { navigation } from '../infrastructure/navigation'

const personalPage = (who = profile()) => ({
  'GET /api/v1/profile': () => Response.json(who),
  'GET /api/v1/profile/second-factors': () => Response.json({ factors: [] }),
  'GET /api/v1/launcher': () => Response.json({ apps: [], heldBack: 0 }),
  'GET /api/v1/dashboard': () => Response.json(dashboard()),
})

const adminPage = (who = admin()) => ({
  ...personalPage(who),
  'GET /api/v1/access': () => Response.json(overview()),
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the bar on every signed-in page', () => {
  it('offers the personal tabs and marks the current one', async () => {
    const { wrapper } = await mountAt('/account', personalPage())
    const tabs = wrapper.findAll('nav.tabs a')
    expect(tabs.map((t) => t.text())).toEqual(['Apps', 'Account', 'Security', 'Tokens'])
    expect(tabs.filter((t) => t.attributes('aria-current') === 'page').map((t) => t.text())).toEqual(['Account'])
  })

  it('marks a tab current on the pages below it', async () => {
    const { wrapper } = await mountAt('/security/password', personalPage())
    expect(wrapper.get('nav.tabs a[aria-current="page"]').text()).toBe('Security')
  })

  it('offers no Admin view to anyone else', async () => {
    const { wrapper } = await mountAt('/account', personalPage())
    expect(wrapper.find('.viewsw').exists()).toBe(false)
  })

  it('lets an Admin switch between the Personal and Admin views', async () => {
    const { wrapper } = await mountAt('/account', personalPage(admin()))
    const views = wrapper.findAll('.viewsw a')
    expect(views.map((v) => [v.text(), v.attributes('href'), v.attributes('aria-current')])).toEqual([
      ['Personal', '/', 'true'],
      ['Admin', '/admin', undefined],
    ])
  })

  it('shows the Admin tabs on an Admin page, with the people waiting for a Group', async () => {
    const { wrapper } = await mountAt('/admin/groups', adminPage())
    expect(wrapper.findAll('nav.tabs a').map((t) => t.text())).toEqual(['Dashboard', 'Unassigned1', 'Groups', 'Apps', 'People', 'Audit'])
    expect(wrapper.get('.viewsw a[aria-current="true"]').text()).toBe('Admin')
    expect(wrapper.get('.viewsw .dot').attributes('aria-label')).toBe('1 waiting')
  })

  it('leaves the count off when nobody waits', async () => {
    const { wrapper } = await mountAt('/admin/groups', { ...adminPage(), 'GET /api/v1/dashboard': () => Response.json(dashboard({ unassigned: [] })) })
    expect(wrapper.find('nav.tabs .count').exists()).toBe(false)
    expect(wrapper.find('.viewsw .dot').exists()).toBe(false)
  })

  it('names who is signed in in the menu', async () => {
    const { wrapper } = await mountAt('/account', personalPage())
    expect(wrapper.get('.sheet-who b').text()).toBe('Ada Lovelace')
    expect(wrapper.get('.sheet-who small').text()).toBe('ada@example.org')
  })

  it('signs out and leaves for the home page', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, requests } = await mountAt('/account', { ...personalPage(), 'DELETE /api/v1/session': new Response(null, { status: 204 }) })
    await wrapper.get('.sheet button.row').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === 'DELETE /api/v1/session')).toBe(true)
    expect(leave).toHaveBeenCalledWith('/')
    leave.mockRestore()
  })

  it('sends someone signed out to sign in', async () => {
    const { router } = await mountAt('/account', { ...personalPage(), 'GET /api/v1/profile': problem(401, 'Sign in first.') })
    expect(router.currentRoute.value.path).toBe('/signin')
  })
})

describe('the Admin pages', () => {
  it('send someone who is not an Admin to their apps', async () => {
    const { router } = await mountAt('/admin/groups', adminPage(profile()))
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('send an Admin without two-factor to set it up first', async () => {
    const { router } = await mountAt('/admin/groups', adminPage({ ...admin(), secondFactor: false }))
    expect(router.currentRoute.value.fullPath).toBe('/security/two-factor?for=admin')
  })

  it('open for an Admin with two-factor', async () => {
    const { router } = await mountAt('/admin/groups', adminPage())
    expect(router.currentRoute.value.path).toBe('/admin/groups')
  })
})
