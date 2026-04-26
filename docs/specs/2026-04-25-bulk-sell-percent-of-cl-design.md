# Bulk Sell at % of CL — Design

**Date:** 2026-04-25
**Branch:** `feature/bulk-sell`
**Worktree:** `.worktrees/bulk-sell`

## Problem

User takes a stack of on-hand cards to a card show and sells them at a uniform percentage of CardLadder (CL) value (e.g., "everything at 70% of CL"). Today, the bulk Record Sale modal requires entering an absolute dollar amount per row — for 30 cards, that's 30 hand-typed prices.

The selection UI, the bulk endpoint, the per-channel fee logic, and the cross-campaign global inventory page all already exist. The missing piece is a pricing mode in the bulk modal that takes one percentage and applies it across all selected cards.

## Domain framing

Sales are not campaign-scoped from the user's perspective. Campaigns frame purchases (capital deployment, sourcing strategy); selling at a show is a global event across the user's whole on-hand stack. The primary surface for this feature is `/inventory` (`GlobalInventoryPage`), not a per-campaign Inventory tab. The per-campaign tab inherits the new modal because it mounts the same `InventoryTab` component.

## Scope

### In scope

- New `BulkRecordSaleModal` component for ≥ 2 selected on-hand cards.
- Pricing mode toggle: `% of CL` (default) and `Flat $`.
- "Fill all" input with live gross total.
- Collapsed per-row review with manual override + reset.
- Pre-flight warning in the bulk action bar when selection includes cards with no CL value.

### Out of scope (deferred)

- Remembering last-used `% of CL` value across sessions.
- A `% of cost` pricing mode (liquidation use case).
- Server-side `pctOfCL` field with stored audit trail.
- An aggregate "sell all to one buyer" / lot-sale entity.
- Mobile-specific UX changes beyond what the existing responsive layout provides.

## Architecture

### File layout

```
web/src/react/pages/campaign-detail/
  RecordSaleModal.tsx                    # SLIMMED: single-sale only
  BulkRecordSaleModal.tsx                # NEW
  saleModal/                             # NEW
    pricingModes.ts                      # types + computeSalePrice
  inventory/
    InventoryHeader.tsx                  # adds CL-missing pre-flight banner
    useInventoryState.ts                 # routes single → RecordSaleModal, bulk → BulkRecordSaleModal
```

### Component boundaries

- **`RecordSaleModal`** — single-card flow only. Drops the `items.length > 1` branch. Keeps the optional listing-detail fields (Original List Price, Price Reductions, Days Listed, Sold at Asking Price). Estimated ~150 lines.
- **`BulkRecordSaleModal`** — bulk-only flow. Owns pricing-mode state, fill-all control, collapsed per-row review, group-by-campaign submit. Estimated ~250 lines.
- **`saleModal/pricingModes.ts`** — pure functions, no React. Exports `PricingMode = 'pctOfCL' | 'flat'` and `computeSalePrice(item, mode, value): number`. Easy to unit test.
- **`useInventoryState.ts`** — already drives modal open/close based on selection size. Adds a branch: `selected.size === 1` → mount `RecordSaleModal`; else mount `BulkRecordSaleModal`.

A shared `Dialog.Root`/`Dialog.Portal`/`Dialog.Content` wrapper was considered and rejected — duplicating ~30 lines across two modals is preferable to a premature shell abstraction. Each modal owns its own dialog chrome.

### Backend

No changes. `POST /api/campaigns/{id}/sales/bulk` and `inventory.Service.CreateBulkSales` already accept `[{purchaseId, salePriceCents}]` per campaign group. The percentage math is performed client-side; the wire format is unchanged.

## UX details

### Bulk modal layout

```
┌─ Record Sale (32 cards) ──────────────────────── ✕ ─┐
│                                                     │
│  Channel: [In person ▾]   Sale Date: [2026-04-25]   │
│                                                     │
│  Pricing:  ●  % of CL     ○  Flat $                 │
│                                                     │
│  ┌────────────────────────────────────────────┐     │
│  │ [ 70 ] %   →   $1,247 total at this %      │     │
│  └────────────────────────────────────────────┘     │
│                                                     │
│  ▾ Review prices (32)                               │
│                                                     │
│  [Cancel]                    [Record 32 Sales]      │
└─────────────────────────────────────────────────────┘
```

### Behaviors

- **Pricing mode toggle.** `% of CL` is the default. Switching to `Flat $` swaps the input label and re-computes every row to the same dollar amount. The numeric value of the input is preserved across mode switches; the unit changes.
- **Fill-all input.** Live recompute as the user types — no Apply button. Live gross total reflects `Σ row prices` post-rounding.
- **Channel default.** Inherits the global `DEFAULT_SALE_CHANNEL = 'ebay'`. User selects `In person` from the dropdown when recording show sales. Not changing this default for the bulk modal.
- **Sale date default.** `localToday()`, as today.
- **Review prices disclosure.** Collapsed by default. Expanded view shows the per-row list that the bulk modal renders today (card name, grade, cost, CL value, campaign name, editable price input).
- **Per-row override.** Edits to a per-row input persist when fill-all `%` changes. An inline `↩ reset to 70% of CL` link appears next to overridden rows; clicking it reverts that row to the computed value.
- **Submit button.** `Record N Sales`. Disabled if any row has price ≤ 0 or if total is $0.
- **Rounding.** `Math.round(clValueCents × pct / 100)` to nearest cent. Matches the backend's existing `math.Round` convention for fees.
- **Live total.** Shows gross (sum of sale prices), not net of fees. Fees vary by channel and would re-compute as the user changes the channel; gross is stable and easier to reason about.

### Pre-flight CL-missing warning

Lives in the bulk action bar (`InventoryHeader.tsx`), not inside the modal. Surfaces *before* the user opens the modal, while they still have list context (filter, sort, search).

```
┌────────────────────────────────────────────────────────────┐
│ 32 selected                                                │
│ ⚠ 3 of 32 cards have no CL value — [Highlight] [Deselect]  │
│ [Add to Sell Sheet]  [List on DH]  [Record Sale]           │
└────────────────────────────────────────────────────────────┘
```

- The warning row only appears when `selectedItems.some(i => !i.purchase.clValueCents)`.
- **`Highlight`** filters the inventory list to show only the no-CL cards in the current selection. User can eyeball them and decide which to deselect or sync.
- **`Deselect`** removes only the no-CL cards from the selection in one click. Most common path for the show use case.
- **The "Record Sale" button stays enabled.** The user is allowed to proceed with no-CL cards in the selection. If they do, the modal opens normally; in `% of CL` mode, no-CL rows compute to `$0.00`, and the modal's existing "N card(s) have no sale price set" guard prevents submission. The user can then expand the review list and type prices manually, switch to `Flat $` mode, or close and deselect.

## Data flow

1. User on `/inventory` — `useGlobalInventory()` returns `AgingItem[]` with `purchase.clValueCents`, `purchase.campaignId`, `currentMarket`. No new query.
2. User checks 32 boxes — selection state in `useInventoryState.ts` (`Set<string>` of purchase IDs).
3. Action bar renders — if selection contains any no-CL row, render the pre-flight warning row.
4. User clicks `Record Sale` — `useInventoryState` opens the modal; `selected.size > 1` → mount `BulkRecordSaleModal`.
5. In the modal — user keeps mode `% of CL`, types `70`, picks channel `In person`. Live total updates.
6. Submit — modal computes `salePriceCents` per row via `computeSalePrice(item, 'pctOfCL', 70)`. Groups items by `campaignId`. Calls `api.createBulkSales(campaignId, channel, saleDate, [{purchaseId, salePriceCents}])` once per campaign group via `Promise.allSettled` (existing logic).
7. Server — `inventory.Service.CreateBulkSales` loops, calls `CreateSale` per item, computes `SaleFeeCents` (channel `inperson` = 0%), `DaysToSell`, `NetProfitCents`, captures market snapshot. Returns `BulkSaleResult{Created, Failed, Errors}` per group.
8. Client aggregates — sums `created` / `failed` across groups; success or partial-failure toast (existing logic).
9. Cache invalidation — uses the existing list verbatim: per-campaign `sales`/`purchases`/`pnl`/`inventory`/`channelPnl`/`daysToSell` plus global `inventory`/`sellSheet`/`health`/`weeklyReview`/`channelVelocity`/`insights`/`suggestions`.
10. Modal closes; inventory list refreshes; sold rows disappear from on-hand.

## Error handling

| Failure mode | Behavior | Source |
|---|---|---|
| Row computes to `salePriceCents = 0` | Existing modal toast: "N card(s) have no sale price set" — submit blocked | Inherited |
| User passes pre-flight with no-CL cards | Those rows compute to `$0`, hit the above guard | Inherited |
| Network failure for one campaign group | `Promise.allSettled` per-group; error toast; other groups commit | Inherited |
| Backend rejects an item inside a group | `BulkSaleResult.Errors[]`; first 3 errors toasted; counts adjust | Inherited |
| All groups fail | Toast "All N sale(s) failed"; modal stays open with state preserved | Inherited |
| User edits row, then changes fill-all `%` | Manual override sticks; `↩ reset to 70% of CL` link offered | New |
| Invalid date | Existing toast: "Please select a sale date" | Inherited |
| Float→cents rounding | `Math.round`, deterministic; matches backend fee rounding | New |

No new error paths land on the backend.

## Testing

### Frontend — new

- **`pricingModes.test.ts`** — table-driven tests for `computeSalePrice`. Cases: pure `% of CL`, `Flat $`, missing CL value (returns 0), zero %, 100%, fractional cents (e.g., 4700 × 70 / 100 = 3290), negative input clamping.
- **`BulkRecordSaleModal.test.tsx`** — render + interaction:
  - Switching modes preserves the input value, re-labels, re-computes.
  - Fill-all input updates live total across all rows.
  - Per-row override survives a fill-all change; reset link reverts to computed.
  - Submit blocked when any row is `$0`.
  - Submit groups by `campaignId` and calls `api.createBulkSales` once per group.
  - Partial-failure toast aggregation matches existing behavior.
- **`InventoryHeader.test.tsx`** — pre-flight warning:
  - Banner appears iff selection contains a no-CL card.
  - `Deselect` drops only the no-CL cards.
  - `Highlight` triggers the filter/scroll behavior (assert callback fired).

### Frontend — existing tests touched

- `RecordSaleModal.test.tsx` (if present) — narrow scope to single-sale only; bulk-related cases move to `BulkRecordSaleModal.test.tsx`.

### Backend

No changes. Existing tests in `internal/domain/inventory/service_test.go` (`TestService_CreateSale_*`) and `internal/adapters/httpserver/handlers/campaigns_purchases_test.go` (`TestHandleCreateSale_POST_*`) cover the bulk endpoint.

### Manual verification

- `make screenshots` — verify no regression on `/inventory` and `/campaigns/:id`.
- `cd web && npm run dev` — exercise show flow with 5+ cards across 2+ campaigns, mix with/without CL value, both pricing modes.

## Open follow-ups

- Remembering last-used `% of CL` (localStorage one-liner) — defer until the user asks.
- `% of cost` pricing mode for liquidation runs — defer; distinct use case.
- Server-side `pctOfCL` audit trail — defer; can be implied later as `salePrice / clValue`.
