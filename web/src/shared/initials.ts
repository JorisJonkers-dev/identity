/** A person's initials, for their avatar. */
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  const first = words[0] ?? '?'
  const last = words.length > 1 ? (words[words.length - 1] as string) : ''
  return (first.charAt(0) + last.charAt(0)).toUpperCase()
}
