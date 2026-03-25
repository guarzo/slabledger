/* eslint-disable @typescript-eslint/no-explicit-any */
import { test as base, Page } from '@playwright/test';

/**
 * Test fixtures for SlabLedger E2E tests
 * Provides common setup and utilities
 */

export type Fixtures = {
  // Page fixture with automatic API mocking enabled
  autoMockedPage: Page;
};

/**
 * Extend base test with custom fixtures
 * Provides automatic API mocking to eliminate backend dependency
 */
export const test = base.extend<Fixtures>({
  // Page fixture with automatic API route mocking
  // Use this in tests that need automatic mocking: test('name', async ({ autoMockedPage }) => ...)
  autoMockedPage: async ({ page }, use) => {
    // Register global API mock BEFORE any navigation
    console.warn('[FIXTURE] Setting up automatic API mocking...');

    await page.route(url => new URL(url).pathname.startsWith('/api/'), async (route) => {
      const url = route.request().url();
      const method = route.request().method();

      console.warn(`[AUTO-MOCK] Intercepted ${method} ${url}`);

      // Handle auth endpoint (return mock authenticated user)
      if (url.includes('/api/auth/user')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { 'Access-Control-Allow-Origin': '*' },
          body: JSON.stringify({
            id: 1,

            username: 'Test User',
            email: 'test@example.com',
            avatar_url: 'https://example.com/avatar.png',
            last_login_at: new Date().toISOString(),
          }),
        });
        return;
      }

      // Handle favorites endpoint
      if (url.includes('/api/favorites')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { 'Access-Control-Allow-Origin': '*' },
          body: JSON.stringify({ favorites: [], total: 0 }),
        });
        return;
      }

      // Handle campaigns endpoint
      if (url.includes('/api/campaigns')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { 'Access-Control-Allow-Origin': '*' },
          body: JSON.stringify([]),
        });
        return;
      }

      // Handle portfolio endpoints for dashboard
      if (url.includes('/api/portfolio/')) {
        const path = new URL(url).pathname;
        const matchedKey = Object.keys(PORTFOLIO_MOCK).find(k => path === k);
        if (matchedKey) {
          await route.fulfill({ status: 200, contentType: 'application/json', headers: { 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify(PORTFOLIO_MOCK[matchedKey]) });
        } else {
          await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
        }
        return;
      }
      if (url.includes('/api/credit/summary')) {
        await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
        return;
      }

      // Default: return empty success for other API calls
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: {
          'Access-Control-Allow-Origin': '*',
        },
        body: JSON.stringify({}),
      });
    });

    console.warn('[FIXTURE] Automatic API mocking ready');
    await use(page);
  },
});

export { expect } from '@playwright/test';

/**
 * Common page objects and selectors
 */
export const selectors = {
  // Navigation
  header: 'header',
  nav: 'nav[role="navigation"]',
  navLink: 'nav a',

  // Dashboard page (home)
  dashboardHeading: 'text=Dashboard',

  // Price Lookup (drawer triggered from header)
  priceLookupButton: 'button[aria-label="Price Lookup"]',

  // Campaigns page
  campaignsHeading: 'text=Campaigns',

  // Common
  loader: '[data-testid="pokeball-loader"]',
  button: 'button',
};

/**
 * Helper to mock all API routes for authenticated pages
 */
export async function mockAllAPIs(page: Page, options?: { favorites?: any[] }) {
  console.warn('[MOCK] Registering catch-all API routes...');

  await page.route(url => new URL(url).pathname.startsWith('/api/'), async (route) => {
    const url = route.request().url();
    console.warn(`[MOCK] Intercepted API call: ${url}`);

    if (url.includes('/api/auth/user')) {
      // Return mock authenticated user
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({
          id: 1,
          google_id: 'test-google-id',
          username: 'Test User',
          email: 'test@example.com',
          avatar_url: 'https://example.com/avatar.png',
          last_login_at: new Date().toISOString(),
        }),
      });
    } else if (url.includes('/api/favorites')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({
          favorites: options?.favorites || [],
          total: options?.favorites?.length || 0,
        }),
      });
    } else if (url.includes('/api/campaigns')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify([]),
      });
    } else if (url.includes('/api/cards/search')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({ cards: [] }),
      });
    } else if (url.includes('/api/portfolio/')) {
      // Dashboard portfolio endpoints — return safe empty data
      const path = new URL(url).pathname;
      const matchedKey = Object.keys(PORTFOLIO_MOCK).find(k => path === k);
      if (matchedKey) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { 'Access-Control-Allow-Origin': '*' },
          body: JSON.stringify(PORTFOLIO_MOCK[matchedKey]),
        });
      } else {
        // weekly-review and others: 404 so react-query gets no data
        await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
      }
    } else if (url.includes('/api/credit/summary')) {
      await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
    } else {
      // Default: return empty success for other API calls
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({}),
      });
    }
  });

  await page.waitForTimeout(100);
  console.warn('[MOCK] All API routes registered');
}

/**
 * Wait for React app to be ready and hydrated
 */
export async function waitForReactHydration(page: Page) {
  await page.waitForFunction(() => {
    // Check if React root exists and has content
    const root = document.querySelector('#root > *');
    return root !== null;
  }, { timeout: 10000 });
}

/**
 * Setup page with mocks and navigate - unified helper for consistent test setup
 */
export async function setupPageWithMocks(
  page: Page,
  path: string,
  options?: { favorites?: any[] }
) {
  console.warn(`[SETUP] Setting up page: ${path}`);

  // Register mock FIRST, before any navigation
  await mockAllAPIs(page, options);

  // Then navigate
  await page.goto(path);

  // Wait for app to be ready
  await waitForReactHydration(page);

  // Wait for page to stabilize
  await page.waitForTimeout(300);

  console.warn('[SETUP] Page setup complete');
}

/**
 * Empty portfolio mock data — single source of truth for dashboard endpoint stubs.
 */
export const PORTFOLIO_MOCK: Record<string, unknown> = {
  '/api/portfolio/health': { campaigns: [], overallROI: 0, totalDeployedCents: 0, totalRecoveredCents: 0, totalAtRiskCents: 0 },
  '/api/portfolio/capital-timeline': { dataPoints: [] },
  '/api/portfolio/insights': { dataSummary: { totalPurchases: 0 } },
  '/api/portfolio/channel-velocity': [],
  '/api/portfolio/suggestions': { suggestions: [] },
};

/**
 * Mock data generators for testing
 */
export const mockData = {
  /**
   * Generate mock favorite items
   */
  favorites: (count: number) => {
    return Array.from({ length: count }, (_, i) => ({
      id: i + 1,
      user_id: 1,
      card_name: `Test Card ${i + 1}`,
      set_name: `Test Set ${Math.floor(i / 10) + 1}`,
      card_number: String(i + 1).padStart(3, '0'),
      image_url: `https://images.pokemontcg.io/base1/${i + 1}.png`,
      notes: i % 3 === 0 ? `Note for card ${i + 1}` : '',
      created_at: new Date().toISOString(),
    }));
  },
};
