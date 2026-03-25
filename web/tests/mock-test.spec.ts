import { test, expect } from '@playwright/test';
import { mockAllAPIs, setupPageWithMocks } from './fixtures';

/**
 * API Mock Verification Tests
 * Purpose: Verify that API mocking works correctly and tests can run without backend
 */

test.describe('API Mock Verification', () => {
  test('should intercept API requests and return mock data', async ({ page }) => {
    // Enable request/response logging
    page.on('response', res => {
      if (res.url().includes('/api/')) {
        console.warn(`[RESPONSE] ${res.url()} - ${res.status()}`);
      }
    });

    // Register mock BEFORE any navigation
    await mockAllAPIs(page);

    // Navigate to dashboard
    await page.goto('/');

    // Wait for page to load
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1000);

    // Check page content - should show dashboard, not login
    const heading = page.locator('h1');
    await expect(heading).toContainText('Dashboard');
  });

  test('should render authenticated state with mock user', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Should show user menu button (username text is hidden on mobile, but button is always visible)
    await expect(page.locator('button[aria-label="User menu for Test User"]')).toBeVisible();

    // Should be on the authenticated dashboard, not redirected to login
    expect(page.url()).toMatch(/\/$/); // root path = dashboard
  });

  test('should handle dashboard page with empty data', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Shows dashboard heading when visiting root (/)
    await expect(page.locator('h1')).toContainText('Dashboard');
  });

  test('should render page with React root', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Check for specific elements
    const hasRoot = await page.locator('#root').count();
    expect(hasRoot).toBeGreaterThan(0);

    // Page should have meaningful content
    const bodyText = await page.locator('body').textContent();
    expect(bodyText).toBeTruthy();
    expect(bodyText!.length).toBeGreaterThan(0);
  });
});
