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

**Key = `"slabledger-sale-" + sale.ID`.** Sale IDs are UUIDs from `s.idGen()`
(`service_crud.go`), stable, and ours.

This is the load-bearing decision. The sale path and the sweep derive the
*identical* key for the same sale, so a sweep call after a successful
`CreateSale` replays (`replayed: true`) rather than disposing inventory twice.
Double-recording becomes structurally impossible instead of a thing we must
remember to avoid.

Void is handled for free: un-sell deletes the `campaign_sales` row, so any
re-sale draws a fresh UUID and therefore a fresh key. A voided key is never
reused, which the contract requires.

**Corollary rule: the request body MUST be a pure function of the sale row.**
`Sale.SaleDate` is date-only (`YYYY-MM-DD`), so `sold_at` is derived
deterministically and clamped into `[purchase_date, now]` *before the first
send*. Sending an unclamped value, taking a 422 for a future timestamp, then
retrying with a corrected body would trip `422 idempotency_key_reused` and
strand that sale permanently.

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

- `campaign_sales`: `dh_sale_id text`, `dh_sale_recorded_at timestamp`
  (`dh_sale_id` is the handle void needs)
- `campaign_purchases`: `dh_sale_conflict text`, `dh_sale_conflict_at timestamp`

The idempotency key is deliberately **not** stored. Deriving it from `sale.ID`
makes it impossible for a stored key and its sale row to disagree.

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

### 7. Un-sell

`DeleteSaleByPurchaseID` (`service_return_inventory.go`) calls
`VoidInventorySale`, keeps the `dh_inventory_id` linkage, and lets the existing
push pipeline relist the item.

Void returns the item to `in_stock` and clears the disposition figures but
deliberately does **not** relist it; relisting is a separate explicit
`PATCH {"status":"listed"}`. Routing the relist through the normal push pipeline
preserves today's user-visible behaviour (un-sell puts the card back on sale)
while dropping the linkage-clearing workaround the current code needs.

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

Plus table-driven unit tests for key derivation, `sold_at` clamping, retry
classification, and conflict flagging; and a `dhdiag`-tagged integration test
exercising record-then-void against live DH (`internal/integration/`, per the
project rule that integration tests either assert or carry the tag).

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
- An inventory count discrepancy was observed while drafting this (DH reported
  48 listed against our 49 unsold). It may be an unreported channel sale — the
  `item_sold_on_channel` case — and should be reconciled independently.
