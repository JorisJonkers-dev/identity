import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import {
  confirmTotpMutation,
  listSecondFactorsOptions,
  listSecondFactorsQueryKey,
  removeSecondFactorMutation,
  startTotpMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { SecondFactor, TotpSetup } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** The signed-in User's authenticator apps, and adding and removing them. */
export function useSecurity() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...listSecondFactorsOptions(), retry: false })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: listSecondFactorsQueryKey() })
  const setup = ref<TotpSetup | null>(null)
  const start = useAction(startTotpMutation())
  const confirm = useAction(confirmTotpMutation())
  const remove = useAction(removeSecondFactorMutation())

  return {
    factors: computed<SecondFactor[]>(() => query.data.value?.factors ?? []),
    isLoading: query.isPending,
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    setup,
    start: {
      ...start,
      run: () => {
        start.run({}, (s) => (setup.value = s))
      },
    },
    confirm: {
      ...confirm,
      run: (label: string, code: string, password: string, done: () => void) => {
        confirm.run({ body: { label, code: code.replace(/\s/g, ''), password } }, () => {
          setup.value = null
          done()
          refresh()
        })
      },
    },
    remove: {
      ...remove,
      run: (factorId: string, password: string) => {
        remove.run({ path: { factorId }, body: { password } }, refresh)
      },
    },
  }
}
