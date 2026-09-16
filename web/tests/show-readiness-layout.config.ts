import { defineConfig, devices } from '@playwright/test';

// Focused DOM publication regression, not the separate real-worker/PG proof.
// Own the server; never reuse a developer server or the screenshot port.
export default defineConfig({
  testDir: '.', testMatch: 'show-readiness-layout.spec.ts',
  timeout: 30_000, retries: 0, workers: 1,
  outputDir: process.env.SHOW_LAYOUT_ARTIFACTS ?? '/tmp/showprep-layout',
  reporter: [['list']],
  use: { baseURL: 'http://127.0.0.1:45173', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 45173 --strictPort',
    url: 'http://127.0.0.1:45173', reuseExistingServer: false,
    env: { PLAYWRIGHT_TEST: 'true' },
  },
});
