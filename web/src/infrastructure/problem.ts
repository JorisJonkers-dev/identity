import { zProblem } from './api/zod.gen'

/** What to tell the user about a failed request: the problem's detail when the server sent one. */
export function describeError(error: unknown): string {
  const problem = zProblem.safeParse(error)
  if (!problem.success) return 'Something went wrong. Try again.'
  return problem.data.detail ?? problem.data.title
}
