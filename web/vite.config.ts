import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig, type Plugin } from 'vitest/config'

const api = process.env.API_ORIGIN ?? 'http://localhost:8080'
// Everything the Go server answers itself; Vite serves the app.
const proxy = Object.fromEntries(['/api', '/oidc', '/.well-known', '/gate', '/.auth', '/icons', '/healthz', '/readyz'].map((p) => [p, api]))

// web/embed.go embeds dist/, and go:embed needs a file there before the app is built, so the
// tracked dist/.gitkeep is put back after every build empties the directory.
const keepDist: Plugin = {
  name: 'keep-dist',
  apply: 'build',
  closeBundle() {
    writeFileSync(resolve(import.meta.dirname, 'dist/.gitkeep'), '')
  },
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), keepDist],
  // IPv4 explicitly: Node binds localhost to ::1 alone, which a browser resolving 127.0.0.1 cannot reach.
  server: { proxy, host: '127.0.0.1', port: 5173, strictPort: true },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.spec.ts'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,vue}'],
      // Generated code is the contract's, and main.ts is the composition root E2E covers.
      exclude: ['src/infrastructure/api/**', 'src/main.ts', 'src/**/*.spec.ts', 'src/env.d.ts'],
      // Every unit-testable line, branch and function (docs/adr/0011); main.ts is the composition
      // root the e2e suite covers.
      thresholds: {
        lines: 100,
        branches: 100,
        functions: 100,
        statements: 100,
      },
    },
  },
})
