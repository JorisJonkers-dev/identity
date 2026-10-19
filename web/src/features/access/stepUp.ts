import type { StepUp } from '../../infrastructure/api/types.gen'
import { counted } from '../admin/format'

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

/** What turning step-up on takes away, as the Admin confirms it. */
export function stepUpLoss(tokens: number, connectedApps: number, app: string): string {
  const apps = counted(connectedApps, 'connected app')
  if (!connectedApps) return `This revokes ${counted(tokens, 'token')} for ${app}. Scripts that use them stop working.`
  if (!tokens) return `This disconnects ${apps} from ${app}. Apps cannot connect again while step-up is on.`
  return `This revokes ${counted(tokens, 'token')} and disconnects ${apps} for ${app}. Scripts using the tokens stop working, and apps cannot connect again while step-up is on.`
}
