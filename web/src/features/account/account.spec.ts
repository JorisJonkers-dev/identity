import type { VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { profile } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import { navigation } from '../../infrastructure/navigation'
import { passkeys } from '../../infrastructure/passkeys'

const ada = Response.json(profile())
const noContent = () => new Response(null, { status: 204 })

function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

async function signIn(wrapper: VueWrapper, password = 'correct horse battery') {
  await wrapper.get('#si-email').setValue('ada@example.org')
  await wrapper.get('#si-pw').setValue(password)
  await wrapper.get('form').trigger('submit')
  await settle()
}

async function typeCode(wrapper: VueWrapper, code: string) {
  await wrapper.get('[aria-label="Digit 1 of 6"]').setValue(code)
  await wrapper.get('form').trigger('submit')
  await settle()
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('signing up', () => {
  async function fill(wrapper: VueWrapper, password: string) {
    await wrapper.get('#su-name').setValue('Ada')
    await wrapper.get('#su-email').setValue('ada@example.org')
    await wrapper.get('#su-pw').setValue(password)
    await wrapper.get('form').trigger('submit')
    await settle()
  }

  it('sends the form and says to check the inbox', async () => {
    const { wrapper, requests } = await mountAt('/signup', { 'POST /api/v1/signup': new Response(null, { status: 202 }) })
    await fill(wrapper, 'correct horse battery')
    expect(requests.at(-1)).toEqual({ key: 'POST /api/v1/signup', body: { email: 'ada@example.org', name: 'Ada', password: 'correct horse battery' } })
    expect(wrapper.get('h1').text()).toBe('Check your email')
    expect(wrapper.get('.lead').text()).toBe('Link sent to ada@example.org.')
  })

  it('asks for a longer password without asking the server', async () => {
    const { wrapper, requests } = await mountAt('/signup', {})
    await fill(wrapper, 'short')
    expect(requests).toEqual([])
    expect(wrapper.get('#su-short').text()).toBe('Use at least 12 characters.')
  })

  it('shows what the server refused', async () => {
    const { wrapper } = await mountAt('/signup', { 'POST /api/v1/signup': problem(422, 'Enter a valid email address.') })
    await fill(wrapper, 'correct horse battery')
    expect(wrapper.get('[role="alert"]').text()).toBe('Enter a valid email address.')
  })

  it('shows the password on request', async () => {
    const { wrapper } = await mountAt('/signup', {})
    await wrapper.get('[aria-label="Show password"]').trigger('click')
    expect(wrapper.get('#su-pw').attributes('type')).toBe('text')
  })
})

describe('confirming an email', () => {
  it('confirms the token from the link with the sign-up password', async () => {
    const { wrapper, requests } = await mountAt('/confirm?token=abc', { 'POST /api/v1/confirmations': noContent() })
    expect(requests).toEqual([])
    await wrapper.get('#cf-pw').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.at(-1)).toEqual({ key: 'POST /api/v1/confirmations', body: { token: 'abc', password: 'correct horse battery' } })
    expect(wrapper.get('h1').text()).toBe('Email confirmed')
    expect(wrapper.get('a.btn').attributes('href')).toBe('/signin')
  })

  it('says when the password or the link is wrong', async () => {
    const { wrapper } = await mountAt('/confirm?token=abc', {
      'POST /api/v1/confirmations': problem(422, 'That is not the password you chose when you signed up. Try again.'),
    })
    await wrapper.get('#cf-pw').setValue('wrong password!!')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That is not the password you chose when you signed up. Try again.')
  })

  it('says when the link has no token, without asking the server', async () => {
    const { wrapper, requests } = await mountAt('/confirm', {})
    expect(requests).toEqual([])
    expect(wrapper.get('h1').text()).toBe('This link is incomplete')
  })

  it('shows the password on request', async () => {
    const { wrapper } = await mountAt('/confirm?token=abc', {})
    await wrapper.get('[aria-label="Show password"]').trigger('click')
    expect(wrapper.get('#cf-pw').attributes('type')).toBe('text')
  })
})

describe('signing in', () => {
  it('goes to the apps once signed in', async () => {
    const { wrapper, router, requests } = await mountAt('/signin', { 'POST /api/v1/session': noContent(), 'GET /api/v1/profile': ada })
    await signIn(wrapper)
    expect(requests.find((r) => r.key === 'POST /api/v1/session')?.body).toEqual({ email: 'ada@example.org', password: 'correct horse battery' })
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('puts the cursor in the email field', async () => {
    const { wrapper } = await mountAt('/signin', {}, { attach: true })
    expect(document.activeElement).toBe(wrapper.get('#si-email').element)
  })

  it('hands back to the Provider when an app sent the person here', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, router } = await mountAt('/signin?authRequestID=a1b2', { 'POST /api/v1/session': noContent() })
    await signIn(wrapper)
    expect(leave).toHaveBeenCalledWith('/oidc/login?authRequestID=a1b2')
    expect(router.currentRoute.value.path).toBe('/signin')
    leave.mockRestore()
  })

  it('goes back to the Gate when a gated app sent the person here', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const gate = '/gate/start?host=stats.jorisjonkers.dev&state=s&rd=%2F'
    const { wrapper } = await mountAt(`/signin?gate=${encodeURIComponent(gate)}`, { 'POST /api/v1/session': noContent() })
    await signIn(wrapper)
    expect(leave).toHaveBeenCalledWith(gate)
    leave.mockRestore()
  })

  it('ignores a gate address that is not the Gate', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, router } = await mountAt(`/signin?gate=${encodeURIComponent('https://evil.test/')}`, {
      'POST /api/v1/session': noContent(),
      'GET /api/v1/profile': ada,
    })
    await signIn(wrapper)
    expect(leave).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/')
    leave.mockRestore()
  })

  it('asks for the code when the User has an authenticator app, then signs in', async () => {
    const { wrapper, router, requests } = await mountAt('/signin', {
      'POST /api/v1/session': Response.json({ methods: ['totp'] }, { status: 202 }),
      'POST /api/v1/session/two-factor': noContent(),
      'GET /api/v1/profile': ada,
    })
    await signIn(wrapper)
    expect(wrapper.get('h1').text()).toBe('Enter your code')
    expect(wrapper.get('.from.who').text()).toBe('ada@example.org')
    expect(wrapper.text()).not.toContain('Use your passkey')
    await typeCode(wrapper, '492039')
    expect(requests.find((r) => r.key === 'POST /api/v1/session/two-factor')?.body).toEqual({ code: '492039' })
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('puts the cursor in the first code box', async () => {
    const { wrapper } = await mountAt('/signin', { 'POST /api/v1/session': Response.json({ methods: ['totp'] }, { status: 202 }) }, { attach: true })
    await signIn(wrapper)
    expect(document.activeElement).toBe(wrapper.get('[aria-label="Digit 1 of 6"]').element)
  })

  it('says when the code is wrong', async () => {
    const { wrapper } = await mountAt('/signin', {
      'POST /api/v1/session': Response.json({ methods: ['totp'] }, { status: 202 }),
      'POST /api/v1/session/two-factor': problem(422, 'That code is wrong or was already used.'),
    })
    await signIn(wrapper)
    await typeCode(wrapper, '000000')
    expect(wrapper.get('[role="alert"]').text()).toBe('That code is wrong or was already used.')
    expect(wrapper.get('[aria-label="Digit 1 of 6"]').attributes('aria-invalid')).toBe('true')
  })

  it('signs in with a passkey when the User has one', async () => {
    const get = vi.spyOn(passkeys, 'get').mockResolvedValue('{"id":"cred-1"}')
    const { wrapper, router, requests } = await mountAt('/signin', {
      'POST /api/v1/session': Response.json({ methods: ['passkey'] }, { status: 202 }),
      'POST /api/v1/session/passkey': Response.json({ options: '{"publicKey":{}}' }),
      'POST /api/v1/session/passkey/assertion': noContent(),
      'GET /api/v1/profile': ada,
    })
    await signIn(wrapper)
    expect(wrapper.get('h1').text()).toBe('Use your passkey')
    expect(wrapper.find('[aria-label="Digit 1 of 6"]').exists()).toBe(false)
    await button(wrapper, 'Use your passkey').trigger('click')
    await settle()
    expect(get).toHaveBeenCalledWith('{"publicKey":{}}')
    expect(requests.find((r) => r.key === 'POST /api/v1/session/passkey/assertion')?.body).toEqual({ response: '{"id":"cred-1"}' })
    expect(router.currentRoute.value.path).toBe('/')
    get.mockRestore()
  })

  it('offers the code and the passkey when the User has both', async () => {
    const { wrapper } = await mountAt('/signin', { 'POST /api/v1/session': Response.json({ methods: ['totp', 'passkey'] }, { status: 202 }) })
    await signIn(wrapper)
    expect(wrapper.find('[aria-label="Digit 1 of 6"]').exists()).toBe(true)
    expect(wrapper.get('.or').text()).toBe('or')
    expect(button(wrapper, 'Use your passkey').exists()).toBe(true)
  })

  it('says when the browser did not use the passkey', async () => {
    const get = vi.spyOn(passkeys, 'get').mockRejectedValue(new Error('NotAllowedError'))
    const { wrapper } = await mountAt('/signin', {
      'POST /api/v1/session': Response.json({ methods: ['passkey'] }, { status: 202 }),
      'POST /api/v1/session/passkey': Response.json({ options: '{"publicKey":{}}' }),
    })
    await signIn(wrapper)
    await button(wrapper, 'Use your passkey').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Passkey sign-in did not finish. Try again.')
    get.mockRestore()
  })

  it('says when the email or the password is wrong', async () => {
    const { wrapper } = await mountAt('/signin', { 'POST /api/v1/session': problem(401, 'The email or the password is wrong.') })
    await signIn(wrapper, 'wrong password!!')
    expect(wrapper.get('[role="alert"]').text()).toBe('The email or the password is wrong.')
    expect(wrapper.get('#si-pw').attributes('aria-invalid')).toBe('true')
  })

  it('shows the password on request, and hides it again', async () => {
    const { wrapper } = await mountAt('/signin', {})
    await wrapper.get('[aria-label="Show password"]').trigger('click')
    expect(wrapper.get('#si-pw').attributes('type')).toBe('text')
    await wrapper.get('[aria-label="Hide password"]').trigger('click')
    expect(wrapper.get('#si-pw').attributes('type')).toBe('password')
  })

  describe('the code boxes', () => {
    const codeStep = async () => {
      const mounted = await mountAt('/signin', { 'POST /api/v1/session': Response.json({ methods: ['totp'] }, { status: 202 }) }, { attach: true })
      await signIn(mounted.wrapper)
      return mounted.wrapper
    }
    const box = (wrapper: VueWrapper, n: number) => wrapper.get(`[aria-label="Digit ${String(n)} of 6"]`)

    it('moves on to the next box with each digit', async () => {
      const wrapper = await codeStep()
      await box(wrapper, 1).setValue('4')
      expect(document.activeElement).toBe(box(wrapper, 2).element)
      expect(box(wrapper, 1).element).toHaveProperty('value', '4')
    })

    it('ignores anything that is not a digit', async () => {
      const wrapper = await codeStep()
      await box(wrapper, 1).setValue('a')
      expect(box(wrapper, 1).element).toHaveProperty('value', '')
      expect(document.activeElement).toBe(box(wrapper, 1).element)
    })

    it('goes back a box on backspace in an empty one', async () => {
      const wrapper = await codeStep()
      await box(wrapper, 1).setValue('4')
      await box(wrapper, 2).trigger('keydown', { key: 'Backspace' })
      expect(document.activeElement).toBe(box(wrapper, 1).element)
    })

    it('stays put on backspace in a filled box', async () => {
      const wrapper = await codeStep()
      await box(wrapper, 1).setValue('4')
      await box(wrapper, 2).setValue('9')
      ;(box(wrapper, 2).element as HTMLInputElement).focus()
      await box(wrapper, 2).trigger('keydown', { key: 'Backspace' })
      expect(document.activeElement).toBe(box(wrapper, 2).element)
    })

    it('selects a box’s digit when it takes the cursor, so typing replaces it', async () => {
      const wrapper = await codeStep()
      await box(wrapper, 1).setValue('4')
      const first = box(wrapper, 1).element as HTMLInputElement
      first.focus()
      await box(wrapper, 1).trigger('focus')
      expect([first.selectionStart, first.selectionEnd]).toEqual([0, 1])
    })
  })
})

describe('proving two-factor again for an app', () => {
  const asks = (methods: string[]) => ({ 'POST /api/v1/session/step-up': Response.json({ methods }) })

  it('asks for a code, then hands back to the Provider', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const { wrapper, requests } = await mountAt('/signin/step-up?authRequestID=a1b2', { ...asks(['totp']), 'POST /api/v1/session/step-up/code': noContent() })
    expect(wrapper.get('h1').text()).toBe('Enter your code')
    expect(wrapper.get('.auth-box p.lead').text()).toBe('This app asks for two-factor again before it opens.')
    await typeCode(wrapper, '492039')
    expect(requests.find((r) => r.key === 'POST /api/v1/session/step-up/code')?.body).toEqual({ code: '492039' })
    expect(leave).toHaveBeenCalledWith('/oidc/login?authRequestID=a1b2')
    leave.mockRestore()
  })

  it('puts the cursor in the first code box', async () => {
    const { wrapper } = await mountAt('/signin/step-up?authRequestID=a1b2', asks(['totp']), { attach: true })
    expect(document.activeElement).toBe(wrapper.get('[aria-label="Digit 1 of 6"]').element)
  })

  it('proves it with a passkey', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const get = vi.spyOn(passkeys, 'get').mockResolvedValue('{"id":"cred-1"}')
    const { wrapper, requests } = await mountAt('/signin/step-up?authRequestID=a1b2', {
      ...asks(['passkey']),
      'POST /api/v1/session/step-up/passkey': Response.json({ options: '{"publicKey":{}}' }),
      'POST /api/v1/session/step-up/passkey/assertion': noContent(),
    })
    expect(wrapper.get('h1').text()).toBe('Use your passkey')
    await button(wrapper, 'Use your passkey').trigger('click')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/session/step-up/passkey/assertion')?.body).toEqual({ response: '{"id":"cred-1"}' })
    expect(leave).toHaveBeenCalledWith('/oidc/login?authRequestID=a1b2')
    get.mockRestore()
    leave.mockRestore()
  })

  it('says when the browser did not use the passkey', async () => {
    const get = vi.spyOn(passkeys, 'get').mockRejectedValue(new Error('NotAllowedError'))
    const { wrapper } = await mountAt('/signin/step-up?authRequestID=a1b2', { ...asks(['passkey']), 'POST /api/v1/session/step-up/passkey': Response.json({ options: '{"publicKey":{}}' }) })
    await button(wrapper, 'Use your passkey').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Passkey check was cancelled. Try again.')
    get.mockRestore()
  })

  it('says when the code is wrong', async () => {
    const { wrapper } = await mountAt('/signin/step-up?authRequestID=a1b2', { ...asks(['totp']), 'POST /api/v1/session/step-up/code': problem(422, 'That code is wrong or was already used.') })
    await typeCode(wrapper, '000000')
    expect(wrapper.get('[role="alert"]').text()).toBe('That code is wrong or was already used.')
  })

  it('sends someone with no two-factor to set it up', async () => {
    const { wrapper } = await mountAt('/signin/step-up?authRequestID=a1b2', { 'POST /api/v1/session/step-up': problem(409, 'Add a passkey or an authenticator app on the Security page first.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Add a passkey or an authenticator app on the Security page first.')
    expect(wrapper.get('a[href="/security/two-factor?for=step-up"]').text()).toBe('Set up two-factor')
  })

  it('signs in first when the session has ended', async () => {
    const { router } = await mountAt('/signin/step-up?authRequestID=a1b2', { 'POST /api/v1/session/step-up': problem(401, 'Sign in.') })
    expect(router.currentRoute.value.fullPath).toBe('/signin?authRequestID=a1b2')
  })

  it('offers the code and the passkey when the User has both', async () => {
    const { wrapper } = await mountAt('/signin/step-up?authRequestID=a1b2', asks(['totp', 'passkey']))
    expect(wrapper.find('[aria-label="Digit 1 of 6"]').exists()).toBe(true)
    expect(wrapper.get('.or').text()).toBe('or')
  })

  it('signs in first, to go to the apps, when the session has ended and no app asked', async () => {
    const { router } = await mountAt('/signin/step-up', { 'POST /api/v1/session/step-up': problem(401, 'Sign in.') })
    expect(router.currentRoute.value.fullPath).toBe('/signin')
  })

  it('goes back to the page that asked for it', async () => {
    const { wrapper, router } = await mountAt('/signin/step-up?next=/security/delete', { ...asks(['totp']), 'POST /api/v1/session/step-up/code': noContent(), 'GET /api/v1/profile': ada })
    await typeCode(wrapper, '492039')
    expect(router.currentRoute.value.path).toBe('/security/delete')
  })

  it('goes back to the Gate when a gated app asked', async () => {
    const leave = vi.spyOn(navigation, 'leave').mockImplementation(() => undefined)
    const gate = '/gate/start?host=stats.jorisjonkers.dev&state=s&rd=%2F'
    const { wrapper } = await mountAt(`/signin/step-up?next=${encodeURIComponent(gate)}`, { ...asks(['totp']), 'POST /api/v1/session/step-up/code': noContent() })
    await typeCode(wrapper, '492039')
    expect(leave).toHaveBeenCalledWith(gate)
    leave.mockRestore()
  })

  it('ignores a page to go back to that is not one of ours', async () => {
    const { wrapper, router } = await mountAt('/signin/step-up?next=//evil.test/', { ...asks(['totp']), 'POST /api/v1/session/step-up/code': noContent(), 'GET /api/v1/profile': ada })
    await typeCode(wrapper, '492039')
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('goes to the apps when no app asked', async () => {
    const { wrapper, router } = await mountAt('/signin/step-up', { ...asks(['totp']), 'POST /api/v1/session/step-up/code': noContent(), 'GET /api/v1/profile': ada })
    await typeCode(wrapper, '492039')
    expect(router.currentRoute.value.path).toBe('/')
  })
})

describe('confirming a new email', () => {
  it('changes the sign-in email once asked', async () => {
    const { wrapper, requests } = await mountAt('/account/email?token=t0k', { 'POST /api/v1/email-changes/confirmation': noContent() })
    expect(wrapper.get('h1').text()).toBe('Use this email to sign in?')
    await button(wrapper, 'Change my sign-in email').trigger('click')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/email-changes/confirmation')?.body).toEqual({ token: 't0k' })
    expect(wrapper.get('h1').text()).toBe('Email changed')
    expect(wrapper.get('a.btn').attributes('href')).toBe('/account')
  })

  it('says when the link is spent', async () => {
    const { wrapper } = await mountAt('/account/email?token=t0k', { 'POST /api/v1/email-changes/confirmation': problem(422, 'This link has expired or was already used. Ask for a new one on the Account page.') })
    await button(wrapper, 'Change my sign-in email').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('This link has expired or was already used. Ask for a new one on the Account page.')
  })

  it('says when the link has no token, without asking the server', async () => {
    const { wrapper, requests } = await mountAt('/account/email', {})
    expect(wrapper.get('h1').text()).toBe('This link is incomplete')
    expect(requests).toHaveLength(0)
  })
})

describe('resetting a forgotten password', () => {
  const check = (methods: string[]) => ({ 'POST /api/v1/password-resets/check': Response.json({ methods }) })
  async function choose(wrapper: VueWrapper, password = 'a brand new passphrase') {
    await wrapper.get('#rs-pw').setValue(password)
    await wrapper.get('form').trigger('submit')
    await settle()
  }

  it('is offered on the sign-in page', async () => {
    const { wrapper } = await mountAt('/signin', {})
    expect(wrapper.get('a[href="/forgot"]').text()).toBe('Forgot password?')
  })

  it('asks for the email and says a link is on its way', async () => {
    const { wrapper, requests } = await mountAt('/forgot', { 'POST /api/v1/password-resets': new Response(null, { status: 202 }) })
    await wrapper.get('#fg-email').setValue('ada@example.org')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/password-resets')?.body).toEqual({ email: 'ada@example.org' })
    expect(wrapper.get('h1').text()).toBe('Check your email')
    expect(wrapper.get('.lead').text()).toBe('If ada@example.org has an account, we sent a link to reset its password. It works once, for 30 minutes.')
  })

  it('shows a refused request', async () => {
    const { wrapper } = await mountAt('/forgot', { 'POST /api/v1/password-resets': problem(503, 'Mail cannot be sent right now. Try again later.') })
    await wrapper.get('#fg-email').setValue('ada@example.org')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Mail cannot be sent right now. Try again later.')
  })

  it('takes a new password when there is no two-factor to prove', async () => {
    const { wrapper, requests } = await mountAt('/reset?token=t0k', { ...check([]), 'POST /api/v1/password-resets/completion': noContent() })
    expect(wrapper.get('h1').text()).toBe('Choose a new password')
    await choose(wrapper)
    expect(requests.find((r) => r.key === 'POST /api/v1/password-resets/completion')?.body).toEqual({ token: 't0k', password: 'a brand new passphrase' })
    expect(wrapper.get('h1').text()).toBe('Password changed')
    expect(wrapper.get('a.btn').attributes('href')).toBe('/signin')
  })

  it('asks for a code first when the User has an authenticator app', async () => {
    const { wrapper, requests } = await mountAt('/reset?token=t0k', { ...check(['totp']), 'POST /api/v1/password-resets/code': noContent(), 'POST /api/v1/password-resets/completion': noContent() })
    expect(wrapper.get('h1').text()).toBe('Enter your code')
    expect(wrapper.get('a[href="mailto:admin@jorisjonkers.dev"]').text()).toBe('admin@jorisjonkers.dev')
    await typeCode(wrapper, '492039')
    expect(requests.find((r) => r.key === 'POST /api/v1/password-resets/code')?.body).toEqual({ token: 't0k', code: '492039' })
    expect(wrapper.get('h1').text()).toBe('Choose a new password')
  })

  it('proves it with a passkey', async () => {
    const get = vi.spyOn(passkeys, 'get').mockResolvedValue('{"id":"cred-1"}')
    const { wrapper, requests } = await mountAt('/reset?token=t0k', {
      ...check(['passkey']),
      'POST /api/v1/password-resets/passkey': Response.json({ options: '{"publicKey":{}}' }),
      'POST /api/v1/password-resets/passkey/assertion': noContent(),
    })
    await button(wrapper, 'Use your passkey').trigger('click')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/password-resets/passkey/assertion')?.body).toEqual({ token: 't0k', response: '{"id":"cred-1"}' })
    expect(wrapper.get('h1').text()).toBe('Choose a new password')
    get.mockRestore()
  })

  it('says when the browser did not use the passkey', async () => {
    const get = vi.spyOn(passkeys, 'get').mockRejectedValue(new Error('NotAllowedError'))
    const { wrapper } = await mountAt('/reset?token=t0k', { ...check(['passkey']), 'POST /api/v1/password-resets/passkey': Response.json({ options: '{"publicKey":{}}' }) })
    await button(wrapper, 'Use your passkey').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Passkey check was cancelled. Try again.')
    get.mockRestore()
  })

  it('offers the code and the passkey when the User has both', async () => {
    const { wrapper } = await mountAt('/reset?token=t0k', check(['totp', 'passkey']))
    expect(wrapper.get('.or').text()).toBe('or')
  })

  it('says when the code is wrong', async () => {
    const { wrapper } = await mountAt('/reset?token=t0k', { ...check(['totp']), 'POST /api/v1/password-resets/code': problem(422, 'That code is wrong or was already used.') })
    await typeCode(wrapper, '000000')
    expect(wrapper.get('[role="alert"]').text()).toBe('That code is wrong or was already used.')
  })

  it('says when the new password is refused', async () => {
    const { wrapper } = await mountAt('/reset?token=t0k', { ...check([]), 'POST /api/v1/password-resets/completion': problem(422, 'Enter a password of 12 to 128 characters.') })
    await choose(wrapper, 'short')
    expect(wrapper.get('[role="alert"]').text()).toBe('Enter a password of 12 to 128 characters.')
  })

  it('says when the link is spent, and offers a new one', async () => {
    const { wrapper } = await mountAt('/reset?token=t0k', { 'POST /api/v1/password-resets/check': problem(422, 'This link has expired or was already used. Ask for a new one.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('This link has expired or was already used. Ask for a new one.')
    expect(wrapper.get('a[href="/forgot"]').text()).toBe('Ask for a new link')
  })

  it('says when the link has no token, without asking the server', async () => {
    const { wrapper, requests } = await mountAt('/reset', {})
    expect(wrapper.get('h1').text()).toBe('This link is incomplete')
    expect(requests).toHaveLength(0)
  })
})

describe('the Account page', () => {
  const answers = (over: Record<string, unknown> = {}) => ({
    'GET /api/v1/profile': () => Response.json(profile(over)),
    'GET /api/v1/profile/two-factor-methods': () => Response.json({ factors: [] }),
    'GET /api/v1/launcher': () => Response.json({ apps: [], heldBack: 0 }),
  })

  it('shows who is signed in', async () => {
    const { wrapper } = await mountAt('/account', answers())
    expect(wrapper.get('h1').text()).toBe('Ada Lovelace')
    expect(wrapper.get('.hero-t p').text()).toBe('ada@example.org')
    expect(wrapper.get('.pills').text()).toContain('Since Oct 2026')
  })

  it('marks an Admin', async () => {
    const { wrapper } = await mountAt('/account', answers({ admin: true, twoFactor: true }))
    expect(wrapper.get('.pill.accent').text()).toBe('Admin')
  })

  it('renames the User', async () => {
    let name = 'Ada Lovelace'
    const { wrapper, requests } = await mountAt('/account', {
      ...answers(),
      'GET /api/v1/profile': () => Response.json(profile({ name })),
      'PATCH /api/v1/profile': () => {
        name = 'Ada King'
        return Response.json(profile({ name }))
      },
    })
    await button(wrapper, 'Edit').trigger('click')
    await wrapper.get('#ac-name').setValue('Ada King')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'PATCH /api/v1/profile')?.body).toEqual({ name: 'Ada King' })
    expect(wrapper.get('.set-v b').text()).toBe('Ada King')
    expect(wrapper.get('.inline-ok').text()).toBe('Saved')
  })

  it('keeps the name when the rename is cancelled', async () => {
    const { wrapper, requests } = await mountAt('/account', answers())
    await button(wrapper, 'Edit').trigger('click')
    await wrapper.get('#ac-name').setValue('Someone else')
    await button(wrapper, 'Cancel').trigger('click')
    expect(requests.some((r) => r.key === 'PATCH /api/v1/profile')).toBe(false)
    expect(wrapper.get('.set-v b').text()).toBe('Ada Lovelace')
  })

  it('asks for a new email with the password, then says where the link went', async () => {
    let pendingEmail: string | undefined
    const { wrapper, requests } = await mountAt('/account', {
      ...answers(),
      'GET /api/v1/profile': () => Response.json(profile(pendingEmail ? { pendingEmail } : {})),
      'POST /api/v1/profile/email-change': () => {
        pendingEmail = 'lovelace@example.org'
        return new Response(null, { status: 202 })
      },
    })
    await button(wrapper, 'Change').trigger('click')
    await wrapper.get('#ac-email').setValue('lovelace@example.org')
    await wrapper.get('#ac-email-pw').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/email-change')?.body).toEqual({ email: 'lovelace@example.org', password: 'correct horse battery' })
    expect(wrapper.get('.set-row.pending small').text()).toBe('Waiting for you to open the link sent to lovelace@example.org')
  })

  it('shows a refused email change', async () => {
    const { wrapper } = await mountAt('/account', { ...answers(), 'POST /api/v1/profile/email-change': problem(422, 'Your password is wrong. Enter it again.') })
    await button(wrapper, 'Change').trigger('click')
    await wrapper.get('#ac-email').setValue('lovelace@example.org')
    await wrapper.get('#ac-email-pw').setValue('wrong')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Your password is wrong. Enter it again.')
    expect(wrapper.find('#ac-email').exists()).toBe(true)
  })

  it('keeps the email when the change is not asked for', async () => {
    const { wrapper, requests } = await mountAt('/account', answers())
    await button(wrapper, 'Change').trigger('click')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('#ac-email').exists()).toBe(false)
    expect(requests.some((r) => r.key.includes('email-change'))).toBe(false)
  })

  it('shows a refused cancel', async () => {
    const { wrapper } = await mountAt('/account', {
      ...answers({ pendingEmail: 'lovelace@example.org' }),
      'DELETE /api/v1/profile/email-change': problem(500, 'Something went wrong. Try again.'),
    })
    await button(wrapper, 'Cancel change').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('cancels a pending email change', async () => {
    let pendingEmail: string | undefined = 'lovelace@example.org'
    const { wrapper, requests } = await mountAt('/account', {
      ...answers(),
      'GET /api/v1/profile': () => Response.json(profile(pendingEmail ? { pendingEmail } : {})),
      'DELETE /api/v1/profile/email-change': () => {
        pendingEmail = undefined
        return noContent()
      },
    })
    await button(wrapper, 'Cancel change').trigger('click')
    await settle()
    expect(requests.some((r) => r.key === 'DELETE /api/v1/profile/email-change')).toBe(true)
    expect(wrapper.find('.set-row.pending').exists()).toBe(false)
  })

  it('shows a refused rename', async () => {
    const { wrapper } = await mountAt('/account', { ...answers(), 'PATCH /api/v1/profile': problem(422, 'Enter a name of 1 to 100 characters.') })
    await button(wrapper, 'Edit').trigger('click')
    await wrapper.get('#ac-name').setValue('   ')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Enter a name of 1 to 100 characters.')
  })

  it('says whether two-factor is on', async () => {
    const { wrapper } = await mountAt('/account', {
      ...answers(),
      'GET /api/v1/profile/two-factor-methods': () =>
        Response.json({ factors: [{ id: '00000000-0000-4000-8000-0000000000f1', kind: 'totp', label: 'Phone', createdAt: '2026-10-01T09:00:00Z' }] }),
    })
    expect(wrapper.get('a[href="/security/two-factor"] .state').text()).toBe('On')
  })

  it('says when two-factor is off', async () => {
    const { wrapper } = await mountAt('/account', answers())
    expect(wrapper.get('a[href="/security/two-factor"] .state').text()).toBe('Off')
  })

  it('counts the apps the User can open', async () => {
    const { wrapper } = await mountAt('/account', {
      ...answers(),
      'GET /api/v1/launcher': () => Response.json({ apps: [{ id: 'grafana', label: 'Grafana', launchUrl: 'https://grafana.test/', roles: ['viewer'] }], heldBack: 0 }),
    })
    expect(wrapper.get('.pills').text()).toContain('1 apps')
  })

  it('shows the problem when the profile cannot load', async () => {
    const { wrapper } = await mountAt('/account', { ...answers(), 'GET /api/v1/profile': problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('leaves the app count out when the apps cannot load', async () => {
    const { wrapper } = await mountAt('/account', { ...answers(), 'GET /api/v1/launcher': problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('.pills').text()).not.toContain('apps')
  })
})

describe('the no-access page', () => {
  it('sends the person back to their apps, or to an Admin', async () => {
    const { wrapper } = await mountAt('/no-access', {})
    expect(wrapper.get('h1').text()).toBe('No access to this app')
    expect(wrapper.get('a.btn').attributes('href')).toBe('/')
    expect(wrapper.get('.auth-foot a').attributes('href')).toBe('mailto:admin@jorisjonkers.dev')
  })
})
