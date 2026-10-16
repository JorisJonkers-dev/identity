import { defineConfig, devices } from '@playwright/test'

// localhost, not 127.0.0.1: Chromium accepts the __Host- session cookie's Secure flag from it.
const origin = 'http://localhost:18573'

// E2E runs the real binary, with the web build embedded, against the compose Postgres: the same
// artifact the image ships. `task e2e` builds it first.
export default defineConfig({
  testDir: 'tests/e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  // A flaky test is a bug to fix, not to retry.
  retries: 0,
  reporter: process.env.CI ? [['github'], ['list']] : 'list',
  use: { baseURL: origin, trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'phone', use: { ...devices['Pixel 7'] } },
  ],
  webServer: {
    command: '../bin/identity',
    url: `${origin}/readyz`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    env: {
      ADDR: new URL(origin).host,
      // Only /metrics, on a port of its own; tests/e2e/metrics.spec.ts scrapes it.
      METRICS_ADDR: '127.0.0.1:18574',
      DATABASE_URL: process.env.DATABASE_URL ?? 'postgres://app:app@localhost:55432/app?sslmode=disable',
      PUBLIC_URL: origin,
      // The compose Mailpit, which offers no TLS; only a local run may send in the clear.
      SMTP_ADDR: 'localhost:51025',
      SMTP_TLS: 'none',
      MAIL_FROM: 'identity@localhost',
      // The compose dev-mode Vault and its fixed root token, which only a local run has.
      VAULT_ADDR: 'http://localhost:58200',
      VAULT_TOKEN: 'dev-only-root',
    },
  },
})
