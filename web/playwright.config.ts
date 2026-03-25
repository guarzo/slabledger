import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for frontend E2E tests
 * See https://playwright.dev/docs/test-configuration
 */
export default defineConfig({
  // Test directory
  testDir: './tests',

  // Only run E2E tests (*.spec.ts), exclude Vitest unit tests (*.test.tsx)
  testMatch: /.*\.spec\.ts$/,

  // Grep pattern for filtering tests by tags
  // Use @smoke tag for critical path tests (e.g., test.describe('Login @smoke', ...))
  // Run smoke tests only: npx playwright test --grep @smoke
  // Run all except smoke: npx playwright test --grep-invert @smoke
  grep: process.env.SMOKE_ONLY ? /@smoke/ : undefined,

  // Run tests in files in parallel
  fullyParallel: true,

  // Fail the build on CI if you accidentally left test.only in the source code
  forbidOnly: !!process.env.CI,

  // Retry failed tests (Phase 6: Task 6.2)
  retries: process.env.CI ? 1 : 0,

  // Use 2 workers in CI for faster execution, unlimited locally (Phase 6: Task 6.2)
  workers: process.env.CI ? 2 : undefined,

  // Global test timeout (Phase 6: Task 6.1)
  // Increased to 60s to handle large dataset tests (200+ items)
  timeout: 60000, // 60 seconds per test

  // Assertion timeout (Phase 6: Task 6.1)
  expect: {
    timeout: 15000, // 15 seconds for expect() assertions (increased for large datasets)
  },

  // Reporter to use
  reporter: [
    ['html'],
    ['list'],
    // Add junit reporter for CI/CD
    process.env.CI ? ['junit', { outputFile: 'test-results/junit.xml' }] : null,
  ].filter(Boolean),

  // Shared settings for all the projects below
  use: {
    // Base URL to use in actions like `await page.goto('/')`
    // CI uses preview server (port 4173), local dev uses dev server (port 5173)
    baseURL: process.env.CI ? 'http://localhost:4173' : 'http://localhost:5173',

    // Action timeout (Phase 6: Task 6.1)
    actionTimeout: 10000, // 10 seconds for actions like click, fill, etc.

    // Navigation timeout (Phase 6: Task 6.1)
    navigationTimeout: 30000, // 30 seconds for page.goto()

    // Collect trace when retrying the failed test
    trace: 'on-first-retry',

    // Screenshot only on failure
    screenshot: 'only-on-failure',

    // Video only on failure
    video: 'retain-on-failure',
  },

  // Configure projects for major browsers
  // In CI, only run Chromium for speed. Locally, run all browsers.
  projects: process.env.CI
    ? [
        {
          name: 'chromium',
          use: { ...devices['Desktop Chrome'] },
        },
      ]
    : [
        {
          name: 'chromium',
          use: { ...devices['Desktop Chrome'] },
        },

        {
          name: 'firefox',
          use: { ...devices['Desktop Firefox'] },
        },

        {
          name: 'webkit',
          use: { ...devices['Desktop Safari'] },
        },

        // Mobile testing
        {
          name: 'Mobile Chrome',
          use: { ...devices['Pixel 5'] },
        },

        {
          name: 'Mobile Safari',
          use: { ...devices['iPhone 12'] },
        },
      ],

  // Run your local dev server before starting the tests
  // In CI, the preview server is already started by the workflow
  webServer: process.env.CI ? undefined : {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: true,
    timeout: 120000,
    env: {
      PLAYWRIGHT_TEST: 'true',
    },
  },
});
