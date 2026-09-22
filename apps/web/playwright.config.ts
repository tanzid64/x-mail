import { defineConfig, devices } from '@playwright/test'

/**
 * The end-to-end suite runs the whole stack — Vite, the Go API, Postgres and a
 * stub provider — on its own ports and its own database, so it can run
 * alongside `make dev` without either disturbing the other.
 *
 * `make infra` must be running; it creates the xmail_e2e database.
 */
const STUB_RESEND_PORT = 8099
const API_PORT = 8091
const WEB_PORT = 5181
const WORKER_HEALTH_PORT = 8093

const E2E_DATABASE_URL =
  process.env.E2E_DATABASE_URL ?? 'postgres://xmail:xmail@localhost:5433/xmail_e2e?sslmode=disable'

/** Shared by the API and the worker so they agree on credentials and storage. */
const BACKEND_ENV = {
  APP_ENV: 'test',
  DATABASE_URL: E2E_DATABASE_URL,
  REDIS_ADDR: 'localhost:6380',
  // A separate Redis database keeps queues and rate-limit counters from
  // leaking between the suite and a running dev server.
  REDIS_DB: '2',
  SESSION_SECRET: 'e2e-session-secret-that-is-long-enough-32',
  ENCRYPTION_MASTER_KEY: 'ZTJlLW1hc3Rlci1rZXktZXhhY3RseS0zMi1ieXRlcyE=',
  // Point the provider at the stub instead of the real Resend API.
  RESEND_BASE_URL: `http://localhost:${STUB_RESEND_PORT}`,
  STORAGE_DRIVER: 'local',
  STORAGE_LOCAL_PATH: './.e2e-storage',
}

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.ts',
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: `http://localhost:${WEB_PORT}`,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'node e2e/stub-resend.mjs',
      port: STUB_RESEND_PORT,
      reuseExistingServer: false,
      env: { STUB_RESEND_PORT: String(STUB_RESEND_PORT) },
      timeout: 30_000,
    },
    {
      command: 'go run ./cmd/server',
      cwd: '../api',
      url: `http://localhost:${API_PORT}/healthz`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        ...BACKEND_ENV,
        HTTP_ADDR: `:${API_PORT}`,
        CORS_ORIGINS: `http://localhost:${WEB_PORT}`,
        APP_BASE_URL: `http://localhost:${WEB_PORT}`,
        // The suite registers a user per test from one IP; the real limit of
        // 5 signups an hour would stop it after the fifth.
        RATE_LIMIT_SCALE: '100',
      },
    },
    {
      // Sending is asynchronous, so a send only completes if a worker is
      // running. Testing the compose flow without one would prove nothing.
      command: 'go run ./cmd/worker',
      cwd: '../api',
      env: { ...BACKEND_ENV, WORKER_HTTP_ADDR: `:${WORKER_HEALTH_PORT}` },
      url: `http://localhost:${WORKER_HEALTH_PORT}/healthz`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'npm run dev',
      url: `http://localhost:${WEB_PORT}`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        WEB_PORT: String(WEB_PORT),
        API_PROXY_TARGET: `http://localhost:${API_PORT}`,
      },
    },
  ],
})
