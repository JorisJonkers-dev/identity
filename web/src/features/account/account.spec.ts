import { afterEach, describe, expect, it, vi } from 'vitest'
import { mountAt, problem, settle } from '../../app/testing'
import { navigation } from '../../infrastructure/navigation'

const profile = { email: 'ada@example.org', name: 'Ada', createdAt: '2026-10-15T12:00:00Z', admin: false }

afterEach(() => {
  document.body.innerHTML = ''
})

describe('signing up', () => {
  it('sends the form and says to check the inbox', async () => {
    const { wrapper, requests } = await mountAt('/signup', { 'POST /api/v1/signup': new Response(null, { status: 202 }) })
    await wrapper.get('#signup-email').setValue('ada@example.org')
    await wrapper.get('#signup-name').setValue('Ada')
    await wrapper.get('#signup-password').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.at(-1)).toEqual({
      key: 'POST /api/v1/signup',
      body: { email: 'ada@example.org', name: 'Ada', password: 'correct horse battery' },
    })
    expect(wrapper.get('[role="status"]').text()).toBe('Check your email for a link to finish signing up.')
  })

  it('shows what to fix', async () => {
    const { wrapper } = await mountAt('/signup', { 'POST /api/v1/signup': problem(422, 'Enter a password of 12 to 128 characters.') })
    await wrapper.get('#signup-email').setValue('ada@example.org')
    await wrapper.get('#signup-name').setValue('Ada')
    await wrapper.get('#signup-password').setValue('short')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Enter a password of 12 to 128 characters.')
  })
})

describe('confirming an email', () => {
  it('confirms the token from the link with the sign-up password', async () => {
    const { wrapper, requests } = await mountAt('/confirm?token=abc', { 'POST /api/v1/confirmations': new Response(null, { status: 204 }) })
    expect(requests).toEqual([])
    await wrapper.get('#confirm-password').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.at(-1)).toEqual({ key: 'POST /api/v1/confirmations', body: { token: 'abc', password: 'correct horse battery' } })
    expect(wrapper.get('[role="status"]').text()).toContain('Your email is confirmed.')
  })

  it('says when the password or the link is wrong', async () => {
    const { wrapper } = await mountAt('/confirm?token=abc', {
      'POST /api/v1/confirmations': problem(422, 'That is not the password you chose when you signed up. Try again.'),
    })
    await wrapper.get('#confirm-password').setValue('wrong password!!')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That is not the password you chose when you signed up. Try again.')
  })

  it('says when the link has no token, without asking the server', async () => {
    const { wrapper, requests } = await mountAt('/confirm', {})
    expect(requests).toEqual([])
    expect(wrapper.get('[role="alert"]').text()).toBe('This link is incomplete. Open the link in your email again.')
  })
})

describe('signing in', () => {
  it('goes to the profile once signed in', async () => {
    const { wrapper, router, requests } = await mountAt('/signin', {
      'POST /api/v1/session': new Response(null, { status: 204 }),
      'GET /api/v1/profile': Response.json(profile),
    })
    await wrapper.get('#signin-email').setValue('ada@example.org')
    await wrapper.get('#signin-password').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/session')?.body).toEqual({ email: 'ada@example.org', password: 'correct horse battery' })
    expect(router.currentRoute.value.path).toBe('/profile')
  })

  it('hands back to the Provider when an app sent the person here', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, router } = await mountAt('/signin?authRequestID=a1b2', { 'POST /api/v1/session': new Response(null, { status: 204 }) })
    await wrapper.get('#signin-email').setValue('ada@example.org')
    await wrapper.get('#signin-password').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(leave).toHaveBeenCalledWith('/oidc/login?authRequestID=a1b2')
    expect(router.currentRoute.value.path).toBe('/signin')
    leave.mockRestore()
  })

  it('says when the email or the password is wrong', async () => {
    const { wrapper } = await mountAt('/signin', { 'POST /api/v1/session': problem(401, 'The email or the password is wrong.') })
    await wrapper.get('#signin-email').setValue('ada@example.org')
    await wrapper.get('#signin-password').setValue('wrong password!!')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('The email or the password is wrong.')
  })
})

describe('the profile', () => {
  it('says loading before the profile arrives', async () => {
    const { wrapper } = await mountAt('/profile', { 'GET /api/v1/profile': () => new Promise<Response>(() => undefined) })
    expect(wrapper.text()).toContain('Loading…')
  })

  it('offers to sign in when nobody is signed in', async () => {
    const { wrapper } = await mountAt('/profile', { 'GET /api/v1/profile': problem(401, 'Sign in first.') })
    expect(wrapper.get('[role="alert"]').text()).toContain('Sign in first.')
    expect(wrapper.get('[role="alert"] a').attributes('href')).toBe('/signin')
  })

  it('shows who is signed in, and renames them', async () => {
    let name = 'Ada'
    const { wrapper, requests } = await mountAt('/profile', {
      'GET /api/v1/profile': () => Response.json({ ...profile, name }),
      'PATCH /api/v1/profile': () => {
        name = 'Ada King'
        return Response.json({ ...profile, name })
      },
    })
    expect(wrapper.get('dl').text()).toContain('ada@example.org')
    expect(wrapper.find('a[href="/access"]').exists()).toBe(false)
    await wrapper.get('#profile-name').setValue('Ada King')
    await wrapper.findAll('form')[0]?.trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'PATCH /api/v1/profile')?.body).toEqual({ name: 'Ada King' })
    expect(wrapper.get('dl').text()).toContain('Ada King')
  })

  it('offers an Admin the access page', async () => {
    const { wrapper } = await mountAt('/profile', { 'GET /api/v1/profile': Response.json({ ...profile, admin: true }) })
    expect(wrapper.get('a[href="/access"]').text()).toBe('Manage access')
  })

  it('shows a refused rename', async () => {
    const { wrapper } = await mountAt('/profile', {
      'GET /api/v1/profile': Response.json(profile),
      'PATCH /api/v1/profile': problem(422, 'Enter a name of 1 to 100 characters.'),
    })
    await wrapper.get('#profile-name').setValue('   ')
    await wrapper.findAll('form')[0]?.trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Enter a name of 1 to 100 characters.')
  })

  it('changes the password, clears the fields and says other sessions ended', async () => {
    const { wrapper, requests } = await mountAt('/profile', {
      'GET /api/v1/profile': Response.json(profile),
      'POST /api/v1/profile/password': new Response(null, { status: 204 }),
    })
    const current = wrapper.get<HTMLInputElement>('#profile-current')
    await current.setValue('old password!!')
    await wrapper.get('#profile-next').setValue('a new password!')
    await wrapper.findAll('form')[1]?.trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/password')?.body).toEqual({ current: 'old password!!', next: 'a new password!' })
    expect(current.element.value).toBe('')
    expect(wrapper.get('[role="status"]').text()).toBe('Your password has been changed. Your other sessions are signed out.')
  })

  it('shows a refused password change', async () => {
    const { wrapper } = await mountAt('/profile', {
      'GET /api/v1/profile': Response.json(profile),
      'POST /api/v1/profile/password': problem(422, 'Your current password is wrong. Enter it again.'),
    })
    await wrapper.get('#profile-current').setValue('wrong password!!')
    await wrapper.get('#profile-next').setValue('a new password!')
    await wrapper.findAll('form')[1]?.trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Your current password is wrong. Enter it again.')
  })

  it('signs out and goes to sign in', async () => {
    const { wrapper, router } = await mountAt('/profile', {
      'GET /api/v1/profile': Response.json(profile),
      'DELETE /api/v1/session': new Response(null, { status: 204 }),
    })
    await wrapper.get('button[type="button"]').trigger('click')
    await settle()
    expect(router.currentRoute.value.path).toBe('/signin')
  })
})
