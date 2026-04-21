# Inventory Filter Workflow Redesign

**Date:** 2026-04-21
**Branch:** `inventory-filter-workflow`
**Status:** Design pending user review

## Problem

The inventory screen's current filter pills group items by coarse lifecycle stages that don't match how an operator actually triages a campaign:

- A cert that isn't listed during intake is physically received and immediately falls into **In Hand**, the same bucket as fully-listed items. It stays there regardless of whether it still needs a DH match, a price, or a final listing click.
- There is no surface that says "show me everything waiting on a DH match" or "show me everything ready to price."
- When an operator does find one of these stuck items, the actions that would unstick it — `Fix DH Match`, `Set price → List` — are buried in the row's overflow menu or split across two separate clicks. The Cert Intake screen proves a better flow is possible: one row, one action, done.

The ask: reshape the filters so each "stuck" state has its own tab, and surface the completing action on the row itself so the operator can finish without leaving the screen.

## Scope

In scope:
- Replace the secondary filter row with status-specific buckets.
- Add a backend `Dismiss` transition that works from any pre-listed state, and a `Restore` transition to reverse it.
- Make each row's primary action match what it actually needs next (match, price+list, list, or restore), matching the Cert Intake pattern.
- Combine set-reviewed-price and list-on-DH into one confirm in the expanded price bar (as Cert Intake does today).

Out of scope:
- Bulk actions in the new buckets (defer).
- Mobile-specific rework beyond keeping `MobileCard` aligned with `DesktopRow` affordance changes.
- Any change to `Needs Attention` aggregation rules.
- Any change to the Sell Sheet flow.

## Design

### Filter taxonomy

Primary pills (unchanged):
- `Needs Attention` (always visible)
- `Pending DH Listing` (visible when count > 0)
- `Sell Sheet` (visible when the active sheet has items)

Secondary pills:
- `All` (always visible)
- `DH Listed` — currently listed on DH marketplace
- `Pending DH Match` — received, no DH inventory link yet, not dismissed
- `Pending Price` — received, matched to DH, no committed price, not listed, not dismissed
- `Skipped on DH Listing` — operator dismissed the item from the DH pipeline
- `Awaiting Intake` — not yet received

`In Hand` is removed. Bookmarks/links using the old `in_hand` tab key alias to `all`.

### Partition

Every item lands in exactly one secondary bucket. Predicate order (evaluate top-down; first match wins):

| Bucket               | Predicate                                                                                           |
|----------------------|-----------------------------------------------------------------------------------------------------|
| Awaiting Intake      | `!receivedAt`                                                                                       |
| Skipped              | `dhPushStatus === 'dismissed'`                                                                      |
| DH Listed            | `dhStatus === 'listed'`                                                                             |
| Pending DH Match     | `!dhInventoryId`                                                                                    |
| Pending Price        | `dhInventoryId && !hasCommittedPrice`                                                               |
| Pending DH Listing   | `dhInventoryId && hasCommittedPrice && dhStatus !== 'listed'` (existing `isReadyToList`; re-list flows here) |

`Needs Attention` stays cross-cutting (overlaps the other buckets).

### Row affordance model

Rows compute an `actionIntent` from the item's own status — filter tab does not influence the row's appearance:

| Intent         | When                                           | Primary action                                                           |
|----------------|------------------------------------------------|--------------------------------------------------------------------------|
| `fix_match`    | `!dhInventoryId` and not skipped               | Prominent "Fix DH Match" pill (promoted out of the overflow menu)        |
| `set_and_list` | matched but no committed price                 | Expand row → `PriceDecisionBar` with confirm label "List on DH"          |
| `list`         | `isReadyToList`                                | Existing green "List" button                                             |
| `restore`      | `isSkipped`                                    | "Restore" pill → `api.undismissDHMatch`                                  |
| `none`         | otherwise (listed, sold, etc.)                 | Existing DH status badge                                                 |

`Dismiss` is a secondary affordance:
- Available whenever `actionIntent ∈ {fix_match, set_and_list, list}` (i.e., any pre-listed state).
- Surfaces in two places: the `⋯` overflow menu, plus a small tertiary link next to the primary action. Confirm via `window.confirm` for first cut.
- Clicking it calls `api.dismissDHMatch` — backend moves the item to `DHPushStatusDismissed`, which re-buckets it into `Skipped`.

### Combined set-price-and-list

`ExpandedDetail`'s `PriceDecisionBar` today calls a "set reviewed price" confirm only; the subsequent "List" requires a second click in the row's action cell. When `isPendingPrice(item)` is true, rewire the `onConfirm(priceCents, source)` path to call `api.setReviewedPrice` and then `api.listPurchaseOnDH` in sequence, with the same error-shape handling `CardIntakeTab.handleSetPriceAndList` uses today:

- Set-price failure → surface error, no list attempt.
- List failure with `status === 409 && error === 'Purchase already listed on DH'` → treat as success.
- List failure containing "stock" → present as "DH push pending — check back after sync".
- All other failures → bubble message.

Confirm button label in this mode: "List on DH". For rows that are already `isReadyToList`, no change — the existing single "List" button is still the action.

### Backend changes

- `internal/adapters/httpserver/handlers/dh_dismiss_handler.go::HandleDismissMatch`: relax the pre-condition from `== DHPushStatusUnmatched` to `in {DHPushStatusPending, DHPushStatusUnmatched, DHPushStatusMatched, DHPushStatusManual, DHPushStatusHeld}`. Reject only if already dismissed or if the purchase is already DH-listed or sold.
- `HandleUndismissMatch`: unchanged.
- Event emission: continue recording `dhevents.TypeDismissed` with source `SourceManualUI` and prev/new status reflecting the actual transition.

### Frontend API additions

`web/src/js/api.ts`:

```ts
dismissDHMatch(purchaseId: string): Promise<{ status: 'dismissed' }>
undismissDHMatch(purchaseId: string): Promise<{ status: 'unmatched' }>
```

Both call the existing endpoints. `useInventoryState` gains `handleDismiss(purchase)` and `handleUndismiss(purchase)` with optimistic list update + error rollback + toast on failure.

### Files touched

Backend:
- `internal/adapters/httpserver/handlers/dh_dismiss_handler.go` + its test

Frontend:
- `web/src/js/api.ts`
- `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` + its test
- `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx`
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`
- `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`
- `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx`
- `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts`

## Testing

- `inventoryCalcs.test.ts`: partition test — every item fixture lands in exactly one secondary bucket; `TabCounts` fields match.
- New predicate unit tests for `isSkipped`, `isPendingDHMatch`, `isPendingPrice`, `isDHListed`, `isAwaitingIntake`.
- Component tests:
  - `DesktopRow` renders the correct `actionIntent` per purchase fixture (match-missing → `fix_match`, no-price → `set_and_list`, ready → `list`, dismissed → `restore`).
  - `ExpandedDetail` combined confirm calls `setReviewedPrice` then `listPurchaseOnDH` in order; 409 "already listed" is swallowed; "stock" error message is rewritten.
  - Dismiss / restore buttons hit the right API methods and optimistically re-bucket the item.
- `dh_dismiss_handler_test.go`: extended state matrix — dismiss allowed from `pending`, `unmatched`, `matched`, `manual`, `held`; rejected from `dismissed` and when DH-listed.

## Error handling

- Combined set-price-and-list: if `setReviewedPrice` fails, surface the error and do not attempt listing. If `listPurchaseOnDH` fails with a recognized pattern, translate as above; otherwise bubble raw message.
- Dismiss: optimistic; on failure, rollback the bucket change and show a toast.
- Restore: same pattern.
- Backend dismiss rejects with `409 Conflict` when the item is already dismissed, listed, or sold — matches the existing handler shape.

## Migration

No DB migration. Backend state constraint is a server-side guard only. Old tab-key links that referenced `in_hand` should fall through to `all` on the frontend.
