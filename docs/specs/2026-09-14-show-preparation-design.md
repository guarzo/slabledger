# Card-show preparation: price support and saved shortlists

**Date:** 2026-09-14

**Status:** Workflow approved in conversation; written specification awaiting user review.

**Scope:** Inventory price-support filtering plus saved show shortlists and packing checks.

**Repository baseline:** `fe8ad267` (`fix(skills): load campaign analysis token file (#700)`).

## Goal

Help the operator choose which physical slabs to bring to a show by answering:
**Do recent matching sales support this card's existing listed price?**

The workflow is:

**Filter inventory → inspect sale evidence → select slabs → save a show shortlist → pack.**

Price evidence informs selection; it does not automatically decide what to bring.

## Approved decisions

- Deliver both an inventory filter and saved, named show shortlists.
- Evaluate the last-synced DH listing price, not a new show-specific price.
- Recent means 30 days. Support means a median at least 90% of listed price.
- At least two matching sales are required for `Supported`; one qualifying sale is
  `Thin evidence`.
- Use all eligible matching sales in the window, not only favorable sales.
- Show individual sale prices, dates, platforms, and source links where available.
- Start show selection with received, unsold inventory.
- Permit deliberate selection of cards with weak or missing evidence.
- Save per-slab membership and packing checks. Flag unavailable cards and changes
  to listing price or support; never silently remove selections on refresh.
- Selection and packing never reprice, delist, reserve, or sell inventory.

The operational details below are proposed conservative defaults for written review.

## Existing foundations and constraints

| Evidence | Design consequence |
|---|---|
| `web/src/react/pages/GlobalInventoryPage.tsx` renders the existing `InventoryTab` and counts `purchase.receivedAt`. | Extend the current global inventory flow; do not build a second inventory browser. |
| `web/src/react/pages/campaign-detail/inventory/useInventorySelection.ts` keeps selection in component state. | Reuse transient selection, but persist show membership separately. |
| `internal/domain/inventory/core_types.go` exposes `DHListingPriceCents` and `DHLastSyncedAt`. | Use the synced listing value and display synchronization time. |
| `internal/domain/inventory/listing_price.go` resolves the latest operator-committed reviewed/override price, deliberately excluding CL valuation. | Use this only to detect a local-versus-DH mismatch, not as a fallback evaluation price. |
| `internal/adapters/storage/postgres/cl_sales_store.go` calculates current summary counts and medians over 90 days. | Do not relabel or change existing `compSummary`; introduce an independent 30-day evaluation. |
| `internal/adapters/scheduler/cardladder_gap_fill.go` fetches page 0 with limit 100 and hardcodes PSA. | Existing stored history is not proof of complete coverage or correct non-PSA matching. |
| `internal/adapters/clients/cardladder/client.go` supports paged sales queries filtered by profile, condition, and grader. | Reuse the client, but verify pagination/order semantics before certifying a complete window. |
| `internal/adapters/clients/cardladder/types.go` exposes sale IDs, dates, prices, profile, condition, grader, platforms, and URLs. | Individual evidence can be retained and independently checked. |
| `internal/domain/inventory/comp_summary.go` lacks completeness and refresh metadata. | Aggregate-only CL/DH summaries cannot satisfy the new evidence contract. |
| `internal/README.md` enforces flat inventory siblings and inward dependencies. | Keep the new workflow behind domain-owned ports; no domain imports of adapters or other inventory siblings. |

A read-only production `/api/inventory` check confirmed that inventory already
returns purchase pricing fields and comp summaries. It did not validate complete
30-day sale coverage, individual upstream sale accuracy, or all variant mappings.

## Price and evidence rules

### Price source

Use the purchase's positive `DHListingPriceCents` as reported by the latest stored
DH synchronization. Label it **DH listed price**, with `DHLastSyncedAt` where known.
It is the last reported price, not a claim that every downstream marketplace is
currently active or priced identically.

A missing/non-positive DH price produces `No listed price`. Do not substitute a
reviewed price, override, recommendation, cost basis, or CardLadder valuation.
If the operator-committed price differs, show a non-blocking mismatch warning.
Missing synchronization time is displayed as unknown, not replaced with today.

### Comparable-sale identity and window

Match the exact card variant, grading company, and grade. Variant identity must
cover set, number, language/edition, and variation where applicable. Names alone
and adjacent-grade prices are insufficient. An unresolved mapping produces
`Needs review`, not a guessed match.

For v1, use detailed CardLadder sold records through the existing client. Do not
merge aggregate-only DH fallback summaries into this calculation. Do not infer
non-PSA coverage from the existing PSA-only refresh; unsupported identity/grader
combinations remain `Needs review` until verified detailed coverage exists.

Because the current sale records retain date-only values, define the window as
30 UTC calendar dates including the evaluation date: `D - 29` through `D`,
inclusive. Display the actual date range. Exclude future-dated records.

Include every valid matching sale in that window from the selected source,
regardless of whether it supports the price. Deduplicate repeated source sale
IDs within the same identity; two owned copies of a card do not multiply its
market-sale count. Do not remove low prices as statistical outliers. Invalid
prices/dates or identity contradictions must be disclosed and prevent treating
an affected, unresolved window as complete.

Use source-reported sold amounts, not asking prices or seller net proceeds.
Currency and the source's treatment of accepted offers/shipping must be verified
before an adapter certifies the evidence as comparable USD sale prices. Unknown
semantics produce `Needs review`; this feature does not invent conversion rules
or promise independently verified transaction settlement.

### Status evaluation

Apply status precedence in this order:

| Status | Rule |
|---|---|
| `No listed price` | No positive DH listing price. |
| `Needs review` | Matching, source availability, freshness, or window completeness is not established. Include a specific reason. |
| `No recent comps` | A successfully completed, current lookup found zero eligible sales in the window. |
| `Below target` | At least one sale exists, and the median is below 90% of DH listed price. |
| `Thin evidence` | Exactly one sale exists, at or above 90% of DH listed price. |
| `Supported` | At least two sales exist, and their median is at or above 90% of DH listed price. |

`Supported` describes observed price evidence, not a guarantee of demand, margin,
or a sale at the show. Sales above listed price count as support.

Keep stored money in integer USD cents. For an even number of comps, compare the
exact average of the two middle prices without rounding first. Round only the
displayed median. For example, a $300 listing passes at a $270 median and fails
at $269.99. A $400 comp is not excluded for exceeding the listing price.

### Completeness and freshness

A complete result means complete **for the queried source, identity, and date
window**, not every marketplace sale in existence. Persist successful-fetch time,
covered date range, source, match identity, and completion/error state, including
successful empty results.

Fetch pages until source exhaustion or until a verified newest-first traversal
covers the entire window. Merely fetching 100 rows is not sufficient. If ordering,
pagination, a rate limit, or a request budget prevents establishing coverage,
return `Needs review` with the available evidence clearly marked partial.

Proposed v1 freshness policy: a complete successful refresh must be no older than
24 hours and cover the current evaluation date range. A date rollover, failed
recheck, or partial replacement leaves prior evidence visible but marked for
review. Refresh is an explicit, bounded operation; opening inventory does not
launch an unbounded upstream scan. Deduplicate work by source identity, not by
owned slab, and retain current client retry/rate-limit conventions.

## User experience

### Inventory

Add a **Price support** filter and compact evidence fields alongside existing
inventory information: DH listed price, 30-day median, sale count, latest sale,
and support status. The expanded row shows the eligible individual sales and
source links, as well as date range, refresh time, matching details, and warnings.

The new support filter must intersect with search and other active inventory
filters, even where current search paths bypass legacy tabs. Do not refactor
unrelated inventory filtering. Counts must describe the actual filtered set.
Pending evaluations are shown as loading, not classified as missing comps.

Entering show selection defaults to received and unsold purchases. The operator
can include unreceived unsold cards as planning candidates, but these are marked
`Not received` and cannot be packed. General inventory browsing is not otherwise
restricted by this feature.

Reuse the existing selection controls for **Add to show shortlist**, allowing
creation of a named list or selection of an existing one. Add explicit selected
purchase IDs, not an implicit server-side interpretation of the current filter.
Changing filters must not silently add or remove saved members.

### Saved shortlist

Provide a compact saved-list view reachable from inventory. Each row contains
card identity, cert, DH listed price, shared evidence status/details, and an
explicit **Packed** checkbox. Display total members, packed count, unavailable
count, and the current known listed value of available members. Report missing
prices separately; do not substitute zero as a known value or reuse CL-valued
selection totals.

Support creating/naming a list, adding/removing members, and setting packed state.
The same slab may belong to multiple planning lists; membership is not a
reservation. Adding the same slab to the same list is idempotent.

Store the last operator-acknowledged price and support status as change-detection
snapshots. These are not editable show prices and never drive current support
classification. Adding a member or explicitly acknowledging its changes updates
the baseline; packing also acknowledges the displayed price/evidence. If a newer
price/evidence version arrived in the meantime, require review rather than
acknowledging unseen data.

On reopen or recheck, compare current data with that baseline. Show **Price changed**
or **Support changed** without clearing membership or packed history. In particular,
a price change after packing prompts the operator to check the physical sticker.
Do not flag a change solely because a refresh timestamp advanced.

A sold, deleted, or otherwise unavailable purchase remains visible as unavailable,
with its saved identity and packing history. Exclude it from available listed-value
totals and reject new pack actions. Removing a row is always an explicit action.
An existing unavailable row may still be unpacked or removed.

Use existing product styling and inline expansion, not a new visual system.
Status labels must work without color. Packing controls and evidence disclosure
must be keyboard accessible and usable on the existing mobile layout.

## Architecture and persistence

Introduce a focused `showprep` domain sibling owning the support evaluator and
shortlist lifecycle. It may depend on the inventory hub and leaf packages, but
not on `liquidation`, `export`, or other inventory siblings. Define narrow ports
for reading purchase/availability data, detailed evidence, and saved lists.
Compose adapters and service wiring in the application layer.

Both inventory and shortlist views request the same server-side evaluator.
Add authenticated batch-summary and lazy evidence-detail operations rather than
expanding or changing the meaning of existing 90-day `compSummary` fields.
Shortlist operations cover create/list/read, membership add/remove, explicit
packed-state updates, and acknowledgment of observed changes. A bounded refresh
operation reports partial failures without rewriting prices or purchases.

Summary and evidence-detail responses identify the evaluation date, evidence
version, and DH price used. A detail fetch after data changes returns the updated
summary too, so the user is not shown a green status for a different set of sales.
Browser reads must remain usable when evidence fetching fails.

Use additive PostgreSQL migrations for logical records:

- **Show list:** stable ID, name, created/updated timestamps.
- **Show list item:** list ID, stable membership ID, original purchase reference,
  saved card/cert identity, added time, packed time, acknowledged price/status,
  and a version for stale-update detection. Enforce unique membership per
  list/purchase. Preserve the item snapshot when its purchase disappears.
- **Verified evidence snapshot:** full source/profile/grader/grade identity,
  covered date range, refresh/completion state, and deduplicated individual sale
  records with provenance. Publish a complete generation atomically; never expose
  a partially replaced generation as complete. Successful empty snapshots must
  be representable.

Do not certify legacy comp rows lacking grader/coverage metadata by backfilling
assumptions. A separate verified snapshot keeps existing CL tables and 90-day
analytics behavior intact. Share provider access and fetch coordination where
practical; do not introduce a second provider integration or silently increase
unbounded scheduled work.

Keep lists within the existing shared authenticated ledger; no new tenant or
public-sharing model. Validate IDs, non-empty bounded names, batch limits, and
membership/availability at the server boundary. Packing and acknowledgment use
explicit values plus stale-version checks, not non-idempotent toggles. New pack
actions must atomically validate current receipt/unsold state and save the packing
update, rather than relying on the browser's earlier inventory read. A sale
committed later still marks the retained member unavailable on the next read.

Apply the repository's service-role-only RLS/revoke migration pattern, including
guards for roles absent from local PostgreSQL. No public evidence or shortlist
endpoints. Treat source URLs as untrusted links, allow only HTTP(S), and render
provider text as text rather than HTML. Keep secrets out of persisted evidence
and logs.

## Error handling and observability

A failed comp lookup must not turn a card into `No recent comps` or erase previous
evidence. Partial batch evaluation must identify which cards are pending/failed;
filter counts and select-all behavior must not pretend those cards were evaluated.
A failed shortlist save retains the user's transient selection for retry.

Report refresh counts, complete/partial/failed identities, rate limiting, and
elapsed time through existing structured logging. Expose per-card data-quality
reasons in the UI; do not require logs to explain why a card needs review.

## Non-goals

- Automated packing rankings, margin optimization, popularity scoring, or carrying limits.
- New show prices, repricing, price publication, delisting, reservations, or sale recording.
- Changing existing 90-day analytics, acquisition/campaign rules, or liquidation pricing.
- Manually editing/excluding unfavorable comps, substituting adjacent grades, or
  fabricating prices from missing evidence.
- New sales-data providers, general grader expansion, exports/print templates,
  offline synchronization, event scheduling, or public sharing in v1.

## Verification and acceptance

Before implementation, exercise the source client's live pagination/order and
sale-field semantics on bounded samples. If a source assumption fails, preserve
`Needs review` and revisit the adapter design; do not weaken the approved rule.
No tests have yet established those external guarantees.

Implementation acceptance checks:

1. Table-driven evaluator tests cover all statuses, precedence, exactly 90%, one
   cent below, above-list comps, odd/even medians, zero price, missing data, and
   injected-clock date boundaries (including future dates).
2. Adapter tests cover exact variant/grader/grade checks, duplicate IDs, more than
   100 sales, repeated/missing pages, pagination exhaustion, empty successful
   lookups, malformed amounts/dates, stale snapshots, and partial/failed refreshes.
3. PostgreSQL tests cover unique list membership, persistence across reload,
   snapshot publication, optimistic conflicts, removed purchases, and sold-state
   races. Verify migration security with and without Supabase roles present.
4. API tests cover authentication, input/batch validation, coherent summary/detail
   versions, bounded refresh results, unavailable packing, and idempotent retries.
5. Frontend tests cover intersecting filters/counts, lazy evidence disclosure,
   partial loading, saved selection, no-price totals, packing persistence, and
   changed-price/support and sold-card warnings. Verify keyboard and mobile use.
6. Explicitly prove that shortlist and refresh operations cannot mutate listing
   prices, listing status, reservations, or sales.
7. Run `go test -race -timeout 10m ./...`, applicable PostgreSQL integration tests,
   frontend tests/typecheck/build, and `make check`; inspect the final diff and
   exercise the complete filter → shortlist → pack → recheck workflow.

This document is a design, not an implementation plan. Implementation planning
starts only after the operator reviews and approves this written specification.
