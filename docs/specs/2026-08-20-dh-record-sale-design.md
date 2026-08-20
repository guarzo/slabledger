# DH sale recording: replacing the broken `sold` status transition

Status: approved design, not yet implemented
Date: 2026-08-20

## Problem

`InventoryAdapter.MarkInventorySold` sends `PATCH /inventory/:id {"status":"sold"}`.
DH rejects this:

```
HTTP 422 {"error":"Invalid status 'sold'. Must be one of: in_stock, listed"}
```

`sold` is a local-only value (`inventory.DHStatusSold`, `core_types.go`). DH's
inventory vocabulary has only `in_stock` and `listed`. The call has therefore
never succeeded, from either caller:

- the sale paths, via `notifyDHSold` (`service_crud.go`), called by both
  `CreateSale` and `CreateBulkSales`
- the hourly `dh-sold-reconciler` DH sweep

On 2026-08-15, 25 cards sold in person stayed live on DH, eBay and Shopify for
four days. The sweep found all 25 correctly every cycle and failed every write
(`retired=0 failed=25`). Local bookkeeping was correct throughout; only the
DH-side write was broken.

Those 25 were mitigated out-of-band with a throwaway `cmd/dh-delist`
(delist-channels + `PATCH in_stock`). They are off-market but carry no DH-side
sale record. The root cause is untouched: the next in-person or bulk sale
drifts identically.

DH has since shipped a purpose-built endpoint. This design adopts it.

## The DH contract

```
POST /api/v1/enterprise/inventory/:id/sale
Idempotency-Key: <required, 1..255>
{ "sale_price_cents": 45000,   // required, PER UNIT
  "quantity": 1,               // optional, default 1, clamped to qty held
  "sold_at": "2026-08-15T14:22:00Z",  // optional, default now
  "counterparty_name": "...",  // optional, max 255
  "notes": "..." }             // optional, max 2000

POST /api/v1/enterprise/sales/:sale_id/void
{ "reason": "..." }            // optional, truncated at 2000
```

Schema: `docs/gitbook-api/.gitbook/assets/enterprise-api.yaml` (v1.3.0) in the
`double-holo-ui` repo, operations `recordInventorySale` and `voidRecordedSale`.

### Contract hazards, and why most do not apply to us

DH documents six traps. Three are inapplicable **because every SlabLedger
purchase is a single graded slab**: `Purchase` has no quantity field, DH's
inventory rows return no `quantity` field, and cert numbers are unique per slab.
Quantity is therefore always 1, every sale is a full sale, and
`sold_inventory_id == dh_inventory_id`.

| Hazard | Applies? | Handling |
|---|---|---|
| `sale_price_cents` is per-unit; `realized_profit_cents` is a total | Degenerate at qty 1 | Never derive one from the other regardless |
| Partial sale returns a NEW `sold_inventory_id` row | No (qty always 1) | Read the field, never assume it equals the addressed id |
| `item_status` describes the ADDRESSED row, read live | Yes | Treat as an open-ended string, never an enum |
| `delisted` means "no live DH ask remains", true even if never listed | Yes | Read it; never infer from quantity |
| Partial sale may CANCEL rather than shrink an ask | No (qty always 1) | n/a |
| `channels` can be non-empty while `delisted` is true | Yes | Ignore `channels` for liveness decisions |

We rely on qty-1 for *scope*, not for *correctness*: the response is parsed by
reading the fields DH returns, so a future multi-unit row degrades to a logged
conflict rather than silent corruption.

## Design

### 1. Interface

`DHSoldNotifier` cannot express a sale. Replace it in `domain/inventory`:

```go
type DHSaleRecorder interface {
	RecordInventorySale(ctx context.Context, req DHSaleRequest) (*DHSaleResult, error)
	VoidInventorySale(ctx context.Context, dhSaleID, reason string) error
}

type DHSaleRequest struct {
	DHInventoryID    int
	IdempotencyKey   string
	SalePriceCents   int // PER UNIT
	SoldAt           time.Time
	CounterpartyName string
	Notes            string
}

type DHSaleResult struct {
	DHSaleID            string
	SoldInventoryID     *int   // nullable: null when replaying a voided sale's key
	Delisted            bool
	ItemStatus          string // open-ended by contract; NOT an enum
	Replayed            bool
	RealizedProfitCents int    // TOTAL across all units, never per-unit
}
```

Consumers to update: `service.go` (option + field), `csvimport/service.go`,
`dhlisting/adapter.go`, `scheduler/dh_sold_reconciler.go`, `scheduler/builder*.go`,
`cmd/slabledger/init_inventory_services.go`, `init_schedulers.go`.

### 2. Idempotency

**Key = `"slabledger-sale-" + <server-generated UUID>`, persisted on the sale row
as `dh_idempotency_key`.**

An earlier draft derived the key from `sale.ID`. That was unsound: `CreateSale`
only fills a *blank* id (`service_crud.go:290`) and the HTTP handler decodes
client input straight into a `Sale` (`campaigns_purchases.go:110`), so a client
can supply its own id — including one belonging to a sale that was later
un-sold and deleted (`sale_store.go:167`), and including one longer than DH's
255-character limit. A client-controllable idempotency key is a client-triggerable
double-disposal.

Generating the key server-side and persisting it keeps the property that made
the derived design attractive — the sale path and the sweep read the *same* key
for the same sale, so a sweep call after a successful `CreateSale` replays
(`replayed: true`) rather than disposing inventory twice — while grounding it in
a column we control rather than a field the client can set. It also changes no
existing API contract; client-supplied sale ids keep working and simply stop
being load-bearing.

**Ordering rule: persist the key before the DH call.** Recording first and
persisting after leaves a successful remote mutation with no key to replay.

Void is handled as before: un-sell deletes the `campaign_sales` row, taking its
key with it, so a re-sale draws a fresh row and a fresh key. A voided key is
never reused, which the contract requires.

**Corollary rule: the request body MUST be a pure function of persisted columns.**
Nothing in it may read the wall clock. If a first attempt built the body from
`now` and a retry rebuilt it from a later `now`, the fingerprint would change
under a fixed key and DH would answer `422 idempotency_key_reused`, stranding
that sale permanently.

`sold_at` is therefore derived as:

```
sold_at = clamp(saleDate, lower = purchaseDate, upper = sale.CreatedAt)
```

- `saleDate` = `Sale.SaleDate` parsed as `YYYY-MM-DD`; **on parse failure, fall
  back to `sale.CreatedAt`.** Neither `ValidateSale` nor `ValidatePurchase`
  enforces date *shape* — both check only non-emptiness
  (`validation.go:216`, `validation.go:223`) — and `CreateSale` silently skips
  its date logic when parsing fails (`service_crud.go:296`). Malformed dates
  therefore already exist in the table and can still be written today.
- `purchaseDate` = `Purchase.PurchaseDate` parsed the same way; **on parse
  failure, omit the lower clamp** and let DH validate.
- The upper bound is `sale.CreatedAt`, **not `now`** — `CreatedAt` is persisted
  and stable across retries, which `now` is not.

Every input is a stored column, so the body is byte-identical on every retry.


### 3. Error taxonomy

Domain sentinels in `domain/inventory`. **Only two are retryable**, both
re-issued byte-identical:

| DH response | Sentinel | Action |
|---|---|---|
| 409 `item_sold_on_channel` | `ErrDHItemSoldOnChannel` (nullable `SoldAt`, `Channel`) | flag for review |
| 409 `item_unavailable` | `ErrDHItemUnavailable` | flag |
| 409 `idempotency_in_progress` | `ErrDHIdempotencyInProgress` | **retry identical** |
| 409 `reversal_would_collide` (void) | `ErrDHReversalWouldCollide` | flag |
| 422 `idempotency_key_reused` | `ErrDHIdempotencyKeyReused` | flag, never retry |
| 422 (code null/absent) | `ErrDHValidation` | flag |
| 503 `lock_contention` | `ErrDHLockContention` | **retry identical** |
| 404 (void only) | `ErrDHSaleNotFound` | treat as success, log |

DH validates only *successful* responses against its schema, so error bodies are
documented but not machine-checked on their side. Parsing is defensive: an
unknown or missing `code` degrades to a generic error chosen by status class,
and unexpected extra or absent fields never hard-fail.

`404` on void covers not-found, another account's sale, a DH marketplace-mirror
deal, and a UI-created deal — none of which we can void and none of which should
fail an un-sell the user already performed locally.

### 4. Conflict flagging

Any non-retryable error, plus `delisted == false` on an apparent success,
records a conflict on the purchase for human review. We never auto-create a
`campaign_sales` row from a `409 item_sold_on_channel`: DH gives no sale price,
and both `sold_at` and `channel` can be null, so synthesising a row would invent
P&L figures.

`delisted == false` as a conflict is an inference, not a contract requirement.
It means an ask may still be live — the exact failure mode of this incident — so
it is worth surfacing even at the cost of some noise. Revisit if it proves noisy.

### 5. Persistence

One migration:

- `campaign_sales`: `dh_idempotency_key text`, `dh_sale_id text`,
  `dh_sale_recorded_at timestamp`
- `campaign_purchases`: `dh_sale_conflict text`, `dh_sale_conflict_at timestamp`

`dh_idempotency_key` is written at sale creation, before any DH call.
`dh_sale_id` is the handle void needs and is written after DH confirms.

An earlier draft derived the key instead of storing it, on the reasoning that a
stored key could drift from its sale row. That reasoning is inverted: the key
must be stable against a *client-supplied* sale id, which a derivation cannot
guarantee. See §2.

### 5a. Recovering a lost sale handle

The window that matters is: **DH recorded and delisted the sale, but persisting
`dh_sale_id` failed.** The item is now correctly off-market, so it leaves the
sweep's view entirely — `dhSoldSweepStatuses` covers only `listed` and
`in_stock` (`dh_sold_reconciler.go:37`) and the sweep acts only on rows DH
returns (`dh_sold_reconciler.go:181`). Nothing would ever revisit it, and
without `dh_sale_id` the sale can never be voided.

A second reconciliation pass closes this, scoped by *our* state rather than
DH's inventory listing:

```sql
SELECT s.* FROM campaign_sales s
JOIN campaign_purchases p ON p.id = s.purchase_id
WHERE s.dh_idempotency_key <> ''
  AND s.dh_sale_id = ''
  AND p.dh_inventory_id <> 0
```

Each row is re-issued with its persisted key and byte-identical body:

- already recorded → `replayed: true` plus the original sale → persist `dh_sale_id`
- not recorded → recorded now → persist `dh_sale_id`

Either way it converges, and because it keys off local columns it survives the
item being delisted on DH.

**Ordering:** persist key → call DH → persist `dh_sale_id` → apply any conflict
flag. A crash at any point leaves a row this pass can finish.

**Concurrent un-sell.** A void needs `dh_sale_id`. If un-sell runs against a
sale that has a key but no handle, it must first replay to obtain the handle,
then void — never skip the void and delete the row, which would orphan a
recorded sale on DH with no way to reverse it.


### 6. Sweep

The sweep stops issuing a status PATCH. It batch-loads sales via the existing
`GetSalesByPurchaseIDs` and records each properly through `RecordInventorySale`.

This also repairs the 25 mitigated items: they get real prices and dates from
our authoritative `campaign_sales` rows, DH's books reconcile with ours, and the
recurring 25x/hour 422 log noise ends.

Note this writes a real `realized_profit_cents` on DH's side computed from our
sale prices. That is intended, but it is a write to DH's ledger, not a local
cleanup.

The sweep keeps keying on `dh_inventory_id` (PR #682): a cert can match several
purchases across re-acquisitions, the inventory id cannot.

This DH-scoped sweep and the locally-scoped handle-recovery pass of §5a are
complementary and both are needed. The sweep catches items DH still offers that
we believe are sold; the recovery pass catches sales DH has already accepted
whose handle we failed to store — which by definition are no longer in the
sweep's window.

### 7. Un-sell

`DeleteSaleByPurchaseID` (`service_return_inventory.go`) calls
`VoidInventorySale` and then puts the purchase into the state the existing
auto-relist path requires.

Void returns the item to `in_stock` and clears the disposition figures but
deliberately does **not** relist it; relisting is a separate explicit
`PATCH {"status":"listed"}`.

**Keeping `dh_inventory_id` is necessary but not sufficient.** The push
scheduler selects only rows with `dh_push_status='pending'`
(`purchase_dh_query_store.go:16`), and for a row that already carries an
inventory id it takes the auto-relist branch only when *all* of
`dh_unlisted_detected_at` is set, a relister is wired, `cert_number` is
non-empty, and `ResolveListingPriceCents(&p) > 0` (`dh_push.go:248`). Otherwise
it merely marks the row `matched` and the card sits in `in_stock` forever.
Today's `ResetDHFieldsForRepushDueToDelete` supplies the pending status and the
unlisted marker as it clears the linkage
(`purchase_dh_push_store.go:218`); a design that only preserved the linkage
would supply neither.

So un-sell needs a new sibling of that reset — one atomic `UPDATE`, because a
partial application strands the row:

```
dh_push_status        = 'pending'
dh_unlisted_detected_at = now()
dh_status             = 'in_stock'
dh_channels_json      = '[]'
dh_inventory_id       -- PRESERVED (void kept the DH row alive)
```

Preserving the id is what distinguishes this from the delete-driven reset:
after a void the DH inventory row still exists, so a fresh push would create a
duplicate.

This reuses `dh_unlisted_detected_at` for "we voided a sale on this row" rather
than its literal meaning of "the reconciler found it missing from DH". That is
a deliberate semantic stretch: it is the field the auto-relist branch keys on,
and inventing a parallel flag would mean touching that branch's predicate.

**Behaviour when relisting cannot proceed.** If listings are globally paused, or
the row has no cert number, or no committed listing price resolves, the
auto-relist branch is skipped and the card stays `in_stock` and off-market until
a price is committed. This is a genuine change from today — currently un-sell
pushes the item as if new — and it must be surfaced in the UI rather than left
silent, or an un-sold card quietly stops being for sale.

A `404` on void is treated as success-with-a-log: it covers not-found, another
account's sale, a DH marketplace-mirror deal, and a UI-created deal, none of
which we can void and none of which should fail an un-sell the user already
performed locally.

Voiding an already-voided sale returns `200` with `reversed: false, items: []`.
That is success, not an error.


## Testing

`TestInventoryAdapter_MarkInventorySold` currently asserts against a permissive
mock that accepts any payload — which is precisely why a body the real API 422s
on shipped to production. Replace it with a **contract-enforcing fake** that:

- rejects a missing or oversized `Idempotency-Key`
- replays on same-key + same-body (`replayed: true`)
- returns `422 idempotency_key_reused` on same-key + different-body, including
  the same body against a different inventory id
- rejects statuses outside `{in_stock, listed}`
- can emit each documented error code, with null `sold_at`/`channel` on
  `item_sold_on_channel`, and with unexpected extra/missing fields

Plus table-driven unit tests for key generation and persistence ordering,
`sold_at` clamping (including both parse-failure fallbacks and the
`CreatedAt`-not-`now` upper bound), retry classification, conflict flagging, the
§5a recovery pass, and the §7 post-void state transition against the
auto-relist predicate in `dh_push.go:248`. Two cases deserve explicit coverage
because they are the ones that bite silently:

- a client-supplied `sale.ID` — including a reused one — must not influence the
  idempotency key
- a crash between the DH call and persisting `dh_sale_id` must be recoverable by
  the §5a pass, and must not double-dispose

Plus a `dhdiag`-tagged integration test exercising record-then-void against live
DH (`internal/integration/`, per the project rule that integration tests either
assert or carry the tag).

## Out of scope

- Multi-unit / partial sale support (no quantity concept exists locally)
- Relisting policy changes beyond routing un-sell through the existing pipeline
- Retiring `cmd/dh-delist` — decided separately once this ships
- Backfilling DH sale records for historical sales that DH already knows about
  (sales made *through* DH need no API call)

## Open risks

- Renaming the interface reaches `csvimport`, widening the blast radius beyond
  the sale paths.
- The 25 backfill writes to DH's ledger; if anything downstream consumes DH's
  P&L, that is a real mutation.
- `delisted == false` conflict-flagging is unproven and may be noisy.
- Un-sell no longer guarantees a relist (§7). When listings are paused or no
  price resolves, the card returns to `in_stock` and stays off-market. This
  needs UI surfacing or it will read as a bug.
- `dh_unlisted_detected_at` now carries two meanings (§7). If a future change
  splits the auto-relist predicate, both writers must be revisited.
- An inventory count discrepancy was observed while drafting this (DH reported
  48 listed against our 49 unsold). It may be an unreported channel sale — the
  `item_sold_on_channel` case — and should be reconciled independently.

## Review history

Reviewed against the repository by Codex on 2026-08-20 (verdict: REVISE). Three
blocking findings and one concern, all confirmed against the code and all folded
in above:

1. §2 — the original derive-from-`sale.ID` key was client-controllable
2. §7 — preserving `dh_inventory_id` alone does not satisfy the auto-relist
   predicate
3. §5a — no recovery path for a sale recorded on DH whose handle we failed to
   persist
4. §2 — date columns are not shape-validated, so the clamp needs parse-failure
   behaviour

A fifth issue surfaced while applying them: the original clamp used `now` as its
upper bound, which is not stable across retries and would itself have tripped
`422 idempotency_key_reused`. Fixed in §2 by clamping to `sale.CreatedAt`.
