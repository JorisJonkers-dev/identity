import type { VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { profile } from '../../app/fixtures'
import { mountAt, problem, settle } from '../../app/testing'
import type { SecondFactor } from '../../infrastructure/api/types.gen'
import { passkeys } from '../../infrastructure/passkeys'

const phone: SecondFactor = { id: '00000000-0000-4000-8000-0000000000f1', kind: 'totp', label: 'Phone', createdAt: '2026-10-16T12:00:00Z', lastUsedAt: '2026-10-18T18:00:00Z' }
const laptop: SecondFactor = { id: '00000000-0000-4000-8000-0000000000f2', kind: 'passkey', label: 'MacBook Pro', createdAt: '2026-10-17T12:00:00Z' }
const setup = {
  secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
  uri: 'otpauth://totp/jorisjonkers.dev:ada@example.org?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=jorisjonkers.dev',
}
const ok = () => new Response(null, { status: 204 })

const page = (factors: SecondFactor[] = []) => ({
  'GET /api/v1/profile': () => Response.json(profile()),
  'GET /api/v1/profile/second-factors': () => Response.json({ factors }),
})

function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found) throw new Error(`no button ${text}`)
  return found
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('the Security page', () => {
  it('says two-factor is off while the User has no way to give it', async () => {
    const { wrapper } = await mountAt('/security', page())
    expect(wrapper.get('.facts3 b').text()).toBe('Off')
    expect(wrapper.get('a[href="/security/two-factor"] small').text()).toBe('Off')
    expect(wrapper.findAll('.facts3 b')[1]?.text()).toBe('Nothing yet')
  })

  it('names the ways the User gives two-factor, and the last one added', async () => {
    const { wrapper } = await mountAt('/security', page([phone, laptop]))
    expect(wrapper.get('.facts3 b').text()).toBe('On')
    expect(wrapper.get('.facts3 small').text()).toBe('2 methods')
    expect(wrapper.get('a[href="/security/two-factor"] small').text()).toBe('Passkey and authenticator app')
    expect(wrapper.findAll('.facts3 b')[1]?.text()).toBe('MacBook Pro')
    expect(wrapper.findAll('.facts3 small')[1]?.text()).toBe('17 Oct')
  })

  it('says one way for one', async () => {
    const { wrapper } = await mountAt('/security', page([phone]))
    expect(wrapper.get('.facts3 small').text()).toBe('1 method')
    expect(wrapper.get('a[href="/security/two-factor"] small').text()).toBe('Authenticator app')
  })

  it('links to the password page', async () => {
    const { wrapper } = await mountAt('/security', page())
    expect(wrapper.find('a[href="/security/password"]').exists()).toBe(true)
  })

  it('shows the problem when the list cannot load', async () => {
    const { wrapper } = await mountAt('/security', { ...page(), 'GET /api/v1/profile/second-factors': problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})

describe('the Two-factor page', () => {
  it('lists each way with its name and when it was last used or added', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page([phone, laptop]))
    expect(wrapper.findAll('.group .row').map((r) => r.get('.grow').text())).toEqual([
      'Authenticator appPhone · used 18 Oct 2026',
      'PasskeyMacBook Pro · added 17 Oct 2026',
    ])
  })

  it('says when two-factor is off, and offers both ways', async () => {
    vi.spyOn(passkeys, 'supported').mockReturnValue(true)
    const { wrapper } = await mountAt('/security/two-factor', page())
    expect(wrapper.get('.off-box b').text()).toBe('Two-factor is off')
    expect(wrapper.get('a[href="/security/two-factor/setup"]').text()).toBe('Set up an authenticator app')
    expect(button(wrapper, 'Add a passkey').exists()).toBe(true)
  })

  it('offers to replace an authenticator app instead of adding a second', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page([phone]))
    expect(wrapper.findAll('a[href="/security/two-factor/setup"]').map((a) => a.text())).toEqual(['Replace'])
  })

  it('offers no passkey where the browser cannot make one', async () => {
    vi.spyOn(passkeys, 'supported').mockReturnValue(false)
    const { wrapper } = await mountAt('/security/two-factor', page())
    expect(wrapper.findAll('button').some((b) => b.text() === 'Add a passkey')).toBe(false)
  })

  it('tells an Admin sent here why', async () => {
    const { wrapper } = await mountAt('/security/two-factor?for=admin', page())
    expect(wrapper.get('.note.warn').text()).toBe('Admin pages need two-factor. Add a passkey or an authenticator app, then sign in again.')
  })

  it('removes a way after asking for the password', async () => {
    let factors = [phone]
    const { wrapper, requests } = await mountAt('/security/two-factor', {
      ...page(),
      'GET /api/v1/profile/second-factors': () => Response.json({ factors }),
      [`POST /api/v1/profile/second-factors/${phone.id}/removal`]: () => {
        factors = []
        return ok()
      },
    })
    await button(wrapper, 'Remove').trigger('click')
    expect(wrapper.get('#rm-h').text()).toBe('Remove the authenticator app?')
    await wrapper.get('#rm-pw').setValue('correct horse battery')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key.endsWith('/removal'))?.body).toEqual({ password: 'correct horse battery' })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.get('.off-box b').text()).toBe('Two-factor is off')
  })

  it('names a passkey as the thing to remove', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page([laptop]))
    await button(wrapper, 'Remove').trigger('click')
    expect(wrapper.get('#rm-h').text()).toBe('Remove this passkey?')
  })

  it('keeps a way when the removal is cancelled', async () => {
    const { wrapper, requests } = await mountAt('/security/two-factor', page([phone]))
    await button(wrapper, 'Remove').trigger('click')
    await wrapper.get('#rm-pw').setValue('typed then cancelled')
    await button(wrapper, 'Cancel').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(requests.some((r) => r.key.endsWith('/removal'))).toBe(false)
    await button(wrapper, 'Remove').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#rm-pw').element.value).toBe('')
  })

  it('closes the dialog on Escape', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page([phone]))
    await button(wrapper, 'Remove').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await settle()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('closes the dialog on a click beside it, not on it', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page([phone]))
    await button(wrapper, 'Remove').trigger('click')
    await wrapper.get('form[role="dialog"]').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    await wrapper.get('.scrim').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('shows a refused removal', async () => {
    const { wrapper } = await mountAt('/security/two-factor', {
      ...page([phone]),
      [`POST /api/v1/profile/second-factors/${phone.id}/removal`]: problem(422, 'Your password is wrong. Enter it again.'),
    })
    await button(wrapper, 'Remove').trigger('click')
    await wrapper.get('#rm-pw').setValue('wrong password!!')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('Your password is wrong. Enter it again.')
  })

  it('adds a passkey after asking for a name and the password', async () => {
    vi.spyOn(passkeys, 'supported').mockReturnValue(true)
    const create = vi.spyOn(passkeys, 'create').mockResolvedValue('{"id":"cred-1"}')
    let factors: SecondFactor[] = []
    const { wrapper, requests } = await mountAt('/security/two-factor', {
      ...page(),
      'GET /api/v1/profile/second-factors': () => Response.json({ factors }),
      'POST /api/v1/profile/second-factors/passkey': Response.json({ options: '{"publicKey":{}}' }),
      'POST /api/v1/profile/second-factors/passkey/confirmation': () => {
        factors = [laptop]
        return ok()
      },
    })
    await button(wrapper, 'Add a passkey').trigger('click')
    await wrapper.get('#pk-l').setValue('MacBook Pro')
    await wrapper.get('#pk-pw').setValue('correct horse battery')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(create).toHaveBeenCalledWith('{"publicKey":{}}')
    expect(requests.find((r) => r.key.endsWith('/passkey/confirmation'))?.body).toEqual({ label: 'MacBook Pro', response: '{"id":"cred-1"}', password: 'correct horse battery' })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.get('.group .row b').text()).toBe('Passkey')
  })

  it('says when the browser made no passkey', async () => {
    vi.spyOn(passkeys, 'supported').mockReturnValue(true)
    vi.spyOn(passkeys, 'create').mockRejectedValue(new Error('NotAllowedError'))
    const { wrapper } = await mountAt('/security/two-factor', { ...page(), 'POST /api/v1/profile/second-factors/passkey': Response.json({ options: '{"publicKey":{}}' }) })
    await button(wrapper, 'Add a passkey').trigger('click')
    await wrapper.get('#pk-l').setValue('MacBook Pro')
    await wrapper.get('#pk-pw').setValue('correct horse battery')
    await wrapper.get('form[role="dialog"]').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('No passkey was made. Try again, then follow the prompt your browser shows.')
  })

  it('goes back to the Security page', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page())
    expect(wrapper.get('a.crumb').attributes('href')).toBe('/security')
  })

  it('keeps the dialog open on any other key', async () => {
    const { wrapper } = await mountAt('/security/two-factor', page([phone]))
    await button(wrapper, 'Remove').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await settle()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
  })

  it('shows the problem when the list cannot load', async () => {
    const { wrapper } = await mountAt('/security/two-factor', { ...page(), 'GET /api/v1/profile/second-factors': problem(500, 'Something went wrong. Try again.') })
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })
})

describe('setting up an authenticator app', () => {
  const answers = (over: Record<string, unknown> = {}) => ({
    ...page(),
    'POST /api/v1/profile/second-factors/totp': Response.json(setup),
    'POST /api/v1/profile/second-factors/totp/confirmation': ok(),
    ...over,
  })

  async function toPassword(wrapper: VueWrapper) {
    await button(wrapper, 'I have scanned it').trigger('click')
    await wrapper.get('[aria-label="Digit 1 of 6"]').setValue('492039')
    await wrapper.get('#tf-l').setValue('Work phone')
    await wrapper.get('form').trigger('submit')
  }

  it('shows the QR code and the key to type instead', async () => {
    const { wrapper } = await mountAt('/security/two-factor/setup', answers())
    expect(wrapper.get('svg.qr').attributes('aria-label')).toBe('QR code for your authenticator app')
    expect(wrapper.get('.secret code').text()).toBe(setup.secret)
    expect(wrapper.get('.steps [aria-current="step"]').text()).toBe('1Scan')
  })

  it('copies the key', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    const { wrapper } = await mountAt('/security/two-factor/setup', answers())
    await button(wrapper, 'Copy').trigger('click')
    await settle()
    expect(writeText).toHaveBeenCalledWith(setup.secret)
    expect(wrapper.get('.secret button').text()).toBe('Copied')
  })

  it('turns two-factor on with the first code, a name and the password', async () => {
    const { wrapper, requests } = await mountAt('/security/two-factor/setup', answers())
    await toPassword(wrapper)
    expect(wrapper.get('.steps [aria-current="step"]').text()).toBe('3Password')
    await wrapper.get('#tf-pw').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(requests.find((r) => r.key.endsWith('/totp/confirmation'))?.body).toEqual({ label: 'Work phone', code: '492039', password: 'correct horse battery' })
    expect(wrapper.get('.off-box b').text()).toBe('Two-factor is on')
  })

  it('puts the cursor in the first code box', async () => {
    const { wrapper } = await mountAt('/security/two-factor/setup', answers(), { attach: true })
    await button(wrapper, 'I have scanned it').trigger('click')
    await settle()
    expect(document.activeElement).toBe(wrapper.get('[aria-label="Digit 1 of 6"]').element)
  })

  it('waits for six digits', async () => {
    const { wrapper } = await mountAt('/security/two-factor/setup', answers())
    await button(wrapper, 'I have scanned it').trigger('click')
    await wrapper.get('[aria-label="Digit 1 of 6"]').setValue('49')
    expect(button(wrapper, 'Continue').attributes('disabled')).toBeDefined()
  })

  it('goes back a step', async () => {
    const { wrapper } = await mountAt('/security/two-factor/setup', answers())
    await toPassword(wrapper)
    await button(wrapper, 'Back').trigger('click')
    expect(wrapper.find('#tf-l').exists()).toBe(true)
    await button(wrapper, 'Back').trigger('click')
    expect(wrapper.find('svg.qr').exists()).toBe(true)
  })

  it('shows a refused confirmation', async () => {
    const { wrapper } = await mountAt('/security/two-factor/setup', answers({ 'POST /api/v1/profile/second-factors/totp/confirmation': problem(422, 'That code is wrong. Enter the code your app shows now.') }))
    await toPassword(wrapper)
    await wrapper.get('#tf-pw').setValue('correct horse battery')
    await wrapper.get('form').trigger('submit')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toBe('That code is wrong. Enter the code your app shows now.')
  })

  it('shows a refused start', async () => {
    const { wrapper } = await mountAt('/security/two-factor/setup', answers({ 'POST /api/v1/profile/second-factors/totp': problem(409, 'You already have an authenticator app.') }))
    expect(wrapper.get('[role="alert"]').text()).toBe('You already have an authenticator app.')
    expect(button(wrapper, 'I have scanned it').attributes('disabled')).toBeDefined()
  })
})

describe('the Password page', () => {
  const answers = (over: Record<string, unknown> = {}) => ({ ...page(), 'POST /api/v1/profile/password': ok(), ...over })

  async function change(wrapper: VueWrapper, next = 'a new password!') {
    await wrapper.get('#pw-cur').setValue('old password!!')
    await wrapper.get('#pw-new').setValue(next)
    await wrapper.get('form').trigger('submit')
    await settle()
  }

  it('changes the password, clears the fields and says other devices are signed out', async () => {
    const { wrapper, requests } = await mountAt('/security/password', answers())
    await change(wrapper)
    expect(requests.find((r) => r.key === 'POST /api/v1/profile/password')?.body).toEqual({ current: 'old password!!', next: 'a new password!' })
    expect(wrapper.get<HTMLInputElement>('#pw-cur').element.value).toBe('')
    expect(wrapper.get<HTMLInputElement>('#pw-new').element.value).toBe('')
    expect(wrapper.get('[role="status"]').text()).toBe('Password changed. Other devices are signed out.')
  })

  it('asks for a longer password without asking the server', async () => {
    const { wrapper, requests } = await mountAt('/security/password', answers())
    await change(wrapper, 'short')
    expect(requests.some((r) => r.key === 'POST /api/v1/profile/password')).toBe(false)
    expect(wrapper.get('.err').text()).toBe('Use at least 12 characters.')
  })

  it('shows a refused change', async () => {
    const { wrapper } = await mountAt('/security/password', answers({ 'POST /api/v1/profile/password': problem(422, 'Your current password is wrong. Enter it again.') }))
    await change(wrapper)
    expect(wrapper.get('[role="alert"]').text()).toBe('Your current password is wrong. Enter it again.')
  })

  it('sends someone who forgot it to an Admin', async () => {
    const { wrapper } = await mountAt('/security/password', answers())
    expect(wrapper.get('.task-aside a').attributes('href')).toBe('mailto:admin@jorisjonkers.dev')
  })
})
