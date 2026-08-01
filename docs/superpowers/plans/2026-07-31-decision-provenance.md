# Decision-Time Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Freeze decision-time provenance on purchases (confidence, population, DH comp fields) and sales (sale_reason, cl-at-sale, channel-fee), and surface confidence×buy% cohorts in `/api/portfolio/analysis`.

**Architecture:** Additive migration `000022` (nullable frozen columns; `forced_liquidation` stays a plain app-maintained boolean so image-only rollback is safe); set-once freeze logic in the inventory domain service, split by when each fact is known (creation-time vs first-successful-capture), with server-authoritative clearing of client-supplied provenance; analysis cohorts computed in the portfolio domain; new fields surfaced through existing shared column lists.

**Tech Stack:** Go 1.26, hexagonal architecture, Postgres (`jackc/pgx/v5/stdlib`), `golang-migrate/migrate/v4`, React/TS frontend.

**Reference spec:** `docs/superpowers/specs/2026-07-31-decision-provenance-design.md`

## Global Constraints

- Hexagonal: domain packages never import adapters (`scripts/check-imports.sh`).
- All money in **cents**; API responses in USD.
- Migrations are up+down pairs, embedded via `embed.FS`, run on startup.
- Table-driven tests; mocks from `internal/testutil/mocks` (never inline).
- Error assertions via `errors.Is` with sentinel errors.
- Source files under 500 lines (warn), 600 (fail) — `scripts/check-file-size.sh`.
- Per-task commits run the **focused** test for that task (fast, non-race). The full `go test -race ./...` gate runs once in Task 16 before the work is considered done; if it surfaces a data race, fix and add a follow-up commit. (This resolves the apparent tension between per-task speed and the race requirement — race is a final gate, not a per-commit gate.)
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
- Modify: `internal/adapters/storage/postgres/purchase_scan_helpers.go` (column lists + scan dests; extend saleColumns/saleColumnsAliased)
- Modify: `internal/adapters/storage/postgres/purchase_store.go` (CreatePurchase INSERT)
- Modify: `internal/adapters/storage/postgres/purchase_price_store.go` (UpdatePurchaseMarketSnapshot set-once market freeze)
- Modify: `internal/adapters/storage/postgres/sale_store.go` (CreateSale INSERT +3 cols; UpdateSaleReason impl)

**Adapters — http**
- Modify: `internal/adapters/httpserver/routes.go` (PATCH route)
- Modify: `internal/adapters/httpserver/handlers/campaigns_purchases.go` (HandleUpdateSaleReason; bulk req struct)

**Mocks**
- Modify: `internal/testutil/mocks/inventory_sale_repo.go` (repo UpdateSaleReasonFn)
- Modify: `internal/testutil/mocks/inventory_service.go` (MockInventoryService.UpdateSaleReason)

**Frontend**
- Modify: `web/src/types/campaigns/core.ts`
- Modify: `web/src/react/pages/campaign-detail/RecordSaleForm.tsx`
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`

**Docs**
- Modify: `docs/SCHEMA.md`, `docs/API.md`, `docs/OPERATIONS.md`

---

## Task 1: Migration 000022 (schema + backfill; forced_liquidation stays plain boolean)

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000022_add_decision_provenance.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000022_add_decision_provenance.down.sql`
- Test: `internal/adapters/storage/postgres/migration_000022_test.go`

**Interfaces:**
- Produces: 6 nullable `campaign_purchases` columns; `campaign_sales.sale_reason` (TEXT, CHECK), `cl_value_at_sale_cents` (BIGINT NOT NULL DEFAULT 0), `channel_fee_pct_at_sale` (DOUBLE PRECISION NULL); `forced_liquidation` remains a plain writable BOOLEAN, re-synced to the backfilled reasons.

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

-- forced_liquidation stays a PLAIN boolean (NOT generated), so the previous
-- image — which INSERTs forced_liquidation explicitly — keeps working after an
-- image-only rollback (see docs/OPERATIONS.md; no DB down-migration on rollback).
-- The app maintains it alongside sale_reason (forced == reason='invoice_pressure').
-- Re-sync it here to match the just-backfilled reasons.
UPDATE campaign_sales SET forced_liquidation = (sale_reason = 'invoice_pressure');

-- Rollback-window compatibility: during a rollback to the previous image, that
-- binary INSERTs rows WITHOUT sale_reason (defaults to ''), setting only
-- forced_liquidation. Re-deploying the new image does NOT rerun this migration,
-- so those rows would keep sale_reason='' and analysis would silently skip them.
-- This trigger derives sale_reason from the boolean on any INSERT/UPDATE that
-- leaves sale_reason empty, so legacy-shaped writes are never lost.
CREATE OR REPLACE FUNCTION campaign_sales_derive_reason() RETURNS trigger AS $$
BEGIN
    IF NEW.sale_reason IS NULL OR NEW.sale_reason = '' THEN
        NEW.sale_reason := CASE WHEN NEW.forced_liquidation
                                THEN 'invoice_pressure' ELSE 'discretionary' END;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER campaign_sales_derive_reason_trg
    BEFORE INSERT OR UPDATE ON campaign_sales
    FOR EACH ROW EXECUTE FUNCTION campaign_sales_derive_reason();
```

The trigger keeps the invariant "`sale_reason` is never empty for a persisted
row" true regardless of which binary writes. It only fills an *empty* reason, so
the new image's explicit richer reasons (`aging_policy` etc.) are never clobbered.
(No separate CHECK forbidding `''` is added because legacy INSERTs would violate
it mid-rollback; the trigger enforces the intent without rejecting those writes.)

**Rollback safety note (in the migration file as a comment and in Task 15 docs):**
because `forced_liquidation` remains an ordinary writable column, the old binary
can still INSERT/read it after rollback. The new provenance columns are additive
and nullable/defaulted, so the old binary ignores them harmlessly. No coordinated
DB down-migration is required. (A down-migration still exists for completeness but
is not part of the normal rollback path; note it collapses specialized reasons —
`aging_policy`/`bulk_lot`/`show_clearout` are lost — which is acceptable only for
a full teardown, not a production rollback.)

- [ ] **Step 2: Write the down migration**

Create `000022_add_decision_provenance.down.sql`:

```sql
-- Drop the compatibility trigger first (it references sale_reason).
DROP TRIGGER IF EXISTS campaign_sales_derive_reason_trg ON campaign_sales;
DROP FUNCTION IF EXISTS campaign_sales_derive_reason();

-- forced_liquidation is already a plain boolean and is left in place (it existed
-- pre-000022). Only the columns 000022 added are dropped.
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

- [ ] **Step 3: Write a backfill migration test (version-stepped)**

`setupTestDB`/`RunMigrations` always migrate to head, and the backfill runs *inside* the 000022 up-migration — so to exercise it we must land legacy rows at **v21** and then step up to v22. Add `internal/adapters/storage/postgres/migration_000022_test.go` following the harness in `migrations_test.go` (same `migratepgx.WithInstance` + `iofs.New(MigrationsFS, "migrations")` construction; skip when Postgres is unreachable):

```go
func TestMigration000022_SaleReasonBackfill(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		url = "postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable"
	}
	logger := mocks.NewMockLogger()
	db, err := Open(context.Background(), url, logger)
	if err != nil {
		t.Skipf("Postgres not reachable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	// Known baseline.
	_, err = db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err)

	// Build a migrate instance on the embedded source.
	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	require.NoError(t, err)
	src, err := iofs.New(MigrationsFS, "migrations")
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	require.NoError(t, err)

	// Migrate to v21 (pre-provenance), seed legacy rows there.
	require.NoError(t, m.Migrate(21))
	// (Insert a campaign + two purchases + two sales via raw SQL against the v21
	//  schema: purchase P1 cl_value_at_purchase_cents=10000; sale S1 channel
	//  'inperson' price 7000 forced_liquidation=FALSE; purchase P2 cl 5000;
	//  sale S2 channel 'ebay' price 6000 forced_liquidation=FALSE.)

	// Step up to v22 — runs the backfill.
	require.NoError(t, m.Steps(1))

	// S1: in-person, 7000 < 0.80*10000 → sale_reason 'invoice_pressure'; forced_liquidation re-synced to TRUE.
	// S2: ebay full price → sale_reason 'discretionary'; forced_liquidation FALSE.
	// Assert sale_reason + forced_liquidation for both via SELECT.
	// (forced_liquidation is a plain column re-synced by the up-migration, not generated.)

	// Rollback-window compatibility: simulate the OLD image inserting a sale after
	// v22 — it writes forced_liquidation only, no sale_reason. The BEFORE trigger
	// must derive sale_reason from the boolean.
	//   INSERT a sale S3 with forced_liquidation=TRUE and NO sale_reason column in
	//   the column list → assert sale_reason == 'invoice_pressure'.
	//   INSERT a sale S4 with forced_liquidation=FALSE, no sale_reason → 'discretionary'.
	//   INSERT a sale S5 WITH sale_reason='aging_policy' → preserved (trigger only
	//   fills empty).

	// Round-trip down/up to confirm reversibility (down drops the trigger+function
	// and the 000022-added columns, keeps forced_liquidation; up re-adds them).
	require.NoError(t, m.Steps(-1))
	require.NoError(t, m.Steps(1))
}
```

Fill the seed/assert SQL inline (raw `db.ExecContext`/`QueryRowContext`). Import `migrate`, `migratepgx`, `iofs`, `mocks`, `require` exactly as `migrations_test.go` does.

- [ ] **Step 4: Run migration + test**

Run: `go build -o /tmp/sl ./cmd/slabledger && go test ./internal/adapters/storage/postgres/ -run 'TestMigration000022|TestMigrations_UpDownUpRoundtrip' -v`
Expected: build OK; PASS (or skipped-with-reason if Postgres unreachable — note which).

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

## Task 5: MarketSnapshot(+Data) provenance fields, applySnapshot, adapter capture

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (MarketSnapshotData +3)
- Modify: `internal/domain/inventory/service.go` (MarketSnapshot +2)
- Modify: `internal/domain/pricing/lookup/adapter.go` (buildMarketSnapshot captures both)
- Test: `internal/domain/inventory/types_core_test.go` (create if absent)
- Test: `internal/domain/pricing/lookup/adapter_test.go` (extend)

**Interfaces:**
- Consumes: `pricing.Price` (`.Market *MarketData`, `.Sources []string`, `.Confidence float64`).
- Produces: `MarketSnapshot.SourceCountRaw int`, `.MarketDataObserved bool`; `MarketSnapshotData.Confidence float64`, `.SourceCountRaw int`, `.MarketDataObserved bool`. `applySnapshot` copies all three from snapshot to data. `buildMarketSnapshot` sets `SourceCountRaw = len(price.Sources)` and `MarketDataObserved = price.Market != nil`.

- [ ] **Step 1: Write the failing tests**

```go
// types_core_test.go — applySnapshot copies all three provenance fields.
func TestApplySnapshotCopiesProvenance(t *testing.T) {
	var d MarketSnapshotData
	snap := &MarketSnapshot{Confidence: 0.9, SourceCountRaw: 2, MarketDataObserved: true, ActiveListings: 3, SalesLast30d: 12}
	d.applySnapshot(snap, "2026-07-31")
	if d.Confidence != 0.9 || d.SourceCountRaw != 2 || !d.MarketDataObserved {
		t.Errorf("provenance not copied: %+v", d)
	}
	if d.ActiveListings != 3 || d.SalesLast30d != 12 {
		t.Errorf("market fields not copied: %+v", d)
	}
}
```

```go
// adapter_test.go — buildMarketSnapshot captures presence + pre-correction count.
// Case A: price.Market != nil with all-zero listings → MarketDataObserved true, ActiveListings 0.
// Case B: price.Market == nil → MarketDataObserved false.
// Case C: price.Sources = ["ebay","dh"] → SourceCountRaw 2 (before any cardladder append).
```

Fill Case A/B/C by constructing a `pricing.Price` and calling `a.buildMarketSnapshot(price, 9)` following the existing `TestGetMarketSnapshot_*` patterns in that file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/inventory/ ./internal/domain/pricing/lookup/ -run 'TestApplySnapshotCopiesProvenance|TestBuildMarketSnapshot_Provenance' -v`
Expected: FAIL (unknown fields)

- [ ] **Step 3: Add fields + copy in applySnapshot + capture in adapter**

In `types_core.go` `MarketSnapshotData`, add after `SnapshotJSON`:

```go
	// Decision-time provenance (persisted to *_at_purchase columns via the freeze paths).
	Confidence         float64 `json:"confidence,omitempty"`         // DH pricing confidence
	SourceCountRaw     int     `json:"sourceCountRaw,omitempty"`     // external platform count, pre-CL-correction
	MarketDataObserved bool    `json:"-"`                            // true when CardLookup market data was present
```

In the `MarketSnapshot` struct (`internal/domain/inventory/service.go`), add two fields so presence and pre-correction count are captured at the true source and survive `applyCLCorrection` (which mutates `Sources`/`SourceCount`):

```go
	// SourceCountRaw is len(Sources) at build time, BEFORE applyCLCorrection
	// appends the "cardladder" anchor. This is the external-platform count.
	SourceCountRaw int `json:"sourceCountRaw,omitempty"`
	// MarketDataObserved is true when the DH CardLookup returned a market block
	// (price.Market != nil) — distinguishing an observed zero from a missing one.
	MarketDataObserved bool `json:"marketDataObserved,omitempty"`
```

In the adapter `buildMarketSnapshot` (`internal/domain/pricing/lookup/adapter.go`): set `snap.SourceCountRaw = len(price.Sources)` where `Sources` is assigned (near line 268-270, before any CL correction runs downstream), and set `snap.MarketDataObserved = price.Market != nil` inside the `if price.Market != nil {` block at line 238.

In `applySnapshot` (`types_core.go`), copy all three (place near the other field copies):

```go
	d.Confidence = snapshot.Confidence
	d.SourceCountRaw = snapshot.SourceCountRaw
	d.MarketDataObserved = snapshot.MarketDataObserved
```

Because these are captured at build time, they are immune to the later
`applyCLCorrection` that appends `cardladder` — no receiver-setter plumbing
needed.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/inventory/ ./internal/domain/pricing/lookup/ -run 'TestApplySnapshotCopiesProvenance|TestBuildMarketSnapshot_Provenance' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/service.go internal/domain/pricing/lookup/adapter.go internal/domain/inventory/types_core_test.go internal/domain/pricing/lookup/adapter_test.go
git commit -m "feat(inventory): capture market presence + pre-correction source count at snapshot build"
```

---

## Task 6: Capture-success signal on the sync capture path

**Files:**
- Modify: `internal/domain/inventory/service_snapshots.go`
- Test: `internal/domain/inventory/service_snapshots_test.go` (create if absent)

**Interfaces:**
- Consumes: `applyCLCorrection`, `applyMarketSnapshot`, `MarketSnapshot` (now carrying `SourceCountRaw`/`MarketDataObserved` from Task 5).
- Produces: `captureMarketSnapshot(ctx, r, card, grade, clValueCents) (*MarketSnapshot, bool)` — bool = a snapshot was actually fetched and applied. Returns `(nil, false)` on skip conditions, on provider error, **and when the provider returns `(nil, nil)`** (the real adapter and the default `MockPriceLookup` both do this — treating it as success would deref nil in `applyCLCorrection`). `SourceCountRaw`/`MarketDataObserved` need **no** stamping here — Task 5 already populated them on the receiver's embedded `MarketSnapshotData` via `applySnapshot`.

**Why no inference:** presence and pre-correction count are captured at the adapter (Task 5), so `captureMarketSnapshot` only needs to signal success/failure so the freeze in Task 7 can gate on it. The async path (`recaptureMarketSnapshotDetailed`) already returns a `snapshotResult` and already routes through `applySnapshot`, so its `MarketSnapshotData` likewise carries the three fields — no change needed there beyond what Task 9 persists.

- [ ] **Step 1: Write the failing test**

```go
func TestCaptureMarketSnapshot_SignalsSuccess(t *testing.T) {
	// service with a mock priceProv returning a snapshot → returns (snap, true),
	// and the receiver's Confidence/SourceCountRaw/MarketDataObserved are set.
	// service with a mock priceProv returning an error → returns (nil, false).
	// service with a mock priceProv returning (nil, nil) → returns (nil, false), NO panic.
	// Build the service using the existing mock-priceLookup harness in service_test.go.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestCaptureMarketSnapshot_SignalsSuccess -v`
Expected: FAIL (captureMarketSnapshot returns nothing)

- [ ] **Step 3: Change signature to signal success**

In `captureMarketSnapshot`, change the signature to `(*MarketSnapshot, bool)`. Return `(nil, false)` on the skip guard (nil provider / empty name / zero grade / generic set), on the provider-error branch, **and when `snapshot == nil`** (guard before touching it — the current code has a latent nil deref here that must not be carried forward). On success:

```go
		snapshot, err := s.priceProv.GetMarketSnapshot(ctx, card, grade)
		if err != nil {
			// ...existing warn log...
			return nil, false
		}
		if snapshot == nil {
			return nil, false // provider had no data; not an observed-zero
		}
		applyCLCorrection(snapshot, clValueCents)
		applyMarketSnapshot(r, snapshot) // copies snapshot incl. the 3 provenance fields into r's MarketSnapshotData
		return snapshot, true
```

Update the sync caller in `service_crud.go CreatePurchase` (Task 7) to consume `(snapshot, ok)`. No changes to `recaptureMarketSnapshotDetailed` here.

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./internal/domain/inventory/ -run 'TestCaptureMarketSnapshot_SignalsSuccess' -v`
Expected: build OK; PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/service_snapshots.go internal/domain/inventory/service_snapshots_test.go
git commit -m "feat(inventory): captureMarketSnapshot signals capture success"
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
func TestCreatePurchase_IgnoresClientForgedProvenance(t *testing.T) {
	// caller submits all 6 *AtPurchase pointers pre-filled with junk (e.g.
	// PopulationAtPurchase=&999999, DHConfidenceAtPurchase=&0.99) AND a provider
	// that returns an ERROR (capture fails).
	// After CreatePurchase: the 4 market fields are nil (capture failed, not forged),
	// PopulationAtPurchase == the real p.Population (or nil if 0), and
	// CLConfidenceAtPurchase is derived from the campaign — NONE of the submitted
	// junk survives.
}
```

Fill in using the existing test harness (see `TestCreateSale...` in `service_test.go` for constructing `service` with mocks). Assert the populated case, the `Population:0 → nil` case, and the forged-input case.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run 'TestCreatePurchase_FreezesCreationFacts|TestCreatePurchase_IgnoresClientForgedProvenance' -v`
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

In `CreatePurchase` (`service_crud.go`), capture the campaign and freeze. **First clear any client-supplied provenance** (P1: the handler decodes the raw body into `inventory.Purchase`, so these pointers are attacker-controllable; the `if == nil` guards below would otherwise permanently freeze forged values). Replace the discarded `GetCampaign` result:

```go
	campaign, err := s.campaigns.GetCampaign(ctx, p.CampaignID)
	if err != nil {
		return fmt.Errorf("campaign lookup: %w", err)
	}
	// Server-authoritative: discard any client-supplied frozen provenance up front,
	// so the set-once guards below can only ever freeze server-derived values.
	p.CLConfidenceAtPurchase = nil
	p.PopulationAtPurchase = nil
	p.DHConfidenceAtPurchase = nil
	p.SourceCountAtPurchase = nil
	p.ActiveListingsAtPurchase = nil
	p.SalesLast30dAtPurchase = nil

	// (a) creation-time facts, set-once
	if c, ok := ParseCLConfidenceMin(campaign.CLConfidence); ok {
		p.CLConfidenceAtPurchase = &c
	}
	if p.Population > 0 {
		pop := p.Population
		p.PopulationAtPurchase = &pop
	}

	if p.SnapshotStatus != SnapshotStatusPending {
		if snap, ok := s.captureMarketSnapshot(ctx, p, p.ToCardIdentity(), p.GradeValue, p.CLValueCents); ok {
			// (b) market-time facts, gated on confirmed capture
			conf := snap.Confidence
			p.DHConfidenceAtPurchase = &conf
			sc := p.SourceCountRaw // set on the embed by applyMarketSnapshot
			p.SourceCountAtPurchase = &sc
			if p.MarketDataObserved {
				al := p.ActiveListings
				sl := p.SalesLast30d
				p.ActiveListingsAtPurchase = &al
				p.SalesLast30dAtPurchase = &sl
			}
		}
	}
```

(The `if == nil` guards are no longer needed once the fields are cleared up front; assignment is unconditional after the clear. Field access `p.SourceCountRaw`/`p.MarketDataObserved` reads the embedded `MarketSnapshotData` populated by Task 5.)

- [ ] **Step 4: Run test**

Run: `go test ./internal/domain/inventory/ -run 'TestCreatePurchase_FreezesCreationFacts|TestCreatePurchase_IgnoresClientForgedProvenance' -v`
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
func TestCreateBulkSales_FreezesProvenance(t *testing.T) {
	// bulk item, no reason → SaleReason "discretionary"; CLValueAtSaleCents == purchase.CLValueCents;
	// *ChannelFeePctAtSale == EffectiveChannelFeePct(channel, campaign).
	// A second item with an in-person channel + invoice due within window → "invoice_pressure".
}
func TestConfirmOrdersSales_FreezesProvenance(t *testing.T) {
	// order-import (eBay/DH) row → SaleReason default, cl-at-sale + channel-fee frozen on the created sale.
}
```

Add `ErrInvalidSaleReason = errors.NewAppError(ErrCodeCampaignValidation, "invalid sale reason")` in `validation.go` alongside `ErrSaleDateBeforePurchase`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run 'TestCreateSale_(Freezes|Preserves|Rejects)|TestCreateBulkSales_FreezesProvenance|TestConfirmOrdersSales_FreezesProvenance' -v`
Expected: FAIL

- [ ] **Step 3: Add fields + freeze logic in all three paths**

In `types_core.go` Sale: add

```go
	SaleReason          string   `json:"saleReason,omitempty"`
	CLValueAtSaleCents  int      `json:"clValueAtSaleCents,omitempty"`
	ChannelFeePctAtSale *float64 `json:"channelFeePctAtSale,omitempty"`
```

Keep the `ForcedLiquidation bool` field writable — it stays a **plain app-maintained boolean** (not generated), kept in sync with `sale_reason`. Add a shared helper in `service_crud.go` that also **clears client-forged provenance** (P1: the HTTP handler decodes the raw body into the domain `Sale`, so a caller could submit `clValueAtSaleCents`/`channelFeePctAtSale`; the server must overwrite them, never trust them):

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
	// Server-authoritative: overwrite any client-supplied values.
	sa.CLValueAtSaleCents = purchase.CLValueCents
	pct := EffectiveChannelFeePct(sa.SaleChannel, campaign)
	sa.ChannelFeePctAtSale = &pct
	// Keep the plain boolean in sync with the reason (app-maintained; not generated).
	sa.ForcedLiquidation = sa.SaleReason == SaleReasonInvoicePressure
	return nil
}
```

Note `SaleReason` is a legitimate client input (preserved when valid), so it is
NOT cleared — only the two derived numeric fields and the boolean are
server-authoritative.

Call `freezeSaleProvenance(sa, purchase, campaign, IsForcedLiquidation(...))` in `CreateSale`, `CreateBulkSales`, and `ConfirmOrdersSales` right after the existing fee computation, replacing the old `sa.ForcedLiquidation = IsForcedLiquidation(...)` line. In `CreateSale`, return the error if non-nil (before persisting). For the bulk/import loops, on error append to `result.Errors` and `continue`.

Also add a **forged-input test** for the sale side:

```go
func TestCreateSale_IgnoresClientForgedProvenance(t *testing.T) {
	// caller submits CLValueAtSaleCents=99999999 and *ChannelFeePctAtSale=0.99;
	// purchase.CLValueCents=12000, ebay campaign 0.10.
	// After CreateSale: CLValueAtSaleCents==12000 and *ChannelFeePctAtSale==0.10 (forged values overwritten).
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/inventory/ -run 'TestCreateSale|TestConfirmOrders|TestCreateBulk' -v`
Expected: PASS (update any existing test that asserted `sa.ForcedLiquidation` directly to assert `SaleReason` instead)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_import_orders.go internal/domain/inventory/validation.go internal/domain/inventory/service_sale_test.go
git commit -m "feat(inventory): freeze sale provenance across all creation paths"
```

---

## Task 11: Persist sale provenance + analytics JOIN scan

**Files:**
- Modify: `internal/adapters/storage/postgres/purchase_scan_helpers.go` (saleColumns, saleColumnsAliased, scanSale, scanPurchaseWithSale)
- Modify: `internal/adapters/storage/postgres/sale_store.go` (CreateSale INSERT)
- Test: `internal/adapters/storage/postgres/sale_store_test.go`, `internal/adapters/storage/postgres/analytics_store_test.go`

**Interfaces:**
- Consumes: Sale new fields (Task 10).
- Produces: `saleColumns`/`saleColumnsAliased` extended with the 3 new columns; `CreateSale` INSERT writes them plus `forced_liquidation` (still a plain writable boolean — no column split needed).

**Note:** because Task 1 keeps `forced_liquidation` as an ordinary writable column (not generated), the existing `saleColumns` is used for both INSERT and read. No `saleInsertColumns` split is required.

- [ ] **Step 1: Write the failing tests**

(a) `sale_store_test.go`: create a sale with `SaleReason "invoice_pressure"`, `CLValueAtSaleCents 9000`, `ChannelFeePctAtSale &0.10`, `ForcedLiquidation true`; `GetSaleByPurchaseID` returns all four. (b) `analytics_store_test.go`: extend the existing roundtrip (`TestGetAllPurchasesWithSalesFieldRoundtrip`) to assert `sale_reason` and `cl_value_at_sale_cents` come back through `GetAllPurchasesWithSales`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/storage/postgres/ -run 'TestCreateSale|TestGetAllPurchasesWithSales' -v`
Expected: FAIL (INSERT/read column list missing the 3 new fields)

- [ ] **Step 3: Extend columns + scans + INSERT**

In `purchase_scan_helpers.go`:
- Extend `saleColumns` with `sale_reason, cl_value_at_sale_cents, channel_fee_pct_at_sale` (append after the existing columns; keep `forced_liquidation` in its current position and match scan-dest order).
- Extend `saleColumnsAliased` with `s.sale_reason, s.cl_value_at_sale_cents, s.channel_fee_pct_at_sale`.
- Extend `scanSale` dests and the `scanPurchaseWithSale` sale-side dests with `sql.NullString`/`sql.NullInt64`/`sql.NullFloat64` locals for the 3 new fields and assign into the Sale (SaleReason from NullString.String; CLValueAtSaleCents from NullInt64; ChannelFeePctAtSale as `*float64` from NullFloat64).

In `sale_store.go` `CreateSale`, add the 3 new columns to the `INSERT INTO campaign_sales (` + saleColumns + `)` list (they're already in `saleColumns`), bump the `VALUES ($1..$N)` count by 3, and add `s.SaleReason, s.CLValueAtSaleCents, s.ChannelFeePctAtSale` to the args. `s.ForcedLiquidation` stays in the args (writable boolean).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/storage/postgres/ -run 'TestCreateSale|TestGetAllPurchasesWithSales|TestGetSale' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/purchase_scan_helpers.go internal/adapters/storage/postgres/sale_store.go internal/adapters/storage/postgres/sale_store_test.go internal/adapters/storage/postgres/analytics_store_test.go
git commit -m "feat(db): persist sale provenance columns (reason, cl-at-sale, channel-fee)"
```

---

## Task 12: UpdateSaleReason repository method + PATCH endpoint

**Files:**
- Modify: `internal/domain/inventory/repository_sale.go` (interface)
- Modify: `internal/domain/inventory/service_interfaces.go` (service interface)
- Modify: `internal/adapters/storage/postgres/sale_store.go` (impl)
- Modify: `internal/testutil/mocks/inventory_sale_repo.go` (repo mock Fn)
- Modify: `internal/testutil/mocks/inventory_service.go` (MockInventoryService method)
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

Impl (`sale_store.go`), campaign-scoped. Because `forced_liquidation` is a plain app-maintained boolean (not generated), the UPDATE must keep it in sync with the new reason:

```go
func (ss *SaleStore) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error {
	result, err := ss.db.ExecContext(ctx,
		`UPDATE campaign_sales SET sale_reason = $1, forced_liquidation = ($1 = 'invoice_pressure'), updated_at = $2
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

Mock repo (`inventory_sale_repo.go`): add field `UpdateSaleReasonFn func(ctx context.Context, campaignID, saleID, reason string) error` and a method delegating to it (returning nil when the Fn is nil, matching the file's existing mock pattern).

Service (`service_crud.go`):

```go
func (s *service) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error {
	if !ValidSaleReasonForPatch(reason) {
		return ErrInvalidSaleReason
	}
	return s.sales.UpdateSaleReason(ctx, campaignID, saleID, reason)
}
```

Add `UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error` to the service interface (`service_interfaces.go`) **and** implement it on `MockInventoryService` (`inventory_service.go`) — the mock implements the full interface, so omitting it breaks compilation of every handler test:

```go
func (m *MockInventoryService) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error {
	if m.UpdateSaleReasonFn != nil {
		return m.UpdateSaleReasonFn(ctx, campaignID, saleID, reason)
	}
	return nil
}
```

(Add the matching `UpdateSaleReasonFn` field to the `MockInventoryService` struct.)

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
git add internal/domain/inventory/repository_sale.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_interfaces.go internal/adapters/storage/postgres/sale_store.go internal/testutil/mocks/inventory_sale_repo.go internal/testutil/mocks/inventory_service.go internal/adapters/httpserver/routes.go internal/adapters/httpserver/handlers/campaigns_purchases.go internal/adapters/httpserver/handlers/campaigns_purchases_test.go internal/adapters/storage/postgres/sale_store_test.go
git commit -m "feat(api): campaign-scoped PATCH sale_reason endpoint"
```

---

## Task 13: Bulk per-item fields (listing details + sale_reason) end-to-end

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (BulkSaleInput +4 fields)
- Modify: `internal/domain/inventory/service_crud.go` (CreateBulkSales copies + freezes)
- Modify: `web/src/js/api/campaignPurchases.ts` (createBulkSales item type)
- Modify: `web/src/react/queries/useCampaignQueries.ts` (useCreateBulkSales item type)
- Modify: `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`
- Test: `internal/domain/inventory/service_crud_test.go`

**Interfaces:**
- Consumes: `freezeSaleProvenance` (Task 10).
- Produces: `BulkSaleInput.OriginalListPriceCents int`, `.PriceReductions int`, `.DaysListed int`, `.SaleReason string` (per item); each carried onto the created `Sale`; reason honored/validated by `freezeSaleProvenance`.

- [ ] **Step 1: Write the failing test**

```go
func TestCreateBulkSales_CopiesPerItemFields(t *testing.T) {
	// items[0] = {PurchaseID, SalePriceCents, OriginalListPriceCents:1500, PriceReductions:2, DaysListed:9, SaleReason:"bulk_lot"}
	// After CreateBulkSales, the captured *Sale carries the 3 listing values AND SaleReason=="bulk_lot".
	// items[1] with SaleReason:"bogus" → that item lands in result.Errors (per-item failure), others still succeed.
}
```

Assert via the mock `SaleRepositoryMock.CreateSaleFn` capturing each `*Sale`, and via `result.Failed`/`result.Errors` for the invalid-reason item.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestCreateBulkSales_CopiesPerItemFields -v`
Expected: FAIL (unknown field)

- [ ] **Step 3: Add per-item fields + copy + freeze**

In `types_core.go`:

```go
type BulkSaleInput struct {
	PurchaseID             string `json:"purchaseId"`
	SalePriceCents         int    `json:"salePriceCents"`
	OriginalListPriceCents int    `json:"originalListPriceCents,omitempty"`
	PriceReductions        int    `json:"priceReductions,omitempty"`
	DaysListed             int    `json:"daysListed,omitempty"`
	SaleReason             string `json:"saleReason,omitempty"`
}
```

In `CreateBulkSales`, when building `sa := &Sale{...}`, add `OriginalListPriceCents: item.OriginalListPriceCents, PriceReductions: item.PriceReductions, DaysListed: item.DaysListed, SaleReason: item.SaleReason`. The existing `freezeSaleProvenance(sa, purchase, campaign, IsForcedLiquidation(...))` call (added in Task 10) already preserves a valid explicit reason, defaults when empty, and returns `ErrInvalidSaleReason` for a bad value — on that error append to `result.Errors` and `continue` (per-item failure).

Frontend request contracts (all three must change or the fields are dropped at the type boundary):
- `web/src/js/api/campaignPurchases.ts`: change the `createBulkSales` item type from `{ purchaseId: string; salePriceCents: number }[]` to include `originalListPriceCents?: number; priceReductions?: number; daysListed?: number; saleReason?: string`.
- `web/src/react/queries/useCampaignQueries.ts`: apply the identical item-type change to `useCreateBulkSales`'s `mutationFn` `data.items`.
- `web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx`: add per-row optional number inputs (mirror `RecordSaleForm`'s "Add listing details" expander) and a `sale_reason` `<select>` (options: discretionary, invoice_pressure, aging_policy, bulk_lot, show_clearout; default empty = server heuristic). Store the four per-item values in **state keyed by purchaseId** (not by array index) so that when a partial failure narrows the retry set the remaining rows keep their values. Labels must be **row-specific and accessible** (e.g. `aria-label={`Days listed for ${cardName}`}`). Include the parsed per-item values on each item of the bulk request payload.

- [ ] **Step 4: Write bulk-modal component tests**

The component has an existing suite (`BulkRecordSaleModal.test.tsx`); extend it (vitest + testing-library, matching that file's setup):

```ts
// 1. Payload: fill per-row fields for 2 items → onSuccess/mutation called with
//    items carrying originalListPriceCents/priceReductions/daysListed/saleReason
//    keyed to the right purchaseId.
// 2. Reset: closing and reopening the modal clears per-row field state.
// 3. Partial-failure retry: simulate a BulkSaleResult with one failed item; the
//    modal narrows to the failed row and that row STILL shows its entered values
//    (keyed by purchaseId, not index).
// 4. Accessibility: each row's inputs are reachable by their row-specific label.
```

- [ ] **Step 5: Run test + frontend build + component tests**

Run: `go test ./internal/domain/inventory/ -run TestCreateBulkSales_CopiesPerItemFields -v && (cd web && npm run build && npm test -- BulkRecordSaleModal)`
Expected: PASS; build OK; component tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/inventory/types_core.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_crud_test.go web/src/js/api/campaignPurchases.ts web/src/react/queries/useCampaignQueries.ts web/src/react/pages/campaign-detail/BulkRecordSaleModal.tsx web/src/react/pages/campaign-detail/BulkRecordSaleModal.test.tsx
git commit -m "feat: per-item listing details and sale_reason in bulk sale path"
```

---

## Task 14: Analysis cohorts (PNLByConfidenceBuy + ByReason)

**Files:**
- Modify: `internal/domain/portfolio/analysis_types.go`
- Create: `internal/domain/portfolio/analysis_cohorts.go` (`analysis.go` is already 392 lines; keep it under the 500-line warn threshold)
- Modify: `internal/domain/portfolio/analysis.go` (call the new cohort fn; extend `computeSplitPNL` ByReason)
- Test: `internal/domain/portfolio/analysis_cohorts_test.go`

**Interfaces:**
- Consumes: `inventory.PurchaseWithSale` (now carrying provenance pointers + sale fields).
- Produces: `CampaignAnalysis.PNLByConfidenceBuy []ConfBuyCohortRow` (**sorted** deterministically); `SplitPNL.ByReason map[string]PNLBlock` (**all 5 keys pre-populated**); `func computeConfidenceBuyCohorts(rows []inventory.PurchaseWithSale) []ConfBuyCohortRow`.

- [ ] **Step 1: Write the failing test**

```go
func TestComputeConfidenceBuyCohorts(t *testing.T) {
	// Row A: CLConfidenceAtPurchase=2, buyCost 7000, clAtPurchase 10000 → confidence "2", buyTerms "70-75".
	// Row B: CLConfidenceAtPurchase nil → confidenceBucket "unknown".
	// Row C: clAtPurchase 0/nil → buyTermsBucket "unknown".
	// Boundary D: buyCost 7500, cl 10000 → ratio exactly 0.75 → "75-80" (lower-inclusive).
	// Boundary E: buyCost 5000, cl 10000 → ratio 0.50 → "50-55" (not "<50").
	// Boundary F: ratio 1.00 → ">=100"; ratio 0.4999 → "<50".
	// avgActiveListings skips nil pointers but includes a real 0.
	// Output is sorted by (confidenceBucket, buyTermsBucket) with "unknown" last.
}
func TestByReasonSplit(t *testing.T) {
	// ByReason has all 5 keys present (zero PNLBlock when no sales); reasons land in the right buckets.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/portfolio/ -run 'TestComputeConfidenceBuyCohorts|TestByReasonSplit' -v`
Expected: FAIL (undefined types)

- [ ] **Step 3: Add types + compute (deterministic)**

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

In new file `analysis_cohorts.go`, implement `computeConfidenceBuyCohorts` with **integer-percentage** bucketing to avoid float boundary drift, lower-inclusive bands:

```go
func confidenceBucket(p inventory.Purchase) string {
	if p.CLConfidenceAtPurchase == nil {
		return "unknown"
	}
	return strconv.Itoa(*p.CLConfidenceAtPurchase)
}

func buyTermsBucket(p inventory.Purchase) string {
	if p.CLValueAtPurchaseCents <= 0 {
		return "unknown"
	}
	// Integer percent of CL, floored — exact at 5% boundaries.
	pct := p.BuyCostCents * 100 / p.CLValueAtPurchaseCents
	if pct < 50 {
		return "<50"
	}
	if pct >= 100 {
		return ">=100"
	}
	lo := (pct / 5) * 5
	return fmt.Sprintf("%d-%d", lo, lo+5)
}
```

- Group rows by `(confidenceBucket, buyTermsBucket)`; accumulate PNL from sold rows; average each provenance pointer **skipping nil**, incrementing the matching coverage counter (a stored 0 counts toward the average and coverage).
- **Sort** the output slice by `ConfidenceBucket` then `BuyTermsBucket`, with `"unknown"` sorted last in each dimension, so the JSON is deterministic across runs (maps iterate randomly in Go).
- In `analysis.go`, call `computeConfidenceBuyCohorts` where each `CampaignAnalysis` is assembled, and extend `computeSplitPNL` to initialize `ByReason` with all 5 reason keys (zero `PNLBlock`) then accumulate `ByReason[r.Sale.SaleReason]` for each sold row (skip a row whose `SaleReason` is `""` — legacy/unknown — from the 5-key buckets; it still counts in Discretionary/Forced as today).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/portfolio/ -run 'TestComputeConfidenceBuyCohorts|TestByReasonSplit|TestComputeSplitPNL' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/portfolio/analysis_types.go internal/domain/portfolio/analysis_cohorts.go internal/domain/portfolio/analysis.go internal/domain/portfolio/analysis_cohorts_test.go
git commit -m "feat(portfolio): deterministic confidence×buy cohorts and by-reason P&L split"
```

---

## Task 15: Frontend request contracts + reason select + docs

**Files:**
- Modify: `web/src/types/campaigns/core.ts` (Sale, Purchase, **CreateSaleInput**)
- Modify: `web/src/react/pages/campaign-detail/RecordSaleForm.tsx`
- Create: `web/src/react/pages/campaign-detail/RecordSaleForm.test.tsx`
- Modify: `docs/SCHEMA.md`, `docs/API.md`, `docs/OPERATIONS.md`
- Test: `RecordSaleForm.test.tsx` + frontend build + existing vitest

**Interfaces:**
- Consumes: Go JSON tags from Tasks 7, 10, 14.

- [ ] **Step 1: Add TS fields + request contract + reason select + test**

In `core.ts`:
- Extend `Sale` with `saleReason?: string; clValueAtSaleCents?: number; channelFeePctAtSale?: number;`.
- Extend `Purchase` with `clConfidenceAtPurchase?: number; populationAtPurchase?: number; dhConfidenceAtPurchase?: number; sourceCountAtPurchase?: number; activeListingsAtPurchase?: number; salesLast30dAtPurchase?: number;`.
- Extend **`CreateSaleInput`** with `saleReason?: string;` — otherwise the single-sale POST drops the field at the type boundary.

In `RecordSaleForm.tsx`, add a `sale_reason` `<select>` (options: discretionary, invoice_pressure, aging_policy, bulk_lot, show_clearout; default empty = server heuristic) and include `saleReason` in the `CreateSaleInput` payload **only when non-empty** (the empty option must be omitted from the payload so the server applies its heuristic default). (The `createSale` api client already forwards the whole `CreateSaleInput`, so no api-client change is needed once the type carries the field.)

Add `RecordSaleForm.test.tsx` (no test exists yet; use the vitest + testing-library setup from `BulkRecordSaleModal.test.tsx`):

```ts
// 1. Selecting "aging_policy" and submitting → the createSale mutation is called
//    with saleReason: "aging_policy" in the payload.
// 2. Leaving the reason on the empty/default option → the submitted payload has
//    NO saleReason key (omitted, not "").
```

- [ ] **Step 2: Update docs**

`docs/SCHEMA.md`: document the 6 nullable purchase columns, the 3 sale columns (note `channel_fee_pct_at_sale` nullable; `sale_reason` TEXT with a CHECK over the 6 allowed values incl. `''`), the `campaign_sales_derive_reason` BEFORE-INSERT/UPDATE trigger (fills an empty `sale_reason` from `forced_liquidation`), and that `forced_liquidation` stays a plain app-maintained boolean. `docs/OPERATIONS.md`: note 000022 is rollback-safe under the documented image-only rollback — new columns are additive/nullable, `forced_liquidation` is unchanged in shape, and the compatibility trigger backfills `sale_reason` for any legacy-shaped insert made by the old image during the rollback window. `docs/API.md`: add `PATCH /api/campaigns/{id}/sales/{saleID}`, the `CreateSaleInput.saleReason` + `BulkSaleInput` per-item fields, and the analysis response additions (`pnlByConfidenceBuy`, `pnl.byReason`); note imported `*_at_sale` values are record-time proxies.

- [ ] **Step 3: Build + test frontend**

Run: `cd web && npm run build && npm test -- RecordSaleForm && npm test`
Expected: build OK; RecordSaleForm test PASS; full suite PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/types/campaigns/core.ts web/src/react/pages/campaign-detail/RecordSaleForm.tsx web/src/react/pages/campaign-detail/RecordSaleForm.test.tsx docs/SCHEMA.md docs/API.md docs/OPERATIONS.md
git commit -m "feat(web/docs): provenance TS types, RecordSaleForm reason select + test, schema/API/ops docs"
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

Stage only the specific files touched by fixups (never `git add -A`, which could sweep in the worktree's untracked scratch or unrelated edits):

```bash
git status                      # review exactly what changed
git add <each fixup file by path>
git commit -m "chore: verification fixups for decision-provenance"
```

---

## Self-Review

**Spec coverage:**
- §1 migration → Task 1 (backfill tested via version-stepped v21→v22, since setupTestDB auto-migrates to head; `forced_liquidation` kept as a plain app-maintained boolean for rollback safety). Purchase 6 cols nullable → Tasks 1,7,8. Sale 3 cols + plain boolean → Tasks 1,10,11. Backfill + CHECK → Task 1.
- §2 MarketSnapshot(+Data) provenance + adapter capture of presence & pre-correction count → Task 5. Capture-success signal incl. nil-snapshot guard → Task 6. Split freeze (creation vs capture) with server-authoritative clearing of client input → Tasks 7,9. Persist → Tasks 8,9. Sale freeze all 3 paths + reason preserve/reject + forged-input clearing + bulk/order-import tests → Task 10. Column persist (no split — boolean writable) → Task 11. PATCH campaign-scoped keeps boolean in sync + MockInventoryService + repo mock (`inventory_sale_repo.go`) → Task 12. EffectiveChannelFeePct pointer via local (addressable) → Tasks 4,10.
- §3 cohorts in a new `analysis_cohorts.go` (analysis.go is 392 lines), integer-% boundary math, sorted deterministic output, all-5-key ByReason → Task 14. saleColumnsAliased scan → Task 11. Frontend request contracts (`CreateSaleInput.saleReason`, `createBulkSales`/`useCreateBulkSales` item types) + selects + bulk per-item incl. reason + component tests → Tasks 13,15. Docs (incl. OPERATIONS rollback note) → Task 15.
- Review-round-4 fixes: market presence from `price.Market != nil` not value inference (#1, Task 5); bulk sale_reason end-to-end (#2, Task 13); frontend contracts (#3, Tasks 13,15); mock compile fixes + correct filenames (#4, Task 12); version-stepped migration test (#5, Task 1); bulk/order-import sale-freeze tests (#6, Task 10); deterministic cohorts in new file (#7, Task 14); race-gate at Task 16 + scoped staging (#8, Global Constraints + Task 16).
- Review-round-5 fixes: forged immutable provenance blocked by server-clears-then-derives on purchase (Task 7 `TestCreatePurchase_IgnoresClientForgedProvenance`) and sale (Task 10 `freezeSaleProvenance` overwrites + `TestCreateSale_IgnoresClientForgedProvenance`) (P1); rollback-safe plain boolean instead of generated column, additive/nullable new cols, OPERATIONS note (Tasks 1,10,11,12,15) (P1); nil-snapshot treated as capture failure, no panic (Task 6) (P1); bulk-modal keyed state + reset + partial-failure retry + a11y component tests (Task 13) (P2).
- Review-round-6 fixes: rollback-window `sale_reason=''` loss closed by a `campaign_sales_derive_reason` BEFORE INSERT/UPDATE trigger + legacy-insert migration test (Task 1) (P1); reference spec de-staled — plain-boolean/no-split/trigger now consistent between spec and plan (P2); Task 7 verify regex includes the forged-input test (P3); `RecordSaleForm.test.tsx` proves explicit reason sent and empty option omitted (Task 15) (P3).
- Exclusions (no backfill of provenance, no async creation-fact fallback, source_count = pre-correction platforms) honored: Tasks 7/9 freeze forward-only; Task 5 pre-correction count.

**Placeholder scan:** All code steps contain concrete code or exact instructions with named symbols. Test bodies that reference the existing harness point to the specific existing test/harness to copy (Tasks 1,7,8,9,11 — DB/service harness is pre-existing and per-repo).

**Type consistency:** `EffectiveChannelFeePct`, `ParseCLConfidenceMin(_)(int, bool)`, `ValidSaleReason`/`ValidSaleReasonForPatch`, `UpdateSaleReason(ctx, campaignID, saleID, reason)` (interface + service + repo mock + MockInventoryService), `SourceCountRaw`/`MarketDataObserved` (on both MarketSnapshot and MarketSnapshotData), `freezeSaleProvenance`, `computeConfidenceBuyCohorts`, pointer fields on Purchase/Sale — all used consistently across tasks.
