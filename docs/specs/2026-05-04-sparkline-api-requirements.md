# Sparkline UI — API requirements

**Date:** 2026-05-04
**Status:** ⏸ Frontend deferred, blocked on backend
**Parent plan:** [docs/plans/2026-05-04-slab-terminal-redesign.md](../plans/2026-05-04-slab-terminal-redesign.md)

---

## Why this exists

The Slab Terminal redesign (Phases 1–3, PRs #357–#365) shipped every plan item *except* three sparkline placements. All three need a daily-snapshot time series the backend doesn't currently expose. Each was attempted during its parent PR, then deferred with an in-PR comment pointing here.

Rather than ship dummy-data sparklines (which would lie precisely when the operator most needs the trend to be honest — same trap as the dashboard `FreshnessLine` fix on PR #358), we left the placements out and documented what unblocks them.

This spec is structured so the backend work can be picked up independently in any order. Each section names:

1. The UI placement waiting on the data
2. The minimum-shape API the frontend needs
3. A non-blocking pre-existing alternative the backend could expose instead
4. The frontend follow-up PR(s) that land once the data is available

---

## 1. Weekly Review sparkline (Dashboard)

### What it would render

A small inline SVG sparkline (~80 × 20px) embedded in the **Weekly Review** section header on the Dashboard, beside the existing `weekStart - weekEnd · in progress · day N of 7` summary. Shows daily profit (or daily revenue if profit is unavailable) across the **current** week, so the operator can see at a glance whether the week is trending the way the headline numbers suggest.

### Why deferred

`useWeeklyReview()` returns a `WeeklyReviewSummary` with totals (`profitThisWeekCents`, `revenueThisWeekCents`, `purchasesThisWeek`, …) and counterparts for last week — but no daily breakdown. With only 5 metrics × 2 weeks, there's nothing to plot as a sparkline.

Originally on the plan in Phase 2.1; deferred in PR #358 with a one-line note in the commit body: "Skipped — Weekly Review sparkline needs daily P&L series the API doesn't return."

### API the frontend needs

Add a `dailyProfitCents: number[]` (and ideally `dailyRevenueCents: number[]`) field to `WeeklyReviewSummary`. The array should be 7 entries long (Sun–Sat or Mon–Sun, locked to the same week-start convention the existing `weekStart` field uses), zero-filled for days with no activity. Days that haven't happened yet in the in-progress week stay zero — the frontend already handles that visually.

```ts
// web/src/types/campaigns/portfolio.ts (or wherever WeeklyReviewSummary lives)
export interface WeeklyReviewSummary {
  // … existing fields
  dailyProfitCents: number[];   // length 7, zero-padded for no-activity days
  dailyRevenueCents: number[];  // optional, same shape; if omitted UI hides the alt-metric toggle
}
```

### Non-blocking alternative

If a daily breakdown is too much surgery for the existing weekly aggregator, a separate `GET /api/portfolio/daily-profit?weekStart=YYYY-MM-DD` endpoint returning just the 7-element array would unblock the same UI. The frontend would do a second fetch and stitch it.

### Frontend follow-up

Single small PR after the API ships:

- New `<WeeklyReviewSparkline data={data.dailyProfitCents} />` component, hand-rolled SVG (consistent with the cost-vs-market diverging bar in PR #359).
- Lives inside `web/src/react/components/portfolio/WeeklyReviewSection.tsx` next to the existing `CollapsibleHeader`.
- ~30 lines of SVG path math + a tooltip on hover. No chart library needed for one sparkline shape.

---

## 2. Inventory 90-day P&L chart strip

### What it would render

A slim 60–80px chart strip above the inventory table on the **Inventory** page (and the same component on campaign-detail's Transactions tab). Two stacked lines:

1. Total **unrealized P&L** over the last 90 days (the headline number trended)
2. **Cost vs market** as a divergent area, so the operator can see both the absolute trajectory and where market sits relative to cost

This is the move the Phase 2.2 plan called "[s]lim 60-80px chart strip above the table — unrealized P&L over the last 90 days + cost-vs-market diverging bar." The cost-vs-market piece shipped (it's a single point-in-time bar, no historical data needed). The 90-day trend is what's still waiting.

### Why deferred

`useGlobalInventory()` returns the current inventory snapshot only. There's no portfolio-snapshot history endpoint. PR #359's commit body documents the deferral: "90-day unrealized P&L sparkline above the table — needs a snapshot series the API doesn't currently return."

### API the frontend needs

A new endpoint, since this is a portfolio-wide series rather than a single-record extension:

```
GET /api/portfolio/snapshots?days=90
→ {
    snapshots: [
      { date: "2026-02-04", costBasisCents: 7459593, marketCents: 9396163, unrealizedPLCents: 1936570 },
      …
    ]
  }
```

Or, since the existing `internal/domain/portfolio/snapshot.go` already constructs `WeeklyHistory []WeeklyReviewSummary` (used by `/api/portfolio/weekly-history`), an alternative is to extend that endpoint with `dailyHistory` covering the last 90 days. Either shape works for the frontend.

### Non-blocking alternative

Backend could compute the series on-demand from existing `campaign_purchases` + `campaign_sales` tables by replaying transactions day-by-day. No new storage table required if the compute cost is acceptable for an interactive endpoint (caching the result for 1 hour would absorb most repeat reads).

### Frontend follow-up

Single PR after the API ships:

- New `<InventoryChartStrip />` component above `InventoryHeader`'s "+$XX,XXX unrealized" hero.
- Two-line SVG (unrealized P&L line + a thinner cost-basis baseline). Hand-rolled or `recharts` if multiple chart shapes start landing in the same PR (per the original plan's "decide on chart library when we need >2 shapes" note).
- Reuses the same time-series component on `campaign-detail/InventoryTab.tsx` after the Transactions tab gains a sparkline placement (out of scope today, but the component should be designed to compose).

---

## 3. Per-campaign 7-day sparkline (Campaigns table)

### What it would render

A tiny ~50 × 14px sparkline cell in each row of the **Campaigns** list table, between the existing "Sell-through" mini-bar and the "Cap · Buy%" column. Shows daily P&L for that campaign over the last 7 days — the operator scans down the column and immediately sees which campaigns are trending up vs. down without clicking into each.

### Why deferred

`useCampaigns()` returns the campaign list; `useCampaignPNL(id)` returns a single point-in-time P&L. There's no daily-by-campaign series. The Phase 3.2 commit body in PR #363 documents this: "Per-row 7-day P&L sparkline — same blocker as Weekly Review + Inventory chart strip: no daily series per campaign in the API."

### API the frontend needs

Either:

(a) **Extend `CampaignPNL`** with a `dailyProfitCents: number[]` (length 7) field — easy if the backend can compute it cheaply per campaign. Used by `useCampaignPNL` (already pulled per-campaign in `CampaignsPage.tsx` for every visible row, so no new fetch overhead).

(b) **New batch endpoint** `GET /api/campaigns/daily-pnl?days=7` returning `{ [campaignId]: number[] }`. Avoids N×7 row-by-row queries if the per-campaign extension is expensive.

```ts
// Option (a)
export interface CampaignPNL {
  // … existing fields
  dailyProfitCents: number[];  // length 7, zero-padded; oldest first
}

// Option (b)
GET /api/campaigns/daily-pnl?days=7
→ { [campaignId: string]: number[] }
```

### Non-blocking alternative

If neither fits cleanly, the frontend can derive a coarser approximation from the existing `useSales(campaignId)` query by bucketing by day on the client. The drawback is N round-trips for N campaigns and missing day-zero-fill on no-activity days — workable for a dashboard with 10 campaigns, ugly at 50+. Document this as the v0 fallback if backend cost is prohibitive.

### Frontend follow-up

Single PR after the API ships:

- New `<CampaignSparkline data={pnl.dailyProfitCents} />` row cell in `web/src/react/pages/campaigns/CampaignsTab.tsx` between the sell-through bar and the Cap · Buy% column.
- Visibility-tiered like the Opportunities columns (PR #364): always visible at lg+, hidden below md to keep narrow viewports clean.
- ~25 lines of SVG. Reuses the same hand-rolled approach as the Inventory chart strip if shipped in the same era.

---

## Cross-cutting decisions

### Chart library

The original plan said "hand-rolled SVG until we need >2 chart shapes." The Slab Terminal redesign shipped exactly one chart shape (Inventory cost-vs-market diverging bar). When all three sparklines above land, the count is 4 distinct shapes:

1. Diverging bar (already shipped)
2. Single-line sparkline (Weekly Review)
3. Two-line strip with area fill (Inventory 90d)
4. Single-line sparkline (Campaigns 7d) — shape 2 reused

The fork point: if shapes 2 + 4 use the same component, the count is 3 distinct shapes — still hand-rollable. If shapes 2 and 4 diverge (e.g. Campaigns sparkline picks up bar-style instead), introduce `recharts` at PR-3 of the sparkline trio.

Defer the library decision until the first sparkline lands. Don't pre-commit — the rest of the typography and theming work hard enough already that adding a 90KB+ chart library before it's clearly needed adds bundle weight without offsetting design value.

### Data freshness discipline

Whichever shape the API lands in, the frontend follow-up must hold the convention ratified across the redesign: **don't fabricate freshness signals.** A sparkline with no data should render an em-dash and a one-line "no data yet" caption, not a flat-zero line that reads as "everything was zero" when the truth is "we don't know."

The dashboard `FreshnessLine` (PR #358 CodeRabbit fix) and the admin `PulseLine` (PR #365) are the existing precedents — return `null` from the component when the input is missing rather than rendering a misleading default.

### Backend conventions to mirror

The existing `weekly-history` endpoint at `internal/adapters/httpserver/handlers/campaigns_finance.go` returning `[]WeeklyReviewSummary` is the closest existing pattern. New daily endpoints should follow the same shape:

- Date in `YYYY-MM-DD` (not ms-epoch — frontend can `Date.parse` either, but ISO matches the rest of the API)
- Money in cents (consistent with the `*Cents` suffix elsewhere)
- Zero-padded for no-activity days (the frontend treats absent vs. zero differently — absent means "no data," zero means "we measured and there was nothing")

---

## Tracking

When this work picks up, the natural rollout is:

1. Ship one of the three API additions (whichever is cheapest)
2. Open the corresponding frontend PR
3. Repeat for the other two

Each frontend PR is small (≤100 lines). The work is bounded — no design exploration needed since all three placements are already drawn into the parent plan and validated against the Slab Terminal aesthetic.

If the backend constraint persists indefinitely, the frontend pages don't suffer materially — the redesign still ships its primary value (information-design hierarchy, typography, color discipline). The sparklines are nice-to-have ambient signals, not load-bearing UX.
