# Sell Sheet Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix sell sheet print truncation, add server-side persistence for cross-device sync, and create a dedicated mobile sell sheet view.

**Architecture:** New `sell_sheet_items` table with REST API endpoints. Print layout drops analytics columns. Mobile gets a dedicated compact view when sell sheet tab is active. Frontend hook switches from localStorage to React Query with optimistic updates.

**Tech Stack:** Go 1.26, SQLite, React, TypeScript, TanStack React Query, TanStack Virtual, Tailwind CSS

---

### Task 1: Database Migration

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000032_sell_sheet_items.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000032_sell_sheet_items.down.sql`

- [ ] **Step 1: Create up migration**

```sql
CREATE TABLE sell_sheet_items (
    user_id     INTEGER NOT NULL,
    purchase_id TEXT    NOT NULL,
    added_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, purchase_id)
);
CREATE INDEX idx_sell_sheet_items_user ON sell_sheet_items(user_id);
```

- [ ] **Step 2: Create down migration**

```sql
DROP INDEX IF EXISTS idx_sell_sheet_items_user;
DROP TABLE IF EXISTS sell_sheet_items;
```

- [ ] **Step 3: Verify migration runs**

Run: `go build ./... && go test ./internal/adapters/storage/sqlite/ -run TestMigrations -v -count=1`
Expected: PASS (migrations auto-run on DB open)

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/000032_sell_sheet_items.*
git commit -m "feat: add sell_sheet_items migration"
```

---

### Task 2: Domain Interface + Mock Stubs

**Files:**
- Modify: `internal/domain/campaigns/repository.go`
- Modify: `internal/domain/campaigns/mock_repo_test.go`
- Modify: `internal/testutil/mocks/campaign_repository.go`

- [ ] **Step 1: Add SellSheetRepository sub-interface**

In `internal/domain/campaigns/repository.go`, add above the composed `Repository` interface (before line 124):

```go
// SellSheetRepository handles sell sheet item persistence.
type SellSheetRepository interface {
	GetSellSheetItems(ctx context.Context, userID int64) ([]string, error)
	AddSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error
	RemoveSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error
	ClearSellSheet(ctx context.Context, userID int64) error
}
```

Then embed `SellSheetRepository` in the composed `Repository` interface:

```go
type Repository interface {
	CampaignCRUD
	PurchaseRepository
	SaleRepository
	AnalyticsRepository
	FinanceRepository
	RevocationRepository
	PriceReviewRepository
	SellSheetRepository
}
```

- [ ] **Step 2: Add stubs to in-package mock**

In `internal/domain/campaigns/mock_repo_test.go`, add after the existing method stubs:

```go
// --- SellSheetRepository stubs ---

func (m *mockRepo) GetSellSheetItems(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
}
func (m *mockRepo) AddSellSheetItems(_ context.Context, _ int64, _ []string) error {
	return nil
}
func (m *mockRepo) RemoveSellSheetItems(_ context.Context, _ int64, _ []string) error {
	return nil
}
func (m *mockRepo) ClearSellSheet(_ context.Context, _ int64) error {
	return nil
}
```

- [ ] **Step 3: Add stubs to shared mock**

In `internal/testutil/mocks/campaign_repository.go`, add function fields to the `MockCampaignRepository` struct:

```go
GetSellSheetItemsFn   func(ctx context.Context, userID int64) ([]string, error)
AddSellSheetItemsFn   func(ctx context.Context, userID int64, purchaseIDs []string) error
RemoveSellSheetItemsFn func(ctx context.Context, userID int64, purchaseIDs []string) error
ClearSellSheetFn      func(ctx context.Context, userID int64) error
```

And add the method implementations at the end of the file:

```go
func (m *MockCampaignRepository) GetSellSheetItems(ctx context.Context, userID int64) ([]string, error) {
	if m.GetSellSheetItemsFn != nil {
		return m.GetSellSheetItemsFn(ctx, userID)
	}
	return nil, nil
}

func (m *MockCampaignRepository) AddSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if m.AddSellSheetItemsFn != nil {
		return m.AddSellSheetItemsFn(ctx, userID, purchaseIDs)
	}
	return nil
}

func (m *MockCampaignRepository) RemoveSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if m.RemoveSellSheetItemsFn != nil {
		return m.RemoveSellSheetItemsFn(ctx, userID, purchaseIDs)
	}
	return nil
}

func (m *MockCampaignRepository) ClearSellSheet(ctx context.Context, userID int64) error {
	if m.ClearSellSheetFn != nil {
		return m.ClearSellSheetFn(ctx, userID)
	}
	return nil
}
```

- [ ] **Step 4: Verify everything compiles**

Run: `go build ./...`
Expected: Success (no errors)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/domain/campaigns/... ./internal/testutil/mocks/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/repository.go internal/domain/campaigns/mock_repo_test.go internal/testutil/mocks/campaign_repository.go
git commit -m "feat: add SellSheetRepository interface and mock stubs"
```

---

### Task 3: SQLite Repository Implementation

**Files:**
- Create: `internal/adapters/storage/sqlite/sell_sheet_repository.go`

- [ ] **Step 1: Write the repository tests**

Run the tests first to verify they fail, then implement. But since the repository methods are simple CRUD against the DB, and the integration test infrastructure already runs migrations, write the implementation directly and test via the handler tests in Task 5.

- [ ] **Step 2: Write the SQLite implementation**

Create `internal/adapters/storage/sqlite/sell_sheet_repository.go`:

```go
package sqlite

import (
	"context"
	"fmt"
	"strings"
)

// GetSellSheetItems returns all purchase IDs on the user's sell sheet.
func (r *CampaignsRepository) GetSellSheetItems(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT purchase_id FROM sell_sheet_items WHERE user_id = ? ORDER BY added_at`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("get sell sheet items: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan sell sheet item: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AddSellSheetItems adds purchase IDs to the user's sell sheet (idempotent).
func (r *CampaignsRepository) AddSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO sell_sheet_items (user_id, purchase_id) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare add sell sheet: %w", err)
	}
	defer stmt.Close()

	for _, id := range purchaseIDs {
		if _, err := stmt.ExecContext(ctx, userID, id); err != nil {
			return fmt.Errorf("add sell sheet item %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// RemoveSellSheetItems removes specific purchase IDs from the user's sell sheet.
func (r *CampaignsRepository) RemoveSellSheetItems(ctx context.Context, userID int64, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(purchaseIDs))
	args := make([]any, 0, len(purchaseIDs)+1)
	args = append(args, userID)
	for i, id := range purchaseIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`DELETE FROM sell_sheet_items WHERE user_id = ? AND purchase_id IN (%s)`,
		strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("remove sell sheet items: %w", err)
	}
	return nil
}

// ClearSellSheet removes all items from the user's sell sheet.
func (r *CampaignsRepository) ClearSellSheet(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM sell_sheet_items WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("clear sell sheet: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/sell_sheet_repository.go
git commit -m "feat: add SQLite sell sheet repository implementation"
```

---

### Task 4: HTTP Handler

**Files:**
- Create: `internal/adapters/httpserver/handlers/sell_sheet_items.go`

- [ ] **Step 1: Create the handler**

Create `internal/adapters/httpserver/handlers/sell_sheet_items.go`:

```go
package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SellSheetItemsHandler handles sell sheet item persistence endpoints.
type SellSheetItemsHandler struct {
	repo   campaigns.SellSheetRepository
	logger observability.Logger
}

// NewSellSheetItemsHandler creates a new sell sheet items handler.
func NewSellSheetItemsHandler(repo campaigns.SellSheetRepository, logger observability.Logger) *SellSheetItemsHandler {
	return &SellSheetItemsHandler{repo: repo, logger: logger}
}

// HandleGetItems handles GET /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleGetItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	ids, err := h.repo.GetSellSheetItems(r.Context(), user.ID)
	if err != nil {
		h.logger.Error(r.Context(), "get sell sheet items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"purchaseIds": ids})
}

// HandleAddItems handles PUT /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleAddItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.PurchaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one purchase ID is required")
		return
	}
	if err := h.repo.AddSellSheetItems(r.Context(), user.ID, req.PurchaseIDs); err != nil {
		h.logger.Error(r.Context(), "add sell sheet items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleRemoveItems handles DELETE /api/sell-sheet/items.
func (h *SellSheetItemsHandler) HandleRemoveItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	var req struct {
		PurchaseIDs []string `json:"purchaseIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.PurchaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one purchase ID is required")
		return
	}
	if err := h.repo.RemoveSellSheetItems(r.Context(), user.ID, req.PurchaseIDs); err != nil {
		h.logger.Error(r.Context(), "remove sell sheet items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleClearItems handles DELETE /api/sell-sheet/items/all.
func (h *SellSheetItemsHandler) HandleClearItems(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	if err := h.repo.ClearSellSheet(r.Context(), user.ID); err != nil {
		h.logger.Error(r.Context(), "clear sell sheet failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/httpserver/handlers/sell_sheet_items.go
git commit -m "feat: add sell sheet items HTTP handler"
```

---

### Task 5: Wire Handler into Router

**Files:**
- Modify: `internal/adapters/httpserver/router.go`
- Modify: `internal/adapters/httpserver/routes.go`

- [ ] **Step 1: Add handler field and config to Router**

In `internal/adapters/httpserver/router.go`, add to the `Router` struct (after `dhHandler` field, around line 45):

```go
sellSheetItemsHandler *handlers.SellSheetItemsHandler
```

Add to `RouterConfig` struct (after `DHHandler` field, around line 77):

```go
SellSheetItemsHandler *handlers.SellSheetItemsHandler // Sell sheet persistence; nil = disabled
```

In `NewRouter`, after the block that sets `rt.dhHandler` (search for where other handlers are conditionally set), add:

```go
if cfg.SellSheetItemsHandler != nil {
	rt.sellSheetItemsHandler = cfg.SellSheetItemsHandler
}
```

- [ ] **Step 2: Register routes**

In `internal/adapters/httpserver/routes.go`, find the block that registers sell sheet routes (around line 100-112). After the existing sell sheet routes (after line 112, before `mux.HandleFunc("/sell-sheet", ...)`), add:

```go
// Sell sheet item persistence
if rt.sellSheetItemsHandler != nil && rt.authMW != nil {
	mux.Handle("GET /api/sell-sheet/items", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleGetItems)))
	mux.Handle("PUT /api/sell-sheet/items", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleAddItems)))
	mux.Handle("DELETE /api/sell-sheet/items", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleRemoveItems)))
	mux.Handle("DELETE /api/sell-sheet/items/all", rt.authMW.RequireAuth(http.HandlerFunc(rt.sellSheetItemsHandler.HandleClearItems)))
}
```

- [ ] **Step 3: Wire handler in main**

Search for where `SellSheetItemsHandler` needs to be created and passed to `RouterConfig`. Find where `NewCampaignsRepository` is called in `cmd/slabledger/main.go` (or wherever the app is wired up). After the campaigns repo is created, add:

```go
sellSheetItemsHandler := handlers.NewSellSheetItemsHandler(campaignsRepo, logger)
```

And add to `RouterConfig`:

```go
SellSheetItemsHandler: sellSheetItemsHandler,
```

- [ ] **Step 4: Verify compilation and tests**

Run: `go build ./... && go test ./internal/adapters/httpserver/... -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/router.go internal/adapters/httpserver/routes.go cmd/
git commit -m "feat: wire sell sheet items handler into router"
```

---

### Task 6: Frontend API Client Methods

**Files:**
- Modify: `web/src/js/api/campaigns.ts`

- [ ] **Step 1: Add type declarations**

In `web/src/js/api/campaigns.ts`, add to the declaration merging block (inside `interface APIClient`), after the existing sell sheet methods (around line 75):

```typescript
// Sell sheet item persistence
getSellSheetItems(): Promise<{ purchaseIds: string[] }>;
addSellSheetItems(purchaseIds: string[]): Promise<void>;
removeSellSheetItems(purchaseIds: string[]): Promise<void>;
clearSellSheetItems(): Promise<void>;
```

- [ ] **Step 2: Add implementations**

After the existing sell sheet method implementations (around line 269), add:

```typescript
proto.getSellSheetItems = async function (this: APIClient): Promise<{ purchaseIds: string[] }> {
  return this.get<{ purchaseIds: string[] }>('/sell-sheet/items');
};

proto.addSellSheetItems = async function (this: APIClient, purchaseIds: string[]): Promise<void> {
  await this.put('/sell-sheet/items', { purchaseIds });
};

proto.removeSellSheetItems = async function (this: APIClient, purchaseIds: string[]): Promise<void> {
  await this.delete('/sell-sheet/items', { purchaseIds });
};

proto.clearSellSheetItems = async function (this: APIClient): Promise<void> {
  await this.delete('/sell-sheet/items/all');
};
```

Note: Check whether the `APIClient` base class has `put` and `delete` methods that accept a body. If `delete` doesn't accept a body, use `this.request('DELETE', '/sell-sheet/items', { body: { purchaseIds } })` or whatever the underlying request method signature is.

- [ ] **Step 3: Add query key**

In `web/src/react/queries/queryKeys.ts`, add to the `portfolio` section (after `globalInventory`):

```typescript
sellSheetItems: ['portfolio', 'sellSheetItems'] as const,
```

- [ ] **Step 4: Commit**

```bash
git add web/src/js/api/campaigns.ts web/src/react/queries/queryKeys.ts
git commit -m "feat: add sell sheet items API client methods and query key"
```

---

### Task 7: Rewrite useSellSheet Hook

**Files:**
- Modify: `web/src/react/hooks/useSellSheet.ts`
- Modify: `web/src/react/hooks/useSellSheet.test.ts`

- [ ] **Step 1: Rewrite the hook**

Replace the contents of `web/src/react/hooks/useSellSheet.ts`:

```typescript
import { useCallback, useMemo, useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from '../queries/queryKeys';

const LEGACY_STORAGE_KEY = 'sellSheetIds';

export interface SellSheetHook {
  /** Set of purchase IDs currently on the sell sheet */
  items: Set<string>;
  /** Add purchase IDs to the sell sheet */
  add: (ids: string[]) => void;
  /** Remove purchase IDs from the sell sheet */
  remove: (ids: string[]) => void;
  /** Clear all items from the sell sheet */
  clear: () => void;
  /** Check if a purchase ID is on the sell sheet */
  has: (id: string) => boolean;
  /** Number of items on the sell sheet */
  count: number;
  /** Whether the initial load is in progress */
  isLoading: boolean;
}

export function useSellSheet(): SellSheetHook {
  const queryClient = useQueryClient();
  const migratedRef = useRef(false);

  const { data: ids = [], isLoading } = useQuery({
    queryKey: queryKeys.portfolio.sellSheetItems,
    queryFn: async () => {
      const res = await api.getSellSheetItems();
      return res.purchaseIds;
    },
    staleTime: 30_000,
  });

  const itemsSet = useMemo(() => new Set(ids), [ids]);

  // One-time migration from localStorage
  useEffect(() => {
    if (isLoading || migratedRef.current) return;
    migratedRef.current = true;
    try {
      const raw = localStorage.getItem(LEGACY_STORAGE_KEY);
      if (!raw) return;
      const legacyIds: string[] = JSON.parse(raw);
      if (!Array.isArray(legacyIds) || legacyIds.length === 0) return;
      // Only migrate if server is empty (avoid duplicating on re-renders)
      if (ids.length > 0) {
        localStorage.removeItem(LEGACY_STORAGE_KEY);
        return;
      }
      api.addSellSheetItems(legacyIds).then(() => {
        localStorage.removeItem(LEGACY_STORAGE_KEY);
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      });
    } catch {
      // Corrupted localStorage — just remove it
      localStorage.removeItem(LEGACY_STORAGE_KEY);
    }
  }, [isLoading, ids, queryClient]);

  const addMutation = useMutation({
    mutationFn: (newIds: string[]) => api.addSellSheetItems(newIds),
    onMutate: async (newIds) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      const prev = queryClient.getQueryData<string[]>(queryKeys.portfolio.sellSheetItems) ?? [];
      const merged = Array.from(new Set([...prev, ...newIds]));
      queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, merged);
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, context.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
    },
  });

  const removeMutation = useMutation({
    mutationFn: (removeIds: string[]) => api.removeSellSheetItems(removeIds),
    onMutate: async (removeIds) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      const prev = queryClient.getQueryData<string[]>(queryKeys.portfolio.sellSheetItems) ?? [];
      const removeSet = new Set(removeIds);
      queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, prev.filter(id => !removeSet.has(id)));
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, context.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
    },
  });

  const clearMutation = useMutation({
    mutationFn: () => api.clearSellSheetItems(),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      const prev = queryClient.getQueryData<string[]>(queryKeys.portfolio.sellSheetItems) ?? [];
      queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, []);
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, context.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
    },
  });

  const add = useCallback((newIds: string[]) => addMutation.mutate(newIds), [addMutation]);
  const remove = useCallback((removeIds: string[]) => removeMutation.mutate(removeIds), [removeMutation]);
  const clear = useCallback(() => clearMutation.mutate(), [clearMutation]);
  const has = useCallback((id: string) => itemsSet.has(id), [itemsSet]);

  return { items: itemsSet, add, remove, clear, has, count: itemsSet.size, isLoading };
}
```

- [ ] **Step 2: Rewrite the tests**

Replace the contents of `web/src/react/hooks/useSellSheet.test.ts`:

```typescript
import { renderHook, act, waitFor } from '@testing-library/react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { useSellSheet } from './useSellSheet';

// Mock the API module
vi.mock('../../js/api', () => ({
  api: {
    getSellSheetItems: vi.fn().mockResolvedValue({ purchaseIds: [] }),
    addSellSheetItems: vi.fn().mockResolvedValue(undefined),
    removeSellSheetItems: vi.fn().mockResolvedValue(undefined),
    clearSellSheetItems: vi.fn().mockResolvedValue(undefined),
  },
}));

import { api } from '../../js/api';

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
}

describe('useSellSheet', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (api.getSellSheetItems as ReturnType<typeof vi.fn>).mockResolvedValue({ purchaseIds: [] });
    localStorage.removeItem('sellSheetIds');
  });

  it('initializes with empty set', async () => {
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.count).toBe(0);
    expect(result.current.has('abc')).toBe(false);
  });

  it('loads items from server', async () => {
    (api.getSellSheetItems as ReturnType<typeof vi.fn>).mockResolvedValue({ purchaseIds: ['id1', 'id2'] });
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.count).toBe(2);
    expect(result.current.has('id1')).toBe(true);
  });

  it('adds items optimistically', async () => {
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    act(() => result.current.add(['a', 'b']));
    // Optimistic update should be immediate
    await waitFor(() => expect(result.current.count).toBe(2));
    expect(result.current.has('a')).toBe(true);
    expect(api.addSellSheetItems).toHaveBeenCalledWith(['a', 'b']);
  });

  it('removes items optimistically', async () => {
    (api.getSellSheetItems as ReturnType<typeof vi.fn>).mockResolvedValue({ purchaseIds: ['a', 'b', 'c'] });
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.count).toBe(3));
    act(() => result.current.remove(['b']));
    await waitFor(() => expect(result.current.count).toBe(2));
    expect(result.current.has('b')).toBe(false);
    expect(api.removeSellSheetItems).toHaveBeenCalledWith(['b']);
  });

  it('clears all items optimistically', async () => {
    (api.getSellSheetItems as ReturnType<typeof vi.fn>).mockResolvedValue({ purchaseIds: ['a', 'b'] });
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.count).toBe(2));
    act(() => result.current.clear());
    await waitFor(() => expect(result.current.count).toBe(0));
    expect(api.clearSellSheetItems).toHaveBeenCalled();
  });

  it('migrates from localStorage when server is empty', async () => {
    localStorage.setItem('sellSheetIds', JSON.stringify(['legacy1', 'legacy2']));
    (api.addSellSheetItems as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
    renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(api.addSellSheetItems).toHaveBeenCalledWith(['legacy1', 'legacy2']));
    expect(localStorage.getItem('sellSheetIds')).toBeNull();
  });
});
```

- [ ] **Step 3: Run tests**

Run: `cd web && npm test -- --run src/react/hooks/useSellSheet.test.ts`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/react/hooks/useSellSheet.ts web/src/react/hooks/useSellSheet.test.ts
git commit -m "feat: rewrite useSellSheet hook to use server-backed React Query"
```

---

### Task 8: Print Layout — CSS and Column Hiding

**Files:**
- Modify: `web/src/styles/print-sell-sheet.css`
- Modify: `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`

- [ ] **Step 1: Add print-hide CSS rules**

In `web/src/styles/print-sell-sheet.css`, add inside the `@media print` block (before the closing `}`):

```css
  /* Hide analytics columns when printing */
  .print-hide-col {
    display: none !important;
  }

  /* Allow card name and subtitle to wrap instead of truncate */
  .glass-table-td .truncate {
    overflow: visible !important;
    white-space: normal !important;
    text-overflow: unset !important;
  }

  /* Hide actions column and checkbox when printing */
  .print-hide-actions {
    display: none !important;
  }
```

- [ ] **Step 2: Add print-hide-col class to DesktopRow analytics columns**

In `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`:

Add `print-hide-col` class to the **P/L column** (around line 145):
```tsx
<div className="glass-table-td flex-shrink-0 text-right tabular-nums print-hide-col" style={{ width: '72px' }}>
```

Add `print-hide-col` class to the **Days column** (around line 157):
```tsx
<div className={`glass-table-td flex-shrink-0 text-center print-hide-col ${daysColor}`} style={{ width: '40px' }}>{item.daysHeld}</div>
```

Add `print-hide-col` class to the **Signal column** (around line 159):
```tsx
<div className="glass-table-td flex-shrink-0 text-center print-hide-col" style={{ width: '48px' }}>
```

Add `print-hide-col` class to the **Status column** (around line 175):
```tsx
<div className="glass-table-td flex-shrink-0 text-center print-hide-col" style={{ width: '72px' }}>
```

Add `print-hide-actions` class to the **Actions column** (around line 197):
```tsx
<div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '28px' }}>
```

- [ ] **Step 3: Add print-hide-col to matching header columns in InventoryTab.tsx**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`, find the header row (around line 649-655). Add `print-hide-col` to the P/L, Days, Signal, and Status header divs:

For P/L SortableHeader (line 649): add `print-hide-col` to className:
```tsx
<SortableHeader label="P/L" sortKey="pl" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right print-hide-col" style={{ width: '72px' }} />
```

For Days SortableHeader (line 650):
```tsx
<SortableHeader label="Days" sortKey="days" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center print-hide-col" style={{ width: '40px' }} />
```

For Signal header (line 651):
```tsx
<div className="glass-table-th flex-shrink-0 text-center print-hide-col" style={{ width: '48px' }}>Signal</div>
```

For Status header (line 653):
```tsx
<div className="glass-table-th flex-shrink-0 text-center print-hide-col" style={{ width: '72px' }}>Status</div>
```

For the checkbox header (line 640-642) and trailing actions header (line 655), add `print-hide-actions`:
```tsx
<div className="glass-table-th flex-shrink-0 !px-1 print-hide-actions" style={{ width: '28px' }}>
```
(Apply to both the checkbox header and the empty trailing actions header.)

- [ ] **Step 4: Verify build**

Run: `cd web && npm run build`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add web/src/styles/print-sell-sheet.css web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat: hide analytics columns in print layout, unwrap truncated card names"
```

---

### Task 9: Mobile Sell Sheet Row Component

**Files:**
- Create: `web/src/react/pages/campaign-detail/inventory/MobileSellSheetRow.tsx`

- [ ] **Step 1: Create the compact row component**

Create `web/src/react/pages/campaign-detail/inventory/MobileSellSheetRow.tsx`:

```tsx
import type { AgingItem } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { bestPrice } from './utils';

interface MobileSellSheetRowProps {
  item: AgingItem;
  onTap: () => void;
}

export default function MobileSellSheetRow({ item, onTap }: MobileSellSheetRowProps) {
  const costBasis = item.purchase.buyCostCents + item.purchase.psaSourcingFeeCents;
  const snap = item.currentMarket;
  const market = snap ? bestPrice(snap) : 0;
  const clValue = item.purchase.clValueCents ?? 0;
  const recPrice = item.recommendedPriceCents ?? item.purchase.reviewedPriceCents ?? 0;
  const recProfitable = recPrice > 0 && recPrice > costBasis;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onTap}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onTap(); } }}
      className="grid items-center border-b border-[rgba(255,255,255,0.04)] active:bg-[rgba(255,255,255,0.04)] cursor-pointer transition-colors"
      style={{
        gridTemplateColumns: '1fr 24px 52px 52px 52px 56px',
        padding: '7px 10px',
        fontSize: '10px',
      }}
    >
      <div className="min-w-0">
        <div className="font-medium text-[var(--text)] truncate" style={{ fontSize: '10px' }}>
          {item.purchase.cardName}
        </div>
        <div className="text-[var(--text-muted)] truncate" style={{ fontSize: '8px' }}>
          {item.purchase.setName && <>{item.purchase.setName}</>}
          {item.purchase.cardNumber && <> &middot; #{item.purchase.cardNumber}</>}
          {item.purchase.certNumber && (
            <> &middot; <span className="font-mono text-[var(--text-secondary,#94a3b8)]">{item.purchase.certNumber}</span></>
          )}
        </div>
      </div>
      <span className="text-center text-[var(--text)]">
        {item.purchase.gradeValue % 1 === 0 ? item.purchase.gradeValue.toFixed(0) : item.purchase.gradeValue}
      </span>
      <span className="text-right tabular-nums text-[var(--text)]">{formatCents(costBasis)}</span>
      <span className="text-right tabular-nums text-[var(--text)]">
        {market > 0 ? formatCents(market) : <span className="text-[var(--text-muted)]">-</span>}
      </span>
      <span className="text-right tabular-nums text-[var(--text)]">
        {clValue > 0 ? formatCents(clValue) : <span className="text-[var(--text-muted)]">&mdash;</span>}
      </span>
      <span className={`text-right tabular-nums font-semibold ${
        recPrice > 0
          ? recProfitable ? 'text-[var(--success)]' : 'text-[var(--danger)]'
          : 'text-[var(--text-muted)]'
      }`}>
        {recPrice > 0 ? formatCents(recPrice) : <span className="italic">&mdash;</span>}
      </span>
    </div>
  );
}
```

- [ ] **Step 2: Verify build**

Run: `cd web && npm run build`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/MobileSellSheetRow.tsx
git commit -m "feat: add MobileSellSheetRow compact component"
```

---

### Task 10: Mobile Sell Sheet View Component

**Files:**
- Create: `web/src/react/pages/campaign-detail/inventory/MobileSellSheetView.tsx`

- [ ] **Step 1: Create the dedicated view component**

Create `web/src/react/pages/campaign-detail/inventory/MobileSellSheetView.tsx`:

```tsx
import { useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { AgingItem } from '../../../../types/campaigns';
import { Button } from '../../../ui';
import MobileSellSheetRow from './MobileSellSheetRow';

interface MobileSellSheetViewProps {
  items: AgingItem[];
  onRecordSale: (item: AgingItem) => void;
  onExit: () => void;
  searchQuery: string;
  onSearch: (query: string) => void;
  sellSheetCount: number;
  isPrinting: boolean;
  onPrint: () => void;
}

export default function MobileSellSheetView({
  items,
  onRecordSale,
  onExit,
  searchQuery,
  onSearch,
  sellSheetCount,
  isPrinting,
  onPrint,
}: MobileSellSheetViewProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 36,
    overscan: 10,
  });

  return (
    <div className="sell-sheet-no-print">
      {/* Compact header */}
      <div className="flex items-center justify-between px-3 py-2.5 bg-[var(--surface-1)] border-b border-[var(--surface-2)]">
        <div className="flex items-center gap-2">
          <span className="text-sm font-bold text-[var(--text)]">Sell Sheet</span>
          <span className="text-xs text-[var(--text-muted)]">{sellSheetCount} items</span>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="secondary" disabled={isPrinting} onClick={onPrint}>
            {isPrinting ? 'Preparing\u2026' : 'Print'}
          </Button>
          <button
            type="button"
            onClick={onExit}
            className="text-xs px-2 py-1 rounded border border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]"
          >
            Exit
          </button>
        </div>
      </div>

      {/* Search */}
      <div className="px-3 py-1.5 border-b border-[rgba(255,255,255,0.04)]">
        <input
          type="text"
          placeholder="Search sell sheet\u2026"
          value={searchQuery}
          onChange={(e) => onSearch(e.target.value)}
          className="w-full bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-md px-2 py-1.5 text-xs text-[var(--text)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-500)]"
        />
      </div>

      {/* Column headers */}
      <div
        className="grid items-center px-2.5 py-1.5 text-[var(--text-muted)] border-b-2 border-[var(--surface-2)]"
        style={{
          gridTemplateColumns: '1fr 24px 52px 52px 52px 56px',
          fontSize: '9px',
          textTransform: 'uppercase',
          letterSpacing: '0.5px',
        }}
      >
        <span>Card</span>
        <span className="text-center">Gr</span>
        <span className="text-right">Cost</span>
        <span className="text-right">Mkt</span>
        <span className="text-right">CL</span>
        <span className="text-right">Rec</span>
      </div>

      {/* Scrollable rows */}
      {items.length === 0 ? (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">
            Select items from any tab and tap &ldquo;Add to Sell Sheet&rdquo;.
          </div>
        </div>
      ) : (
        <div
          ref={scrollRef}
          className="overflow-y-auto scrollbar-dark overscroll-contain touch-pan-y"
          style={{ maxHeight: 'calc(100dvh - 130px)' }}
        >
          <div style={{ height: `${virtualizer.getTotalSize()}px`, position: 'relative' }}>
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const item = items[virtualRow.index];
              return (
                <div
                  key={item.purchase.id}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <MobileSellSheetRow
                    item={item}
                    onTap={() => onRecordSale(item)}
                  />
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify build**

Run: `cd web && npm run build`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/MobileSellSheetView.tsx
git commit -m "feat: add MobileSellSheetView dedicated mobile component"
```

---

### Task 11: Integrate Mobile Sell Sheet View into InventoryTab

**Files:**
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`

- [ ] **Step 1: Add import**

At the top of `InventoryTab.tsx`, add after the existing inventory component imports (around line 28):

```tsx
import MobileSellSheetView from './inventory/MobileSellSheetView';
```

- [ ] **Step 2: Add dedicated mobile sell sheet rendering**

Find the line `{isMobile ? (` (around line 579). Replace the entire mobile/desktop conditional block with a three-way conditional:

Before the existing `{isMobile ? (` block (line 579), add the mobile sell sheet view:

```tsx
{isMobile && sellSheetActive ? (
  <MobileSellSheetView
    items={filteredAndSortedItems}
    onRecordSale={(item) => openSaleModal([item])}
    onExit={() => setFilterTab('needs_review')}
    searchQuery={searchQuery}
    onSearch={setSearchQuery}
    sellSheetCount={pageSellSheetCount}
    isPrinting={isPrinting}
    onPrint={() => {
      setIsPrinting(true);
      requestAnimationFrame(() => {
        window.print();
        setIsPrinting(false);
      });
    }}
  />
) : isMobile ? (
```

This creates the three-way: mobile sell sheet → mobile normal → desktop.

- [ ] **Step 3: Hide chrome when mobile sell sheet is active**

The stats bar, selected items bar, review summary bar, crack candidates banner, filter tabs, and sell sheet empty state should all be hidden when `isMobile && sellSheetActive`. The simplest approach: wrap the entire block from the stats bar (line 343) to just before the rendering conditional (line 578) in a condition:

Find the opening `{/* Summary stat cards` comment (around line 343). Before it, add:

```tsx
{!(isMobile && sellSheetActive) && (
<>
```

Find the sell sheet empty state block ending (around line 577, before `{isMobile ? (`). After it, add:

```tsx
</>
)}
```

This hides stats, selected items bar, print button, crack candidates, review summary, filter tabs, and empty state when the mobile sell sheet view is showing (it has its own empty state).

- [ ] **Step 4: Verify build**

Run: `cd web && npm run build`
Expected: Success

- [ ] **Step 5: Run frontend tests**

Run: `cd web && npm test -- --run`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat: integrate dedicated mobile sell sheet view in InventoryTab"
```

---

### Task 12: Update API Documentation

**Files:**
- Modify: `docs/API.md`

- [ ] **Step 1: Add sell sheet items endpoints**

Find the sell sheet section in `docs/API.md` and add the new endpoints:

```markdown
### GET /api/sell-sheet/items

Get the authenticated user's sell sheet item IDs.

**Auth:** RequireAuth

**Response:** `200 OK`
```json
{
  "purchaseIds": ["uuid1", "uuid2"]
}
```

### PUT /api/sell-sheet/items

Add purchase IDs to the sell sheet (idempotent).

**Auth:** RequireAuth

**Request body:**
```json
{
  "purchaseIds": ["uuid1", "uuid2"]
}
```

**Response:** `204 No Content`

### DELETE /api/sell-sheet/items

Remove specific purchase IDs from the sell sheet.

**Auth:** RequireAuth

**Request body:**
```json
{
  "purchaseIds": ["uuid1", "uuid2"]
}
```

**Response:** `204 No Content`

### DELETE /api/sell-sheet/items/all

Clear all items from the sell sheet.

**Auth:** RequireAuth

**Response:** `204 No Content`
```

- [ ] **Step 2: Commit**

```bash
git add docs/API.md
git commit -m "docs: add sell sheet items persistence endpoints to API reference"
```

---

### Task 13: Final Verification

- [ ] **Step 1: Run full Go test suite**

Run: `go test -race -timeout 5m ./...`
Expected: All PASS

- [ ] **Step 2: Run full frontend test suite**

Run: `cd web && npm test -- --run`
Expected: All PASS

- [ ] **Step 3: Run quality checks**

Run: `make check`
Expected: PASS (lint, architecture import check, file size check)

- [ ] **Step 4: Build frontend**

Run: `cd web && npm run build`
Expected: Success

- [ ] **Step 5: Build backend**

Run: `go build -o slabledger ./cmd/slabledger`
Expected: Success
