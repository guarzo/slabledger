# P6 — adapters/storage/sqlite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use **superpowers:subagent-driven-development** to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. See Setup section below for worktree creation.

**Goal:** Fix correctness, reduce duplication, and improve test coverage in `internal/adapters/storage/sqlite/`.

**Architecture:** All changes are confined to `internal/adapters/storage/sqlite/`. The three tasks are ordered by severity: generalize chunked bulk-lookup helpers first (HIGH), fix profitability_provider error handling (HIGH), remove unnecessary transaction in AcceptAISuggestion (LOW). Test coverage additions follow.

**Tech Stack:** Go 1.21+, SQLite via `database/sql`, Go generics (`[T any]`), table-driven tests.

---

## Setup

```bash
# Create worktree from the main repo root (not from within another worktree)
git -C /workspace worktree add /workspace/.worktrees/plan-p6-sqlite -b feature/polish-p6-sqlite
cd /workspace/.worktrees/plan-p6-sqlite
```

Verify:
```bash
go build ./internal/adapters/storage/sqlite/...
go test -race ./internal/adapters/storage/sqlite/...
```
Expected: builds and all tests pass.

---

## Task 1: Generalize chunked bulk-lookup helpers with Go generics

**Why:** `GetPurchasesByGraderAndCertNumbers` and `GetPurchasesByCertNumbers` both have identical 20-line `for rows.Next()` scan loops with manual error handling. `GetPurchasesByIDs` already uses the `scanRows` helper. The first two should use a shared generic chunked-query helper.

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchase_cert_store.go`

- [ ] **Step 1: Read the existing scanRows helper signature**

Read `internal/adapters/storage/sqlite/scan_helpers.go` (or wherever `scanRows` is defined) to confirm its signature. The expected signature is:

```go
func scanRows[T any](ctx context.Context, rows *sql.Rows, scan func(*sql.Rows) (T, error)) ([]T, error)
```

- [ ] **Step 2: Rewrite GetPurchasesByGraderAndCertNumbers to use scanRows**

In `internal/adapters/storage/sqlite/purchase_cert_store.go`, replace the manual `for rows.Next()` loop (lines 45–66) with `scanRows`:

```go
func (ps *PurchaseStore) GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+1)
		args = append(args, grader)
		for i, cn := range chunk {
			placeholders[i] = "?"
			args = append(args, cn)
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE grader = ? AND cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by grader/cert chunk: %w", err)
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
			var p inventory.Purchase
			return p, scanPurchase(rs, &p)
		})
		if err != nil {
			return nil, fmt.Errorf("scan purchases by grader/cert chunk: %w", err)
		}
		for i := range purchases {
			result[purchases[i].CertNumber] = &purchases[i]
		}
	}
	return result, nil
}
```

- [ ] **Step 3: Rewrite GetPurchasesByCertNumbers to use scanRows**

Apply the same pattern to `GetPurchasesByCertNumbers` (lines 75–127):

```go
func (ps *PurchaseStore) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, cn := range chunk {
			placeholders[i] = "?"
			args[i] = cn
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by cert chunk: %w", err)
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
			var p inventory.Purchase
			return p, scanPurchase(rs, &p)
		})
		if err != nil {
			return nil, fmt.Errorf("scan purchases by cert chunk: %w", err)
		}
		for i := range purchases {
			result[purchases[i].CertNumber] = &purchases[i]
		}
	}
	return result, nil
}
```

- [ ] **Step 4: Build to verify**

```bash
go build ./internal/adapters/storage/sqlite/...
```

Expected: no errors.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/adapters/storage/sqlite/...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/storage/sqlite/purchase_cert_store.go
git commit -m "refactor: use scanRows helper in GetPurchasesByCertNumbers and GetPurchasesByGraderAndCertNumbers"
```

---

## Task 2: Fix profitability_provider to add prominent error logging

**Why:** `GetProfitablePatterns` already propagates sub-query errors via `errors.Join` — it does NOT silently swallow them. However, the comment says "failures are tolerated so the caller receives whatever partial data is available." The concern is that when sub-queries fail, the partial profile is returned WITH an error, but the caller may not log it. The fix is to add a `Warn` log when partial errors occur so failures surface in operational monitoring, even though returning partial data is intentional.

**Files:**
- Modify: `internal/adapters/storage/sqlite/profitability_provider.go`

- [ ] **Step 1: Add logger field to ProfitabilityProvider**

Update the struct and constructor:

```go
// ProfitabilityProvider queries campaign_purchases + campaign_sales
// to surface historical profitability patterns for the AI picks engine.
type ProfitabilityProvider struct {
	db     *sql.DB
	logger observability.Logger
}

// NewProfitabilityProvider creates a new ProfitabilityProvider backed by db.
func NewProfitabilityProvider(db *sql.DB, logger observability.Logger) *ProfitabilityProvider {
	return &ProfitabilityProvider{db: db, logger: logger}
}
```

Add the import at the top:

```go
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/picks"
)
```

- [ ] **Step 2: Add Warn log in GetProfitablePatterns when errors occur**

Replace the error return at lines 59–61 with:

```go
	if len(errs) > 0 {
		joined := errors.Join(errs...)
		p.logger.Warn(context.Background(), "GetProfitablePatterns: partial failure, returning incomplete profile",
			observability.Err(joined))
		return profile, joined
	}
```

- [ ] **Step 3: Find all callers of NewProfitabilityProvider and update them**

```bash
grep -r "NewProfitabilityProvider" /workspace/.worktrees/campaigns-decomposition --include="*.go" -l
```

Update each caller to pass a logger. Likely in `cmd/slabledger/init_services.go` or `init_inventory_services.go`. Example update:

```go
// Before:
profProvider := sqlite.NewProfitabilityProvider(db)
// After:
profProvider := sqlite.NewProfitabilityProvider(db, logger)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/adapters/storage/sqlite/...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/storage/sqlite/profitability_provider.go
git commit -m "fix: add prominent warn log to ProfitabilityProvider on sub-query failure"
```

---

## Task 3: Remove unnecessary transaction in AcceptAISuggestion

**Why:** `AcceptAISuggestion` (lines 57–93 of `purchase_price_store.go`) wraps a single `UPDATE` statement in an explicit transaction. The conditional `WHERE` clause already provides optimistic-lock semantics. A transaction adds overhead without benefit for a single statement.

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchase_price_store.go`

- [ ] **Step 1: Rewrite AcceptAISuggestion without transaction**

Replace lines 57–93 with:

```go
func (ps *PurchaseStore) AcceptAISuggestion(ctx context.Context, purchaseID string, priceCents int) error {
	if priceCents <= 0 {
		return inventory.ErrNoAISuggestion
	}

	now := time.Now()
	setAt := now.Format(time.RFC3339)

	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET override_price_cents = ?, override_source = 'ai_accepted', override_set_at = ?,
		     ai_suggested_price_cents = 0, ai_suggested_at = '',
		     updated_at = ?
		 WHERE id = ? AND ai_suggested_price_cents = ? AND ai_suggested_at != ''`,
		priceCents, setAt, now, purchaseID, priceCents,
	)
	if err != nil {
		return fmt.Errorf("accept ai suggestion: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrNoAISuggestion
	}
	return nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/adapters/storage/sqlite/...
```

Expected: no errors.

- [ ] **Step 3: Run tests**

```bash
go test -race ./internal/adapters/storage/sqlite/...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/purchase_price_store.go
git commit -m "refactor: remove unnecessary transaction in AcceptAISuggestion (single UPDATE)"
```

---

## Task 4: Add positional grouping comments to CreatePurchase INSERT

**Why:** The 54-column INSERT in `purchase_store.go` is difficult to maintain. Comments grouping columns by concern improve readability without requiring a full query builder.

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchase_store.go`

- [ ] **Step 1: Add grouping comments to the INSERT statement**

Replace the `query` string inside `CreatePurchase` (lines 38–54) with:

```go
	query := `
		INSERT INTO campaign_purchases (
			-- identity
			id, campaign_id, card_name, cert_number, card_number, set_name, grader, grade_value,
			-- costs
			cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents,
			-- dates
			population, purchase_date, created_at, updated_at,
			-- market snapshot
			last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
			active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
			-- provenance
			received_at, psa_ship_date, invoice_date, was_refunded, front_image_url, back_image_url, purchase_source,
			-- PSA
			psa_listing_title, snapshot_status, snapshot_retry_count,
			-- price overrides
			override_price_cents, override_source, override_set_at,
			-- AI suggestions
			ai_suggested_price_cents, ai_suggested_at,
			-- misc
			card_year, ebay_export_flagged_at,
			-- review
			reviewed_price_cents, reviewed_at, review_source,
			-- DH
			dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json, dh_status, dh_push_status, dh_candidates,
			-- gem/spec
			gem_rate_id, psa_spec_id
		) VALUES (
			-- identity
			?, ?, ?, ?, ?, ?, ?, ?,
			-- costs
			?, ?, ?,
			-- dates
			?, ?, ?, ?,
			-- market snapshot
			?, ?, ?, ?, ?, ?, ?, ?, ?,
			-- provenance
			?, ?, ?, ?, ?, ?, ?,
			-- PSA
			?, ?, ?,
			-- price overrides
			?, ?, ?,
			-- AI suggestions
			?, ?,
			-- misc
			?, ?,
			-- review
			?, ?, ?,
			-- DH
			?, ?, ?, ?, ?, ?, ?, ?,
			-- gem/spec
			?, ?
		)
	`
```

- [ ] **Step 2: Verify column count still matches (54 columns)**

Count the `?` placeholders in the new query. Expected: 54.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/storage/sqlite/...
go test -race ./internal/adapters/storage/sqlite/...
```

Expected: all pass. No functional change.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/purchase_store.go
git commit -m "docs: add positional grouping comments to CreatePurchase 54-column INSERT"
```

---

## Task 5: Add table-driven tests for critical purchase_store paths

**Why:** `GetPurchase`, `ListPurchasesByCampaign`, `GetPurchaseByCertNumber`, and `UpdatePurchase` are zero-coverage functions that are on the critical path of the application. Using an in-memory SQLite DB for testing these.

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchase_store_test.go` (create if absent)

- [ ] **Step 1: Confirm existing test file and imports**

Check `internal/adapters/storage/sqlite/purchase_store_test.go`. If it exists, read the top to understand existing test helpers (look for `setupTestDB()` or similar). If not, you'll create it.

- [ ] **Step 2: Add test helper for in-memory DB**

If no test DB helper exists, add one in `purchase_store_test.go`:

```go
package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	// Run migrations
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestPurchase(campaignID string) *inventory.Purchase {
	return &inventory.Purchase{
		ID:           "test-purchase-1",
		CampaignID:   campaignID,
		CardName:     "Charizard",
		CertNumber:   "12345678",
		CardNumber:   "4",
		SetName:      "Base Set",
		Grader:       "PSA",
		GradeValue:   9,
		BuyCostCents: 10000,
		PurchaseDate: time.Now().Format("2006-01-02"),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}
```

Note: check the actual package name and RunMigrations function. If migrations are run differently (e.g. via `MigrateDB`), use the correct function name by searching the codebase:

```bash
grep -r "func.*Migrat" /workspace/.worktrees/campaigns-decomposition/internal/adapters/storage/sqlite/ --include="*.go"
```

- [ ] **Step 3: Add TestPurchaseStore_GetPurchase**

```go
func TestPurchaseStore_GetPurchase(t *testing.T) {
	db := newTestDB(t)
	logger := observability.NewNopLogger()
	store := sqlite.NewPurchaseStore(db, logger)
	campaignStore := sqlite.NewCampaignStore(db, logger)
	ctx := context.Background()

	// Seed a campaign
	campaign := &inventory.Campaign{ID: "camp-1", Name: "Test Campaign"}
	if err := campaignStore.CreateCampaign(ctx, campaign); err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	// Seed a purchase
	p := newTestPurchase("camp-1")
	if err := store.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	tests := []struct {
		name      string
		id        string
		wantErr   bool
		wantFound bool
	}{
		{
			name:      "existing purchase",
			id:        "test-purchase-1",
			wantFound: true,
		},
		{
			name:    "not found returns ErrPurchaseNotFound",
			id:      "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.GetPurchase(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected purchase, got nil")
			}
			if got.ID != tt.id {
				t.Errorf("got ID %q, want %q", got.ID, tt.id)
			}
		})
	}
}
```

- [ ] **Step 4: Add TestPurchaseStore_ListPurchasesByCampaign**

```go
func TestPurchaseStore_ListPurchasesByCampaign(t *testing.T) {
	db := newTestDB(t)
	logger := observability.NewNopLogger()
	store := sqlite.NewPurchaseStore(db, logger)
	campaignStore := sqlite.NewCampaignStore(db, logger)
	ctx := context.Background()

	if err := campaignStore.CreateCampaign(ctx, &inventory.Campaign{ID: "camp-2", Name: "Test Campaign 2"}); err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	// Create 2 purchases for camp-2, 1 for camp-other
	p1 := newTestPurchase("camp-2")
	p1.ID = "p-1"
	p1.CertNumber = "11111111"
	p2 := newTestPurchase("camp-2")
	p2.ID = "p-2"
	p2.CertNumber = "22222222"
	pOther := newTestPurchase("camp-other")
	pOther.ID = "p-other"
	pOther.CertNumber = "33333333"

	for _, p := range []*inventory.Purchase{p1, p2, pOther} {
		// camp-other doesn't exist as campaign, create it first
		_ = campaignStore.CreateCampaign(ctx, &inventory.Campaign{ID: p.CampaignID, Name: p.CampaignID})
		if err := store.CreatePurchase(ctx, p); err != nil {
			t.Fatalf("create purchase %s: %v", p.ID, err)
		}
	}

	tests := []struct {
		name       string
		campaignID string
		wantCount  int
	}{
		{name: "two purchases for camp-2", campaignID: "camp-2", wantCount: 2},
		{name: "one purchase for camp-other", campaignID: "camp-other", wantCount: 1},
		{name: "empty for unknown campaign", campaignID: "unknown", wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.ListPurchasesByCampaign(ctx, tt.campaignID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("got %d purchases, want %d", len(got), tt.wantCount)
			}
		})
	}
}
```

- [ ] **Step 5: Add TestPurchaseStore_GetPurchaseByCertNumber**

```go
func TestPurchaseStore_GetPurchaseByCertNumber(t *testing.T) {
	db := newTestDB(t)
	logger := observability.NewNopLogger()
	store := sqlite.NewPurchaseStore(db, logger)
	campaignStore := sqlite.NewCampaignStore(db, logger)
	ctx := context.Background()

	if err := campaignStore.CreateCampaign(ctx, &inventory.Campaign{ID: "camp-3", Name: "Camp 3"}); err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	p := newTestPurchase("camp-3")
	p.ID = "p-cert-test"
	p.CertNumber = "99887766"
	if err := store.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	// Note: GetPurchaseByCertNumber signature is (ctx, grader, certNumber)
	tests := []struct {
		name       string
		grader     string
		certNumber string
		wantFound  bool
	}{
		{name: "existing cert returns purchase", grader: "PSA", certNumber: "99887766", wantFound: true},
		{name: "wrong grader returns nil", grader: "CGC", certNumber: "99887766", wantFound: false},
		{name: "missing cert returns nil without error", grader: "PSA", certNumber: "00000000", wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.GetPurchaseByCertNumber(ctx, tt.grader, tt.certNumber)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantFound && got == nil {
				t.Fatal("expected purchase, got nil")
			}
			if !tt.wantFound && got != nil {
				t.Errorf("expected nil, got purchase with ID %s", got.ID)
			}
		})
	}
}
```

- [ ] **Step 6: Run the new tests**

```bash
go test -race -v ./internal/adapters/storage/sqlite/... -run "TestPurchaseStore_GetPurchase|TestPurchaseStore_ListPurchasesByCampaign|TestPurchaseStore_GetPurchaseByCertNumber"
```

Expected: all pass.

Note: If `RunMigrations` or `NewCampaignStore` API doesn't match, check the actual sqlite package for available constructors and migration functions:

```bash
grep -r "func New.*Store\|func.*Migrat" /workspace/.worktrees/campaigns-decomposition/internal/adapters/storage/sqlite/ --include="*.go" | head -30
```

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/storage/sqlite/purchase_store_test.go
git commit -m "test: add table-driven tests for GetPurchase, ListPurchasesByCampaign, GetPurchaseByCertNumber"
```

---

## Verification

After all tasks:

```bash
go build ./...
go test -race -timeout 10m ./...
make check
```

Expected: all pass, no regressions.
