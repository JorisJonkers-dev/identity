import { describe, expect, it } from 'vitest'
import type { AuditEvent, SignInHour } from '../../infrastructure/api/types.gen'
import { ago, clock, counted, dayOf, describeEvent, drawChart, initials, scaleFor, tone } from './format'

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
    ['a Second Factor added', { action: 'second-factor-added', user: noor }, '**Noor Bakker** added a Second Factor'],
    ['a Second Factor removed', { action: 'second-factor-removed', user: noor }, '**Noor Bakker** removed a Second Factor'],
    ['an Admin made', { action: 'admin-promoted', user: noor }, '**Noor Bakker** was made an Admin'],
    ['a Group made', { action: 'group-created', actor: joris, groupName: 'Family' }, '**Joris** made the Group Family'],
    ['a Group renamed', { action: 'group-renamed', actor: joris, groupName: 'Kin' }, '**Joris** renamed a Group to Kin'],
    ['a Group deleted', { action: 'group-deleted', actor: joris, groupName: 'Kin' }, '**Joris** deleted the Group Kin'],
    ['a member added', { action: 'member-added', actor: joris, user: noor, groupName: 'Family' }, '**Joris** added Noor Bakker to Family'],
    ['a member removed', { action: 'member-removed', actor: joris, user: noor, groupName: 'Family' }, '**Joris** took Noor Bakker out of Family'],
    ['a role given', { action: 'role-assigned', actor: joris, groupName: 'Family', role: 'viewer', client: grafana }, '**Joris** gave Family [viewer] on Grafana'],
    ['a role taken away', { action: 'role-removed', actor: joris, groupName: 'Family', role: 'viewer', client: grafana }, '**Joris** took [viewer] on Grafana away from Family'],
    ['a token made', { action: 'token-minted', kind: 'tokens', user: noor }, '**Noor Bakker** created a token'],
    ['a token revoked', { action: 'token-revoked', kind: 'tokens', user: noor }, '**Noor Bakker** revoked a token'],
  ])('%s', (_, e, want) => {
    expect(sentence(e)).toBe(want)
  })

  it('names a User who is gone, and a Group and app it no longer knows', () => {
    expect(sentence({ action: 'role-assigned', actor: { id: joris.id } })).toBe('**a deleted User** gave a Group [] on an app')
  })

  it('names a User by their email when they have no name', () => {
    expect(sentence({ action: 'signed-in', user: { id: noor.id, email: noor.email } })).toBe('**noor@example.org** signed in')
  })
})

describe('an event is coloured by what happened', () => {
  it.each<[Partial<AuditEvent>, string]>([
    [{ action: 'signed-in', kind: 'sign-in' }, 'ok'],
    [{ action: 'sign-in-failed', kind: 'sign-in' }, 'bad'],
    [{ action: 'group-created', kind: 'access' }, 'a'],
    [{ action: 'user-renamed', kind: 'people' }, ''],
  ])('%o is %s', (e, want) => {
    expect(tone(event(e))).toBe(want)
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

describe('the sign-ins chart', () => {
  const start = new Date(2026, 9, 15, 22).getTime()
  const day = (signIns: number[], failed: number[] = []): SignInHour[] =>
    signIns.map((n, i) => ({ at: new Date(start + i * 3600000).toISOString(), signIns: n, failed: failed[i] ?? 0 }))

  it.each([
    [0, 5],
    [5, 5],
    [6, 10],
    [19, 20],
    [21, 50],
    [101, 200],
  ])('scales %d sign-ins in an hour to a top of %d', (most, top) => {
    expect(scaleFor(most)).toBe(top)
  })

  it('draws each hour from left to right, the most at the top of the scale', () => {
    const hours = Array.from({ length: 24 }, (_, i) => (i === 23 ? 20 : 0))
    const c = drawChart(day(hours, [1]))
    expect(c.max).toBe(20)
    expect(c.line.split(' ')[0]).toBe('30.0,110.0')
    expect(c.line.split(' ')[23]).toBe('470.0,20.0')
    expect(c.failed.split(' ')[0]).toBe('30.0,105.5')
    expect(c.end).toEqual({ x: 470, y: 20 })
    expect(c.area).toBe(`M30,110 L${c.line.split(' ').join(' L')} L470,110 Z`)
  })

  it('labels the first, middle and last hour', () => {
    const c = drawChart(day(Array.from({ length: 24 }, () => 0)))
    expect([c.first, c.middle, c.last]).toEqual(['22:00', '10:00', '21:00'])
    expect(c.middleX).toBe(259.6)
  })

  it('sums the day up for a screen reader', () => {
    const hours = Array.from({ length: 24 }, (_, i) => (i === 3 ? 7 : 1))
    expect(drawChart(day(hours, [0, 2])).summary).toBe('30 sign-ins in the last 24 hours. 2 failed sign-ins. The busiest hour had 7 sign-ins, around 01:00.')
  })

  it('says when there were no sign-ins', () => {
    expect(drawChart(day(Array.from({ length: 24 }, () => 0), [1])).summary).toBe('No sign-ins in the last 24 hours. 1 failed sign-in.')
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
})
