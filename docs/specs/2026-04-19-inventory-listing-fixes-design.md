# Inventory listing follow-ups: no-price listing + DH deletion detection

**Date:** 2026-04-19
**Status:** Approved (pending written-spec review)
**Branch:** `inventory-listing-fixes`

## Problem

Two issues in the DH listing workflow surface as friction for operators:

1. **No-price listing.** When a purchase has no `reviewedPriceCents`, the "List on DH" row button still appears. Clicking it returns HTTP 409 "Review the price before listing on DH," but the UI surfaces only a toast — there is no path from the error back to the price-review UI. The user has to find the expand action themselves.
2. **DH-side deletion.** When an item is manually deleted on DoubleHolo (not sold, just removed), the reconciler that detects it runs **once per day**. Between the delete and the next reconciler tick, local DB still shows the item as `listed`, and after the reset runs silently there is no indication in the UI that this specific row was unlisted by DH rather than flowing through the normal push pipeline.

## Goals

- Guide users through the existing price-review flow instead of blocking them with an unactionable error.
- Detect DH-side deletions within an hour (automatically) or on demand (manually), and visibly mark re-listable rows so operators can see the round-trip happened.
- No new listing endpoints, no new price sources, no changes to `ResolveListingPriceCents`.

## Non-goals

- Bulk-select UI changes. Bulk "List on DH" from the sell-sheet keeps its current silent-skip behaviour for price-less items (operator confirmed).
- One-click "save price + list" fusion button. Design is two-step: save price, then list.
- Webhook-driven DH state sync. Hourly poll + manual trigger is sufficient.
- Admin panel restructure. Only a single "Sync DH now" button is added.

## Current state (verified)

### Listing gate (backend)

- `internal/adapters/httpserver/handlers/campaigns_dh_listing.go:72` — handler rejects with 409 when `p.ReviewedPriceCents == 0`.
- `internal/domain/dhlisting/dh_listing_service.go:191` — service skips items where `ResolveListingPriceCents(p) == 0`.
- `internal/domain/dhlisting/dh_push_safety.go:17-19` — `ResolveListingPriceCents` returns only `p.ReviewedPriceCents`; CL deliberately excluded to prevent silent stale listings.

### Row button logic (frontend)

- `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts:71-73` — `isReadyToList` returns true for any received + `dhInventoryId != 0` + not-`listed` item. **It does not check `reviewedPriceCents`.**
- `DesktopRow.tsx:231` and `MobileCard.tsx:246` — both render "List on DH" when `isReadyToList(item) && dhInventoryId`.
- `ExpandedDetail.tsx` — already contains the `PriceDecisionBar` with an `onReviewed` callback that saves `reviewedPriceCents`.

### Reconciler (backend)

- `internal/domain/dhlisting/dh_reconcile.go` — fetches DH inventory snapshot, resets local purchases whose `dh_inventory_id` is missing from the snapshot.
- `internal/adapters/storage/sqlite/purchase_dh_store.go:144-157` — `ResetDHFieldsForRepush` clears `dh_inventory_id`, `dh_status`, `dh_listing_price_cents`, `dh_channels_json`, `dh_hold_reason`; sets `dh_push_status='pending'`. **Preserves `reviewed_price_cents`, `dh_card_id`, `dh_cert_status`.**
- `internal/adapters/scheduler/dh_reconcile.go` — daily scheduler, fires at `RefreshHour`. `RunOnce(ctx)` is exported.
- `internal/domain/dhevents/events.go` — reconciler already records a `TypeUnlisted` event with `source = dh_reconcile` for every reset.

### Existing admin trigger pattern

Handlers that expose manual scheduler triggers follow the same shape:
- `internal/adapters/httpserver/handlers/psa_sync_handler.go`
- `internal/adapters/httpserver/handlers/cardladder.go`
- `internal/adapters/httpserver/handlers/marketmovers.go`
- `internal/adapters/httpserver/handlers/dh_ingest_orders_handler.go`

Each accepts a small interface with `RunOnce(ctx) error` and is wired under `authMW.RequireAdmin`.

## Design

### Issue 1 — Guide users through price review

**Backend: no changes.** The 409 guard in the handler stays as defense-in-depth. `ResolveListingPriceCents` stays as-is.

**Frontend — `web/src/react/pages/campaign-detail/inventory/`:**

1. **New predicate in `inventoryCalcs.ts`:**
   ```ts
   export function needsPriceReview(item: AgingItem): boolean {
     return (
       !!item.purchase.receivedAt &&
       !!item.purchase.dhInventoryId &&
       item.purchase.dhStatus !== 'listed' &&
       (item.purchase.reviewedPriceCents ?? 0) === 0
     );
   }
   ```

2. **Tighten `isReadyToList`** to require a reviewed price:
   ```ts
   export function isReadyToList(item: AgingItem): boolean {
     return (
       !!item.purchase.receivedAt &&
       !!item.purchase.dhInventoryId &&
       item.purchase.dhStatus !== 'listed' &&
       (item.purchase.reviewedPriceCents ?? 0) > 0
     );
   }
   ```
   Knock-on: the `ready_to_list` filter-tab count drops to only truly-listable items; price-less items surface via `needsPriceReview`. The existing `filterAndSortItems` `ready_to_list` branch uses `isReadyToList`, so its count automatically becomes accurate. No new filter tab — price-less items remain discoverable via the expanded-row flow and the existing "Needs attention" predicate if relevant.

3. **Row button update in `DesktopRow.tsx` and `MobileCard.tsx`:**
   - If `needsPriceReview(item)` → render **"Set price"** button. `onClick={() => onExpand(item.purchase.id)}` (reuses the existing expand callback — no new plumbing).
   - Else if `isReadyToList(item) && !!item.purchase.dhInventoryId` → render **"List on DH"** (unchanged behaviour).
   - The two branches are mutually exclusive thanks to the tightened `isReadyToList`.

4. **No changes to `ExpandedDetail.tsx` or `PriceDecisionBar`.** The existing `onReviewed` callback already saves `reviewedPriceCents`. After save, the parent's state update causes `needsPriceReview` → false and `isReadyToList` → true, so the row button flips from "Set price" to "List on DH" automatically on next render. The user closes the expand and clicks "List on DH."

5. **Bulk sell-sheet unchanged.** `SellSheetView.tsx`'s bulk "List on DH (N)" keeps current behaviour: sends all selected IDs; backend skips price-less items and returns counts in the toast.

### Issue 2 — Detect DH-side deletion

#### 2a. Cadence: daily → hourly

**File:** `internal/platform/config/defaults.go` (and related in `types.go`, `loader.go`).
- Default `DHReconcileConfig.Interval`: `24 * time.Hour` → `1 * time.Hour`.
- Drop `DHReconcileConfig.RefreshHour` entirely (field + env binding + references). Hourly runs don't need an hour-of-day anchor. The scheduler's `InitialDelay` in `dh_reconcile.go:53` simplifies to `0` (run immediately on startup, then every `Interval`).
- Env var `DH_RECONCILE_INTERVAL` (new; see `.env.example`). `DH_RECONCILE_REFRESH_HOUR` removed; if operators have it set it becomes a no-op — note in `docs/SCHEDULERS.md`.

**Rate-limit note:** 24× more `FetchAllInventoryIDs` calls per day. The DH enterprise API allows this based on current usage; worst case we back off the default to every 2-4h later without any code change. Document in `.env.example`.

#### 2b. Manual trigger endpoint

**New handler:** `internal/adapters/httpserver/handlers/dh_reconcile_handler.go`.

```go
type DHReconcileRunner interface {
    RunOnce(ctx context.Context) error
    GetLastRunResult() *dhlisting.ReconcileResult
}

type DHReconcileHandler struct {
    runner DHReconcileRunner
    logger observability.Logger
}

func (h *DHReconcileHandler) HandleTrigger(w http.ResponseWriter, r *http.Request) {
    // POST → runs reconciler synchronously, returns ReconcileResult JSON.
    // Returns 503 when runner is nil.
    // Returns 502 with error message when RunOnce fails.
    // Returns 200 with the last result on success.
}
```

**Route:** `POST /api/admin/dh-reconcile/trigger`, wrapped with `authMW.RequireAdmin`.

**Wiring:** `cmd/slabledger/init_schedulers.go` / wherever `DHReconcileScheduler` is instantiated — pass it into the handler constructor. `DHReconcileScheduler` already satisfies the `DHReconcileRunner` interface.

#### 2c. Track "unlisted from DH" signal

**Migration:** `internal/adapters/storage/sqlite/migrations/000070_dh_unlisted_detected_at.up.sql`
```sql
ALTER TABLE campaign_purchases ADD COLUMN dh_unlisted_detected_at TIMESTAMP NULL;
```
**Down:** drop the column (SQLite workaround: rebuild table, consistent with prior migrations in this repo).

**New storage method:** `ResetDHFieldsForRepushDueToDelete(ctx, purchaseID)` — same as `ResetDHFieldsForRepush` plus `dh_unlisted_detected_at = ?` (current time). A separate method (rather than a parameter) makes the reconciler call site explicit and keeps the existing reset signature unchanged for all other callers.

**Reconciler:** `internal/domain/dhlisting/dh_reconcile.go` — `DHReconcileResetter` interface gains a second method `ResetDHFieldsForRepushDueToDelete`. The reconciler calls the new variant.

**Clear on successful list:** `internal/domain/dhlisting/dh_listing_service.go` around line 267 (post-channel-sync, when `UpdateInventoryStatus` to `DHStatusListed` succeeds) — call a new storage method `ClearDHUnlistedDetectedAt(ctx, purchaseID)` to null the column. This means the badge disappears the moment the item is successfully re-listed.

**API exposure:** `Purchase` struct (`internal/domain/inventory/types_core.go`) gains `DHUnlistedDetectedAt *time.Time` with JSON tag `dhUnlistedDetectedAt,omitempty`. Matching TS interface in `web/src/types/campaigns/` (e.g. `core.ts`) gains `dhUnlistedDetectedAt?: string`.

#### 2d. Row badge

**`inventoryCalcs.ts`:**
```ts
export function wasUnlistedFromDH(item: AgingItem): boolean {
  return !!item.purchase.dhUnlistedDetectedAt;
}
```

**Render:** `DesktopRow.tsx` and `MobileCard.tsx` — add a small badge near the existing DH status chip:
> **Re-list (removed from DH)**

Visible whenever `wasUnlistedFromDH(item)` is true. Paired with either the "Set price" or "List on DH" button depending on whether `reviewedPriceCents` survived (it does — `ResetDHFieldsForRepush` preserves it).

#### 2e. Admin "Sync DH now" button

Placement: the existing admin Tools area that hosts the DH push config (`web/src/react/pages/admin/` — the tab/section with PSA sync and CL sync buttons).

Button calls `POST /api/admin/dh-reconcile/trigger`. On success, toast shows:
- `Reset N items removed from DH` when `result.reset > 0`.
- `All DH-linked items still present on DH` when `result.missingOnDH === 0`.
- `N reset, M errors — check logs` when `len(errors) > 0`.

### Tests

**Backend:**
- `internal/domain/dhlisting/dh_reconcile_test.go` — add a case asserting the reconciler calls `ResetDHFieldsForRepushDueToDelete` (not the plain reset). Use a fake resetter with separate counters per method.
- New handler test `dh_reconcile_handler_test.go` — mirror `psa_sync_handler_test.go` shape. Cover success, `nil runner` → 503, RunOnce error → 502.
- `purchase_dh_store_test.go` (if it exists; otherwise extend the closest sibling) — test `ResetDHFieldsForRepushDueToDelete` sets the timestamp; `ClearDHUnlistedDetectedAt` nulls it.
- Storage tests for the new migration (up + down).

**Frontend:**
- `inventoryCalcs.test.ts` — cover `needsPriceReview`, `wasUnlistedFromDH`, and the tightened `isReadyToList`.
- Row component test (or an extension of existing ones) — "Set price" renders when `reviewedPriceCents == 0`; "List on DH" renders when present; badge renders when `dhUnlistedDetectedAt` is set.

### Observability

- Existing `dhevents.TypeUnlisted` events from the reconciler are unchanged.
- New handler logs on trigger: scanned/missing/reset/errors counts.
- Scheduler log fields drop `refreshHour` (no longer relevant); add no new fields.

## File-change summary

**Backend (Go):**
- `internal/platform/config/defaults.go`, `types.go`, `loader.go` — interval default 24h → 1h; remove `RefreshHour`.
- `internal/adapters/scheduler/dh_reconcile.go` — simplify `InitialDelay` to 0; drop `refreshHour` log field.
- `docs/SCHEDULERS.md` — update cadence and env-var notes.
- `internal/adapters/storage/sqlite/migrations/000070_dh_unlisted_detected_at.up.sql` + `.down.sql`.
- `internal/adapters/storage/sqlite/purchase_dh_store.go` — add `ResetDHFieldsForRepushDueToDelete`, `ClearDHUnlistedDetectedAt`.
- `internal/domain/dhlisting/dh_reconcile.go` — extend `DHReconcileResetter` interface; call new method.
- `internal/domain/dhlisting/dh_listing_service.go` — clear the timestamp on successful list.
- `internal/domain/inventory/types_core.go` — add `DHUnlistedDetectedAt *time.Time`.
- `internal/domain/inventory/repository_dh.go` (and purchase read path) — select the new column.
- `internal/adapters/httpserver/handlers/dh_reconcile_handler.go` (new).
- `internal/adapters/httpserver/routes.go` — wire the route.
- `cmd/slabledger/init_schedulers.go` (or equivalent) — pass the scheduler into the handler.
- Tests as listed above.

**Frontend (TS/React):**
- `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts` — `needsPriceReview`, `wasUnlistedFromDH`, tighter `isReadyToList`.
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` — button branching + badge.
- `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx` — same.
- `web/src/types/campaigns/core.ts` (or the Purchase type location) — add `dhUnlistedDetectedAt?: string`.
- `web/src/js/api/admin.ts` — new `triggerDHReconcile()` method on the admin API client.
- `web/src/react/pages/admin/…` — add "Sync DH now" button alongside existing sync buttons.
- Tests for `inventoryCalcs` and row components.

## Rollout

1. Migration ships on next deploy; `dh_unlisted_detected_at` defaults to NULL for all existing rows — no backfill needed.
2. Frontend and backend changes ship together (single PR).
3. Operator-visible changes:
   - Price-less items in inventory show "Set price" instead of a failing "List on DH."
   - After deleting an item on DH, within up to 1 hour (or immediately via the "Sync DH now" button) the item reappears with a "Re-list (removed from DH)" badge and a "List on DH" button (price preserved).

## Risks

- **DH rate limits.** Hourly snapshot × 24 requests/day. If the DH API limits become an issue, raise the interval env var without code change.
- **Reconciler still fails closed.** Unchanged — snapshot fetch error aborts with zero resets. Good.
- **Badge noise.** If operators delete many DH listings as normal workflow, the badge becomes ambient. Mitigation: clears automatically on successful re-list.
