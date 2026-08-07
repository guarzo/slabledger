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

Backfilled by the first reconciliation run.

### 3. Resolution

Two hops: `adjusted_description` → `psa_campaign_snapshot` portal campaign →
`campaigns.psa_campaign_request_id` → our campaign. The indirection handles name
drift (portal `Crystal` → our `Crystal Pokemon`). Matching on the portal name is
case-insensitive and trimmed; anything beyond that is treated as unresolved rather
than guessed at.

### 4. Import path

`service_import_psa.go:104`: if `PSACampaignName` resolves, use that campaign
directly and skip `FindMatchingCampaign` entirely; set `attribution_source = 'psa'`.
Otherwise fall back to the existing matcher unchanged, set
`attribution_source = 'inferred'`, and store the raw name either way.

### 5. Reconciliation

Runs at the end of each harvest, over the snapshot rows. For each row whose name
resolves, if `campaign_id` differs from the resolved campaign, move it. Emits one
log line per run with the move count.

Rows PSA cannot resolve are untouched and continue to flow through
`psa_pending_items` as they do today. PSA-resolved rows never enter that queue.

### 6. Campaign-derived frozen fields

Two columns are derived from the campaign at purchase time and never recomputed:

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
cl_confidence_at_purchase, psa_sourcing_fee_cents)` for every affected row to a
committed CSV artifact under `docs/private/`.

Historical per-campaign P&L will change: the 2026-07-27→07-31 ledger findings will
stop reproducing once ~20 purchases leave Modern PSA 10. This makes that diffable
rather than mysterious.

## Error handling

- **Missing/empty `adjusted_description`** — treat as "PSA has no answer". Fall back
  to inference. Not an error; the dimension is nullable.
- **Name present but unresolvable** — expected steady-state condition (the ten dead
  names), not a failure. Store the name, keep the inferred `campaign_id`, log at
  info with the unresolved name so new portal-side renames are visible.
- **Resolved to a campaign, but the purchase cert is unknown to us** — cannot occur
  today (zero unmatched snapshot rows) but must not panic; log and skip.
- **Snapshot stale** — the existing 26h staleness guard governs. Reconciliation must
  not run against a stale snapshot and silently reattribute on old data.
- **Reconciliation partially fails** — per-row failures are logged and skipped; a
  failure must not abort the harvest or leave `campaign_id` and `attribution_source`
  inconsistent. Each row's updates apply together.

## Testing

- `mapper_test.go`: table-driven, covering `adjusted_description` present, absent,
  empty, and null-shaped.
- Resolution: table-driven over exact match, case/whitespace drift, name-drift via
  `psa_campaign_request_id`, and unresolvable name. Uses
  `internal/testutil/mocks`; no inline mocks.
- Import path: PSA name resolves → matcher not called, `attribution_source = 'psa'`;
  unresolvable → matcher called, `attribution_source = 'inferred'`, name still
  stored.
- Reconciliation: agreement → no write; disagreement → `campaign_id` moves and both
  frozen fields are handled per §6; unresolvable → no move, name written.
- Frozen fields: explicit cases for `purchase_date >= campaigns.updated_at`
  (re-derive) and `purchase_date < campaigns.updated_at` (null), for both columns.
- Migration up/down applies cleanly against local Postgres.
- `go test -race ./...` and `make check` before any completion claim.

## Verification

The first reconciliation run should move ~23 of 45 rows, ~20 of them out of Modern
PSA 10, and leave exactly 10 rows unresolved with their names recorded. Numbers
materially different from that mean the resolution logic is wrong.

Most moved rows are expected to have `cl_confidence_at_purchase` nulled per §6.
Record the actual re-derived/nulled split in the before-state artifact so the impact
on the 8/15 experiment's sample size is known rather than discovered later.

## Open follow-ups

- **Campaign parameter history.** §6 nulls `cl_confidence_at_purchase` for most
  moved rows purely because no history exists to prove what was in force. Recording
  parameter changes would recover those rows and would benefit any future
  decision-provenance work. Not required for this change.

## Out of scope

- Backfilling dormant campaign records so the ten dead names resolve.
- Any attribution for the 1533 non-instant-offer purchases.
- Reconciling PSA's aggregate `embed-campaign-name-items-purchased` tile against our
  counts. That tile is grain (campaign, day) → count with no cert number, so it can
  cross-check totals but can never attribute a purchase.
- A review/approval UI for reattribution.
