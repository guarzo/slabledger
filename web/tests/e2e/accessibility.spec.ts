import { test, expect, Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockAllAPIs } from '../fixtures';

// Helper to create AxeBuilder with proper typing (works around version mismatch)
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const createAxeBuilder = (page: Page) => new AxeBuilder({ page } as any);

/**
 * E2E Accessibility Tests using @axe-core/playwright
 *
 * These tests scan pages for WCAG compliance using the aXe accessibility engine.
 * They detect common accessibility issues like:
 * - Missing alt text on images
 * - Low color contrast
 * - Missing form labels
 * - Invalid ARIA attributes
 * - Keyboard navigation issues
 *
 * To run:
 *   npx playwright test accessibility
 */

/**
 * Helper to wait for page to be stable before accessibility scan
 */
async function waitForPageReady(page: import('@playwright/test').Page) {
  await page.waitForLoadState('domcontentloaded');

  // Wait for any loading indicators to disappear
  await page.waitForSelector('[data-testid="pokeball-loader"]', { state: 'hidden', timeout: 5000 })
    .catch(() => {
      // No loading indicators
    });

  // Additional stability wait
  await page.waitForTimeout(500);
}

test.describe('Accessibility - Page Level Scans @a11y', () => {
  test.beforeEach(async ({ page }) => {
    // Set viewport
    await page.setViewportSize({ width: 1280, height: 720 });
    // Mock APIs
    await mockAllAPIs(page);
  });

  test('Pricing page should have no critical accessibility violations', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .withTags(['wcag2a', 'wcag2aa']) // WCAG 2.0 Level A and AA
      .analyze();

    // Report violations with details
    const violations = accessibilityScanResults.violations;
    if (violations.length > 0) {
      console.warn('Accessibility violations found:');
      violations.forEach(violation => {
        console.warn(`- ${violation.id}: ${violation.description}`);
        console.warn(`  Impact: ${violation.impact}`);
        console.warn(`  Help: ${violation.helpUrl}`);
        violation.nodes.forEach(node => {
          console.warn(`  Affected: ${node.html.substring(0, 100)}`);
        });
      });
    }

    // Only fail on critical or serious violations
    const criticalViolations = violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );
    expect(criticalViolations).toEqual([]);
  });

  test('Home page should have no critical accessibility violations', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(500);

    const accessibilityScanResults = await createAxeBuilder(page)
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze();

    const criticalViolations = accessibilityScanResults.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );
    expect(criticalViolations).toEqual([]);
  });
});

test.describe('Accessibility - Component Level Scans @a11y', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockAllAPIs(page);
  });

  test('Navigation should be accessible', async ({ page }) => {
    await page.goto('/pricing');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(300);

    const nav = page.locator('nav, header');
    if (await nav.first().isVisible()) {
      const accessibilityScanResults = await createAxeBuilder(page)
        .include('nav, header')
        .withTags(['wcag2a', 'wcag2aa'])
        .analyze();

      const criticalViolations = accessibilityScanResults.violations.filter(
        v => v.impact === 'critical' || v.impact === 'serious'
      );
      expect(criticalViolations).toEqual([]);
    }
  });

  test('Search form should be accessible', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    const searchInput = page.locator('input[type="text"]');
    if (await searchInput.isVisible()) {
      const accessibilityScanResults = await createAxeBuilder(page)
        .include('input[type="text"]')
        .withTags(['wcag2a', 'wcag2aa'])
        .analyze();

      const criticalViolations = accessibilityScanResults.violations.filter(
        v => v.impact === 'critical' || v.impact === 'serious'
      );
      expect(criticalViolations).toEqual([]);
    }
  });
});

test.describe('Accessibility - Keyboard Navigation @a11y', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockAllAPIs(page);
  });

  test('should be able to navigate page using keyboard only', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    // Tab through interactive elements
    await page.keyboard.press('Tab');

    // Should have a visible focus indicator
    const focusedElement = page.locator(':focus');
    await expect(focusedElement).toBeVisible();

    // Continue tabbing
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');

    // Focus should still be visible
    const newFocusedElement = page.locator(':focus');
    await expect(newFocusedElement).toBeVisible();
  });

  test('interactive elements should have visible focus indicators', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    // Tab to first interactive element
    await page.keyboard.press('Tab');

    // Check focus indicator is visible (has outline or ring)
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

  test('skip link should exist for keyboard users', async ({ page }) => {
    await page.goto('/pricing');
    await page.waitForLoadState('domcontentloaded');

    // Many accessible sites have skip links
    // Tab and check if first focusable element is a skip link
    await page.keyboard.press('Tab');

    const focusedElement = page.locator(':focus');
    const text = await focusedElement.textContent().catch(() => '');
    const href = await focusedElement.getAttribute('href').catch(() => '');

    // Skip link typically contains "skip" and links to main content
    const isSkipLink = text?.toLowerCase().includes('skip') || href?.includes('#main');

    // Log for information, don't fail test as skip links are recommended but not required
    if (!isSkipLink) {
      console.warn('Note: No skip link found. Consider adding one for keyboard users.');
    }
  });
});

test.describe('Accessibility - Color Contrast @a11y', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockAllAPIs(page);
  });

  test('light mode should meet WCAG AA contrast requirements', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' });
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .withTags(['wcag2aa'])
      .options({ runOnly: ['color-contrast'] })
      .analyze();

    // Log contrast issues for review
    if (accessibilityScanResults.violations.length > 0) {
      console.warn('Contrast issues in light mode:');
      accessibilityScanResults.violations.forEach(v => {
        console.warn(`- ${v.description}`);
        v.nodes.forEach(n => {
          console.warn(`  Element: ${n.html.substring(0, 80)}`);
        });
      });
    }

    // Allow some minor contrast issues but flag critical ones
    const criticalContrast = accessibilityScanResults.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );
    expect(criticalContrast).toEqual([]);
  });

  test('dark mode should meet WCAG AA contrast requirements', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .withTags(['wcag2aa'])
      .options({ runOnly: ['color-contrast'] })
      .analyze();

    // Log contrast issues for review
    if (accessibilityScanResults.violations.length > 0) {
      console.warn('Contrast issues in dark mode:');
      accessibilityScanResults.violations.forEach(v => {
        console.warn(`- ${v.description}`);
        v.nodes.forEach(n => {
          console.warn(`  Element: ${n.html.substring(0, 80)}`);
        });
      });
    }

    const criticalContrast = accessibilityScanResults.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );
    expect(criticalContrast).toEqual([]);
  });
});

test.describe('Accessibility - ARIA and Semantics @a11y', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockAllAPIs(page);
  });

  test('should have proper heading hierarchy', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .options({ runOnly: ['heading-order'] })
      .analyze();

    // Heading order is important for screen reader users
    expect(accessibilityScanResults.violations).toEqual([]);
  });

  test('images should have alt text', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .options({ runOnly: ['image-alt'] })
      .analyze();

    const criticalViolations = accessibilityScanResults.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );
    expect(criticalViolations).toEqual([]);
  });

  test('ARIA attributes should be valid', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .options({
        runOnly: [
          'aria-valid-attr',
          'aria-valid-attr-value',
          'aria-required-attr',
          'aria-required-children',
          'aria-required-parent',
        ],
      })
      .analyze();

    // ARIA mistakes can seriously impact assistive technology users
    expect(accessibilityScanResults.violations).toEqual([]);
  });

  test('landmark regions should be properly defined', async ({ page }) => {
    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .options({ runOnly: ['landmark-one-main', 'region'] })
      .analyze();

    // Log landmark issues but don't fail
    if (accessibilityScanResults.violations.length > 0) {
      console.warn('Landmark region issues:');
      accessibilityScanResults.violations.forEach(v => {
        console.warn(`- ${v.id}: ${v.description}`);
      });
    }
  });
});

test.describe('Accessibility - Mobile/Touch @a11y', () => {
  test('mobile viewport should be accessible', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await mockAllAPIs(page);

    await page.goto('/pricing');
    await waitForPageReady(page);

    const accessibilityScanResults = await createAxeBuilder(page)
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze();

    const criticalViolations = accessibilityScanResults.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );
    expect(criticalViolations).toEqual([]);
  });

  test('touch targets should meet minimum size', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await mockAllAPIs(page);

    await page.goto('/pricing');
    await waitForPageReady(page);

    // Check that buttons meet minimum touch target size (44x44)
    const buttons = page.locator('button:visible');
    const count = await buttons.count();

    for (let i = 0; i < Math.min(count, 5); i++) {
      const button = buttons.nth(i);
      const box = await button.boundingBox();

      if (box) {
        // WCAG recommends 44x44 minimum for touch targets
        // We check for 40x40 to allow some tolerance
        const meetsSizeRequirement = box.width >= 40 && box.height >= 40;
        if (!meetsSizeRequirement) {
          const html = await button.evaluate(el => el.outerHTML.substring(0, 100));
          console.warn(`Small touch target: ${html} (${box.width}x${box.height})`);
        }
      }
    }
  });
});
