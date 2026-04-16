/**
 * Screenshot all pages of the app.
 *
 * Uses the real Go backend with data/slabledger.db for authentic screenshots.
 * Auth is mocked so the frontend renders as a logged-in admin user.
 *
 * Prerequisites:
 *   - Go backend running on port 4173 with LOCAL_API_TOKEN set
 *   - Frontend built (web/dist exists)
 *
 * Run via: make screenshots
 */
import { test } from '@playwright/test';

const AUTH_TOKEN = process.env.SCREENSHOT_TOKEN || 'playwright-screenshots';

const MOCK_USER = {
  id: 1,
  username: 'demo',
  email: 'demo@cardyeti.com',
  avatar_url: '',
  is_admin: true,
  last_login_at: '2026-04-07T00:00:00Z',
};

/** Mock auth + any endpoints that won't resolve cleanly in the screenshot environment. */
async function setupAuth(page: import('@playwright/test').Page, opts?: { skipAuth?: boolean }) {
  // Add bearer token so the backend authenticates API requests
  await page.setExtraHTTPHeaders({ Authorization: `Bearer ${AUTH_TOKEN}` });

  // Mock the auth/user endpoint — the frontend checks this to determine login state.
  await page.route('**/api/auth/user', async (route) => {
    if (opts?.skipAuth) {
      return route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthorized' }) });
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_USER) });
  });

  // Mock advisor SSE endpoints — these fail without a configured AI provider
  await page.route('**/api/advisor/**', async (route) => {
    return route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' });
  });
}

const SCREENSHOT_DIR = 'screenshots';

// Use the first campaign from the real database.
// The campaign-detail page needs a real ID; we fetch it dynamically.
let firstCampaignId: string | null = null;

const PAGES = [
  { name: 'login', path: '/login', skipAuth: true },
  { name: 'dashboard', path: '/' },
  { name: 'campaigns', path: '/campaigns' },
  { name: 'campaign-detail', path: () => `/campaigns/${firstCampaignId || 'unknown'}` },
  { name: 'inventory', path: '/inventory', filterTab: 'Needs Attention' },
  { name: 'inventory-expanded', path: '/inventory', filterTab: 'Needs Attention', expandRow: true, desktopOnly: true },
  { name: 'tools', path: '/tools' },
  { name: 'admin-users', path: '/admin' },
  { name: 'admin-pricing', path: '/admin', tabLabel: 'Pricing' },
  { name: 'admin-stats', path: '/admin', tabLabel: 'Stats' },
  { name: 'admin-integrations', path: '/admin', tabLabel: 'Integrations' },
];

const VIEWPORTS = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'mobile', width: 390, height: 844 },   // iPhone 14
];

async function screenshotPage(
  page: import('@playwright/test').Page,
  pg: { name: string; path: string | (() => string); skipAuth?: boolean; tabLabel?: string; filterTab?: string; expandRow?: boolean; desktopOnly?: boolean },
  viewport: { name: string; width: number; height: number },
) {
  await setupAuth(page, { skipAuth: pg.skipAuth });

  await page.setViewportSize({ width: viewport.width, height: viewport.height });

  const url = typeof pg.path === 'function' ? pg.path() : pg.path;
  await page.goto(url, { waitUntil: 'networkidle', timeout: 30000 });

  // Wait for React to mount and render meaningful content (not just a loader)
  try {
    await page.waitForSelector('#main-content', { timeout: 10000 });
    await page.waitForFunction(() => {
      const loaders = document.querySelectorAll('[data-testid="pokeball-loader"]');
      if (loaders.length > 0) return false;
      const main = document.querySelector('#main-content');
      return main && main.textContent && main.textContent.trim().length > 30;
    }, { timeout: 15000 });
  } catch {
    // Page may still be usable
  }

  // If a specific tab is requested, click it and wait for content to update
  if (pg.tabLabel) {
    const tab = page.getByRole('tab', { name: pg.tabLabel, exact: true });
    await tab.click();
    await page.waitForTimeout(800);
    // Wait for any newly triggered requests to settle
    await page.waitForLoadState('networkidle').catch(() => {});
    // Wait for "Loading..." text to clear (e.g. integration status cards)
    try {
      await page.waitForFunction(() => {
        const els = document.querySelectorAll('p');
        return ![...els].some(el => /^Loading .* status/.test(el.textContent || ''));
      }, { timeout: 10000 });
    } catch {
      // Some loading states may not resolve (e.g. missing external service config)
    }
  }

  // If a specific filter tab is requested (e.g. inventory filter buttons), click it.
  // The button may not render if the backend is unavailable (CI without Go server).
  if (pg.filterTab) {
    try {
      const filterBtn = page.getByRole('button', { name: new RegExp(pg.filterTab), exact: false });
      await filterBtn.click({ timeout: 10000 });
      await page.waitForTimeout(500);
      // Wait for rows to render after filter change
      try {
        await page.locator('div[role="row"]').first().waitFor({ state: 'visible', timeout: 5000 });
      } catch {
        // Filter may result in no rows
      }
    } catch {
      // Filter button may not exist (e.g. empty inventory with no backend data)
    }
  }

  // If row expansion is requested (inventory detail), click the first data row.
  // Row may not exist if backend is unavailable (CI without Go server).
  if (pg.expandRow) {
    try {
      const row = page.locator('div[role="row"]').first();
      await row.click({ timeout: 10000 });
      await page.waitForTimeout(500);
      // Wait for the expanded detail panel to appear
      await page.locator('.glass-vrow-expanded').waitFor({ state: 'visible', timeout: 5000 });
    } catch {
      // Rows may not exist or expansion may not be available
    }
  }

  // Extra time for React state updates and animations
  await page.waitForTimeout(1000);

  const dir = viewport.name === 'desktop' ? SCREENSHOT_DIR : `${SCREENSHOT_DIR}/${viewport.name}`;
  await page.screenshot({
    path: `${dir}/${pg.name}.png`,
    fullPage: false,
    timeout: 30000,
  });
}

test.describe('screenshot all pages', () => {
  test.use({ actionTimeout: 60000 });

  // Fetch the first campaign ID from the real backend before running tests
  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    try {
      const response = await page.request.get('/api/campaigns', {
        headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
      });
      if (response.ok()) {
        const campaigns = await response.json();
        if (Array.isArray(campaigns) && campaigns.length > 0) {
          firstCampaignId = campaigns[0].id;
        }
      }
    } catch (err) {
      // Fall back — campaign-detail will show a not-found page
      console.error('Failed to fetch campaigns for firstCampaignId:', err);
    }
    await page.close();
  });

  for (const viewport of VIEWPORTS) {
    for (const pg of PAGES) {
      // Skip desktop-only pages on mobile viewports
      if (pg.desktopOnly && viewport.name !== 'desktop') continue;
      test(`${viewport.name}: ${pg.name}`, async ({ page }) => {
        await screenshotPage(page, pg, viewport);
      });
    }
  }
});
