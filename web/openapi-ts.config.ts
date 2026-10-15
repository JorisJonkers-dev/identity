import { defineConfig } from '@hey-api/openapi-ts'

// The whole API layer is generated from the contract: types, a fetch SDK that validates every
// response with zod, and vue-query options. Never hand-edit src/infrastructure/api.
export default defineConfig({
  input: '../openapi/v1/openapi.yaml',
  output: { path: 'src/infrastructure/api' },
  plugins: [
    '@hey-api/client-fetch',
    '@hey-api/typescript',
    // RFC 3339, which the contract's date-time names, allows any offset, not only Z.
    { name: 'zod', dates: { offset: true } },
    { name: '@hey-api/sdk', validator: true },
    '@tanstack/vue-query',
  ],
})
