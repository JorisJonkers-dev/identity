import { counted } from '../admin/format'

/** What an app's connected-apps setting means, as its page says it. */
export function mcpSummary(on: boolean, connectedApps: number, stepUp: boolean, app: string): string {
  if (!on) return `Off. No app can use ${app} as someone.`
  if (stepUp) return `On, but apps cannot connect while ${app} asks for two-factor again.`
  return `People can let apps use ${app} as them. ${counted(connectedApps, 'app')} connected.`
}
