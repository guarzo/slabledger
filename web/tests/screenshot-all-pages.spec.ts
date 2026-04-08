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

const MOCK_CACHE_STATS = {
  enabled: true,
  totalSets: 48,
  finalizedSets: 45,
  discoveredSets: 3,
  lastUpdated: '2026-04-07T08:00:00Z',
  registryVersion: 'v2.1',
  sets: [
    { id: 'base1', name: 'Base Set', series: 'Base', releaseDate: '1999-01-09', totalCards: 102, status: 'finalized', fetchedAt: '2026-04-01T00:00:00Z' },
    { id: 'jungle', name: 'Jungle', series: 'Base', releaseDate: '1999-06-16', totalCards: 64, status: 'finalized', fetchedAt: '2026-04-01T00:00:00Z' },
    { id: 'fossil', name: 'Fossil', series: 'Base', releaseDate: '1999-10-10', totalCards: 62, status: 'finalized', fetchedAt: '2026-04-01T00:00:00Z' },
  ],
};

const MOCK_CARD_REQUESTS: object[] = [];

const MOCK_PRICING_DIAGNOSTICS = {
  totalMappedCards: 312,
  unmappedCards: 8,
  recentFailures: [
    { provider: 'doubleholo', errorType: 'timeout', count: 2, lastSeen: '2026-04-07T07:30:00Z' },
  ],
};

const MOCK_PRICE_FLAGS = {
  flags: [
    {
      id: 1,
      purchaseId: '101',
      flaggedBy: 1,
      flaggedAt: '2026-04-06T12:00:00Z',
      reason: 'source_disagreement',
      cardName: 'Charizard Base Set',
      grade: 9,
      certNumber: '12345678',
      flaggedByEmail: 'demo@cardyeti.com',
      marketPriceCents: 35000,
      clValueCents: 32000,
      reviewedPriceCents: 34000,
    },
  ],
  total: 1,
};

const MOCK_AI_USAGE = {
  configured: true,
  summary: {
    totalCalls: 87,
    successRate: 97.7,
    totalInputTokens: 42000,
    totalOutputTokens: 18000,
    totalTokens: 60000,
    avgLatencyMs: 620,
    rateLimitHits: 0,
    callsLast24h: 12,
    lastCallAt: '2026-04-07T08:45:00Z',
    totalCostCents: 310,
  },
  operations: [
    { operation: 'advisor', calls: 55, errors: 1, successRate: 98.2, avgLatencyMs: 580, totalTokens: 38000, totalCostCents: 210 },
    { operation: 'social_content', calls: 32, errors: 1, successRate: 96.9, avgLatencyMs: 690, totalTokens: 22000, totalCostCents: 100 },
  ],
  timestamp: '2026-04-07T09:00:00Z',
};

const MOCK_PRICE_OVERRIDE_STATS = {
  totalUnsold: 6,
  overrideCount: 2,
  manualCount: 1,
  costMarkupCount: 1,
  aiAcceptedCount: 0,
  pendingSuggestions: 3,
  overrideTotalUsd: 420.00,
  suggestionTotalUsd: 380.00,
};

const MOCK_DH_STATUS = {
  intelligence_count: 1240,
  intelligence_last_fetch: '2026-04-07T06:00:00Z',
  suggestions_count: 48,
  suggestions_last_fetch: '2026-04-07T06:05:00Z',
  unmatched_count: 8,
  pending_count: 2,
  mapped_count: 304,
  bulk_match_running: false,
  api_health: { total_calls: 142, failures: 3, success_rate: 97.9 },
  dh_inventory_count: 312,
  dh_listings_count: 289,
  dh_orders_count: 57,
};

const MOCK_DH_PUSH_CONFIG = {
  swingPctThreshold: 10,
  swingMinCents: 500,
  disagreementPctThreshold: 15,
  unreviewedChangePctThreshold: 20,
  unreviewedChangeMinCents: 1000,
  updatedAt: '2026-04-01T00:00:00Z',
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
      return route.fulfill({ json: MOCK_CACHE_STATS });
    }
    if (path === '/api/admin/card-requests') {
      return route.fulfill({ json: MOCK_CARD_REQUESTS });
    }
    if (path === '/api/admin/pricing-diagnostics') {
      return route.fulfill({ json: MOCK_PRICING_DIAGNOSTICS });
    }
    if (path === '/api/admin/price-flags') {
      return route.fulfill({ json: MOCK_PRICE_FLAGS });
    }
    if (path === '/api/admin/ai-usage') {
      return route.fulfill({ json: MOCK_AI_USAGE });
    }
    if (path === '/api/admin/price-override-stats') {
      return route.fulfill({ json: MOCK_PRICE_OVERRIDE_STATS });
    }
    if (path === '/api/admin/dh-push-config') {
      return route.fulfill({ json: MOCK_DH_PUSH_CONFIG });
    }
    if (path === '/api/admin/dh-status') {
      return route.fulfill({ json: { healthy: true, cachedCards: 150 } });
    }
    if (path === '/api/admin/dh-unmatched') {
      return route.fulfill({ json: [] });
    }
    if (path === '/api/admin/cardladder/status') {
      return route.fulfill({ json: { configured: false } });
    }
    if (path === '/api/admin/marketmovers/status') {
      return route.fulfill({ json: { configured: false } });
    }
    if (path.match(/\/admin\//)) {
      return route.fulfill({ json: [] });
    }

    // DH endpoints
    if (path === '/api/dh/status') {
      return route.fulfill({ json: MOCK_DH_STATUS });
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
  { name: 'admin-integrations', path: '/admin' },
  { name: 'admin-users', path: '/admin', tabLabel: 'Users' },
  { name: 'admin-card-data', path: '/admin', tabLabel: 'Card Data' },
  { name: 'admin-pricing', path: '/admin', tabLabel: 'Pricing' },
  { name: 'admin-ai', path: '/admin', tabLabel: 'AI' },
];

const VIEWPORTS = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'mobile', width: 390, height: 844 },   // iPhone 14
];

async function screenshotPage(
  page: import('@playwright/test').Page,
  pg: { name: string; path: string; skipAuth?: boolean; tabLabel?: string },
  viewport: { name: string; width: number; height: number },
) {
  await setupMocks(page, { skipAuth: pg.skipAuth });

  await page.setViewportSize({ width: viewport.width, height: viewport.height });

  await page.goto(pg.path, { waitUntil: 'networkidle', timeout: 30000 });

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
  }

  // Dismiss any Vite error overlay
  await page.evaluate(() => document.querySelector('vite-error-overlay')?.remove()).catch(() => {});

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

  for (const viewport of VIEWPORTS) {
    for (const pg of PAGES) {
      test(`${viewport.name}: ${pg.name}`, async ({ page }) => {
        await screenshotPage(page, pg, viewport);
      });
    }
  }
});
