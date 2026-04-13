import { test, expect, selectors, setupPageWithMocks } from './fixtures';

/**
 * E2E tests for core page rendering
 * Tests that the main pages load correctly with mocked APIs
 */

test.describe('Dashboard Page - Rendering @smoke', () => {
  test('should render the dashboard page', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Should show page heading
    const heading = page.locator('h1');
    await expect(heading).toContainText('Dashboard');
  });

  test('should render header with navigation', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Header should be visible
    const header = page.locator(selectors.header);
    await expect(header).toBeVisible();

    // Navigation should have links (Dashboard, Campaigns, Inventory, Insights, Tools)
    const navLinks = page.locator(selectors.navLink);
    const count = await navLinks.count();
    expect(count).toBeGreaterThanOrEqual(3);
  });

  test('should show authenticated user in header', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Should show user menu button (username text is hidden on mobile, but button is always visible)
    await expect(page.locator('button[aria-label="User menu for Test User"]')).toBeVisible();
  });
});

test.describe('Dashboard Page - Rendering', () => {
  test('should show dashboard when navigating to root', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Should show dashboard heading
    await expect(page.locator('h1')).toContainText('Dashboard');
  });
});

test.describe('Navigation @smoke', () => {
  test('should navigate between pages', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Navigate to Campaigns
    const campaignsLink = page.locator('nav a', { hasText: /Campaign/ });
    await campaignsLink.click();
    await page.waitForURL('**/campaigns');

    // Navigate to Dashboard
    const dashboardLink = page.locator('nav a', { hasText: /Dashboard|Home/ });
    await dashboardLink.click();
    await page.waitForURL(/^https?:\/\/[^/]+\/$/);

    await expect(page.locator('h1')).toContainText('Dashboard');

    // Navigate to Inventory
    const inventoryLink = page.locator('nav a', { hasText: /Inventory|Inv/ });
    await inventoryLink.click();
    await page.waitForURL('**/inventory');
  });

  test('should show dashboard at root', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Root shows dashboard
    await expect(page.locator('h1')).toContainText('Dashboard');
  });
});
