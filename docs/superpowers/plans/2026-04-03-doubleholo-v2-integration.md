# DoubleHolo v2 API Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate DoubleHolo Enterprise API v2 for cert resolution, inventory management, and automated sales recording — eliminating the intermediate Card Ladder listing tool from the inventory-to-sale pipeline.

**Architecture:** Extends the existing DH client (`internal/adapters/clients/dh/`) with new enterprise API methods (cert resolution sync/async, inventory CRUD, orders feed). Two new schedulers poll DH for inventory status and completed sales. The PSA import flow is enhanced to batch-resolve certs via async job submission. Domain types gain DH tracking fields via a new migration.

**Tech Stack:** Go 1.26, SQLite (WAL), `golang.org/x/time/rate`, `httpx` unified HTTP client, table-driven tests with `testutil/mocks`.

**Spec:** `docs/superpowers/specs/2026-04-03-doubleholo-v2-workflow-design.md`

---

## File Structure

### New Files
- `internal/adapters/storage/sqlite/migrations/000030_doubleholo_v2.up.sql` — Schema changes
- `internal/adapters/storage/sqlite/migrations/000030_doubleholo_v2.down.sql` — Rollback
- `internal/adapters/clients/dh/types_v2.go` — Request/response types for v2 endpoints
- `internal/adapters/clients/dh/inventory.go` — Inventory and orders client methods
- `internal/adapters/clients/dh/certs.go` — Cert resolution client methods
- `internal/adapters/clients/dh/certs_test.go` — Tests for cert resolution
- `internal/adapters/clients/dh/inventory_test.go` — Tests for inventory/orders
- `internal/adapters/scheduler/dh_inventory_poll.go` — Inventory status poll scheduler
- `internal/adapters/scheduler/dh_inventory_poll_test.go` — Tests
- `internal/adapters/scheduler/dh_orders_poll.go` — Orders poll scheduler
- `internal/adapters/scheduler/dh_orders_poll_test.go` — Tests

### Modified Files
- `internal/domain/campaigns/types.go` — Add DH fields to Purchase, OrderID to Sale, new SaleChannel constant
- `internal/domain/campaigns/import_types.go` — Add OrderID to OrdersConfirmItem
- `internal/domain/campaigns/service_import_orders.go` — Support OrderID in ConfirmOrdersSales
- `internal/domain/campaigns/repository.go` — Add UpdatePurchaseDHFields, GetPurchasesByDHCertStatus methods
- `internal/adapters/storage/sqlite/purchases_repository.go` — Implement new repo methods
- `internal/adapters/storage/sqlite/purchase_scan_helpers.go` — Add new columns to scan lists
- `internal/adapters/storage/sqlite/sales_repository.go` — Add order_id column to CreateSale/scan
- `internal/adapters/scheduler/builder.go` — Wire new schedulers into BuildDeps/BuildGroup
- `internal/platform/config/types.go` — Add DHv2Config fields (poll intervals)

---

## Task 1: Database Migration — Add DH v2 Fields

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000030_doubleholo_v2.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000030_doubleholo_v2.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- Add DH v2 tracking fields to purchases
ALTER TABLE campaign_purchases ADD COLUMN dh_card_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN dh_inventory_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN dh_cert_status TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN dh_listing_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN dh_channels_json TEXT NOT NULL DEFAULT '';

-- Index for querying by DH cert resolution status (reconciliation, push-to-DH)
CREATE INDEX idx_purchases_dh_cert_status ON campaign_purchases(dh_cert_status)
    WHERE dh_cert_status != '';

-- Add order_id to sales for DH order poll idempotency
ALTER TABLE campaign_sales ADD COLUMN order_id TEXT NOT NULL DEFAULT '';

-- Unique constraint on order_id (only for non-empty values, allows multiple empty)
CREATE UNIQUE INDEX idx_sales_order_id ON campaign_sales(order_id)
    WHERE order_id != '';
```

- [ ] **Step 2: Write the down migration**

```sql
-- SQLite does not support DROP COLUMN in older versions, but Go 1.26 uses
-- modern SQLite which does. These are safe.
DROP INDEX IF EXISTS idx_sales_order_id;
ALTER TABLE campaign_sales DROP COLUMN order_id;

DROP INDEX IF EXISTS idx_purchases_dh_cert_status;
ALTER TABLE campaign_purchases DROP COLUMN dh_channels_json;
ALTER TABLE campaign_purchases DROP COLUMN dh_listing_price_cents;
ALTER TABLE campaign_purchases DROP COLUMN dh_cert_status;
ALTER TABLE campaign_purchases DROP COLUMN dh_inventory_id;
ALTER TABLE campaign_purchases DROP COLUMN dh_card_id;
```

- [ ] **Step 3: Verify migration applies cleanly**

Run: `go test ./internal/adapters/storage/sqlite/... -run TestMigrations -count=1 -v`

If no migration test exists, verify manually:

```bash
rm -f /tmp/test_migrate.db
go run ./cmd/slabledger -db /tmp/test_migrate.db &
sleep 2
kill %1
```

Expected: Server starts without migration errors.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/000030_doubleholo_v2.up.sql \
        internal/adapters/storage/sqlite/migrations/000030_doubleholo_v2.down.sql
git commit -m "feat: add migration 000030 for DH v2 fields (purchases + sales)"
```

---

## Task 2: Domain Type Changes — Purchase, Sale, SaleChannel

**Files:**
- Modify: `internal/domain/campaigns/types.go`
- Modify: `internal/domain/campaigns/import_types.go`

- [ ] **Step 1: Add DH fields to Purchase struct**

In `internal/domain/campaigns/types.go`, add these fields to the `Purchase` struct after the `ReviewSource` field (before `CreatedAt`):

```go
	// DoubleHolo v2 integration fields
	DHCardID           int    `json:"dhCardId,omitempty"`           // DH card identity (from cert resolution)
	DHInventoryID      int    `json:"dhInventoryId,omitempty"`      // DH inventory item ID (from inventory push)
	DHCertStatus       string `json:"dhCertStatus,omitempty"`       // Resolution state: matched, ambiguous, not_found, unresolved, resolving
	DHListingPriceCents int   `json:"dhListingPriceCents,omitempty"` // Current DH listing price
	DHChannelsJSON     string `json:"dhChannelsJson,omitempty"`     // Per-channel sync status JSON blob
```

- [ ] **Step 2: Add OrderID to Sale struct**

In `internal/domain/campaigns/types.go`, add this field to the `Sale` struct after `UpdatedAt`:

```go
	// DoubleHolo v2 order tracking
	OrderID string `json:"orderId,omitempty"` // DH order_id for idempotency
```

- [ ] **Step 3: Add SaleChannelDoubleHolo constant**

In `internal/domain/campaigns/types.go`, add to the `SaleChannel` constants block:

```go
	SaleChannelDoubleHolo SaleChannel = "doubleholo"
```

- [ ] **Step 4: Add OrderID to OrdersConfirmItem**

In `internal/domain/campaigns/import_types.go`, add to `OrdersConfirmItem`:

```go
	OrderID        string      `json:"orderId,omitempty"`
```

- [ ] **Step 5: Run tests to verify no regressions**

Run: `go test ./internal/domain/campaigns/... -count=1 -v`
Expected: All existing tests pass (new fields have zero values by default).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/types.go internal/domain/campaigns/import_types.go
git commit -m "feat: add DH v2 fields to Purchase, Sale, and OrdersConfirmItem"
```

---

## Task 3: Repository Layer — Scan Helpers and CRUD Updates

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchase_scan_helpers.go`
- Modify: `internal/adapters/storage/sqlite/purchases_repository.go`
- Modify: `internal/adapters/storage/sqlite/sales_repository.go`

- [ ] **Step 1: Add purchase columns to scan helpers**

In `internal/adapters/storage/sqlite/purchase_scan_helpers.go`, append these 5 columns to the end of `purchaseColumns` (before the closing backtick):

```
dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json
```

And the same with `p.` prefix to `purchaseColumnsAliased`.

In the `purchaseScanDests` function, append these destinations:

```go
	&p.DHCardID, &p.DHInventoryID, &p.DHCertStatus, &p.DHListingPriceCents, &p.DHChannelsJSON,
```

- [ ] **Step 2: Add purchase columns to CreatePurchase**

In `internal/adapters/storage/sqlite/purchases_repository.go`, in the `CreatePurchase` method, add the 5 new columns to the INSERT statement and their corresponding values:

Add to column list: `dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json`

Add to values: `p.DHCardID, p.DHInventoryID, p.DHCertStatus, p.DHListingPriceCents, p.DHChannelsJSON`

Add 5 more `?` placeholders to the VALUES clause.

- [ ] **Step 3: Add UpdatePurchaseDHFields method**

In `internal/adapters/storage/sqlite/purchases_repository.go`, add:

```go
// UpdatePurchaseDHFields updates DH v2 tracking fields on a purchase.
func (r *PurchasesRepository) UpdatePurchaseDHFields(ctx context.Context, id string, cardID, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, updated_at = ?
		 WHERE id = ?`,
		cardID, inventoryID, certStatus, listingPriceCents, channelsJSON, time.Now(), id,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}
```

- [ ] **Step 4: Add GetPurchasesByDHCertStatus method**

In `internal/adapters/storage/sqlite/purchases_repository.go`, add:

```go
// GetPurchasesByDHCertStatus returns purchases with the given DH cert resolution status.
func (r *PurchasesRepository) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases WHERE dh_cert_status = ? ORDER BY updated_at ASC LIMIT ?`,
		purchaseColumns,
	)
	rows, err := r.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var purchases []campaigns.Purchase
	for rows.Next() {
		var p campaigns.Purchase
		if err := scanPurchase(rows, &p); err != nil {
			return nil, err
		}
		purchases = append(purchases, p)
	}
	return purchases, rows.Err()
}
```

- [ ] **Step 5: Add order_id to sales repository**

In `internal/adapters/storage/sqlite/sales_repository.go`:

1. Add `order_id` to the end of `saleColumns`.
2. Add `&s.OrderID` to the end of the `scanSale` function.
3. Add `s.OrderID` to the end of the `CreateSale` INSERT values, and add the column name and an extra `?` placeholder.

- [ ] **Step 6: Add UpdatePurchaseDHFields and GetPurchasesByDHCertStatus to Repository interface**

In `internal/domain/campaigns/repository.go`, add to `PurchaseRepository`:

```go
	UpdatePurchaseDHFields(ctx context.Context, id string, cardID, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error
	GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]Purchase, error)
```

- [ ] **Step 7: Add to mock repository**

In `internal/testutil/mocks/campaign_repository.go`, add:

```go
	UpdatePurchaseDHFieldsFn      func(ctx context.Context, id string, cardID, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error
	GetPurchasesByDHCertStatusFn  func(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error)
```

And implement the corresponding methods with the standard Fn-field pattern (check Fn != nil, else return sensible default).

- [ ] **Step 8: Run all tests**

Run: `go test ./internal/adapters/storage/sqlite/... ./internal/domain/campaigns/... -count=1 -v`
Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/adapters/storage/sqlite/purchase_scan_helpers.go \
        internal/adapters/storage/sqlite/purchases_repository.go \
        internal/adapters/storage/sqlite/sales_repository.go \
        internal/domain/campaigns/repository.go \
        internal/testutil/mocks/campaign_repository.go
git commit -m "feat: add DH v2 repository methods and scan helpers"
```

---

## Task 4: DH Client — v2 Types

**Files:**
- Create: `internal/adapters/clients/dh/types_v2.go`

- [ ] **Step 1: Write v2 request/response types**

```go
package dh

// --- Cert Resolution Types ---

// CertResolveRequest is a single cert to resolve.
type CertResolveRequest struct {
	CertNumber string `json:"cert_number"`
	CardName   string `json:"card_name,omitempty"`
	SetName    string `json:"set_name,omitempty"`
	CardNumber string `json:"card_number,omitempty"`
	Year       string `json:"year,omitempty"`
	Variant    string `json:"variant,omitempty"`
	Language   string `json:"language,omitempty"`
}

// CertResolution is the result of resolving a single cert.
type CertResolution struct {
	CertNumber           string                `json:"cert_number"`
	Status               string                `json:"status"` // "matched", "ambiguous", "not_found"
	DHCardID             int                   `json:"dh_card_id,omitempty"`
	CardName             string                `json:"card_name,omitempty"`
	SetName              string                `json:"set_name,omitempty"`
	CardNumber           string                `json:"card_number,omitempty"`
	Grade                float64               `json:"grade,omitempty"`
	ImageURL             string                `json:"image_url,omitempty"`
	CurrentMarketPriceCents int               `json:"current_market_price_cents,omitempty"`
	Candidates           []CertResolutionCandidate `json:"candidates,omitempty"`
}

// CertResolutionCandidate is one possible match for an ambiguous cert.
type CertResolutionCandidate struct {
	DHCardID   int    `json:"dh_card_id"`
	CardName   string `json:"card_name"`
	SetName    string `json:"set_name"`
	CardNumber string `json:"card_number"`
	ImageURL   string `json:"image_url"`
}

// CertResolveBatchRequest is the request body for POST /enterprise/certs/resolve_batch.
type CertResolveBatchRequest struct {
	Certs []CertResolveRequest `json:"certs"`
}

// CertResolveBatchResponse is the response from POST /enterprise/certs/resolve_batch.
type CertResolveBatchResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"` // "queued"
	TotalCerts int    `json:"total_certs"`
}

// CertResolutionJobStatus is the response from GET /enterprise/certs/resolve_batch/:job_id.
type CertResolutionJobStatus struct {
	JobID         string           `json:"job_id"`
	Status        string           `json:"status"` // "queued", "processing", "completed", "failed"
	TotalCerts    int              `json:"total_certs"`
	ResolvedCount int              `json:"resolved_count"`
	Results       []CertResolution `json:"results,omitempty"`
}

// --- Inventory Types ---

// InventoryItem is a single item to push to DH inventory.
type InventoryItem struct {
	DHCardID       int    `json:"dh_card_id"`
	CertNumber     string `json:"cert_number"`
	GradingCompany string `json:"grading_company"`
	Grade          float64 `json:"grade"`
	CostBasisCents int    `json:"cost_basis_cents"`
}

// InventoryPushRequest is the request body for POST /inventory.
type InventoryPushRequest struct {
	Items []InventoryItem `json:"items"`
}

// InventoryChannelStatus is the per-channel sync status.
type InventoryChannelStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pending", "active", "error"
}

// InventoryResult is the per-item response from inventory push.
type InventoryResult struct {
	DHInventoryID     int                      `json:"dh_inventory_id"`
	CertNumber        string                   `json:"cert_number"`
	Status            string                   `json:"status"` // "active", "pending", "failed"
	AssignedPriceCents int                     `json:"assigned_price_cents"`
	Channels          []InventoryChannelStatus `json:"channels,omitempty"`
	Error             string                   `json:"error,omitempty"`
}

// InventoryPushResponse is the response from POST /inventory.
type InventoryPushResponse struct {
	Results []InventoryResult `json:"results"`
}

// InventoryListItem is a single item in the inventory list response.
type InventoryListItem struct {
	DHInventoryID     int                      `json:"dh_inventory_id"`
	DHCardID          int                      `json:"dh_card_id"`
	CertNumber        string                   `json:"cert_number"`
	CardName          string                   `json:"card_name"`
	SetName           string                   `json:"set_name"`
	CardNumber        string                   `json:"card_number"`
	GradingCompany    string                   `json:"grading_company"`
	Grade             float64                  `json:"grade"`
	Status            string                   `json:"status"`
	ListingPriceCents int                      `json:"listing_price_cents"`
	CostBasisCents    int                      `json:"cost_basis_cents"`
	Channels          []InventoryChannelStatus `json:"channels,omitempty"`
	CreatedAt         string                   `json:"created_at"`
	UpdatedAt         string                   `json:"updated_at"`
}

// InventoryListResponse is the response from GET /inventory.
type InventoryListResponse struct {
	Items []InventoryListItem `json:"items"`
	Meta  PaginationMeta      `json:"meta"`
}

// PaginationMeta holds pagination metadata.
type PaginationMeta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalCount int `json:"total_count"`
}

// InventoryUpdate is the request body for PATCH /inventory/:id.
type InventoryUpdate struct {
	CostBasisCents int `json:"cost_basis_cents"`
}

// --- Orders Types ---

// Order is a single completed sale from DH.
type Order struct {
	OrderID        string    `json:"order_id"`
	CertNumber     string    `json:"cert_number"`
	DHCardID       int       `json:"dh_card_id"`
	CardName       string    `json:"card_name"`
	SetName        string    `json:"set_name"`
	Grade          float64   `json:"grade"`
	SalePriceCents int       `json:"sale_price_cents"`
	Channel        string    `json:"channel"` // "dh", "ebay", "shopify"
	Fees           OrderFees `json:"fees"`
	NetAmountCents *int      `json:"net_amount_cents"` // nullable — only when all fees known
	SoldAt         string    `json:"sold_at"`           // ISO 8601
}

// OrderFees is the fee breakdown for an order.
type OrderFees struct {
	ChannelFeeCents *int `json:"channel_fee_cents"` // nullable
	CommissionCents *int `json:"commission_cents"`   // nullable
}

// OrdersResponse is the response from GET /orders.
type OrdersResponse struct {
	Orders []Order        `json:"orders"`
	Meta   PaginationMeta `json:"meta"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/adapters/clients/dh/...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/clients/dh/types_v2.go
git commit -m "feat: add DH v2 request/response types"
```

---

## Task 5: DH Client — Cert Resolution Methods

**Files:**
- Create: `internal/adapters/clients/dh/certs.go`
- Create: `internal/adapters/clients/dh/certs_test.go`

- [ ] **Step 1: Write the test file**

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

func TestClient_ResolveCert(t *testing.T) {
	expected := CertResolution{
		CertNumber: "12345678",
		Status:     "matched",
		DHCardID:   51942,
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4/102",
		Grade:      9,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/resolve", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req CertResolveRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "12345678", req.CertNumber)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	result, err := client.ResolveCert(context.Background(), CertResolveRequest{CertNumber: "12345678"})

	require.NoError(t, err)
	require.Equal(t, "matched", result.Status)
	require.Equal(t, 51942, result.DHCardID)
	require.Equal(t, "Charizard", result.CardName)
}

func TestClient_ResolveCertsBatch(t *testing.T) {
	expected := CertResolveBatchResponse{
		JobID:      "job_abc123",
		Status:     "queued",
		TotalCerts: 2,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/resolve_batch", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req CertResolveBatchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Certs, 2)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	job, err := client.ResolveCertsBatch(context.Background(), []CertResolveRequest{
		{CertNumber: "11111111"},
		{CertNumber: "22222222"},
	})

	require.NoError(t, err)
	require.Equal(t, "job_abc123", job.JobID)
	require.Equal(t, "queued", job.Status)
	require.Equal(t, 2, job.TotalCerts)
}

func TestClient_GetCertResolutionJob(t *testing.T) {
	expected := CertResolutionJobStatus{
		JobID:         "job_abc123",
		Status:        "completed",
		TotalCerts:    2,
		ResolvedCount: 2,
		Results: []CertResolution{
			{CertNumber: "11111111", Status: "matched", DHCardID: 100},
			{CertNumber: "22222222", Status: "not_found"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/resolve_batch/job_abc123", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	status, err := client.GetCertResolutionJob(context.Background(), "job_abc123")

	require.NoError(t, err)
	require.Equal(t, "completed", status.Status)
	require.Equal(t, 2, status.ResolvedCount)
	require.Len(t, status.Results, 2)
	require.Equal(t, "matched", status.Results[0].Status)
	require.Equal(t, "not_found", status.Results[1].Status)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/adapters/clients/dh/... -run TestClient_Resolve -count=1 -v`
Expected: FAIL — methods don't exist yet.

- [ ] **Step 3: Write the implementation**

```go
package dh

import (
	"context"
	"fmt"
	"net/url"
)

// ResolveCert resolves a single PSA cert synchronously via the enterprise API.
func (c *Client) ResolveCert(ctx context.Context, req CertResolveRequest) (*CertResolution, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve", c.baseURL)

	var resp CertResolution
	if err := c.postEnterprise(ctx, fullURL, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ResolveCertsBatch submits up to 500 certs for async resolution. Returns a job ID.
func (c *Client) ResolveCertsBatch(ctx context.Context, certs []CertResolveRequest) (*CertResolveBatchResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve_batch", c.baseURL)
	body := CertResolveBatchRequest{Certs: certs}

	var resp CertResolveBatchResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCertResolutionJob polls for the status and results of a batch cert resolution job.
func (c *Client) GetCertResolutionJob(ctx context.Context, jobID string) (*CertResolutionJobStatus, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve_batch/%s", c.baseURL, url.PathEscape(jobID))

	var resp CertResolutionJobStatus
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 4: Add enterprise HTTP helpers to client.go**

In `internal/adapters/clients/dh/client.go`, add these two methods. They are identical to `get` and `post` but use Bearer auth instead of the integration API key header:

```go
const enterpriseAuthHeader = "Authorization"

// getEnterprise performs a GET request with Bearer auth for the enterprise API.
func (c *Client) getEnterprise(ctx context.Context, fullURL string, dest any) error {
	if !c.Available() {
		return apperrors.ConfigMissing("dh_api_key", "DH_INTEGRATION_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.apiKey,
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Get(ctx, fullURL, headers, c.timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	return nil
}

// postEnterprise performs a POST request with Bearer auth for the enterprise API.
func (c *Client) postEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	if !c.Available() {
		return apperrors.ConfigMissing("dh_api_key", "DH_INTEGRATION_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return apperrors.ProviderInvalidRequest(providerName, err)
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.apiKey,
		"Content-Type":       "application/json",
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Post(ctx, fullURL, headers, bodyBytes, c.timeout)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return apperrors.ProviderInvalidResponse(providerName, err)
	}
	return nil
}
```

- [ ] **Step 5: Update newTestClient to use Bearer auth**

In `internal/adapters/clients/dh/client_test.go`, verify that `newTestClient` returns a client with an API key set (e.g., `"test-key"`). The existing `newTestClient` already does this — the test server assertions in Step 1 validate the `"Bearer test-key"` header.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/clients/dh/... -run TestClient_Resolve -count=1 -v`
Expected: All 3 tests PASS.

- [ ] **Step 7: Run full client test suite for regressions**

Run: `go test ./internal/adapters/clients/dh/... -count=1 -v`
Expected: All tests pass (existing integration API tests still use `get`/`post` with X-Integration-API-Key).

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/clients/dh/certs.go \
        internal/adapters/clients/dh/certs_test.go \
        internal/adapters/clients/dh/client.go
git commit -m "feat: add DH cert resolution client methods (sync + async batch)"
```

---

## Task 6: DH Client — Inventory and Orders Methods

**Files:**
- Create: `internal/adapters/clients/dh/inventory.go`
- Create: `internal/adapters/clients/dh/inventory_test.go`

- [ ] **Step 1: Write the test file**

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

func TestClient_PushInventory(t *testing.T) {
	expected := InventoryPushResponse{
		Results: []InventoryResult{
			{
				DHInventoryID:      98765,
				CertNumber:         "12345678",
				Status:             "active",
				AssignedPriceCents: 7500,
				Channels: []InventoryChannelStatus{
					{Name: "ebay", Status: "active"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory", r.URL.Path)

		var req InventoryPushRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Items, 1)
		require.Equal(t, 51942, req.Items[0].DHCardID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.PushInventory(context.Background(), []InventoryItem{
		{DHCardID: 51942, CertNumber: "12345678", GradingCompany: "psa", Grade: 9.0, CostBasisCents: 5000},
	})

	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	require.Equal(t, 98765, resp.Results[0].DHInventoryID)
	require.Equal(t, "active", resp.Results[0].Status)
}

func TestClient_ListInventory(t *testing.T) {
	expected := InventoryListResponse{
		Items: []InventoryListItem{
			{DHInventoryID: 98765, CertNumber: "12345678", Status: "active", ListingPriceCents: 7500},
		},
		Meta: PaginationMeta{Page: 1, PerPage: 25, TotalCount: 1},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.URL.Path, "/api/v1/enterprise/inventory")
		require.Equal(t, "active", r.URL.Query().Get("status"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.ListInventory(context.Background(), InventoryFilters{Status: "active", Page: 1, PerPage: 25})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, 98765, resp.Items[0].DHInventoryID)
}

func TestClient_GetOrders(t *testing.T) {
	expected := OrdersResponse{
		Orders: []Order{
			{
				OrderID:        "dh-12345",
				CertNumber:     "12345678",
				SalePriceCents: 7500,
				Channel:        "ebay",
				SoldAt:         "2026-04-02T14:30:00Z",
				Fees: OrderFees{
					ChannelFeeCents: intPtr(994),
				},
				NetAmountCents: intPtr(6506),
			},
		},
		Meta: PaginationMeta{Page: 1, PerPage: 25, TotalCount: 1},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.URL.Path, "/api/v1/enterprise/orders")
		require.NotEmpty(t, r.URL.Query().Get("since"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, err := client.GetOrders(context.Background(), OrderFilters{Since: "2026-04-01T00:00:00Z", Page: 1, PerPage: 25})

	require.NoError(t, err)
	require.Len(t, resp.Orders, 1)
	require.Equal(t, "dh-12345", resp.Orders[0].OrderID)
	require.Equal(t, 7500, resp.Orders[0].SalePriceCents)
	require.Equal(t, intPtr(994), resp.Orders[0].Fees.ChannelFeeCents)
}

func intPtr(v int) *int { return &v }
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/adapters/clients/dh/... -run "TestClient_Push|TestClient_List|TestClient_GetOrders" -count=1 -v`
Expected: FAIL — methods don't exist yet.

- [ ] **Step 3: Write the implementation**

```go
package dh

import (
	"context"
	"fmt"
	"net/url"
)

// InventoryFilters are query parameters for GET /inventory.
type InventoryFilters struct {
	Status       string
	CertNumber   string
	UpdatedSince string
	Page         int
	PerPage      int
}

// OrderFilters are query parameters for GET /orders.
type OrderFilters struct {
	Since   string // ISO 8601, required
	Channel string
	Page    int
	PerPage int
}

// PushInventory creates or updates inventory items on DH (upsert semantics).
func (c *Client) PushInventory(ctx context.Context, items []InventoryItem) (*InventoryPushResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory", c.baseURL)
	body := InventoryPushRequest{Items: items}

	var resp InventoryPushResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListInventory retrieves current inventory with optional filters.
func (c *Client) ListInventory(ctx context.Context, filters InventoryFilters) (*InventoryListResponse, error) {
	params := url.Values{}
	if filters.Status != "" {
		params.Set("status", filters.Status)
	}
	if filters.CertNumber != "" {
		params.Set("cert_number", filters.CertNumber)
	}
	if filters.UpdatedSince != "" {
		params.Set("updated_since", filters.UpdatedSince)
	}
	if filters.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", filters.Page))
	}
	if filters.PerPage > 0 {
		params.Set("per_page", fmt.Sprintf("%d", filters.PerPage))
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory?%s", c.baseURL, params.Encode())

	var resp InventoryListResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateInventory updates the cost basis for an inventory item.
func (c *Client) UpdateInventory(ctx context.Context, inventoryID int, update InventoryUpdate) error {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d", c.baseURL, inventoryID)

	var resp json.RawMessage
	if err := c.patchEnterprise(ctx, fullURL, update, &resp); err != nil {
		return err
	}
	return nil
}

// DelistInventory removes an item from all channels.
func (c *Client) DelistInventory(ctx context.Context, inventoryID int) error {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d", c.baseURL, inventoryID)
	return c.deleteEnterprise(ctx, fullURL)
}

// GetOrders retrieves completed sales from DH.
func (c *Client) GetOrders(ctx context.Context, filters OrderFilters) (*OrdersResponse, error) {
	params := url.Values{}
	params.Set("since", filters.Since)
	if filters.Channel != "" {
		params.Set("channel", filters.Channel)
	}
	if filters.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", filters.Page))
	}
	if filters.PerPage > 0 {
		params.Set("per_page", fmt.Sprintf("%d", filters.PerPage))
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/orders?%s", c.baseURL, params.Encode())

	var resp OrdersResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 4: Add PATCH and DELETE enterprise helpers to client.go**

In `internal/adapters/clients/dh/client.go`, add:

```go
// patchEnterprise performs a PATCH request with Bearer auth for the enterprise API.
func (c *Client) patchEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	if !c.Available() {
		return apperrors.ConfigMissing("dh_api_key", "DH_INTEGRATION_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return apperrors.ProviderInvalidRequest(providerName, err)
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.apiKey,
		"Content-Type":       "application/json",
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Patch(ctx, fullURL, headers, bodyBytes, c.timeout)
	if err != nil {
		return err
	}

	if dest != nil {
		if err := json.Unmarshal(resp.Body, dest); err != nil {
			return apperrors.ProviderInvalidResponse(providerName, err)
		}
	}
	return nil
}

// deleteEnterprise performs a DELETE request with Bearer auth for the enterprise API.
func (c *Client) deleteEnterprise(ctx context.Context, fullURL string) error {
	if !c.Available() {
		return apperrors.ConfigMissing("dh_api_key", "DH_INTEGRATION_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.apiKey,
		"Accept":             "application/json",
	}

	_, err := c.httpClient.Delete(ctx, fullURL, headers, c.timeout)
	return err
}
```

**Important:** Check if `httpx.Client` has `Patch` and `Delete` methods before implementing. If not, implement these helpers using Go's standard `net/http` package directly — create a `*http.Request` with the appropriate method, apply rate limiting and Bearer auth the same way as `getEnterprise`/`postEnterprise`, and use `c.httpClient`'s underlying transport or `http.DefaultClient`. The `UpdateInventory` and `DelistInventory` methods are lower priority (not used by schedulers), so this can be deferred if `httpx` doesn't support these verbs.

- [ ] **Step 5: Add `encoding/json` import to inventory.go**

Add to the import block in `inventory.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/clients/dh/... -count=1 -v`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/clients/dh/inventory.go \
        internal/adapters/clients/dh/inventory_test.go \
        internal/adapters/clients/dh/client.go
git commit -m "feat: add DH inventory and orders client methods"
```

---

## Task 7: Orders Poll Scheduler

**Files:**
- Create: `internal/adapters/scheduler/dh_orders_poll.go`
- Create: `internal/adapters/scheduler/dh_orders_poll_test.go`

- [ ] **Step 1: Write the test file**

```go
package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

type mockDHOrdersClient struct {
	orders    *dh.OrdersResponse
	err       error
	callCount atomic.Int32
}

func (m *mockDHOrdersClient) GetOrders(_ context.Context, _ dh.OrderFilters) (*dh.OrdersResponse, error) {
	m.callCount.Add(1)
	return m.orders, m.err
}

func TestDHOrdersPoll_NoOrders(t *testing.T) {
	client := &mockDHOrdersClient{
		orders: &dh.OrdersResponse{Orders: []dh.Order{}},
	}
	syncState := newMockSyncStateStore()
	campaignSvc := &mocks.MockCampaignService{}

	s := NewDHOrdersPollScheduler(client, syncState, campaignSvc, mocks.NewMockLogger(), DHOrdersPollConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	s.poll(context.Background())
	require.Equal(t, int32(1), client.callCount.Load())
}

func TestDHOrdersPoll_RecordsSale(t *testing.T) {
	feeCents := 994
	netCents := 6506
	client := &mockDHOrdersClient{
		orders: &dh.OrdersResponse{
			Orders: []dh.Order{
				{
					OrderID:        "dh-12345",
					CertNumber:     "99998888",
					SalePriceCents: 7500,
					Channel:        "ebay",
					SoldAt:         "2026-04-02T14:30:00Z",
					Fees:           dh.OrderFees{ChannelFeeCents: &feeCents},
					NetAmountCents: &netCents,
				},
			},
		},
	}
	syncState := newMockSyncStateStore()

	var importedRows []campaigns.OrdersExportRow
	var confirmedItems []campaigns.OrdersConfirmItem
	campaignSvc := &mocks.MockCampaignService{
		ImportOrdersSalesFn: func(_ context.Context, rows []campaigns.OrdersExportRow) (*campaigns.OrdersImportResult, error) {
			importedRows = rows
			return &campaigns.OrdersImportResult{
				Matched: []campaigns.OrdersImportMatch{
					{
						CertNumber:     "99998888",
						SaleChannel:    campaigns.SaleChannelEbay,
						SaleDate:       "2026-04-02",
						SalePriceCents: 7500,
						SaleFeeCents:   994,
						PurchaseID:     "purchase-1",
					},
				},
			}, nil
		},
		ConfirmOrdersSalesFn: func(_ context.Context, items []campaigns.OrdersConfirmItem) (*campaigns.BulkSaleResult, error) {
			confirmedItems = items
			return &campaigns.BulkSaleResult{Created: 1}, nil
		},
	}

	s := NewDHOrdersPollScheduler(client, syncState, campaignSvc, mocks.NewMockLogger(), DHOrdersPollConfig{
		Enabled:  true,
		Interval: 1 * time.Hour,
	})

	s.poll(context.Background())

	require.Len(t, importedRows, 1)
	require.Equal(t, "99998888", importedRows[0].CertNumber)
	require.Equal(t, 7.50, importedRows[0].UnitPrice)
	require.Len(t, confirmedItems, 1)
	require.Equal(t, "dh-12345", confirmedItems[0].OrderID)
}

func TestDHOrdersPoll_Disabled(t *testing.T) {
	s := NewDHOrdersPollScheduler(nil, nil, nil, mocks.NewMockLogger(), DHOrdersPollConfig{Enabled: false})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler should return immediately when disabled")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/adapters/scheduler/... -run TestDHOrdersPoll -count=1 -v`
Expected: FAIL — type doesn't exist yet.

- [ ] **Step 3: Write the implementation**

```go
package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const syncStateKeyDHOrdersPoll = "dh_orders_last_poll"

// DHOrdersClient is the subset of dh.Client used by the orders poll scheduler.
type DHOrdersClient interface {
	GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}

// DHOrdersPollConfig controls the orders poll scheduler.
type DHOrdersPollConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHOrdersPollScheduler polls DH for completed sales and auto-records them.
type DHOrdersPollScheduler struct {
	StopHandle
	client      DHOrdersClient
	syncState   SyncStateStore
	campaignSvc campaigns.Service
	logger      observability.Logger
	config      DHOrdersPollConfig
}

// NewDHOrdersPollScheduler creates a new orders poll scheduler.
func NewDHOrdersPollScheduler(
	client DHOrdersClient,
	syncState SyncStateStore,
	campaignSvc campaigns.Service,
	logger observability.Logger,
	config DHOrdersPollConfig,
) *DHOrdersPollScheduler {
	if config.Interval <= 0 {
		config.Interval = 30 * time.Minute
	}
	return &DHOrdersPollScheduler{
		StopHandle:  NewStopHandle(),
		client:      client,
		syncState:   syncState,
		campaignSvc: campaignSvc,
		logger:      logger.With(context.Background(), observability.String("component", "dh-orders-poll")),
		config:      config,
	}
}

// Start begins the polling loop.
func (s *DHOrdersPollScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "dh orders poll scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-orders-poll",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.poll)
}

func (s *DHOrdersPollScheduler) poll(ctx context.Context) {
	since, err := s.syncState.Get(ctx, syncStateKeyDHOrdersPoll)
	if err != nil {
		s.logger.Warn(ctx, "failed to read orders poll checkpoint", observability.Err(err))
	}
	if since == "" {
		since = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	}

	resp, err := s.client.GetOrders(ctx, dh.OrderFilters{Since: since, PerPage: 100})
	if err != nil {
		s.logger.Error(ctx, "failed to fetch DH orders", observability.Err(err))
		return
	}

	if len(resp.Orders) == 0 {
		s.logger.Info(ctx, "no new DH orders")
		return
	}

	// Convert DH orders to OrdersExportRows for the existing import pipeline
	rows := make([]campaigns.OrdersExportRow, 0, len(resp.Orders))
	orderIDMap := make(map[string]string) // certNumber -> orderID
	for _, order := range resp.Orders {
		channel := mapDHChannel(order.Channel)
		saleDate := parseDHSoldAt(order.SoldAt)

		rows = append(rows, campaigns.OrdersExportRow{
			OrderNumber:  order.OrderID,
			Date:         saleDate,
			SalesChannel: channel,
			ProductTitle: order.CardName,
			Grader:       "PSA",
			CertNumber:   order.CertNumber,
			Grade:        order.Grade,
			UnitPrice:    float64(order.SalePriceCents) / 100.0,
		})
		orderIDMap[order.CertNumber] = order.OrderID
	}

	// Run through ImportOrdersSales for cert matching and validation
	result, err := s.campaignSvc.ImportOrdersSales(ctx, rows)
	if err != nil {
		s.logger.Error(ctx, "failed to import DH orders", observability.Err(err))
		return
	}

	// Log skipped/unmatched orders
	for _, skip := range result.AlreadySold {
		s.logger.Info(ctx, "DH order already recorded",
			observability.String("cert", skip.CertNumber),
			observability.String("reason", skip.Reason))
	}
	for _, skip := range result.NotFound {
		s.logger.Warn(ctx, "DH order cert not found in inventory",
			observability.String("cert", skip.CertNumber))
	}

	// Auto-confirm matched orders
	if len(result.Matched) > 0 {
		confirmItems := make([]campaigns.OrdersConfirmItem, 0, len(result.Matched))
		for _, m := range result.Matched {
			confirmItems = append(confirmItems, campaigns.OrdersConfirmItem{
				PurchaseID:     m.PurchaseID,
				SaleChannel:    m.SaleChannel,
				SaleDate:       m.SaleDate,
				SalePriceCents: m.SalePriceCents,
				OrderID:        orderIDMap[m.CertNumber],
			})
		}

		bulkResult, err := s.campaignSvc.ConfirmOrdersSales(ctx, confirmItems)
		if err != nil {
			s.logger.Error(ctx, "failed to confirm DH orders", observability.Err(err))
			return
		}

		s.logger.Info(ctx, "DH orders recorded",
			observability.Int("created", bulkResult.Created),
			observability.Int("failed", bulkResult.Failed),
			observability.Int("already_sold", len(result.AlreadySold)),
			observability.Int("not_found", len(result.NotFound)))
	}

	// Advance checkpoint to the latest order timestamp
	latestSoldAt := findLatestSoldAt(resp.Orders)
	if latestSoldAt != "" {
		if err := s.syncState.Set(ctx, syncStateKeyDHOrdersPoll, latestSoldAt); err != nil {
			s.logger.Warn(ctx, "failed to update orders poll checkpoint", observability.Err(err))
		}
	}
}

// mapDHChannel maps DH channel names to domain SaleChannel values.
func mapDHChannel(channel string) campaigns.SaleChannel {
	switch channel {
	case "ebay":
		return campaigns.SaleChannelEbay
	case "shopify":
		return campaigns.SaleChannelTCGPlayer
	case "dh":
		return campaigns.SaleChannelDoubleHolo
	default:
		return campaigns.SaleChannelOther
	}
}

// parseDHSoldAt extracts YYYY-MM-DD from an ISO 8601 timestamp.
func parseDHSoldAt(soldAt string) string {
	t, err := time.Parse(time.RFC3339, soldAt)
	if err != nil {
		return time.Now().Format("2006-01-02")
	}
	return t.Format("2006-01-02")
}

// findLatestSoldAt returns the latest sold_at timestamp from a list of orders.
func findLatestSoldAt(orders []dh.Order) string {
	var latest string
	for _, o := range orders {
		if o.SoldAt > latest {
			latest = o.SoldAt
		}
	}
	return latest
}
```

- [ ] **Step 4: Support OrderID in ConfirmOrdersSales**

In `internal/domain/campaigns/service_import_orders.go`, in the `ConfirmOrdersSales` method, where it creates the `Sale` struct, add:

```go
	OrderID: item.OrderID,
```

This is a minimal change — just pass through the field from `OrdersConfirmItem` to `Sale`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/scheduler/... -run TestDHOrdersPoll -count=1 -v`
Expected: All 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/scheduler/dh_orders_poll.go \
        internal/adapters/scheduler/dh_orders_poll_test.go \
        internal/domain/campaigns/service_import_orders.go
git commit -m "feat: add DH orders poll scheduler with auto-confirm"
```

---

## Task 8: Inventory Status Poll Scheduler

**Files:**
- Create: `internal/adapters/scheduler/dh_inventory_poll.go`
- Create: `internal/adapters/scheduler/dh_inventory_poll_test.go`

- [ ] **Step 1: Write the test file**

```go
package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

type mockDHInventoryClient struct {
	resp      *dh.InventoryListResponse
	err       error
	callCount atomic.Int32
}

func (m *mockDHInventoryClient) ListInventory(_ context.Context, _ dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	m.callCount.Add(1)
	return m.resp, m.err
}

type mockDHFieldsUpdater struct {
	updates   []dhFieldsUpdate
	updateErr error
}

type dhFieldsUpdate struct {
	ID                string
	InventoryID       int
	CertStatus        string
	ListingPriceCents int
	ChannelsJSON      string
}

func (m *mockDHFieldsUpdater) UpdatePurchaseDHFields(_ context.Context, id string, _ int, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error {
	m.updates = append(m.updates, dhFieldsUpdate{
		ID: id, InventoryID: inventoryID, CertStatus: certStatus,
		ListingPriceCents: listingPriceCents, ChannelsJSON: channelsJSON,
	})
	return m.updateErr
}

type mockPurchaseByCertLookup struct {
	purchaseIDs map[string]string // certNumber -> purchaseID
}

func (m *mockPurchaseByCertLookup) GetPurchaseIDByCertNumber(_ context.Context, certNumber string) (string, error) {
	if id, ok := m.purchaseIDs[certNumber]; ok {
		return id, nil
	}
	return "", nil
}

func TestDHInventoryPoll_UpdatesPurchase(t *testing.T) {
	client := &mockDHInventoryClient{
		resp: &dh.InventoryListResponse{
			Items: []dh.InventoryListItem{
				{
					DHInventoryID:     98765,
					CertNumber:        "12345678",
					Status:            "active",
					ListingPriceCents: 7500,
					Channels: []dh.InventoryChannelStatus{
						{Name: "ebay", Status: "active"},
					},
				},
			},
			Meta: dh.PaginationMeta{Page: 1, PerPage: 25, TotalCount: 1},
		},
	}
	syncState := newMockSyncStateStore()
	updater := &mockDHFieldsUpdater{}
	lookup := &mockPurchaseByCertLookup{
		purchaseIDs: map[string]string{"12345678": "purchase-1"},
	}

	s := NewDHInventoryPollScheduler(client, syncState, updater, lookup, mocks.NewMockLogger(), DHInventoryPollConfig{
		Enabled:  true,
		Interval: 2 * time.Hour,
	})

	s.poll(context.Background())

	require.Len(t, updater.updates, 1)
	require.Equal(t, "purchase-1", updater.updates[0].ID)
	require.Equal(t, 98765, updater.updates[0].InventoryID)
	require.Equal(t, 7500, updater.updates[0].ListingPriceCents)
}

func TestDHInventoryPoll_Disabled(t *testing.T) {
	s := NewDHInventoryPollScheduler(nil, nil, nil, nil, mocks.NewMockLogger(), DHInventoryPollConfig{Enabled: false})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler should return immediately when disabled")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/adapters/scheduler/... -run TestDHInventoryPoll -count=1 -v`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

```go
package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const syncStateKeyDHInventoryPoll = "dh_inventory_last_poll"

// DHInventoryListClient is the subset of dh.Client used by the inventory poll scheduler.
type DHInventoryListClient interface {
	ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
}

// DHFieldsUpdater updates DH tracking fields on a purchase.
type DHFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, cardID, inventoryID int, certStatus string, listingPriceCents int, channelsJSON string) error
}

// PurchaseByCertLookup resolves a cert number to a purchase ID.
type PurchaseByCertLookup interface {
	GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error)
}

// DHInventoryPollConfig controls the inventory poll scheduler.
type DHInventoryPollConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHInventoryPollScheduler polls DH for inventory status updates.
type DHInventoryPollScheduler struct {
	StopHandle
	client    DHInventoryListClient
	syncState SyncStateStore
	updater   DHFieldsUpdater
	lookup    PurchaseByCertLookup
	logger    observability.Logger
	config    DHInventoryPollConfig
}

// NewDHInventoryPollScheduler creates a new inventory poll scheduler.
func NewDHInventoryPollScheduler(
	client DHInventoryListClient,
	syncState SyncStateStore,
	updater DHFieldsUpdater,
	lookup PurchaseByCertLookup,
	logger observability.Logger,
	config DHInventoryPollConfig,
) *DHInventoryPollScheduler {
	if config.Interval <= 0 {
		config.Interval = 2 * time.Hour
	}
	return &DHInventoryPollScheduler{
		StopHandle: NewStopHandle(),
		client:     client,
		syncState:  syncState,
		updater:    updater,
		lookup:     lookup,
		logger:     logger.With(context.Background(), observability.String("component", "dh-inventory-poll")),
		config:     config,
	}
}

// Start begins the polling loop.
func (s *DHInventoryPollScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "dh inventory poll scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-inventory-poll",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.poll)
}

func (s *DHInventoryPollScheduler) poll(ctx context.Context) {
	since, err := s.syncState.Get(ctx, syncStateKeyDHInventoryPoll)
	if err != nil {
		s.logger.Warn(ctx, "failed to read inventory poll checkpoint", observability.Err(err))
	}

	resp, err := s.client.ListInventory(ctx, dh.InventoryFilters{
		Status:       "active",
		UpdatedSince: since,
		PerPage:      100,
	})
	if err != nil {
		s.logger.Error(ctx, "failed to fetch DH inventory", observability.Err(err))
		return
	}

	if len(resp.Items) == 0 {
		return
	}

	var updated, skipped int
	var latestUpdatedAt string

	for _, item := range resp.Items {
		purchaseID, err := s.lookup.GetPurchaseIDByCertNumber(ctx, item.CertNumber)
		if err != nil {
			s.logger.Warn(ctx, "cert lookup failed", observability.String("cert", item.CertNumber), observability.Err(err))
			skipped++
			continue
		}
		if purchaseID == "" {
			skipped++
			continue
		}

		channelsJSON, _ := json.Marshal(item.Channels)

		if err := s.updater.UpdatePurchaseDHFields(ctx, purchaseID,
			item.DHCardID, item.DHInventoryID, "matched",
			item.ListingPriceCents, string(channelsJSON),
		); err != nil {
			s.logger.Warn(ctx, "failed to update purchase DH fields",
				observability.String("purchase_id", purchaseID), observability.Err(err))
			continue
		}
		updated++

		if item.UpdatedAt > latestUpdatedAt {
			latestUpdatedAt = item.UpdatedAt
		}
	}

	s.logger.Info(ctx, "inventory poll complete",
		observability.Int("updated", updated),
		observability.Int("skipped", skipped))

	if latestUpdatedAt != "" {
		if err := s.syncState.Set(ctx, syncStateKeyDHInventoryPoll, latestUpdatedAt); err != nil {
			s.logger.Warn(ctx, "failed to update inventory poll checkpoint", observability.Err(err))
		}
	}
}
```

- [ ] **Step 4: Add GetPurchaseIDByCertNumber to the SQLite repository**

In `internal/adapters/storage/sqlite/purchases_repository.go`, add:

```go
// GetPurchaseIDByCertNumber returns the purchase ID for a given cert number.
func (r *PurchasesRepository) GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_purchases WHERE cert_number = ?`, certNumber,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/scheduler/... -run TestDHInventoryPoll -count=1 -v`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/scheduler/dh_inventory_poll.go \
        internal/adapters/scheduler/dh_inventory_poll_test.go \
        internal/adapters/storage/sqlite/purchases_repository.go
git commit -m "feat: add DH inventory status poll scheduler"
```

---

## Task 9: Config and Wiring — Connect Schedulers to BuildGroup

**Files:**
- Modify: `internal/platform/config/types.go`
- Modify: `internal/adapters/scheduler/builder.go`

- [ ] **Step 1: Add v2 config fields**

In `internal/platform/config/types.go`, extend `DHConfig`:

```go
type DHConfig struct {
	Enabled              bool
	CacheTTLHours        int
	RateLimitRPS         int
	OrdersPollInterval   time.Duration // default: 30m
	InventoryPollInterval time.Duration // default: 2h
}
```

- [ ] **Step 2: Load new env vars**

In `internal/platform/config/loader.go`, in the DH section, add:

```go
if v := os.Getenv("DH_ORDERS_POLL_INTERVAL"); v != "" {
	if d, err := time.ParseDuration(v); err == nil {
		cfg.DH.OrdersPollInterval = d
	}
}
if v := os.Getenv("DH_INVENTORY_POLL_INTERVAL"); v != "" {
	if d, err := time.ParseDuration(v); err == nil {
		cfg.DH.InventoryPollInterval = d
	}
}
```

- [ ] **Step 3: Add new dependencies to BuildDeps**

In `internal/adapters/scheduler/builder.go`, add to `BuildDeps`:

```go
	// DH v2 dependencies (optional)
	DHOrdersClient        DHOrdersClient
	DHInventoryListClient DHInventoryListClient
	DHFieldsUpdater       DHFieldsUpdater
	PurchaseByCertLookup  PurchaseByCertLookup
	CampaignService       campaigns.Service
```

- [ ] **Step 4: Wire schedulers in BuildGroup**

In `internal/adapters/scheduler/builder.go`, in the `BuildGroup` function, after the existing DH scheduler blocks, add:

```go
	// DH v2: Orders poll scheduler
	if deps.DHOrdersClient != nil && deps.SyncStateStore != nil && deps.CampaignService != nil {
		ordersPollCfg := DHOrdersPollConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: cfg.DH.OrdersPollInterval,
		}
		schedulers = append(schedulers, NewDHOrdersPollScheduler(
			deps.DHOrdersClient,
			deps.SyncStateStore,
			deps.CampaignService,
			deps.Logger,
			ordersPollCfg,
		))
	}

	// DH v2: Inventory status poll scheduler
	if deps.DHInventoryListClient != nil && deps.SyncStateStore != nil && deps.DHFieldsUpdater != nil && deps.PurchaseByCertLookup != nil {
		inventoryPollCfg := DHInventoryPollConfig{
			Enabled:  cfg.DH.Enabled,
			Interval: cfg.DH.InventoryPollInterval,
		}
		schedulers = append(schedulers, NewDHInventoryPollScheduler(
			deps.DHInventoryListClient,
			deps.SyncStateStore,
			deps.DHFieldsUpdater,
			deps.PurchaseByCertLookup,
			deps.Logger,
			inventoryPollCfg,
		))
	}
```

- [ ] **Step 5: Wire in main/server setup**

Find where `BuildDeps` is populated (likely `cmd/slabledger/` or a server setup file) and add the new dependency assignments. The DH client (`*dh.Client`) implements all the new interfaces, and the SQLite purchases repository implements `DHFieldsUpdater` and `PurchaseByCertLookup`:

```go
	// In the BuildDeps population:
	DHOrdersClient:        dhClient,        // *dh.Client implements DHOrdersClient
	DHInventoryListClient: dhClient,        // *dh.Client implements DHInventoryListClient
	DHFieldsUpdater:       purchasesRepo,   // *sqlite.PurchasesRepository
	PurchaseByCertLookup:  purchasesRepo,   // *sqlite.PurchasesRepository
	CampaignService:       campaignService, // campaigns.Service
```

- [ ] **Step 6: Run all tests**

Run: `go test ./... -count=1 -timeout 5m`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/config/types.go \
        internal/platform/config/loader.go \
        internal/adapters/scheduler/builder.go
git commit -m "feat: wire DH v2 schedulers into config and BuildGroup"
```

---

## Task 10: Full Integration Test

**Files:** None new — this is a verification task.

- [ ] **Step 1: Run the full test suite**

Run: `go test -race -timeout 10m ./...`
Expected: All tests pass, no race conditions.

- [ ] **Step 2: Run quality checks**

Run: `make check`
Expected: Lint, architecture import checks, and file size checks all pass.

- [ ] **Step 3: Verify the app starts**

```bash
go build -o slabledger ./cmd/slabledger && echo "BUILD OK"
```

Expected: Clean build.

- [ ] **Step 4: Commit any fixes**

If any issues were found, fix and commit them.

---

## Summary

| Task | Component | New Files | Modified Files |
|------|-----------|-----------|----------------|
| 1 | Migration | 2 | 0 |
| 2 | Domain types | 0 | 2 |
| 3 | Repository layer | 0 | 5 |
| 4 | Client types | 1 | 0 |
| 5 | Client: cert resolution | 2 | 1 |
| 6 | Client: inventory/orders | 2 | 1 |
| 7 | Orders poll scheduler | 2 | 1 |
| 8 | Inventory poll scheduler | 2 | 1 |
| 9 | Config and wiring | 0 | 3 |
| 10 | Integration verification | 0 | 0 |

**Not in scope for this plan** (per spec's "Out of Scope" + separate follow-up tasks):
- PSA import flow enhancement (cert resolution during import) — depends on UI work for progress indication and disambiguation
- Push-to-DH HTTP handler — depends on UI for triggering
- Integration API → Enterprise API migration (search, market data) — separate task per spec's migration strategy
- Fee mapping refinements for DH's Shopify channel — needs business confirmation
