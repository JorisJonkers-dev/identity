import * as z from 'zod'
import { client } from './api/client.gen'

/** Points the generated client at the page's own origin: the Go binary serves the API beside the app. */
export function configureApi(options: { baseUrl?: string; fetch?: typeof fetch } = {}): void {
  // The server's Content-Security-Policy allows no eval, so zod must not probe for it.
  z.config({ jitless: true })
  client.setConfig({ baseUrl: options.baseUrl ?? '', ...(options.fetch ? { fetch: options.fetch } : {}) })
}
