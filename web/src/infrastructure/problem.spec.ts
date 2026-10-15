import { describe, expect, it } from 'vitest'
import { describeError } from './problem'

describe('describeError', () => {
  it("prefers a problem's detail, then its title", () => {
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400, detail: 'Fix it.' })).toBe('Fix it.')
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400 })).toBe('Bad Request')
  })

  it('falls back for anything that is not a problem', () => {
    expect(describeError(new TypeError('Failed to fetch'))).toBe('Something went wrong. Try again.')
  })
})
