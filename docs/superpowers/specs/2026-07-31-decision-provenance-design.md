# Decision-Time Provenance for Purchases & Sales

**Date:** 2026-07-31
**Migration:** `000022_add_decision_provenance`
**Status:** Design v2 (post-review), ready for implementation plan

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

Confirmed by inspection; these change the implementation:

1. **No `cardLadderConfidenceMinimum` on the purchase's campaign.**
   `inventory.Campaign` has `CLConfidence string` — a *range* like `"2.5-4"`.
   The scalar exists only on PSA-portal `psacampaign` types.
   **Decision:** parse the truncated min of the range (`"2.5-4" → 2`), mirroring
   `psacampaign.splitRange`. No new campaign column.

2. **Item 4 is not a broken-plumbing bug.** Three sale-creation paths:
   - **Single-card `RecordSaleModal`** → `HandleCreateSale` → `service.CreateSale`
     — *does* store `days_listed`/`original_list_price_cents`/`price_reductions`.
   - **`BulkRecordSaleModal`** → `HandleBulkSales` → `CreateBulkSales` — builds
     the Sale from only `{price, channel, date}`.
   - **Order CSV import** → `ConfirmOrdersSales` — CSV carries no listing history.

   All 754 historical rows came through bulk + import. Components live under
   `web/src/react/pages/campaign-detail/` but the **only mount point is
   `/inventory`** (`GlobalInventoryPage` → `InventoryTab`); no campaign-detail
   route exists. Folder name is legacy.
   **Decision:** add per-item listing-detail fields to `BulkRecordSaleModal`.

3. **DH ingest already copies these fields — my v1 "fix" was wrong (review #2).**
   `buildMarketSnapshot` (adapter.go:266-270) already sets
   `snap.Confidence = price.Confidence` and `snap.SourceCount = len(price.Sources)`.
   `pricing.Price` has **no** `SourceCount` field (only `Sources []string`).
   The real gap is downstream: `MarketSnapshotData` (the struct persisted to
   columns) has **no** `Confidence`/`SourceCount` fields, and `applySnapshot`
   (types_core.go:131) discards them — they survive only inside `SnapshotJSON`.

## Decisions (confirmed with operator, v2)

- **Confidence source:** truncated min of `campaign.CLConfidence` range.
- **Frozen-zero semantics (review #1):** the 6 purchase provenance columns and
  `channel_fee_pct_at_sale` are **nullable** (`NULL` = not captured, `0` =
  genuinely observed zero). No `DEFAULT 0`. Analytics skip `NULL`, keep real
  zeros.
- **`source_count_at_purchase` meaning (review #3):** **provider platform count
  captured BEFORE `applyCLCorrection`** — i.e. `len(price.Sources)` of external
  pricing platforms (DH/eBay/etc.), excluding the `cardladder` anchor that
  correction unconditionally appends. Represents external comp breadth, not our
  own signal. Requires capturing the count pre-correction and threading it to the
  freeze (see § 2). Documented under that name; not "comp thickness."
- **Freeze lifecycle (review #1 + #2) — SPLIT by when each fact is known:**
  - `cl_confidence_at_purchase` and `population_at_purchase` are known at **row
    creation** → freeze set-once in `CreatePurchase`, independent of any
    snapshot. Campaign confidence is editable later (`campaign_store.go
    UpdateCampaign`) and population refreshes via `UpdatePurchaseCLValue`, so
    waiting for async enrichment would record post-decision values.
  - The 4 market fields (dh_confidence, source_count, active_listings,
    sales_last_30d) freeze at the **first SUCCESSFUL snapshot** — gated on
    confirmed capture, never on a silent provider failure (review #2).
  - Async enrichment fills `cl_confidence`/`population` **only as a NULL
    compatibility fallback** (for rows created before this migration that never
    got them); it does **not** fetch current campaign state as the normal path.
- **Item 4 scope:** add per-item listing fields to `BulkRecordSaleModal`.
- **`sale_reason` UX (review #5):** creation **preserves** an explicit valid
  reason and defaults to the heuristic only when the field is empty. PATCH for
  later edits.
- **`forced_liquidation` back-compat:** `GENERATED ALWAYS AS
  (sale_reason = 'invoice_pressure') STORED`. Operator setting a non-pressure
  reason flips the legacy boolean false (correct semantic; changes the "Forced"
  P&L bucket).
- **Sale-side freeze applies to all three paths**, but for imports these are
  **record-time proxies**, not exact decision-time values (review #9).
- **Buy-term bucket (review #7):** `buyCost / cl_value_at_purchase_cents` (frozen
  CL, never current), 5% bands; `<50%` and `>=100%` each collapse to one bucket;
  missing frozen CL → explicit `unknown` bucket.
- **Analysis richness:** option (A) — full confidence×buy% cohort block now.

## § 1 — Migration `000022_add_decision_provenance`

Additive; set-once enforced in Go (INSERT/first-enrichment only).

### `campaign_purchases` — 6 frozen columns, **NULLABLE** (set at first enrichment)

| Column | Type | Source |
|---|---|---|
| `cl_confidence_at_purchase` | `SMALLINT` (NULL) | truncated min of `campaign.CLConfidence` |
| `population_at_purchase` | `BIGINT` (NULL) | `purchase.Population` |
| `dh_confidence_at_purchase` | `DOUBLE PRECISION` (NULL) | snapshot `Confidence` |
| `source_count_at_purchase` | `BIGINT` (NULL) | `len(price.Sources)` = distinct platforms |
| `active_listings_at_purchase` | `BIGINT` (NULL) | snapshot `ActiveListings` |
| `sales_last_30d_at_purchase` | `BIGINT` (NULL) | snapshot `SalesLast30d` |

`NULL` = never captured; a stored `0` = a real measured zero. No backfill.

### `campaign_sales` — 3 new columns + boolean→reason swap

| Column | Type | Notes |
|---|---|---|
| `sale_reason` | `TEXT NOT NULL DEFAULT ''` + `CHECK (sale_reason IN ('','discretionary','invoice_pressure','aging_policy','bulk_lot','show_clearout'))` | `''` = legacy/unknown, only via backfill/creation-default gaps |
| `cl_value_at_sale_cents` | `BIGINT NOT NULL DEFAULT 0` | frozen at sale (0 = unknown) |
| `channel_fee_pct_at_sale` | `DOUBLE PRECISION` (NULL) | frozen at sale; NULL=uncaptured (legacy), 0.0=real free channel |

### `forced_liquidation` back-compat sequence (up migration)

1. Add `sale_reason` (with CHECK), `cl_value_at_sale_cents`,
   `channel_fee_pct_at_sale`.
2. Backfill `sale_reason` (derivable only):
   - `forced_liquidation = TRUE` → `invoice_pressure`
   - in-person channel (`inperson`/`local`/`cardshow`) AND `sale_price_cents <
     0.80 * cl_value_at_purchase_cents` (join purchase; only where
     `cl_value_at_purchase_cents > 0`) → `invoice_pressure` (~36 sales, ~$5.2K)
   - else → `discretionary`
3. `DROP COLUMN forced_liquidation`.
4. Re-add `forced_liquidation BOOLEAN GENERATED ALWAYS AS
   (sale_reason = 'invoice_pressure') STORED`.

**Down migration** reverses: drop generated col → re-add plain boolean → `UPDATE
... = (sale_reason='invoice_pressure')` → drop 3 sale cols + 6 purchase cols.

## § 2 — Go domain & adapter layer

### Types (`internal/domain/inventory/types_core.go`)
- Add `Confidence float64` and `SourceCountRaw int` to **`MarketSnapshotData`**
  (the shared struct persisted to columns) and copy them in `applySnapshot`.
  Without this the async persist path drops both values (root cause). Note
  `SourceCountRaw` must be the **pre-CL-correction** platform count (see the
  capture gate below), distinct from `MarketSnapshot.SourceCount` which
  `applyCLCorrection` inflates with `cardladder`.
- `Purchase`: add 6 provenance fields as **`*int`/`*float64` pointers** (nullable
  ⇔ NULL column) so measured-zero ≠ missing. `json:",omitempty"`.
- `Sale`: add `SaleReason string`, `CLValueAtSaleCents int`,
  `ChannelFeePctAtSale *float64` (nullable — legacy NULL vs real 0.0).
  `ForcedLiquidation bool` becomes **read-only** (generated).
- `SaleReason*` const block. Two guards: `ValidSaleReason(s)` (allows `""`, used
  for creation-default fallback) and `ValidSaleReasonForPatch(s)` (rejects `""`).

### Confidence parse helper
Exported `ParseCLConfidenceMin(s string) (int, bool)` — truncated min of a range;
`ok=false` on unparseable input (→ leave column NULL). Unit-tested.

### Freeze lifecycle — split by when each fact is known

**(a) Row-creation facts — `cl_confidence_at_purchase`, `population_at_purchase`.**
Frozen in `service_crud.go CreatePurchase`, set-once, **independent of any
snapshot**:
- Keep the campaign returned by `GetCampaign` (currently discarded); set
  confidence from `ParseCLConfidenceMin(campaign.CLConfidence)` when `ok` and the
  pointer is nil.
- Set population from `p.Population` when nil.
- These freeze even when the purchase is created `pending` (PSA bulk), because
  they don't depend on the price provider. This is the decision-time value;
  waiting for async enrichment would capture edited-campaign / refreshed-pop
  values instead (review #1).

**(b) Market facts — `dh_confidence`, `source_count`, `active_listings`,
`sales_last_30d`.** Frozen on the **first SUCCESSFUL snapshot**, gated on
confirmed capture (review #2):
- `captureMarketSnapshot` currently returns nothing and silently returns on
  provider error — copying fields after it would store 0 as a real observation.
  Fix: have the capture helpers **signal success** (return a bool / the applied
  `*MarketSnapshot`, or freeze only when `snapshot.SnapshotDate` is non-empty).
  Freeze the 4 market pointers **only** on confirmed apply; a failed capture
  leaves them NULL.
- Capture the **pre-correction** platform count: read `len(snapshot.Sources)`
  *before* `applyCLCorrection` appends `cardladder`, and carry it into
  `SourceCountRaw` (review #3). Both the sync (`captureMarketSnapshot`) and async
  (`recaptureMarketSnapshotDetailed`) helpers call `applyCLCorrection`, so both
  must snapshot the count before that call.
- Sync path: freeze in `CreatePurchase` when capture succeeds. Async path:
  `UpdatePurchaseMarketSnapshot` (`purchase_price_store.go:255`) sets the 4
  columns **only when currently NULL** (`col = CASE WHEN col IS NULL THEN $n ELSE
  col END`), sourced from the now-provenance-carrying `MarketSnapshotData`. First
  successful writer wins.

**(c) Async compatibility fallback.** `processSnapshotsByStatus`
(`service_snapshots.go:217`) loops purchases with the full `p` in scope. For rows
predating this migration whose (a)-fields are still NULL, backfill
`population_at_purchase` from `p.Population` and `cl_confidence_at_purchase` from
the owning campaign (fetch once per distinct `CampaignID`, cache in a
`map[string]int`). This is a **NULL-only fallback**, not the normal path — new
rows already have (a) frozen at creation.

### Persist (`purchase_store.go` + scan helpers)
- Extend `CreatePurchase` INSERT + `purchaseColumns`/`purchaseColumnsAliased` +
  `scanPurchase` (scan into `sql.Null*` → pointers).

### Freeze at sale-creation (all 3 paths)
- `sa.CLValueAtSaleCents = purchase.CLValueCents`
- `sa.ChannelFeePctAtSale = &EffectiveChannelFeePct(channel, campaign)` (pointer;
  always captured on new sales, so non-nil going forward) — new helper extracted
  from `CalculateSaleFee` so fee cents and pct never diverge.
- `sa.SaleReason`: **preserve** a caller-supplied valid reason; default to
  `invoice_pressure if IsForcedLiquidation(...) else discretionary` only when
  empty (review #5).
- Stop setting `ForcedLiquidation`.

### Sale column split (review #4)
- `saleInsertColumns` — excludes generated `forced_liquidation`; adds the 3 new
  sale columns; used by all INSERTs.
- `saleColumns` — full read/scan list, still includes `forced_liquidation` + the
  3 new columns.
- **`saleColumnsAliased`** — the LEFT-JOIN list used by portfolio analysis
  (`analytics_store.go:190`) — currently has only `forced_liquidation` and none
  of the new fields. **Must** be extended with `sale_reason`,
  `cl_value_at_sale_cents`, `channel_fee_pct_at_sale`, and `scanPurchaseWithSale`
  extended to scan them (`sql.Null*`), or `SplitPNL.ByReason` reads nothing
  (review #4).
- Update `sale_store.go` INSERT placeholder count; `scanSale`/`scanPurchaseWithSale`.

### Operator override (review #6)
- `PATCH /api/campaigns/{id}/sales/{saleID}` → `ValidSaleReasonForPatch`
  (rejects `""`) → `SaleRepository.UpdateSaleReason(ctx, campaignID, saleID,
  reason)`. The UPDATE is **campaign-scoped** (`WHERE id=$saleID AND purchase_id
  IN (SELECT id FROM campaign_purchases WHERE campaign_id=$campaignID)`) so the
  path param is enforced; returns not-found if the sale isn't in that campaign.
- Add to `SaleRepository` interface + Postgres impl + `mocks.SaleRepositoryMock`.

## § 3 — Analysis API surface (option A) & frontend

Consumer is the **campaign-analysis skill** (no frontend TS consumer). Analysis
load reuses shared column lists + `scanPurchaseWithSale`, so provenance flows in
once § 2 extends them.

### `analysis_types.go` / `analysis.go`
1. **`PNLByConfidenceBuy []ConfBuyCohortRow`.** Row =
   `{confidenceBucket string, buyTermsBucket string, n, soldCount,
   revenueCents, netProfitCents, roiPct, avgSourceCount, avgActiveListings,
   avgSalesLast30d, avgPopulationAtBuy, coverage...}`.
   - `confidenceBucket` (review #5): **always a string** to avoid a union-valued
     JSON field — the numeric level as a string (`"2"`, `"3"`) or `"unknown"`
     when `cl_confidence_at_purchase` is NULL. No `*int`/union.
   - `buyTermsBucket` = `buyCost / cl_value_at_purchase_cents` (frozen CL only),
     5% bands; `<50%`→`"<50"`, `>=100%`→`">=100"`; frozen CL NULL/0 → `"unknown"`.
   - averages computed only over rows where that pointer is non-nil (NULL
     skipped; measured `0` included), each with a coverage count.
2. **`SplitPNL.ByReason map[string]PNLBlock`** keyed by the 5 reasons.
   `Discretionary`/`Forced` kept for back-compat.

### Frontend
- `web/src/types/campaigns/core.ts`: add new `Sale`/`Purchase` fields matching
  Go JSON tags (provenance fields optional/nullable).
- `RecordSaleForm` (single): add `sale_reason` `<select>` (sent on create,
  preserved server-side).
- `BulkRecordSaleModal`: add `sale_reason` `<select>` **and** the three
  listing-detail inputs as **per-item** fields; extend `BulkSaleInput`
  (`types_core.go:407`) + handler + `CreateBulkSales` to carry them per card.
  Partial-failure retries preserve per-item values because they live on the item.

### Docs
- `SCHEMA.md`: new columns (nullable note, CHECK, generated col).
- `API.md`: PATCH route, `BulkSaleInput` shape change, analysis additions.
  Document imported `*_at_sale` values as **record-time proxies** (review #9).

## Testing
- `ParseCLConfidenceMin` table (ranges/singles/junk → ok=false).
- **Split lifecycle:** confidence+population freeze at creation even for a
  `pending` (no-snapshot) purchase; market fields stay NULL until a successful
  snapshot.
- **Capture-success gate:** a purchase whose provider lookup fails leaves the 4
  market columns NULL (never a false 0); a successful capture with a real 0
  active-listings stores 0.
- **Pre-correction source count:** `SourceCountRaw` excludes the `cardladder`
  source that `applyCLCorrection` appends.
- Set-once: async first-enrichment and refresh-after-enrichment never overwrite a
  non-nil value; async fallback fills confidence/population only when NULL.
- `MarketSnapshotData.applySnapshot` round-trips Confidence/SourceCountRaw.
- `EffectiveChannelFeePct` parity with `CalculateSaleFee` across channels; fee
  pointer non-nil on new sales, NULL on legacy rows.
- Sale freeze across all 3 paths sets cl-at-sale, channel-fee, reason default;
  explicit creation reason is preserved, empty defaults to heuristic.
- Generated-column INSERT via `saleInsertColumns` succeeds; scan reads
  `forced_liquidation` back.
- **Analysis JOIN scan:** `saleColumnsAliased` + `scanPurchaseWithSale` return
  `sale_reason` so `ByReason` is populated (regression guard for review #4).
- `UpdateSaleReason`: rejects `""`, rejects sale not in campaign, reflects into
  generated boolean.
- Migration up/down round-trip incl. backfill rules + CHECK.
- Cohort buckets: `<50`/`>=100`/`unknown` boundaries; `confidenceBucket` always a
  string incl. `"unknown"`; buy% uses frozen CL not current.

## Explicit exclusions
- No historical backfill of provenance/listing fields (no source).
- No new campaign column for confidence.
- No frontend analysis-page types (no UI consumer).
- No change to `population` live-refresh (only the frozen counterpart protected).
- `source_count_at_purchase` is **external provider platform count
  (pre-CL-correction)**, explicitly not comp-sales count and not including the
  Card Ladder anchor.
