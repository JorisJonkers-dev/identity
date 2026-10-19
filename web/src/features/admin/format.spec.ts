import { describe, expect, it } from 'vitest'
import type { AuditEvent } from '../../infrastructure/api/types.gen'
import { initials } from '../../shared/initials'
import { ago, clock, counted, dayOf, describeEvent } from './format'

const joris = { id: '00000000-0000-4000-8000-0000000000a1', name: 'Joris', email: 'joris@example.org' }
const noor = { id: '00000000-0000-4000-8000-0000000000a2', name: 'Noor Bakker', email: 'noor@example.org' }
const grafana = { id: 'grafana', label: 'Grafana' }

function event(e: Partial<AuditEvent>): AuditEvent {
  return { id: '00000000-0000-4000-8000-0000000000e1', at: '2026-10-16T21:14:02Z', kind: 'access', action: 'group-created', ...e }
}

const sentence = (e: Partial<AuditEvent>) =>
  describeEvent(event(e))
    .map((p) => (p.as === 'name' ? `**${p.text}**` : p.as === 'pill' ? `[${p.text}]` : p.text))
    .join('')

describe('an Audit Event reads as a sentence', () => {
  it.each<[string, Partial<AuditEvent>, string]>([
    ['a sign-in', { action: 'signed-in', user: noor }, '**Noor Bakker** signed in'],
    ['a sign-in to an app', { action: 'signed-in', user: noor, client: grafana }, '**Noor Bakker** signed in to Grafana'],
    ['a wrong password', { action: 'sign-in-failed', user: noor, email: 'noor@example.org', reason: 'password' }, 'Failed sign-in for **noor@example.org** (wrong email or password)'],
    ['a wrong code', { action: 'sign-in-failed', user: noor, reason: 'second-factor' }, 'Failed sign-in for **Noor Bakker** (wrong code or passkey)'],
    ['no access to an app', { action: 'sign-in-failed', user: noor, client: grafana, reason: 'no-access' }, 'Failed sign-in for **Noor Bakker** to Grafana (no access)'],
    ['a sign-in with no address', { action: 'sign-in-failed' }, 'Failed sign-in for **an address that is not an email**'],
    ['signing oneself out', { action: 'signed-out', user: noor }, '**Noor Bakker** signed out'],
    ['an Admin signing someone out', { action: 'signed-out', actor: joris, user: noor }, '**Joris** signed **Noor Bakker** out on every device'],
    ['a new User', { action: 'user-created', user: noor }, '**Noor Bakker** signed up and confirmed their email'],
    ['a new name', { action: 'user-renamed', user: noor }, '**Noor Bakker** changed their name'],
    ['a new password', { action: 'password-changed', user: noor }, '**Noor Bakker** changed their password'],
    ['a new email', { action: 'email-changed', user: noor, email: 'old@example.org' }, '**Noor Bakker** changed their sign-in email from old@example.org to noor@example.org'],
    ['a new email of someone since deleted', { action: 'email-changed', user: { id: noor.id }, email: 'old@example.org' }, '**a deleted User** changed their sign-in email from old@example.org'],
    ['a password reset', { action: 'password-reset', user: noor }, '**Noor Bakker** reset their password through a mailed link'],
    ['a person renamed by an Admin', { action: 'user-renamed', actor: joris, user: noor }, '**Joris** renamed **Noor Bakker**'],
    ['an account deleted by an Admin', { action: 'account-deleted', actor: joris, user: { id: noor.id } }, '**Joris** deleted the account of **a deleted User**'],
    ['an account deleted', { action: 'account-deleted', user: { id: noor.id } }, '**a deleted User** deleted their own account'],
    ['a new email from an address no longer known', { action: 'email-changed', user: noor }, '**Noor Bakker** changed their sign-in email from an earlier address to noor@example.org'],
    ['a Second Factor added', { action: 'second-factor-added', user: noor }, '**Noor Bakker** added a Second Factor'],
    ['a Second Factor removed', { action: 'second-factor-removed', user: noor }, '**Noor Bakker** removed a Second Factor'],
    ['an Admin made', { action: 'admin-promoted', user: noor }, '**Noor Bakker** was made an Admin'],
    ['a Group made', { action: 'group-created', actor: joris, groupName: 'Family' }, '**Joris** made the Group Family'],
    ['a Group renamed', { action: 'group-renamed', actor: joris, groupName: 'Kin' }, '**Joris** renamed a Group to Kin'],
    ['a Group deleted', { action: 'group-deleted', actor: joris, groupName: 'Kin' }, '**Joris** deleted the Group Kin'],
    ['two-factor required', { action: 'group-two-factor-required', actor: joris, groupName: 'Family' }, '**Joris** made Family require two-factor'],
    ['two-factor no longer required', { action: 'group-two-factor-dropped', actor: joris, groupName: 'Family' }, '**Joris** stopped Family requiring two-factor'],
    ['a member added', { action: 'member-added', actor: joris, user: noor, groupName: 'Family' }, '**Joris** added Noor Bakker to Family'],
    ['a member removed', { action: 'member-removed', actor: joris, user: noor, groupName: 'Family' }, '**Joris** took Noor Bakker out of Family'],
    ['a role given', { action: 'role-assigned', actor: joris, groupName: 'Family', role: 'viewer', client: grafana }, '**Joris** gave Family [viewer] on Grafana'],
    ['a role taken away', { action: 'role-removed', actor: joris, groupName: 'Family', role: 'viewer', client: grafana }, '**Joris** took [viewer] on Grafana away from Family'],
    ['a role given to a person', { action: 'role-assigned', actor: joris, user: noor, role: 'viewer', client: grafana }, '**Joris** gave Noor Bakker [viewer] on Grafana'],
    ['a person’s role taken away', { action: 'role-removed', actor: joris, user: noor, role: 'viewer', client: grafana }, '**Joris** took [viewer] on Grafana away from Noor Bakker'],
    ['a token made', { action: 'token-minted', kind: 'tokens', user: noor }, '**Noor Bakker** created a token'],
    ['a token revoked', { action: 'token-revoked', kind: 'tokens', user: noor }, '**Noor Bakker** revoked a token'],
    ['an app given a new secret', { action: 'client-secret-rotated', actor: joris, client: grafana }, '**Joris** gave Grafana a new secret'],
    ['step-up set', { action: 'step-up-changed', actor: joris, client: grafana, minutes: 15 }, '**Joris** set Grafana to ask for two-factor again after 15 minutes'],
    ['a Machine Identity added', { action: 'machine-created', actor: joris, machineName: 'Backup' }, '**Joris** added the Machine Identity Backup'],
    ['a Machine Identity deleted', { action: 'machine-deleted', actor: joris, machineName: 'Backup' }, '**Joris** deleted the Machine Identity Backup'],
    ['a Machine Identity given a secret', { action: 'machine-secret-rotated', actor: joris, machineName: 'Backup' }, '**Joris** gave the Machine Identity Backup a new secret'],
    ['a role given to a Machine Identity', { action: 'role-assigned', actor: joris, machineName: 'Backup', role: 'viewer', client: grafana }, '**Joris** gave Backup [viewer] on Grafana'],
    ['step-up turned off', { action: 'step-up-changed', actor: joris, client: grafana, minutes: 0 }, '**Joris** turned off asking for two-factor again in Grafana'],
    ['an app connected', { action: 'connected-app-authorized', kind: 'tokens', user: noor, client: grafana, appName: 'Claude Code' }, '**Noor Bakker** connected Claude Code to Grafana'],
    ['an app disconnected by its User', { action: 'connected-app-revoked', kind: 'tokens', user: noor, client: grafana, appName: 'Claude Code' }, '**Noor Bakker** disconnected Claude Code from Grafana'],
    ['an app disconnected by an Admin', { action: 'connected-app-revoked', kind: 'tokens', actor: joris, user: noor, client: grafana, appName: 'Claude Code' }, '**Joris** disconnected Claude Code from Grafana for **Noor Bakker**'],
    ['an app disconnected for a reused token', { action: 'connected-app-revoked', kind: 'tokens', user: noor, client: grafana, appName: 'Claude Code', reason: 'token-reused' }, 'Claude Code was disconnected from Grafana for **Noor Bakker** because its token was used twice, which can mean it was copied'],
    ['an unnamed app connected', { action: 'connected-app-authorized', kind: 'tokens', user: noor, client: grafana }, '**Noor Bakker** connected an app to Grafana'],
    ['apps let connect', { action: 'mcp-resource-marked', actor: joris, client: grafana }, '**Joris** let apps connect to Grafana'],
    ['apps stopped connecting', { action: 'mcp-resource-unmarked', actor: joris, client: grafana }, '**Joris** stopped apps connecting to Grafana'],
  ])('%s', (_, e, want) => {
    expect(sentence(e)).toBe(want)
  })

  it('names a User who is gone, and a Group and app it no longer knows', () => {
    expect(sentence({ action: 'role-assigned', actor: { id: joris.id } })).toBe('**a deleted User** gave a Group [] on an app')
  })

  it('names a Machine Identity it no longer knows', () => {
    expect(sentence({ action: 'machine-deleted', actor: joris })).toBe('**Joris** deleted a Machine Identity')
  })

  it('names a User by their email when they have no name', () => {
    expect(sentence({ action: 'signed-in', user: { id: noor.id, email: noor.email } })).toBe('**noor@example.org** signed in')
  })
})

describe('times', () => {
  const now = new Date(2026, 9, 16, 21, 30)

  it('shows the time of day to the second', () => {
    expect(clock(new Date(2026, 9, 16, 21, 14, 2).toISOString())).toBe('21:14:02')
  })

  it.each<[Date, string]>([
    [new Date(2026, 9, 16, 0, 1), 'Today'],
    [new Date(2026, 9, 15, 23, 59), 'Yesterday'],
    [new Date(2026, 9, 15, 0, 0), 'Yesterday'],
    [new Date(2026, 9, 14, 23, 59), '14 Oct'],
  ])('names the day of %s as %s', (at, want) => {
    expect(dayOf(at.toISOString(), now)).toBe(want)
  })

  it.each<[number, string]>([
    [0, 'just now'],
    [5, '5m ago'],
    [120, '2h ago'],
    [30 * 60, 'yesterday'],
    [72 * 60, '3 days ago'],
  ])('says %d minutes ago as %s', (minutes, want) => {
    expect(ago(new Date(now.getTime() - minutes * 60000).toISOString(), now)).toBe(want)
  })
})

describe('initials', () => {
  it.each([
    ['Noor Bakker', 'NB'],
    ['Sam de Vries', 'SV'],
    ['joris', 'J'],
    ['  ', '?'],
  ])('of %j are %s', (name, want) => {
    expect(initials(name)).toBe(want)
  })
})

describe('counts', () => {
  it.each([
    [0, '0 sign-ins'],
    [1, '1 sign-in'],
    [2, '2 sign-ins'],
  ])('say %d as %s', (n, want) => {
    expect(counted(n, 'sign-in')).toBe(want)
  })

  it('takes an irregular plural', () => {
    expect([counted(1, 'person', 'people'), counted(12, 'person', 'people')]).toEqual(['1 person', '12 people'])
  })
})
