import { describe, expect, it } from 'vitest'
import { navigation } from './navigation'

describe('navigation', () => {
  it('moves the browser', () => {
    navigation.leave('#elsewhere')
    expect(window.location.hash).toBe('#elsewhere')
  })
})
