# DH Log-Noise & Functionality Fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate three dominant DH error patterns in production logs: missing grade params on `/recent-sales`, repeated 404s on dead DH card IDs, and infinite `partner_card_error` retries.

**Architecture:** Single PR. Backend Go changes in `internal/domain/pricing/`, `internal/adapters/clients/dh/`, `internal/adapters/clients/dhprice/`, `internal/adapters/scheduler/`, `internal/adapters/storage/postgres/`, plus a new migration and admin endpoints. One frontend change in `web/src/react/pages/admin/DHStatsPanel.tsx`. Hexagonal invariant preserved (new `DHCardTombstoneRepo` domain interface; postgres implements).

**Tech Stack:** Go 1.26 (hexagonal arch, table-driven tests, Fn-field mocks in `internal/testutil/mocks/`), Postgres via `pgx/v5`, `golang-migrate` embedded migrations, React/TypeScript frontend, Vite proxy.

**Spec:** `docs/superpowers/specs/2026-05-25-dh-log-noise-fixes-design.md`

**Worktree/branch:** `/workspace/.worktrees/dh-log-noise-fixes` on `dh-log-noise-fixes`.

---

## File Structure

**Create:**
- `internal/adapters/storage/postgres/migrations/000011_add_dh_card_tombstones.up.sql`
- `internal/adapters/storage/postgres/migrations/000011_add_dh_card_tombstones.down.sql`
- `internal/adapters/storage/postgres/dh_card_tombstone_store.go`
- `internal/adapters/storage/postgres/dh_card_tombstone_store_test.go`
- `internal/testutil/mocks/dh_card_tombstone_repo.go`
- `internal/adapters/httpserver/handlers/dh_tombstones_handler.go`
- `internal/adapters/httpserver/handlers/dh_tombstones_handler_test.go`

**Modify:**
- `internal/domain/pricing/provider.go` — add `Grade int` to `Card` and `CardLookup`; add `DHCardTombstoneRepo` interface.
- `internal/adapters/clients/dh/client.go` — `RecentSales(ctx, cardID, gradingCompany, grade)`; `MarketDataEnterprise(ctx, cardID, grade)`.
- `internal/adapters/clients/dhprice/provider.go` — widen internal `dhClient` interface; wire tombstone repo via functional option; pass grade through.
- `internal/adapters/clients/dhprice/provider_test.go` — update mock signature and add new tests.
- `internal/adapters/scheduler/price_refresh.go` — populate `pricing.Card{Grade: ...}` from purchase.
- `internal/adapters/scheduler/card_trajectory_refresh.go` — pass `grade=10` to `MarketDataEnterprise`; consult tombstone repo on 404.
- `internal/adapters/scheduler/dh_intelligence_refresh.go` — pass `grade=10` to `MarketDataEnterprise`; consult tombstone repo on 404.
- `internal/adapters/scheduler/dh_psa_import.go` — `partner_card_error` branch: increment counter, auto-dismiss at 5.
- `internal/adapters/scheduler/dh_psa_import_test.go` — partner_card_error tests.
- `internal/adapters/storage/postgres/purchase_dh_push_store.go` — broaden counter-reset CASE.
- `internal/adapters/storage/postgres/purchase_dh_push_store_test.go` — counter-reset tests.
- `internal/adapters/httpserver/router.go` (or wherever admin routes wire) — register two new endpoints.
- `cmd/slabledger/main.go` — construct postgres tombstone store, inject into providers/schedulers, register handlers.
- `web/src/react/pages/admin/DHStatsPanel.tsx` — add tombstone count + "Clear all" button.
- `web/src/react/queries/useAdminQueries.ts` (or sibling) — add `useDHTombstoneCount`, `useClearDHTombstones`.

---

## Task 1: Add `Grade` field to pricing types

**Files:**
- Modify: `internal/domain/pricing/provider.go:9-12,33-41`

- [ ] **Step 1: Add `Grade int` to `CardLookup` and `Card`**

Edit `internal/domain/pricing/provider.go`. In `CardLookup`:

```go
type CardLookup struct {
    Name            string
    Number          string
    PSAListingTitle string
    Grade           int // PSA grade (1-10); 0 means unknown
}
```

In `Card`:

```go
type Card struct {
    Name   string
    Number string
    Set    string
    PSAListingTitle string
    Grade           int // PSA grade (1-10); 0 means unknown / not graded
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/pricing/provider.go
git commit -m "pricing: add Grade field to Card and CardLookup"
```

---

## Task 2: Add `DHCardTombstoneRepo` interface

**Files:**
- Modify: `internal/domain/pricing/provider.go` (append)

- [ ] **Step 1: Append interface**

At the bottom of `internal/domain/pricing/provider.go`:

```go
// DHCardTombstoneRepo tracks DH card IDs that have repeatedly 404'd so we can
// stop hammering DH's lookup endpoint for IDs that no longer exist.
//
// Implementations MUST be safe for concurrent use.
type DHCardTombstoneRepo interface {
    // IsTombstoned returns true if the card has reached the tombstone threshold.
    IsTombstoned(ctx context.Context, cardID int) (bool, error)

    // RecordFailure increments the failure counter for cardID and returns the
    // new attempt count. Callers should treat attempts >= 3 as the tombstone
    // threshold.
    RecordFailure(ctx context.Context, cardID int, errMsg string) (attempts int, err error)

    // Clear removes the tombstone for a single card ID.
    Clear(ctx context.Context, cardID int) error

    // ClearAll removes all tombstones and returns the number removed.
    ClearAll(ctx context.Context) (cleared int, err error)

    // Count returns the number of cards currently tombstoned (attempts >= 3).
    Count(ctx context.Context) (int, error)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/pricing/provider.go
git commit -m "pricing: add DHCardTombstoneRepo interface"
```

---

## Task 3: Migration `000011_add_dh_card_tombstones`

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000011_add_dh_card_tombstones.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000011_add_dh_card_tombstones.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- 000011_add_dh_card_tombstones.up.sql
CREATE TABLE IF NOT EXISTS dh_card_tombstones (
    dh_card_id    BIGINT PRIMARY KEY,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts      INT NOT NULL DEFAULT 1,
    last_error    TEXT NOT NULL DEFAULT ''
);
```

- [ ] **Step 2: Write down migration**

```sql
-- 000011_add_dh_card_tombstones.down.sql
DROP TABLE IF EXISTS dh_card_tombstones;
```

- [ ] **Step 3: Build (embedded FS picks up the new files)**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/postgres/migrations/000011_add_dh_card_tombstones.*.sql
git commit -m "postgres: add dh_card_tombstones migration"
```

---

## Task 4: Postgres `DHCardTombstoneStore` (TDD)

**Files:**
- Create: `internal/adapters/storage/postgres/dh_card_tombstone_store.go`
- Test: `internal/adapters/storage/postgres/dh_card_tombstone_store_test.go`

- [ ] **Step 1: Write failing test**

Create `dh_card_tombstone_store_test.go`. Use the existing postgres test harness pattern (look at a sibling `*_store_test.go` for the `setupTestDB`/`newTestStore` pattern — copy it). Cases:

```go
func TestDHCardTombstoneStore(t *testing.T) {
    db := setupTestDB(t) // sibling pattern
    store := NewDHCardTombstoneStore(db)
    ctx := context.Background()

    // 1. Initially not tombstoned
    ts, err := store.IsTombstoned(ctx, 100)
    require.NoError(t, err)
    require.False(t, ts)

    // 2. First failure: attempts=1, not tombstoned
    n, err := store.RecordFailure(ctx, 100, "404")
    require.NoError(t, err)
    require.Equal(t, 1, n)
    ts, _ = store.IsTombstoned(ctx, 100)
    require.False(t, ts)

    // 3. Reach threshold at 3
    _, _ = store.RecordFailure(ctx, 100, "404")
    n, _ = store.RecordFailure(ctx, 100, "404")
    require.Equal(t, 3, n)
    ts, _ = store.IsTombstoned(ctx, 100)
    require.True(t, ts)

    // 4. Count
    c, err := store.Count(ctx)
    require.NoError(t, err)
    require.Equal(t, 1, c)

    // 5. Clear individual
    require.NoError(t, store.Clear(ctx, 100))
    ts, _ = store.IsTombstoned(ctx, 100)
    require.False(t, ts)

    // 6. ClearAll
    _, _ = store.RecordFailure(ctx, 200, "x")
    _, _ = store.RecordFailure(ctx, 200, "x")
    _, _ = store.RecordFailure(ctx, 200, "x")
    cleared, err := store.ClearAll(ctx)
    require.NoError(t, err)
    require.Equal(t, 1, cleared)
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/adapters/storage/postgres/ -run TestDHCardTombstoneStore -v`
Expected: compile error (`NewDHCardTombstoneStore` not defined).

- [ ] **Step 3: Implement store**

Create `internal/adapters/storage/postgres/dh_card_tombstone_store.go`:

```go
package postgres

import (
    "context"
    "database/sql"
    "fmt"
)

const tombstoneThreshold = 3

type DHCardTombstoneStore struct {
    db *sql.DB
}

func NewDHCardTombstoneStore(db *sql.DB) *DHCardTombstoneStore {
    return &DHCardTombstoneStore{db: db}
}

func (s *DHCardTombstoneStore) IsTombstoned(ctx context.Context, cardID int) (bool, error) {
    var attempts int
    err := s.db.QueryRowContext(ctx,
        `SELECT attempts FROM dh_card_tombstones WHERE dh_card_id = $1`,
        cardID,
    ).Scan(&attempts)
    if err == sql.ErrNoRows {
        return false, nil
    }
    if err != nil {
        return false, fmt.Errorf("is tombstoned: %w", err)
    }
    return attempts >= tombstoneThreshold, nil
}

func (s *DHCardTombstoneStore) RecordFailure(ctx context.Context, cardID int, errMsg string) (int, error) {
    var attempts int
    err := s.db.QueryRowContext(ctx,
        `INSERT INTO dh_card_tombstones (dh_card_id, attempts, last_error)
         VALUES ($1, 1, $2)
         ON CONFLICT (dh_card_id) DO UPDATE
           SET attempts = dh_card_tombstones.attempts + 1,
               last_seen_at = NOW(),
               last_error = EXCLUDED.last_error
         RETURNING attempts`,
        cardID, errMsg,
    ).Scan(&attempts)
    if err != nil {
        return 0, fmt.Errorf("record failure: %w", err)
    }
    return attempts, nil
}

func (s *DHCardTombstoneStore) Clear(ctx context.Context, cardID int) error {
    _, err := s.db.ExecContext(ctx, `DELETE FROM dh_card_tombstones WHERE dh_card_id = $1`, cardID)
    if err != nil {
        return fmt.Errorf("clear tombstone: %w", err)
    }
    return nil
}

func (s *DHCardTombstoneStore) ClearAll(ctx context.Context) (int, error) {
    res, err := s.db.ExecContext(ctx, `DELETE FROM dh_card_tombstones`)
    if err != nil {
        return 0, fmt.Errorf("clear all tombstones: %w", err)
    }
    n, err := res.RowsAffected()
    if err != nil {
        return 0, fmt.Errorf("clear all tombstones rows: %w", err)
    }
    return int(n), nil
}

func (s *DHCardTombstoneStore) Count(ctx context.Context) (int, error) {
    var n int
    err := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM dh_card_tombstones WHERE attempts >= $1`,
        tombstoneThreshold,
    ).Scan(&n)
    if err != nil {
        return 0, fmt.Errorf("count tombstones: %w", err)
    }
    return n, nil
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/adapters/storage/postgres/ -run TestDHCardTombstoneStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/dh_card_tombstone_store*.go
git commit -m "postgres: add DHCardTombstoneStore"
```

---

## Task 5: Mock for `DHCardTombstoneRepo`

**Files:**
- Create: `internal/testutil/mocks/dh_card_tombstone_repo.go`

- [ ] **Step 1: Write mock**

```go
package mocks

import "context"

type DHCardTombstoneRepoMock struct {
    IsTombstonedFn   func(ctx context.Context, cardID int) (bool, error)
    RecordFailureFn  func(ctx context.Context, cardID int, errMsg string) (int, error)
    ClearFn          func(ctx context.Context, cardID int) error
    ClearAllFn       func(ctx context.Context) (int, error)
    CountFn          func(ctx context.Context) (int, error)
}

func (m *DHCardTombstoneRepoMock) IsTombstoned(ctx context.Context, cardID int) (bool, error) {
    if m.IsTombstonedFn != nil {
        return m.IsTombstonedFn(ctx, cardID)
    }
    return false, nil
}
func (m *DHCardTombstoneRepoMock) RecordFailure(ctx context.Context, cardID int, errMsg string) (int, error) {
    if m.RecordFailureFn != nil {
        return m.RecordFailureFn(ctx, cardID, errMsg)
    }
    return 1, nil
}
func (m *DHCardTombstoneRepoMock) Clear(ctx context.Context, cardID int) error {
    if m.ClearFn != nil {
        return m.ClearFn(ctx, cardID)
    }
    return nil
}
func (m *DHCardTombstoneRepoMock) ClearAll(ctx context.Context) (int, error) {
    if m.ClearAllFn != nil {
        return m.ClearAllFn(ctx)
    }
    return 0, nil
}
func (m *DHCardTombstoneRepoMock) Count(ctx context.Context) (int, error) {
    if m.CountFn != nil {
        return m.CountFn(ctx)
    }
    return 0, nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/testutil/mocks/dh_card_tombstone_repo.go
git commit -m "mocks: add DHCardTombstoneRepoMock"
```

---

## Task 6: `dh.Client.RecentSales` grade params (TDD)

**Files:**
- Modify: `internal/adapters/clients/dh/client.go:121-137`
- Modify: any existing `dh/client_test.go` (if present) — else add new test file.

- [ ] **Step 1: Write failing test**

In `internal/adapters/clients/dh/client_test.go` (create if absent) — table-driven:

```go
func TestRecentSalesGradeParams(t *testing.T) {
    cases := []struct {
        name    string
        grade   int
        wantURL string
        wantErr bool
    }{
        {"grade10", 10, "/api/v1/enterprise/cards/123/recent-sales?grading_company=PSA&grade=10", false},
        {"grade9",  9,  "/api/v1/enterprise/cards/123/recent-sales?grading_company=PSA&grade=9",  false},
        {"missing", 0, "", true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            var gotURL string
            srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                gotURL = r.URL.RequestURI()
                w.Header().Set("Content-Type", "application/json")
                _, _ = w.Write([]byte(`{"sales":[]}`))
            }))
            defer srv.Close()
            c := NewClient(srv.URL, WithEnterpriseKey("k"))
            _, err := c.RecentSales(context.Background(), 123, "PSA", tc.grade)
            if tc.wantErr {
                require.Error(t, err)
                require.Empty(t, gotURL, "must not issue HTTP call when grade invalid")
                return
            }
            require.NoError(t, err)
            require.Equal(t, tc.wantURL, gotURL)
        })
    }
}
```

- [ ] **Step 2: Run, expect FAIL (compile)**

Run: `go test ./internal/adapters/clients/dh/ -run TestRecentSalesGradeParams -v`
Expected: FAIL — signature mismatch.

- [ ] **Step 3: Update `RecentSales`**

Replace `func (c *Client) RecentSales(ctx context.Context, cardID int) ([]RecentSale, error)` at `internal/adapters/clients/dh/client.go:121`:

```go
func (c *Client) RecentSales(ctx context.Context, cardID int, gradingCompany string, grade int) ([]RecentSale, error) {
    if grade <= 0 {
        return nil, apperrors.ProviderInvalidRequest(providerName,
            fmt.Errorf("grade required for recent-sales lookup (card_id=%d)", cardID))
    }
    fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/%d/recent-sales?grading_company=%s&grade=%d",
        c.baseURL, cardID, gradingCompany, grade)

    var resp struct {
        Sales []RecentSale `json:"sales"`
    }
    if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
        return nil, err
    }
    for i, sale := range resp.Sales {
        if sale.Price <= 0 || strings.TrimSpace(sale.SoldAt) == "" {
            return nil, apperrors.ProviderInvalidResponse(providerName,
                fmt.Errorf("sale[%d] has non-positive price or empty date for card_id=%d", i, cardID))
        }
    }
    return resp.Sales, nil
}
```

- [ ] **Step 4: Update `MarketDataEnterprise` to accept grade**

In `internal/adapters/clients/dh/client.go:141`:

```go
func (c *Client) MarketDataEnterprise(ctx context.Context, cardID int, grade int) (*MarketDataResponse, error) {
    lookup, err := c.CardLookup(ctx, cardID)
    if err != nil {
        return nil, err
    }
    // ...unchanged lookup→resp construction...
    sales, err := c.RecentSales(ctx, cardID, "PSA", grade)
    if err != nil {
        if c.logger != nil {
            c.logger.Warn(ctx, "dh: recent sales fetch failed, returning partial market data",
                observability.Int("card_id", cardID), observability.Err(err))
        }
    } else if len(sales) > 0 {
        resp.RecentSales = sales
        resp.HasData = true
    }
    return resp, nil
}
```

- [ ] **Step 5: Run test, expect PASS — also `go build ./...` to surface stale callers**

Run: `go test ./internal/adapters/clients/dh/ -v && go build ./... 2>&1 | head -40`
Expected: dh test PASS. Build will fail in `dhprice/provider.go`, `scheduler/card_trajectory_refresh.go`, `scheduler/dh_intelligence_refresh.go`, and tests — fixed in subsequent tasks.

- [ ] **Step 6: Commit (allow downstream callers temporarily broken)**

Don't commit yet — Task 7-9 fix the callers; commit after Task 9.

---

## Task 7: `dhprice.Provider` — thread grade + tombstone

**Files:**
- Modify: `internal/adapters/clients/dhprice/provider.go:20-30,80-130`
- Modify: `internal/adapters/clients/dhprice/provider_test.go` (signature updates)

- [ ] **Step 1: Widen internal `dhClient` interface**

In `internal/adapters/clients/dhprice/provider.go:20` change:

```go
type dhClient interface {
    RecentSales(ctx context.Context, cardID int, gradingCompany string, grade int) ([]dh.RecentSale, error)
    CardLookup(ctx context.Context, cardID int) (*dh.CardLookupResponse, error)
}
```

- [ ] **Step 2: Add tombstone repo via functional option**

Near other options in `provider.go`:

```go
func WithTombstoneRepo(r pricing.DHCardTombstoneRepo) Option {
    return func(p *Provider) { p.tombstones = r }
}
```

And add `tombstones pricing.DHCardTombstoneRepo` to the `Provider` struct.

- [ ] **Step 3: Update `GetPrice` to consult tombstone + pass grade**

Replace `GetPrice` body around line 80:

```go
func (p *Provider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    if p.client == nil || p.idResolver == nil {
        return nil, nil
    }

    extID, err := p.idResolver.GetExternalID(ctx, card.Name, card.Set, card.Number, providerKey)
    if err != nil {
        return nil, err
    }
    if extID == "" {
        return nil, nil
    }

    cardID, err := strconv.Atoi(extID)
    if err != nil {
        return nil, fmt.Errorf("dhprice: invalid card ID %q for card %s/%s/%s: %w", extID, card.Name, card.Set, card.Number, err)
    }

    // Tombstone short-circuit (fail-open on error).
    if p.tombstones != nil {
        if ts, tsErr := p.tombstones.IsTombstoned(ctx, cardID); tsErr == nil && ts {
            if p.logger != nil {
                p.logger.Debug(ctx, "dhprice: skipping tombstoned card",
                    observability.Int("dh_card_id", cardID))
            }
            return nil, nil
        }
    }

    sales, err := p.client.RecentSales(ctx, cardID, "PSA", card.Grade)
    if err != nil {
        if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderNotFound) && p.tombstones != nil {
            p.recordTombstone(ctx, cardID, err)
        }
        return nil, err
    }
    if len(sales) == 0 {
        return nil, nil
    }

    price := buildPrice(card.Name, sales)
    if price == nil {
        return nil, nil
    }

    lookup, err := p.client.CardLookup(ctx, cardID)
    if err != nil {
        if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderNotFound) && p.tombstones != nil {
            p.recordTombstone(ctx, cardID, err)
        }
        if p.logger != nil {
            p.logger.Warn(ctx, "dhprice: CardLookup failed (non-fatal)",
                observability.String("card", card.Name),
                observability.Err(err))
        }
    } else if lookup != nil && hasMarketData(&lookup.MarketData) {
        applyMarketData(price, &lookup.MarketData)
    }

    return price, nil
}

func (p *Provider) recordTombstone(ctx context.Context, cardID int, cause error) {
    n, err := p.tombstones.RecordFailure(ctx, cardID, cause.Error())
    if err != nil {
        if p.logger != nil {
            p.logger.Warn(ctx, "dhprice: tombstone record failed", observability.Err(err))
        }
        return
    }
    if p.logger != nil {
        if n >= 3 {
            p.logger.Info(ctx, "dhprice: dh card tombstoned, no further lookups",
                observability.Int("dh_card_id", cardID),
                observability.Int("attempts", n))
        } else {
            p.logger.Info(ctx, "dhprice: dh card lookup failed",
                observability.Int("dh_card_id", cardID),
                observability.Int("attempts", n))
        }
    }
}
```

Also extend `LookupCard` to forward `Grade`:

```go
func (p *Provider) LookupCard(ctx context.Context, setName string, card pricing.CardLookup) (*pricing.Price, error) {
    pc := pricing.Card{
        Name:            card.Name,
        Number:          card.Number,
        Set:             setName,
        PSAListingTitle: card.PSAListingTitle,
        Grade:           card.Grade,
    }
    return p.GetPrice(ctx, pc)
}
```

- [ ] **Step 4: Update test mock signature**

In `internal/adapters/clients/dhprice/provider_test.go`, every `RecentSalesFn: func(_ context.Context, _ int)` → `RecentSalesFn: func(_ context.Context, _ int, _ string, _ int)`. Use sed:

```bash
sed -i 's|RecentSalesFn: func(_ context.Context, _ int)|RecentSalesFn: func(_ context.Context, _ int, _ string, _ int)|g' internal/adapters/clients/dhprice/provider_test.go
```

Then update the mock-type definition in the test file (search for `type ... dhClient` mock; if mock is in same file, fix the method signature). Verify via:

```bash
grep -n "func.*RecentSales" internal/adapters/clients/dhprice/provider_test.go
```

Each should have the 4-arg signature.

- [ ] **Step 5: Add new tombstone tests**

Append to `provider_test.go`:

```go
func TestGetPrice_TombstoneHit_SkipsAPI(t *testing.T) {
    var called bool
    p := newTestProvider(t,
        withMockClient(&dhClientMock{
            RecentSalesFn: func(context.Context, int, string, int) ([]dh.RecentSale, error) {
                called = true
                return nil, nil
            },
        }),
        WithTombstoneRepo(&mocks.DHCardTombstoneRepoMock{
            IsTombstonedFn: func(context.Context, int) (bool, error) { return true, nil },
        }),
    )
    got, err := p.GetPrice(context.Background(), pricing.Card{Name: "X", Set: "Y", Number: "1", Grade: 10})
    require.NoError(t, err)
    require.Nil(t, got)
    require.False(t, called, "must not call DH when tombstoned")
}
```

(Adjust `newTestProvider` / `withMockClient` to whatever helpers the existing tests use.)

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/clients/dhprice/ -v`
Expected: PASS.

- [ ] **Step 7: Don't commit yet — Tasks 8-9 unblock build**

---

## Task 8: Scheduler `price_refresh.go` — populate Grade

**Files:**
- Modify: `internal/adapters/scheduler/price_refresh.go:205`

- [ ] **Step 1: Add Grade to the `pricing.Card{...}` construction**

Find the existing `pc := pricing.Card{...}` near line 205. Add `Grade: int(purchase.GradeValue)`. If `GradeValue` is a float, use `int(math.Round(purchase.GradeValue))` (verify type via `grep -n "GradeValue" internal/domain/inventory/types.go`).

```go
pc := pricing.Card{
    Name:   purchase.CardName,
    Set:    purchase.SetName,
    Number: purchase.CardNumber,
    Grade:  int(purchase.GradeValue),
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/adapters/scheduler/...`
Expected: succeeds for this file (other schedulers still broken by `MarketDataEnterprise` signature change; fixed in Task 9).

---

## Task 9: Scheduler `MarketDataEnterprise` callers — pass grade=10 + tombstone on 404

**Files:**
- Modify: `internal/adapters/scheduler/card_trajectory_refresh.go:~115`
- Modify: `internal/adapters/scheduler/dh_intelligence_refresh.go:136,271`

- [ ] **Step 1: Update calls + tombstone handling**

For each callsite in both files, replace:

```go
resp, fetchErr := s.dhClient.MarketDataEnterprise(ctx, cardIDInt)
```

with:

```go
const defaultIntelGrade = 10

if s.tombstones != nil {
    if ts, tsErr := s.tombstones.IsTombstoned(ctx, cardIDInt); tsErr == nil && ts {
        s.logger.Debug(ctx, "scheduler: skipping tombstoned DH card",
            observability.Int("dh_card_id", cardIDInt))
        continue // or return — match the existing loop pattern
    }
}
resp, fetchErr := s.dhClient.MarketDataEnterprise(ctx, cardIDInt, defaultIntelGrade)
if fetchErr != nil && apperrors.HasErrorCode(fetchErr, apperrors.ErrCodeProviderNotFound) && s.tombstones != nil {
    if n, recErr := s.tombstones.RecordFailure(ctx, cardIDInt, fetchErr.Error()); recErr == nil {
        if n >= 3 {
            s.logger.Info(ctx, "scheduler: dh card tombstoned, no further lookups",
                observability.Int("dh_card_id", cardIDInt), observability.Int("attempts", n))
        } else {
            s.logger.Info(ctx, "scheduler: dh card lookup failed",
                observability.Int("dh_card_id", cardIDInt), observability.Int("attempts", n))
        }
    }
}
```

- [ ] **Step 2: Add `tombstones pricing.DHCardTombstoneRepo` field + constructor option to both schedulers**

In each scheduler's struct + `NewXxxScheduler`, add the field. Use the existing functional-options pattern (look at sibling `With...` options). Add `WithTombstoneRepo(r) Option { return func(s *...) { s.tombstones = r } }`.

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 4: Run scheduler unit tests**

Run: `go test ./internal/adapters/scheduler/... -count=1`
Expected: PASS (existing tests should still pass since tombstones default to nil).

- [ ] **Step 5: Commit (single commit covering Tasks 6-9)**

```bash
git add internal/adapters/clients/dh/ \
        internal/adapters/clients/dhprice/ \
        internal/adapters/scheduler/price_refresh.go \
        internal/adapters/scheduler/card_trajectory_refresh.go \
        internal/adapters/scheduler/dh_intelligence_refresh.go
git commit -m "dh: require grade param for recent-sales; tombstone dead DH card IDs"
```

---

## Task 10: Broaden counter-reset in `UpdatePurchaseDHPushStatus` (TDD)

**Files:**
- Modify: `internal/adapters/storage/postgres/purchase_dh_push_store.go:65-75`
- Test: `internal/adapters/storage/postgres/purchase_dh_push_store_test.go` (create or extend)

- [ ] **Step 1: Write failing test**

```go
func TestUpdatePurchaseDHPushStatus_CounterReset(t *testing.T) {
    db := setupTestDB(t)
    store := NewPurchaseStore(db)
    ctx := context.Background()
    id := seedPurchaseWithAttempts(t, db, 4) // helper: inserts row with dh_push_attempts=4

    cases := []struct {
        status   string
        wantZero bool
    }{
        {"pending", true},
        {"matched", true},
        {"unmatched_created", true},
        {"override_corrected", true},
        {"already_listed", true},
        {"dismissed", false},
        {"unmatched", false},
        {"held", false},
    }
    for _, tc := range cases {
        t.Run(tc.status, func(t *testing.T) {
            resetPurchaseAttempts(t, db, id, 4) // helper: SET dh_push_attempts=4
            require.NoError(t, store.UpdatePurchaseDHPushStatus(ctx, id, tc.status))
            n := readAttempts(t, db, id)
            if tc.wantZero {
                require.Equal(t, 0, n)
            } else {
                require.Equal(t, 4, n)
            }
        })
    }
}
```

- [ ] **Step 2: Run, expect FAIL on `unmatched_created`/`override_corrected`/`already_listed`**

Run: `go test ./internal/adapters/storage/postgres/ -run TestUpdatePurchaseDHPushStatus_CounterReset -v`
Expected: FAIL on the three new statuses.

- [ ] **Step 3: Broaden CASE**

In `internal/adapters/storage/postgres/purchase_dh_push_store.go:65-75`:

```go
func (ps *PurchaseStore) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
    return ps.execAndExpectRow(ctx, "update DH push status",
        `UPDATE campaign_purchases
         SET dh_push_status = $1,
             dh_push_attempts = CASE WHEN $2 IN ('pending', 'matched', 'unmatched_created', 'override_corrected', 'already_listed')
                                     THEN 0 ELSE dh_push_attempts END,
             updated_at = $3
         WHERE id = $4`,
        status, status, time.Now(), id,
    )
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/adapters/storage/postgres/ -run TestUpdatePurchaseDHPushStatus_CounterReset -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/purchase_dh_push_store*.go
git commit -m "postgres: reset dh_push_attempts on all success transitions"
```

---

## Task 11: Auto-dismiss after 5 partner_card_error (TDD)

**Files:**
- Modify: `internal/adapters/scheduler/dh_push.go` — extend `DHPushScheduler` to hold a `purchaseRepo` field exposing `IncrementDHPushAttempts`.
- Modify: `internal/adapters/scheduler/dh_psa_import.go:126-132`
- Test: `internal/adapters/scheduler/dh_psa_import_test.go` (extend)

- [ ] **Step 1: Add `DHPushAttemptIncrementer` interface + scheduler field**

In `internal/adapters/scheduler/dh_push.go`, alongside other minor interfaces:

```go
// DHPushAttemptIncrementer atomically increments and returns the per-purchase
// dh_push_attempts counter. Used for auto-dismiss-after-N failure flows.
type DHPushAttemptIncrementer interface {
    IncrementDHPushAttempts(ctx context.Context, id string) (int, error)
}
```

Add `attemptInc DHPushAttemptIncrementer` to the `DHPushScheduler` struct. Add functional option:

```go
func WithDHPushAttemptIncrementer(a DHPushAttemptIncrementer) DHPushOption {
    return func(s *DHPushScheduler) { s.attemptInc = a }
}
```

- [ ] **Step 2: Write failing test**

In `dh_psa_import_test.go`:

```go
func TestPushViaPSAImport_PartnerCardError_AutoDismissAt5(t *testing.T) {
    cases := []struct {
        name           string
        attempts       int
        wantStatus     inventory.DHPushStatus
        wantStatusCall bool
    }{
        {"first attempt", 1, "", false},
        {"fourth attempt", 4, "", false},
        {"fifth attempt triggers dismiss", 5, inventory.DHPushStatusDismissed, true},
        {"sixth (safety)", 6, inventory.DHPushStatusDismissed, true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            var gotStatus inventory.DHPushStatus
            var statusCalled bool
            importer := &fakePSAImporter{
                resp: &dh.PSAImportResponse{Success: true, Results: []dh.PSAImportResult{
                    {Resolution: dh.PSAImportStatusPartnerCardError, Error: "blank identity"},
                }},
            }
            s := newTestScheduler(t,
                withPSAImporter(importer),
                withAttemptInc(&fakeAttemptInc{n: tc.attempts}),
                withStatusUpdater(&fakeStatusUpdater{onUpdate: func(_ context.Context, _ string, st inventory.DHPushStatus) error {
                    statusCalled = true
                    gotStatus = st
                    return nil
                }}),
            )
            res := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "C"})
            require.Equal(t, processSkipped, res)
            require.Equal(t, tc.wantStatusCall, statusCalled)
            if tc.wantStatusCall {
                require.Equal(t, tc.wantStatus, gotStatus)
            }
        })
    }
}
```

(`fakeAttemptInc`, `fakeStatusUpdater`, `newTestScheduler` patterns: mirror the existing test helpers in the file. If none exist, build minimal ones inline.)

- [ ] **Step 3: Run, expect FAIL**

Run: `go test ./internal/adapters/scheduler/ -run TestPushViaPSAImport_PartnerCardError_AutoDismissAt5 -v`
Expected: FAIL — current `partner_card_error` branch never dismisses.

- [ ] **Step 4: Rewrite the `partner_card_error` branch**

In `internal/adapters/scheduler/dh_psa_import.go`, replace `case dh.PSAImportStatusPartnerCardError:` at line 126:

```go
case dh.PSAImportStatusPartnerCardError:
    if s.attemptInc == nil {
        // Fallback: log + skip (legacy behavior, keeps tests without inc'er working).
        s.logger.Warn(ctx, "dh push: psa_import partner_card_error (no incrementer wired)",
            observability.String("purchaseID", p.ID),
            observability.String("cert", p.CertNumber),
            observability.String("dhError", result.Error))
        s.recordSkipEvent(ctx, p, "partner_card_error: "+result.Error)
        return processSkipped
    }
    attempts, incErr := s.attemptInc.IncrementDHPushAttempts(ctx, p.ID)
    if incErr != nil {
        s.logger.Warn(ctx, "dh push: failed to increment partner_card_error counter",
            observability.String("purchaseID", p.ID), observability.Err(incErr))
        s.recordSkipEvent(ctx, p, "partner_card_error: "+result.Error)
        return processSkipped
    }
    if attempts >= 5 {
        s.logger.Info(ctx, "dh push: auto-dismissing after 5 partner_card_error attempts",
            observability.String("purchaseID", p.ID),
            observability.String("cert", p.CertNumber),
            observability.String("dhError", result.Error))
        if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusDismissed); updateErr != nil {
            s.logger.Error(ctx, "dh push: failed to auto-dismiss",
                observability.String("purchaseID", p.ID), observability.Err(updateErr))
        }
        s.recordSkipEvent(ctx, p, "auto_dismissed_partner_card_error: "+result.Error)
        return processSkipped
    }
    s.logger.Debug(ctx, "dh push: partner_card_error, leaving pending",
        observability.String("purchaseID", p.ID),
        observability.String("cert", p.CertNumber),
        observability.Int("attempts", attempts),
        observability.String("dhError", result.Error))
    s.recordSkipEvent(ctx, p, "partner_card_error: "+result.Error)
    return processSkipped
```

- [ ] **Step 5: Run, expect PASS**

Run: `go test ./internal/adapters/scheduler/ -run TestPushViaPSAImport_PartnerCardError_AutoDismissAt5 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/scheduler/dh_push.go \
        internal/adapters/scheduler/dh_psa_import.go \
        internal/adapters/scheduler/dh_psa_import_test.go
git commit -m "scheduler: auto-dismiss DH push after 5 partner_card_error attempts"
```

---

## Task 12: Admin endpoints — tombstone count + clear (TDD)

**Files:**
- Create: `internal/adapters/httpserver/handlers/dh_tombstones_handler.go`
- Test: `internal/adapters/httpserver/handlers/dh_tombstones_handler_test.go`
- Modify: `internal/adapters/httpserver/router.go` (or wherever admin routes are wired — check `grep -rn "admin/dh-reconcile" internal/adapters/httpserver/`)

- [ ] **Step 1: Write failing test**

```go
func TestDHTombstonesHandler(t *testing.T) {
    repo := &mocks.DHCardTombstoneRepoMock{
        CountFn:    func(context.Context) (int, error) { return 42, nil },
        ClearAllFn: func(context.Context) (int, error) { return 42, nil },
    }
    h := NewDHTombstonesHandler(repo, observability.NewNoopLogger())

    t.Run("count", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodGet, "/api/admin/dh-tombstones/count", nil)
        rr := httptest.NewRecorder()
        h.HandleCount(rr, req)
        require.Equal(t, http.StatusOK, rr.Code)
        var body struct{ Count int `json:"count"` }
        require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
        require.Equal(t, 42, body.Count)
    })
    t.Run("clear", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodPost, "/api/admin/dh-tombstones/clear", nil)
        rr := httptest.NewRecorder()
        h.HandleClearAll(rr, req)
        require.Equal(t, http.StatusOK, rr.Code)
        var body struct{ Cleared int `json:"cleared"` }
        require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
        require.Equal(t, 42, body.Cleared)
    })
}
```

- [ ] **Step 2: Run, expect FAIL (compile)**

Run: `go test ./internal/adapters/httpserver/handlers/ -run TestDHTombstonesHandler -v`
Expected: FAIL.

- [ ] **Step 3: Implement handler**

```go
package handlers

import (
    "encoding/json"
    "net/http"

    "github.com/guarzo/slabledger/internal/domain/observability"
    "github.com/guarzo/slabledger/internal/domain/pricing"
)

type DHTombstonesHandler struct {
    repo   pricing.DHCardTombstoneRepo
    logger observability.Logger
}

func NewDHTombstonesHandler(repo pricing.DHCardTombstoneRepo, logger observability.Logger) *DHTombstonesHandler {
    return &DHTombstonesHandler{repo: repo, logger: logger}
}

func (h *DHTombstonesHandler) HandleCount(w http.ResponseWriter, r *http.Request) {
    n, err := h.repo.Count(r.Context())
    if err != nil {
        h.logger.Error(r.Context(), "tombstones count", observability.Err(err))
        writeError(w, http.StatusInternalServerError, "count failed")
        return
    }
    writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

func (h *DHTombstonesHandler) HandleClearAll(w http.ResponseWriter, r *http.Request) {
    n, err := h.repo.ClearAll(r.Context())
    if err != nil {
        h.logger.Error(r.Context(), "tombstones clear", observability.Err(err))
        writeError(w, http.StatusInternalServerError, "clear failed")
        return
    }
    writeJSON(w, http.StatusOK, map[string]int{"cleared": n})
}
```

- [ ] **Step 4: Register routes**

Locate the admin route block (`grep -rn "dh-reconcile/trigger" internal/adapters/httpserver/`) and add adjacent entries:

```go
mux.Handle("GET /api/admin/dh-tombstones/count", adminMiddleware(http.HandlerFunc(dhTombstonesHandler.HandleCount)))
mux.Handle("POST /api/admin/dh-tombstones/clear", adminMiddleware(http.HandlerFunc(dhTombstonesHandler.HandleClearAll)))
```

Pattern-match against the existing admin route style — copy verbatim.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/httpserver/handlers/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_tombstones_handler*.go \
        internal/adapters/httpserver/router.go
git commit -m "http: add admin endpoints for DH card tombstones"
```

---

## Task 13: Wire everything in `cmd/slabledger/main.go`

**Files:**
- Modify: `cmd/slabledger/main.go`

- [ ] **Step 1: Construct tombstone store + inject**

Find existing `NewPurchaseStore` / `NewDHPushScheduler` / `NewDHPriceProvider` construction. Add:

```go
tombstoneStore := postgres.NewDHCardTombstoneStore(db)
```

Pass `dhprice.WithTombstoneRepo(tombstoneStore)` when constructing the price provider. Pass `scheduler.WithTombstoneRepo(tombstoneStore)` to `card_trajectory_refresh` and `dh_intelligence_refresh`. Pass `scheduler.WithDHPushAttemptIncrementer(purchaseStore)` to `NewDHPushScheduler`.

Construct and register the admin handler:

```go
dhTombstonesHandler := handlers.NewDHTombstonesHandler(tombstoneStore, logger)
// ...pass into router wiring...
```

- [ ] **Step 2: Build + run unit tests**

Run: `go build ./... && go test -race ./... -timeout 5m`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/slabledger/main.go
git commit -m "cmd: wire DH tombstone store + push attempt incrementer"
```

---

## Task 14: Frontend — tombstone panel UI

**Files:**
- Modify: `web/src/react/queries/useAdminQueries.ts`
- Modify: `web/src/react/pages/admin/DHStatsPanel.tsx`

- [ ] **Step 1: Add hooks**

Append to `useAdminQueries.ts` (follow the sibling `useDHStatus`/`useTriggerDHReconcile` pattern):

```ts
export function useDHTombstoneCount(opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ['dh-tombstone-count'],
    queryFn: async () => {
      const res = await api.get<{ count: number }>('/api/admin/dh-tombstones/count');
      return res.count;
    },
    enabled: opts?.enabled ?? true,
  });
}

export function useClearDHTombstones() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const res = await api.post<{ cleared: number }>('/api/admin/dh-tombstones/clear', {});
      return res.cleared;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['dh-tombstone-count'] }),
  });
}
```

- [ ] **Step 2: Render in `DHStatsPanel.tsx`**

Inside the panel (alongside the existing reconcile row), add:

```tsx
const { data: tombstoneCount } = useDHTombstoneCount({ enabled });
const clearTombstones = useClearDHTombstones();

const handleClearTombstones = async () => {
  if (!confirm('Clear all DH card tombstones? Cards will retry on next cycle.')) return;
  try {
    const cleared = await clearTombstones.mutateAsync();
    toast.success(`Cleared ${cleared} tombstones`);
  } catch {
    toast.error('Clear tombstones failed');
  }
};
```

In the JSX, add a row matching the reconcile row's structure:

```tsx
<div className="flex items-center justify-between">
  <div>
    <div className="text-sm font-medium">DH card tombstones</div>
    <div className="text-xs text-[var(--text-muted)]">
      {tombstoneCount ?? 0} card IDs skipped after 3 consecutive 404s
    </div>
  </div>
  <Button size="sm" onClick={handleClearTombstones} disabled={clearTombstones.isPending}>
    Clear all
  </Button>
</div>
```

- [ ] **Step 3: Build frontend**

Run: `cd web && npm run build`
Expected: build succeeds (no TS errors).

- [ ] **Step 4: Commit**

```bash
git add web/src/react/queries/useAdminQueries.ts web/src/react/pages/admin/DHStatsPanel.tsx
git commit -m "web: surface DH tombstone count + clear-all on admin panel"
```

---

## Task 15: Manual verification — live DH curl

**Files:** none

- [ ] **Step 1: Curl recent-sales with grade params**

Pick a known-good DH card ID (e.g. 46809 from the production logs). With `DH_ENTERPRISE_API_KEY` from `/workspace/.env`:

```bash
source /workspace/.env
curl -sS -A "slabledger/1.0" \
  -H "Authorization: Bearer $DH_ENTERPRISE_API_KEY" \
  "$DH_API_BASE_URL/api/v1/enterprise/cards/46809/recent-sales?grading_company=PSA&grade=9" | jq '.sales | length, .sales[0]'
```

Expected: a sales count > 0 and a sale object with `Price` and `SoldAt` populated, matching the existing `dh.RecentSale` Go struct field tags.

- [ ] **Step 2: Curl a known-dead ID (should 404)**

```bash
curl -sS -A "slabledger/1.0" \
  -H "Authorization: Bearer $DH_ENTERPRISE_API_KEY" \
  -o /dev/null -w "%{http_code}\n" \
  "$DH_API_BASE_URL/api/v1/enterprise/cards/lookup?card_id=82648"
```

Expected: `404`.

---

## Task 16: Quality gates + PR

**Files:** none

- [ ] **Step 1: Full check**

```bash
make check
go test -race -timeout 10m ./...
cd web && npm run build && cd -
```

Expected: all green.

- [ ] **Step 2: Push branch + open PR**

```bash
git push -u origin dh-log-noise-fixes
gh pr create --title "DH log-noise & functionality fixes" --body "$(cat <<'EOF'
## Summary
- Thread PSA grade through DH `/recent-sales` (was 400'ing every call)
- Tombstone DH card IDs after 3 consecutive 404s; admin "Clear all" escape hatch
- Auto-dismiss DH push purchases after 5 `partner_card_error` attempts; counter resets on success transitions

Spec: docs/superpowers/specs/2026-05-25-dh-log-noise-fixes-design.md

## Test plan
- [ ] Unit tests pass (`go test -race ./...`)
- [ ] `make check` clean
- [ ] `npm run build` clean
- [ ] Live curl against DH `/recent-sales?grading_company=PSA&grade=9` returns 200
- [ ] After deploy: log volume for `grading_required` drops to 0; 404 storm tombstones within ~3 cycles; stuck `partner_card_error` rows land in Skipped tab after 5 cycles
EOF
)"
```

---

## Self-review notes

- Spec sections covered: Fix #1 → Tasks 1, 6, 7, 8, 9, 14 (front-end). Fix #2 → Tasks 2, 3, 4, 5, 7, 9, 12, 13, 14. Fix #3 → Tasks 10, 11.
- Counter-reset list matches spec.
- Tombstone threshold = 3 (matches spec).
- Auto-dismiss threshold = 5 (matches spec).
- `grading_company` hardcoded `"PSA"` per spec.
- Intelligence sweeps default `grade=10` per spec.
- Fail-open on tombstone-repo error preserved (Task 7, Task 9).
- No per-ID tombstone UI (per spec out-of-scope).
- No backfill (per spec out-of-scope).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-25-dh-log-noise-fixes.md`. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
