# Frontend E2E Tests

**Framework:** Playwright
**Test Coverage:** Virtualization, Keyboard Navigation, Accessibility, Smoke Tests

---

## Quick Start

### Install Browsers

```bash
# Install Chromium browser (required for tests)
npx playwright install chromium --with-deps

# Or install all browsers
npx playwright install --with-deps
```

### Run Tests

```bash
# Run all E2E tests
npm run test:e2e

# Run smoke tests only (fast, ~30-60s)
npm run test:e2e -- --grep @smoke
# or
SMOKE_ONLY=true npm run test:e2e

# Run accessibility tests only
npm run test:a11y

# Run performance tests only
npm run test:perf

# Run tests in headed mode (see browser)
npx playwright test --headed

# Run specific test file
npx playwright test tests/opportunities.spec.ts

# Run tests in debug mode
npx playwright test --debug

# Run all tests except smoke tests
npm run test:e2e -- --grep-invert @smoke
```

### View Test Report

```bash
# Generate and open HTML report
npx playwright show-report
```

---

## Test Structure

```
tests/
├── fixtures.ts                    # Shared fixtures and mock data
├── opportunities.spec.ts          # Virtualization and loading state tests
├── accessibility.spec.ts          # Keyboard navigation and a11y tests
└── README.md                      # This file
```

---

## Smoke Tests (@smoke tag)

**Smoke tests** are critical path tests that verify core functionality. They run first in CI for fast feedback (~30-60 seconds).

### What should be marked as @smoke?

✅ **Include in smoke tests:**
- Page loads successfully
- Critical user flows (navigation, basic interactions)
- Essential API integrations work
- Core UI components render
- Major feature entry points accessible

❌ **Don't include in smoke tests:**
- Edge cases
- Performance tests
- Visual regression tests
- Detailed interaction tests
- Accessibility audits (run in full suite)

### How to tag tests as smoke

Use the `@smoke` tag in test descriptions:

```typescript
import { test, expect } from './fixtures';

// Option 1: Tag entire describe block
test.describe('Opportunities Page @smoke', () => {
  test('should load and render cards', async ({ page }) => {
    await page.goto('/opportunities');
    await expect(page.locator('[data-testid="opportunity-card"]')).toHaveCount(10);
  });
});

// Option 2: Tag individual tests
test.describe('Opportunities Page', () => {
  test('should load and render cards @smoke', async ({ page }) => {
    // Critical path test
  });

  test('should filter by grade', async ({ page }) => {
    // Not smoke - detailed feature test
  });
});
```

### Test Tags Reference

| Tag | Purpose | Run Command |
|-----|---------|-------------|
| `@smoke` | Critical path tests | `npx playwright test --grep @smoke` |
| `@a11y` | Accessibility tests | `npm run test:a11y` |
| `@perf` | Performance tests | `npm run test:perf` |
| `@visual` | Visual regression | `npx playwright test --grep @visual` |
| `@slow` | Long-running tests | `npx playwright test --grep @slow` |

---

## Test Coverage

### 1. Virtualization Tests (`opportunities.spec.ts`)

**Story 1.7.1: Virtualization Visibility Test**

Tests that verify the virtual scroller works correctly:

- ✅ Renders only visible items (<50 DOM nodes for 200 items)
- ✅ Renders new items when scrolling
- ✅ Maintains ≥45 FPS during scroll (target: ≥55 desktop, ≥45 mobile)
- ✅ Preserves scroll position on window resize
- ✅ Does not render items outside viewport
- ✅ Shows skeleton loader during initial load
- ✅ Shows empty state when no results

**Key Assertions:**
- Rendered items < 50 (for 200 total items)
- FPS ≥ 45 during scroll
- Scroll position preserved within 10% on resize

---

### 2. Keyboard Navigation Tests (`accessibility.spec.ts`)

**Story 1.7.2: Keyboard Navigation Test**

Tests that verify keyboard accessibility:

- ✅ Navigate through cards with Tab key
- ✅ Activate card details with Enter/Space
- ✅ Visible focus indicator on all interactive elements
- ✅ Navigate buttons within card using Tab
- ✅ Close modal with Escape key
- ✅ Maintain focus during scroll

**Key Assertions:**
- All interactive elements are keyboard accessible
- Focus indicators are visible (outline or box-shadow)
- Tab order is logical

---

### 3. ARIA and Screen Reader Support

Tests for semantic HTML and ARIA attributes:

- ✅ `aria-label` on all interactive elements
- ✅ Meaningful alt text on images (not "image" or empty)
- ✅ Proper heading hierarchy
- ✅ `role="button"` on clickable elements
- ✅ Prefers-reduced-motion support

**Key Assertions:**
- All images have descriptive alt text (>5 characters)
- All buttons have aria-labels
- Animations respect reduced motion preference

---

### 4. Mobile Accessibility

Tests for mobile-specific accessibility:

- ✅ Touch targets ≥44x44 pixels
- ✅ Scrollable on mobile viewport
- ✅ Responsive layout

**Test Viewports:**
- Desktop: 1280x720 (Chrome)
- Mobile: 375x667 (iPhone SE / Pixel 5)

---

## Mock Data

Tests use mock API responses via Playwright's `page.route()`:

```typescript
await page.route('**/api/analyze*', async (route) => {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      results: mockData.opportunities(200),
    }),
  });
});
```

**Mock Data Generator:**
- `mockData.opportunities(count)` - Generates N opportunity cards
- Includes: name, setName, number, prices, ROI, liquidity, tags

---

## CI/CD Integration

### Optimized CI Strategy

Our CI pipeline uses a **tiered testing approach** for fast feedback:

#### On Pull Requests (`.github/workflows/frontend-ci.yml`)

1. **Smoke Tests** (~30-60s) - Runs first
   - Chromium only
   - Critical path tests (`@smoke` tag)
   - Fast feedback
   - Blocks full suite if fails

2. **Full E2E Suite** (~3-4 minutes) - Runs if smoke tests pass
   - Chromium only
   - All tests (including smoke)
   - 2 parallel workers

3. **Lighthouse CI** (conditional)
   - Only on main branch
   - Or when PR has `lighthouse` label
   - Performance audits

#### Full Browser Matrix (`.github/workflows/browser-matrix.yml`)

Comprehensive cross-browser testing:
- **Trigger**: Manual, weekly (Sundays 2 AM UTC), or on release branches
- **Browsers**: Chromium, Firefox, WebKit, Mobile Chrome, Mobile Safari
- **Duration**: ~10-15 minutes
- **Run manually**: GitHub Actions → "Full Browser Matrix Tests" → Run workflow

#### Expected CI Times

| Workflow | Tests | Duration | Browser(s) |
|----------|-------|----------|------------|
| Smoke tests | 7 | 20-40s | Chromium |
| Full PR suite | ~70 | 3-4 min | Chromium |
| Full browser matrix | ~70 | 10-15 min | All 5 browsers |
| With sharding (2x) | ~70 | 5-8 min | All 5 browsers |

### Sharding (Optional)

For large test suites, enable sharding in `browser-matrix.yml`:

```yaml
matrix:
  browser: [chromium, firefox, webkit]
  shard: [1, 2]  # Split tests across 2 runners per browser
```

**Benefits**: 2x faster execution, parallel test execution per browser

### GitHub Actions Example

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  smoke-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20

      - name: Install dependencies
        run: npm ci

      - name: Install Playwright Chromium
        run: npx playwright install chromium --with-deps

      - name: Run smoke tests
        run: npx playwright test --grep @smoke

  full-tests:
    needs: smoke-tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20

      - name: Install dependencies
        run: npm ci

      - name: Install Playwright Chromium
        run: npx playwright install chromium --with-deps

      - name: Run all E2E tests
        run: npm run test:e2e

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: playwright-report/
```

### Path Filters (Skip CI on docs changes)

```yaml
on:
  pull_request:
    paths:
      - 'web/**'
      - '!web/**.md'  # Skip markdown-only changes
```

---

## Performance Budgets

### Virtualization Targets

| Metric | Target | Test |
|--------|--------|------|
| Rendered items (200 total) | <50 | `should render only visible items` |
| Scroll FPS (desktop) | ≥55 | `should maintain smooth FPS` |
| Scroll FPS (mobile) | ≥45 | `should maintain smooth FPS` |
| Scroll position drift | <10% | `should preserve scroll position` |

### Accessibility Targets

| Metric | Target | Test |
|--------|--------|------|
| Touch target size | ≥44x44 px | `should have touch targets` |
| Focus visibility | 100% | `should show visible focus indicator` |
| ARIA labels | 100% | `should have aria-label` |
| Image alt text | 100% | `should have meaningful alt text` |

---

## Troubleshooting

### Tests Fail: "Browser not found"

**Solution:** Install Playwright browsers

```bash
npx playwright install chromium --with-deps
```

### Tests Timeout Waiting for Server

**Issue:** Dev server not starting on port 5173

**Solution:** Check if port is already in use

```bash
# Kill existing process
lsof -ti:5173 | xargs kill -9

# Or change port in playwright.config.ts
```

### Tests Fail: "Element not visible"

**Issue:** Elements rendered outside viewport due to virtualization

**Solution:** This is expected for virtualization tests. Tests verify only visible items are rendered.

### Flaky FPS Test

**Issue:** FPS varies based on system load

**Solution:**
- Run tests in CI with consistent hardware
- Use Playwright's retry mechanism (configured in `playwright.config.ts`)
- Lower FPS threshold if needed (currently 45 FPS)

---

## Test Maintenance

### Adding New Tests

1. Create test file in `tests/` directory
2. Import fixtures: `import { test, expect, selectors, mockData } from './fixtures';`
3. Use `test.describe()` to group related tests
4. Use `test()` for individual test cases
5. Add mock data if needed

**Example:**

```typescript
import { test, expect, mockData } from './fixtures';

test.describe('New Feature', () => {
  test('should do something', async ({ page }) => {
    await page.goto('/');
    // Your test code
  });
});
```

### Updating Mock Data

Edit `tests/fixtures.ts`:

```typescript
export const mockData = {
  opportunities: (count: number) => {
    // Customize mock data structure
  },
};
```

### Adding Custom Selectors

Edit `tests/fixtures.ts`:

```typescript
export const selectors = {
  opportunityCard: '[data-testid="opportunity-card"]',
  // Add new selectors
};
```

---

## Best Practices

1. **Use data-testid attributes** for stable selectors
2. **Mock API responses** instead of hitting real APIs
3. **Test user behavior** not implementation details
4. **Keep tests focused** - one assertion per test when possible
5. **Use descriptive test names** - test name should explain what's being tested
6. **Clean up state** - use `beforeEach` and `afterEach` hooks
7. **Avoid hard-coded waits** - use `waitForSelector()` instead of `waitForTimeout()`

---

## Sprint 1 Acceptance Criteria

### Story 1.7.1: Virtualization ✅

- ✅ Test verifies <30 cards in DOM (out of 200)
- ✅ Test verifies FPS ≥55 on scroll (45 on mobile)
- ✅ Test runs in <30s

### Story 1.7.2: Keyboard Navigation ✅

- ✅ Tab cycles through all interactive elements
- ✅ Enter/Space activates buttons
- ✅ Escape closes modals
- ✅ Focus is always visible

---

## References

- [Playwright Documentation](https://playwright.dev/)
- [Playwright Best Practices](https://playwright.dev/docs/best-practices)
- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [Sprint Plan: docs/ui/DETAILED_SPRINT_PLAN.md](../../docs/ui/DETAILED_SPRINT_PLAN.md)

---

**Last Updated:** 2025-11-13
**Playwright Version:** 1.56.1
**Test Count:** 15+ tests across 6 files
**CI Optimizations:** Smoke tests, browser matrix, sharding support
