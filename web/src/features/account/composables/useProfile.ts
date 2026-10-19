import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  askEmailChangeMutation,
  cancelEmailChangeMutation,
  changePasswordMutation,
  getProfileOptions,
  getProfileQueryKey,
  updateProfileMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Profile } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** The signed-in User's Profile, and what they can change about it. */
export function useProfile() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...getProfileOptions(), retry: false })
  const rename = useAction(updateProfileMutation())
  const ask = useAction(askEmailChangeMutation())
  const cancel = useAction(cancelEmailChangeMutation())
  const refresh = () => void queryClient.invalidateQueries({ queryKey: getProfileQueryKey() })
  return {
    profile: computed<Profile | undefined>(() => query.data.value),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    rename: {
      ...rename,
      run: (name: string) => {
        rename.run({ body: { name } }, () => void queryClient.invalidateQueries({ queryKey: getProfileQueryKey() }))
      },
    },
    changePassword: useAction(changePasswordMutation()),
    askEmail: {
      ...ask,
      run: (email: string, password: string, done: () => void) => {
        ask.run({ body: { email, password } }, () => {
          done()
          refresh()
        })
      },
    },
    cancelEmail: {
      ...cancel,
      run: () => {
        cancel.run({}, refresh)
      },
    },
  }
}
