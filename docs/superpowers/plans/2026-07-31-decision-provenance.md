# Decision-Time Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Freeze decision-time provenance on purchases (confidence, population, DH comp fields) and sales (sale_reason, cl-at-sale, channel-fee), and surface confidence×buy% cohorts in `/api/portfolio/analysis`.

**Architecture:** Additive migration `000022` (nullable frozen columns + generated `forced_liquidation`); set-once freeze logic in the inventory domain service, split by when each fact is known (creation-time vs first-successful-capture); analysis cohorts computed in the portfolio domain; new fields surfaced through existing shared column lists.

**Tech Stack:** Go 1.26, hexagonal architecture, Postgres (`jackc/pgx/v5/stdlib`), `golang-migrate/migrate/v4`, React/TS frontend.

**Reference spec:** `docs/superpowers/specs/2026-07-31-decision-provenance-design.md`

## Global Constraints

- Hexagonal: domain packages never import adapters (`scripts/check-imports.sh`).
- All money in **cents**; API responses in USD.
- Migrations are up+down pairs, embedded via `embed.FS`, run on startup.
- Table-driven tests; mocks from `internal/testutil/mocks` (never inline).
- Error assertions via `errors.Is` with sentinel errors.
- Source files under 500 lines (warn), 600 (fail) — `scripts/check-file-size.sh`.
- Always run `go test -race` before committing.
- **Set-once invariant:** a frozen column, once non-NULL, is never overwritten.
- **NULL = missing, stored value = observed.** Never store missing-as-zero.
- Frontend TS types in `web/src/types/` manually mirror Go JSON tags.
- Next migration number is `000022`.

---

## File Structure

**Migration**
- Create: `internal/adapters/storage/postgres/migrations/000022_add_decision_provenance.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000022_add_decision_provenance.down.sql`

**Domain — inventory**
- Modify: `internal/domain/inventory/types_core.go` (MarketSnapshotData +3 fields; Purchase +6 pointers; Sale +3 fields)
- Create: `internal/domain/inventory/sale_reason.go` (SaleReason consts + validators)
- Create: `internal/domain/inventory/confidence_parse.go` (ParseCLConfidenceMin)
- Modify: `internal/domain/inventory/channel_fees.go` (EffectiveChannelFeePct)
- Modify: `internal/domain/inventory/service_snapshots.go` (capture success + pre-correction source count + MarketDataObserved)
- Modify: `internal/domain/inventory/service_crud.go` (freeze at creation; freeze market on capture; sale freeze; reason preserve)
- Modify: `internal/domain/inventory/service_import_orders.go` (sale freeze in bulk + order-import paths)
- Modify: `internal/domain/inventory/repository_sale.go` (UpdateSaleReason)
- Modify: `internal/domain/inventory/types_core.go` (BulkSaleInput +3 per-item fields)

**Domain — portfolio**
- Modify: `internal/domain/portfolio/analysis_types.go` (ConfBuyCohortRow, ByReason)
- Modify: `internal/domain/portfolio/analysis.go` (compute cohorts + ByReason)

**Adapters — postgres**
- Modify: `internal/adapters/storage/postgres/purchase_scan_helpers.go` (column lists + scan dests, saleInsertColumns split)
- Modify: `internal/adapters/storage/postgres/purchase_store.go` (CreatePurchase INSERT)
- Modify: `internal/adapters/storage/postgres/purchase_price_store.go` (UpdatePurchaseMarketSnapshot set-once market freeze)
- Modify: `internal/adapters/storage/postgres/sale_store.go` (CreateSale via saleInsertColumns; UpdateSaleReason impl)

**Adapters — http**
- Modify: `internal/adapters/httpserver/routes.go` (PATCH route)
- Modify: `internal/adapters/httpserver/handlers/campaigns_purchases.go` (HandleUpdateSaleReason; bulk req struct)

**Mocks**
- Modify: `internal/testutil/mocks/sale_repository.go` (UpdateSaleReasonFn)

**Frontend**
- Modify: `web/src/types/campaigns/core.ts`
- Modify: `web/src/react/pages/campaign-detail/RecordSaleForm.tsx`
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`

**Docs**
- Modify: `docs/SCHEMA.md`, `docs/API.md`

---

## Task 1: Migration 000022 (schema + backfill + generated column)

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000022_add_decision_provenance.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000022_add_decision_provenance.down.sql`
- Test: `internal/adapters/storage/postgres/migration_000022_test.go`

**Interfaces:**
- Produces: 6 nullable `campaign_purchases` columns; `campaign_sales.sale_reason` (TEXT, CHECK), `cl_value_at_sale_cents` (BIGINT NOT NULL DEFAULT 0), `channel_fee_pct_at_sale` (DOUBLE PRECISION NULL); `forced_liquidation` regenerated as GENERATED STORED.

- [ ] **Step 1: Write the up migration**

Create `000022_add_decision_provenance.up.sql`:

```sql
-- Purchase decision-time provenance (nullable: NULL=not captured, value=observed).
ALTER TABLE campaign_purchases
    ADD COLUMN cl_confidence_at_purchase    SMALLINT,
    ADD COLUMN population_at_purchase        BIGINT,
    ADD COLUMN dh_confidence_at_purchase     DOUBLE PRECISION,
    ADD COLUMN source_count_at_purchase      BIGINT,
    ADD COLUMN active_listings_at_purchase   BIGINT,
    ADD COLUMN sales_last_30d_at_purchase    BIGINT;

-- Sale provenance.
ALTER TABLE campaign_sales
    ADD COLUMN sale_reason             TEXT NOT NULL DEFAULT ''
        CHECK (sale_reason IN ('', 'discretionary', 'invoice_pressure', 'aging_policy', 'bulk_lot', 'show_clearout')),
    ADD COLUMN cl_value_at_sale_cents  BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN channel_fee_pct_at_sale DOUBLE PRECISION;

-- Backfill sale_reason from derivable data.
UPDATE campaign_sales SET sale_reason = 'invoice_pressure'
    WHERE forced_liquidation = TRUE;

UPDATE campaign_sales s SET sale_reason = 'invoice_pressure'
    FROM campaign_purchases p
    WHERE s.purchase_id = p.id
      AND s.sale_reason = ''
      AND s.sale_channel IN ('inperson', 'local', 'cardshow')
      AND p.cl_value_at_purchase_cents > 0
      AND s.sale_price_cents < 0.80 * p.cl_value_at_purchase_cents;

UPDATE campaign_sales SET sale_reason = 'discretionary' WHERE sale_reason = '';

-- Replace the plain boolean with a generated column derived from sale_reason.
ALTER TABLE campaign_sales DROP COLUMN forced_liquidation;
ALTER TABLE campaign_sales
    ADD COLUMN forced_liquidation BOOLEAN NOT NULL
        GENERATED ALWAYS AS (sale_reason = 'invoice_pressure') STORED;
```

- [ ] **Step 2: Write the down migration**

Create `000022_add_decision_provenance.down.sql`:

```sql
ALTER TABLE campaign_sales DROP COLUMN forced_liquidation;
ALTER TABLE campaign_sales ADD COLUMN forced_liquidation BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE campaign_sales SET forced_liquidation = (sale_reason = 'invoice_pressure');

ALTER TABLE campaign_sales
    DROP COLUMN channel_fee_pct_at_sale,
    DROP COLUMN cl_value_at_sale_cents,
    DROP COLUMN sale_reason;

ALTER TABLE campaign_purchases
    DROP COLUMN sales_last_30d_at_purchase,
    DROP COLUMN active_listings_at_purchase,
    DROP COLUMN source_count_at_purchase,
    DROP COLUMN dh_confidence_at_purchase,
    DROP COLUMN population_at_purchase,
    DROP COLUMN cl_confidence_at_purchase;
```

- [ ] **Step 3: Write a migration round-trip test**

Add `internal/adapters/storage/postgres/migration_000022_test.go`. Follow the existing migration-test pattern in that directory (search for another `migration_*_test.go` or a test that opens a test DB via the shared harness). The test must, against a fresh test DB:
1. Seed a purchase with `cl_value_at_purchase_cents = 10000` and a sale on channel `inperson`, `sale_price_cents = 7000`, `forced_liquidation = FALSE`.
2. Run migrations to 000022.
3. Assert that sale's `sale_reason = 'invoice_pressure'` (7000 < 0.80×10000) and `forced_liquidation = TRUE`.
4. Seed/assert a second sale on `ebay` at full price → `sale_reason = 'discretionary'`, `forced_liquidation = FALSE`.

If no local test-DB harness exists in that package, mark this test `//go:build integration` and document running it via the integration tag; otherwise use the existing harness.

- [ ] **Step 4: Run migration + test**

Run: `go build -o /tmp/sl ./cmd/slabledger && go test ./internal/adapters/storage/postgres/ -run Migration_000022 -v`
Expected: build OK; test PASS (or skipped-with-reason if integration-tagged and DB absent — note which).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/migrations/000022_* internal/adapters/storage/postgres/migration_000022_test.go
git commit -m "feat(db): migration 000022 decision-time provenance columns"
```

---

## Task 2: SaleReason constants + validators

**Files:**
- Create: `internal/domain/inventory/sale_reason.go`
- Test: `internal/domain/inventory/sale_reason_test.go`

**Interfaces:**
- Produces: consts `SaleReasonDiscretionary`, `SaleReasonInvoicePressure`, `SaleReasonAgingPolicy`, `SaleReasonBulkLot`, `SaleReasonShowClearout` (all `string`); `func ValidSaleReason(s string) bool` (allows the 5 values + `""`); `func ValidSaleReasonForPatch(s string) bool` (the 5 values only, rejects `""`).

- [ ] **Step 1: Write the failing test**

```go
package inventory

import "testing"

func TestValidSaleReason(t *testing.T) {
	tests := []struct {
		in        string
		valid     bool
		validPatch bool
	}{
		{"discretionary", true, true},
		{"invoice_pressure", true, true},
		{"aging_policy", true, true},
		{"bulk_lot", true, true},
		{"show_clearout", true, true},
		{"", true, false},
		{"bogus", false, false},
	}
	for _, tt := range tests {
		if got := ValidSaleReason(tt.in); got != tt.valid {
			t.Errorf("ValidSaleReason(%q)=%v want %v", tt.in, got, tt.valid)
		}
		if got := ValidSaleReasonForPatch(tt.in); got != tt.validPatch {
			t.Errorf("ValidSaleReasonForPatch(%q)=%v want %v", tt.in, got, tt.validPatch)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestValidSaleReason -v`
Expected: FAIL (undefined: ValidSaleReason)

- [ ] **Step 3: Write minimal implementation**

Create `sale_reason.go`:

```go
package inventory

const (
	SaleReasonDiscretionary   = "discretionary"
	SaleReasonInvoicePressure = "invoice_pressure"
	SaleReasonAgingPolicy     = "aging_policy"
	SaleReasonBulkLot         = "bulk_lot"
	SaleReasonShowClearout    = "show_clearout"
)

var validSaleReasons = map[string]bool{
	SaleReasonDiscretionary:   true,
	SaleReasonInvoicePressure: true,
	SaleReasonAgingPolicy:     true,
	SaleReasonBulkLot:         true,
	SaleReasonShowClearout:    true,
}

// ValidSaleReason allows the five reasons plus "" (unknown/legacy).
func ValidSaleReason(s string) bool { return s == "" || validSaleReasons[s] }

// ValidSaleReasonForPatch allows only the five explicit reasons (rejects "").
func ValidSaleReasonForPatch(s string) bool { return validSaleReasons[s] }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/inventory/ -run TestValidSaleReason -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/sale_reason.go internal/domain/inventory/sale_reason_test.go
git commit -m "feat(inventory): sale_reason constants and validators"
```

---

## Task 3: ParseCLConfidenceMin helper

**Files:**
- Create: `internal/domain/inventory/confidence_parse.go`
- Test: `internal/domain/inventory/confidence_parse_test.go`

**Interfaces:**
- Produces: `func ParseCLConfidenceMin(s string) (int, bool)` — truncated integer min of a range like `"2.5-4"`; `ok=false` on unparseable/empty input.

- [ ] **Step 1: Write the failing test**

```go
package inventory

import "testing"

func TestParseCLConfidenceMin(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"2.5-4", 2, true},
		{"3-5", 3, true},
		{"4", 4, true},
		{"2.9", 2, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseCLConfidenceMin(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseCLConfidenceMin(%q)=(%d,%v) want (%d,%v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestParseCLConfidenceMin -v`
Expected: FAIL (undefined)

- [ ] **Step 3: Write minimal implementation**

Create `confidence_parse.go`:

```go
package inventory

import (
	"math"
	"strconv"
	"strings"
)

// ParseCLConfidenceMin returns the truncated integer minimum of a CL confidence
// range like "2.5-4" (→ 2). ok is false when the input is empty or unparseable.
func ParseCLConfidenceMin(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	lo := strings.TrimSpace(strings.SplitN(s, "-", 2)[0])
	f, err := strconv.ParseFloat(lo, 64)
	if err != nil {
		return 0, false
	}
	return int(math.Trunc(f)), true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/inventory/ -run TestParseCLConfidenceMin -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/confidence_parse.go internal/domain/inventory/confidence_parse_test.go
git commit -m "feat(inventory): ParseCLConfidenceMin range helper"
```

---

## Task 4: EffectiveChannelFeePct helper

**Files:**
- Modify: `internal/domain/inventory/channel_fees.go`
- Test: `internal/domain/inventory/channel_fees_test.go` (create if absent)

**Interfaces:**
- Consumes: existing `EffectiveFeePct(c *Campaign) float64`, `NormalizeChannel`, `DefaultWebsiteFeePct`, `constants.Sale*`.
- Produces: `func EffectiveChannelFeePct(channel SaleChannel, campaign *Campaign) float64` returning the fee fraction used by `CalculateSaleFee` for that channel (eBay/TCGPlayer → campaign eBay pct; website → 0.03; else → 0).

- [ ] **Step 1: Write the failing test**

```go
func TestEffectiveChannelFeePct(t *testing.T) {
	c := &Campaign{EbayFeePct: 0.10}
	tests := []struct {
		channel SaleChannel
		want    float64
	}{
		{SaleChannelEbay, 0.10},
		{SaleChannelTCGPlayer, 0.10},
		{SaleChannelWebsite, DefaultWebsiteFeePct},
		{SaleChannelInPerson, 0},
	}
	for _, tt := range tests {
		if got := EffectiveChannelFeePct(tt.channel, c); got != tt.want {
			t.Errorf("EffectiveChannelFeePct(%s)=%v want %v", tt.channel, got, tt.want)
		}
		// Parity: fee cents == round(price * pct) for a sample price.
		const price = 10000
		wantFee := CalculateSaleFee(tt.channel, price, c)
		gotFee := int(math.Round(float64(price) * got))
		if gotFee != wantFee {
			t.Errorf("parity mismatch %s: pct-derived %d vs CalculateSaleFee %d", tt.channel, gotFee, wantFee)
		}
	}
}
```

(Add `import "math"` to the test file if not present.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestEffectiveChannelFeePct -v`
Expected: FAIL (undefined: EffectiveChannelFeePct)

- [ ] **Step 3: Implement + refactor CalculateSaleFee to reuse it**

In `channel_fees.go`, add the helper and route `CalculateSaleFee` through it so they never diverge:

```go
// EffectiveChannelFeePct returns the fee fraction applied to a sale on the given
// channel — the same rate CalculateSaleFee uses.
func EffectiveChannelFeePct(channel SaleChannel, campaign *Campaign) float64 {
	switch NormalizeChannel(channel) {
	case SaleChannelEbay:
		return EffectiveFeePct(campaign)
	case SaleChannelWebsite:
		return DefaultWebsiteFeePct
	default:
		return 0
	}
}
```

Then change `CalculateSaleFee`'s body to:

```go
func CalculateSaleFee(channel SaleChannel, salePriceCents int, campaign *Campaign) int {
	return int(math.Round(float64(salePriceCents) * EffectiveChannelFeePct(channel, campaign)))
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/domain/inventory/ -run 'TestEffectiveChannelFeePct|TestCalculateSaleFee' -v`
Expected: PASS (existing CalculateSaleFee tests still green)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/channel_fees.go internal/domain/inventory/channel_fees_test.go
git commit -m "feat(inventory): EffectiveChannelFeePct extracted from CalculateSaleFee"
```

---

## Task 5: MarketSnapshotData provenance fields + applySnapshot

**Files:**
- Modify: `internal/domain/inventory/types_core.go`
- Test: `internal/domain/inventory/types_core_test.go` (create if absent)

**Interfaces:**
- Consumes: `MarketSnapshot` (has `Confidence float64`, `SourceCount int`, `ActiveListings`, `SalesLast30d`).
- Produces: `MarketSnapshotData.Confidence float64`, `.SourceCountRaw int`, `.MarketDataObserved bool`. `applySnapshot` copies Confidence and requires callers to set SourceCountRaw/MarketDataObserved (see Task 6). For this task, `applySnapshot` sets `d.Confidence = snapshot.Confidence`.

- [ ] **Step 1: Write the failing test**

```go
func TestApplySnapshotCopiesConfidence(t *testing.T) {
	var d MarketSnapshotData
	snap := &MarketSnapshot{Confidence: 0.9, ActiveListings: 3, SalesLast30d: 12}
	d.applySnapshot(snap, "2026-07-31")
	if d.Confidence != 0.9 {
		t.Errorf("Confidence=%v want 0.9", d.Confidence)
	}
	if d.ActiveListings != 3 || d.SalesLast30d != 12 {
		t.Errorf("market fields not copied: %+v", d)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestApplySnapshotCopiesConfidence -v`
Expected: FAIL (unknown field Confidence in MarketSnapshotData)

- [ ] **Step 3: Add fields + copy in applySnapshot**

In `types_core.go` `MarketSnapshotData`, add after `SnapshotJSON`:

```go
	// Decision-time provenance (persisted to *_at_purchase columns via the freeze paths).
	Confidence         float64 `json:"confidence,omitempty"`         // DH pricing confidence
	SourceCountRaw     int     `json:"sourceCountRaw,omitempty"`     // external platform count, pre-CL-correction
	MarketDataObserved bool    `json:"-"`                            // true when CardLookup market data was present
```

In `applySnapshot`, add `d.Confidence = snapshot.Confidence` (place near the other field copies). Do **not** derive `SourceCountRaw`/`MarketDataObserved` here — those are set by the capture path in Task 6.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/inventory/ -run TestApplySnapshotCopiesConfidence -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/types_core_test.go
git commit -m "feat(inventory): MarketSnapshotData carries confidence/source-count/observed"
```

---

## Task 6: Capture success signal + pre-correction source count + MarketDataObserved

**Files:**
- Modify: `internal/domain/inventory/service_snapshots.go`
- Test: `internal/domain/inventory/service_snapshots_test.go` (create if absent)

**Interfaces:**
- Consumes: `applyCLCorrection(snapshot, clValueCents)` (appends cardladder, recomputes SourceCount); `applyMarketSnapshot(r, snapshot)`; `MarketSnapshot`.
- Produces: `captureMarketSnapshot` returns `(*MarketSnapshot, bool)` where bool=capture succeeded; before calling `applyCLCorrection`, it records `preCorrectionSourceCount := len(snapshot.Sources)` and `marketObserved := snapshot has market data` and stamps them onto the receiver's `MarketSnapshotData` (SourceCountRaw, MarketDataObserved). Same for `recaptureMarketSnapshotDetailed`.

**Note on "market observed":** `MarketSnapshot` is flattened (no `Market` pointer). Treat market data as observed when `snapshot.ActiveListings > 0 || snapshot.SalesLast30d > 0 || snapshot.LowestListCents > 0`. This mirrors `dhprice.hasMarketData`. Add a small helper `snapshotHasMarketData(*MarketSnapshot) bool` in this file.

- [ ] **Step 1: Write the failing test**

```go
func TestSnapshotHasMarketData(t *testing.T) {
	if snapshotHasMarketData(&MarketSnapshot{}) {
		t.Error("empty snapshot should not be observed")
	}
	if !snapshotHasMarketData(&MarketSnapshot{ActiveListings: 2}) {
		t.Error("active listings should count as observed")
	}
	if !snapshotHasMarketData(&MarketSnapshot{SalesLast30d: 1}) {
		t.Error("recent sales should count as observed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestSnapshotHasMarketData -v`
Expected: FAIL (undefined)

- [ ] **Step 3: Implement helper + wire capture paths**

Add to `service_snapshots.go`:

```go
func snapshotHasMarketData(s *MarketSnapshot) bool {
	return s != nil && (s.ActiveListings > 0 || s.SalesLast30d > 0 || s.LowestListCents > 0)
}
```

In `captureMarketSnapshot`, change the signature to return `(*MarketSnapshot, bool)`. On the provider-error branch return `(nil, false)`. On success, before `applyCLCorrection`:

```go
		preCount := len(snapshot.Sources)
		observed := snapshotHasMarketData(snapshot)
		applyCLCorrection(snapshot, clValueCents)
		applyMarketSnapshot(r, snapshot)
		if md, ok := r.(*MarketSnapshotData); ok {
			// direct receiver — rare; the pointer receivers below handle Purchase/Sale
		}
		return snapshot, true
```

Because `r` is a `snapshotReceiver` embedding `MarketSnapshotData`, expose two setters on the receiver interface OR set the fields via the concrete embed. Simplest: have `applyMarketSnapshot` accept the pre-computed `preCount`/`observed` and set them on the embedded `MarketSnapshotData`. Update `applyMarketSnapshot(r, snapshot, preCount, observed)` accordingly and set `d.SourceCountRaw = preCount; d.MarketDataObserved = observed` inside it (alongside where it currently calls `applySnapshot`).

Apply the identical pre-correction capture in `recaptureMarketSnapshotDetailed` (which builds a local `MarketSnapshotData data`): compute `preCount`/`observed` before `applyCLCorrection(snapshot, clValueCents)`, then after `data.applySnapshot(...)` set `data.SourceCountRaw = preCount; data.MarketDataObserved = observed`.

Update the sync caller in `service_crud.go CreatePurchase` (Task 7) to consume the new `(snapshot, ok)` return.

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./internal/domain/inventory/ -run 'TestSnapshotHasMarketData' -v`
Expected: build OK; PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/service_snapshots.go internal/domain/inventory/service_snapshots_test.go
git commit -m "feat(inventory): capture success signal + pre-correction source count + market-observed flag"
```

---

## Task 7: Purchase struct pointers + freeze at creation + freeze market on capture

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (Purchase +6 pointers)
- Modify: `internal/domain/inventory/service_crud.go` (CreatePurchase)
- Test: `internal/domain/inventory/service_crud_test.go` (or existing service_test.go)

**Interfaces:**
- Consumes: `ParseCLConfidenceMin` (Task 3), `captureMarketSnapshot` new return (Task 6), `Campaign.CLConfidence`.
- Produces: `Purchase.CLConfidenceAtPurchase *int`, `.PopulationAtPurchase *int`, `.DHConfidenceAtPurchase *float64`, `.SourceCountAtPurchase *int`, `.ActiveListingsAtPurchase *int`, `.SalesLast30dAtPurchase *int` (all `json:",omitempty"`). CreatePurchase freezes creation-facts + market-facts.

- [ ] **Step 1: Write the failing test**

Using `mocks.NewInMemoryCampaignStore()` and the service constructor pattern from existing `service_test.go`, write:

```go
func TestCreatePurchase_FreezesCreationFacts(t *testing.T) {
	// campaign with CLConfidence "2.5-4"; purchase with Population 50, no snapshot pending.
	// After CreatePurchase: CLConfidenceAtPurchase == 2, PopulationAtPurchase == 50.
	// A purchase with Population 0 → PopulationAtPurchase stays nil.
}
```

Fill in using the existing test harness (see `TestCreateSale...` in `service_test.go` for constructing `service` with mocks). Assert both the populated and the `Population:0 → nil` cases.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestCreatePurchase_FreezesCreationFacts -v`
Expected: FAIL (unknown field CLConfidenceAtPurchase)

- [ ] **Step 3: Add pointers + freeze logic**

In `types_core.go` Purchase, after `CLValueAtPurchaseCents`:

```go
	CLConfidenceAtPurchase   *int     `json:"clConfidenceAtPurchase,omitempty"`
	PopulationAtPurchase     *int     `json:"populationAtPurchase,omitempty"`
	DHConfidenceAtPurchase   *float64 `json:"dhConfidenceAtPurchase,omitempty"`
	SourceCountAtPurchase    *int     `json:"sourceCountAtPurchase,omitempty"`
	ActiveListingsAtPurchase *int     `json:"activeListingsAtPurchase,omitempty"`
	SalesLast30dAtPurchase   *int     `json:"salesLast30dAtPurchase,omitempty"`
```

In `CreatePurchase` (`service_crud.go`), capture the campaign and freeze. Replace the discarded `GetCampaign` result:

```go
	campaign, err := s.campaigns.GetCampaign(ctx, p.CampaignID)
	if err != nil {
		return fmt.Errorf("campaign lookup: %w", err)
	}
	// (a) creation-time facts, set-once
	if p.CLConfidenceAtPurchase == nil {
		if c, ok := ParseCLConfidenceMin(campaign.CLConfidence); ok {
			p.CLConfidenceAtPurchase = &c
		}
	}
	if p.PopulationAtPurchase == nil && p.Population > 0 {
		pop := p.Population
		p.PopulationAtPurchase = &pop
	}

	if p.SnapshotStatus != SnapshotStatusPending {
		if snap, ok := s.captureMarketSnapshot(ctx, p, p.ToCardIdentity(), p.GradeValue, p.CLValueCents); ok {
			// (b) market-time facts, gated on confirmed capture
			if p.DHConfidenceAtPurchase == nil {
				conf := snap.Confidence
				p.DHConfidenceAtPurchase = &conf
			}
			if p.SourceCountAtPurchase == nil {
				sc := p.SourceCountRaw // set on the embed by applyMarketSnapshot
				p.SourceCountAtPurchase = &sc
			}
			if p.MarketDataObserved {
				al := p.ActiveListings
				sl := p.SalesLast30d
				p.ActiveListingsAtPurchase = &al
				p.SalesLast30dAtPurchase = &sl
			}
		}
	}
```

(Adjust field access to match how Task 6 stamps `SourceCountRaw`/`MarketDataObserved` onto the embedded `MarketSnapshotData` — `p.SourceCountRaw`, `p.MarketDataObserved`.)

- [ ] **Step 4: Run test**

Run: `go test ./internal/domain/inventory/ -run TestCreatePurchase_FreezesCreationFacts -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_crud_test.go
git commit -m "feat(inventory): freeze purchase provenance at creation + on capture"
```

---

## Task 8: Persist purchase provenance (INSERT + column lists + scan)

**Files:**
- Modify: `internal/adapters/storage/postgres/purchase_scan_helpers.go`
- Modify: `internal/adapters/storage/postgres/purchase_store.go`
- Test: `internal/adapters/storage/postgres/purchase_store_test.go` (extend existing round-trip test)

**Interfaces:**
- Consumes: Purchase pointer fields (Task 7).
- Produces: 6 columns in `purchaseColumns`, `purchaseColumnsAliased`, `purchaseScanDests` (scanning into the pointer fields); CreatePurchase INSERT writes them.

- [ ] **Step 1: Write/extend the failing round-trip test**

Extend the existing purchase create→get test (find `TestCreatePurchase` / round-trip in `purchase_store_test.go`) to set `CLConfidenceAtPurchase`, `PopulationAtPurchase`, `DHConfidenceAtPurchase`, `SourceCountAtPurchase`, `ActiveListingsAtPurchase`, `SalesLast30dAtPurchase` on the input, then assert `GetPurchase` returns equal pointer values, and that a nil input pointer round-trips as nil (NULL).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/storage/postgres/ -run TestCreatePurchase -v`
Expected: FAIL (columns absent / scan mismatch) — or a clear column-count error.

- [ ] **Step 3: Extend column lists, scan dests, and INSERT**

In `purchase_scan_helpers.go`, append to both `purchaseColumns` and `purchaseColumnsAliased` (with `p.` prefix) after `cl_value_at_purchase_cents`:

```
	, cl_confidence_at_purchase, population_at_purchase, dh_confidence_at_purchase,
	source_count_at_purchase, active_listings_at_purchase, sales_last_30d_at_purchase
```

Append to `purchaseScanDests` after `&p.CLValueAtPurchaseCents`:

```go
		&p.CLConfidenceAtPurchase, &p.PopulationAtPurchase, &p.DHConfidenceAtPurchase,
		&p.SourceCountAtPurchase, &p.ActiveListingsAtPurchase, &p.SalesLast30dAtPurchase,
```

(`database/sql` scans NULL into `*int`/`*float64` as nil automatically.)

In `purchase_store.go` `CreatePurchase`, add the 6 columns to the INSERT column list and 6 positional params ($56–$61) after `cl_value_at_purchase_cents`/`$55`, passing the pointer fields directly (they encode NULL when nil).

- [ ] **Step 4: Run test**

Run: `go test ./internal/adapters/storage/postgres/ -run TestCreatePurchase -v`
Expected: PASS (or integration-skip noted)

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/purchase_scan_helpers.go internal/adapters/storage/postgres/purchase_store.go internal/adapters/storage/postgres/purchase_store_test.go
git commit -m "feat(db): persist purchase decision-time provenance columns"
```

---

## Task 9: Async set-once market freeze in UpdatePurchaseMarketSnapshot

**Files:**
- Modify: `internal/adapters/storage/postgres/purchase_price_store.go`
- Test: `internal/adapters/storage/postgres/purchase_price_store_test.go`

**Interfaces:**
- Consumes: `MarketSnapshotData` with `Confidence`, `SourceCountRaw`, `MarketDataObserved`, `ActiveListings`, `SalesLast30d`.
- Produces: `UpdatePurchaseMarketSnapshot` additionally freezes `dh_confidence_at_purchase`, `source_count_at_purchase` when currently NULL; freezes `active_listings_at_purchase`, `sales_last_30d_at_purchase` when NULL **and** `MarketDataObserved`.

- [ ] **Step 1: Write the failing test**

Extend/author a test: insert a purchase (snapshot pending, all provenance NULL), call `UpdatePurchaseMarketSnapshot` with a `MarketSnapshotData{Confidence:0.9, SourceCountRaw:2, MarketDataObserved:true, ActiveListings:3, SalesLast30d:5}`, then `GetPurchase` and assert the 4 market provenance pointers are set. Then call it again with different values and assert they are **unchanged** (set-once). Add a third case with `MarketDataObserved:false` → `active_listings_at_purchase`/`sales_last_30d_at_purchase` stay NULL.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/storage/postgres/ -run UpdatePurchaseMarketSnapshot -v`
Expected: FAIL

- [ ] **Step 3: Extend the UPDATE**

Rewrite the query to add set-once freezes. Market-observed columns are gated with a boolean param `$14`:

```sql
UPDATE campaign_purchases SET last_sold_cents = $1, lowest_list_cents = $2, conservative_cents = $3,
	median_cents = $4, mid_price_cents = $5, last_sold_date = $6,
	active_listings = $7, sales_last_30d = $8, trend_30d = $9, snapshot_date = $10,
	snapshot_json = $11, updated_at = $12,
	dh_confidence_at_purchase = CASE WHEN dh_confidence_at_purchase IS NULL THEN $15 ELSE dh_confidence_at_purchase END,
	source_count_at_purchase  = CASE WHEN source_count_at_purchase IS NULL THEN $16 ELSE source_count_at_purchase END,
	active_listings_at_purchase = CASE WHEN active_listings_at_purchase IS NULL AND $14 THEN $7 ELSE active_listings_at_purchase END,
	sales_last_30d_at_purchase  = CASE WHEN sales_last_30d_at_purchase  IS NULL AND $14 THEN $8 ELSE sales_last_30d_at_purchase END
WHERE id = $13
```

Pass params: existing $1–$13, plus `$14 = snap.MarketDataObserved`, `$15 = snap.Confidence`, `$16 = snap.SourceCountRaw`.

- [ ] **Step 4: Run test**

Run: `go test ./internal/adapters/storage/postgres/ -run UpdatePurchaseMarketSnapshot -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/purchase_price_store.go internal/adapters/storage/postgres/purchase_price_store_test.go
git commit -m "feat(db): async set-once freeze of DH provenance on snapshot enrichment"
```

---

## Task 10: Sale struct fields + freeze at sale-creation (all 3 paths) + reason preserve

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (Sale +3 fields; make ForcedLiquidation read-only)
- Modify: `internal/domain/inventory/service_crud.go` (CreateSale + CreateBulkSales)
- Modify: `internal/domain/inventory/service_import_orders.go` (ConfirmOrdersSales)
- Test: `internal/domain/inventory/service_sale_test.go`

**Interfaces:**
- Consumes: `EffectiveChannelFeePct` (Task 4), `ValidSaleReason`, `IsForcedLiquidation`, `SaleReason*`.
- Produces: `Sale.SaleReason string`, `.CLValueAtSaleCents int`, `.ChannelFeePctAtSale *float64`. All 3 creation paths set them; explicit valid reason preserved, empty → heuristic default, non-empty invalid → `ErrInvalidSaleReason`.

- [ ] **Step 1: Write the failing test**

```go
func TestCreateSale_FreezesSaleProvenance(t *testing.T) {
	// campaign EbayFeePct 0.10; purchase CLValueCents 12000; sale on ebay.
	// After CreateSale: CLValueAtSaleCents==12000; *ChannelFeePctAtSale==0.10;
	// SaleReason=="discretionary" (no invoice pressure).
}
func TestCreateSale_PreservesExplicitReason(t *testing.T) {
	// sale with SaleReason "aging_policy" → preserved (not overwritten by heuristic).
}
func TestCreateSale_RejectsInvalidReason(t *testing.T) {
	// sale with SaleReason "bogus" → errors.Is(err, ErrInvalidSaleReason).
}
```

Add `ErrInvalidSaleReason = errors.NewAppError(ErrCodeCampaignValidation, "invalid sale reason")` in `validation.go` alongside `ErrSaleDateBeforePurchase`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run 'TestCreateSale_(Freezes|Preserves|Rejects)' -v`
Expected: FAIL

- [ ] **Step 3: Add fields + freeze logic in all three paths**

In `types_core.go` Sale: add

```go
	SaleReason          string   `json:"saleReason,omitempty"`
	CLValueAtSaleCents  int      `json:"clValueAtSaleCents,omitempty"`
	ChannelFeePctAtSale *float64 `json:"channelFeePctAtSale,omitempty"`
```

Update the `ForcedLiquidation` doc comment to note it is now a generated read-only column (kept for back-compat). Add a shared helper in `service_crud.go`:

```go
func freezeSaleProvenance(sa *Sale, purchase *Purchase, campaign *Campaign, forced bool) error {
	if sa.SaleReason != "" && !ValidSaleReason(sa.SaleReason) {
		return ErrInvalidSaleReason
	}
	if sa.SaleReason == "" {
		if forced {
			sa.SaleReason = SaleReasonInvoicePressure
		} else {
			sa.SaleReason = SaleReasonDiscretionary
		}
	}
	sa.CLValueAtSaleCents = purchase.CLValueCents
	pct := EffectiveChannelFeePct(sa.SaleChannel, campaign)
	sa.ChannelFeePctAtSale = &pct
	return nil
}
```

Call `freezeSaleProvenance(sa, purchase, campaign, IsForcedLiquidation(...))` in `CreateSale`, `CreateBulkSales`, and `ConfirmOrdersSales` right after the existing `ForcedLiquidation`/fee computation, and stop assigning `sa.ForcedLiquidation`. In `CreateSale`, return the error if non-nil (before persisting). For the bulk/import loops, on error append to `result.Errors` and `continue`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/inventory/ -run 'TestCreateSale|TestConfirmOrders|TestCreateBulk' -v`
Expected: PASS (update any existing test that asserted `sa.ForcedLiquidation` directly to assert `SaleReason` instead)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_import_orders.go internal/domain/inventory/validation.go internal/domain/inventory/service_sale_test.go
git commit -m "feat(inventory): freeze sale provenance across all creation paths"
```

---

## Task 11: Sale column split + persist + analytics JOIN scan

**Files:**
- Modify: `internal/adapters/storage/postgres/purchase_scan_helpers.go` (saleInsertColumns, saleColumns, saleColumnsAliased, scanSale, scanPurchaseWithSale)
- Modify: `internal/adapters/storage/postgres/sale_store.go` (CreateSale uses saleInsertColumns)
- Test: `internal/adapters/storage/postgres/sale_store_test.go`, `internal/adapters/storage/postgres/analytics_store_test.go`

**Interfaces:**
- Consumes: Sale new fields (Task 10).
- Produces: `saleInsertColumns` (all writable sale columns incl. the 3 new, **excluding** generated `forced_liquidation`); `saleColumns`/`saleColumnsAliased` extended with the 3 new columns; scans populate them.

- [ ] **Step 1: Write the failing tests**

(a) `sale_store_test.go`: create a sale with `SaleReason "invoice_pressure"`, `CLValueAtSaleCents 9000`, `ChannelFeePctAtSale &0.10`; `GetSaleByPurchaseID` returns them and `ForcedLiquidation == true` (generated). (b) `analytics_store_test.go`: extend the existing roundtrip (`TestGetAllPurchasesWithSalesFieldRoundtrip`) to assert `sale_reason` and `cl_value_at_sale_cents` come back through `GetAllPurchasesWithSales`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/storage/postgres/ -run 'TestCreateSale|TestGetAllPurchasesWithSales' -v`
Expected: FAIL (INSERT tries to write generated column / aliased list missing fields)

- [ ] **Step 3: Split columns + extend scans**

In `purchase_scan_helpers.go`:
- Add `saleInsertColumns` = current `saleColumns` **minus** `forced_liquidation`, **plus** `sale_reason, cl_value_at_sale_cents, channel_fee_pct_at_sale`.
- Extend `saleColumns` (read) with the 3 new columns (keep `forced_liquidation` last for scan-order stability, but append new cols before it — match scan dest order).
- Extend `saleColumnsAliased` with `s.sale_reason, s.cl_value_at_sale_cents, s.channel_fee_pct_at_sale`.
- Extend `scanSale` dests and the `scanPurchaseWithSale` sale-side dests with `sql.NullString`/`sql.NullInt64`/`sql.NullFloat64` locals for the 3 new fields and assign into the Sale (SaleReason from NullString.String; CLValueAtSaleCents from NullInt64; ChannelFeePctAtSale as `*float64` from NullFloat64).

In `sale_store.go` `CreateSale`, change `INSERT INTO campaign_sales (` + saleColumns + `)` to `saleInsertColumns`, update the `VALUES ($1..$N)` count, and add `s.SaleReason, s.CLValueAtSaleCents, s.ChannelFeePctAtSale` to the args (drop `s.ForcedLiquidation`).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/storage/postgres/ -run 'TestCreateSale|TestGetAllPurchasesWithSales|TestGetSale' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/purchase_scan_helpers.go internal/adapters/storage/postgres/sale_store.go internal/adapters/storage/postgres/sale_store_test.go internal/adapters/storage/postgres/analytics_store_test.go
git commit -m "feat(db): split sale insert/read columns for generated forced_liquidation; persist sale provenance"
```

---

## Task 12: UpdateSaleReason repository method + PATCH endpoint

**Files:**
- Modify: `internal/domain/inventory/repository_sale.go` (interface)
- Modify: `internal/adapters/storage/postgres/sale_store.go` (impl)
- Modify: `internal/testutil/mocks/sale_repository.go` (mock Fn)
- Modify: `internal/adapters/httpserver/routes.go` (route)
- Modify: `internal/adapters/httpserver/handlers/campaigns_purchases.go` (handler + service method)
- Modify: `internal/domain/inventory/service_crud.go` (service UpdateSaleReason)
- Test: `internal/adapters/storage/postgres/sale_store_test.go`, `internal/adapters/httpserver/handlers/campaigns_purchases_test.go`

**Interfaces:**
- Produces: `SaleRepository.UpdateSaleReason(ctx, campaignID, saleID, reason string) error`; service `UpdateSaleReason`; `PATCH /api/campaigns/{id}/sales/{saleID}`.

- [ ] **Step 1: Write the failing tests**

(a) DB test: seed a sale in campaign A; `UpdateSaleReason(ctx, "A", saleID, "aging_policy")` succeeds and `forced_liquidation` flips to false; `UpdateSaleReason(ctx, "B", saleID, ...)` returns `ErrSaleNotFound` (campaign scoping). (b) handler test: PATCH with `{"saleReason":""}` → 400; `{"saleReason":"bogus"}` → 400; `{"saleReason":"bulk_lot"}` → 200.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/storage/postgres/ ./internal/adapters/httpserver/handlers/ -run 'UpdateSaleReason' -v`
Expected: FAIL (undefined)

- [ ] **Step 3: Implement interface, impl, mock, service, handler, route**

Interface (`repository_sale.go`): add `UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error`.

Impl (`sale_store.go`), campaign-scoped, reusing `execAndExpectRow`-style:

```go
func (ss *SaleStore) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error {
	result, err := ss.db.ExecContext(ctx,
		`UPDATE campaign_sales SET sale_reason = $1, updated_at = $2
		 WHERE id = $3 AND purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = $4)`,
		reason, time.Now(), saleID, campaignID)
	if err != nil {
		return fmt.Errorf("update sale reason: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update sale reason rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}
```

Mock (`sale_repository.go`): add `UpdateSaleReasonFn func(ctx, campaignID, saleID, reason string) error` and method delegating to it.

Service (`service_crud.go`):

```go
func (s *service) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error {
	if !ValidSaleReasonForPatch(reason) {
		return ErrInvalidSaleReason
	}
	return s.sales.UpdateSaleReason(ctx, campaignID, saleID, reason)
}
```

Add `UpdateSaleReason` to the service interface (`service_interfaces.go`).

Handler (`campaigns_purchases.go`): `HandleUpdateSaleReason` reads `{id}` and `{saleID}` path params, decodes `{ saleReason string }`, calls the service; map `ErrInvalidSaleReason`→400, `ErrSaleNotFound`→404.

Route (`routes.go`), in the sales block:

```go
mux.Handle("PATCH /api/campaigns/{id}/sales/{saleID}", authRoute(rt.campaignsHandler.HandleUpdateSaleReason))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/storage/postgres/ ./internal/adapters/httpserver/handlers/ ./internal/domain/inventory/ -run 'UpdateSaleReason' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/repository_sale.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_interfaces.go internal/adapters/storage/postgres/sale_store.go internal/testutil/mocks/sale_repository.go internal/adapters/httpserver/routes.go internal/adapters/httpserver/handlers/campaigns_purchases.go internal/adapters/httpserver/handlers/campaigns_purchases_test.go internal/adapters/storage/postgres/sale_store_test.go
git commit -m "feat(api): campaign-scoped PATCH sale_reason endpoint"
```

---

## Task 13: Bulk listing-detail fields (per-item) end-to-end

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (BulkSaleInput +3 fields)
- Modify: `internal/domain/inventory/service_crud.go` (CreateBulkSales copies them onto Sale)
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
- Test: `internal/domain/inventory/service_crud_test.go`

**Interfaces:**
- Produces: `BulkSaleInput.OriginalListPriceCents int`, `.PriceReductions int`, `.DaysListed int` (per item); each carried onto the created `Sale`.

- [ ] **Step 1: Write the failing test**

```go
func TestCreateBulkSales_CopiesListingFields(t *testing.T) {
	// items[0] = {PurchaseID, SalePriceCents, OriginalListPriceCents:1500, PriceReductions:2, DaysListed:9}
	// After CreateBulkSales, the persisted sale carries those three values.
}
```

Assert via the mock `SaleRepository.CreateSaleFn` capturing the `*Sale`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestCreateBulkSales_CopiesListingFields -v`
Expected: FAIL (unknown field)

- [ ] **Step 3: Add per-item fields + copy**

In `types_core.go`:

```go
type BulkSaleInput struct {
	PurchaseID             string `json:"purchaseId"`
	SalePriceCents         int    `json:"salePriceCents"`
	OriginalListPriceCents int    `json:"originalListPriceCents,omitempty"`
	PriceReductions        int    `json:"priceReductions,omitempty"`
	DaysListed             int    `json:"daysListed,omitempty"`
}
```

In `CreateBulkSales`, when building `sa := &Sale{...}`, add `OriginalListPriceCents: item.OriginalListPriceCents, PriceReductions: item.PriceReductions, DaysListed: item.DaysListed`.

Frontend `BulkRecordSaleModal.tsx`: add three optional number inputs (mirror `RecordSaleForm`'s "Add listing details" expander) and include the parsed per-item values in each item of the bulk request payload.

- [ ] **Step 4: Run test + frontend build**

Run: `go test ./internal/domain/inventory/ -run TestCreateBulkSales_CopiesListingFields -v && (cd web && npm run build)`
Expected: PASS; frontend build OK

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_crud_test.go web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx
git commit -m "feat: per-item listing detail fields in bulk sale path"
```

---

## Task 14: Analysis cohorts (PNLByConfidenceBuy + ByReason)

**Files:**
- Modify: `internal/domain/portfolio/analysis_types.go`
- Modify: `internal/domain/portfolio/analysis.go`
- Test: `internal/domain/portfolio/analysis_test.go`

**Interfaces:**
- Consumes: `inventory.PurchaseWithSale` (now carrying provenance pointers + sale fields).
- Produces: `CampaignAnalysis.PNLByConfidenceBuy []ConfBuyCohortRow`; `SplitPNL.ByReason map[string]PNLBlock`.

- [ ] **Step 1: Write the failing test**

```go
func TestComputeConfidenceBuyCohorts(t *testing.T) {
	// Row A: CLConfidenceAtPurchase=2, buyCost 7000, clAtPurchase 10000 → bucket "70-75".
	// Row B: CLConfidenceAtPurchase nil → confidenceBucket "unknown".
	// Row C: clAtPurchase 0/nil → buyTermsBucket "unknown".
	// avgActiveListings skips nil pointers, includes a real 0.
}
func TestByReasonSplit(t *testing.T) {
	// sales with reasons discretionary + invoice_pressure land in the right buckets.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/portfolio/ -run 'TestComputeConfidenceBuyCohorts|TestByReasonSplit' -v`
Expected: FAIL (undefined types)

- [ ] **Step 3: Add types + compute**

In `analysis_types.go`:

```go
type ConfBuyCohortRow struct {
	ConfidenceBucket   string  `json:"confidenceBucket"` // "2","3",... or "unknown"
	BuyTermsBucket     string  `json:"buyTermsBucket"`   // "70-75","<50",">=100","unknown"
	N                  int     `json:"n"`
	SoldCount          int     `json:"soldCount"`
	RevenueCents       int     `json:"revenueCents"`
	NetProfitCents     int     `json:"netProfitCents"`
	ROIPct             float64 `json:"roiPct"`
	AvgSourceCount     float64 `json:"avgSourceCount"`
	AvgActiveListings  float64 `json:"avgActiveListings"`
	AvgSalesLast30d    float64 `json:"avgSalesLast30d"`
	AvgPopulationAtBuy float64 `json:"avgPopulationAtBuy"`
	CoverageSourceCount int    `json:"coverageSourceCount"`
	CoverageMarket      int    `json:"coverageMarket"`
	CoveragePopulation  int    `json:"coveragePopulation"`
}
```

Add `PNLByConfidenceBuy []ConfBuyCohortRow` to `CampaignAnalysis` and `ByReason map[string]PNLBlock` to `SplitPNL`.

In `analysis.go`:
- `confidenceBucket(p)`: `p.CLConfidenceAtPurchase == nil` → `"unknown"`, else `strconv.Itoa(*p)`.
- `buyTermsBucket(p)`: if `CLValueAtPurchaseCents <= 0` → `"unknown"`; ratio = `float64(BuyCostCents)/float64(CLValueAtPurchaseCents)`; `<0.50`→`"<50"`, `>=1.00`→`">=100"`, else 5% band `fmt.Sprintf("%d-%d", lo, lo+5)` where `lo = int(math.Floor(ratio*20))*5`.
- Group rows by `(confidenceBucket, buyTermsBucket)`, accumulate PNL for sold rows, and average each provenance pointer **skipping nil** (incrementing the matching coverage counter; a stored 0 counts).
- Extend `computeSplitPNL` to also populate `ByReason[r.Sale.SaleReason]`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/portfolio/ -run 'TestComputeConfidenceBuyCohorts|TestByReasonSplit|TestComputeSplitPNL' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/portfolio/analysis_types.go internal/domain/portfolio/analysis.go internal/domain/portfolio/analysis_test.go
git commit -m "feat(portfolio): confidence×buy cohorts and by-reason P&L split"
```

---

## Task 15: Frontend TS types + single-sale reason select + docs

**Files:**
- Modify: `web/src/types/campaigns/core.ts`
- Modify: `web/src/react/pages/campaign-detail/RecordSaleForm.tsx`
- Modify: `docs/SCHEMA.md`, `docs/API.md`
- Test: frontend build + existing vitest

**Interfaces:**
- Consumes: Go JSON tags from Tasks 7, 10, 14.

- [ ] **Step 1: Add TS fields + reason select**

In `core.ts`, extend the `Sale` interface with `saleReason?: string; clValueAtSaleCents?: number; channelFeePctAtSale?: number;` and the `Purchase` interface with `clConfidenceAtPurchase?: number; populationAtPurchase?: number; dhConfidenceAtPurchase?: number; sourceCountAtPurchase?: number; activeListingsAtPurchase?: number; salesLast30dAtPurchase?: number;`.

In `RecordSaleForm.tsx`, add a `sale_reason` `<select>` (options: discretionary, invoice_pressure, aging_policy, bulk_lot, show_clearout; default empty = server heuristic) and include `saleReason` in the create payload when non-empty.

- [ ] **Step 2: Update docs**

`docs/SCHEMA.md`: document the 6 nullable purchase columns, the 3 sale columns (note `channel_fee_pct_at_sale` nullable, `sale_reason` CHECK), and `forced_liquidation` now GENERATED. `docs/API.md`: add `PATCH /api/campaigns/{id}/sales/{saleID}`, the `BulkSaleInput` per-item fields, and the analysis response additions (`pnlByConfidenceBuy`, `pnl.byReason`); note imported `*_at_sale` values are record-time proxies.

- [ ] **Step 3: Build + test frontend**

Run: `cd web && npm run build && npm test`
Expected: build OK; tests PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/types/campaigns/core.ts web/src/react/pages/campaign-detail/RecordSaleForm.tsx docs/SCHEMA.md docs/API.md
git commit -m "feat(web/docs): provenance TS types, sale_reason select, schema/API docs"
```

---

## Task 16: Full verification pass

**Files:** none (verification only)

- [ ] **Step 1: Full backend suite with race**

Run: `go test -race -timeout 10m ./...`
Expected: PASS

- [ ] **Step 2: Quality gates**

Run: `make check`
Expected: lint + architecture + file-size all pass. If any modified file exceeds 500 lines, split at a natural boundary and re-commit.

- [ ] **Step 3: Build binary + frontend**

Run: `go build -o /tmp/sl ./cmd/slabledger && (cd web && npm run build)`
Expected: both OK

- [ ] **Step 4: Migration smoke**

Run: start the binary against a scratch DB (or the devcontainer Postgres) and confirm migrations apply through 000022 without error, then confirm `down` to 000021 and back up.

- [ ] **Step 5: Commit any fixups**

```bash
git add -A
git commit -m "chore: verification fixups for decision-provenance"
```

---

## Self-Review

**Spec coverage:**
- §1 migration → Task 1. Purchase 6 cols nullable → Tasks 1,7,8. Sale 3 cols + generated boolean → Tasks 1,10,11. Backfill + CHECK → Task 1.
- §2 MarketSnapshotData fields → Task 5. Capture success/pre-correction/observed → Task 6. Split freeze (creation vs capture) → Tasks 7,9. Persist → Tasks 8,9. Sale freeze all 3 paths + reason preserve/reject → Task 10. Column split → Task 11. PATCH campaign-scoped → Task 12. EffectiveChannelFeePct pointer (addressable) → Tasks 4,10.
- §3 cohorts (confidenceBucket string, frozen-CL buckets, coverage-guarded avgs) → Task 14. ByReason → Task 14. saleColumnsAliased scan → Task 11. Frontend types + selects + bulk per-item → Tasks 13,15. Docs → Task 15.
- Exclusions (no backfill of provenance, no async creation-fact fallback, source_count = pre-correction platforms) honored: Tasks 7/9 freeze forward-only; Task 6 pre-correction count.

**Placeholder scan:** All code steps contain concrete code or exact instructions with named symbols. Test bodies that reference the existing harness point to the specific existing test to copy (Tasks 1,7,8,9,11 — DB/service harness is pre-existing and per-repo).

**Type consistency:** `EffectiveChannelFeePct`, `ParseCLConfidenceMin(_)( int, bool)`, `ValidSaleReason`/`ValidSaleReasonForPatch`, `UpdateSaleReason(ctx, campaignID, saleID, reason)`, `SourceCountRaw`/`MarketDataObserved`, pointer fields on Purchase/Sale — all used consistently across tasks.
