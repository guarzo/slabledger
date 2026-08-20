# DH Sale Recording Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the never-working `MarkInventorySold` (`PATCH {"status":"sold"}`, which DH rejects with 422) with DH's purpose-built sale endpoint, so a card sold locally actually stops being for sale on DH, eBay and Shopify.

**Architecture:** A new `DHSaleRecorder` domain port replaces `DHSoldNotifier`. Each sale carries a server-generated idempotency key, persisted before any DH call, which makes the sale path and the reconciler converge on the same key so double-disposal is structurally impossible. Two reconciliation passes cover the two distinct drift shapes: a DH-scoped sweep for items DH still offers, and a locally-scoped recovery pass for sales DH already accepted whose handle we failed to store. Un-sell voids the DH sale and reorders its local writes so every failure point is retry-safe.

**Tech Stack:** Go 1.26, Postgres (pgx/v5 stdlib, golang-migrate embedded migrations), hexagonal architecture, table-driven tests with `internal/testutil/mocks`.

**Spec:** `docs/specs/2026-08-20-dh-record-sale-design.md`

## Global Constraints

- **Every task before Task 12 must leave `go build ./...` green.** `DHSoldNotifier` and `MarkInventorySold` stay in place until the final cleanup task; intermediate tasks only add.
- **Idempotency key format:** `"slabledger-sale-" + <server-generated UUID>`, max 255 chars, persisted as `campaign_sales.dh_idempotency_key` **before** the DH call that uses it.
- **Ordering rule (spec §5b):** mint/persist key → call DH → persist `dh_sale_id` → apply any conflict flag.
- **Un-sell ordering (spec §7):** void on DH → reset purchase columns → delete sale row. Deleting last is what preserves the void handle on failure.
- **Request-body purity (spec §2):** the DH request body must be a pure function of persisted columns. Nothing in it may read the wall clock. `sold_at` is always UTC-normalised and clamped to `[purchaseDate, sale.CreatedAt]`.
- **Retryable errors are exactly two:** `ErrDHIdempotencyInProgress` and `ErrDHLockContention`. Every other failure is permanent and must be conflict-flagged, never retried.
- **Never synthesise a sale row** from a `409 item_sold_on_channel`: DH supplies no price, and both `sold_at` and `channel` may be null.
- **Quantity is always 1.** Every purchase is a single graded slab; send `quantity: 1` explicitly and never derive `sale_price_cents` (per-unit) from `realized_profit_cents` (total) or vice versa.
- **New text columns are `NOT NULL DEFAULT ''`**, matching the existing schema convention — every predicate in this design tests `= ''`, and nullable columns would miss every pre-existing row.
- **Go commands need the corporate proxy vars unset** in this environment: prefix with `unset GOPROXY GOSUMDB GOPRIVATE HTTP_PROXY HTTPS_PROXY http_proxy https_proxy;` or export the unset once per shell.
- **`make check` must pass before the branch is done**: lint, hexagonal import check, flat-sibling rule, file size (500 warn / 600 fail), function length (262), and `scripts/check-doc-paths.sh`.

## Task order

Execute in numeric order 1 → 12. Tasks appear in the file as 1, 2, 3, 7, 8, 4, 5, 6, 9–12 (an artefact of how the sections were drafted); the numbers, not the file positions, are the execution order.

Dependencies: 1–3 are foundational. 4–6 (DH client, error classifier, adapter) and 7–8 (storage, ports and mocks) are independent of each other and may run in either order. 9–11 need both branches. 12 is terminal and **must** be last — `DHSoldNotifier` and `MarkInventorySold` stay alive until then so every intermediate task leaves `go build ./...` green.

One forward reference to watch: `inventory.SaleNeedingDHRecord` is declared in Task 8 but consumed by Task 7. Whichever you run first must add the struct.

**Task 12 has an open decision** (csvimport's inline DH notification) that must be answered before it is implemented — see the callout on that task.


---

### Task 1: Migration 000044_add_dh_sale_recording

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000044_add_dh_sale_recording.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000044_add_dh_sale_recording.down.sql`
- Test: `internal/adapters/storage/postgres/migration_000044_test.go`

**Interfaces:**
- Consumes: nothing (pure schema change)
- Produces: `campaign_sales.dh_idempotency_key`, `campaign_sales.dh_sale_id`, `campaign_sales.dh_sale_recorded_at`, `campaign_purchases.dh_sale_conflict`, `campaign_purchases.dh_sale_conflict_at`

- [ ] **Step 1: Write the up migration**

Create `internal/adapters/storage/postgres/migrations/000044_add_dh_sale_recording.up.sql`:

```sql
-- DH sale recording (see docs/specs/2026-08-20-dh-record-sale-design.md, §5).
-- Replaces the broken PATCH {"status":"sold"} transition -- DH's inventory
-- vocabulary only ever accepted in_stock/listed, so every sale-side write to
-- dh_status='sold' has 422'd since the feature shipped (see the design doc's
-- Problem section for the 2026-08-19 incident this repairs). DH now offers a
-- purpose-built POST .../inventory/:id/sale endpoint that requires a client
-- idempotency key; these columns are where that key and its outcome live.
--
-- campaign_sales.dh_idempotency_key / dh_sale_id use the same TEXT NOT NULL
-- DEFAULT '' convention as every other provenance column added since
-- migration 000040 (price_source) and 000041 (cl_value_at_*_source), and for
-- the identical reason: every predicate that reads these columns (the §5b
-- recovery query, the mint-on-first-need compare-and-set) tests `= ''` or
-- `<> ''`. A nullable column would make those predicates silently skip every
-- pre-existing row instead of treating it as "not yet recorded" -- the two
-- are operationally different (a NULL needs a NULL-aware predicate everywhere
-- it is read; forgetting one spot is invisible until it is not) even though
-- both mean "no value yet." '' is also the only encoding here that a legacy
-- (pre-migration) binary's INSERT reproduces for free: it omits the column,
-- Postgres applies the DEFAULT, and the row lands exactly where a
-- migration-aware backfill would have put it, so no rollback-window trigger
-- is needed the way 000022's campaign_sales_derive_reason_trg was for
-- sale_reason. dh_sale_recorded_at stays a nullable TIMESTAMP: it answers
-- "when," and unlike the two TEXT columns there is no ambiguity to disambiguate
-- with a sentinel -- NULL already means "never recorded" unambiguously.
--
-- campaign_purchases.dh_sale_conflict follows the same reasoning: it is the
-- §5b recovery pass's terminal-state marker (`dh_sale_conflict = ''` is the
-- predicate that stops a permanently-failed sale from being retried forever),
-- so it must be NOT NULL DEFAULT '' for the same reason as the sale-side
-- columns. dh_sale_conflict_at is nullable TIMESTAMP for the same "when" reason.
--
-- NO BACKFILL of dh_idempotency_key. Every sale row that predates this
-- migration -- including the 25 stranded in the 2026-08-15 incident -- was
-- inserted before this column existed (sale_store.go:27), so there is no
-- correct key to backfill: a key is only correct if it was persisted before
-- the DH call that used it (§5a/§5b), and no historical DH call exists for
-- these rows to have used one. Minting one now, in this migration, would
-- create a key that no DH request has ever carried -- indistinguishable later
-- from a key that really was used, and therefore just as dangerous as the
-- guessed-value backfill migration 000041 rejected for cl_value_at_purchase.
-- Legacy rows are onboarded lazily instead, by the compare-and-set in §5a,
-- the first time a writer needs a key for that row.
ALTER TABLE campaign_sales
    ADD COLUMN dh_idempotency_key   TEXT NOT NULL DEFAULT '',
    ADD COLUMN dh_sale_id           TEXT NOT NULL DEFAULT '',
    ADD COLUMN dh_sale_recorded_at  TIMESTAMP;

ALTER TABLE campaign_purchases
    ADD COLUMN dh_sale_conflict     TEXT NOT NULL DEFAULT '',
    ADD COLUMN dh_sale_conflict_at  TIMESTAMP;
```

- [ ] **Step 2: Write the down migration**

Create `internal/adapters/storage/postgres/migrations/000044_add_dh_sale_recording.down.sql`:

```sql
ALTER TABLE campaign_purchases
    DROP COLUMN dh_sale_conflict,
    DROP COLUMN dh_sale_conflict_at;

ALTER TABLE campaign_sales
    DROP COLUMN dh_idempotency_key,
    DROP COLUMN dh_sale_id,
    DROP COLUMN dh_sale_recorded_at;
```

- [ ] **Step 3: Write the migration test**

Create `internal/adapters/storage/postgres/migration_000044_test.go`. This migration is purely additive with no backfill, so the test is simpler than the backfill-style migration tests (e.g. `migration_000043_test.go`): it steps from v43 to v44, confirms the new columns exist with the right defaults on a pre-existing row, and confirms the down migration removes them cleanly.

```go
package postgres

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

// TestMigration000044_AddDHSaleRecording exercises migration 000044 in
// isolation: it lands a legacy row at v43 (pre-DH-sale-recording schema),
// steps up to v44, and confirms the new columns land at their documented
// defaults for that pre-existing row -- the whole reason this migration
// carries no backfill (see the migration's own comment).
func TestMigration000044_AddDHSaleRecording(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err)

	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	require.NoError(t, err)
	src, err := iofs.New(MigrationsFS, "migrations")
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	require.NoError(t, err)

	// Migrate to v43 (pre-DH-sale-recording), seed a legacy row there.
	require.NoError(t, m.Migrate(43))

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaigns (id, name) VALUES ('camp1', 'Test Campaign')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number, purchase_date)
		VALUES ('p1', 'camp1', 'Card One', 'CERT1', '2026-01-01')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date)
		VALUES ('s1', 'p1', 'inperson', 45000, '2026-01-15')`)
	require.NoError(t, err)

	// Step up to v44.
	require.NoError(t, m.Steps(1))

	var idempotencyKey, saleID string
	var recordedAt *string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT dh_idempotency_key, dh_sale_id, dh_sale_recorded_at::text FROM campaign_sales WHERE id = 's1'`,
	).Scan(&idempotencyKey, &saleID, &recordedAt))
	require.Equal(t, "", idempotencyKey, "legacy row must land at the '' sentinel, not a minted key")
	require.Equal(t, "", saleID)
	require.Nil(t, recordedAt, "dh_sale_recorded_at must be NULL, not a synthesized timestamp")

	var conflict string
	var conflictAt *string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT dh_sale_conflict, dh_sale_conflict_at::text FROM campaign_purchases WHERE id = 'p1'`,
	).Scan(&conflict, &conflictAt))
	require.Equal(t, "", conflict)
	require.Nil(t, conflictAt)

	// Step down to v43 and confirm the columns are gone.
	require.NoError(t, m.Steps(-1))

	var colCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE (table_name = 'campaign_sales' AND column_name IN ('dh_idempotency_key', 'dh_sale_id', 'dh_sale_recorded_at'))
		   OR (table_name = 'campaign_purchases' AND column_name IN ('dh_sale_conflict', 'dh_sale_conflict_at'))`,
	).Scan(&colCount))
	require.Equal(t, 0, colCount, "down migration must drop all five columns")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/storage/postgres/... -run TestMigration000044_AddDHSaleRecording -v`
Expected: PASS. Requires a local Postgres reachable per `requireTestDB` (`testhelper_test.go:68`) — same requirement every other `migration_0000NN_test.go` in the package has; it skips, rather than fails, without one.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/migrations/000044_add_dh_sale_recording.up.sql \
        internal/adapters/storage/postgres/migrations/000044_add_dh_sale_recording.down.sql \
        internal/adapters/storage/postgres/migration_000044_test.go
git commit -m "feat(dh): add migration 000044 for DH sale recording columns"
```

---

### Task 2: Domain types, sentinel errors, and struct fields

**Files:**
- Create: `internal/domain/inventory/dh_sale.go`
- Create: `internal/domain/inventory/dh_sale_test.go`
- Modify: `internal/domain/inventory/errors.go:8-42` (error codes, sentinel vars)
- Modify: `internal/domain/inventory/sale_types.go` (three fields on `Sale`)
- Modify: `internal/domain/inventory/core_types.go` (two fields on `Purchase`)

**Interfaces:**
- Consumes: `internal/domain/errors.NewAppError` / `ErrorCode` / `HasErrorCode` (`errors.go:186`)
- Produces: `DHSaleRecorder`, `DHSaleRequest`, `DHSaleResult`, `DHIdempotencyKeyPrefix`, `NewDHIdempotencyKey`, `IsRetryableDHSaleError`, `DHChannelSaleError`, the eight `ErrCodeDH*`/`ErrDH*` sentinels, and the new `Sale`/`Purchase` fields

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/inventory/dh_sale_test.go`:

```go
package inventory

import (
	"errors"
	"testing"
	"time"
)

func TestIsRetryableDHSaleError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"item sold on channel is not retryable", ErrDHItemSoldOnChannel, false},
		{"item unavailable is not retryable", ErrDHItemUnavailable, false},
		{"idempotency in progress IS retryable", ErrDHIdempotencyInProgress, true},
		{"reversal would collide is not retryable", ErrDHReversalWouldCollide, false},
		{"idempotency key reused is not retryable", ErrDHIdempotencyKeyReused, false},
		{"validation error is not retryable", ErrDHValidation, false},
		{"lock contention IS retryable", ErrDHLockContention, true},
		{"sale not found is not retryable", ErrDHSaleNotFound, false},
		{"unrelated error is not retryable", ErrPurchaseNotFound, false},
		{"nil is not retryable", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableDHSaleError(tt.err); got != tt.want {
				t.Errorf("IsRetryableDHSaleError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDHChannelSaleError(t *testing.T) {
	soldAt := time.Date(2026, 8, 15, 14, 22, 0, 0, time.UTC)
	channel := "ebay"

	tests := []struct {
		name string
		err  *DHChannelSaleError
	}{
		{"both fields populated", &DHChannelSaleError{SoldAt: &soldAt, Channel: &channel}},
		// Both fields may be nil per the DH contract (§3): DH can return a
		// 409 item_sold_on_channel with neither field populated.
		{"both fields nil", &DHChannelSaleError{}},
		{"only channel", &DHChannelSaleError{Channel: &channel}},
		{"only soldAt", &DHChannelSaleError{SoldAt: &soldAt}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, ErrDHItemSoldOnChannel) {
				t.Error("expected errors.Is to match ErrDHItemSoldOnChannel via Unwrap")
			}
			if tt.err.Error() == "" {
				t.Error("expected a non-empty error message")
			}
			if IsRetryableDHSaleError(tt.err) {
				t.Error("a channel sale is a permanent conflict, never retryable")
			}
		})
	}
}

func TestNewDHIdempotencyKey(t *testing.T) {
	got := NewDHIdempotencyKey(func() string { return "abc-123" })
	want := DHIdempotencyKeyPrefix + "abc-123"
	if got != want {
		t.Errorf("NewDHIdempotencyKey() = %q, want %q", got, want)
	}
	if len(got) > 255 {
		t.Errorf("key length %d exceeds DH's 255-char limit", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/... -run 'TestIsRetryableDHSaleError|TestDHChannelSaleError|TestNewDHIdempotencyKey' -v`
Expected: FAIL — compile error, `undefined: ErrDHItemSoldOnChannel` (and the other new identifiers).

- [ ] **Step 3: Add the sentinel errors to errors.go**

In `internal/domain/inventory/errors.go`, extend the existing `const` block (after `ErrCodePurchaseHasSale`):

```go
	// DH sale recording (docs/specs/2026-08-20-dh-record-sale-design.md, §3)
	ErrCodeDHItemSoldOnChannel     errors.ErrorCode = "ERR_DH_ITEM_SOLD_ON_CHANNEL"
	ErrCodeDHItemUnavailable       errors.ErrorCode = "ERR_DH_ITEM_UNAVAILABLE"
	ErrCodeDHIdempotencyInProgress errors.ErrorCode = "ERR_DH_IDEMPOTENCY_IN_PROGRESS"
	ErrCodeDHReversalWouldCollide  errors.ErrorCode = "ERR_DH_REVERSAL_WOULD_COLLIDE"
	ErrCodeDHIdempotencyKeyReused  errors.ErrorCode = "ERR_DH_IDEMPOTENCY_KEY_REUSED"
	ErrCodeDHValidation            errors.ErrorCode = "ERR_DH_VALIDATION"
	ErrCodeDHLockContention        errors.ErrorCode = "ERR_DH_LOCK_CONTENTION"
	ErrCodeDHSaleNotFound          errors.ErrorCode = "ERR_DH_SALE_NOT_FOUND"
```

And extend the `var` block (after `ErrPurchaseHasSale`):

```go
	// DH sale recording sentinels. Only ErrDHIdempotencyInProgress and
	// ErrDHLockContention are retryable byte-identical (see
	// IsRetryableDHSaleError in dh_sale.go) — every other sentinel here
	// represents a permanent outcome that a caller must flag for review
	// rather than resubmit.
	ErrDHItemSoldOnChannel     = errors.NewAppError(ErrCodeDHItemSoldOnChannel, "item already sold on another channel")
	ErrDHItemUnavailable       = errors.NewAppError(ErrCodeDHItemUnavailable, "item unavailable for sale")
	ErrDHIdempotencyInProgress = errors.NewAppError(ErrCodeDHIdempotencyInProgress, "a request with this idempotency key is already in progress")
	ErrDHReversalWouldCollide  = errors.NewAppError(ErrCodeDHReversalWouldCollide, "voiding this sale would collide with a later reversal")
	ErrDHIdempotencyKeyReused  = errors.NewAppError(ErrCodeDHIdempotencyKeyReused, "idempotency key reused with a different request body")
	ErrDHValidation            = errors.NewAppError(ErrCodeDHValidation, "DH rejected the sale request as invalid")
	ErrDHLockContention        = errors.NewAppError(ErrCodeDHLockContention, "DH inventory row is locked by a concurrent operation")
	ErrDHSaleNotFound          = errors.NewAppError(ErrCodeDHSaleNotFound, "DH sale not found")
```

- [ ] **Step 4: Write dh_sale.go**

Create `internal/domain/inventory/dh_sale.go`:

```go
package inventory

import (
	"context"
	"fmt"
	"time"

	domainerrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// DHSaleRecorder records and voids sales against DH's inventory-sale API
// (docs/specs/2026-08-20-dh-record-sale-design.md). It replaces the earlier
// DHSoldNotifier, which could only express a status PATCH -- and DH's
// inventory vocabulary never accepted the "sold" status that notifier sent.
type DHSaleRecorder interface {
	RecordInventorySale(ctx context.Context, req DHSaleRequest) (*DHSaleResult, error)
	VoidInventorySale(ctx context.Context, dhSaleID, reason string) error
}

// DHSaleRequest is the input to RecordInventorySale. Every field must be a
// pure function of persisted columns (§2 of the design) -- nothing in it may
// read the wall clock -- because it is re-sent byte-identical on any retry
// under the same IdempotencyKey, and DH treats a changed body under a reused
// key as a permanent error (ErrDHIdempotencyKeyReused).
type DHSaleRequest struct {
	DHInventoryID    int
	IdempotencyKey   string
	SalePriceCents   int       // PER UNIT, never derived from RealizedProfitCents
	SoldAt           time.Time // already UTC-normalised and clamped; see DeriveDHSoldAt
	CounterpartyName string
	Notes            string
}

// DHSaleResult is the parsed response from RecordInventorySale. Every field
// is read directly off the DH response rather than inferred, because this
// system's qty-always-1 assumption is a scope simplification, not something
// DH's response shape guarantees (see the design doc's contract-hazards table).
type DHSaleResult struct {
	DHSaleID            string
	SoldInventoryID     *int   // nullable: DH addressed a different row (partial sale) or none
	Delisted            bool   // true means "no live DH ask remains"; never inferred
	ItemStatus          string // open-ended by contract; NOT an enum
	Replayed            bool   // true when this call matched an already-recorded idempotency key
	RealizedProfitCents int    // TOTAL across units, never per-unit
}

// DHIdempotencyKeyPrefix marks a key as server-minted (§2 of the design). The
// key is never derived from a client-controllable value such as Sale.ID: a
// client can supply its own sale id (the HTTP handler decodes client input
// straight into a Sale), including a reused one, which would make a derived
// key a client-triggerable double-disposal.
const DHIdempotencyKeyPrefix = "slabledger-sale-"

// NewDHIdempotencyKey mints a fresh key by calling gen (typically a UUID
// generator) and prefixing it. Callers must persist the result via
// SaleRepository.SetSaleIdempotencyKeyIfAbsent BEFORE issuing the DH call --
// recording first and persisting after leaves a successful remote mutation
// with no key to replay (§2).
func NewDHIdempotencyKey(gen func() string) string {
	return DHIdempotencyKeyPrefix + gen()
}

// retryableDHSaleCodes is EXACTLY the two-member set the design (§3) permits
// re-issuing byte-identical: idempotency_in_progress and lock_contention.
// Every other sentinel represents a permanent outcome.
var retryableDHSaleCodes = []domainerrors.ErrorCode{
	ErrCodeDHIdempotencyInProgress,
	ErrCodeDHLockContention,
}

// IsRetryableDHSaleError reports whether the identical request may be
// re-issued. Only ErrDHIdempotencyInProgress and ErrDHLockContention qualify;
// every other DH sale sentinel (and any non-DH error) is permanent.
func IsRetryableDHSaleError(err error) bool {
	if err == nil {
		return false
	}
	for _, code := range retryableDHSaleCodes {
		if domainerrors.HasErrorCode(err, code) {
			return true
		}
	}
	return false
}

// DHChannelSaleError carries the optional detail DH returns with a 409
// item_sold_on_channel response. Both fields may be nil -- DH's contract
// permits an empty detail body even on this specific error code.
type DHChannelSaleError struct {
	SoldAt  *time.Time
	Channel *string
}

func (e *DHChannelSaleError) Error() string {
	channel := "unknown channel"
	if e.Channel != nil {
		channel = *e.Channel
	}
	if e.SoldAt != nil {
		return fmt.Sprintf("item already sold on %s at %s", channel, e.SoldAt.Format(time.RFC3339))
	}
	return fmt.Sprintf("item already sold on %s", channel)
}

// Unwrap lets errors.Is(err, ErrDHItemSoldOnChannel) match, so callers that
// only care about the sentinel (conflict flagging, retry classification)
// don't need to type-assert *DHChannelSaleError.
func (e *DHChannelSaleError) Unwrap() error {
	return ErrDHItemSoldOnChannel
}
```

`errors.go` imports `internal/domain/errors` under the plain name `errors`; `dh_sale.go` aliases it as `domainerrors` so the file can also use stdlib naming without collision.

- [ ] **Step 5: Add fields to Sale and Purchase**

In `internal/domain/inventory/sale_types.go`, add to the `Sale` struct (after the existing fields, before the closing brace):

```go
	// DHIdempotencyKey/DHSaleID/DHSaleRecordedAt track this sale's DH
	// inventory-sale recording (docs/specs/2026-08-20-dh-record-sale-design.md).
	// DHIdempotencyKey is minted server-side (never from Sale.ID -- see
	// DHIdempotencyKeyPrefix) either at creation or lazily by the §5a
	// compare-and-set. DHSaleID is DH's handle, needed to void, and is
	// written only after DH confirms. Both TEXT columns default to '' for
	// pre-migration rows; DHSaleRecordedAt is nil until DH confirms.
	DHIdempotencyKey string     `json:"dhIdempotencyKey,omitempty"`
	DHSaleID         string     `json:"dhSaleId,omitempty"`
	DHSaleRecordedAt *time.Time `json:"dhSaleRecordedAt,omitempty"`
```

In `internal/domain/inventory/core_types.go`, add to the `Purchase` struct (after the existing fields, before the closing brace):

```go
	// DHSaleConflict/DHSaleConflictAt flag a purchase whose DH sale recording
	// failed permanently or looked incomplete (delisted == false on an
	// apparent success). §5b's recovery pass treats DHSaleConflict == ""
	// as its terminal predicate: a flagged row is skipped until a human
	// clears it, which is also how a resolved conflict is re-driven.
	DHSaleConflict   string     `json:"dhSaleConflict,omitempty"`
	DHSaleConflictAt *time.Time `json:"dhSaleConflictAt,omitempty"`
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/domain/inventory/... -run 'TestIsRetryableDHSaleError|TestDHChannelSaleError|TestNewDHIdempotencyKey' -v`
Expected: PASS

Then confirm nothing else broke:

Run: `go build ./... && go test ./internal/domain/inventory/... -race`
Expected: build succeeds, existing tests still PASS

- [ ] **Step 7: Commit**

```bash
git add internal/domain/inventory/dh_sale.go \
        internal/domain/inventory/dh_sale_test.go \
        internal/domain/inventory/errors.go \
        internal/domain/inventory/sale_types.go \
        internal/domain/inventory/core_types.go
git commit -m "feat(dh): add DHSaleRecorder domain types and sale-recording sentinels"
```

---

### Task 3: DeriveDHSoldAt

**Files:**
- Create: `internal/domain/inventory/dh_sale_time.go`
- Test: `internal/domain/inventory/dh_sale_time_test.go`

**Interfaces:**
- Consumes: nothing (pure function, stdlib `time` only)
- Produces: `func DeriveDHSoldAt(saleDate, purchaseDate string, createdAt time.Time) time.Time`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/inventory/dh_sale_time_test.go`:

```go
package inventory

import (
	"testing"
	"time"
)

func TestDeriveDHSoldAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		saleDate     string
		purchaseDate string
		createdAt    time.Time
		want         time.Time
	}{
		{
			name:         "normal case: saleDate within [purchaseDate, createdAt]",
			saleDate:     "2026-01-15",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "saleDate before purchaseDate clamps up to purchaseDate",
			saleDate:     "2025-12-01",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "saleDate after createdAt clamps down to createdAt",
			saleDate:     "2026-02-01",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         createdAt,
		},
		{
			name:         "malformed saleDate falls back to createdAt",
			saleDate:     "not-a-date",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         createdAt,
		},
		{
			name:         "malformed purchaseDate omits the lower clamp",
			saleDate:     "2025-06-01",
			purchaseDate: "garbage",
			createdAt:    createdAt,
			want:         time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveDHSoldAt(tt.saleDate, tt.purchaseDate, tt.createdAt)
			if !got.Equal(tt.want) {
				t.Errorf("DeriveDHSoldAt(%q, %q, %v) = %v, want %v",
					tt.saleDate, tt.purchaseDate, tt.createdAt, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("DeriveDHSoldAt() location = %v, want UTC", got.Location())
			}
		})
	}
}

// TestDeriveDHSoldAt_TimezoneIndependence proves the result is identical
// regardless of the zone createdAt arrives in (design doc §2, finding A from
// the second review): CreatedAt is written by an unnormalised time.Now() into
// a timezone-less column, so if DeriveDHSoldAt read wall-clock fields without
// normalising first, the same stored instant could produce two different
// sold_at values depending on which zone the process ran in -- and under a
// fixed idempotency key, DH would 422 the second as idempotency_key_reused.
func TestDeriveDHSoldAt_TimezoneIndependence(t *testing.T) {
	saleDate := "2026-01-15"
	purchaseDate := "2026-01-01"

	// The same instant, expressed once in UTC and once in a fixed non-UTC
	// zone. FixedZone keeps this hermetic without touching time.Local.
	utcCreatedAt := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)
	nonUTCZone := time.FixedZone("UTC-5", -5*60*60)
	nonUTCCreatedAt := utcCreatedAt.In(nonUTCZone)

	if !utcCreatedAt.Equal(nonUTCCreatedAt) {
		t.Fatal("test setup bug: the two createdAt values must denote the same instant")
	}

	gotUTC := DeriveDHSoldAt(saleDate, purchaseDate, utcCreatedAt)
	gotNonUTC := DeriveDHSoldAt(saleDate, purchaseDate, nonUTCCreatedAt)

	if !gotUTC.Equal(gotNonUTC) {
		t.Fatalf("different instants for UTC vs non-UTC createdAt: %v vs %v", gotUTC, gotNonUTC)
	}
	if gotUTC.Location() != time.UTC || gotNonUTC.Location() != time.UTC {
		t.Fatal("DeriveDHSoldAt must always return a UTC-located time.Time")
	}

	// Also exercise the upper-clamp path under a non-UTC createdAt, since that
	// is where a naive implementation using createdAt's wall-clock fields
	// (rather than its normalised instant) would diverge.
	lateSaleDate := "2026-02-01" // after createdAt in both cases
	gotUTCClamped := DeriveDHSoldAt(lateSaleDate, purchaseDate, utcCreatedAt)
	gotNonUTCClamped := DeriveDHSoldAt(lateSaleDate, purchaseDate, nonUTCCreatedAt)
	if !gotUTCClamped.Equal(gotNonUTCClamped) {
		t.Fatalf("clamped result differs by input zone: %v vs %v", gotUTCClamped, gotNonUTCClamped)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/... -run TestDeriveDHSoldAt -v`
Expected: FAIL with a compile error, `undefined: DeriveDHSoldAt`.

- [ ] **Step 3: Write the implementation**

Create `internal/domain/inventory/dh_sale_time.go`:

```go
package inventory

import "time"

// dhSoldAtDateLayout is the shape both Sale.SaleDate and Purchase.PurchaseDate
// are stored in. Neither ValidateSale nor ValidatePurchase enforces this shape
// (both check only non-emptiness), so malformed values already exist in the
// table and DeriveDHSoldAt must degrade gracefully rather than panic or error.
const dhSoldAtDateLayout = "2006-01-02"

// DeriveDHSoldAt implements the sold_at derivation from design doc §2:
// sold_at = clamp(saleDate, lower = purchaseDate, upper = createdAt).
//
// This exists to keep the DH sale-recording request body a pure function of
// persisted columns -- never the wall clock -- so a retry under the same
// idempotency key resends a byte-identical body. createdAt is the upper
// bound (not "now") for the same reason: it is stored and stable across
// retries, whereas "now" is not.
//
// On a malformed saleDate, the result falls back to createdAt entirely (there
// is no better single instant to offer DH). On a malformed purchaseDate, only
// the lower clamp is omitted -- the parsed saleDate still passes through the
// upper clamp.
//
// The result is always .UTC(): createdAt may arrive in any location, and
// campaign_sales.created_at is a timezone-less column, so failing to normalise
// here would let the same stored instant produce two different sold_at values
// depending on the process's local zone -- a silent idempotency-key collision.
func DeriveDHSoldAt(saleDate, purchaseDate string, createdAt time.Time) time.Time {
	createdAt = createdAt.UTC()

	sale, err := time.Parse(dhSoldAtDateLayout, saleDate)
	if err != nil {
		return createdAt
	}
	sale = sale.UTC()

	if purchase, err := time.Parse(dhSoldAtDateLayout, purchaseDate); err == nil {
		purchase = purchase.UTC()
		if sale.Before(purchase) {
			sale = purchase
		}
	}

	if sale.After(createdAt) {
		sale = createdAt
	}

	return sale.UTC()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/inventory/... -run TestDeriveDHSoldAt -v`
Expected: PASS (both `TestDeriveDHSoldAt` and `TestDeriveDHSoldAt_TimezoneIndependence`)

Run: `go test ./internal/domain/inventory/... -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/dh_sale_time.go internal/domain/inventory/dh_sale_time_test.go
git commit -m "feat(dh): add DeriveDHSoldAt sold_at clamping helper"
```

---

### Task 7: Storage layer

**Files:**
- Modify: `internal/adapters/storage/postgres/purchase_scan_helpers.go`
- Modify: `internal/adapters/storage/postgres/sale_store.go`
- Modify: `internal/adapters/storage/postgres/purchase_dh_push_store.go`
- Create: `internal/adapters/storage/postgres/purchase_columns_test.go`
- Create: `internal/adapters/storage/postgres/purchase_dh_push_store_test.go` (verified: does not exist today)
- Test: `internal/adapters/storage/postgres/sale_store_test.go` (extend)

**Interfaces:**
- Consumes: `inventory.Sale.{DHIdempotencyKey,DHSaleID,DHSaleRecordedAt}` and `inventory.Purchase.{DHSaleConflict,DHSaleConflictAt}` (Task 2); migration 000044's columns (Task 1)
- Produces: `SaleStore.SetSaleIdempotencyKeyIfAbsent`, `SaleStore.SetSaleDHSaleID`, `SaleStore.ListSalesNeedingDHRecord`, `PurchaseStore.SetDHSaleConflict`, `PurchaseStore.ClearDHSaleConflict`, `PurchaseStore.ResetDHFieldsForRelistAfterVoid`

> **Note on `inventory.SaleNeedingDHRecord`:** it is declared in Task 8 (`repository_sale.go`). If you execute Task 7 first, add that struct here so the package compiles; Task 8 then only adds the interface methods.

- [ ] **Step 1: Add the three new sale columns to `saleColumns`, `saleNulls`, `saleScanDests`, and `(*saleNulls).sale()`**

In `purchase_scan_helpers.go`, extend the canonical list (order matters — this is positional):

```go
const saleColumns = `id, purchase_id, sale_channel, sale_price_cents, sale_fee_cents,
	sale_date, days_to_sell, net_profit_cents, created_at, updated_at,
	last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
	active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
	original_list_price_cents, price_reductions, days_listed, sold_at_asking_price,
	was_cracked, order_id, forced_liquidation, sale_reason, cl_value_at_sale_cents, channel_fee_pct_at_sale,
	their_comp_cents, price_source, cl_value_at_sale_observed_at, cl_value_at_sale_source,
	dh_idempotency_key, dh_sale_id, dh_sale_recorded_at`
```

`saleColumnsAliased` is `var saleColumnsAliased = aliasColumns(saleColumns, "s")` (`purchase_scan_helpers.go:76`) — **do not touch it**, it picks the new columns up automatically. That derivation is SLA-85's fix (commit `e2c3f765`); hand-editing it here would reintroduce the drift class its own comment documents.

Add three fields to `saleNulls`:

```go
type saleNulls struct {
	// ... existing fields unchanged ...
	clValueAtSaleObservedAt sql.NullString
	clValueAtSaleSource     sql.NullString
	dhIdempotencyKey        sql.NullString
	dhSaleID                sql.NullString
	dhSaleRecordedAt        sql.NullTime
}
```

Append to `saleScanDests`, in the same order as the new `saleColumns` tail:

```go
func saleScanDests(n *saleNulls) []any {
	return []any{
		&n.id, &n.purchaseID, &n.saleChannel, &n.salePriceCents, &n.saleFeeCents,
		&n.saleDate, &n.daysToSell, &n.netProfitCents, &n.createdAt, &n.updatedAt,
		&n.lastSoldCents, &n.lowestListCents, &n.conservativeCents, &n.medianCents,
		&n.activeListings, &n.salesLast30d, &n.trend30d, &n.snapshotDate, &n.snapshotJSON,
		&n.originalListPriceCents, &n.priceReductions, &n.daysListed, &n.soldAtAskingPrice,
		&n.wasCracked, &n.orderID, &n.forcedLiquidation,
		&n.saleReason, &n.clValueAtSaleCents, &n.channelFeePctAtSale,
		&n.theirCompCents, &n.priceSource,
		&n.clValueAtSaleObservedAt, &n.clValueAtSaleSource,
		&n.dhIdempotencyKey, &n.dhSaleID, &n.dhSaleRecordedAt,
	}
}
```

And in `(*saleNulls).sale()`, add to the struct literal, handling the nullable timestamp the way `channelFeePctAtSale` already is:

```go
func (n *saleNulls) sale() inventory.Sale {
	s := inventory.Sale{
		// ... existing fields unchanged ...
		CLValueAtSaleObservedAt: n.clValueAtSaleObservedAt.String,
		CLValueAtSaleSource:     n.clValueAtSaleSource.String,
		DHIdempotencyKey:        n.dhIdempotencyKey.String,
		DHSaleID:                n.dhSaleID.String,
	}
	// ... existing trailing assignments unchanged ...
	if n.dhSaleRecordedAt.Valid {
		v := n.dhSaleRecordedAt.Time
		s.DHSaleRecordedAt = &v
	}
	return s
}
```

- [ ] **Step 2: Add the two new purchase columns to `purchaseColumns`, `purchaseColumnsAliased`, and `purchaseScanDests`**

Unlike `saleColumnsAliased`, **`purchaseColumnsAliased` is hand-maintained** — a separate `const` with its own `p.`-prefixed list at `purchase_scan_helpers.go:34`. Both lists must be edited by hand, and getting them out of sync is SLA-85's failure mode transplanted to purchases. Step 8 adds the guard test that does not exist today.

```go
const purchaseColumns = `id, campaign_id, card_name, cert_number, card_number, set_name,
	...
	psa_campaign_name, attribution_source,
	dh_sale_conflict, dh_sale_conflict_at`
```

```go
const purchaseColumnsAliased = `p.id, p.campaign_id, p.card_name, p.cert_number, p.card_number, p.set_name,
	...
	p.psa_campaign_name, p.attribution_source,
	p.dh_sale_conflict, p.dh_sale_conflict_at`
```

`purchaseScanDests` — append after `psaCampaignName, attributionSource`:

```go
func purchaseScanDests(p *inventory.Purchase, psaCampaignName, attributionSource *sql.NullString) []any {
	return []any{
		// ... existing entries unchanged ...
		psaCampaignName, attributionSource,
		&p.DHSaleConflict, &p.DHSaleConflictAt,
	}
}
```

`DHSaleConflictAt` is `*time.Time`, scanned directly like `p.DHUnlistedDetectedAt` already is. `DHSaleConflict` is `NOT NULL DEFAULT ''`, so it scans straight into the `string` field like `DHHoldReason`.

- [ ] **Step 3: Update `CreateSale`'s positional INSERT — placeholders go from `$33` to `$36`**

`saleColumns` now has 36 entries, but the `VALUES` list is a hand-counted `$N` sequence with no compiler check that the lengths match. Miscounting silently shifts every argument after the mismatch into the wrong column.

```go
func (ss *SaleStore) CreateSale(ctx context.Context, s *inventory.Sale) error {
	query := `
		INSERT INTO campaign_sales (` + saleColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36)
	`
	_, err := ss.db.ExecContext(ctx, query,
		s.ID, s.PurchaseID, string(s.SaleChannel), s.SalePriceCents,
		s.SaleFeeCents, s.SaleDate, s.DaysToSell, s.NetProfitCents,
		s.CreatedAt, s.UpdatedAt,
		s.LastSoldCents, s.LowestListCents, s.ConservativeCents, s.MedianCents,
		s.ActiveListings, s.SalesLast30d, s.Trend30d, s.SnapshotDate, s.SnapshotJSON,
		s.OriginalListPriceCents, s.PriceReductions, s.DaysListed, s.SoldAtAskingPrice,
		s.WasCracked, s.OrderID, s.ForcedLiquidation,
		s.SaleReason, s.CLValueAtSaleCents, s.ChannelFeePctAtSale,
		s.TheirCompCents, s.PriceSource, s.CLValueAtSaleObservedAt, s.CLValueAtSaleSource,
		s.DHIdempotencyKey, s.DHSaleID, s.DHSaleRecordedAt,
	)
	if err != nil && isUniqueConstraintError(err) {
		return inventory.ErrDuplicateSale
	}
	if err != nil {
		return fmt.Errorf("create sale: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Implement `SetSaleIdempotencyKeyIfAbsent` (spec §5a compare-and-set)**

In `sale_store.go`:

```go
// SetSaleIdempotencyKeyIfAbsent mints an idempotency key for a sale that has
// none, via compare-and-set (spec §5a). It always returns the EFFECTIVE key:
// the one it just wrote, or — if a concurrent caller won the race — the one
// that caller wrote. Two callers can therefore never send two different keys
// for the same sale to DH.
func (ss *SaleStore) SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error) {
	var effective string
	err := ss.db.QueryRowContext(ctx, `
		UPDATE campaign_sales
		   SET dh_idempotency_key = $1
		 WHERE id = $2 AND dh_idempotency_key = ''
		RETURNING dh_idempotency_key`,
		key, saleID,
	).Scan(&effective)
	if err == nil {
		return effective, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("set sale idempotency key if absent: %w", err)
	}

	// Lost the race, or the sale already had a key from an earlier call:
	// re-read whatever is there now. A genuinely missing sale surfaces as
	// ErrSaleNotFound rather than handing an empty string to DH.
	var existing sql.NullString
	err = ss.db.QueryRowContext(ctx,
		`SELECT dh_idempotency_key FROM campaign_sales WHERE id = $1`, saleID,
	).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", inventory.ErrSaleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("re-read sale idempotency key after lost race: %w", err)
	}
	return existing.String, nil
}
```

- [ ] **Step 5: Implement `SetSaleDHSaleID`**

```go
// SetSaleDHSaleID persists the DH-issued sale handle after a successful
// RecordInventorySale call (or a replay). Without this handle a later void
// can never reach DH (spec §5b, §7).
func (ss *SaleStore) SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error {
	result, err := ss.db.ExecContext(ctx,
		`UPDATE campaign_sales SET dh_sale_id = $1, dh_sale_recorded_at = $2, updated_at = $3 WHERE id = $4`,
		dhSaleID, recordedAt.UTC(), time.Now(), saleID,
	)
	if err != nil {
		return fmt.Errorf("set sale dh sale id: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set sale dh sale id rows affected: %w", err)
	}
	if rows == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}
```

- [ ] **Step 6: Implement `ListSalesNeedingDHRecord` (spec §5b query, exactly)**

```go
// ListSalesNeedingDHRecord returns sales that need a DH-side sale handle
// (spec §5b). It is scoped by OUR state, not DH's inventory listing, so it
// catches sales DH already accepted whose handle we failed to persist — a
// window the DH-inventory-scoped sweep can never see, because a successfully
// recorded sale delists the item and drops it out of that sweep's view.
//
// dh_sale_conflict = '' is the terminal-state clause and is load-bearing:
// only two DH error codes are retryable (spec §3); every other failure is
// permanent. Without this clause, a sale that failed with a permanent error
// (e.g. 422 idempotency_key_reused) would keep its key, never gain a handle,
// and be re-attempted every cycle forever — reproducing the hourly-422 noise
// this design exists to end, just against a new endpoint. A human clearing
// dh_sale_conflict on the purchase is what re-enrolls the row.
//
// Rows with no idempotency key are intentionally included (there is no
// "key <> ''" clause) — those are the pre-migration legacy sales, including
// the 25 from the 2026-08-15 incident, which mint a key on first visit via
// SetSaleIdempotencyKeyIfAbsent (spec §5a) rather than being skipped.
func (ss *SaleStore) ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error) {
	query := `
		SELECT ` + saleColumnsAliased + `, p.dh_inventory_id, p.purchase_date
		FROM campaign_sales s
		JOIN campaign_purchases p ON p.id = s.purchase_id
		WHERE s.dh_sale_id = ''
		  AND p.dh_inventory_id <> 0
		  AND p.dh_sale_conflict = ''
		ORDER BY s.created_at ASC
		LIMIT $1
	`
	rows, err := ss.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list sales needing dh record: %w", err)
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.SaleNeedingDHRecord, error) {
		var n saleNulls
		var dhInventoryID int
		var purchaseDate string
		dests := append(saleScanDests(&n), &dhInventoryID, &purchaseDate)
		if err := rs.Scan(dests...); err != nil {
			return inventory.SaleNeedingDHRecord{}, err
		}
		return inventory.SaleNeedingDHRecord{
			Sale:          n.sale(),
			DHInventoryID: dhInventoryID,
			PurchaseDate:  purchaseDate,
		}, nil
	})
}
```

`scanRows` is the existing generic helper at `internal/adapters/storage/postgres/scan.go:11`.

- [ ] **Step 7: Implement the three `PurchaseStore` methods**

In `purchase_dh_push_store.go`, using the existing `execAndExpectRow` helper (returns `inventory.ErrPurchaseNotFound` on zero rows, matching every other method in the file). `inventory.DHStatusInStock` is verified to exist at `core_types.go:23`.

```go
// SetDHSaleConflict flags a purchase for human review after a non-retryable
// DH sale-recording error, or an apparent success with delisted == false
// (spec §4). ListSalesNeedingDHRecord excludes flagged purchases until the
// flag is cleared, which is also how a resolved conflict is re-driven.
func (ps *PurchaseStore) SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error {
	now := time.Now()
	return ps.execAndExpectRow(ctx, "set dh sale conflict",
		`UPDATE campaign_purchases
		 SET dh_sale_conflict = $1, dh_sale_conflict_at = $2, updated_at = $3
		 WHERE id = $4`,
		reason, now, now, purchaseID,
	)
}

// ClearDHSaleConflict clears a previously flagged conflict, re-enrolling the
// row in ListSalesNeedingDHRecord on the next recovery pass.
func (ps *PurchaseStore) ClearDHSaleConflict(ctx context.Context, purchaseID string) error {
	return ps.execAndExpectRow(ctx, "clear dh sale conflict",
		`UPDATE campaign_purchases
		 SET dh_sale_conflict = '', dh_sale_conflict_at = NULL, updated_at = $1
		 WHERE id = $2`,
		time.Now(), purchaseID,
	)
}

// ResetDHFieldsForRelistAfterVoid mirrors ResetDHFieldsForRepushDueToDelete
// (spec §7) but PRESERVES dh_inventory_id: a void keeps the DH inventory row
// alive, so a fresh push here would create a duplicate rather than relisting
// the same row. dh_status is set to in_stock — the void already returned the
// item to that state on DH's side — and dh_unlisted_detected_at is reused
// (deliberately, per spec §7) as the signal the auto-relist branch in
// dh_push.go:248 keys on, exactly as the delete-driven reset does.
func (ps *PurchaseStore) ResetDHFieldsForRelistAfterVoid(ctx context.Context, purchaseID string) error {
	now := time.Now()
	return ps.execAndExpectRow(ctx, "reset DH fields for relist after void",
		`UPDATE campaign_purchases
		 SET dh_push_status = $1,
		     dh_status = $2,
		     dh_channels_json = '[]',
		     dh_unlisted_detected_at = $3,
		     updated_at = $4
		 WHERE id = $5`,
		inventory.DHPushStatusPending, inventory.DHStatusInStock, now, now, purchaseID,
	)
}
```

- [ ] **Step 8: Guard against `purchaseColumns`/`purchaseColumnsAliased` drift**

Create `internal/adapters/storage/postgres/purchase_columns_test.go`, mirroring the existing `sale_columns_test.go`.

```go
package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestPurchaseColumnsMatchScanDests pins the one invariant the compiler
// cannot: the canonical column list and the scan-destination slice must have
// the same length, or every Scan silently reads the wrong value into the
// wrong field — the failure class SLA-85 (e2c3f765) fixed for sales.
func TestPurchaseColumnsMatchScanDests(t *testing.T) {
	var p inventory.Purchase
	var psaCampaignName, attributionSource sql.NullString
	cols := strings.Split(purchaseColumns, ",")
	dests := purchaseScanDests(&p, &psaCampaignName, &attributionSource)
	require.Len(t, dests, len(cols), "purchaseScanDests must have one destination per purchaseColumns entry")
}

// TestPurchaseColumnsAliasedMatchesCanonical guards the hand-maintained JOIN
// variant: unlike saleColumnsAliased, purchaseColumnsAliased has no
// aliasColumns() derivation, so a column added to one list and not the other
// compiles cleanly and only fails at query time.
func TestPurchaseColumnsAliasedMatchesCanonical(t *testing.T) {
	canonical := strings.Split(purchaseColumns, ",")
	aliased := strings.Split(purchaseColumnsAliased, ",")
	require.Len(t, aliased, len(canonical), "aliased list must cover every canonical column")
	for i := range canonical {
		want := "p." + strings.TrimSpace(canonical[i])
		require.Equal(t, want, strings.TrimSpace(aliased[i]))
	}
}
```

- [ ] **Step 9: Behavioural tests**

This repo does **not** gate postgres store tests behind `-tags integration`; they are ordinary `_test.go` files under `internal/adapters/storage/postgres/` using `requireTestDB(t)` (`testhelper_test.go:68`) and `resetSchemaAndMigrate` (`:100`). Follow that pattern — do not add a build tag or invent a harness.

Extend `sale_store_test.go`:

- `TestSetSaleIdempotencyKeyIfAbsent_MintsOnce` — create a sale with a blank key; call twice with two *different* candidate keys; assert both calls return the *first* key.
- `TestSetSaleIdempotencyKeyIfAbsent_SaleNotFound` — nonexistent sale ID; assert `errors.Is(err, inventory.ErrSaleNotFound)`.
- `TestSetSaleDHSaleID_Roundtrip` — create sale, set handle, reload via `GetSaleByPurchaseID`, assert `DHSaleID` matches and `DHSaleRecordedAt` compares with `.Equal` (not `==`).
- `TestListSalesNeedingDHRecord_Scoping` — seed three purchase+sale pairs: (a) blank `dh_sale_id`, `dh_inventory_id > 0`, blank conflict → appears; (b) same but conflict set → does NOT appear; (c) `dh_sale_id` already set → does NOT appear. Assert the returned `DHInventoryID`/`PurchaseDate` match the seeded purchase.
- `TestListSalesNeedingDHRecord_IncludesLegacyKeylessSales` — a sale with blank `dh_idempotency_key` **must** appear. This is the regression guard for the second-review finding that the original predicate excluded the 25.

Create `purchase_dh_push_store_test.go`:

- `TestResetDHFieldsForRelistAfterVoid_PreservesInventoryID` — seed a purchase with `dh_inventory_id > 0`, call the method, reload, assert `dh_inventory_id` **unchanged**, `dh_push_status == pending`, `dh_status == in_stock`, `dh_channels_json == "[]"`, `dh_unlisted_detected_at` non-nil. The preserved inventory ID is the whole point — a duplicate DH row is the failure this guards.
- `TestSetDHSaleConflict_ClearDHSaleConflict_Roundtrip` — set, assert both fields populated; clear, assert both back to zero value / nil.

- [ ] **Step 10: Verify and commit**

```bash
unset GOPROXY GOSUMDB GOPRIVATE HTTP_PROXY HTTPS_PROXY http_proxy https_proxy
go build ./...
go vet ./...
go test ./internal/adapters/storage/postgres/... -run 'Sale|PurchaseColumns|DHSaleConflict|RelistAfterVoid' -race
```

```bash
git add internal/adapters/storage/postgres/purchase_scan_helpers.go \
        internal/adapters/storage/postgres/sale_store.go \
        internal/adapters/storage/postgres/purchase_dh_push_store.go \
        internal/adapters/storage/postgres/purchase_columns_test.go \
        internal/adapters/storage/postgres/purchase_dh_push_store_test.go \
        internal/adapters/storage/postgres/sale_store_test.go
git commit -m "feat(dh): add storage layer for DH sale recording"
```

---

### Task 8: Interfaces and mocks

**Files:**
- Modify: `internal/domain/inventory/repository_sale.go`
- Modify: `internal/domain/inventory/repository_purchase_dh.go`
- Modify: `internal/testutil/mocks/inventory_purchase_repo.go`
- Modify: `internal/testutil/mocks/inmemory_campaign_store.go` (the `Fn` override fields live on the struct here)
- Modify: `internal/testutil/mocks/inmemory_sale_store.go`
- Create: `internal/testutil/mocks/inventory_dh_sale_recorder.go`

**Interfaces:**
- Consumes: `inventory.DHSaleRequest`, `inventory.DHSaleResult`, `inventory.DHSaleRecorder` (Task 2); the six store methods from Task 7
- Produces: extended `inventory.SaleRepository` and `inventory.PurchaseDHRepository`; `mocks.DHSaleRecorderMock`; extended `mocks.PurchaseRepositoryMock` and `mocks.InMemoryCampaignStore`

> **Verified:** there is **no** standalone `SaleRepositoryMock` in `internal/testutil/mocks/`. `InMemoryCampaignStore` (`inmemory_campaign_store.go:18-19`, fields `Purchases map[string]*inventory.Purchase` and `Sales map[string]*inventory.Sale`) is the only sale double, and it is what gets extended. Service-layer tests in later tasks use it plus `DHSaleRecorderMock`.

- [ ] **Step 1: Extend `SaleRepository`**

In `internal/domain/inventory/repository_sale.go` — add `"time"` to the imports:

```go
	// SetSaleIdempotencyKeyIfAbsent is a compare-and-set (spec §5a). It returns
	// the EFFECTIVE key: the one it just wrote, or the pre-existing one if
	// another writer won the race.
	SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error)
	SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error
	// ListSalesNeedingDHRecord returns sales scoped by our own state (missing
	// dh_sale_id, linked to a live DH inventory row, no open conflict) that
	// need recording or replaying against DH (spec §5b).
	ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]SaleNeedingDHRecord, error)
```

And the accompanying struct (skip if Task 7 already added it):

```go
// SaleNeedingDHRecord pairs a sale with the DH-side context its recording
// call needs but that does not live on the Sale row itself.
type SaleNeedingDHRecord struct {
	Sale          Sale
	DHInventoryID int
	PurchaseDate  string
}
```

- [ ] **Step 2: Extend `PurchaseDHRepository`**

Per CLAUDE.md's narrowest-port rule, `PurchaseDHRepository` (`repository_purchase_dh.go`) is the correct home: all three methods mutate DH v2 tracking columns exclusively and touch no card metadata, pricing, images, or campaign attribution. It is also where `ResetDHFieldsForRepushDueToDelete` already lives, for the structurally identical delete-driven reset.

```go
	// SetDHSaleConflict flags a purchase for human review after a
	// non-retryable DH sale-recording error, or an apparent success with
	// delisted == false (spec §4). A flagged purchase is excluded from
	// ListSalesNeedingDHRecord until the flag is cleared.
	SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error
	// ClearDHSaleConflict clears a previously flagged conflict, re-enrolling
	// the row in the next §5b recovery pass.
	ClearDHSaleConflict(ctx context.Context, purchaseID string) error
	// ResetDHFieldsForRelistAfterVoid mirrors ResetDHFieldsForRepushDueToDelete
	// but PRESERVES dh_inventory_id (the DH row is still alive after a void)
	// and sets dh_status to in_stock. Used by the un-sell path (spec §7) to
	// route a voided sale back through the push pipeline's auto-relist branch
	// without creating a duplicate DH inventory row.
	ResetDHFieldsForRelistAfterVoid(ctx context.Context, purchaseID string) error
```

- [ ] **Step 3: Extend `PurchaseRepositoryMock`**

In `internal/testutil/mocks/inventory_purchase_repo.go`, add three `Fn` fields grouped with the other DH fields:

```go
	SetDHSaleConflictFn               func(ctx context.Context, purchaseID, reason string) error
	ClearDHSaleConflictFn             func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRelistAfterVoidFn func(ctx context.Context, purchaseID string) error
```

And three methods, matching the zero-value-default style of the existing ones:

```go
func (m *PurchaseRepositoryMock) SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error {
	if m.SetDHSaleConflictFn != nil {
		return m.SetDHSaleConflictFn(ctx, purchaseID, reason)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ClearDHSaleConflict(ctx context.Context, purchaseID string) error {
	if m.ClearDHSaleConflictFn != nil {
		return m.ClearDHSaleConflictFn(ctx, purchaseID)
	}
	return nil
}

func (m *PurchaseRepositoryMock) ResetDHFieldsForRelistAfterVoid(ctx context.Context, purchaseID string) error {
	if m.ResetDHFieldsForRelistAfterVoidFn != nil {
		return m.ResetDHFieldsForRelistAfterVoidFn(ctx, purchaseID)
	}
	return nil
}
```

- [ ] **Step 4: Extend `InMemoryCampaignStore`**

Add the three `Fn` override fields to the struct in `inmemory_campaign_store.go` (alongside the existing sale `Fn` fields):

```go
	SetSaleIdempotencyKeyIfAbsentFn func(ctx context.Context, saleID, key string) (string, error)
	SetSaleDHSaleIDFn               func(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error
	ListSalesNeedingDHRecordFn      func(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error)
```

Add the behavioural implementations to `inmemory_sale_store.go` (add `"time"` to its imports):

```go
func (m *InMemoryCampaignStore) SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error) {
	if m.SetSaleIdempotencyKeyIfAbsentFn != nil {
		return m.SetSaleIdempotencyKeyIfAbsentFn(ctx, saleID, key)
	}
	s, ok := m.Sales[saleID]
	if !ok {
		return "", inventory.ErrSaleNotFound
	}
	// Mirror the store's compare-and-set: only the first caller's key sticks,
	// and every caller receives the effective key.
	if s.DHIdempotencyKey == "" {
		s.DHIdempotencyKey = key
	}
	return s.DHIdempotencyKey, nil
}

func (m *InMemoryCampaignStore) SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error {
	if m.SetSaleDHSaleIDFn != nil {
		return m.SetSaleDHSaleIDFn(ctx, saleID, dhSaleID, recordedAt)
	}
	s, ok := m.Sales[saleID]
	if !ok {
		return inventory.ErrSaleNotFound
	}
	s.DHSaleID = dhSaleID
	t := recordedAt
	s.DHSaleRecordedAt = &t
	return nil
}

func (m *InMemoryCampaignStore) ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error) {
	if m.ListSalesNeedingDHRecordFn != nil {
		return m.ListSalesNeedingDHRecordFn(ctx, limit)
	}
	var result []inventory.SaleNeedingDHRecord
	for _, s := range m.Sales {
		if s.DHSaleID != "" {
			continue
		}
		p, ok := m.Purchases[s.PurchaseID]
		if !ok || p.DHInventoryID == 0 || p.DHSaleConflict != "" {
			continue
		}
		if limit > 0 && len(result) >= limit {
			break
		}
		result = append(result, inventory.SaleNeedingDHRecord{
			Sale:          *s,
			DHInventoryID: p.DHInventoryID,
			PurchaseDate:  p.PurchaseDate,
		})
	}
	return result, nil
}
```

- [ ] **Step 5: Add `DHSaleRecorderMock`**

Create `internal/testutil/mocks/inventory_dh_sale_recorder.go`. Do **not** delete `DHSoldNotifierMock` — Task 12 removes it once `DHSoldNotifier` is retired everywhere.

```go
package mocks

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// DHSaleRecorderMock is a test double for inventory.DHSaleRecorder. It records
// every RecordInventorySale/VoidInventorySale call in full — not just the
// inventory ID — so tests can assert on the idempotency key and SoldAt that
// were actually sent. That is exactly what spec §2's pure-function-of-
// persisted-columns invariant needs: a test asserting the same key and SoldAt
// across a retry catches a regression a call-count assertion would miss.
type DHSaleRecorderMock struct {
	RecordInventorySaleFn func(ctx context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error)
	VoidInventorySaleFn   func(ctx context.Context, dhSaleID, reason string) error

	mu       sync.Mutex
	recorded []inventory.DHSaleRequest
	voided   []DHVoidCall
}

// DHVoidCall is one recorded VoidInventorySale invocation.
type DHVoidCall struct {
	DHSaleID string
	Reason   string
}

func (m *DHSaleRecorderMock) RecordInventorySale(ctx context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
	m.mu.Lock()
	m.recorded = append(m.recorded, req)
	m.mu.Unlock()
	if m.RecordInventorySaleFn != nil {
		return m.RecordInventorySaleFn(ctx, req)
	}
	return &inventory.DHSaleResult{
		DHSaleID:   "dh-sale-mock",
		Delisted:   true,
		ItemStatus: "sold",
	}, nil
}

func (m *DHSaleRecorderMock) VoidInventorySale(ctx context.Context, dhSaleID, reason string) error {
	m.mu.Lock()
	m.voided = append(m.voided, DHVoidCall{DHSaleID: dhSaleID, Reason: reason})
	m.mu.Unlock()
	if m.VoidInventorySaleFn != nil {
		return m.VoidInventorySaleFn(ctx, dhSaleID, reason)
	}
	return nil
}

// RecordedSales returns every DHSaleRequest passed to RecordInventorySale, in
// order — including the idempotency key and SoldAt, so a caller can assert
// that a retry sent a byte-identical request.
func (m *DHSaleRecorderMock) RecordedSales() []inventory.DHSaleRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]inventory.DHSaleRequest(nil), m.recorded...)
}

// VoidedSales returns every VoidInventorySale call, in order.
func (m *DHSaleRecorderMock) VoidedSales() []DHVoidCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DHVoidCall(nil), m.voided...)
}

var _ inventory.DHSaleRecorder = (*DHSaleRecorderMock)(nil)
```

- [ ] **Step 6: Compile check**

The `var _ inventory.SaleRepository = (*SaleStore)(nil)` and equivalent `PurchaseStore` assertions already exist; this step verifies them rather than adding code.

```bash
unset GOPROXY GOSUMDB GOPRIVATE HTTP_PROXY HTTPS_PROXY http_proxy https_proxy
go build ./...
go vet ./...
go test ./internal/testutil/... ./internal/domain/inventory/... -race
```

If the build fails because a store is missing one of the new methods, reconcile the **name against this plan**, not the other way round — do not soften an interface to match a drifted implementation.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/inventory/repository_sale.go \
        internal/domain/inventory/repository_purchase_dh.go \
        internal/testutil/mocks/inventory_purchase_repo.go \
        internal/testutil/mocks/inmemory_campaign_store.go \
        internal/testutil/mocks/inmemory_sale_store.go \
        internal/testutil/mocks/inventory_dh_sale_recorder.go
git commit -m "feat(dh): extend sale and purchase-dh repository ports for sale recording"
```

---

### Task 4: DH client sale + void endpoints

**Files:**
- Create: `internal/adapters/clients/dh/inventory_sale.go`
- Test: `internal/adapters/clients/dh/inventory_sale_test.go`

**Interfaces:**
- Consumes: `doEnterprise` / `postEnterprise` on `*dh.Client` — no changes to `client.go`
- Produces: `dh.InventorySaleRequest/Response`, `dh.VoidSaleRequest/Response`, `(*Client).RecordInventorySale`, `(*Client).VoidInventorySale`

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/clients/dh/inventory_sale_test.go`:

```go
package dh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_RecordInventorySale(t *testing.T) {
	var gotHeader, gotPath string
	var gotBody InventorySaleRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Idempotency-Key")
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(InventorySaleResponse{
			SaleID:              "sale_123",
			DHInventoryID:       98765,
			SoldInventoryID:     nil, // DH sends null; must parse to a nil pointer
			ItemStatus:          "sold",
			Delisted:            true,
			RealizedProfitCents: 1500,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, WithEnterpriseKey("test_key"))
	resp, err := c.RecordInventorySale(context.Background(), 98765, "slabledger-sale-abc123", InventorySaleRequest{
		SalePriceCents:   45000,
		Quantity:         5, // deliberately wrong; must be forced to 1
		SoldAt:           "2026-08-15T14:22:00Z",
		CounterpartyName: "Jane Buyer",
	})

	require.NoError(t, err)
	require.Equal(t, "slabledger-sale-abc123", gotHeader)
	require.Equal(t, "/api/v1/enterprise/inventory/98765/sale", gotPath)
	require.Equal(t, 1, gotBody.Quantity, "quantity must always be forced to 1: every purchase is a single slab")
	require.Equal(t, "sale_123", resp.SaleID)
	require.Nil(t, resp.SoldInventoryID, "null sold_inventory_id must parse to a nil pointer, not a zero int")
	require.True(t, resp.Delisted)
	require.Equal(t, 1500, resp.RealizedProfitCents)
}

func TestClient_VoidInventorySale(t *testing.T) {
	var gotPath string
	var gotBody VoidSaleRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VoidSaleResponse{Reversed: true})
	}))
	defer server.Close()

	c := NewClient(server.URL, WithEnterpriseKey("test_key"))
	resp, err := c.VoidInventorySale(context.Background(), "sale_123", VoidSaleRequest{Reason: "returned"})

	require.NoError(t, err)
	require.Equal(t, "/api/v1/enterprise/sales/sale_123/void", gotPath)
	require.Equal(t, "returned", gotBody.Reason)
	require.True(t, resp.Reversed)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/dh/... -run 'TestClient_RecordInventorySale|TestClient_VoidInventorySale' -v`
Expected: FAIL — compile error, `undefined: InventorySaleRequest`.

- [ ] **Step 3: Write the implementation**

Create `internal/adapters/clients/dh/inventory_sale.go`:

```go
package dh

import (
	"context"
	"fmt"
)

// InventorySaleRequest is the body for POST .../inventory/:id/sale.
type InventorySaleRequest struct {
	SalePriceCents   int    `json:"sale_price_cents"`
	Quantity         int    `json:"quantity,omitempty"`
	SoldAt           string `json:"sold_at,omitempty"`
	CounterpartyName string `json:"counterparty_name,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

// InventorySaleResponse is DH's response to a recorded sale. ItemStatus is
// open-ended by contract, never an enum; SoldInventoryID is nullable and, at
// our qty-1 scope, is read but never assumed to equal the addressed id.
type InventorySaleResponse struct {
	SaleID              string `json:"sale_id"`
	DHInventoryID       int    `json:"dh_inventory_id"`
	SoldInventoryID     *int   `json:"sold_inventory_id"`
	ItemStatus          string `json:"item_status"`
	Delisted            bool   `json:"delisted"`
	Replayed            bool   `json:"replayed"`
	RealizedProfitCents int    `json:"realized_profit_cents"`
}

// VoidSaleRequest is the body for POST .../sales/:sale_id/void.
type VoidSaleRequest struct {
	Reason string `json:"reason,omitempty"`
}

// VoidSaleResponse is DH's response to a void. Reversed is false, not an
// error, when the sale was already void.
type VoidSaleResponse struct {
	Reversed bool                `json:"reversed"`
	Items    []InventoryListItem `json:"items"`
}

// RecordInventorySale POSTs .../inventory/:id/sale with the required
// Idempotency-Key header. Quantity is always forced to 1: every SlabLedger
// purchase is a single graded slab (design doc, "Contract hazards"), so
// partial-sale semantics never apply here.
func (c *Client) RecordInventorySale(ctx context.Context, inventoryID int, idempotencyKey string, req InventorySaleRequest) (*InventorySaleResponse, error) {
	req.Quantity = 1
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/sale", c.baseURL, inventoryID)

	var resp InventorySaleResponse
	extraHeaders := map[string]string{"Idempotency-Key": idempotencyKey}
	if err := c.doEnterprise(ctx, "POST", fullURL, req, &resp, extraHeaders); err != nil {
		return nil, err
	}
	return &resp, nil
}

// VoidInventorySale POSTs .../sales/:sale_id/void.
func (c *Client) VoidInventorySale(ctx context.Context, dhSaleID string, req VoidSaleRequest) (*VoidSaleResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/sales/%s/void", c.baseURL, dhSaleID)

	var resp VoidSaleResponse
	if err := c.postEnterprise(ctx, fullURL, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/clients/dh/... -run 'TestClient_RecordInventorySale|TestClient_VoidInventorySale' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/dh/inventory_sale.go internal/adapters/clients/dh/inventory_sale_test.go
git commit -m "feat(dh): add sale-recording and void client endpoints"
```

---

### Task 5: Error classifier

**Files:**
- Create: `internal/adapters/clients/dhlisting/dh_sale_errors.go`
- Test: `internal/adapters/clients/dhlisting/dh_sale_errors_test.go`

**Interfaces:**
- Consumes: `httpx.UpstreamError{StatusCode, Body}` (`upstream_error.go:22-29`); the eight domain sentinels and `inventory.DHChannelSaleError` (Task 2)
- Produces: `func classifyDHSaleError(err error) error`

> **Package note:** `classifyDHSaleError` is unexported, so its test file must be `package dhlisting` (internal). The adapter tests in Task 6 are `package dhlisting_test` (external, matching the existing `adapter_test.go:1`). Both may coexist in the directory.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/clients/dhlisting/dh_sale_errors_test.go`:

```go
package dhlisting

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestClassifyDHSaleError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{"409 item_sold_on_channel with values", 409, `{"code":"item_sold_on_channel","sold_at":"2026-08-15T14:22:00Z","channel":"ebay"}`, inventory.ErrDHItemSoldOnChannel},
		{"409 item_sold_on_channel with nulls", 409, `{"code":"item_sold_on_channel","sold_at":null,"channel":null}`, inventory.ErrDHItemSoldOnChannel},
		{"409 item_unavailable", 409, `{"code":"item_unavailable"}`, inventory.ErrDHItemUnavailable},
		{"409 idempotency_in_progress", 409, `{"code":"idempotency_in_progress"}`, inventory.ErrDHIdempotencyInProgress},
		{"409 reversal_would_collide", 409, `{"code":"reversal_would_collide"}`, inventory.ErrDHReversalWouldCollide},
		{"409 unknown code degrades by status class", 409, `{"code":"something_new"}`, inventory.ErrDHValidation},
		{"409 absent code degrades by status class", 409, `{}`, inventory.ErrDHValidation},
		{"409 malformed JSON degrades by status class", 409, `not json`, inventory.ErrDHValidation},
		{"409 empty body degrades by status class", 409, ``, inventory.ErrDHValidation},
		{"422 idempotency_key_reused", 422, `{"code":"idempotency_key_reused"}`, inventory.ErrDHIdempotencyKeyReused},
		{"422 code null degrades by status class", 422, `{"code":null}`, inventory.ErrDHValidation},
		{"503 lock contention", 503, `{"code":"lock_contention"}`, inventory.ErrDHLockContention},
		{"503 with no body", 503, ``, inventory.ErrDHLockContention},
		{"404 not found", 404, `{"code":"not_found"}`, inventory.ErrDHSaleNotFound},
		{"extra unexpected fields never hard-fail", 409, `{"code":"item_unavailable","extra":{"nested":true},"more":[1,2,3]}`, inventory.ErrDHItemUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpx.UpstreamError{
				Provider:   "dh",
				Op:         "POST /api/v1/enterprise/inventory/1/sale",
				StatusCode: tt.statusCode,
				Body:       tt.body,
			}
			require.ErrorIs(t, classifyDHSaleError(upstream), tt.wantErr)
		})
	}

	t.Run("item_sold_on_channel carries null SoldAt and Channel", func(t *testing.T) {
		upstream := &httpx.UpstreamError{StatusCode: 409, Body: `{"code":"item_sold_on_channel","sold_at":null,"channel":null}`}
		var channelErr *inventory.DHChannelSaleError
		require.ErrorAs(t, classifyDHSaleError(upstream), &channelErr)
		require.Nil(t, channelErr.SoldAt)
		require.Nil(t, channelErr.Channel)
	})

	t.Run("item_sold_on_channel carries populated SoldAt and Channel", func(t *testing.T) {
		upstream := &httpx.UpstreamError{StatusCode: 409, Body: `{"code":"item_sold_on_channel","sold_at":"2026-08-15T14:22:00Z","channel":"ebay"}`}
		var channelErr *inventory.DHChannelSaleError
		require.ErrorAs(t, classifyDHSaleError(upstream), &channelErr)
		require.Equal(t, time.Date(2026, 8, 15, 14, 22, 0, 0, time.UTC), *channelErr.SoldAt)
		require.Equal(t, "ebay", *channelErr.Channel)
	})

	t.Run("unmapped status passes through unchanged", func(t *testing.T) {
		upstream := &httpx.UpstreamError{StatusCode: 400, Body: `{"code":"whatever"}`}
		require.Same(t, upstream, classifyDHSaleError(upstream))
	})

	t.Run("non-UpstreamError passes through unchanged", func(t *testing.T) {
		wantErr := errors.New("network failure")
		require.Same(t, wantErr, classifyDHSaleError(wantErr))
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/dhlisting/... -run TestClassifyDHSaleError -v`
Expected: FAIL — compile error, `undefined: classifyDHSaleError`.

- [ ] **Step 3: Write the implementation**

Create `internal/adapters/clients/dhlisting/dh_sale_errors.go`:

```go
package dhlisting

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// dhSaleErrorBody is the documented DH error shape for the sale/void
// endpoints (design doc §3). DH validates only successful responses against
// its schema, so error bodies are not machine-checked on their side: every
// field here is parsed defensively. A field that is absent, null, malformed,
// or of the wrong type must never panic or hard-fail — it degrades to
// classifying purely by HTTP status.
type dhSaleErrorBody struct {
	Code    string     `json:"code"`
	SoldAt  *time.Time `json:"sold_at"`
	Channel *string    `json:"channel"`
}

// classifyDHSaleError maps an httpx.UpstreamError to an inventory sentinel
// per design doc §3. Errors that are not UpstreamErrors (network failures,
// timeouts, circuit-breaker trips) pass through unchanged.
func classifyDHSaleError(err error) error {
	var ue *httpx.UpstreamError
	if !errors.As(err, &ue) {
		return err
	}

	var body dhSaleErrorBody
	// Ignore the unmarshal error: a malformed or empty body leaves body
	// zero-valued, which falls through to the by-status-class default below.
	_ = json.Unmarshal([]byte(ue.Body), &body)

	switch ue.StatusCode {
	case 409:
		switch body.Code {
		case "item_sold_on_channel":
			return &inventory.DHChannelSaleError{SoldAt: body.SoldAt, Channel: body.Channel}
		case "item_unavailable":
			return fmt.Errorf("%w: %w", inventory.ErrDHItemUnavailable, err)
		case "idempotency_in_progress":
			return fmt.Errorf("%w: %w", inventory.ErrDHIdempotencyInProgress, err)
		case "reversal_would_collide":
			return fmt.Errorf("%w: %w", inventory.ErrDHReversalWouldCollide, err)
		default:
			return fmt.Errorf("%w: %w", inventory.ErrDHValidation, err)
		}
	case 422:
		if body.Code == "idempotency_key_reused" {
			return fmt.Errorf("%w: %w", inventory.ErrDHIdempotencyKeyReused, err)
		}
		return fmt.Errorf("%w: %w", inventory.ErrDHValidation, err)
	case 503:
		return fmt.Errorf("%w: %w", inventory.ErrDHLockContention, err)
	case 404:
		return fmt.Errorf("%w: %w", inventory.ErrDHSaleNotFound, err)
	default:
		return err
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/clients/dhlisting/... -run TestClassifyDHSaleError -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/dhlisting/dh_sale_errors.go internal/adapters/clients/dhlisting/dh_sale_errors_test.go
git commit -m "feat(dh): classify DH sale and void error responses into domain sentinels"
```

---

### Task 6: InventoryAdapter implements DHSaleRecorder

**Files:**
- Modify: `internal/adapters/clients/dhlisting/adapter.go` (imports; client interface + constructor at `:113-139`; new methods before the `var _` block at `:226`)
- Modify: `internal/adapters/clients/dhlisting/adapter_test.go:53-79` (extend `stubInventoryClient` so it still satisfies the widened interface)
- Test: `internal/adapters/clients/dhlisting/adapter_sale_test.go` (new)
- Test: `internal/adapters/clients/dhlisting/dh_sale_fake_test.go` (new — the contract-enforcing fake)

`adapter.go` is 283 lines today; these methods add ~60, landing well inside the 500-line warning threshold, so no split is needed.

**Interfaces:**
- Consumes: Task 4's client types and methods; `classifyDHSaleError` (Task 5); `inventory.DHSaleRecorder/Request/Result` and `ErrDHSaleNotFound` (Task 2)
- Produces: `*InventoryAdapter` satisfying `inventory.DHSaleRecorder`

- [ ] **Step 1: Extend `stubInventoryClient` in `adapter_test.go`**

The widened client interface breaks compilation until the stub implements it. Add to `adapter_test.go` (`package dhlisting_test`), after the existing struct and its `SyncChannels` method:

```go
type recordSaleCall struct {
	inventoryID    int
	idempotencyKey string
	req            dh.InventorySaleRequest
}

type voidSaleCall struct {
	dhSaleID string
	req      dh.VoidSaleRequest
}
```

Add these fields to `stubInventoryClient`:

```go
	recordSaleFn func(ctx context.Context, inventoryID int, idempotencyKey string, req dh.InventorySaleRequest) (*dh.InventorySaleResponse, error)
	voidSaleFn   func(ctx context.Context, dhSaleID string, req dh.VoidSaleRequest) (*dh.VoidSaleResponse, error)

	recordSaleCalls []recordSaleCall
	voidSaleCalls   []voidSaleCall
```

And these methods:

```go
func (s *stubInventoryClient) RecordInventorySale(ctx context.Context, inventoryID int, idempotencyKey string, req dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
	s.recordSaleCalls = append(s.recordSaleCalls, recordSaleCall{inventoryID, idempotencyKey, req})
	if s.recordSaleFn != nil {
		return s.recordSaleFn(ctx, inventoryID, idempotencyKey, req)
	}
	return &dh.InventorySaleResponse{}, nil
}

func (s *stubInventoryClient) VoidInventorySale(ctx context.Context, dhSaleID string, req dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
	s.voidSaleCalls = append(s.voidSaleCalls, voidSaleCall{dhSaleID, req})
	if s.voidSaleFn != nil {
		return s.voidSaleFn(ctx, dhSaleID, req)
	}
	return &dh.VoidSaleResponse{}, nil
}
```

`rotatingInventoryClient` embeds `stubInventoryClient`, so it inherits both automatically — no change needed there.

- [ ] **Step 2: Write the adapter unit tests**

Create `internal/adapters/clients/dhlisting/adapter_sale_test.go`:

```go
package dhlisting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestInventoryAdapter_RecordInventorySale(t *testing.T) {
	t.Run("translates the request, formats sold_at as UTC RFC3339, translates the response", func(t *testing.T) {
		client := &stubInventoryClient{
			recordSaleFn: func(_ context.Context, inventoryID int, _ string, _ dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
				return &dh.InventorySaleResponse{
					SaleID:              "sale_1",
					DHInventoryID:       inventoryID,
					SoldInventoryID:     dh.IntPtr(inventoryID),
					ItemStatus:          "sold",
					Delisted:            true,
					RealizedProfitCents: 2500,
				}, nil
			},
		}

		// A non-UTC input zone: the adapter must normalise before formatting,
		// or a retry from a differently-zoned process would change the body.
		soldAt := time.Date(2026, 8, 15, 9, 22, 0, 0, time.FixedZone("EDT", -4*3600))
		result, err := adapter.NewInventoryAdapter(client).RecordInventorySale(context.Background(), inventory.DHSaleRequest{
			DHInventoryID:    314,
			IdempotencyKey:   "slabledger-sale-abc",
			SalePriceCents:   45000,
			SoldAt:           soldAt,
			CounterpartyName: "Jane Buyer",
			Notes:            "in person",
		})

		require.NoError(t, err)
		require.Len(t, client.recordSaleCalls, 1)
		call := client.recordSaleCalls[0]
		require.Equal(t, 314, call.inventoryID)
		require.Equal(t, "slabledger-sale-abc", call.idempotencyKey)
		require.Equal(t, 45000, call.req.SalePriceCents)
		require.Equal(t, "2026-08-15T13:22:00Z", call.req.SoldAt, "sold_at must be UTC-normalised RFC3339 regardless of the input zone")
		require.Equal(t, "Jane Buyer", call.req.CounterpartyName)

		require.Equal(t, "sale_1", result.DHSaleID)
		require.Equal(t, 314, *result.SoldInventoryID)
		require.True(t, result.Delisted)
		require.Equal(t, 2500, result.RealizedProfitCents)
	})

	t.Run("classifies a client error into a domain sentinel", func(t *testing.T) {
		client := &stubInventoryClient{
			recordSaleFn: func(context.Context, int, string, dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
				return nil, &httpx.UpstreamError{StatusCode: 409, Body: `{"code":"item_unavailable"}`}
			},
		}
		_, err := adapter.NewInventoryAdapter(client).RecordInventorySale(context.Background(), inventory.DHSaleRequest{DHInventoryID: 1})
		require.ErrorIs(t, err, inventory.ErrDHItemUnavailable)
	})
}

func TestInventoryAdapter_VoidInventorySale(t *testing.T) {
	t.Run("forwards dh sale id and reason", func(t *testing.T) {
		client := &stubInventoryClient{}
		require.NoError(t, adapter.NewInventoryAdapter(client).VoidInventorySale(context.Background(), "sale_1", "returned"))
		require.Len(t, client.voidSaleCalls, 1)
		require.Equal(t, "sale_1", client.voidSaleCalls[0].dhSaleID)
		require.Equal(t, "returned", client.voidSaleCalls[0].req.Reason)
	})

	t.Run("treats a not-found sale as success, not an error", func(t *testing.T) {
		client := &stubInventoryClient{
			voidSaleFn: func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
				return nil, &httpx.UpstreamError{StatusCode: 404, Body: `{"code":"not_found"}`}
			},
		}
		err := adapter.NewInventoryAdapter(client).VoidInventorySale(context.Background(), "sale_missing", "")
		require.NoError(t, err, "404 covers not-found/foreign/mirror/UI-created deals; none should fail a local un-sell")
	})

	t.Run("surfaces a non-404 error", func(t *testing.T) {
		client := &stubInventoryClient{
			voidSaleFn: func(context.Context, string, dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
				return nil, errors.New("network fail")
			},
		}
		require.Error(t, adapter.NewInventoryAdapter(client).VoidInventorySale(context.Background(), "sale_1", ""))
	})
}
```

- [ ] **Step 3: Write the contract-enforcing fake**

This is the replacement for `TestInventoryAdapter_MarkInventorySold`, which asserted against a permissive mock that accepted any payload — the direct reason a body the real API 422s on reached production. Create `internal/adapters/clients/dhlisting/dh_sale_fake_test.go`:

```go
package dhlisting_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// fakeDHSaleServer enforces the DH contract rather than accepting whatever we
// send: it requires the Idempotency-Key header, replays on same-key+same-body,
// and 422s on same-key+different-body — including the identical body against a
// different inventory id, the trap the design doc calls out explicitly.
type fakeDHSaleServer struct {
	*httptest.Server

	mu     sync.Mutex
	sales  map[string]recordedSale // keyed by Idempotency-Key
	voided map[string]bool         // keyed by dh_sale_id
	nextID int
}

type recordedSale struct {
	inventoryID int
	bodyHash    string
	saleID      string
}

func newFakeDHSaleServer(t *testing.T) *fakeDHSaleServer {
	t.Helper()
	f := &fakeDHSaleServer{sales: map[string]recordedSale{}, voided: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeDHSaleServer) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/sale"):
		f.handleSale(w, r)
	case strings.HasSuffix(r.URL.Path, "/void"):
		f.handleVoid(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeDHSaleServer) handleSale(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 255 {
		writeJSONError(w, http.StatusUnprocessableEntity, "validation", "Idempotency-Key must be 1-255 characters")
		return
	}

	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	inventoryID, _ := strconv.Atoi(segments[len(segments)-2])

	var req dh.InventorySaleRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	bodyBytes, _ := json.Marshal(req)
	hash := fmt.Sprintf("%x", sha256.Sum256(bodyBytes))

	f.mu.Lock()
	defer f.mu.Unlock()

	if existing, ok := f.sales[key]; ok {
		if existing.bodyHash == hash && existing.inventoryID == inventoryID {
			writeJSON(w, http.StatusOK, dh.InventorySaleResponse{
				SaleID: existing.saleID, DHInventoryID: inventoryID,
				ItemStatus: "sold", Delisted: true, Replayed: true,
			})
			return
		}
		writeJSONError(w, http.StatusUnprocessableEntity, "idempotency_key_reused", "idempotency key reused with a different request")
		return
	}

	f.nextID++
	saleID := fmt.Sprintf("sale_%d", f.nextID)
	f.sales[key] = recordedSale{inventoryID: inventoryID, bodyHash: hash, saleID: saleID}

	writeJSON(w, http.StatusOK, dh.InventorySaleResponse{
		SaleID: saleID, DHInventoryID: inventoryID,
		ItemStatus: "sold", Delisted: true, Replayed: false,
	})
}

func (f *fakeDHSaleServer) handleVoid(w http.ResponseWriter, r *http.Request) {
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	saleID := segments[len(segments)-2]

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.voided[saleID] {
		writeJSON(w, http.StatusOK, dh.VoidSaleResponse{Reversed: false})
		return
	}
	for _, s := range f.sales {
		if s.saleID == saleID {
			f.voided[saleID] = true
			writeJSON(w, http.StatusOK, dh.VoidSaleResponse{Reversed: true})
			return
		}
	}
	http.NotFound(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}

func TestInventoryAdapter_RecordInventorySale_ContractFake(t *testing.T) {
	fake := newFakeDHSaleServer(t)
	a := adapter.NewInventoryAdapter(dh.NewClient(fake.Server.URL, dh.WithEnterpriseKey("test_key")))

	req := inventory.DHSaleRequest{
		DHInventoryID:  100,
		IdempotencyKey: "slabledger-sale-key-1",
		SalePriceCents: 45000,
	}

	result, err := a.RecordInventorySale(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.Replayed, "first call records")

	result, err = a.RecordInventorySale(context.Background(), req)
	require.NoError(t, err)
	require.True(t, result.Replayed, "identical retry must replay, not double-dispose")

	t.Run("same key, different price: idempotency_key_reused", func(t *testing.T) {
		mutated := req
		mutated.SalePriceCents = 50000
		_, err := a.RecordInventorySale(context.Background(), mutated)
		require.ErrorIs(t, err, inventory.ErrDHIdempotencyKeyReused)
	})

	t.Run("same key, same body, different inventory id: idempotency_key_reused", func(t *testing.T) {
		mutated := req
		mutated.DHInventoryID = 200
		_, err := a.RecordInventorySale(context.Background(), mutated)
		require.ErrorIs(t, err, inventory.ErrDHIdempotencyKeyReused)
	})

	t.Run("missing idempotency key is rejected", func(t *testing.T) {
		noKey := req
		noKey.IdempotencyKey = ""
		noKey.DHInventoryID = 999
		_, err := a.RecordInventorySale(context.Background(), noKey)
		require.ErrorIs(t, err, inventory.ErrDHValidation)
	})
}

func TestInventoryAdapter_VoidInventorySale_ContractFake(t *testing.T) {
	fake := newFakeDHSaleServer(t)
	a := adapter.NewInventoryAdapter(dh.NewClient(fake.Server.URL, dh.WithEnterpriseKey("test_key")))

	result, err := a.RecordInventorySale(context.Background(), inventory.DHSaleRequest{
		DHInventoryID:  1,
		IdempotencyKey: "slabledger-sale-void-1",
		SalePriceCents: 1000,
	})
	require.NoError(t, err)

	require.NoError(t, a.VoidInventorySale(context.Background(), result.DHSaleID, "returned"))
	require.NoError(t, a.VoidInventorySale(context.Background(), result.DHSaleID, "returned"),
		"re-voiding an already-voided sale is success (reversed:false), not an error")
	require.NoError(t, a.VoidInventorySale(context.Background(), "sale_never_existed", ""),
		"void of an unknown sale id is success-with-a-log")
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/adapters/clients/dhlisting/... -run 'TestInventoryAdapter_RecordInventorySale|TestInventoryAdapter_VoidInventorySale' -v`
Expected: FAIL — `*dhlisting.InventoryAdapter has no field or method RecordInventorySale`.

- [ ] **Step 5: Write the implementation**

In `adapter.go`, add `"time"` to the imports, then widen the client interface field and the constructor parameter (both at `:113-139` — they must match exactly):

```go
type InventoryAdapter struct {
	client interface {
		UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
		SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
		RecordInventorySale(ctx context.Context, inventoryID int, idempotencyKey string, req dh.InventorySaleRequest) (*dh.InventorySaleResponse, error)
		VoidInventorySale(ctx context.Context, dhSaleID string, req dh.VoidSaleRequest) (*dh.VoidSaleResponse, error)
	}
	rotator dh.PSAKeyRotator
	logger  observability.Logger
}
```

Apply the identical four-method interface to `NewInventoryAdapter`'s parameter.

Add the two methods just before the `var _` block:

```go
// RecordInventorySale posts a sale for the given inventory item to DH via the
// purpose-built sale-recording endpoint, translating between domain and dh
// wire types and classifying any error into a domain sentinel. sold_at is
// always sent UTC-normalised RFC3339 (design §2): the request body must be a
// pure function of persisted columns so a retry under the same idempotency key
// is byte-identical.
func (a *InventoryAdapter) RecordInventorySale(ctx context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
	dhReq := dh.InventorySaleRequest{
		SalePriceCents:   req.SalePriceCents,
		SoldAt:           req.SoldAt.UTC().Format(time.RFC3339),
		CounterpartyName: req.CounterpartyName,
		Notes:            req.Notes,
	}

	resp, err := a.client.RecordInventorySale(ctx, req.DHInventoryID, req.IdempotencyKey, dhReq)
	if err != nil {
		return nil, classifyDHSaleError(err)
	}

	return &inventory.DHSaleResult{
		DHSaleID:            resp.SaleID,
		SoldInventoryID:     resp.SoldInventoryID,
		Delisted:            resp.Delisted,
		ItemStatus:          resp.ItemStatus,
		Replayed:            resp.Replayed,
		RealizedProfitCents: resp.RealizedProfitCents,
	}, nil
}

// VoidInventorySale voids a previously-recorded DH sale. DH returns 404 for a
// sale it cannot find under our credentials — not found, another account's
// sale, a marketplace-mirror deal, or a UI-created deal (design §7) — none of
// which we can reverse and none of which should fail an un-sell the user
// already performed locally, so it is logged and treated as success.
func (a *InventoryAdapter) VoidInventorySale(ctx context.Context, dhSaleID, reason string) error {
	_, err := a.client.VoidInventorySale(ctx, dhSaleID, dh.VoidSaleRequest{Reason: reason})
	if err == nil {
		return nil
	}

	classified := classifyDHSaleError(err)
	if errors.Is(classified, inventory.ErrDHSaleNotFound) {
		a.logger.Info(ctx, "dh: void target not found, treating as already voided",
			observability.String("dh_sale_id", dhSaleID))
		return nil
	}
	return classified
}
```

Add the assertion alongside the existing ones (keep `DHSoldNotifier` until Task 12):

```go
var _ inventory.DHSaleRecorder = (*InventoryAdapter)(nil)
```

- [ ] **Step 6: Run tests and the full build**

Run: `go build ./... && go test ./internal/adapters/clients/dhlisting/... -v`
Expected: PASS, including the still-present `TestInventoryAdapter_MarkInventorySold` (unaffected — the widened interface is satisfied by the extended stub), and `go build ./...` green per the ordering rule.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/clients/dhlisting/adapter.go \
        internal/adapters/clients/dhlisting/adapter_test.go \
        internal/adapters/clients/dhlisting/adapter_sale_test.go \
        internal/adapters/clients/dhlisting/dh_sale_fake_test.go
git commit -m "feat(dh): InventoryAdapter implements DHSaleRecorder"
```

---

### Task 9: Service records DH sales

**Files:**
- Modify: `internal/domain/inventory/service.go` (field next to `dhSoldNotifier`; new option next to `WithDHSoldNotifier`)
- Modify: `internal/domain/inventory/service_crud.go:286-354` (CreateSale), `:430-491` (CreateBulkSales), `:356-371` (replace `notifyDHSold`'s body)
- Test: `internal/domain/inventory/service_sale_test.go` (new, `package inventory_test`)

**Interfaces:**
- Consumes: `DHSaleRecorder`, `DHSaleRequest`, `DeriveDHSoldAt`, `NewDHIdempotencyKey`, `IsRetryableDHSaleError` (Tasks 2–3); `SaleRepository.SetSaleDHSaleID`, `PurchaseDHRepository.SetDHSaleConflict` (Tasks 7–8); `mocks.DHSaleRecorderMock`, `mocks.NewInMemoryCampaignStore` (Task 8)
- Produces: `service.buildDHSaleRequest`, `service.recordDHSale`, `service.flagDHSaleConflict` (all unexported; Task 10 calls `buildDHSaleRequest` directly)

- [ ] **Step 1: Add the field and option**

In `service.go`, next to the existing `dhSoldNotifier` field:

```go
	// dhSaleRecorder records (and, on un-sell, voids) sales on DH via the
	// purpose-built sale endpoint. It supersedes dhSoldNotifier's status-PATCH
	// approach, which DH rejects (422 "Invalid status 'sold'"). Both are wired
	// during the migration; only this one survives Task 12's cleanup.
	dhSaleRecorder DHSaleRecorder
```

And next to `WithDHSoldNotifier`:

```go
// WithDHSaleRecorder injects a DH sale recorder so a local sale is also
// recorded (and, on un-sell, voided) on DH. Optional — if nil, no DH sale
// call is made and the sale still commits locally.
func WithDHSaleRecorder(r DHSaleRecorder) ServiceOption {
	return func(s *service) { s.dhSaleRecorder = r }
}
```

- [ ] **Step 2: Write the failing test for key-minting order**

Create `internal/domain/inventory/service_sale_test.go`:

```go
package inventory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// seedSaleFixture creates a campaign and a DH-linked purchase, returning both.
func seedSaleFixture(t *testing.T, svc inventory.Service, repo *mocks.InMemoryCampaignStore) (*inventory.Campaign, *inventory.Purchase) {
	t.Helper()
	ctx := context.Background()
	c := &inventory.Campaign{Name: "Test"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	p := &inventory.Purchase{CampaignID: c.ID, PurchaseDate: "2026-06-01", DHInventoryID: 4001}
	if err := repo.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}
	return c, p
}

func TestService_CreateSale_MintsIdempotencyKeyBeforeDHCall(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			if req.IdempotencyKey == "" {
				t.Fatal("RecordInventorySale called with an empty idempotency key")
			}
			if !strings.HasPrefix(req.IdempotencyKey, inventory.DHIdempotencyKeyPrefix) {
				t.Fatalf("idempotency key %q missing prefix %q", req.IdempotencyKey, inventory.DHIdempotencyKeyPrefix)
			}
			return &inventory.DHSaleResult{DHSaleID: "dh-sale-1", Delisted: true}, nil
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	c, p := seedSaleFixture(t, svc, repo)
	sa := &inventory.Sale{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelInPerson, SalePriceCents: 5000, SaleDate: "2026-07-01"}
	if err := svc.CreateSale(ctx, sa, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	got, err := repo.GetSaleByPurchaseID(ctx, p.ID)
	if err != nil {
		t.Fatalf("reload sale: %v", err)
	}
	if got.DHIdempotencyKey == "" {
		t.Fatal("DHIdempotencyKey was not persisted")
	}
	if got.DHSaleID != "dh-sale-1" {
		t.Fatalf("DHSaleID = %q, want dh-sale-1", got.DHSaleID)
	}
}
```

`withTestIDGen()` already exists at `internal/domain/inventory/service_test.go:21` in this same `inventory_test` package.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/... -run TestService_CreateSale_MintsIdempotencyKey -v`
Expected: FAIL — compile error, `undefined: inventory.WithDHSaleRecorder` (or `mocks.DHSaleRecorderMock` if Task 8 has not landed).

- [ ] **Step 4: Replace `notifyDHSold`'s body**

In `service_crud.go`, replace the `notifyDHSold` function with these three:

```go
// buildDHSaleRequest builds the DH sale-record body from persisted columns
// only — never the wall clock (design §2 corollary) — so a retry with the
// same key issues a byte-identical body.
func (s *service) buildDHSaleRequest(sa *Sale, purchase *Purchase, key string) DHSaleRequest {
	return DHSaleRequest{
		DHInventoryID:  purchase.DHInventoryID,
		IdempotencyKey: key,
		SalePriceCents: sa.SalePriceCents,
		SoldAt:         DeriveDHSoldAt(sa.SaleDate, purchase.PurchaseDate, sa.CreatedAt),
	}
}

// recordDHSale records the sale on DH so the item is retired there. It
// replaces the old notifyDHSold status-PATCH, which DH rejects (422 "Invalid
// status 'sold'. Must be one of: in_stock, listed"). Best-effort by design:
// the sale is already committed locally and a DH outage must not fail it.
//
// A retryable failure is left unflagged for the §5b recovery pass — the key is
// already persisted, so the next cycle's identical request IS the retry. Any
// other failure, or a success that leaves the item not delisted, is flagged on
// the purchase for human review. A 409 item_sold_on_channel is never turned
// into a synthesized sale row: DH supplies no price, and both sold_at and
// channel may be nil (design §4).
func (s *service) recordDHSale(ctx context.Context, op string, sa *Sale, purchase *Purchase) {
	if s.dhSaleRecorder == nil || purchase.DHInventoryID == 0 {
		return
	}

	req := s.buildDHSaleRequest(sa, purchase, sa.DHIdempotencyKey)
	result, err := s.dhSaleRecorder.RecordInventorySale(ctx, req)
	if err != nil {
		if IsRetryableDHSaleError(err) {
			if s.logger != nil {
				s.logger.Warn(ctx, op+": dh sale record retryable failure, deferring to recovery pass",
					observability.String("purchaseID", sa.PurchaseID),
					observability.Err(err))
			}
			return
		}
		s.flagDHSaleConflict(ctx, op, purchase.ID, err.Error())
		return
	}

	if setErr := s.sales.SetSaleDHSaleID(ctx, sa.ID, result.DHSaleID, time.Now()); setErr != nil && s.logger != nil {
		s.logger.Warn(ctx, op+": failed to persist dh_sale_id",
			observability.String("purchaseID", sa.PurchaseID),
			observability.String("dhSaleID", result.DHSaleID),
			observability.Err(setErr))
	}

	// delisted == false means an ask may still be live — the exact failure
	// mode of the 2026-08-15 incident — so it is surfaced, not assumed benign.
	if !result.Delisted {
		s.flagDHSaleConflict(ctx, op, purchase.ID, "dh sale recorded but item not delisted")
	}
}

// flagDHSaleConflict records a purchase-level conflict for human review.
// Best-effort: a failure to write the flag is logged, not propagated — the
// sale already committed and the DH call already resolved either way.
func (s *service) flagDHSaleConflict(ctx context.Context, op, purchaseID, reason string) {
	if err := s.purchases.SetDHSaleConflict(ctx, purchaseID, reason); err != nil && s.logger != nil {
		s.logger.Warn(ctx, op+": failed to flag dh sale conflict",
			observability.String("purchaseID", purchaseID),
			observability.Err(err))
	}
}
```

- [ ] **Step 5: Mint the key and call `recordDHSale` from both sale paths**

In `CreateSale`, immediately after the existing `if sa.ID == "" { sa.ID = s.idGen() }`:

```go
	// Mint the idempotency key at creation, before any DH call (design §5a).
	// The row does not exist yet, so no compare-and-set is needed here — a
	// legacy sale predating this column mints lazily instead, via the §5b
	// recovery pass (Task 11).
	sa.DHIdempotencyKey = NewDHIdempotencyKey(s.idGen)
```

Replace the `notifyDHSold` call in `CreateSale` with:

```go
	s.recordDHSale(ctx, "create sale", sa, purchase)
```

In `CreateBulkSales`, after its `sa.ID = s.idGen()`:

```go
		sa.DHIdempotencyKey = NewDHIdempotencyKey(s.idGen)
```

And replace its `notifyDHSold` call with:

```go
		s.recordDHSale(ctx, "bulk sale", sa, purchase)
```

- [ ] **Step 6: Write the conflict-flagging table test**

Append to `service_sale_test.go`:

```go
func TestService_CreateSale_DHSaleConflictFlagging(t *testing.T) {
	tests := []struct {
		name         string
		recordErr    error
		result       *inventory.DHSaleResult
		wantConflict bool
		wantSaleDHID string
	}{
		{"non-retryable failure flags conflict", inventory.ErrDHItemUnavailable, nil, true, ""},
		{"retryable in-progress does NOT flag", inventory.ErrDHIdempotencyInProgress, nil, false, ""},
		{"retryable lock contention does NOT flag", inventory.ErrDHLockContention, nil, false, ""},
		{"success but not delisted flags conflict", nil, &inventory.DHSaleResult{DHSaleID: "dh-1", Delisted: false}, true, "dh-1"},
		{"success and delisted flags nothing", nil, &inventory.DHSaleResult{DHSaleID: "dh-2", Delisted: true}, false, "dh-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			recorder := &mocks.DHSaleRecorderMock{
				RecordInventorySaleFn: func(context.Context, inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
					if tt.recordErr != nil {
						return nil, tt.recordErr
					}
					return tt.result, nil
				},
			}
			svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
				withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
			ctx := context.Background()

			c, p := seedSaleFixture(t, svc, repo)
			sa := &inventory.Sale{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelInPerson, SalePriceCents: 5000, SaleDate: "2026-07-01"}
			if err := svc.CreateSale(ctx, sa, c, p); err != nil {
				t.Fatalf("CreateSale: %v", err)
			}

			gotP, err := repo.GetPurchase(ctx, p.ID)
			if err != nil {
				t.Fatalf("reload purchase: %v", err)
			}
			if hasConflict := gotP.DHSaleConflict != ""; hasConflict != tt.wantConflict {
				t.Fatalf("DHSaleConflict = %q (set=%v), want set=%v", gotP.DHSaleConflict, hasConflict, tt.wantConflict)
			}

			gotSale, err := repo.GetSaleByPurchaseID(ctx, p.ID)
			if err != nil {
				t.Fatalf("reload sale: %v", err)
			}
			if gotSale.DHSaleID != tt.wantSaleDHID {
				t.Fatalf("DHSaleID = %q, want %q", gotSale.DHSaleID, tt.wantSaleDHID)
			}
		})
	}
}
```

- [ ] **Step 7: Verify no `notifyDHSold` call sites remain, and the build is green**

```bash
unset GOPROXY GOSUMDB GOPRIVATE HTTP_PROXY HTTPS_PROXY http_proxy https_proxy
grep -n "notifyDHSold" internal/domain/inventory/*.go   # expect: no matches
go build ./...
go test ./internal/domain/inventory/... -run 'TestService_CreateSale|TestCreateBulkSales' -race -v
```

- [ ] **Step 8: Commit**

```bash
git add internal/domain/inventory/service.go internal/domain/inventory/service_crud.go internal/domain/inventory/service_sale_test.go
git commit -m "feat(dh): record sales via the DH sale endpoint instead of the broken status PATCH"
```

---

### Task 10: Un-sell via void, correctly ordered

**Files:**
- Modify: `internal/domain/inventory/service_return_inventory.go` (full rewrite; currently 53 lines)
- Test: `internal/domain/inventory/service_return_inventory_test.go` (new, `package inventory_test`)

**Interfaces:**
- Consumes: `DHSaleRecorder.VoidInventorySale`, `PurchaseDHRepository.ResetDHFieldsForRelistAfterVoid`, `SaleRepository.SetSaleDHSaleID`, `service.buildDHSaleRequest` (Task 9, same package)
- Produces: nothing downstream — this is a leaf rewrite

- [ ] **Step 1: Write the failing ordering test**

Create `internal/domain/inventory/service_return_inventory_test.go`:

```go
package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// The ordering IS the thing under test (design §7): void on DH, then reset the
// purchase, then delete the sale row LAST. Deleting last is what makes the
// sequence retry-safe — the sale row holds dh_sale_id, the only handle that
// can void.
func TestDeleteSaleByPurchaseID_VoidThenResetThenDelete(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	var calls []string

	recorder := &mocks.DHSaleRecorderMock{
		VoidInventorySaleFn: func(_ context.Context, dhSaleID, _ string) error {
			calls = append(calls, "void:"+dhSaleID)
			return nil
		},
	}
	repo.ResetDHFieldsForRelistAfterVoidFn = func(_ context.Context, purchaseID string) error {
		calls = append(calls, "reset:"+purchaseID)
		return nil
	}
	repo.DeleteSaleByPurchaseIDFn = func(_ context.Context, purchaseID string) error {
		calls = append(calls, "delete:"+purchaseID)
		return nil
	}

	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	sale := &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01",
		DHIdempotencyKey: "slabledger-sale-abc", DHSaleID: "dh-sale-77",
	}
	if err := repo.CreateSale(ctx, sale); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}

	want := []string{"void:dh-sale-77", "reset:" + p.ID, "delete:" + p.ID}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/... -run TestDeleteSaleByPurchaseID_VoidThenReset -v`
Expected: FAIL — the current implementation calls `ResetDHFieldsForRepushDueToDelete` and deletes the sale first, so `calls` comes back in the wrong order (or missing the void entirely).

- [ ] **Step 3: Rewrite `service_return_inventory.go`**

Full replacement:

```go
package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DeleteSaleByPurchaseID removes the sale associated with a purchase,
// returning the item to unsold inventory.
//
// Ordering (design §7): void on DH -> reset the purchase's DH linkage ->
// delete the sale row, LAST. Deleting last is what makes the sequence
// retry-safe: the sale row holds dh_sale_id, the only handle that can void, so
// a failure at the reset step leaves the row intact and the whole sequence can
// simply be retried. The reverse order loses the handle first and strands the
// purchase with no way to reverse the DH-side sale.
//
// The intermediate state (purchase reset, sale row still present) is safe
// rather than merely tolerable: the push query excludes purchases that have a
// sale row (`AND s.id IS NULL`, purchase_dh_query_store.go:21), so the item
// cannot be relisted while it still reads as sold.
//
// A purchase whose sale never reached DH — no dh_sale_id and no
// dh_idempotency_key, e.g. an eBay/Shopify import with no DH history — skips
// straight to the local reset and delete: there is nothing to void.
func (s *service) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	sale, err := s.sales.GetSaleByPurchaseID(ctx, purchaseID)
	if err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}

	if sale.DHSaleID != "" || sale.DHIdempotencyKey != "" {
		p, getErr := s.purchases.GetPurchase(ctx, purchaseID)
		if getErr != nil {
			return fmt.Errorf("delete sale for purchase %s: load purchase: %w", purchaseID, getErr)
		}

		dhSaleID, handleErr := s.ensureDHSaleHandle(ctx, sale, p)
		if handleErr != nil {
			return fmt.Errorf("delete sale for purchase %s: recover dh sale handle: %w", purchaseID, handleErr)
		}

		if dhSaleID != "" {
			if err := s.voidDHSale(ctx, purchaseID, dhSaleID); err != nil {
				return fmt.Errorf("delete sale for purchase %s: void on dh: %w", purchaseID, err)
			}
			if err := s.purchases.ResetDHFieldsForRelistAfterVoid(ctx, purchaseID); err != nil {
				// The sale row is still here (we delete last), so dh_sale_id
				// survives and a retry re-runs void — idempotent, DH returns
				// reversed:false — then reset, harmlessly.
				return fmt.Errorf("delete sale for purchase %s: reset dh fields after void: %w", purchaseID, err)
			}
		}
	}

	if err := s.sales.DeleteSaleByPurchaseID(ctx, purchaseID); err != nil {
		return fmt.Errorf("delete sale for purchase %s: %w", purchaseID, err)
	}
	return nil
}

// ensureDHSaleHandle returns the dh_sale_id needed to void, replaying the
// original request if the sale has a key but its handle was never persisted
// (design §5b "Concurrent un-sell"). It never skips the void and deletes the
// row anyway — that would orphan a recorded sale on DH with no way to reverse
// it.
func (s *service) ensureDHSaleHandle(ctx context.Context, sale *Sale, p *Purchase) (string, error) {
	if sale.DHSaleID != "" {
		return sale.DHSaleID, nil
	}
	if s.dhSaleRecorder == nil || p.DHInventoryID == 0 {
		return "", nil
	}

	req := s.buildDHSaleRequest(sale, p, sale.DHIdempotencyKey)
	result, err := s.dhSaleRecorder.RecordInventorySale(ctx, req)
	if err != nil {
		return "", err
	}
	if setErr := s.sales.SetSaleDHSaleID(ctx, sale.ID, result.DHSaleID, time.Now()); setErr != nil && s.logger != nil {
		s.logger.Warn(ctx, "un-sell: failed to persist recovered dh_sale_id",
			observability.String("purchaseID", p.ID),
			observability.String("dhSaleID", result.DHSaleID),
			observability.Err(setErr))
	}
	return result.DHSaleID, nil
}

// voidDHSale reverses a recorded sale on DH. A 404 is success-with-a-log
// (design §7): it covers not-found, another account's sale, a DH
// marketplace-mirror deal, or a UI-created deal — none of which we can void
// and none of which should fail an un-sell the user already performed locally.
func (s *service) voidDHSale(ctx context.Context, purchaseID, dhSaleID string) error {
	if s.dhSaleRecorder == nil {
		return nil
	}
	err := s.dhSaleRecorder.VoidInventorySale(ctx, dhSaleID, "un-sell")
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrDHSaleNotFound) {
		if s.logger != nil {
			s.logger.Info(ctx, "un-sell: dh sale already gone, treating void as success",
				observability.String("purchaseID", purchaseID),
				observability.String("dhSaleID", dhSaleID))
		}
		return nil
	}
	return err
}
```

- [ ] **Step 4: Test — no DH history skips the void**

```go
func TestDeleteSaleByPurchaseID_NoDHHistorySkipsVoid(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	recorder := &mocks.DHSaleRecorderMock{
		VoidInventorySaleFn: func(context.Context, string, string) error {
			t.Fatal("VoidInventorySale must not be called for a purchase with no DH history")
			return nil
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	p := &inventory.Purchase{CampaignID: c.ID, PurchaseDate: "2026-06-01"} // no DHInventoryID
	if err := repo.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}
	if err := repo.CreateSale(ctx, &inventory.Sale{ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01"}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}
	if _, err := repo.GetSaleByPurchaseID(ctx, p.ID); !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatalf("sale still present after un-sell: err=%v", err)
	}
}
```

- [ ] **Step 5: Test — a reset failure leaves the sale row intact and is retry-safe**

```go
func TestDeleteSaleByPurchaseID_ResetFailureLeavesSaleRowIntact(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	recorder := &mocks.DHSaleRecorderMock{}
	repo.ResetDHFieldsForRelistAfterVoidFn = func(context.Context, string) error {
		return errors.New("db timeout")
	}

	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	if err := repo.CreateSale(ctx, &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01", DHSaleID: "dh-77",
	}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err == nil {
		t.Fatal("expected an error from the failed reset")
	}

	got, err := repo.GetSaleByPurchaseID(ctx, p.ID)
	if err != nil {
		t.Fatalf("sale row was deleted despite the reset failure: %v", err)
	}
	if got.DHSaleID != "dh-77" {
		t.Fatalf("sale row corrupted: DHSaleID = %q, want dh-77", got.DHSaleID)
	}

	// Retry: reset now succeeds and the delete completes.
	repo.ResetDHFieldsForRelistAfterVoidFn = nil
	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("retry DeleteSaleByPurchaseID: %v", err)
	}
	if _, err := repo.GetSaleByPurchaseID(ctx, p.ID); !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatal("sale row should be gone after the successful retry")
	}
}
```

- [ ] **Step 6: Test — a missing handle is replayed before voiding**

```go
func TestDeleteSaleByPurchaseID_ReplaysForMissingHandle(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	var recordedKey, voidedID string
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			recordedKey = req.IdempotencyKey
			return &inventory.DHSaleResult{DHSaleID: "dh-recovered", Replayed: true, Delisted: true}, nil
		},
		VoidInventorySaleFn: func(_ context.Context, dhSaleID, _ string) error {
			voidedID = dhSaleID
			return nil
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	// Key present (DH confirmed the call) but dh_sale_id never persisted —
	// the crash window design §5b names.
	if err := repo.CreateSale(ctx, &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01",
		DHIdempotencyKey: "slabledger-sale-orphan",
	}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}
	if recordedKey != "slabledger-sale-orphan" {
		t.Fatalf("replay used key %q, want the persisted key", recordedKey)
	}
	if voidedID != "dh-recovered" {
		t.Fatalf("voided id = %q, want the id obtained from the replay", voidedID)
	}
}
```

- [ ] **Step 7: Build and run**

```bash
go build ./...
go test ./internal/domain/inventory/... -run TestDeleteSaleByPurchaseID -race -v
```

- [ ] **Step 8: Commit**

```bash
git add internal/domain/inventory/service_return_inventory.go internal/domain/inventory/service_return_inventory_test.go
git commit -m "fix(dh): void the recorded sale before un-selling, in retry-safe order"
```

---

### Task 11: Sweep rewrite + recovery pass

**Files:**
- Modify: `internal/adapters/scheduler/dh_sold_reconciler.go` (rewrite the DH-side portion)
- Modify: `internal/adapters/scheduler/builder.go` (BuildDeps additions), `builder_schedulers.go:255-280`
- Modify: `cmd/slabledger/init_schedulers.go`, `cmd/slabledger/runtime.go`
- Test: `internal/adapters/scheduler/dh_sold_reconciler_test.go`

**Interfaces:**
- Consumes: `inventory.DHSaleRecorder`, `DeriveDHSoldAt`, `NewDHIdempotencyKey`, `IsRetryableDHSaleError` (Tasks 2–3); `SaleRepository.GetSalesByPurchaseIDs` (existing), `.SetSaleIdempotencyKeyIfAbsent`, `.SetSaleDHSaleID`, `.ListSalesNeedingDHRecord` (Tasks 7–8); `PurchaseDHRepository.SetDHSaleConflict` (Tasks 7–8)
- Produces: nothing consumed by Task 12 beyond "the old notifier plumbing here is now unused"

**Why both passes exist, and why neither subsumes the other:** the sweep is scoped by DH's inventory listing (`dhSoldSweepStatuses` = `listed`/`in_stock`) and only revisits items DH still offers; the moment DH accepts a sale the item leaves that listing, so the sweep never sees it again — including the exact window where DH confirmed the sale but persisting `dh_sale_id` failed. The recovery pass is scoped by local columns (`dh_sale_id = ''`) instead, so it keeps finding that row after DH has delisted it. The sweep repairs "DH still thinks it's for sale"; the recovery pass repairs "we lost the handle to something DH already closed."

- [ ] **Step 1: Add the narrow ports**

In `dh_sold_reconciler.go`, after the existing `DHInventoryPurchaseResolver`:

```go
// DHSalesByPurchaseLister batch-loads sales for a set of purchases, keyed by
// purchase ID. The DH sweep uses it to find the sale behind an item DH still
// offers, so it can record a real price and date instead of the retired
// status PATCH.
type DHSalesByPurchaseLister interface {
	GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error)
}

// DHSaleHandleRecoveryLister finds sales DH has already accepted (or
// attempted) whose handle we failed to persist — the design §5b recovery
// scope. Keyed off local columns, not DH's inventory listing, so it keeps
// finding a row after DH has delisted the item.
type DHSaleHandleRecoveryLister interface {
	ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error)
}

// DHSaleWriter persists the outcome of a DH sale-record call: the minted
// idempotency key (compare-and-set, so two workers can never send different
// keys for one sale — design §5a) and, on success, the DH-assigned id.
type DHSaleWriter interface {
	SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error)
	SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error
}

// DHSaleConflictSetter flags a purchase for human review when DH sale
// recording fails non-retryably, or succeeds without delisting the item.
type DHSaleConflictSetter interface {
	SetDHSaleConflict(ctx context.Context, purchaseID, reason string) error
}

// dhSaleRecoveryBatchSize bounds how many sales the recovery pass attempts per
// cycle, matching the paging bound the DH sweep already uses.
const dhSaleRecoveryBatchSize = 200
```

Imports become:

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 2: Replace the dependency fields and options**

Replace the optional-dependency fields on `DHSoldReconcilerScheduler`:

```go
	// Optional DH-side sweep dependencies; the sweep is skipped unless all are
	// present. Wired together by WithDHSoldSweep.
	client      DHInventoryListClient
	resolver    DHInventoryPurchaseResolver
	salesLister DHSalesByPurchaseLister

	// Optional §5b recovery-pass dependency, wired by WithDHSaleHandleRecovery.
	recoveryLister DHSaleHandleRecoveryLister

	// Shared by both passes: recording, persisting, and flagging are identical
	// whichever pass found the row (see recordSale).
	recorder       inventory.DHSaleRecorder
	writer         DHSaleWriter
	conflictSetter DHSaleConflictSetter
```

Replace `WithDHSoldSweep` and add `WithDHSaleHandleRecovery`:

```go
// WithDHSoldSweep enables the DH-side sweep, which finds items DH still offers
// for a purchase we have already sold and records a proper sale for them
// (design §6) instead of the retired status PATCH. Without it the reconciler
// only repairs the local dh_status column and DH-side drift persists.
func WithDHSoldSweep(
	client DHInventoryListClient,
	resolver DHInventoryPurchaseResolver,
	salesLister DHSalesByPurchaseLister,
	recorder inventory.DHSaleRecorder,
	writer DHSaleWriter,
	conflictSetter DHSaleConflictSetter,
) DHSoldReconcilerOption {
	return func(s *DHSoldReconcilerScheduler) {
		s.client = client
		s.resolver = resolver
		s.salesLister = salesLister
		s.recorder = recorder
		s.writer = writer
		s.conflictSetter = conflictSetter
	}
}

// WithDHSaleHandleRecovery enables the design §5b recovery pass, which finds
// sales DH has already accepted whose dh_sale_id we failed to persist and
// completes them. Complementary to WithDHSoldSweep, not a substitute — see the
// package doc above for why neither subsumes the other.
func WithDHSaleHandleRecovery(
	lister DHSaleHandleRecoveryLister,
	recorder inventory.DHSaleRecorder,
	writer DHSaleWriter,
	conflictSetter DHSaleConflictSetter,
) DHSoldReconcilerOption {
	return func(s *DHSoldReconcilerScheduler) {
		s.recoveryLister = lister
		s.recorder = recorder
		s.writer = writer
		s.conflictSetter = conflictSetter
	}
}
```

- [ ] **Step 3: Update the enablement checks and startup log**

```go
func (s *DHSoldReconcilerScheduler) sweepEnabled() bool {
	return s.client != nil && s.resolver != nil && s.salesLister != nil &&
		s.recorder != nil && s.writer != nil && s.conflictSetter != nil
}

// recoveryEnabled reports whether the §5b dependencies are wired, independent
// of sweepEnabled.
func (s *DHSoldReconcilerScheduler) recoveryEnabled() bool {
	return s.recoveryLister != nil && s.recorder != nil && s.writer != nil && s.conflictSetter != nil
}
```

Extend the existing startup block in `Start` so the recovery pass's absence is equally visible — the original log line exists precisely so the hole cannot be silently open:

```go
	if s.sweepEnabled() {
		s.logger.Info(ctx, "dh sold reconciler: DH sweep enabled")
	} else {
		s.logger.Warn(ctx, "dh sold reconciler: DH sweep disabled, sold items will not be retired on DH")
	}
	if s.recoveryEnabled() {
		s.logger.Info(ctx, "dh sold reconciler: DH sale handle recovery pass enabled")
	} else {
		s.logger.Warn(ctx, "dh sold reconciler: DH sale handle recovery pass disabled, a lost dh_sale_id cannot be repaired")
	}
```

- [ ] **Step 4: Wire both passes into `reconcile`**

Keep the local-pass body exactly as-is; replace the single trailing defer with:

```go
	// Both passes are independent by design, so LIFO defer order between them
	// has no correctness effect.
	defer s.sweepDH(ctx)
	defer s.recoverDHSaleHandles(ctx)
```

- [ ] **Step 5: Rewrite `sweepDH` and add `recordSale`**

```go
// sweepDH finds items DH still offers even though we recorded the sale, and
// records a proper sale for them (design §6) — replacing the status PATCH DH
// rejects. It keys on DH inventory ID rather than cert number (PR #682): a
// cert can match several purchases across re-acquisitions, the inventory id
// cannot.
func (s *DHSoldReconcilerScheduler) sweepDH(ctx context.Context) {
	if !s.sweepEnabled() {
		return
	}

	recorded, failed := 0, 0
	for _, status := range dhSoldSweepStatuses {
		items, err := s.fetchInventoryByStatus(ctx, status)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to list DH inventory",
				observability.String("status", status), observability.Err(err))
			continue
		}

		byInventoryID, err := s.resolvePurchases(ctx, items)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to resolve purchases for DH inventory",
				observability.String("status", status), observability.Err(err))
			continue
		}

		soldPurchaseIDs := make([]string, 0, len(items))
		for _, item := range items {
			if p := byInventoryID[item.DHInventoryID]; p != nil && p.DHStatus == inventory.DHStatusSold {
				soldPurchaseIDs = append(soldPurchaseIDs, p.ID)
			}
		}
		if len(soldPurchaseIDs) == 0 {
			continue
		}
		salesByPurchase, err := s.salesLister.GetSalesByPurchaseIDs(ctx, soldPurchaseIDs)
		if err != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to load sales for DH inventory",
				observability.String("status", status), observability.Err(err))
			continue
		}

		for _, item := range items {
			p := byInventoryID[item.DHInventoryID]
			if p == nil || p.DHStatus != inventory.DHStatusSold {
				continue
			}
			sale := salesByPurchase[p.ID]
			if sale == nil {
				// A sold purchase with no sale row is a data inconsistency the
				// sweep cannot repair by guessing a price. Skip it.
				continue
			}
			if err := s.recordSale(ctx, p, sale); err != nil {
				failed++
				s.logger.Warn(ctx, "dh sold reconciler: failed to record sale on DH",
					observability.String("purchaseID", p.ID),
					observability.String("cert", item.CertNumber),
					observability.Int("dhInventoryID", item.DHInventoryID),
					observability.Err(err))
				continue
			}
			recorded++
			s.logger.Info(ctx, "dh sold reconciler: recorded sale on DH for item still offered there",
				observability.String("purchaseID", p.ID),
				observability.String("cert", item.CertNumber),
				observability.String("dhStatus", status))
		}
	}

	if recorded > 0 || failed > 0 {
		s.logger.Info(ctx, "dh sold reconciler: DH sweep completed",
			observability.Int("recorded", recorded), observability.Int("failed", failed))
	}
}

// recordSale mints an idempotency key if the sale predates this feature (§5a),
// calls DH, and persists the result. Both passes call this — the
// mint -> call -> persist -> flag ordering (§5b) is identical regardless of
// which pass found the row, so a crash at any point leaves it in a state
// either pass can finish.
func (s *DHSoldReconcilerScheduler) recordSale(ctx context.Context, p *inventory.Purchase, sale *inventory.Sale) error {
	key := sale.DHIdempotencyKey
	if key == "" {
		effective, err := s.writer.SetSaleIdempotencyKeyIfAbsent(ctx, sale.ID, inventory.NewDHIdempotencyKey(uuid.NewString))
		if err != nil {
			return fmt.Errorf("mint idempotency key: %w", err)
		}
		key = effective
	}

	req := inventory.DHSaleRequest{
		DHInventoryID:  p.DHInventoryID,
		IdempotencyKey: key,
		SalePriceCents: sale.SalePriceCents,
		SoldAt:         inventory.DeriveDHSoldAt(sale.SaleDate, p.PurchaseDate, sale.CreatedAt),
	}

	result, err := s.recorder.RecordInventorySale(ctx, req)
	if err != nil {
		// A conflict flag is what stops a permanently-failed sale from being
		// retried forever: ListSalesNeedingDHRecord filters on
		// dh_sale_conflict = '' (§5b). A retryable error is left unflagged —
		// the key is persisted, so the next cycle's identical request IS the
		// retry.
		if !inventory.IsRetryableDHSaleError(err) {
			if cErr := s.conflictSetter.SetDHSaleConflict(ctx, p.ID, err.Error()); cErr != nil {
				s.logger.Warn(ctx, "dh sold reconciler: failed to flag conflict",
					observability.String("purchaseID", p.ID), observability.Err(cErr))
			}
		}
		return err
	}

	if setErr := s.writer.SetSaleDHSaleID(ctx, sale.ID, result.DHSaleID, time.Now()); setErr != nil {
		return fmt.Errorf("persist dh_sale_id: %w", setErr)
	}
	if !result.Delisted {
		if cErr := s.conflictSetter.SetDHSaleConflict(ctx, p.ID, "dh sale recorded but item not delisted"); cErr != nil {
			s.logger.Warn(ctx, "dh sold reconciler: failed to flag conflict",
				observability.String("purchaseID", p.ID), observability.Err(cErr))
		}
	}
	return nil
}

// recoverDHSaleHandles is the design §5b pass: it finds sales DH has already
// accepted (or attempted) whose dh_sale_id we failed to persist, and completes
// them. Unlike sweepDH it is scoped by local columns, not DH's inventory
// listing, so it keeps finding a row after DH has delisted the item — the
// exact window this pass exists to close.
func (s *DHSoldReconcilerScheduler) recoverDHSaleHandles(ctx context.Context) {
	if !s.recoveryEnabled() {
		return
	}

	rows, err := s.recoveryLister.ListSalesNeedingDHRecord(ctx, dhSaleRecoveryBatchSize)
	if err != nil {
		s.logger.Warn(ctx, "dh sold reconciler: failed to list sales needing dh record", observability.Err(err))
		return
	}

	recovered, failed := 0, 0
	for _, row := range rows {
		sale := row.Sale
		p := &inventory.Purchase{ID: sale.PurchaseID, DHInventoryID: row.DHInventoryID, PurchaseDate: row.PurchaseDate}
		if err := s.recordSale(ctx, p, &sale); err != nil {
			failed++
			s.logger.Warn(ctx, "dh sold reconciler: recovery pass failed to record sale",
				observability.String("purchaseID", p.ID), observability.Err(err))
			continue
		}
		recovered++
	}

	if recovered > 0 || failed > 0 {
		s.logger.Info(ctx, "dh sold reconciler: handle recovery pass completed",
			observability.Int("recovered", recovered), observability.Int("failed", failed))
	}
}
```

- [ ] **Step 6: Add the test-local sales-lister stub**

`DHSalesByPurchaseLister` has one method. `InMemoryCampaignStore` already implements it (it satisfies `SaleRepository`), but the sweep tests need to serve a fixed map without seeding a whole store, so add this stub to `dh_sold_reconciler_test.go` — consistent with the file's existing test-local helpers like `listClientByStatus`:

```go
// stubSalesLister serves a fixed purchaseID -> sale map, satisfying
// DHSalesByPurchaseLister.
type stubSalesLister struct {
	byPurchaseID map[string]*inventory.Sale
	err          error
}

func (l *stubSalesLister) GetSalesByPurchaseIDs(_ context.Context, ids []string) (map[string]*inventory.Sale, error) {
	if l.err != nil {
		return nil, l.err
	}
	out := make(map[string]*inventory.Sale, len(ids))
	for _, id := range ids {
		if s, ok := l.byPurchaseID[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}
```

- [ ] **Step 7: Update the existing sweep tests for the new signature**

`TestDHSoldReconciler_SweepDH`, `TestDHSoldReconciler_SweepSkipsZeroInventoryIDs`, `TestDHSoldReconciler_SweepsBothListableStatuses`, and `TestDHSoldReconciler_LocalPassRunsBeforeSweep` all construct `&mocks.DHSoldNotifierMock{}` and call the three-arg `WithDHSoldSweep`. Update each:

- swap the notifier for a `&mocks.DHSaleRecorderMock{}` plus a `&stubSalesLister{byPurchaseID: ...}`
- call the six-arg `WithDHSoldSweep(client, resolver, salesLister, recorder, writer, conflictSetter)`, passing `mocks.NewInMemoryCampaignStore()` as the writer and `&mocks.PurchaseRepositoryMock{}` as the conflict setter
- rename the assertion variable from "marked" to "recorded" — `MarkInventorySold` no longer exists; assert on `recorder.RecordedSales()` and the `DHInventoryID` of each

Tests that assert only on resolver input (e.g. `SweepSkipsZeroInventoryIDs`) can pass an empty `&stubSalesLister{}`, since no recording is expected.

- [ ] **Step 8: Test — a legacy keyless sale gets a key minted**

```go
func TestDHSoldReconciler_RecoveryPass_MintsKeyForLegacySale(t *testing.T) {
	store := mocks.NewInMemoryCampaignStore()
	sale := inventory.Sale{ID: "s1", PurchaseID: "p1", SalePriceCents: 5000, SaleDate: "2026-07-01"} // no key: legacy
	store.ListSalesNeedingDHRecordFn = func(context.Context, int) ([]inventory.SaleNeedingDHRecord, error) {
		return []inventory.SaleNeedingDHRecord{{Sale: sale, DHInventoryID: 9001, PurchaseDate: "2026-06-01"}}, nil
	}
	var mintedFor, mintedKey string
	store.SetSaleIdempotencyKeyIfAbsentFn = func(_ context.Context, saleID, key string) (string, error) {
		mintedFor, mintedKey = saleID, key
		return key, nil
	}
	var setDHSaleID string
	store.SetSaleDHSaleIDFn = func(_ context.Context, _ string, dhSaleID string, _ time.Time) error {
		setDHSaleID = dhSaleID
		return nil
	}

	var gotKey string
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			gotKey = req.IdempotencyKey
			return &inventory.DHSaleResult{DHSaleID: "dh-legacy-1", Delisted: true}, nil
		},
	}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSaleHandleRecovery(store, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.recoverDHSaleHandles(context.Background())

	if mintedFor != "s1" {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent called for %q, want s1", mintedFor)
	}
	if mintedKey == "" || gotKey != mintedKey {
		t.Fatalf("minted key %q was not the key sent to DH (%q)", mintedKey, gotKey)
	}
	if setDHSaleID != "dh-legacy-1" {
		t.Fatalf("SetSaleDHSaleID got %q, want dh-legacy-1", setDHSaleID)
	}
}
```

- [ ] **Step 9: Test — a replayed sale does not re-mint or double-record**

```go
func TestDHSoldReconciler_RecoveryPass_ReplayDoesNotDoubleRecord(t *testing.T) {
	store := mocks.NewInMemoryCampaignStore()
	sale := inventory.Sale{
		ID: "s1", PurchaseID: "p1", SalePriceCents: 5000, SaleDate: "2026-07-01",
		DHIdempotencyKey: "slabledger-sale-existing",
	}
	store.ListSalesNeedingDHRecordFn = func(context.Context, int) ([]inventory.SaleNeedingDHRecord, error) {
		return []inventory.SaleNeedingDHRecord{{Sale: sale, DHInventoryID: 9001, PurchaseDate: "2026-06-01"}}, nil
	}
	var mintCalls int
	store.SetSaleIdempotencyKeyIfAbsentFn = func(context.Context, string, string) (string, error) {
		mintCalls++
		return "", nil
	}
	var persistedID string
	store.SetSaleDHSaleIDFn = func(_ context.Context, _ string, dhSaleID string, _ time.Time) error {
		persistedID = dhSaleID
		return nil
	}

	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			if req.IdempotencyKey != "slabledger-sale-existing" {
				t.Fatalf("replay used key %q, want the existing persisted key", req.IdempotencyKey)
			}
			return &inventory.DHSaleResult{DHSaleID: "dh-existing", Replayed: true, Delisted: true}, nil
		},
	}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSaleHandleRecovery(store, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.recoverDHSaleHandles(context.Background())

	if mintCalls != 0 {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent called %d times for a sale that already had a key, want 0", mintCalls)
	}
	if persistedID != "dh-existing" {
		t.Fatalf("SetSaleDHSaleID got %q, want dh-existing", persistedID)
	}
}
```

- [ ] **Step 10: Test — a conflict-flagged sale is never recorded**

```go
func TestDHSoldReconciler_RecoveryPass_SkipsConflictFlaggedSale(t *testing.T) {
	// ListSalesNeedingDHRecord's own predicate (§5b) excludes rows with
	// dh_sale_conflict <> ''. This asserts the scheduler records nothing for a
	// row the lister withheld — i.e. the terminal-state gate holds end-to-end.
	store := mocks.NewInMemoryCampaignStore()
	store.ListSalesNeedingDHRecordFn = func(context.Context, int) ([]inventory.SaleNeedingDHRecord, error) {
		return nil, nil
	}
	var recordCalls int
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(context.Context, inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			recordCalls++
			return &inventory.DHSaleResult{DHSaleID: "dh-x", Delisted: true}, nil
		},
	}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSaleHandleRecovery(store, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.recoverDHSaleHandles(context.Background())

	if recordCalls != 0 {
		t.Fatalf("RecordInventorySale called %d times, want 0", recordCalls)
	}
}
```

- [ ] **Step 11: Rewire `BuildDeps` and the builder**

In `builder.go`, after the existing `DHSoldNotifier` field (which Task 12 removes):

```go
	// DHSaleRecorder records sales on DH via the purpose-built sale endpoint
	// and voids them on un-sell. Optional — enables both reconciler passes.
	DHSaleRecorder domainCampaigns.DHSaleRecorder

	// DHSaleStore gives the reconciler read/write access to campaign_sales:
	// batch sale lookup, idempotency-key minting, dh_sale_id persistence.
	// Optional — nil disables both passes.
	DHSaleStore domainCampaigns.SaleRepository
```

Replace `buildDHSoldReconcilerScheduler`:

```go
func buildDHSoldReconcilerScheduler(cfg *config.Config, deps BuildDeps) *DHSoldReconcilerScheduler {
	if deps.PurchaseRepo == nil {
		return nil
	}
	soldReconcilerCfg := DHSoldReconcilerConfig{
		Enabled:  cfg.DHSoldReconciler.Enabled,
		Interval: cfg.DHSoldReconciler.Interval,
	}
	var opts []DHSoldReconcilerOption
	if deps.DHSaleRecorder != nil && deps.DHSaleStore != nil {
		if deps.DHInventoryListClient != nil {
			opts = append(opts, WithDHSoldSweep(
				deps.DHInventoryListClient,
				deps.PurchaseRepo,
				deps.DHSaleStore,
				deps.DHSaleRecorder,
				deps.DHSaleStore,
				deps.PurchaseRepo,
			))
		}
		opts = append(opts, WithDHSaleHandleRecovery(
			deps.DHSaleStore,
			deps.DHSaleRecorder,
			deps.DHSaleStore,
			deps.PurchaseRepo,
		))
	}
	return NewDHSoldReconcilerScheduler(
		deps.PurchaseRepo, deps.PurchaseRepo, deps.Logger, soldReconcilerCfg, opts...,
	)
}
```

- [ ] **Step 12: Thread the store and adapter through `cmd/slabledger`**

Add `SaleStore *postgres.SaleStore` to `schedulerDeps` in `init_schedulers.go`, populate it from `runtime.go`'s `schedulerDeps()` (`SaleStore: w.campaignsInit.saleStore`), and inside the existing `if deps.DHClient != nil` block:

```go
		if deps.DHClient.EnterpriseAvailable() {
			dhSaleAdapter := dhlistingadapter.NewInventoryAdapter(deps.DHClient)
			buildDeps.DHSoldNotifier = dhSaleAdapter
			buildDeps.DHSaleRecorder = dhSaleAdapter
		}
```

Plus, alongside the other nil-safe assignments:

```go
	if deps.SaleStore != nil {
		buildDeps.DHSaleStore = deps.SaleStore
	}
```

- [ ] **Step 13: Build and test**

```bash
go build ./...
go test ./internal/adapters/scheduler/... -run TestDHSoldReconciler -race -v
```

- [ ] **Step 14: Commit**

```bash
git add internal/adapters/scheduler/dh_sold_reconciler.go internal/adapters/scheduler/dh_sold_reconciler_test.go \
        internal/adapters/scheduler/builder.go internal/adapters/scheduler/builder_schedulers.go \
        cmd/slabledger/init_schedulers.go cmd/slabledger/runtime.go
git commit -m "feat(dh): record real sales in the sold-reconciler sweep, add the handle-recovery pass"
```

---

### Task 12: Remove the dead path

> **DECISION REQUIRED BEFORE STARTING THIS TASK.** See Step 1: `csvimport`'s
> `ConfirmOrdersSales` currently attempts an inline (broken) DH notification.
> Removing `DHSoldNotifier` forces a choice between "rely on the reconciler's
> next cycle" and "give csvimport its own recorder". This is a real behavioural
> change, not mechanical cleanup, and must be answered before implementing.

**Files:**
- Modify: `internal/domain/inventory/service.go` (remove `DHSoldNotifier`, field, `WithDHSoldNotifier`)
- Modify: `internal/domain/csvimport/service.go`, `service_import_orders.go`
- Modify: `internal/adapters/clients/dhlisting/adapter.go` (remove `MarkInventorySold`), `adapter_test.go` (remove `TestInventoryAdapter_MarkInventorySold`)
- Delete: `internal/testutil/mocks/inventory_dh_sold_notifier.go`
- Modify: `internal/adapters/scheduler/builder.go`, `cmd/slabledger/init_inventory_services.go`, `init_schedulers.go`
- Modify: `docs/SCHEDULERS.md`, `docs/DH_INVENTORY.md`

**Interfaces:**
- Consumes: everything from Tasks 2–11. Produces: nothing — terminal task.

- [ ] **Step 1: Resolve the csvimport decision, then apply it**

`service_import_orders.go` calls `dhSoldNotifier.MarkInventorySold` inline when confirming order-import sales. Two options:

- **(a) Rely on the reconciler.** Delete the notifier block entirely. `dh_status` is already flipped to `'sold'` on that path, so the sweep (Task 11) picks up the drift on its next cycle. Cost: up to `DH_SOLD_RECONCILER_INTERVAL` (default 1h) of exposure where the item is sold locally but still live on DH.
- **(b) Give csvimport its own recorder.** Add `inventory.DHSaleRecorder` to `csvimport.Deps` and mirror Task 9's mint → record → persist → flag sequence. Costs more code; closes the window immediately.

Given this whole plan exists because items stayed live on DH for four days, **(b) is the safer default** — but it is the user's call, and option (a) is defensible since the reconciler now genuinely works. Record the decision here before writing code.

- [ ] **Step 2: Remove `DHSoldNotifier` from the domain**

Delete the `DHSoldNotifier` interface, the `dhSoldNotifier` field, and `WithDHSoldNotifier` from `service.go`. Remove the corresponding `Deps.DHSoldNotifier` field and `service.dhSoldNotifier` field/assignment from `csvimport/service.go`.

- [ ] **Step 3: Remove `MarkInventorySold` from the adapter**

Delete `MarkInventorySold` and `var _ inventory.DHSoldNotifier = (*InventoryAdapter)(nil)` from `adapter.go`. Keep `RecordInventorySale`, `VoidInventorySale`, and their `DHSaleRecorder` assertion.

- [ ] **Step 4: Delete the mock and the permissive-mock test**

```bash
git rm internal/testutil/mocks/inventory_dh_sold_notifier.go
```

Delete `TestInventoryAdapter_MarkInventorySold` from `adapter_test.go`. This is the test the design doc names: it asserted against a mock that accepted any payload, which is exactly why the real 422 shipped undetected. Its replacement is Task 6's contract-enforcing fake — confirm that fake is present and passing before deleting this.

- [ ] **Step 5: Remove the wiring**

Delete the `DHSoldNotifier` field from `builder.go`'s `BuildDeps`, drop the `buildDeps.DHSoldNotifier` assignment from `init_schedulers.go` (leaving only `DHSaleRecorder`), and in `init_inventory_services.go` replace the `dhSoldNotifier` variable and its `WithDHSoldNotifier` option with:

```go
	// DH sale recorder — records (and, on un-sell, voids) sales on DH via the
	// purpose-built sale endpoint.
	if dhClient != nil && dhClient.EnterpriseAvailable() {
		campaignOpts = append(campaignOpts, inventory.WithDHSaleRecorder(dhlistingadapter.NewInventoryAdapter(dhClient)))
	}
```

Also remove `DHSoldNotifier: dhSoldNotifier` from the `csvimport.Deps{}` literal.

- [ ] **Step 6: Update the docs**

Replace the DH Sold Reconciler body in `docs/SCHEDULERS.md`:

```markdown
### DH Sold Reconciler

**File:** `dh_sold_reconciler.go`
**Purpose:** Repairs purchases that have a linked sale but whose `dh_status` never
advanced to `sold`, and records those sales properly on DH.

A safety net for the best-effort `dh_status` update inside `CreateSale` — that update
is deliberately non-fatal to the sale, so something has to catch the misses. Beyond the
local column repair it runs two DH-side passes: a sweep that records a real sale via the
DH sale endpoint for anything DH still lists that we have already sold, and a
handle-recovery pass that finds sales DH already accepted whose `dh_sale_id` we failed
to persist. See `docs/specs/2026-08-20-dh-record-sale-design.md`.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_SOLD_RECONCILER_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_SOLD_RECONCILER_INTERVAL` | `1h` | How often to run |
```

Then run `grep -n "MarkInventorySold\|notifyDHSold\|status.*sold" docs/DH_INVENTORY.md` and update any prose describing the old status-PATCH behaviour.

- [ ] **Step 7: Recommend the fate of `cmd/dh-delist/`**

The untracked `cmd/dh-delist/` incident tool was the out-of-band mitigation for the 25 drifted cards; the design doc defers its fate. **Do not delete it in this task.** Once the recovery pass has run in production and repaired those 25 (verify their `dh_sale_id` is populated and `dh_sale_conflict` is empty), it has no remaining purpose and should go in a follow-up. State this in the PR description rather than silently keeping or deleting it.

- [ ] **Step 8: Full verification**

```bash
unset GOPROXY GOSUMDB GOPRIVATE HTTP_PROXY HTTPS_PROXY http_proxy https_proxy
go build ./...
go test -race -timeout 10m ./...
make check
```

`make check` runs `scripts/check-doc-paths.sh`, which fails if a doc cites a path deleted here, plus the hexagonal import check and the file-size and function-length budgets.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "cleanup(dh): remove the dead MarkInventorySold status-PATCH path"
```

---
