# PriceDecisionBar — Shared Price Selection Component

**Date:** 2026-03-31
**Status:** Approved
**Scope:** Extract shared `PriceDecisionBar` component to replace 4 inconsistent price selection UIs

## Problem

Four screens implement the same core workflow — "pick a price from available sources or type a custom one" — but each does it differently:

| Screen | Quick-pick sources | Custom input | Skip | Handles no-data |
|--------|-------------------|-------------|------|-----------------|
| Inventory ReviewActionBar | CL, Market, Last Sold | Yes | No | Yes (custom only) |
| eBay Export | None | Inline edit (separate mode) | Yes | Blocked |
| Shopify Sync | None | No | Yes | Blocked |
| Price Override Dialog | 12% Over Cost | Yes | Cancel | Yes |

Users on eBay Export and Shopify Sync cannot select market or cost as a price source — they must manually type prices. When a card has no CL or market data, those screens completely block the user with no way to proceed.

## Solution

Extract a single `PriceDecisionBar` component that all 4 screens embed inline. It combines the best of each current implementation:
- Quick-pick source buttons (from inventory ReviewActionBar)
- Skip option (from eBay/Shopify)
- Custom dollar input (from all)
- Cost Basis as a source (always available, from purchase data)

## Component API

```typescript
// web/src/react/ui/PriceDecisionBar.tsx

export interface PriceSource {
  label: string;        // "CL", "Market", "Cost", "Last Sold"
  priceCents: number;   // 0 = unavailable (button disabled, shows "—")
  source: string;       // "cl", "market", "cost_basis", "last_sold", "manual"
}

export interface PriceDecisionBarProps {
  /** Available price sources as quick-pick buttons */
  sources: PriceSource[];
  /** Pre-selected source (by source key) */
  preSelected?: string;
  /** Called when user confirms a price */
  onConfirm: (priceCents: number, source: string) => void;
  /** Optional skip action (eBay export, Shopify sync) */
  onSkip?: () => void;
  /** Optional flag action (inventory review) */
  onFlag?: () => void;
  /** Current decision state for visual feedback */
  status?: 'pending' | 'accepted' | 'skipped';
  /** Disable all interactions */
  disabled?: boolean;
  /** Show loading spinner on confirm button */
  isSubmitting?: boolean;
  /** Confirm button label (default: "Confirm") */
  confirmLabel?: string;
}
```

## Quick-Pick Sources

All consumers provide 4 source buttons:

| Button | Source key | Data field |
|--------|-----------|------------|
| CL | `cl` | `clValueCents` |
| Market | `market` | `marketMedianCents` / `marketPriceCents` |
| Cost | `cost_basis` | `costBasisCents` (buyCostCents + psaSourcingFeeCents) |
| Last Sold | `last_sold` | `lastSoldCents` |

If a source has no data, pass `priceCents: 0` — the button renders disabled with "—".

## Pre-Selection Priority

The `preSelected` prop is set by each consumer following this priority:

1. **Reviewed price** — if `reviewedPriceCents > 0`, pre-select the source whose price matches `reviewedPriceCents`. If the reviewed price doesn't match any current source (e.g., it was set manually), populate the custom $ input with the reviewed price and leave source buttons unselected.
2. **CL** — if `clValueCents > 0`
3. **Market** — if `marketMedianCents > 0`
4. **Cost Basis** — always available as final fallback (buyCostCents + psaSourcingFeeCents is always known)

This ensures the "no data" dead end is eliminated: Cost Basis is always known.

## Visual States

### Pending (default)
- Source buttons displayed inline, pre-selected button highlighted with accent border
- Custom $ input synced with selected source's price
- Confirm button enabled when any selection or custom value exists
- Skip button visible (if `onSkip` provided)

### Accepted
- All buttons dimmed (reduced opacity)
- Green checkmark badge shows locked price: "✓ $285.00"
- "Change" button replaces Confirm/Skip to re-open editing

### Skipped
- Entire bar dimmed
- "Skipped" label shown
- "Undo" button to reverse

## Interaction Behaviors

- Clicking a source button selects it AND populates the $ input
- Typing in the $ input clears the source button selection (source becomes "manual")
- Enter key in the $ input triggers confirm
- Pre-selection happens on mount — user can immediately click Confirm to accept

## Bulk Actions

In eBay Export and Shopify Sync, "Accept All" iterates all items and confirms each row's pre-selected source/price. This respects the per-item priority logic — each item gets its best available source.

Items already marked as "skipped" are not affected by Accept All.

## Integration Plan

### eBay Export (`EbayExportTab.tsx`)
- Replace Accept/Edit/Skip buttons + inline number input with `PriceDecisionBar`
- Remove `editingId` / `editPrice` / `confirmEdit` state — custom input lives inside the component
- Decision type simplifies: `{ action: 'accept'; priceCents: number; source: string } | { action: 'skip' }`
- Each row builds sources from item fields

### Shopify Sync (`ShopifySyncPage.tsx`)
- Replace `ReviewRow`'s Update/Skip buttons with `PriceDecisionBar`
- Sources from existing `ShopifyPriceSyncMatch` fields + new `lastSoldCents`
- Users can now pick a different source instead of being locked to the server's recommendation
- Source tracking added to decisions

### Inventory Review (`ExpandedDetail.tsx`)
- Replace `ReviewActionBar` import with `PriceDecisionBar`
- Add Cost Basis to the sources array (currently missing)
- No `onSkip` — passes `onFlag` instead
- No `status` prop (single-item context, not bulk)

### Price Override Dialog (deferred)
- Keep as-is for now. Can adopt `PriceDecisionBar` in a follow-up, keeping AI suggestion buttons separately.

## Backend Changes

### eBay Export — `EbayExportItem` additions
Add to Go struct (`internal/domain/campaigns/ebay_types.go`) and populate in service (`service_ebay_export.go`):

| Field | Type | Source |
|-------|------|--------|
| `CostBasisCents` | int | `BuyCostCents + PSASourcingFeeCents` |
| `LastSoldCents` | int | Market snapshot `LastSoldCents` |
| `ReviewedPriceCents` | int | `Purchase.ReviewedPriceCents` |
| `ReviewedAt` | string (nullable) | `Purchase.ReviewedAt` |

### Shopify Sync — `ShopifyPriceSyncMatch` addition
Add `LastSoldCents` (int) to the Go struct and populate from market snapshot.

### TypeScript types
Update `EbayExportItem` and `ShopifyPriceSyncMatch` in `web/src/types/campaigns/core.ts` to match.

All changes are additive — existing consumers won't break.

## File Changes Summary

| Action | File |
|--------|------|
| **New** | `web/src/react/ui/PriceDecisionBar.tsx` |
| **Edit** | `web/src/react/pages/tools/EbayExportTab.tsx` |
| **Edit** | `web/src/react/pages/ShopifySyncPage.tsx` |
| **Edit** | `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx` |
| **Edit** | `web/src/types/campaigns/core.ts` |
| **Delete** | `web/src/react/pages/campaign-detail/inventory/ReviewActionBar.tsx` |
| **Edit** | `internal/domain/campaigns/ebay_types.go` |
| **Edit** | `internal/domain/campaigns/service_ebay_export.go` |
| **Edit** | `internal/domain/campaigns/service_sell_sheet.go` (Shopify sync service logic) |
| **Edit** | `internal/domain/campaigns/analytics_types.go` (`ShopifyPriceSyncMatch` struct) |

## Out of Scope

- `PriceOverrideDialog.tsx` — follow-up work
- `PriceSignalCard.tsx` — stays as-is, used in `ExpandedDetail` for the 6-signal grid
- Bulk "Set All to..." dropdown — not needed, Accept All uses per-row pre-selection
- Price history/charts in the decision bar — keep it simple
