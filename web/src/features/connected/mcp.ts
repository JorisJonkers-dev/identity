import { counted } from '../admin/format'

/** How an app checks a tool's token: through the Gate, by introspection with its secret, or not at all. */
export type TokenCheck = 'gate' | 'introspection' | 'none'

/** What an app's connected-apps setting means, as its page says it. */
export function mcpSummary(on: boolean, connectedApps: number, stepUp: boolean, app: string, tokenCheck: TokenCheck): string {
  if (tokenCheck === 'none') return `${app} cannot take tools. It signs people in itself and has no secret, so it cannot check a tool's token.`
  if (!on) return `Off. No tool can use ${app} as someone.`
  if (stepUp) return `On, but tools cannot connect while ${app} asks for two-factor again.`
  const own = tokenCheck === 'introspection' ? ` ${app} checks each tool's token itself.` : ''
  return `People can let tools use ${app} as them. ${counted(connectedApps, 'tool')} connected.${own}`
}
