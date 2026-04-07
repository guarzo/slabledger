# UI Friction Redesign

Date: 2026-04-07

## Problem

The app is structured around campaigns as the organizing unit, but the daily workflow centers on global inventory. The result is excessive context-switching (Tools → Inventory → Campaigns), a crowded 7-item navigation with thin pages that don't justify their own routes, and an inventory table that's dense and missing key actions.

## Goals

1. Reduce navigation from 7 to 4 top-level routes
2. Fix inventory table: fewer columns, visible sell button, unflag action, simplified filters
3. Redesign campaign list as compact rows with inline P&L
4. Bridge the import-to-review workflow gap
5. Simplify login page to use the Card Yeti logo

## Design

### 1. Navigation Restructure (7 → 4 items)

**New navigation:** Dashboard, Inventory, Campaigns, Tools (+ Admin for admin users)

| Current Page | New Location | Rationale |
|---|---|---|
| Dashboard | Dashboard (stays) | Absorbs Watchlist, Opportunities, Capital Exposure |
| Watchlist | → Dashboard section | Already shows 4 items on Dashboard; full list becomes a collapsible section |
| Opportunities | → Dashboard section | Picks + Acquisition Watchlist are small, reference-style content |
| Insights | → Dashboard + Campaigns | Capital exposure/timeline → Dashboard. Weekly Review stays on Dashboard (removed from Insights). Invoices → Campaigns. |
| Content | → Tools tab | Social content generation is a tool, not a daily destination |
| Inventory | Inventory (stays) | Primary workspace — gets the fixes |
| Campaigns | Campaigns (stays) | Gets compact row styling + inline P&L |

**Legacy redirects:** `/watchlist` → `/`, `/opportunities` → `/`, `/insights` → `/`, `/content` → `/tools`

**Files to modify:**
- `web/src/react/components/Navigation.tsx` — reduce `navItems` to 4
- `web/src/react/App.tsx` — remove routes for Watchlist, Opportunities, Insights, Content; add redirects
- Remove lazy imports for WatchlistPage, OpportunitiesPage, InsightsPage, ContentPage

### 2. Dashboard Absorbs Content

The Dashboard page (`DashboardPage.tsx`) gains new sections. All sections are collapsible via `SectionErrorBoundary` wrappers.

**Layout order:**
1. Hero Stats Bar (unchanged)
2. Capital Exposure + Timeline (moved from InsightsPage — `CapitalExposurePanel` + `CapitalTimelineChart`)
3. Weekly Review (unchanged, but now only lives here — removed from Insights)
4. AI Weekly Intelligence (unchanged)
5. Opportunities (moved from OpportunitiesPage — `PicksList` + `AcquisitionWatchlist`)
6. Watchlist (moved from WatchlistPage — full `WatchlistSection`, not capped at 4)

**Files to modify:**
- `web/src/react/pages/DashboardPage.tsx` — add sections from Insights, Opportunities, Watchlist

**Files to delete (or keep as redirect-only):**
- `web/src/react/pages/WatchlistPage.tsx`
- `web/src/react/pages/OpportunitiesPage.tsx`
- `web/src/react/pages/InsightsPage.tsx`

### 3. Inventory Table Fixes

#### 3a. Column Reduction (12+ → 7 + actions)

**Kept on the row:** Checkbox, Card Name (+ set + cert), Grade, Cost, Market, P/L, Days, Sell button

**Moved to expanded detail:** CL Value, Signal, Recommended Price, Status badge, EV

The expanded detail (`ExpandedDetail.tsx`) already shows most of this data. The row becomes scannable at a glance.

**Files to modify:**
- `web/src/react/pages/campaign-detail/InventoryTab.tsx` — remove columns from header and row rendering
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` — remove CL, Signal, Status, EV, Rec Price columns; add visible Sell button
- `web/src/react/pages/campaign-detail/inventory/SortableHeader.tsx` — no changes needed, just fewer headers rendered

#### 3b. Visible Sell Button on Desktop Rows

Replace the ⋮ overflow menu as the primary action with a compact "Sell" button. The ⋮ menu remains for secondary actions (Set Price, Fix Pricing, Flag).

**Current desktop row:** ⋮ menu (28px) contains Record Sale, Set Price, Fix Pricing
**Proposed desktop row:** "Sell" button (visible, ~48px) + ⋮ menu (28px) for Set Price, Fix Pricing, Flag

**Files to modify:**
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` — add Sell button column, remove Record Sale from dropdown

#### 3c. Resolve Flag Action

When a card has an open flag (`item.hasOpenFlag`), the expanded detail (`ExpandedDetail.tsx`) shows a "Resolve Flag" button next to the existing Flag button in the `PriceDecisionBar`.

This calls the same `resolvePriceFlag` API already used by `PriceFlagsTab.tsx` (admin). The API endpoint is `POST /admin/price-flags/:id/resolve`.

**Backend change needed:** The aging item currently has `hasOpenFlag` (boolean) but no flag ID. The resolve API (`POST /admin/price-flags/:id/resolve`) needs a flag ID. Two options:

- **Option A (preferred):** Change `OpenFlagPurchaseIDs` to return `map[string]int64` (purchaseID → flagID) and surface `openFlagId` on the `AgingItem` struct. Minimal change — one query adjustment, one new JSON field.
- **Option B:** Add a new endpoint `POST /api/purchases/:id/resolve-flag` that resolves the open flag by purchase ID. More REST-ful but adds a new route.

Go with Option A — it's simpler and keeps the existing resolve API.

**Files to modify:**
- `internal/adapters/storage/sqlite/price_flags_repository.go` — change `OpenFlagPurchaseIDs` to return `map[string]int64`
- `internal/domain/campaigns/repository.go` — update interface signature
- `internal/domain/campaigns/analytics_types.go` — add `OpenFlagID int64` field to `AgingItem`
- `internal/domain/campaigns/service_analytics.go` — set `OpenFlagID` in `applyOpenFlags`
- `web/src/types/campaigns/analytics.ts` — add `openFlagId?: number`
- `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx` — add Resolve Flag button when `item.hasOpenFlag` is true, using `item.openFlagId`
- `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts` — add `handleResolveFlag` handler

#### 3d. Filter Tab Simplification (7 → 4)

**Current tabs:** Needs Review, Large Gap, No Data, Flagged, Card Show, All, Sell Sheet
**Proposed tabs:** Exceptions, Sell Sheet, All, Card Show

"Exceptions" combines Large Gap + No Data + Flagged into a single tab — these are the items that need attention. The old "Needs Review" tab (unreviewed cards with no specific problem) is removed as a separate filter; those cards appear in "All" and are surfaced by the urgency sort when relevant. The urgency sort (`reviewUrgencySort`) already orders items by severity within any tab. The specific exception type (large gap, no data, flagged) is visible in the expanded detail row.

**Files to modify:**
- `web/src/react/pages/campaign-detail/InventoryTab.tsx` — update filter tab rendering
- `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` — add `exceptions` count (= large_gap + no_data + flagged), update `FilterTab` type
- `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts` — change default `filterTab` from `'needs_review'` to `'exceptions'`

### 4. Campaign List Compact Rows

Replace the chunky card-with-PNL-grid layout with compact rows (~52px each vs ~120px).

**Design:**
- Phase indicated by a left accent bar (green = active, amber = pending, gray = closed)
- Name + filters on the left
- P&L (net profit + ROI), sell-through (sold/total + pct), and buy terms (daily cap + CL%) inline on the right
- Closed campaigns rendered at reduced opacity
- Chevron indicator for navigation

**Files to modify:**
- `web/src/react/pages/campaigns/CampaignsTab.tsx` — replace card layout with compact row layout, inline PNL data directly instead of using PNLBadge
- `web/src/react/pages/campaigns/PNLBadge.tsx` — can be deleted (data inlined into row)

### 5. Campaigns Page Absorbs Invoices

`InvoicesSection` (from `InsightsPage`) moves to the campaigns page. Invoices are campaign-related spend tracking, not portfolio-level analytics.

**Files to modify:**
- `web/src/react/pages/CampaignsPage.tsx` — add `InvoicesSection` below the campaign list

### 6. Tools — Post-Import Bridge + Content Tab

#### 6a. After-Import Prompt

After a successful import (PSA or CL), the import result banner includes a link: "Review prices →" that navigates to `/inventory`. This bridges the context switch from import to review.

**Files to modify:**
- `web/src/react/pages/campaigns/OperationsTab.tsx` — add a `Link` to `/inventory` in the import result display

#### 6b. Content Tab

Tools page gets a 4th tab. The existing `ContentPage` component is rendered as a tab content panel. The Content page component is reused unchanged.

**Files to modify:**
- `web/src/react/pages/ToolsPage.tsx` — add Content tab, lazy-import ContentPage
- `web/src/react/pages/ContentPage.tsx` — may need to remove the outer page wrapper (heading + max-w container) since ToolsPage provides its own

### 7. Login Page Simplification

Replace the showcase card (broken external CDN image) with the Card Yeti logo as the hero. Remove the `CardPriceCard` component and all showcase data constants.

**Design:**
- Card Yeti business logo (`card-yeti-business-logo.png`) as the centered hero image
- "SlabLedger" heading + "Graded Card Portfolio Tracker" subtitle
- Google sign-in button
- Feature pills (Campaign tracking, P&L analytics, Price lookup)

**Files to modify:**
- `web/src/react/pages/LoginPage.tsx` — remove `SHOWCASE_CARD`, `SHOWCASE_PRICES`, `SHOWCASE_GRADE_DATA`, `CardPriceCard` import; replace with logo import and simpler layout

## What's NOT Changing

- Campaign detail page (Overview, Transactions, Tuning, Settings tabs)
- Admin page
- Backend APIs — no changes needed
- Mobile inventory cards (already have visible Sell button)
- RecordSaleModal, PriceOverrideDialog, PriceHintDialog
- All query hooks and data fetching logic
- Authentication flow (just the login page visual)

## Scope Estimate

This is primarily a frontend rearrangement. Most changes involve:
- Moving existing components between pages
- Removing columns/tabs from inventory
- Adding a button (Sell) and an action (Resolve Flag) to existing components
- Restyling campaign list rows
- Simplifying the login page

One minor backend change: surfacing `openFlagId` on the aging item (query adjustment + new JSON field). No new API endpoints. No new data fetching hooks (all data is already available in existing queries).
