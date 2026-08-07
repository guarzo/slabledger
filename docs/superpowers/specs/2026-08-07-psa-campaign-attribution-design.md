# PSA Campaign Attribution

**Date:** 2026-08-07
**Status:** Design approved, pending implementation plan

## Problem

We infer which campaign made each purchase. PSA tells us directly, and we are
already storing the answer without reading it.

Attribution today runs through `FindMatchingCampaign` / `PurchaseMatchesCampaign`
(`internal/domain/inventory/matching.go:41`), which guesses from year range, grade
range, and price range scaled by `BuyTermsCLPct`. Against PSA's own answer it is
wrong roughly half the time: of the 45 PSA instant-offer purchases in production,
~22 agree with PSA and ~23 disagree. Twenty are misattributed *into* "Modern PSA
10", a campaign confirmed off the slate on 2026-08-01. That distorts per-campaign
P&L, fill rates, and tuning suggestions.

PSA's answer is the `adjusted_description` column on the Lightdash
`embed-itemized-purchases` tile — the same tile the harvester already reads. The
harvester persists tile rows raw into `psa_portal_snapshot.rows`
(`internal/adapters/clients/psaportal/harvester.go:108`), so the campaign name is
already in production data. Only `mapRow`
(`internal/adapters/clients/psaportal/mapper.go`) discards it, because it maps 11
named columns and `adjusted_description` is not among them.

## Scope

Verified against the live tile on 2026-08-07:

```
LIMIT: 1000   (returns 45)
filters:
  marketplace_listings_buyer_payment_date        notNull
  fct_instantoffers_offers_origination_source    equals [psa-grading-offer, psa-vault-offer]
  fct_instantoffers_offers_buyers_collectors_id  notEquals []
```

No date filter and no truncation: 45 rows is the **complete history** of our PSA
instant-offer purchases. The 2026-07-06 start date is simply when we began buying
via instant offers.

Coverage is therefore small but total within its domain, and grows with every
future offer purchase:

| `purchase_source`   | count | range                     |
|---------------------|-------|---------------------------|
| `(blank)`           |   764 | 2026-03-23 → 2026-07-31   |
| `Grading`           |   528 | 2026-03-10 → 2026-06-22   |
| `Vault`             |   241 | 2026-03-10 → 2026-06-22   |
| `psa-vault-offer`   |    23 | 2026-07-16 → 2026-08-06   |
| `psa-grading-offer` |    22 | 2026-07-06 → 2026-08-06   |

The 45 snapshot rows map 1:1 onto the 45 `psa-*-offer` purchases; zero snapshot
rows are unmatched. The other 1533 purchases were acquired outside the instant-offer
mechanism, so PSA has no campaign attribution for them and never will. Inference
remains the only option there.

## Decisions

**PSA is authoritative.** Where PSA gives an answer, it wins — for new imports and
retroactively for existing purchases. Inference becomes the fallback path.

**Reconciliation is automatic.** Every harvest reconciles; no proposal queue, no
approval step. This is safe only because we do not expect to override PSA. The
tradeoff accepted: a hand-assignment that contradicts PSA would be undone by the
next harvest.

**Unresolvable names keep their inferred attribution.** PSA gives a campaign *name*,
not an ID. Resolution requires the name to still exist in the portal, and
`psa_campaign_snapshot` is a current-state singleton with no history. Ten rows name
campaigns deleted in the 2026-07-27/28 band restructure:

```
Brady modern | 2026-07-25 | 149554568 | 10 |  461
Brady modern | 2026-07-25 | 163418950 | 10 | 1261
Brady modern | 2026-07-25 | 163418975 | 10 |  401
Brady modern | 2026-07-25 | 163418960 | 10 |  401
Brady modern | 2026-07-25 | 163418973 | 10 |  401
Brady modern | 2026-07-25 | 159746010 |  7 |  573
Brady modern | 2026-07-26 | 47273920  |  9 | 1212
Modern 10    | 2026-07-06 | 161722199 | 10 |  758
Modern 10    | 2026-07-06 | 152931233 | 10 |  377
Modern 8     | 2026-07-16 | 160987870 |  8 |  486
```

`psa_campaign_push_queue` shows the replacement portal campaigns `Modern`
(`3c8be401-…`) and `Modern High Band` (`7b7b73d2-…`) were both created 2026-07-28 —
after every purchase above. For these, `campaign_id` is left as inferred and the
disagreement is surfaced rather than resolved. Creating dormant historical campaign
records so these names resolve is a possible follow-up, explicitly out of scope here.

## Changes

### 1. Capture the field

`mapper.go`: add `colCampaignName = "adjusted_description"` and map it to a new
`PSACampaignName string` field on `PSAExportRow`
(`internal/domain/inventory/import_types.go:25`). Absent or empty values are
tolerated — it is a nullable dimension.

No harvester change, no new tile, no new authentication.

### 2. Migration (`000023_add_psa_campaign_attribution`)

Two nullable columns on `campaign_purchases`, no defaults:

- `psa_campaign_name TEXT` — the raw string from PSA, stored verbatim **always**,
  including when it does not resolve. This is the irreversible part: the portal
  deletes campaigns and our snapshot is current-state-only, so a name not captured
  at import is lost permanently.
- `attribution_source TEXT` — `psa` | `inferred` | `manual`. Without it,
  `campaign_id` holds an undifferentiated mix of ~45 PSA-truth and ~1533 inferred
  rows, and every consumer of per-campaign P&L reads that mix blind.

Unlike the decision-provenance columns from `000015`/`000022`, these are **not**
set-once. `psa_campaign_name` is refreshed on every harvest, because PSA is
authoritative.

**`attribution_source` lifecycle.** The column is only useful if it is never null in
steady state, so every write path must set it:

- **Legacy backfill (in the migration, not the first reconciliation run).** All
  existing rows are set to `inferred`. Attribution for every one of them came from
  `FindMatchingCampaign` or a hand assignment we can no longer distinguish, and
  `inferred` is the weaker, safer claim. Doing this in the migration rather than in
  reconciliation matters: reconciliation only visits the 45 snapshot rows, so a
  backfill deferred to it would leave 1533 rows null indefinitely.
- **Manual reassignment.** The existing path (`service_crud.go:321-334`,
  `repository_purchase.go:50-60`) must set `manual`. It does not today.
- **Non-PSA creation.** Any purchase created outside the PSA offer path defaults to
  `inferred`.
- **Reconciliation.** Sets `psa` on rows it resolves.

A null value after this ships means a write path was missed, and should be treated
as a defect rather than a valid state.

### 3. Resolution

Two hops: `adjusted_description` → `psa_campaign_snapshot` portal campaign →
`campaigns.psa_campaign_request_id` → our campaign. The indirection handles name
drift (portal `Crystal` → our `Crystal Pokemon`). Matching on the portal name is
case-insensitive and trimmed; anything beyond that is treated as unresolved rather
than guessed at.

**Ownership — this cannot live where the obvious reading puts it.** The portal
campaign snapshot port and types belong to `internal/domain/psacampaign`
(`repository.go:32-36`), and `psacampaign` already imports `inventory`
(`internal/domain/psacampaign/mapper.go:8`). Having `inventory` import `psacampaign`
to reach the snapshot would create an import cycle and violate the flat-sibling rule
that `scripts/check-imports.sh` enforces — `make check` would fail.

Instead:

- `inventory` declares its own narrow port, e.g.
  `PSACampaignResolver { ResolveCampaignID(ctx, psaName string) (string, bool, error) }`,
  alongside its existing repository interfaces. It depends on the interface only,
  and knows nothing about `psacampaign` or the snapshot table.
- The implementation lives in the adapter layer, where both the campaign snapshot
  store and the campaigns table are already reachable, and is injected via the
  established functional-option pattern (as `WithPriceLookup` does).

This keeps the two-hop lookup out of the domain entirely and preserves the
hexagonal invariant.

### 4. Import path

`service_import_psa.go:104`: if `PSACampaignName` resolves, use that campaign
directly and skip `FindMatchingCampaign` entirely; set `attribution_source = 'psa'`.
Otherwise fall back to the existing matcher unchanged, set
`attribution_source = 'inferred'`, and store the raw name either way.

### 5. Reconciliation

Runs at the end of each harvest, over the snapshot rows. For each row whose name
resolves, if `campaign_id` differs from the resolved campaign, move it and set
`attribution_source = 'psa'`. Emits one log line per run with the move count.

**Preconditions.** Reconciliation refuses to run unless **both** snapshots are
fresh. The existing 26h guard covers only the itemized-row snapshot
(`internal/adapters/clients/psaportal/snapshotprovider.go:53-65`);
`psa_campaign_snapshot` has no equivalent — its reader returns `fetched_at` without
validating it (`internal/adapters/storage/postgres/psa_campaign_snapshot_store.go:49-66`),
and a campaign-refresh failure is only logged while harvesting continues
(`cmd/psa-harvest/main.go:109-124`). Without a second guard, fresh purchase rows
would be resolved through a stale campaign list — names would silently fail to
resolve, or worse, resolve to a superseded campaign. Apply the same 26h bound to
`psa_campaign_snapshot.fetched_at` and skip the run (logging why) if either is
stale.

**Sold purchases are skipped.** `UpdatePurchaseCampaign` refuses reassignment when a
linked sale exists and returns `ErrPurchaseHasSale`
(`internal/adapters/storage/postgres/purchase_store.go:336-362`) — this is a
deliberate guard, not an obstacle to route around. Sales freeze their own
campaign-derived values (`sale_fee_cents`, `channel_fee_pct_at_sale`, and
`net_profit_cents`, which also depends on the purchase's `psa_sourcing_fee_cents` —
`service_crud.go:133-165`), and analytics reads stored `net_profit_cents` rather
than recomputing it (`internal/adapters/storage/postgres/analytics_store.go:25-49`).
Moving a sold purchase's campaign without repairing all of that would leave the
ledger internally inconsistent.

So: for a sold purchase where PSA disagrees, write `psa_campaign_name`, leave
`campaign_id` alone, and log the disagreement. Do not attempt the write and do not
treat `ErrPurchaseHasSale` as a run failure.

All 45 PSA offer purchases are currently unsold, so the first run encounters none of
these. This is steady-state behavior: as these cards sell, the skip path becomes the
common one, and without it an unattended harvest would start erroring every run.

Repairing sale-side financial provenance on reattribution is deliberately **out of
scope** — see Open follow-ups.

**Unresolved rows.** The claim that these "continue to flow through
`psa_pending_items` as they do today" is false, and the design must not rely on it:
existing purchases are routed to `handleExistingPSAPurchase` and skip matching
entirely (`service_import_psa.go:63-76`), and pending items are created only from
*new* rows classified ambiguous or unmatched (`service_import_psa.go:157-185`). No
existing code path enqueues an existing purchase.

Reconciliation must therefore do it explicitly:

- PSA names a campaign that does not resolve → enqueue a pending item carrying the
  raw name, if one does not already exist for that cert. Keep `campaign_id` as
  inferred, per the decision above.
- PSA later resolves a name that previously did not (the portal campaign reappears,
  or we add a historical record) → apply the move and resolve any stale pending row
  for that cert, so the queue does not accumulate items that are no longer open.
- PSA-resolved rows never enter the queue.

### 6. Campaign-derived frozen fields

This section applies only to **unsold** purchases; sold ones are skipped entirely
(§5). Two columns on the purchase are derived from the campaign at purchase time and
never recomputed:

- `cl_confidence_at_purchase` (`service_crud.go:77-79`, from `campaign.CLConfidence`)
- `psa_sourcing_fee_cents` (`service_import_psa.go:257`, from
  `campaign.PSASourcingFeeCents`)

When `campaign_id` moves, both describe a campaign that did not make the purchase.
Re-deriving is not clean either: a provenance column is meant to hold the value *as
of the purchase date*, and only the campaign's current config is readable. For
purchases predating the 2026-07-27/28 restructure, current config is not what was in
force.

Handled per-column, because the two have different characters:

- **`psa_sourcing_fee_cents` — re-derive** from the corrected campaign's current
  config. It is a stable PSA program parameter and it feeds cost basis directly;
  leaving it attached to the wrong campaign actively distorts P&L.
- **`cl_confidence_at_purchase` — re-derive only when provably safe, otherwise
  null.** The `campaigns` table carries only `created_at`/`updated_at`; there is no
  parameter history and no audit table, so we cannot ask "did `cl_confidence` change
  since this purchase?". The one sound predicate available is
  `purchase_date >= campaigns.updated_at` — the campaign has not been written at all
  since the purchase, so its current config is provably what was in force. Re-derive
  in that case; null otherwise.

  This is deliberately conservative and will null most of the moved rows, since
  `updated_at` bumps on any write including unrelated ones. That is the intended
  bias: the 8/15 buy-terms experiment turns on this column, and a fabricated
  anachronistic value corrupts it silently, whereas a null is visible. Recovering
  more rows requires real parameter history, which is a separate change.

### 7. Before-state snapshot

Before the first reconciliation writes anything, dump `(purchase_id, campaign_id,
attribution_source, cl_confidence_at_purchase, psa_sourcing_fee_cents)` for every
affected row to a CSV file.

This artifact is **untracked and operational, not committed**. `docs/private/` is
gitignored precisely because it holds sensitive local-only business data
(`.gitignore:90-91`), so "commit a CSV under `docs/private/`" is self-contradictory.
Write it to an untracked path outside the repository (or an explicitly ignored
`tmp/` path), keep it until the reattribution has been reviewed and the numbers
accepted, then delete it. It contains per-purchase cost and campaign data and should
not be shared or attached to a PR.

Historical per-campaign P&L will change: the 2026-07-27→07-31 ledger findings will
stop reproducing once ~20 purchases leave Modern PSA 10. This makes that diffable
rather than mysterious.

## Error handling

- **Missing/empty `adjusted_description`** — treat as "PSA has no answer". Fall back
  to inference. Not an error; the dimension is nullable.
- **Name present but unresolvable** — expected steady-state condition (the ten dead
  names), not a failure. Store the name, keep the inferred `campaign_id`, enqueue a
  pending item per §5, and log at info with the unresolved name so new portal-side
  renames are visible.
- **Resolved to a campaign, but the purchase cert is unknown to us** — cannot occur
  today (zero unmatched snapshot rows) but must not panic; log and skip.
- **Purchase has a linked sale** — skip per §5. `ErrPurchaseHasSale` from a write we
  chose not to guard against is a defect, not an expected error: reconciliation
  checks for the sale first rather than attempting the update and catching failure.
- **Either snapshot stale** — the 26h bound applies to both the itemized-row snapshot
  and `psa_campaign_snapshot` (§5). Reconciliation skips the run and logs which
  snapshot was stale; it must never reattribute on old data.
- **Reconciliation partially fails** — per-row failures are logged and skipped; a
  failure must not abort the harvest or leave `campaign_id` and `attribution_source`
  inconsistent. Each row's updates apply together.

## Testing

- `mapper_test.go`: table-driven, covering `adjusted_description` present, absent,
  empty, and null-shaped.
- Resolution: table-driven over exact match, case/whitespace drift, name-drift via
  `psa_campaign_request_id`, and unresolvable name. Exercised through the
  `PSACampaignResolver` port with a mock, per §3. Uses `internal/testutil/mocks`;
  no inline mocks.
- Import path: PSA name resolves → matcher not called, `attribution_source = 'psa'`;
  unresolvable → matcher called, `attribution_source = 'inferred'`, name still
  stored.
- Reconciliation: agreement → no write; disagreement → `campaign_id` moves and both
  frozen fields are handled per §6; unresolvable → no move, name written, pending
  item enqueued; previously-unresolvable now resolving → move applied and stale
  pending item resolved.
- Sold purchases: disagreement on a purchase with a linked sale → no campaign write,
  name recorded, no error surfaced to the harvest.
- Staleness: stale itemized snapshot → skipped; stale `psa_campaign_snapshot` →
  skipped; both fresh → runs.
- Frozen fields: explicit cases for `purchase_date >= campaigns.updated_at`
  (re-derive) and `purchase_date < campaigns.updated_at` (null), for both columns.
- `attribution_source`: migration backfills legacy rows to `inferred`; manual
  reassignment sets `manual`.
- Migration up/down applies cleanly against local Postgres.
- `go test -race ./...` and `make check` before any completion claim. `make check`
  is load-bearing here: it runs `scripts/check-imports.sh`, which is what catches a
  regression of the §3 import-cycle boundary.

## Verification

The first reconciliation run should move ~23 of 45 rows, ~20 of them out of Modern
PSA 10, and leave exactly 10 rows unresolved with their names recorded. Numbers
materially different from that mean the resolution logic is wrong.

All 45 PSA offer purchases are currently unsold, so the first run should skip zero
rows for the sale guard. A non-zero skip count on the first run means the sale-state
check is wrong.

After the migration, no `campaign_purchases` row should have a null
`attribution_source`.

Most moved rows are expected to have `cl_confidence_at_purchase` nulled per §6.
Record the actual re-derived/nulled split in the before-state artifact so the impact
on the 8/15 experiment's sample size is known rather than discovered later.

## Open follow-ups

- **Campaign parameter history.** §6 nulls `cl_confidence_at_purchase` for most
  moved rows purely because no history exists to prove what was in force. Recording
  parameter changes would recover those rows and would benefit any future
  decision-provenance work. Not required for this change.
- **Sale-side provenance repair on reattribution.** §5 skips sold purchases because
  correcting one means recomputing `sale_fee_cents`, `channel_fee_pct_at_sale`, and
  the stored `net_profit_cents` that analytics reads directly. That is a financial
  data migration in its own right and needs its own design. Until it exists, sold
  purchases keep their inferred campaign permanently — the skip is a deferral, not a
  fix, and the population it affects grows as inventory sells.

## Out of scope

- Backfilling dormant campaign records so the ten dead names resolve.
- Any attribution for the 1533 non-instant-offer purchases.
- Repairing campaign attribution on purchases that already have a linked sale.
- Reconciling PSA's aggregate `embed-campaign-name-items-purchased` tile against our
  counts. That tile is grain (campaign, day) → count with no cert number, so it can
  cross-check totals but can never attribute a purchase.
- A review/approval UI for reattribution.
