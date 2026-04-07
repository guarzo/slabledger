/**
 * Screenshot all pages of the app.
 *
 * Mocks auth and API responses so every page renders without a real backend.
 * Run: cd web && npx playwright test tests/screenshot-all-pages.spec.ts --project=chromium
 */
import { test } from '@playwright/test';

const MOCK_USER = {
  id: 1,
  username: 'demo',
  email: 'demo@cardyeti.com',
  avatar_url: '',
  is_admin: true,
  last_login_at: '2026-04-07T00:00:00Z',
};

const MOCK_CAMPAIGN = {
  id: '1',
  name: 'PSA March 2026',
  phase: 'active',
  sport: 'Pokemon',
  yearRange: '1999-2002',
  gradeRange: '8-10',
  targetSpendCents: 500000,
  ebayFeePct: 13,
  dailySpendCapCents: 10000,
  expectedFillRate: 0.75,
  createdAt: '2026-03-01T00:00:00Z',
  updatedAt: '2026-04-01T00:00:00Z',
};

const MOCK_PURCHASE = {
  id: '101',
  campaignId: '1',
  title: 'Charizard Base Set PSA 9',
  certNumber: '12345678',
  cardNumber: '4',
  priceCents: 35000,
  grade: 9,
  quantity: 1,
  createdAt: '2026-03-05T00:00:00Z',
};

const MOCK_PURCHASE_2 = {
  id: '102',
  campaignId: '1',
  title: 'Blastoise Base Set PSA 8',
  certNumber: '12345679',
  cardNumber: '2',
  priceCents: 15000,
  grade: 8,
  quantity: 1,
  createdAt: '2026-03-06T00:00:00Z',
};

const MOCK_SALE = {
  id: '201',
  purchaseId: '101',
  campaignId: '1',
  salePriceCents: 45000,
  saleChannel: 'ebay',
  saleDate: '2026-03-20',
  createdAt: '2026-03-20T00:00:00Z',
};

const MOCK_PNL = {
  campaignId: '1',
  totalSpendCents: 500000,
  totalRevenueCents: 650000,
  totalFeesCents: 84500,
  netProfitCents: 65500,
  roi: 13.1,
  avgDaysToSell: 10.5,
  totalPurchases: 20,
  totalSold: 14,
  totalUnsold: 6,
  sellThroughPct: 70,
};

const MOCK_CAPITAL_SUMMARY = {
  capitalBudgetCents: 1000000,
  outstandingCents: 350000,
  exposurePct: 35,
  refundedCents: 0,
  paidCents: 650000,
  unpaidInvoiceCount: 1,
  alertLevel: 'ok',
};

const MOCK_WEEKLY_REVIEW = {
  weekStart: '2026-03-31',
  weekEnd: '2026-04-06',
  purchasesThisWeek: 5,
  purchasesLastWeek: 3,
  spendThisWeekCents: 75000,
  spendLastWeekCents: 45000,
  salesThisWeek: 8,
  salesLastWeek: 6,
  revenueThisWeekCents: 120000,
  revenueLastWeekCents: 90000,
  profitThisWeekCents: 25000,
  profitLastWeekCents: 18000,
  byChannel: [
    { channel: 'ebay', saleCount: 5, revenueCents: 80000, feesCents: 10400, netProfitCents: 15000, avgDaysToSell: 10.5 },
    { channel: 'local', saleCount: 3, revenueCents: 40000, feesCents: 0, netProfitCents: 10000, avgDaysToSell: 5.2 },
  ],
  capitalExposurePct: 35,
  topPerformers: [
    { certNumber: '12345678', cardName: 'Charizard Base Set', grade: 9, grader: 'PSA', channel: 'ebay', profitCents: 10000 },
  ],
  bottomPerformers: [],
};

const MOCK_PORTFOLIO_HEALTH = {
  totalInvestedCents: 500000,
  totalRevenueCents: 650000,
  totalProfitCents: 65500,
  roi: 13.1,
  activeCampaigns: 1,
  totalCards: 20,
  soldCards: 14,
};

const MOCK_INVENTORY_ITEM = {
  purchase: MOCK_PURCHASE_2,
  campaignName: 'PSA March 2026',
  marketPriceCents: 18000,
  delta: 3000,
  deltaPct: 20.0,
  agingDays: 32,
  source: 'dh',
};

const MOCK_API_USAGE = {
  providers: [{
    name: 'doubleholo',
    blocked: false,
    today: {
      calls: 142,
      limit: 500,
      remaining: 358,
      successRate: 98.6,
      avgLatencyMs: 245,
      rateLimitHits: 0,
    },
  }],
  timestamp: '2026-04-07T09:00:00Z',
};

/** Set up API mocks. Pass skipAuth=true for the login page. */
async function setupMocks(page: import('@playwright/test').Page, opts?: { skipAuth?: boolean }) {
  await page.route((url) => url.pathname.startsWith('/api/'), async (route) => {
    const url = route.request().url();
    const path = new URL(url).pathname;

    // Auth
    if (path === '/api/auth/user') {
      if (opts?.skipAuth) {
        return route.fulfill({ status: 401, json: { error: 'unauthorized' } });
      }
      return route.fulfill({ json: MOCK_USER });
    }

    // Campaigns
    if (path === '/api/campaigns' && route.request().method() === 'GET') {
      return route.fulfill({ json: [MOCK_CAMPAIGN] });
    }
    if (path.match(/^\/api\/campaigns\/[^/]+$/) && route.request().method() === 'GET') {
      return route.fulfill({ json: MOCK_CAMPAIGN });
    }
    if (path.match(/\/purchases$/)) {
      return route.fulfill({ json: [MOCK_PURCHASE, MOCK_PURCHASE_2] });
    }
    if (path.match(/\/sales$/)) {
      return route.fulfill({ json: [MOCK_SALE] });
    }
    if (path.match(/\/pnl$/)) {
      return route.fulfill({ json: MOCK_PNL });
    }
    if (path.match(/\/pnl-by-channel$/)) {
      return route.fulfill({
        json: [
          { channel: 'ebay', saleCount: 10, revenueCents: 450000, feesCents: 58500, netProfitCents: 41500, avgDaysToSell: 10.5 },
          { channel: 'local', saleCount: 4, revenueCents: 200000, feesCents: 0, netProfitCents: 24000, avgDaysToSell: 5.2 },
        ],
      });
    }
    if (path.match(/\/fill-rate/)) {
      return route.fulfill({
        json: [
          { date: '2026-04-06', spendCents: 15000, capCents: 10000, fillRatePct: 1.5, purchaseCount: 3 },
          { date: '2026-04-05', spendCents: 8000, capCents: 10000, fillRatePct: 0.8, purchaseCount: 2 },
          { date: '2026-04-04', spendCents: 10000, capCents: 10000, fillRatePct: 1.0, purchaseCount: 2 },
        ],
      });
    }
    if (path.match(/\/days-to-sell$/)) {
      return route.fulfill({
        json: [
          { label: '0-7d', min: 0, max: 7, count: 4 },
          { label: '8-14d', min: 8, max: 14, count: 6 },
          { label: '15-30d', min: 15, max: 30, count: 3 },
          { label: '31+d', min: 31, max: 999, count: 1 },
        ],
      });
    }
    if (path.match(/\/inventory$/) && !path.startsWith('/api/inventory')) {
      return route.fulfill({
        json: {
          items: [{ ...MOCK_INVENTORY_ITEM, purchase: MOCK_PURCHASE_2 }],
          warnings: [],
        },
      });
    }
    if (path.match(/\/tuning$/)) {
      return route.fulfill({ json: { targetMarginPct: 20, minPriceCents: 500 } });
    }
    if (path.match(/\/crack-candidates$/)) {
      return route.fulfill({ json: [] });
    }
    if (path.match(/\/expected-values$/)) {
      return route.fulfill({ json: [] });
    }
    if (path.match(/\/activation-checklist$/)) {
      return route.fulfill({ json: { items: [], ready: true } });
    }
    if (path.match(/\/projections$/)) {
      return route.fulfill({ json: {} });
    }

    // Portfolio / global
    if (path === '/api/portfolio/health') {
      return route.fulfill({ json: MOCK_PORTFOLIO_HEALTH });
    }
    if (path === '/api/portfolio/channel-velocity') {
      return route.fulfill({ json: { channels: [] } });
    }
    if (path === '/api/portfolio/insights') {
      return route.fulfill({ json: { insights: [] } });
    }
    if (path === '/api/portfolio/capital-timeline') {
      return route.fulfill({ json: { points: [] } });
    }
    if (path === '/api/portfolio/weekly-review') {
      return route.fulfill({ json: MOCK_WEEKLY_REVIEW });
    }
    if (path === '/api/portfolio/sell-sheet' || path === '/api/sell-sheet') {
      return route.fulfill({ json: { items: [] } });
    }
    if (path === '/api/inventory') {
      return route.fulfill({
        json: {
          items: [MOCK_INVENTORY_ITEM],
          warnings: [],
        },
      });
    }

    // Sell sheet items
    if (path === '/api/sell-sheet/items') {
      return route.fulfill({ json: { purchaseIds: [] } });
    }

    // Credit
    if (path === '/api/credit/summary') {
      return route.fulfill({ json: MOCK_CAPITAL_SUMMARY });
    }
    if (path === '/api/credit/invoices' || path === '/api/invoices') {
      return route.fulfill({ json: [] });
    }

    // Favorites
    if (path.match(/\/favorites/)) {
      return route.fulfill({ json: { items: [], total: 0 } });
    }

    // Admin endpoints
    if (path === '/api/admin/api-usage') {
      return route.fulfill({ json: MOCK_API_USAGE });
    }
    if (path === '/api/admin/allowlist') {
      return route.fulfill({ json: [{ email: 'demo@cardyeti.com', addedAt: '2026-01-01T00:00:00Z' }] });
    }
    if (path === '/api/admin/users') {
      return route.fulfill({ json: [MOCK_USER] });
    }
    if (path === '/api/admin/cache-stats') {
      return route.fulfill({ json: { entries: 42, hitRate: 0.85 } });
    }
    if (path === '/api/admin/dh-status') {
      return route.fulfill({ json: { healthy: true, cachedCards: 150 } });
    }
    if (path === '/api/admin/dh-unmatched') {
      return route.fulfill({ json: [] });
    }
    if (path.match(/\/admin\//)) {
      return route.fulfill({ json: [] });
    }

    // Social
    if (path.match(/\/social/)) {
      return route.fulfill({ json: [] });
    }
    if (path.match(/\/instagram/)) {
      return route.fulfill({ json: { connected: false } });
    }

    // Advisor
    if (path.match(/\/advisor/)) {
      return route.fulfill({ json: {} });
    }

    // Picks / suggestions
    if (path.match(/\/picks/) || path.match(/\/suggestions/)) {
      return route.fulfill({ json: [] });
    }
    if (path.match(/\/watchlist/)) {
      return route.fulfill({ json: { items: [] } });
    }

    // Pricing
    if (path.match(/\/pricing/)) {
      return route.fulfill({ json: [] });
    }

    // Catch-all: return empty JSON
    return route.fulfill({ json: {} });
  });
}

const SCREENSHOT_DIR = 'screenshots';

const PAGES = [
  { name: 'login', path: '/login', skipAuth: true },
  { name: 'dashboard', path: '/' },
  { name: 'campaigns', path: '/campaigns' },
  { name: 'campaign-detail', path: '/campaigns/1' },
  { name: 'inventory', path: '/inventory' },
  { name: 'tools', path: '/tools' },
  { name: 'admin', path: '/admin' },
];

test.describe('screenshot all pages', () => {
  test.use({ actionTimeout: 60000 });

  for (const pg of PAGES) {
    test(`screenshot: ${pg.name}`, async ({ page }) => {
      // Log all API requests for debugging
      page.on('request', (req) => {
        if (req.url().includes('/api/')) {
          console.log(`[${pg.name}] REQ: ${req.method()} ${new URL(req.url()).pathname}`);
        }
      });
      page.on('response', (res) => {
        if (res.url().includes('/api/')) {
          console.log(`[${pg.name}] RES: ${res.status()} ${new URL(res.url()).pathname}`);
        }
      });

      await setupMocks(page, { skipAuth: pg.skipAuth });

      await page.setViewportSize({ width: 1440, height: 900 });

      await page.goto(pg.path, { waitUntil: 'networkidle', timeout: 30000 });

      // Wait for React to mount and render meaningful content (not just a loader)
      try {
        await page.waitForSelector('#main-content', { timeout: 10000 });
        // Wait for loading spinners to disappear
        await page.waitForFunction(() => {
          const loaders = document.querySelectorAll('[data-testid="pokeball-loader"]');
          if (loaders.length > 0) return false;
          const main = document.querySelector('#main-content');
          return main && main.textContent && main.textContent.trim().length > 30;
        }, { timeout: 15000 });
      } catch {
        // Page may still be usable
      }

      // Dismiss any Vite error overlay
      await page.evaluate(() => document.querySelector('vite-error-overlay')?.remove()).catch(() => {});

      // Extra time for React state updates and animations
      await page.waitForTimeout(1000);

      await page.screenshot({
        path: `${SCREENSHOT_DIR}/${pg.name}.png`,
        fullPage: false,
        timeout: 30000,
      });
    });
  }
});
