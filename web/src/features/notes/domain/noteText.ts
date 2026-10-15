/** The longest note, in characters; the server and the database hold the same rule. */
export const MAX_NOTE_LENGTH = 500

export type NoteTextCheck =
  | { kind: 'ok'; text: string }
  | { kind: 'empty' }
  | { kind: 'tooLong'; length: number }

/** Trims raw and checks its length in characters, not UTF-16 code units, as the server does. */
export function checkNoteText(raw: string): NoteTextCheck {
  const text = raw.trim()
  const length = Array.from(text).length
  if (length === 0) return { kind: 'empty' }
  if (length > MAX_NOTE_LENGTH) return { kind: 'tooLong', length }
  return { kind: 'ok', text }
}

/** The message to show for a check, or null when the text can be sent. */
export function describeNoteTextCheck(check: NoteTextCheck): string | null {
  switch (check.kind) {
    case 'ok':
      return null
    case 'empty':
      return 'Write something first.'
    case 'tooLong':
      return `${String(check.length - MAX_NOTE_LENGTH)} characters too long.`
  }
}
