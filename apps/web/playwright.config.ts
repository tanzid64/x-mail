import { defineConfig, devices } from '@playwright/test'

/**
 * End-to-end tests drive the real stack: the Vite dev server, the Go API, and
 * the Postgres/Redis started by `make infra`.
 *
 * Both servers are started here and reused if they are already running, so the
 * suite works whether or not `make dev` is up.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? 'list' : [['list']],
  use: {
    baseURL: 'http://localhost:5180',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'go run ./cmd/server',
      cwd: '../api',
      url: 'http://localhost:8090/healthz',
      reuseExistingServer: true,
      timeout: 120_000,
    },
    {
      command: 'npm run dev',
      url: 'http://localhost:5180',
      reuseExistingServer: true,
      timeout: 120_000,
    },
  ],
})
