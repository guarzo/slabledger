import { test, expect } from '@playwright/test';
import { mockAllAPIs } from '../fixtures';

/**
 * Visual Regression Tests
 *
 * These tests capture screenshots of key pages and components to detect
 * unintended visual changes. Baselines are stored in tests/e2e/__screenshots__/
 *
 * To update baselines:
 *   npx playwright test visual-regression --update-snapshots
 *
 * To run tests:
 *   npx playwright test visual-regression
 */

/**
 * Helper to wait for page stability after navigation
 */
async function waitForPageStability(page: import('@playwright/test').Page, timeout = 3000) {
  // Wait for DOM content to load first
  await page.waitForLoadState('domcontentloaded');

  // Wait for loading indicators to disappear
  await page.waitForSelector('[data-testid="pokeball-loader"]', { state: 'hidden', timeout })
    .catch(() => {
      // No loading indicator
    });

  // Wait for images to load with a timeout to avoid hanging on external images
  await page.evaluate(() => {
    const timeout = 3000;
    const images = Array.from(document.images).filter(img => !img.complete);
    return Promise.race([
      Promise.all(images.map(img => new Promise(resolve => {
        img.onload = img.onerror = resolve;
      }))),
      new Promise(resolve => setTimeout(resolve, timeout))
    ]);
  });

  // Additional stability wait for animations and rendering
  await page.waitForTimeout(500);
}

test.describe('Visual Regression - Pages @visual', () => {
  // Skip full-page visual tests in CI - page heights vary across environments
  // due to font rendering and CSS computation differences.
  // Run locally with: npx playwright test visual-regression --update-snapshots
  test.skip(!process.env.RUN_VISUAL_REGRESSION, 'Visual tests skipped unless RUN_VISUAL_REGRESSION is set');

  test.beforeEach(async ({ page }) => {
    // Set viewport to desktop size
    await page.setViewportSize({ width: 1280, height: 720 });

    // Mock all API endpoints to prevent backend dependency
    await mockAllAPIs(page);
  });

  test('Pricing page - initial load', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageStability(page);

    // Take screenshot with generous diff tolerance for CI stability
    await expect(page).toHaveScreenshot('pricing-page-initial.png', {
      fullPage: true,
      animations: 'disabled',
      maxDiffPixelRatio: 0.20, // 20% tolerance for cross-environment differences
    });
  });

  test('Favorites page - empty state', async ({ page }) => {
    await page.goto('/favorites');
    await waitForPageStability(page);

    await expect(page).toHaveScreenshot('favorites-page-empty.png', {
      fullPage: true,
      animations: 'disabled',
      maxDiffPixelRatio: 0.20,
    });
  });

  test('Campaigns page', async ({ page }) => {
    await page.goto('/campaigns');
    await waitForPageStability(page);

    await expect(page).toHaveScreenshot('campaigns-page.png', {
      fullPage: true,
      animations: 'disabled',
      maxDiffPixelRatio: 0.15,
    });
  });
});

test.describe('Visual Regression - Components @visual', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    // Mock all API endpoints
    await mockAllAPIs(page);
  });

  test('Header navigation', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageStability(page);

    // Screenshot of header
    const header = page.locator('header').first();
    if (await header.isVisible()) {
      await expect(header).toHaveScreenshot('header-navigation.png', {
        animations: 'disabled',
        maxDiffPixelRatio: 0.15,
      });
    }
  });

  test('Search form', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageStability(page);

    // Screenshot of search area
    const searchInput = page.locator('input[type="text"]').first();
    if (await searchInput.isVisible()) {
      // Take a screenshot of the parent container
      const container = searchInput.locator('..');
      await expect(container).toHaveScreenshot('search-form.png', {
        animations: 'disabled',
        maxDiffPixelRatio: 0.10,
      });
    }
  });
});

test.describe('Visual Regression - States @visual', () => {
  // Skip state visual tests in CI - element dimensions vary across environments
  // due to font rendering and CSS computation differences.
  // Run locally with: npx playwright test visual-regression --update-snapshots
  test.skip(!process.env.RUN_VISUAL_REGRESSION, 'Visual tests skipped unless RUN_VISUAL_REGRESSION is set');

  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
  });

  test('Empty favorites state', async ({ page }) => {
    await mockAllAPIs(page);

    await page.goto('/favorites');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(500);

    // Look for empty state message
    const emptyState = page.locator('text=No cards on your watchlist yet').first();
    const isVisible = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);

    if (isVisible) {
      await expect(emptyState).toHaveScreenshot('empty-favorites-state.png', {
        animations: 'disabled',
        maxDiffPixelRatio: 0.10,
      });
    }
  });
});

test.describe('Visual Regression - Responsive @visual', () => {
  // Skip responsive visual tests in CI - they require locally maintained baselines
  // due to deviceScaleFactor and font rendering differences across environments.
  // Run locally with: npx playwright test visual-regression --update-snapshots
  test.skip(!process.env.RUN_VISUAL_REGRESSION, 'Visual tests skipped unless RUN_VISUAL_REGRESSION is set');

  test.beforeEach(async ({ page }) => {
    // Mock all API endpoints
    await mockAllAPIs(page);
  });

  test('Tablet - Pricing page', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 }); // iPad
    await page.goto('/pricing');
    await waitForPageStability(page, 5000);

    await expect(page).toHaveScreenshot('tablet-pricing-page.png', {
      fullPage: true,
      animations: 'disabled',
      maxDiffPixelRatio: 0.20,
    });
  });

  test('Mobile - Pricing page', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 }); // iPhone SE
    await page.goto('/pricing');
    await waitForPageStability(page, 5000);

    await expect(page).toHaveScreenshot('mobile-pricing-page.png', {
      fullPage: true,
      animations: 'disabled',
      maxDiffPixelRatio: 0.20,
    });
  });
});

test.describe('Visual Regression - Dark Mode @visual', () => {
  // Skip dark mode visual tests in CI - full-page heights vary across environments
  // due to font rendering and CSS computation differences.
  // Run locally with: npx playwright test visual-regression --update-snapshots
  test.skip(!process.env.RUN_VISUAL_REGRESSION, 'Visual tests skipped unless RUN_VISUAL_REGRESSION is set');

  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    // Enable dark mode via media emulation
    await page.emulateMedia({ colorScheme: 'dark' });
    // Mock all API endpoints
    await mockAllAPIs(page);
  });

  test('Dark mode - Pricing page', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageStability(page);

    await expect(page).toHaveScreenshot('dark-pricing-page.png', {
      fullPage: true,
      animations: 'disabled',
      maxDiffPixelRatio: 0.20,
    });
  });
});

test.describe('Visual Regression - Hover States @visual', () => {
  // Skip hover state visual tests in CI - element dimensions vary across environments
  // due to font rendering and CSS computation differences.
  // Run locally with: npx playwright test visual-regression --update-snapshots
  test.skip(!process.env.RUN_VISUAL_REGRESSION, 'Visual tests skipped unless RUN_VISUAL_REGRESSION is set');

  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockAllAPIs(page);
  });

  test('Nav link hover state', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageStability(page);

    // Find a nav link that's not active
    const navLink = page.locator('nav a').nth(1);
    if (await navLink.isVisible()) {
      await navLink.hover();
      await page.waitForTimeout(300);

      await expect(navLink).toHaveScreenshot('nav-link-hover-state.png', {
        animations: 'disabled',
        maxDiffPixelRatio: 0.25,
      });
    }
  });

  test('Button hover state', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageStability(page);

    // Find first visible button
    const button = page.locator('button:visible').first();
    if (await button.isVisible()) {
      // Hover over the button
      await button.hover();
      await page.waitForTimeout(200);

      await expect(button).toHaveScreenshot('button-hover-state.png', {
        animations: 'disabled',
        maxDiffPixelRatio: 0.15,
      });
    }
  });
});
