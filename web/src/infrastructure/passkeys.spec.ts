import { afterEach, describe, expect, it, vi } from 'vitest'
import { passkeys } from './passkeys'

class FakeCredential {
  toJSON() {
    return { id: 'cred-1', type: 'public-key' }
  }
  static parseCreationOptionsFromJSON = vi.fn((o: unknown) => ({ parsed: o }))
  static parseRequestOptionsFromJSON = vi.fn((o: unknown) => ({ parsed: o }))
}

function withBrowser(create: () => unknown, get: () => unknown) {
  vi.stubGlobal('PublicKeyCredential', FakeCredential)
  const credentials = { create: vi.fn(create), get: vi.fn(get) }
  Object.defineProperty(navigator, 'credentials', { value: credentials, configurable: true })
  return credentials
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('passkeys', () => {
  it('says whether the browser has passkeys', () => {
    vi.stubGlobal('PublicKeyCredential', undefined)
    expect(passkeys.supported()).toBe(false)
    vi.stubGlobal('PublicKeyCredential', {})
    expect(passkeys.supported()).toBe(false)
    vi.stubGlobal('PublicKeyCredential', FakeCredential)
    expect(passkeys.supported()).toBe(true)
  })

  it('makes a passkey from the options and answers with its JSON', async () => {
    const credentials = withBrowser(
      () => Promise.resolve(new FakeCredential()),
      () => Promise.resolve(null),
    )
    await expect(passkeys.create('{"publicKey":{"challenge":"abc"}}')).resolves.toBe('{"id":"cred-1","type":"public-key"}')
    expect(FakeCredential.parseCreationOptionsFromJSON).toHaveBeenCalledWith({ challenge: 'abc' })
    expect(credentials.create).toHaveBeenCalledWith({ publicKey: { parsed: { challenge: 'abc' } } })
  })

  it('signs with a passkey, and refuses when the browser gives nothing back', async () => {
    withBrowser(
      () => Promise.resolve(null),
      () => Promise.resolve(new FakeCredential()),
    )
    await expect(passkeys.get('{"publicKey":{"challenge":"xyz"}}')).resolves.toBe('{"id":"cred-1","type":"public-key"}')
    expect(FakeCredential.parseRequestOptionsFromJSON).toHaveBeenCalledWith({ challenge: 'xyz' })
    await expect(passkeys.create('{"publicKey":{}}')).rejects.toThrow('no passkey')
  })
})
