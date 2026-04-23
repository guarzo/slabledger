# Decoupled Comp Refresh & Last-Sold Backfill

## Problem

The current CL sales comp refresh (Phase 4 of `CardLadderRefreshScheduler`) depends on `cl_card_mappings` — it only refreshes comps for cards that have been mapped through the CL card identification pipeline. Cards that have a `gem_rate_id` from DH cert resolution (Task 2) but no `cl_card_mappings` row are invisible to the comp refresh.

Additionally, `last_sold_date` and `last_sold_cents` on `campaign_purchases` are not populated from comp data — they come from a separate (currently broken) path. This means the liquidation preview can't show when a card last sold without inspecting the comp table directly.

## Proposed Solution

### 1. CompRefreshStore (Postgres adapter)

New file: `internal/adapters/storage/postgres/comp_refresh.go`

**`ListUnsoldCardsNeedingComps(ctx, cutoffDays int) ([]UnsoldCardForComps, error)`**

Query unsold purchases that have `gem_rate_id` set, where either:
- No comps exist in `cl_sales_comps` for that `gem_rate_id` + condition
- The most recent comp is older than `cutoffDays`

Uses a `LATERAL` join to check the latest comp date per card. Returns `(purchaseID, gemRateID, gradeValue)` tuples for the scheduler to iterate.

Important: the `condition` column in `cl_sales_comps` uses display format (`"PSA 10"`, `"PSA 9"`, `"PSA 9.5"`) — build conditions as `'PSA ' || grade_value` (preserving decimals), not the gemRate format (`"g10"`).

**`BackfillLastSoldFromComps(ctx) (int, error)`**

Bulk UPDATE that sets `last_sold_date` and `last_sold_cents` on `campaign_purchases` from the most recent comp in `cl_sales_comps` matching each card's `gem_rate_id` + condition. Only updates rows where the existing `last_sold_date` is null/empty or older than the comp.

### 2. Comp Refresh Scheduler

New scheduler (or new phase in existing `CardLadderRefreshScheduler`) that:

1. Calls `ListUnsoldCardsNeedingComps(ctx, 30)` to find cards needing fresh comps
2. For each unique `(gemRateID, condition)` pair, fetches comps from CL sales archive via the existing `cardladder.Client.FetchSalesComps`
3. Upserts results into `cl_sales_comps` via existing `CLSalesStore.UpsertSaleComp`
4. After all comps are refreshed, calls `BackfillLastSoldFromComps` to propagate latest sale data to purchases
5. Rate-limited by the CL client's built-in limiter (same as current gap fill)

### Design Decisions

- **Separate scheduler vs. new phase**: Adding as Phase 4b in the existing `CardLadderRefreshScheduler` is simplest — it already has the CL client and sales store wired in. The decoupled query replaces the `cl_card_mappings`-dependent path; the old path can be kept as fallback for cards without `gem_rate_id`.
- **Condition format**: Must use `"PSA {grade}"` to match `cl_sales_comps` storage format. Half-grades (9.5) must preserve the decimal.
- **Dedup**: Multiple purchases can share the same `gem_rate_id` — dedup by `(gemRateID, condition)` before fetching to avoid redundant API calls.
- **Backfill timing**: Run `BackfillLastSoldFromComps` once at the end of the refresh cycle, not per-card — single bulk UPDATE is more efficient.

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/adapters/storage/postgres/comp_refresh.go` | Create | `CompRefreshStore` with both queries |
| `internal/adapters/scheduler/cardladder_refresh.go` | Modify | Add Phase 4b using `CompRefreshStore` |
| `cmd/slabledger/init_schedulers.go` | Modify | Wire `CompRefreshStore` into scheduler |

### Dependencies

- `gem_rate_id` populated on purchases (Task 2 — done, DH now returns it on matched certs)
- CL sales archive API access (existing `cardladder.Client`)
- `cl_sales_comps` table (existing)
