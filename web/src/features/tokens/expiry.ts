import type { PersonalToken } from '../../infrastructure/api/types.gen'

const day = 24 * 3600 * 1000

function date(at: string): string {
  return new Date(at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

/** When a token ends, as its row says it: in days while that is soon, else the date. */
export function expiry(t: PersonalToken, now: Date): string {
  if (t.state === 'revoked') return 'revoked'
  if (t.state === 'expired') return `expired ${date(t.expiresAt)}`
  const days = Math.ceil((new Date(t.expiresAt).getTime() - now.getTime()) / day)
  if (t.state === 'expiring') return days <= 1 ? 'expires within a day' : `expires in ${String(days)} days`
  return `expires ${date(t.expiresAt)}`
}

/** The tokens that end within seven days, which the Launcher warns about. */
export function expiring(tokens: PersonalToken[]): PersonalToken[] {
  return tokens.filter((t) => t.state === 'expiring')
}
