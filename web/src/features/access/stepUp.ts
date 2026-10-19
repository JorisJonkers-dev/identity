import type { StepUp } from '../../infrastructure/api/types.gen'

/** The step-up windows an Admin picks from, as the dropdown offers them. */
export const stepUpChoices = [
  { value: '0', label: 'Off' },
  { value: '5', label: '5 minutes' },
  { value: '15', label: '15 minutes' },
  { value: '60', label: '60 minutes' },
]

/** The window a dropdown choice stands for. */
export function minutesOf(choice: string): StepUp['minutes'] {
  return Number(choice) as StepUp['minutes']
}

/** What an app's step-up means for the person opening it. */
export function stepUpSummary(minutes: number | undefined, app: string): string {
  return minutes ? `Asks for two-factor again after ${String(minutes)} minutes.` : `Off. Signed-in people open ${app} without a new check.`
}
