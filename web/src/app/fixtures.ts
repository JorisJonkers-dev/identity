import type { AccessOverview, Dashboard, PersonalToken, Profile } from '../infrastructure/api/types.gen'

/** What the server answers, shaped as the contract says, for the specs to start from. */

export const ids = {
  joris: '00000000-0000-4000-8000-0000000000a1',
  ada: '00000000-0000-4000-8000-0000000000a2',
  noor: '00000000-0000-4000-8000-0000000000a3',
  admins: '00000000-0000-4000-8000-0000000000b1',
  family: '00000000-0000-4000-8000-0000000000b2',
  friends: '00000000-0000-4000-8000-0000000000b3',
}

const day = 24 * 3600 * 1000

export function profile(over: Partial<Profile> = {}): Profile {
  return { email: 'ada@example.org', name: 'Ada Lovelace', createdAt: '2026-10-01T09:00:00Z', admin: false, secondFactor: false, ...over }
}

export const admin = () => profile({ email: 'joris@example.org', name: 'Joris Jonkers', admin: true, secondFactor: true })

export function dashboard(over: Partial<Dashboard> = {}): Dashboard {
  const start = Date.UTC(2026, 9, 15, 22)
  return {
    users: 14,
    usersThisWeek: 1,
    unassigned: [{ id: ids.noor, name: 'Noor Bakker', email: 'noor@example.org', confirmedAt: new Date(Date.now() - 2.5 * 3600000).toISOString() }],
    signIns: 186,
    failedSignIns: 4,
    hours: Array.from({ length: 24 }, (_, i) => ({ at: new Date(start + i * 3600000).toISOString(), signIns: i, failed: i === 20 ? 3 : 0 })) as Dashboard['hours'],
    activeSessions: 23,
    sessionUsers: 12,
    tokensExpiring: 3,
    ...over,
  }
}

export function token(over: Partial<PersonalToken> = {}): PersonalToken {
  return {
    id: '00000000-0000-4000-8000-0000000000c1',
    name: 'grafana-cli',
    client: { id: 'grafana', label: 'Grafana' },
    roles: ['viewer'],
    createdAt: '2026-10-01T09:00:00Z',
    expiresAt: new Date(Date.now() + 5.5 * day).toISOString(),
    state: 'expiring',
    ...over,
  }
}

/** Joris administers; Ada is in Family, which gives Grafana viewer and Jellyfin user; Noor waits for a Group. */
export function overview(over: Partial<AccessOverview> = {}): AccessOverview {
  return {
    groups: [
      { id: ids.admins, name: 'Admins', admins: true, members: [ids.joris] },
      { id: ids.family, name: 'Family', admins: false, members: [ids.joris, ids.ada] },
      { id: ids.friends, name: 'Friends', admins: false, members: [] },
    ],
    users: [
      { id: ids.joris, email: 'joris@example.org', name: 'Joris Jonkers' },
      { id: ids.ada, email: 'ada@example.org', name: 'Ada Lovelace' },
      { id: ids.noor, email: 'noor@example.org', name: 'Noor Bakker' },
    ],
    clients: [
      { id: 'grafana', label: 'Grafana', roles: ['editor', 'viewer'] },
      { id: 'jellyfin', label: 'Jellyfin', roles: ['user'] },
      { id: 'vault', label: 'Vault', roles: ['user'] },
    ],
    assignments: [
      { groupId: ids.family, clientId: 'grafana', role: 'viewer' },
      { groupId: ids.family, clientId: 'jellyfin', role: 'user' },
    ],
    ...over,
  }
}
