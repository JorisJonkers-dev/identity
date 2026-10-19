import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  addMemberMutation,
  assignRoleMutation,
  askUserEmailChangeMutation,
  assignUserRoleMutation,
  createGroupMutation,
  deleteGroupMutation,
  deleteUserMutation,
  getAccessOptions,
  getAccessQueryKey,
  removeMemberMutation,
  renameGroupMutation,
  renameUserMutation,
  setGroupTwoFactorMutation,
  signOutUserMutation,
  unassignRoleMutation,
  unassignUserRoleMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { AccessOverview } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** Groups, their members and their App Roles, and every change an Admin can make to them. */
export function useAccess() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...getAccessOptions(), retry: false })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: getAccessQueryKey() })

  const create = useAction(createGroupMutation())
  const rename = useAction(renameGroupMutation())
  const remove = useAction(deleteGroupMutation())
  const join = useAction(addMemberMutation())
  const leave = useAction(removeMemberMutation())
  const assign = useAction(assignRoleMutation())
  const unassign = useAction(unassignRoleMutation())
  const signOut = useAction(signOutUserMutation())
  const give = useAction(assignUserRoleMutation())
  const take = useAction(unassignUserRoleMutation())
  const twoFactor = useAction(setGroupTwoFactorMutation())
  const renamePerson = useAction(renameUserMutation())
  const emailPerson = useAction(askUserEmailChangeMutation())
  const deletePerson = useAction(deleteUserMutation())
  const actions = [create, rename, remove, join, leave, assign, unassign, signOut, give, take, twoFactor, renamePerson, emailPerson, deletePerson]

  const overview = computed<AccessOverview | undefined>(() => query.data.value)
  return {
    overview,
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    actionError: computed(() => actions.map((a) => a.error.value).find((e) => e !== null) ?? null),
    createGroup: (name: string, done: () => void) => {
      create.run({ body: { name } }, () => {
        done()
        refresh()
      })
    },
    renameGroup: (groupId: string, name: string) => {
      rename.run({ path: { groupId }, body: { name } }, refresh)
    },
    // done runs before the caller leaves the page: a callback for an unmounted page never fires.
    deleteGroup: (groupId: string, done: () => void) => {
      remove.run({ path: { groupId } }, () => {
        refresh()
        done()
      })
    },
    addMember: (groupId: string, userId: string) => {
      join.run({ path: { groupId, userId } }, refresh)
    },
    removeMember: (groupId: string, userId: string) => {
      leave.run({ path: { groupId, userId } }, refresh)
    },
    signOutUser: (userId: string, done: () => void) => {
      signOut.run({ path: { userId } }, done)
    },
    setRole: (groupId: string, clientId: string, role: string, held: boolean) => {
      const path = { groupId, clientId, role }
      if (held) assign.run({ path }, refresh)
      else unassign.run({ path }, refresh)
    },
    renameUser: (userId: string, name: string) => {
      renamePerson.run({ path: { userId }, body: { name } }, refresh)
    },
    askUserEmail: (userId: string, email: string, done: () => void) => {
      emailPerson.run({ path: { userId }, body: { email } }, done)
    },
    // done runs before the caller leaves the page: a callback for an unmounted page never fires.
    deleteUser: (userId: string, done: () => void) => {
      deletePerson.run({ path: { userId } }, () => {
        refresh()
        done()
      })
    },
    setTwoFactor: (groupId: string, required: boolean) => {
      twoFactor.run({ path: { groupId }, body: { required } }, refresh)
    },
    setUserRole: (userId: string, clientId: string, role: string, held: boolean) => {
      const path = { userId, clientId, role }
      if (held) give.run({ path }, refresh)
      else take.run({ path }, refresh)
    },
  }
}
