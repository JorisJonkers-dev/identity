import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import {
  confirmPasskeyMutation,
  confirmTotpMutation,
  listTwoFactorMethodsOptions,
  listTwoFactorMethodsQueryKey,
  removeTwoFactorMethodMutation,
  startPasskeyMutation,
  startTotpMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { TwoFactorMethod, TotpSetup } from '../../../infrastructure/api/types.gen'
import { passkeys } from '../../../infrastructure/passkeys'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** The signed-in User's authenticator apps and passkeys, and adding and removing them. */
export function useSecurity() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...listTwoFactorMethodsOptions(), retry: false })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: listTwoFactorMethodsQueryKey() })
  const setup = ref<TotpSetup | null>(null)
  const start = useAction(startTotpMutation())
  const confirm = useAction(confirmTotpMutation())
  const remove = useAction(removeTwoFactorMethodMutation())
  const startKey = useAction(startPasskeyMutation())
  const confirmKey = useAction(confirmPasskeyMutation())
  const browserRefused = ref<string | null>(null)

  return {
    factors: computed<TwoFactorMethod[]>(() => query.data.value?.factors ?? []),
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
    passkeysSupported: passkeys.supported(),
    addPasskey: {
      isPending: computed(() => startKey.isPending.value || confirmKey.isPending.value),
      error: computed(() => browserRefused.value ?? startKey.error.value ?? confirmKey.error.value),
      run: (label: string, password: string, done: () => void) => {
        browserRefused.value = null
        startKey.run({}, (options) => {
          passkeys.create(options.options).then(
            (response) => {
              confirmKey.run({ body: { label, response, password } }, () => {
                done()
                refresh()
              })
            },
            () => (browserRefused.value = 'No passkey was made. Try again, then follow the prompt your browser shows.'),
          )
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
