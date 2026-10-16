import { afterEach, describe, expect, it } from 'vitest'
import { mountAt, problem, settle } from '../../app/testing'

const phone = { id: '00000000-0000-4000-8000-0000000000f1', kind: 'totp', label: 'Phone', createdAt: '2026-10-16T12:00:00Z', lastUsedAt: '2026-10-16T18:00:00Z' }
const setup = {
  secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
  uri: 'otpauth://totp/jorisjonkers.dev:ada@example.org?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=jorisjonkers.dev',
}
const ok = () => new Response(null, { status: 204 })

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the Security page', () => {
  it('says loading, then that there are no apps yet', async () => {
    let answer: (r: Response) => void = () => undefined
    const { wrapper } = await mountAt('/security', { 'GET /api/v1/profile/second-factors': () => new Promise<Response>((r) => (answer = r)) })
    expect(wrapper.text()).toContain('Loading…')
    answer(Response.json({ factors: [] }))
    await settle()
    expect(wrapper.text()).toContain('You have no authenticator apps yet.')
  })

  it('offers sign-in when nobody is signed in', async () => {
    const { wrapper } = await mountAt('/security', { 'GET /api/v1/profile/second-factors': problem(401, 'Sign in first.') })
    expect(wrapper.get('[role="alert"]').text()).toContain('Sign in first.')
  })

  it('adds an authenticator app from its QR code, its first code and the password', async () => {
    let factors: (typeof phone)[] = []
    const { wrapper, requests } = await mountAt('/security', {
      'GET /api/v1/profile/second-factors': () => Response.json({ factors }),
      'POST /api/v1/profile/second-factors/totp': Response.json(setup),
      'POST /api/v1/profile/second-factors/totp/confirmation': () => {
        factors = [phone]
        return ok()
      },
    })
    await wrapper.get('button[type="button"]').trigger('click')
    await settle()
    expect(wrapper.get('svg[role="img"]').attributes('aria-label')).toBe('QR code for your authenticator app')
    expect(wrapper.findAll('svg rect').length).toBeGreaterThan(100)
    expect(wrapper.get('code').text()).toBe(setup.secret)
    await wrapper.get('#totp-label').setValue('Phone')
    await wrapper.get('#totp-code').setValue('492 039')
    await wrapper.get('#totp-password').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/second-factors/totp/confirmation')?.body).toEqual({
      label: 'Phone',
      code: '492039',
      password: 'correct horse battery',
    })
    expect(wrapper.get('[role="status"]').text()).toContain('Authenticator app added.')
    expect(wrapper.get('ul').text()).toContain('Phone')
    expect(wrapper.get('ul').text()).toContain('last used')
    expect(wrapper.find('svg').exists()).toBe(false)
  })

  it('shows a refused start and a refused confirmation', async () => {
    const { wrapper } = await mountAt('/security', {
      'GET /api/v1/profile/second-factors': Response.json({ factors: [] }),
      'POST /api/v1/profile/second-factors/totp': problem(503, "Authenticator apps can't be used right now."),
    })
    await wrapper.get('button[type="button"]').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe("Authenticator apps can't be used right now.")

    const second = await mountAt('/security', {
      'GET /api/v1/profile/second-factors': Response.json({ factors: [] }),
      'POST /api/v1/profile/second-factors/totp': Response.json(setup),
      'POST /api/v1/profile/second-factors/totp/confirmation': problem(422, 'Your password is wrong. Enter it again.'),
    })
    await second.wrapper.get('button[type="button"]').trigger('click')
    await settle()
    await second.wrapper.get('#totp-label').setValue('Phone')
    await second.wrapper.get('#totp-code').setValue('492039')
    await second.wrapper.get('#totp-password').setValue('wrong')
    await second.wrapper.get('form').trigger('submit')
    await settle()
    expect(second.wrapper.get('[role="alert"]').text()).toBe('Your password is wrong. Enter it again.')
  })

  it('removes an app after asking for the password, or keeps it', async () => {
    let factors = [{ ...phone, lastUsedAt: undefined }]
    const { wrapper, requests } = await mountAt('/security', {
      'GET /api/v1/profile/second-factors': () => Response.json({ factors }),
      [`POST /api/v1/profile/second-factors/${phone.id}/removal`]: () => {
        factors = []
        return ok()
      },
    })
    expect(wrapper.get('ul').text()).not.toContain('last used')
    const removeButton = () => wrapper.get('li button[type="button"]')
    await removeButton().trigger('click')
    await wrapper.get('li button[type="button"]').trigger('click')
    expect(wrapper.find(`#remove-${phone.id}`).exists()).toBe(false)

    await removeButton().trigger('click')
    await wrapper.get('li form').trigger('submit')
    await settle()
    await wrapper.get(`#remove-${phone.id}`).setValue('correct horse battery')
    await wrapper.get('li form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === `POST /api/v1/profile/second-factors/${phone.id}/removal`)?.body).toEqual({ password: 'correct horse battery' })
    expect(wrapper.text()).toContain('You have no authenticator apps yet.')
  })

  it('shows a refused removal', async () => {
    const { wrapper } = await mountAt('/security', {
      'GET /api/v1/profile/second-factors': Response.json({ factors: [phone] }),
      [`POST /api/v1/profile/second-factors/${phone.id}/removal`]: problem(422, 'Your password is wrong. Enter it again.'),
    })
    await wrapper.get('li button[type="button"]').trigger('click')
    await wrapper.get(`#remove-${phone.id}`).setValue('wrong')
    await wrapper.get('li form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Your password is wrong. Enter it again.')
  })

  it('tells an Admin sent here nothing until it knows their apps', async () => {
    const { wrapper } = await mountAt('/security?for=admin', { 'GET /api/v1/profile/second-factors': () => new Promise<Response>(() => undefined) })
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('tells an Admin sent here what to do', async () => {
    const none = await mountAt('/security?for=admin', { 'GET /api/v1/profile/second-factors': Response.json({ factors: [] }) })
    expect(none.wrapper.get('[role="status"]').text()).toContain('Admins need an authenticator app')
    document.body.innerHTML = ''
    const some = await mountAt('/security?for=admin', { 'GET /api/v1/profile/second-factors': Response.json({ factors: [phone] }) })
    expect(some.wrapper.get('[role="status"]').text()).toContain('Sign out, then sign in again')
  })
})
