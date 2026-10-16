import { describe, expect, it } from 'vitest'
import { mountAt, problem } from './testing'
import { checkStub } from './contract'

const json = 'application/json'
const problemJson = 'application/problem+json'

describe('checking a stubbed answer against the contract', () => {
  it('accepts an answer the contract describes', () => {
    expect(checkStub('GET', '/api/v1/version', 200, json, { version: '0.4.0' })).toBeNull()
  })

  it('refuses a field the contract does not allow', () => {
    expect(checkStub('GET', '/api/v1/version', 200, json, { version: '0.4.0', build: 7 })).toMatch(/must NOT have additional properties/)
  })

  it('refuses an answer missing a required field', () => {
    expect(checkStub('GET', '/api/v1/version', 200, json, {})).toMatch(/must have required property 'version'/)
  })

  it('matches a path with parameters to its template', () => {
    expect(checkStub('PUT', '/api/v1/access/groups/00000000-0000-4000-8000-0000000000b2/members/00000000-0000-4000-8000-0000000000a2', 204, null, null)).toBeNull()
  })

  it('accepts a problem under a range of statuses', () => {
    expect(checkStub('GET', '/api/v1/access', 403, problemJson, { type: 'about:blank', title: 'Forbidden', status: 403, detail: 'Only Admins manage access.' })).toBeNull()
  })

  it('refuses a problem the contract does not describe', () => {
    expect(checkStub('GET', '/api/v1/access', 403, problemJson, { title: 'Forbidden', reason: 'no' })).not.toBeNull()
  })

  it('falls back to the default answer for a status no range covers', () => {
    expect(checkStub('GET', '/api/v1/access', 500, problemJson, { type: 'about:blank', title: 'Internal Server Error', status: 500 })).toBeNull()
  })

  it('refuses a path the contract does not have', () => {
    expect(checkStub('GET', '/api/v1/nothing', 200, json, {})).toBe('the contract has no GET /api/v1/nothing')
  })

  it('refuses a method the path does not take', () => {
    expect(checkStub('DELETE', '/api/v1/version', 204, null, null)).toBe('the contract has no DELETE /api/v1/version')
  })

  it('checks a status the operation does not name against its default answer', () => {
    expect(checkStub('PUT', '/api/v1/access/groups/a/members/b', 200, problemJson, {})).toMatch(/must have required property/)
  })

  it('refuses a body on an answer the contract says has none', () => {
    expect(checkStub('PUT', '/api/v1/access/groups/a/members/b', 204, json, { ok: true })).toBe('PUT /api/v1/access/groups/a/members/b answers 204 with no body')
  })

  it('refuses an empty answer where the contract has a body', () => {
    expect(checkStub('GET', '/api/v1/version', 200, null, null)).toBe('GET /api/v1/version answers 200 as application/json, not no body')
  })

  it('refuses a media type the answer does not have', () => {
    expect(checkStub('GET', '/api/v1/version', 200, 'text/plain', 'x')).toBe('GET /api/v1/version answers 200 as application/json, not text/plain')
  })
})

describe('a page test with a stub that lies', () => {
  it.fails('fails, even when the page shows nothing wrong', async () => {
    await mountAt('/', {
      'GET /api/v1/launcher': problem(401, 'Sign in first.'),
      'GET /api/v1/version': Response.json({ version: '0.4.0', build: 7 }),
    })
  })

  it('passes when every stub matches the contract', async () => {
    await mountAt('/', {
      'GET /api/v1/launcher': problem(401, 'Sign in first.'),
      'GET /api/v1/version': Response.json({ version: '0.4.0' }),
    })
  })
})
