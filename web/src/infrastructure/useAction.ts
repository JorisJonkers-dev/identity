import { type UseMutationOptions, useMutation } from '@tanstack/vue-query'
import { computed } from 'vue'
import { describeError } from './problem'

/**
 * One API call a form makes: run it, and read whether it is running, what went wrong, and whether
 * it succeeded. Components never call the API themselves.
 */
export function useAction<TData, TError, TVariables>(options: UseMutationOptions<TData, TError, TVariables>) {
  const mutation = useMutation(options)
  return {
    run: (variables: TVariables, onSuccess?: () => void) => {
      mutation.mutate(variables, { onSuccess: () => onSuccess?.() })
    },
    isPending: mutation.isPending,
    isDone: mutation.isSuccess,
    error: computed(() => (mutation.error.value ? describeError(mutation.error.value) : null)),
  }
}
