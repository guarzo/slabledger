# Decision-Time Provenance for Purchases & Sales

**Date:** 2026-07-31
**Migration:** `000022_add_decision_provenance`
**Status:** Design approved, ready for implementation plan

## Problem

SlabLedger stores *current* state well but *what-we-believed-at-decision-time*
poorly, making retrospective analysis of the buying/selling thesis impossible.
Migration `000015` established the set-once frozen-snapshot pattern with
`cl_value_at_purchase_cents`; this work extends that pattern to the fields the
8/15 buy-terms experiment and sell-side diagnostics need.

**Priority 1 blocks a live experiment starting 2026-08-15** whose whole question
is which *combination* of confidence and buy% clears — and confidence-at-purchase
is currently unrecoverable.

## Territory corrections (map vs. codebase)

The original request's map diverged from the code in ways that change the
implementation. Confirmed by inspection:

1. **No `cardLadderConfidenceMinimum` on the purchase's campaign.**
   `inventory.Campaign` has `CLConfidence string` — a *range* like `"2.5-4"`.
   The scalar `CardLadderConfidenceMinimum int` exists only on the PSA-portal
   `psacampaign` types, not in the `CreatePurchase` path.
   **Decision:** parse the min of the range (`"2.5-4" → 2`, truncated), mirroring
   the existing `psacampaign.splitRange` convention. No new campaign column.

2. **Item 4 is not a broken-plumbing bug.** There are three sale-creation paths:
   - **Single-card `RecordSaleModal`** → `HandleCreateSale` → `service.CreateSale`
     — *does* store `days_listed`/`original_list_price_cents`/`price_reductions`
     (behind the form's collapsed "Add listing details" expander).
   - **`BulkRecordSaleModal`** → `HandleBulkSales` → `CreateBulkSales` — builds
     the Sale from only `{price, channel, date}`; never sets those fields.
   - **Order CSV import** → `ConfirmOrdersSales` — CSV carries no listing history.

   All 754 historical rows came through bulk + import, hence 0/754.
   These components live under `web/src/react/pages/campaign-detail/` but the
   **only mount point is the `/inventory` page** (`GlobalInventoryPage` →
   `InventoryTab`); there is no campaign-detail route in `App.tsx`. The folder
   name is legacy.
   **Decision:** add the three listing-detail fields to `BulkRecordSaleModal`
   (the high-volume path). CSV imports legitimately leave them 0 = unknown.

3. **DH comp-thickness (item 3) is partly an ingest bug.**
   `pricing/lookup/adapter.go buildMarketSnapshot` never copies `Confidence` or
   `SourceCount` from the DH `Price` into the snapshot — this is why `confidence`
   is null on every row. `ActiveListings`/`SalesLast30d` already flow when
   `price.Market != nil` (rare at purchase time). Historical rows cannot be
   backfilled.
   **Decision:** fix the ingest going forward; freeze the four DH fields at
   purchase-creation; accept historical rows stay 0 = unknown.

## Decisions (confirmed with operator)

- **Confidence source:** parse min of `campaign.CLConfidence` range, truncated.
- **Item 4 scope:** add listing-detail fields to `BulkRecordSaleModal`.
- **DH ingest:** fix ingest + freeze new columns; no historical backfill.
- **`sale_reason` override:** single-card form `<select>` + dedicated PATCH endpoint.
- **`forced_liquidation` back-compat:** becomes a `GENERATED ALWAYS AS
  (sale_reason = 'invoice_pressure') STORED` column. Consequence accepted: an
  operator setting `aging_policy`/`bulk_lot`/`show_clearout` flips the legacy
  boolean to false. That is the correct semantic (only invoice pressure is
  "forced") and changes what the existing "Forced" P&L bucket counts.
- **Sale-side freeze applies to all three paths** including eBay/DH/Shopify
  import — `cl_value_at_sale_cents` and `channel_fee_pct_at_sale` need no
  external call (`purchase.CLValueCents` and `campaign`+`channel` are already
  in scope in every path).
- **`sale_reason` default at creation:** `invoice_pressure` when the
  `IsForcedLiquidation` heuristic is true, else `discretionary`. The richer
  values are operator-only (nothing at creation can derive them).
- **Analysis richness:** option (A) — build the full confidence×buy% cohort
  block now, not just raw per-row fields.

## § 1 — Migration `000022_add_decision_provenance`

Additive columns; set-once semantics enforced in Go (INSERT-only, never in any
UPDATE), matching `000015`.

### `campaign_purchases` — 6 frozen columns (set at purchase-creation)

| Column | Type | Source at create |
|---|---|---|
| `cl_confidence_at_purchase` | `SMALLINT NOT NULL DEFAULT 0` | min of `campaign.CLConfidence` range (truncated) |
| `population_at_purchase` | `BIGINT NOT NULL DEFAULT 0` | `purchase.Population` |
| `dh_confidence_at_purchase` | `DOUBLE PRECISION NOT NULL DEFAULT 0` | snapshot `Confidence` |
| `source_count_at_purchase` | `BIGINT NOT NULL DEFAULT 0` | snapshot `SourceCount` |
| `active_listings_at_purchase` | `BIGINT NOT NULL DEFAULT 0` | snapshot `ActiveListings` |
| `sales_last_30d_at_purchase` | `BIGINT NOT NULL DEFAULT 0` | snapshot `SalesLast30d` |

### `campaign_sales` — 3 new columns + boolean→reason swap

| Column | Type | Notes |
|---|---|---|
| `sale_reason` | `TEXT NOT NULL DEFAULT ''` | one of: `discretionary`, `invoice_pressure`, `aging_policy`, `bulk_lot`, `show_clearout` |
| `cl_value_at_sale_cents` | `BIGINT NOT NULL DEFAULT 0` | frozen at sale (item 6) |
| `channel_fee_pct_at_sale` | `DOUBLE PRECISION NOT NULL DEFAULT 0` | frozen at sale (item 7) |

### `forced_liquidation` back-compat sequence (up migration)

1. Add `sale_reason TEXT NOT NULL DEFAULT ''`, `cl_value_at_sale_cents`,
   `channel_fee_pct_at_sale`.
2. Backfill `sale_reason` (derivable data only):
   - `forced_liquidation = TRUE` → `invoice_pressure`
   - in-person channel AND `sale_price_cents < 0.80 *
     cl_value_at_purchase_cents` (join purchase; only where
     `cl_value_at_purchase_cents > 0`) → `invoice_pressure`
     — captures the ~36 sales (~$5.2K) the 6-day heuristic missed.
   - else → `discretionary`
3. `DROP COLUMN forced_liquidation`.
4. Re-add `forced_liquidation BOOLEAN GENERATED ALWAYS AS
   (sale_reason = 'invoice_pressure') STORED`. Immutable expression → all
   existing reads (`analysis.go:114,216`, scan helpers) work unchanged.

**Down migration** reverses: drop generated col → re-add plain
`forced_liquidation BOOLEAN NOT NULL DEFAULT FALSE` → `UPDATE ... SET
forced_liquidation = (sale_reason = 'invoice_pressure')` → drop the 3 new sale
columns and the 6 purchase columns.

In-person channel set for backfill = `inperson`, `local`, `cardshow` (matches
`inventory.forcedChannels`).

## § 2 — Go domain & adapter layer

### Types (`internal/domain/inventory/types_core.go`)
- `Purchase`: add 6 `*AtPurchase` fields (int/int/float64/int/int/int),
  `json:"...AtPurchase,omitempty"`.
- `Sale`: add `SaleReason string`, `CLValueAtSaleCents int`,
  `ChannelFeePctAtSale float64`. Keep `ForcedLiquidation bool` but treat as
  **read-only** (generated column; Go stops writing it).
- New `SaleReason*` string-const block + `ValidSaleReason(string) bool` guard
  (5 values + `""`).

### Snapshot ingest fix (`internal/domain/pricing/lookup/adapter.go`)
In `buildMarketSnapshot`, copy `snap.Confidence = price.Confidence` and
`snap.SourceCount = price.SourceCount` from the DH `Price`. Root-cause fix for
"confidence null on all rows." `ActiveListings`/`SalesLast30d` already flow via
`price.Market`.

### Confidence parse helper (`internal/domain/inventory/`)
Exported `ParseCLConfidenceMin(s string) int` — truncating min of a range like
`"2.5-4"` → `2`; returns 0 on unparseable input. Unit-testable; mirrors
`psacampaign.splitRange` truncation.

### Freeze at purchase-creation (`service_crud.go CreatePurchase`)
- Keep the campaign returned by `GetCampaign` (currently discarded at ~:60).
- After `captureMarketSnapshot`, set the 6 frozen fields **set-once** (only when
  currently 0): confidence from `ParseCLConfidenceMin(campaign.CLConfidence)`,
  population from `p.Population`, the four DH fields from the applied snapshot.

### Persist (`purchase_store.go`)
- Extend `CreatePurchase` INSERT (6 new params).
- Extend `purchaseColumns` + `purchaseColumnsAliased` + `scanPurchase`.
- `UpdatePurchaseCLValue`: **no change** — it correctly writes live `population`.
  Add a comment at the `population = $2` line noting `population_at_purchase` is
  the frozen counterpart and must never be added to an UPDATE.

### Freeze at sale-creation (all 3 paths: `CreateSale`, `CreateBulkSales`,
`ConfirmOrdersSales`)
- `sa.CLValueAtSaleCents = purchase.CLValueCents`
- `sa.ChannelFeePctAtSale = EffectiveChannelFeePct(channel, campaign)` — new
  helper extracted from `CalculateSaleFee`'s branch logic so fee cents and pct
  never diverge (eBay/TCGPlayer → campaign eBay pct; website → 3%; else → 0).
- `sa.SaleReason = invoice_pressure if IsForcedLiquidation(...) else
  discretionary`.
- Stop setting `sa.ForcedLiquidation` (generated).
- Extend `sale_store.go` INSERT + `saleColumns` + `saleColumnsAliased` +
  `scanSale`/`scanPurchaseWithSale`.

### Operator override (item 5)
- New `PATCH /api/campaigns/{id}/sales/{saleID}` handler → validates reason via
  `ValidSaleReason` → `SaleRepository.UpdateSaleReason(ctx, saleID, reason)`
  (`UPDATE sale_reason, updated_at`). Generated `forced_liquidation` follows.
- Add `UpdateSaleReason` to the `SaleRepository` interface + Postgres impl +
  `internal/testutil/mocks` `SaleRepositoryMock`.

## § 3 — Analysis API surface (option A) & frontend

Consumer of `/api/portfolio/analysis` is the **campaign-analysis skill** (no
frontend TS consumer). The analysis load path (`GetPurchasesWithSales` /
`GetAllPurchasesWithSales`) already reuses the shared column lists +
`scanPurchaseWithSale`, so frozen fields flow in automatically once § 2 extends
those.

### `analysis_types.go` / `analysis.go` additions (per `CampaignAnalysis`)
1. **`PNLByConfidenceBuy []ConfBuyCohortRow`** — the experiment deliverable.
   Row = `{clConfidenceAtPurchase, buyTermsBucket, n, soldCount, revenueCents,
   netProfitCents, roiPct, avgSourceCount, avgActiveListings, avgSalesLast30d,
   avgPopulationAtBuy}`. Rows with `cl_confidence_at_purchase = 0` go in an
   explicit `unknown` bucket, never mixed into a real level.
2. **`SplitPNL.ByReason map[string]PNLBlock`** — keyed by the 5 reasons; the 36
   reclassified sales now land in `invoice_pressure`. `Discretionary`/`Forced`
   kept as-is for back-compat.
3. **Comp-thickness averages** on cohort rows computed over frozen fields with a
   coverage count (`N`/`Total`/`CoveragePct` pattern from `BPCLStats`);
   skip-if-zero before averaging so 0 = unknown never dilutes.

### Frontend (`web/src/types/campaigns/core.ts` + components)
- Add new `Sale`/`Purchase` fields to match Go JSON tags: `saleReason`,
  `clValueAtSaleCents`, `channelFeePctAtSale`, the 6 `*AtPurchase`.
- `RecordSaleForm` (single): add `sale_reason` `<select>`.
- `BulkRecordSaleModal`: add `sale_reason` `<select>` **and** the three
  listing-detail inputs (item 4: `originalListPrice`, `priceReductions`,
  `daysListed`) + wire them into the bulk request/handler/`CreateBulkSales`.

### Docs
- `docs/SCHEMA.md`: new columns + generated-column note.
- `docs/API.md`: PATCH route + analysis response additions.

## "0/'' = unknown" invariant

Enforced in the compute layer (skip-if-zero before averaging; explicit `unknown`
cohort buckets), consistent with `computeBPCLAtBuy`'s existing
`CLValueAtPurchaseCents == 0` guard. A pre-instrumentation row must never read as
a real confidence 0 or population 0.

## Testing

- Table-driven, mocks from `internal/testutil/mocks`.
- `ParseCLConfidenceMin` unit table (ranges, single values, junk → 0).
- Set-once guard: creating a purchase twice / refreshing CL never overwrites a
  non-zero frozen field.
- `EffectiveChannelFeePct` parity with `CalculateSaleFee` across channels.
- Sale freeze across all three creation paths (single/bulk/import) sets
  cl-at-sale, channel-fee, and reason default.
- Migration up/down round-trip incl. `sale_reason` backfill rules and the
  generated `forced_liquidation` reflecting reason.
- `UpdateSaleReason` PATCH validates against `ValidSaleReason`.
- Analysis: confidence×buy% cohort buckets `0` into `unknown`; `ByReason` split;
  coverage-guarded averages.

## Explicit exclusions

- No historical backfill of DH comp fields or listing-detail fields (no source).
- No new campaign column for confidence (parsed from existing range).
- No frontend analysis-page types (endpoint has no UI consumer).
- No change to `population` live-refresh behavior (only the frozen counterpart
  is protected).
