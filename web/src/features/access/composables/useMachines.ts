import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  assignMachineRoleMutation,
  createMachineMutation,
  deleteMachineMutation,
  listMachinesOptions,
  listMachinesQueryKey,
  rotateMachineSecretMutation,
  unassignMachineRoleMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Machine, MachinesOverview } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** Machine Identities, their App Roles and secrets, and every change an Admin can make to them. */
export function useMachines() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...listMachinesOptions(), retry: false })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: listMachinesQueryKey() })

  const create = useAction(createMachineMutation())
  const remove = useAction(deleteMachineMutation())
  const give = useAction(assignMachineRoleMutation())
  const take = useAction(unassignMachineRoleMutation())
  const rotate = useAction(rotateMachineSecretMutation())
  const actions = [create, remove, give, take, rotate]

  return {
    overview: computed<MachinesOverview | undefined>(() => query.data.value),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    actionError: computed(() => actions.map((a) => a.error.value).find((e) => e !== null) ?? null),
    createMachine: (body: { name: string; namespace: string; serviceAccount: string }, done: (m: Machine) => void) => {
      create.run({ body }, (m) => {
        refresh()
        done(m)
      })
    },
    // done runs before the caller leaves the page: a callback for an unmounted page never fires.
    deleteMachine: (machineId: string, done: () => void) => {
      remove.run({ path: { machineId } }, () => {
        refresh()
        done()
      })
    },
    setRole: (machineId: string, clientId: string, role: string, held: boolean) => {
      const path = { machineId, clientId, role }
      if (held) give.run({ path }, refresh)
      else take.run({ path }, refresh)
    },
    newSecret: (machineId: string, done: (secret: string) => void) => {
      rotate.run({ path: { machineId } }, (s) => {
        done(s.secret)
      })
    },
  }
}
