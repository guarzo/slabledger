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
    waiting for async enrichment would record post-decision values. Population
    freezes only when `> 0` (PSA bulk hardcodes 0 = unknown; review #1).
  - The 4 market fields freeze on **confirmed capture**, in two presence groups
    (review #2): `dh_confidence`/`source_count` when pricing succeeds;
    `active_listings`/`sales_last_30d` only when CardLookup market data was
    actually observed (`price.Market != nil`). Never on a silent provider
    failure.
  - **No async backfill** of the creation-time facts (review #3): pre-migration
    rows stay NULL forever. Reading *current* campaign/pop would be
    post-decision and contradicts "No backfill." New rows freeze them at creation.
- **Item 4 scope:** add per-item listing fields to `BulkRecordSaleModal`.
- **`sale_reason` UX (review #5):** creation **preserves** an explicit valid
  reason and defaults to the heuristic only when the field is empty. PATCH for
  later edits.
- **`forced_liquidation` back-compat:** stays a **plain app-maintained boolean**
  (NOT generated) so the documented image-only rollback stays safe — the previous
  binary INSERTs the column explicitly. The app + migration + PATCH keep it in
  sync with `sale_reason` (`forced == reason='invoice_pressure'`), and a
  `campaign_sales_derive_reason` BEFORE-INSERT/UPDATE trigger fills an empty
  `sale_reason` from the boolean so legacy-shaped writes during a rollback window
  are never lost. Operator setting a non-pressure reason flips the boolean false
  (correct semantic; changes the "Forced" P&L bucket).
- **Sale-side freeze applies to all three paths**, but for imports these are
  **record-time proxies**, not exact decision-time values (review #9).
- **Buy-term bucket (review #7):** `buyCost / cl_value_at_purchase_cents` (frozen
  CL, never current), 5% bands; `<50%` and `>=100%` each collapse to one bucket;
  missing frozen CL → explicit `unknown` bucket.
- **Analysis richness:** option (A) — full confidence×buy% cohort block now.

## § 1 — Migration `000022_add_decision_provenance`

Additive; set-once enforced in Go (INSERT/first-enrichment only).

### `campaign_purchases` — 6 frozen columns, **NULLABLE** (split freeze timing; see § 2)

| Column | Type | Source | Frozen when |
|---|---|---|---|
| `cl_confidence_at_purchase` | `SMALLINT` (NULL) | truncated min of `campaign.CLConfidence` | row creation |
| `population_at_purchase` | `BIGINT` (NULL) | `purchase.Population` (only if > 0) | row creation |
| `dh_confidence_at_purchase` | `DOUBLE PRECISION` (NULL) | snapshot `Confidence` | first successful capture |
| `source_count_at_purchase` | `BIGINT` (NULL) | `SourceCountRaw` = external platforms, pre-CL-correction | first successful capture |
| `active_listings_at_purchase` | `BIGINT` (NULL) | snapshot `ActiveListings` | first capture w/ market data observed |
| `sales_last_30d_at_purchase` | `BIGINT` (NULL) | snapshot `SalesLast30d` | first capture w/ market data observed |

`NULL` = never captured; a stored `0` = a real measured zero. No backfill.

### `campaign_sales` — 3 new columns + boolean→reason swap

| Column | Type | Notes |
|---|---|---|
| `sale_reason` | `TEXT NOT NULL DEFAULT ''` + `CHECK (sale_reason IN ('','discretionary','invoice_pressure','aging_policy','bulk_lot','show_clearout'))` | `''` = legacy/unknown, only via backfill/creation-default gaps |
| `cl_value_at_sale_cents` | `BIGINT NOT NULL DEFAULT 0` | frozen at sale (0 = unknown) |
| `channel_fee_pct_at_sale` | `DOUBLE PRECISION` (NULL) | frozen at sale; NULL=uncaptured (legacy), 0.0=real free channel |

### `forced_liquidation` back-compat sequence (up migration)

1. Add `sale_reason` (with CHECK over the 6 allowed values incl. `''`),
   `cl_value_at_sale_cents`, `channel_fee_pct_at_sale`.
2. Backfill `sale_reason` (derivable only):
   - `forced_liquidation = TRUE` → `invoice_pressure`
   - in-person channel (`inperson`/`local`/`cardshow`) AND `sale_price_cents <
     0.80 * cl_value_at_purchase_cents` (join purchase; only where
     `cl_value_at_purchase_cents > 0`) → `invoice_pressure` (~36 sales, ~$5.2K)
   - else → `discretionary`
3. Re-sync the existing `forced_liquidation` boolean:
   `UPDATE ... SET forced_liquidation = (sale_reason = 'invoice_pressure')`.
   The column is **kept as a plain writable boolean** — NOT dropped, NOT
   converted to generated — so the previous image (which INSERTs it explicitly)
   still works after an image-only rollback.
4. Create a `campaign_sales_derive_reason` BEFORE INSERT/UPDATE trigger that fills
   an empty `sale_reason` from `forced_liquidation`
   (`TRUE → 'invoice_pressure'`, else `'discretionary'`). This closes the
   rollback-window gap: if the old image inserts a sale with only
   `forced_liquidation` set, the trigger derives a non-empty reason so analysis
   never silently skips it. The trigger only fills *empty* reasons, so the new
   image's richer reasons are never clobbered.

**Down migration** reverses: drop the trigger + function, drop the 3 sale cols +
6 purchase cols; `forced_liquidation` is left in place (it predates 000022).

## § 2 — Go domain & adapter layer

### Types (`internal/domain/inventory/types_core.go`)
- Add `Confidence float64`, `SourceCountRaw int`, and `MarketDataObserved bool`
  to **`MarketSnapshotData`** (the shared struct persisted to columns) and set
  them in `applySnapshot`. Without this the async persist path drops the values
  (root cause). `SourceCountRaw` is the **pre-CL-correction** platform count (see
  the capture gate), distinct from `MarketSnapshot.SourceCount` which
  `applyCLCorrection` inflates with `cardladder`. `MarketDataObserved` = "`CardLookup`
  market data was present" — gates the Group-2 freeze so a nil `price.Market`
  doesn't store false zero listings/velocity (review #2).
- `Purchase`: add 6 provenance fields as **`*int`/`*float64` pointers** (nullable
  ⇔ NULL column) so measured-zero ≠ missing. `json:",omitempty"`.
- `Sale`: add `SaleReason string`, `CLValueAtSaleCents int`,
  `ChannelFeePctAtSale *float64` (nullable — legacy NULL vs real 0.0).
  `ForcedLiquidation bool` stays a **plain writable field**, kept in sync with
  `SaleReason` by the freeze helper (`forced == reason=='invoice_pressure'`).
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
- Set population from `p.Population` **only when `p.Population > 0`** (review #1):
  PSA bulk import hardcodes `Population: 0` (`service_import_psa.go:258`) because
  the export lacks it, and a certified card cannot genuinely have grade
  population 0 — so 0 here means "unknown," and freezing it would store
  missing-as-measured. Leave NULL when 0.
- These freeze even when the purchase is created `pending` (PSA bulk), because
  they don't depend on the price provider. This is the decision-time value;
  waiting for async enrichment would capture edited-campaign / refreshed-pop
  values instead (review #1).

**(b) Market facts — frozen on confirmed capture, in TWO presence groups
(review #2).** DH pricing can succeed from sales data while `CardLookup` fails
non-fatally (`dhprice/provider.go:138`), leaving `price.Market` nil — so a
general snapshot success does **not** mean listings/velocity were observed. Carry
a `MarketDataObserved bool` through `MarketSnapshot` → `MarketSnapshotData` (set
true only when `price.Market != nil`), and split the freeze:
- **Group 1 — `dh_confidence`, `source_count`:** freeze when the snapshot capture
  succeeds at all (pricing data present).
- **Group 2 — `active_listings`, `sales_last_30d`:** freeze **only when
  `MarketDataObserved`** is true; otherwise leave NULL (a real 0 listings is
  stored only when CardLookup actually returned market data).
- `captureMarketSnapshot`/`recaptureMarketSnapshotDetailed` must signal capture
  success (return a bool / the applied `*MarketSnapshot`, or gate on non-empty
  `SnapshotDate`) — copying fields after a silent provider-error `return` would
  store false zeros.
- Capture the **pre-correction** platform count: read `len(snapshot.Sources)`
  *before* `applyCLCorrection` appends `cardladder`, into `SourceCountRaw`
  (review #3). Both sync and async helpers call `applyCLCorrection`, so both must
  snapshot the count before that call.
- Sync path: freeze in `CreatePurchase` on success. Async path:
  `UpdatePurchaseMarketSnapshot` (`purchase_price_store.go:255`) sets each column
  **only when currently NULL** (`col = CASE WHEN col IS NULL THEN $n ELSE col
  END`), Group 2 additionally gated on `MarketDataObserved`. First successful
  writer wins.

**(c) No async backfill of creation-time facts (review #3).** Pre-migration rows
keep `cl_confidence_at_purchase`/`population_at_purchase` **NULL forever**. A
fallback that reads *current* campaign confidence or *current* population would
record post-decision values (both mutate: `UpdateCampaign`,
`UpdatePurchaseCLValue`) — invalid provenance, and contradicting the "No
backfill" exclusion. New rows already freeze (a) at creation; historical rows are
simply unknown.

### Persist (`purchase_store.go` + scan helpers)
- Extend `CreatePurchase` INSERT + `purchaseColumns`/`purchaseColumnsAliased` +
  `scanPurchase` (scan into `sql.Null*` → pointers).

### Freeze at sale-creation (all 3 paths)
- `sa.CLValueAtSaleCents = purchase.CLValueCents`
- `sa.ChannelFeePctAtSale`: compute `pct := EffectiveChannelFeePct(channel,
  campaign)` into a local, then assign `&pct` (a function result is not
  addressable in Go — review #4). Always captured on new sales → non-nil going
  forward. New helper extracted from `CalculateSaleFee` so fee cents and pct
  never diverge.
- `sa.SaleReason`: **preserve** a caller-supplied valid reason; reject a
  **non-empty invalid** reason with a validation error (`ValidSaleReason` allows
  only the 5 values + `""`); default to `invoice_pressure if
  IsForcedLiquidation(...) else discretionary` only when empty (review #4/#5).
- Set `ForcedLiquidation = (SaleReason == 'invoice_pressure')` in the same freeze
  helper (plain boolean, app-maintained — not generated).
- **Clear client-forged provenance:** the HTTP handler decodes the raw body into
  the domain struct, so `CLValueAtSaleCents`/`ChannelFeePctAtSale` are
  attacker-controllable. The freeze helper overwrites them unconditionally from
  server-derived values; never trust the request (review round-5 P1).

### Sale column persistence (no split needed)
Because `forced_liquidation` stays an ordinary writable boolean, a single
`saleColumns` list serves both INSERT and read — no `saleInsertColumns` split.
- Extend `saleColumns` with `sale_reason`, `cl_value_at_sale_cents`,
  `channel_fee_pct_at_sale`.
- **`saleColumnsAliased`** — the LEFT-JOIN list used by portfolio analysis
  (`analytics_store.go:190`) — currently has only `forced_liquidation` and none
  of the new fields. **Must** be extended with the same 3 columns, and
  `scanPurchaseWithSale` extended to scan them (`sql.Null*`), or
  `SplitPNL.ByReason` reads nothing (review #4).
- Update `sale_store.go` INSERT (add the 3 columns + `forced_liquidation` stays
  in the args) and placeholder count; `scanSale`/`scanPurchaseWithSale`.

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
- `SCHEMA.md`: new columns (nullable note, `sale_reason` CHECK), the
  `campaign_sales_derive_reason` trigger, and that `forced_liquidation` stays a
  plain app-maintained boolean.
- `OPERATIONS.md`: 000022 is rollback-safe (additive/nullable cols; boolean
  unchanged; trigger backfills reason for legacy-shaped writes).
- `API.md`: PATCH route, `CreateSaleInput.saleReason` + `BulkSaleInput` shape
  change, analysis additions. Document imported `*_at_sale` values as
  **record-time proxies** (review #9).

## Testing
- `ParseCLConfidenceMin` table (ranges/singles/junk → ok=false).
- **Split lifecycle:** confidence freezes at creation even for a `pending`
  (no-snapshot) purchase; population freezes at creation only when `> 0` (a PSA
  bulk row with `Population: 0` stays NULL); market fields stay NULL until a
  successful snapshot.
- **Capture-success + presence gates:** a purchase whose provider lookup fails
  leaves all 4 market columns NULL; a success where `price.Market` is nil
  (CardLookup failed) freezes `dh_confidence`/`source_count` but leaves
  `active_listings`/`sales_last_30d` NULL; a full success with real 0 listings
  stores 0.
- **No async backfill:** a pre-migration row with NULL confidence/population is
  left NULL by the enrichment worker (never reads current campaign/pop).
- **Pre-correction source count:** `SourceCountRaw` excludes the `cardladder`
  source that `applyCLCorrection` appends.
- Set-once: async first-enrichment and refresh-after-enrichment never overwrite a
  non-nil value; async fallback fills confidence/population only when NULL.
- `MarketSnapshotData.applySnapshot` round-trips Confidence/SourceCountRaw.
- `EffectiveChannelFeePct` parity with `CalculateSaleFee` across channels; fee
  pointer non-nil on new sales, NULL on legacy rows.
- Sale freeze across all 3 paths sets cl-at-sale, channel-fee, reason default;
  explicit creation reason is preserved, empty defaults to heuristic, a non-empty
  invalid reason is rejected with a validation error (review #4); client-forged
  cl-at-sale/channel-fee are overwritten server-side (round-5 P1).
- Sale INSERT writes the 3 new columns + the plain `forced_liquidation` boolean;
  scan reads them back. No column split.
- Rollback-window: a legacy-shaped INSERT (only `forced_liquidation`, no
  `sale_reason`) gets a derived reason from the `campaign_sales_derive_reason`
  trigger (round-5 P1).
- **Analysis JOIN scan:** `saleColumnsAliased` + `scanPurchaseWithSale` return
  `sale_reason` so `ByReason` is populated (regression guard for review #4).
- `UpdateSaleReason`: rejects `""`, rejects sale not in campaign, keeps the plain
  `forced_liquidation` boolean in sync.
- Migration up/down round-trip incl. backfill rules + CHECK + trigger.
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
