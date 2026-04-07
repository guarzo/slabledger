# UI Friction Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the frontend around the actual daily workflow — reduce nav from 7→4, fix inventory table density/actions, redesign campaign list, bridge import-to-review gap, simplify login page.

**Architecture:** Frontend-only rearrangement except for one small backend change (surfacing `openFlagId` on aging items). Existing components are moved between pages, not rewritten. Inventory table columns are reduced and a visible Sell button added.

**Tech Stack:** React 19, TypeScript, Tailwind CSS, Radix UI, TanStack React Query, React Router, Go backend (one minor change)

**Spec:** `docs/superpowers/specs/2026-04-07-ui-friction-redesign.md`

---

### Task 1: Backend — Surface openFlagId on AgingItem

The resolve-flag API needs a flag ID, but the aging item only has `hasOpenFlag` (boolean). Change `OpenFlagPurchaseIDs` to return `map[string]int64` (purchaseID → flagID) and add `OpenFlagID` to AgingItem.

**Files:**
- Modify: `internal/domain/campaigns/repository.go:125` — change `OpenFlagPurchaseIDs` return type to `map[string]int64`
- Modify: `internal/domain/campaigns/analytics_types.go:54` — add `OpenFlagID int64` field after `HasOpenFlag`
- Modify: `internal/domain/campaigns/service_analytics.go:340-354` — update `applyOpenFlags` to set both `HasOpenFlag` and `OpenFlagID`
- Modify: `internal/adapters/storage/sqlite/price_flags_repository.go:163-179` — change query to `SELECT purchase_id, id FROM price_flags WHERE resolved_at IS NULL`, return `map[string]int64`
- Modify: `internal/testutil/mocks/campaign_repository.go:842-844` — update mock return type to `map[string]int64`
- Modify: `internal/domain/campaigns/mock_repo_test.go:661-663` — update mock return type to `map[string]int64`
- Modify: `web/src/types/campaigns/analytics.ts:87` — add `openFlagId?: number` after `hasOpenFlag`

- [ ] **Step 1:** Update the repository interface — change `OpenFlagPurchaseIDs` signature from `(map[string]bool, error)` to `(map[string]int64, error)`
- [ ] **Step 2:** Update `analytics_types.go` — add `OpenFlagID int64 \`json:"openFlagId,omitempty"\`` to `AgingItem` struct after `HasOpenFlag`
- [ ] **Step 3:** Update `price_flags_repository.go` — change the SQL to `SELECT purchase_id, id FROM price_flags WHERE resolved_at IS NULL` and scan into `map[string]int64`
- [ ] **Step 4:** Update `service_analytics.go` `applyOpenFlags` — use the int64 map: `items[i].OpenFlagID = flaggedIDs[items[i].Purchase.ID]` and set `HasOpenFlag = true` when value > 0
- [ ] **Step 5:** Update both mock files to return `map[string]int64{}`
- [ ] **Step 6:** Update `web/src/types/campaigns/analytics.ts` — add `openFlagId?: number`
- [ ] **Step 7:** Run `go test ./internal/domain/campaigns/... ./internal/adapters/storage/sqlite/...` to verify no breakage
- [ ] **Step 8:** Commit: `feat: surface openFlagId on aging items for resolve-flag action`

---

### Task 2: Login Page Simplification

Replace the broken showcase card with the Card Yeti business logo.

**Files:**
- Modify: `web/src/react/pages/LoginPage.tsx` — remove SHOWCASE_CARD, SHOWCASE_PRICES, SHOWCASE_GRADE_DATA constants and CardPriceCard import; replace showcase section with logo
- Modify: `web/src/css/LoginPage.css` — remove `.login-showcase`, `.showcase-card-wrapper`, `.showcase-caption` styles; add `.login-logo` style

- [ ] **Step 1:** Edit `LoginPage.tsx` — remove `SHOWCASE_CARD`, `SHOWCASE_PRICES`, `SHOWCASE_GRADE_DATA` constants and the `CardPriceCard`, `GradeKey`, `GradeData` imports. Replace the `{/* Showcase Card */}` section with:
```tsx
{/* Logo */}
<div className="login-logo">
  <img src={logoSrc} alt="Card Yeti" className="login-logo-img" />
</div>
```
Add import: `import logoSrc from '../../assets/card-yeti-business-logo.png';`
- [ ] **Step 2:** Edit `LoginPage.css` — remove `.login-showcase`, `.showcase-card-wrapper`, `.showcase-caption` rules. Add:
```css
.login-logo {
  position: relative;
  z-index: 1;
  text-align: center;
}
.login-logo-img {
  width: 180px;
  height: auto;
  filter: drop-shadow(0 10px 30px rgba(0, 0, 0, 0.3));
}
```
- [ ] **Step 3:** Verify login page renders with `cd web && npm run build`
- [ ] **Step 4:** Commit: `fix: replace broken showcase card with Card Yeti logo on login page`

---

### Task 3: Navigation Restructure (7 → 4)

Reduce nav items and add legacy redirects.

**Files:**
- Modify: `web/src/react/components/Navigation.tsx` — reduce `navItems` to 4 entries
- Modify: `web/src/react/App.tsx` — remove routes for Watchlist, Opportunities, Insights, Content; add redirects; remove lazy imports

- [ ] **Step 1:** Edit `Navigation.tsx` — replace `navItems` array with:
```ts
const navItems = [
  { path: '/', label: 'Dashboard', shortLabel: 'Home' },
  { path: '/inventory', label: 'Inventory', shortLabel: 'Inventory' },
  { path: '/campaigns', label: 'Campaigns', shortLabel: 'Campaigns' },
  { path: '/tools', label: 'Tools', shortLabel: 'Tools' },
];
```
- [ ] **Step 2:** Edit `App.tsx` — remove lazy imports for `WatchlistPage`, `OpportunitiesPage`, `InsightsPage`, `ContentPage`. Remove the 4 corresponding `<Route>` elements. Add redirects:
```tsx
<Route path="/watchlist" element={<Navigate to="/" replace />} />
<Route path="/opportunities" element={<Navigate to="/" replace />} />
<Route path="/insights" element={<Navigate to="/" replace />} />
<Route path="/content" element={<Navigate to="/tools" replace />} />
```
- [ ] **Step 3:** Verify build: `cd web && npm run build`
- [ ] **Step 4:** Commit: `refactor: reduce navigation from 7 to 4 items with legacy redirects`

---

### Task 4: Dashboard Absorbs Content

Move Watchlist, Opportunities, Capital Exposure/Timeline into Dashboard as collapsible sections.

**Files:**
- Modify: `web/src/react/pages/DashboardPage.tsx` — add new sections with imports from existing components

- [ ] **Step 1:** Add imports to `DashboardPage.tsx`:
```ts
import { useCapitalTimeline, useCapitalSummary } from '../queries/useCampaignQueries';
import CapitalTimelineChart from '../components/portfolio/CapitalTimelineChart';
import CapitalExposurePanel from '../components/portfolio/CapitalExposurePanel';
import PicksList from '../components/picks/PicksList';
import AcquisitionWatchlist from '../components/picks/AcquisitionWatchlist';
```
- [ ] **Step 2:** Add query hooks in the component body:
```ts
const { data: capitalTimeline } = useCapitalTimeline();
const { data: capitalData } = useCapitalSummary();
```
Note: `useCapitalSummary` is already imported — deduplicate.
- [ ] **Step 3:** Add sections to the JSX after HeroStatsBar, before Weekly Review:
  - Capital Exposure + Timeline (wrapped in SectionErrorBoundary, conditional on data)
  - After AI Intelligence: Opportunities section (PicksList + AcquisitionWatchlist in SectionErrorBoundary)
  - Update Watchlist section: remove `maxItems={4}` prop so full list renders
- [ ] **Step 4:** Verify build: `cd web && npm run build`
- [ ] **Step 5:** Commit: `feat: dashboard absorbs watchlist, opportunities, capital exposure sections`

---

### Task 5: Campaigns Page — Compact Rows + Invoices

Restyle campaign list as compact rows with inline P&L and add InvoicesSection.

**Files:**
- Modify: `web/src/react/pages/campaigns/CampaignsTab.tsx` — replace card layout with compact rows
- Delete: `web/src/react/pages/campaigns/PNLBadge.tsx` — data inlined into rows
- Modify: `web/src/react/pages/CampaignsPage.tsx` — add InvoicesSection import and render below campaign list

- [ ] **Step 1:** Rewrite `CampaignsTab.tsx` campaign list — replace the card-style `<Link>` block with compact rows:
  - Left side: 3px accent bar colored by phase (active=green, pending=amber, closed=gray), campaign name, filter summary (sport, grade, price range)
  - Right side: inline P&L from `pnlMap` (net profit + ROI), sell-through (sold/total + pct), buy terms (daily cap + CL%)
  - Closed campaigns at `opacity-50`
  - Chevron icon on the right
  - Remove `PNLBadge` import and usage
- [ ] **Step 2:** Delete `PNLBadge.tsx`
- [ ] **Step 3:** Edit `CampaignsPage.tsx` — add import for `InvoicesSection` from `../components/insights/InvoicesSection` and render it below the `SectionErrorBoundary` for CampaignsTab:
```tsx
<SectionErrorBoundary sectionName="Invoices">
  <InvoicesSection />
</SectionErrorBoundary>
```
- [ ] **Step 4:** Verify build: `cd web && npm run build`
- [ ] **Step 5:** Commit: `feat: campaign list compact rows with inline P&L + invoices section`

---

### Task 6: Inventory — Column Reduction + Sell Button

Reduce desktop columns from 12+ to 7, add visible Sell button, remove Record Sale from dropdown.

**Files:**
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx` — remove CL, Signal, Status, EV, Rec Price column headers; add Sell column header
- Modify: `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` — remove CL, Signal, Status, EV, Rec Price cells; add visible Sell button; remove "Record Sale" from DropdownMenu; add "Flag" to DropdownMenu

- [ ] **Step 1:** Edit `InventoryTab.tsx` — in the glass-table-header, remove the column headers for: CL (`width: 68px`), Signal (`width: 48px`), Rec. (`width: 68px`), Status (`width: 72px`). Remove the conditional EV header. Add a "Sell" header (`width: 48px`) before the actions overflow column.
- [ ] **Step 2:** Edit `DesktopRow.tsx` — remove the JSX for: CL value cell, Signal cell, Rec. Price cell, Status badge cell, conditional EV cell. Add a visible Sell button cell before the overflow menu:
```tsx
<div className="glass-table-td flex-shrink-0 text-center !px-1" style={{ width: '48px' }} onClick={e => e.stopPropagation()}>
  <button
    type="button"
    onClick={onRecordSale}
    className="text-xs font-medium px-2 py-1 rounded bg-[var(--brand-500)]/20 text-[var(--brand-400)] hover:bg-[var(--brand-500)]/40 transition-colors"
  >
    Sell
  </button>
</div>
```
- [ ] **Step 3:** In the DropdownMenu in `DesktopRow.tsx`, remove the "Record Sale" item (it's now a visible button). Add a "Flag" item that calls `onOpenFlagDialog` (pass it as a new prop, or reuse the existing expansion mechanism).
- [ ] **Step 4:** Verify build: `cd web && npm run build`
- [ ] **Step 5:** Commit: `feat: reduce inventory columns and add visible Sell button on desktop`

---

### Task 7: Inventory — Filter Tab Simplification + Resolve Flag

Reduce filter tabs from 7 to 4 and add resolve flag action in expanded detail.

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` — update `FilterTab` type, add `exceptions` count
- Modify: `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts` — change default filterTab to `'exceptions'`, add `handleResolveFlag`
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx` — update filter tab rendering to 4 tabs
- Modify: `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx` — add Resolve Flag button

- [ ] **Step 1:** Edit `inventoryCalcs.ts`:
  - Update `FilterTab` type: `'exceptions' | 'sell_sheet' | 'all' | 'card_show'`
  - Add `exceptions` to `TabCounts`: `exceptions: number` (= large_gap + no_data + flagged)
  - Update `computeInventoryMeta` to compute `counts.exceptions`
  - Update `filterAndSortItems`: add `'exceptions'` case that filters to `status === 'large_gap' || status === 'no_data' || status === 'flagged'`
- [ ] **Step 2:** Edit `useInventoryState.ts`:
  - Change default `filterTab` from `'needs_review'` to `'exceptions'`
  - Add `handleResolveFlag` callback that calls `api.resolvePriceFlag(flagId)` and invalidates inventory queries
  - Return `handleResolveFlag` from the hook
- [ ] **Step 3:** Edit `InventoryTab.tsx` — replace the 7 filter tab buttons with 4:
  - Exceptions (color: `var(--warning)`, count: `tabCounts.exceptions`)
  - Sell Sheet (color: `var(--brand-400)`, count: `pageSellSheetCount`)
  - All (color: `var(--text)`, count: `tabCounts.all`)
  - Card Show (color: `var(--brand-400)`, count: `tabCounts.card_show`)
- [ ] **Step 4:** Edit `ExpandedDetail.tsx` — add a "Resolve Flag" button next to the existing Flag button when `item.hasOpenFlag && item.openFlagId`:
```tsx
{item.hasOpenFlag && item.openFlagId && (
  <Button
    variant="success"
    size="sm"
    onClick={() => onResolveFlag(item.openFlagId)}
    disabled={isSubmitting}
  >
    Resolve Flag
  </Button>
)}
```
Pass `onResolveFlag` prop from InventoryTab through to ExpandedDetail.
- [ ] **Step 5:** Verify build: `cd web && npm run build`
- [ ] **Step 6:** Run `cd web && npm test` to check for test failures
- [ ] **Step 7:** Commit: `feat: simplify inventory filters to 4 tabs + add resolve flag action`

---

### Task 8: Tools — Content Tab + Post-Import Bridge

Add Content as a 4th tab in Tools and add "Review prices →" link after imports.

**Files:**
- Modify: `web/src/react/pages/ToolsPage.tsx` — add Content tab
- Modify: `web/src/react/pages/ContentPage.tsx` — remove outer page wrapper for embedded use
- Modify: `web/src/react/pages/campaigns/OperationsTab.tsx` — add review link in import results

- [ ] **Step 1:** Edit `ToolsPage.tsx`:
  - Add `content` to TABS array: `{ id: 'content', label: 'Content' }`
  - Add lazy import: `const ContentPage = lazy(() => import('./ContentPage'));`
  - Add tab content:
```tsx
<Tabs.Content value="content">
  <SectionErrorBoundary sectionName="Content">
    <Suspense fallback={<PokeballLoader />}>
      <ContentPage embedded />
    </Suspense>
  </SectionErrorBoundary>
</Tabs.Content>
```
- [ ] **Step 2:** Edit `ContentPage.tsx` — add `embedded` prop. When `embedded` is true, skip the outer `max-w-6xl mx-auto px-4` wrapper and the `<h1>` heading (ToolsPage provides those).
- [ ] **Step 3:** Edit `OperationsTab.tsx` — in both import result banners (CL and PSA), add a Link after the stats:
```tsx
import { Link } from 'react-router-dom';
// ... in the result banner:
<Link to="/inventory" className="text-xs font-medium text-[var(--brand-400)] hover:text-[var(--brand-300)] underline ml-2">
  Review prices →
</Link>
```
- [ ] **Step 4:** Verify build: `cd web && npm run build`
- [ ] **Step 5:** Commit: `feat: add Content tab to Tools + post-import review link`

---

### Task 9: Cleanup + Quality Check

Remove dead page files, run full quality checks.

**Files:**
- Delete: `web/src/react/pages/WatchlistPage.tsx`
- Delete: `web/src/react/pages/OpportunitiesPage.tsx`
- Delete: `web/src/react/pages/InsightsPage.tsx`

- [ ] **Step 1:** Delete the 3 page files that are now absorbed elsewhere
- [ ] **Step 2:** Run `cd web && npm run build` — verify no import errors
- [ ] **Step 3:** Run `cd web && npm test` — verify all tests pass
- [ ] **Step 4:** Run `cd web && npx tsc --noEmit` — verify no type errors
- [ ] **Step 5:** Run `go test ./...` from repo root — verify backend tests pass
- [ ] **Step 6:** Run `make check` — verify lint + architecture + file size checks pass
- [ ] **Step 7:** Commit: `chore: remove absorbed page files and verify quality checks`
