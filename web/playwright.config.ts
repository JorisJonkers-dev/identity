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
  // An Admin's flow signs up, sets up two-factor and makes a Group before it starts.
  timeout: 60_000,
  // CI shards each write a blob report, which one job merges into a single HTML report.
  reporter: process.env.CI ? [['blob'], ['github'], ['list']] : 'list',
  use: { baseURL: origin, trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'phone', use: { ...devices['Pixel 7'] } },
  ],
  webServer: [
    {
      // A Client's own host, serving the icons tests/e2e/clients.yaml names.
      command: 'node tests/e2e/icon-host.ts',
      url: 'http://localhost:18603/healthz',
      reuseExistingServer: !process.env.CI,
    },
    {
      command: '../bin/identity',
      url: `${origin}/readyz`,
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
      env: {
        ADDR: new URL(origin).host,
        // Only /metrics, on a port of its own; tests/e2e/metrics.spec.ts scrapes it.
        METRICS_ADDR: '127.0.0.1:18574',
        CLIENTS_FILE: 'tests/e2e/clients.yaml',
        DATABASE_URL: process.env.DATABASE_URL ?? 'postgres://app:app@localhost:25432/app?sslmode=disable',
        PUBLIC_URL: origin,
        // The compose Mailpit, which offers no TLS; only a local run may send in the clear. CI's
        // e2e job reaches its service containers by name, so it names them in E2E_*.
        SMTP_ADDR: process.env.E2E_SMTP_ADDR ?? 'localhost:21025',
        SMTP_TLS: 'none',
        MAIL_FROM: 'identity@localhost',
        // The compose dev-mode Vault and its fixed root token, which only a local run has.
        VAULT_ADDR: process.env.E2E_VAULT_ADDR ?? 'http://localhost:28200',
        VAULT_TOKEN: 'dev-only-root',
        // Only linked from the dashboard; tests/e2e/admin.spec.ts never follows it.
        GRAFANA_URL: 'https://grafana.example.org/d/identity',
      },
    },
  ],
})
