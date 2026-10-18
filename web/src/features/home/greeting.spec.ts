import { describe, expect, it } from 'vitest'
import { greeting } from './greeting'

describe('the greeting', () => {
  it.each([
    ['in the morning', 8, 'Good morning, Ada'],
    ['in the afternoon', 14, 'Good afternoon, Ada'],
    ['in the evening', 21, 'Good evening, Ada'],
  ])('says good morning, afternoon or evening %s', (_, hour, want) => {
    expect(greeting('Ada Lovelace', new Date(2026, 9, 18, hour))).toBe(want)
  })

  it('greets without a name until the name is known', () => {
    expect(greeting(undefined, new Date(2026, 9, 18, 9))).toBe('Good morning')
  })
})
