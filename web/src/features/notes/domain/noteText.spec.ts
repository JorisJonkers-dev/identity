import { describe, expect, it } from 'vitest'
import { checkNoteText, describeNoteTextCheck, MAX_NOTE_LENGTH } from './noteText'

describe('checkNoteText', () => {
  it('trims and accepts text', () => {
    expect(checkNoteText('  Buy milk \n')).toEqual({ kind: 'ok', text: 'Buy milk' })
  })

  it('refuses text that is empty once trimmed', () => {
    expect(checkNoteText(' \t ')).toEqual({ kind: 'empty' })
  })

  it('counts characters, not UTF-16 code units', () => {
    expect(checkNoteText('😀'.repeat(MAX_NOTE_LENGTH)).kind).toBe('ok')
    expect(checkNoteText('😀'.repeat(MAX_NOTE_LENGTH + 1))).toEqual({ kind: 'tooLong', length: MAX_NOTE_LENGTH + 1 })
  })
})

describe('describeNoteTextCheck', () => {
  it('has nothing to say about sendable text', () => {
    expect(describeNoteTextCheck({ kind: 'ok', text: 'x' })).toBeNull()
  })

  it('explains each refusal', () => {
    expect(describeNoteTextCheck({ kind: 'empty' })).toBe('Write something first.')
    expect(describeNoteTextCheck({ kind: 'tooLong', length: MAX_NOTE_LENGTH + 3 })).toBe('3 characters too long.')
  })
})
