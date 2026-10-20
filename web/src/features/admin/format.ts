import type { AuditEvent, AuditPerson } from '../../infrastructure/api/types.gen'

/** One piece of an Audit Event's sentence, and how it is set: a name, a role pill, or plain. */
export interface Part {
  text: string
  as?: 'name' | 'pill'
}

function name(p: AuditPerson | undefined): string {
  return p?.name ?? p?.email ?? 'a deleted User'
}

const why: Record<NonNullable<AuditEvent['reason']>, string> = {
  password: ' (wrong email or password)',
  'second-factor': ' (wrong code or passkey)',
  'no-access': ' (no access)',
  'token-reused': '',
  'by-tool': '',
}

/** The sentence an Audit Event reads as, the way the Audit page shows it. */
export function describeEvent(e: AuditEvent): Part[] {
  const actor: Part = { text: name(e.actor), as: 'name' }
  const user: Part = { text: name(e.user), as: 'name' }
  const to = e.client ? ` to ${e.client.label}` : ''
  const group = e.groupName ?? 'a Group'
  const machine = e.machineName ? `the Machine Identity ${e.machineName}` : 'a Machine Identity'
  // A role given to a person or a Machine Identity directly names it; one given to a Group names the Group.
  const holder = e.machineName ?? (e.groupName === undefined && e.user ? name(e.user) : group)
  const role: Part = { text: e.role ?? '', as: 'pill' }
  const client = e.client?.label ?? 'an app'
  const app = e.appName ?? 'an app'
  switch (e.action) {
    case 'signed-in':
      return [user, { text: ` signed in${to}` }]
    case 'sign-in-failed':
      return [
        { text: 'Failed sign-in for ' },
        { text: e.email ?? (e.user ? name(e.user) : 'an address that is not an email'), as: 'name' },
        { text: `${to}${e.reason ? why[e.reason] : ''}` },
      ]
    case 'signed-out':
      return e.actor ? [actor, { text: ' signed ' }, user, { text: ' out on every device' }] : [user, { text: ' signed out' }]
    case 'user-created':
      return [user, { text: ' signed up and confirmed their email' }]
    case 'user-renamed':
      return e.actor ? [actor, { text: ' renamed ' }, user] : [user, { text: ' changed their name' }]
    case 'password-changed':
      return [user, { text: ' changed their password' }]
    case 'password-reset':
      return [user, { text: ' reset their password through a mailed link' }]
    case 'account-deleted':
      return e.actor ? [actor, { text: ' deleted the account of ' }, user] : [user, { text: ' deleted their own account' }]
    case 'email-changed':
      return [user, { text: ` changed their sign-in email from ${e.email ?? 'an earlier address'}${e.user?.email ? ` to ${e.user.email}` : ''}` }]
    case 'second-factor-added':
      return [user, { text: ' added a Second Factor' }]
    case 'second-factor-removed':
      return [user, { text: ' removed a Second Factor' }]
    case 'admin-promoted':
      return [user, { text: ' was made an Admin' }]
    case 'group-created':
      return [actor, { text: ` made the Group ${group}` }]
    case 'group-renamed':
      return [actor, { text: ` renamed a Group to ${group}` }]
    case 'group-deleted':
      return [actor, { text: ` deleted the Group ${group}` }]
    case 'group-two-factor-required':
      return [actor, { text: ` made ${group} require two-factor` }]
    case 'group-two-factor-dropped':
      return [actor, { text: ` stopped ${group} requiring two-factor` }]
    case 'member-added':
      return [actor, { text: ` added ${name(e.user)} to ${group}` }]
    case 'member-removed':
      return [actor, { text: ` took ${name(e.user)} out of ${group}` }]
    case 'role-assigned':
      return [actor, { text: ` gave ${holder} ` }, role, { text: ` on ${client}` }]
    case 'role-removed':
      return [actor, { text: ' took ' }, role, { text: ` on ${client} away from ${holder}` }]
    case 'token-minted':
      return [user, { text: ' created a token' }]
    case 'token-revoked':
      return [user, { text: ' revoked a token' }]
    case 'client-secret-rotated':
      return [actor, { text: ` gave ${client} a new secret` }]
    case 'machine-created':
      return [actor, { text: ` added ${machine}` }]
    case 'machine-deleted':
      return [actor, { text: ` deleted ${machine}` }]
    case 'machine-secret-rotated':
      return [actor, { text: ` gave ${machine} a new secret` }]
    case 'connected-app-authorized':
      return [user, { text: ` connected ${app} to ${client}` }]
    case 'connected-app-revoked':
      if (e.actor) return [actor, { text: ` disconnected ${app} from ${client} for ` }, user]
      if (e.reason === 'token-reused') return [{ text: `${app} was disconnected from ${client} for ` }, user, { text: ' because its token was used twice, which can mean it was copied' }]
      if (e.reason === 'by-tool') return [{ text: `${app} disconnected itself from ${client}. ` }, user, { text: ' had connected it' }]
      return [user, { text: ` disconnected ${app} from ${client}` }]
    case 'mcp-resource-marked':
      return [actor, { text: ` let apps connect to ${client}` }]
    case 'mcp-resource-unmarked':
      return [actor, { text: ` stopped apps connecting to ${client}` }]
    case 'step-up-changed':
      return [actor, { text: e.minutes ? ` set ${client} to ask for two-factor again after ${String(e.minutes)} minutes` : ` turned off asking for two-factor again in ${client}` }]
  }
}

/** The time of day an event happened, as 21:14:02. */
export function clock(at: string): string {
  return new Date(at).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

/** The day an event happened: Today, Yesterday, or 14 Oct. */
export function dayOf(at: string, now: Date): string {
  const day = new Date(at)
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  if (day.getTime() >= midnight) return 'Today'
  if (day.getTime() >= midnight - 24 * 3600 * 1000) return 'Yesterday'
  return day.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

/** How long ago something happened, as the Unassigned queue says it. */
export function ago(at: string, now: Date): string {
  const minutes = Math.floor((now.getTime() - new Date(at).getTime()) / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${String(minutes)}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${String(hours)}h ago`
  if (hours < 48) return 'yesterday'
  return `${String(Math.floor(hours / 24))} days ago`
}

/** A count and its noun, as 1 sign-in or 2 sign-ins. */
export function counted(n: number, noun: string, plural = `${noun}s`): string {
  return `${String(n)} ${n === 1 ? noun : plural}`
}
