# Tools Reorganization Design

Date: 2026-04-05

## Context

The DH enterprise API integration has replaced much of the original workflow (inventory push, listing, order tracking, price sync). The Tools section needs reorganization to reflect this shift — separating daily-use tools from transitional/legacy ones, improving DH matching UX, and cleaning up the dashboard.

## Changes Overview

1. **Tools page**: Reorganize from 5 tabs to 3 (Daily Ops, Card Intake, Legacy)
2. **Insights page**: Add PSA Invoices section
3. **Admin > DH**: Redesign stats to show health metrics + DH counts
4. **Dashboard**: Remove Active Campaigns grid

---

## 1. Tools Page — Tab Restructure

### Current State

5 tabs: Import/Export, Card Intake, eBay Export, Import Sales, Invoices

### New State

3 tabs: **Daily Ops**, **Card Intake**, **Legacy**

### Daily Ops Tab

Replaces the current "Import / Export" tab. Contains the daily workflow items:

**Import/Export Cards (3-column grid):**
- **PSA Import** — unchanged, upload PSA CSV and create invoices
- **Export for Card Ladder** — unchanged, download inventory CSV (with "Only missing CL data" checkbox)
- **Import from Card Ladder** — unchanged, upload matched CL CSV

External Import and Price Sync are removed from this grid (moved to Legacy).

**DH Unmatched Items (new section, below the grid):**
- Moved from Admin > Integrations > DH tab
- Card with DH badge icon, "Unmatched DH Items" heading, unmatched count badge
- Improved table styling: uppercase column headers, alternating row shading, monospace cert numbers, cleaner input/fix button layout
- Columns: Cert, Card Name, Set, Grade, Value, DH Match (input + Fix button)
- Only shown when unmatched count > 0

**Import results** continue to appear inline below the card grid, as they do today.

### Card Intake Tab

**Unchanged.** Stays as its own tab — it's a full interactive scanning workflow used several times a week, not daily. Needs the full real estate.

### Legacy Tab

New tab containing transitional tools that will be removed after full DH migration. All displayed as compact cards in a 2-column grid:

- **External Import** — condensed to a card with upload button. Tagged "Transitional"
- **Price Sync** — condensed to a card. Tagged "Transitional"
- **eBay Export** — condensed from full tab to a card. Clicking "Load Items" expands the workflow inline within the card. Tagged "Transitional"
- **Import Sales** — condensed from full tab to a card. Clicking "Upload Orders CSV" expands the workflow inline within the card. Tagged "Transitional"

Each card shows: icon, title, one-line description, action button, "Transitional" tag.

---

## 2. Insights Page — PSA Invoices Section

### Placement

New section added after the existing Credit Health panel, before Portfolio Insights.

### Content

- Section title: "PSA Invoices"
- Subtitle: "Payment tracking for PSA submissions"
- Full invoice table with columns: Date, Total, Paid, Status, Due, Action
- Status badges: paid (green), partial (amber), unpaid (red)
- "Mark Paid" button for unpaid invoices
- No summary header (paid/outstanding totals) — paid/unpaid status badges are sufficient

### Component

Move `InvoicesTab` content to a new `InvoicesSection` component. Reuse the existing `useInvoices()` and `useUpdateInvoice()` hooks.

---

## 3. Admin > DH — Stats Redesign

### Current State

5 summary cards: Market Intelligence, Suggestions, Mapped Cards, Pending Push, Unmatched Cards. Plus Bulk Match trigger and unmatched fix table.

### New State

Three sections within the DH admin panel:

**Integration Health (3-card row):**
- **API Health** — success/failure rate over rolling 7 days, total calls and failure count
- **Match Rate** — percentage of cards successfully matched, matched/total counts
- **Unmatched** — count of unmatched cards, percentage of total inventory

**DoubleHolo Counts (3-card row):**
- **Inventory** — items in DH inventory (from DH API)
- **Listings** — active DH listings (from DH API)
- **Orders** — total DH orders (from DH API)

**Bulk Match (stays):**
- "Run Bulk Match" button with running state indicator
- Description text unchanged

### Unmatched Fix Table

The unmatched fix table (with cert/card/set/grade/value/fix columns) is **removed from Admin**. It now lives on the Tools > Daily Ops tab. Admin retains the unmatched count stat and the bulk match trigger.

### Backend Considerations

- **API Health tracking**: Need to instrument the DH HTTP client to record success/failure counts. Could use a simple rolling counter in the scheduler or a lightweight DB table.
- **DH counts**: Need a new endpoint or extend the existing DH status endpoint to fetch inventory/listings/orders counts from the DH API.
- The current Market Intelligence and Suggestions stats are dropped unless they still provide value — confirm with user.

---

## 4. Dashboard — Remove Active Campaigns

### Change

Remove the Active Campaigns grid that currently renders between the Hero Stats Bar and the Weekly Review section.

### Rationale

- Takes up significant vertical space, pushing insights below the fold
- The same information is available on the Campaigns page
- Removing it surfaces Weekly Review, AI Intelligence, and Watchlist without scrolling

### Implementation

Delete the `activeCampaigns` useMemo, the campaign health lookup, and the grid rendering block from `DashboardPage.tsx`. No replacement needed.

---

## File Impact Summary

### Frontend Files Modified

| File | Change |
|------|--------|
| `web/src/react/pages/ToolsPage.tsx` | Restructure tabs: Daily Ops, Card Intake, Legacy |
| `web/src/react/pages/campaigns/OperationsTab.tsx` | Remove External Import card; rename to DailyOpsTab or keep as OperationsTab |
| `web/src/react/pages/tools/EbayExportTab.tsx` | Adapt to render as compact card with inline expansion |
| `web/src/react/pages/tools/ImportSalesTab.tsx` | Adapt to render as compact card with inline expansion |
| `web/src/react/pages/tools/InvoicesTab.tsx` | Move to new `InvoicesSection` component for Insights |
| `web/src/react/pages/InsightsPage.tsx` | Add InvoicesSection after Credit Health |
| `web/src/react/pages/admin/DHTab.tsx` | Redesign stats, remove unmatched fix table |
| `web/src/react/pages/DashboardPage.tsx` | Remove Active Campaigns grid |
| New: `web/src/react/pages/tools/LegacyTab.tsx` | New tab component housing legacy cards |
| New: `web/src/react/pages/tools/DHUnmatchedSection.tsx` | DH unmatched table with improved styling (moved from DHTab) |
| New: `web/src/react/components/insights/InvoicesSection.tsx` | Invoices section for Insights page |

### Backend Files (potentially modified)

| File | Change |
|------|--------|
| DH HTTP client adapter | Add success/failure tracking for API health stats |
| DH status endpoint handler | Extend to return API health metrics + DH inventory/listings/orders counts |
| `types/apiStatus` (frontend) | Update DHStatus type to include new fields |

---

## Open Questions

1. **Market Intelligence & Suggestions stats** — are these dropped entirely, or do they still provide value in Admin?
2. **API health tracking granularity** — rolling 7 days? Per-endpoint or aggregate? Simple counter in memory or persisted?
3. **eBay Export / Import Sales inline expansion** — should these open a modal instead of expanding inline? Inline keeps context but may feel cramped.
