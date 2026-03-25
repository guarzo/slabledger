import { test, expect, selectors, setupPageWithMocks } from './fixtures';

/**
 * E2E tests for keyboard navigation and accessibility
 * Tests keyboard nav, ARIA attributes, and mobile accessibility
 * on the actual app pages (dashboard, watchlist, campaigns)
 */

test.describe('Keyboard Navigation', () => {
  test('should be able to focus the Price Lookup button', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    const lookupButton = page.locator(selectors.priceLookupButton);
    await lookupButton.focus();

    const isFocused = await lookupButton.evaluate((el) => {
      return document.activeElement === el;
    });
    expect(isFocused).toBe(true);
  });

  test('should be able to tab through navigation links', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Tab to first interactive element
    await page.keyboard.press('Tab');

    // Should have a visible focus indicator
    const focusedElement = page.locator(':focus');
    await expect(focusedElement).toBeVisible();

    // Continue tabbing through navigation
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');

    // Focus should still be visible
    const newFocusedElement = page.locator(':focus');
    await expect(newFocusedElement).toBeVisible();
  });

  test('should show visible focus indicator', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Tab to first interactive element
    await page.keyboard.press('Tab');

    // Check focus indicator is visible
    const focusedElement = page.locator(':focus');
    const focusStyles = await focusedElement.evaluate((el) => {
      const styles = window.getComputedStyle(el);
      return {
        outline: styles.outline,
        outlineWidth: styles.outlineWidth,
        boxShadow: styles.boxShadow,
      };
    });

    // Should have some kind of focus indicator
    const hasFocusIndicator =
      (focusStyles.outlineWidth && focusStyles.outlineWidth !== '0px') ||
      (focusStyles.boxShadow && focusStyles.boxShadow !== 'none');

    expect(hasFocusIndicator).toBeTruthy();
  });

  test('should have focusable buttons', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Check that buttons or links exist and are focusable
    const interactiveElements = page.locator('button, a[href]');
    const count = await interactiveElements.count();

    // Should have interactive elements
    expect(count).toBeGreaterThan(0);

    // First element should be focusable
    if (count > 0) {
      await interactiveElements.first().focus();
      const isFocused = await interactiveElements.first().evaluate((el) => {
        return document.activeElement === el;
      });
      expect(isFocused).toBe(true);
    }
  });
});

test.describe('Accessibility - ARIA and Screen Reader Support', () => {
  test('should have aria-label on navigation', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    const nav = page.locator(selectors.nav);
    const ariaLabel = await nav.getAttribute('aria-label');

    expect(ariaLabel).toBeTruthy();
    expect(ariaLabel?.length).toBeGreaterThan(5);
  });

  test('should have alt text on logo image', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    const logoImg = page.locator('header img').first();
    if (await logoImg.isVisible()) {
      const alt = await logoImg.getAttribute('alt');
      expect(alt).toBeTruthy();
    }
  });

  test('should have semantic landmark roles', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Header should have banner role
    const header = page.locator('header[role="banner"]');
    await expect(header).toBeVisible();

    // Navigation should have navigation role
    const nav = page.locator('nav[role="navigation"]');
    await expect(nav).toBeVisible();

    // Main content should exist
    const main = page.locator('main#main-content');
    await expect(main).toBeVisible();
  });
});

test.describe('Mobile Accessibility', () => {
  test.use({
    viewport: { width: 375, height: 667 } // iPhone SE
  });

  test('should render correctly on mobile viewport', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Header should still be visible
    const header = page.locator(selectors.header);
    await expect(header).toBeVisible();

    // Price Lookup button should still be visible
    const lookupButton = page.locator(selectors.priceLookupButton);
    await expect(lookupButton).toBeVisible();

    // Navigation should be visible
    const nav = page.locator(selectors.nav);
    await expect(nav).toBeVisible();
  });

  test('should be scrollable on mobile', async ({ page }) => {
    await setupPageWithMocks(page, '/');

    // Scroll the page
    await page.evaluate(() => {
      window.scrollBy(0, 200);
    });
    await page.waitForTimeout(200);

    // Header should be sticky (still visible after scroll)
    const header = page.locator(selectors.header);
    await expect(header).toBeVisible();
  });
});
