import type { AccessOverview, Group, Person } from '../../infrastructure/api/types.gen'

/** The Groups a person is in, admins first, as the overview orders them. */
export function groupsOf(o: AccessOverview, userId: string): Group[] {
  return o.groups.filter((g) => g.members.includes(userId))
}

/** The people in a Group, by name. */
export function membersOf(o: AccessOverview, group: Group): Person[] {
  return o.users.filter((u) => group.members.includes(u.id))
}

/** Whether a Group holds a role on an app. */
export function holds(o: AccessOverview, groupId: string, clientId: string, role: string): boolean {
  return o.assignments.some((a) => a.groupId === groupId && a.clientId === clientId && a.role === role)
}

/** The Groups holding a role on an app. */
export function holders(o: AccessOverview, clientId: string, role: string): Group[] {
  return o.groups.filter((g) => holds(o, g.id, clientId, role))
}

/** The apps a Group holds any role on. */
export function appsOf(o: AccessOverview, groupId: string): string[] {
  return [...new Set(o.assignments.filter((a) => a.groupId === groupId).map((a) => a.clientId))]
}

/** What a person holds on each app through their Groups: role to the Groups that give it. */
export function accessOf(o: AccessOverview, userId: string): Map<string, Map<string, string[]>> {
  const out = new Map<string, Map<string, string[]>>()
  for (const g of groupsOf(o, userId)) {
    for (const a of o.assignments.filter((x) => x.groupId === g.id)) {
      const roles = out.get(a.clientId) ?? new Map<string, string[]>()
      roles.set(a.role, [...(roles.get(a.role) ?? []), g.name])
      out.set(a.clientId, roles)
    }
  }
  return out
}

/** Whether a person holds a role on an app directly, without a Group. */
export function holdsDirect(o: AccessOverview, userId: string, clientId: string, role: string): boolean {
  return o.userAssignments.some((a) => a.userId === userId && a.clientId === clientId && a.role === role)
}

/** Whether a person waits for access: no role directly or through a Group, and no Admin. */
export function unassigned(o: AccessOverview, userId: string): boolean {
  const groups = groupsOf(o, userId)
  return !groups.some((g) => g.admins) && !o.userAssignments.some((a) => a.userId === userId) && accessOf(o, userId).size === 0
}

/** Every role any app declares, sorted. */
export function allRoles(o: AccessOverview): string[] {
  return [...new Set(o.clients.flatMap((c) => c.roles))].sort()
}

/** The Group a page shows: the one asked for, else the first that gives app roles, else the admins, which identity always has. */
export function groupToShow(o: AccessOverview, id: unknown): Group {
  return o.groups.find((g) => g.id === id) ?? o.groups.find((g) => !g.admins) ?? (o.groups[0] as Group)
}
