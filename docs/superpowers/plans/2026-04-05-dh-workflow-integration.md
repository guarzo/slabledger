# DH Workflow Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire DH inventory push, status transitions, and channel sync into the existing card lifecycle so cards flow automatically from purchase → DH tracking → listing → channel sync.

**Architecture:** Extend the existing DH client with new methods (UpdateInventory, SyncChannels, DelistChannels) and add `status` to PushInventory. Wire these into two trigger points: (1) the bulk match handler pushes matched cards as `in_stock`, and (2) the cert import handler flips cards to `listed` + triggers channel sync. Update the inventory poll scheduler to handle `in_stock`/`listed` status values.

**Tech Stack:** Go 1.26, httpx (unified HTTP client), httptest (test servers), testify/require

**Spec:** `docs/superpowers/specs/2026-04-05-dh-workflow-review-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/adapters/clients/dh/types_v2.go` | Add status constants, `Status` field to types, new request/response types |
| Modify | `internal/adapters/clients/dh/client.go` | Add `patchEnterprise` and `deleteEnterprise` HTTP helpers |
| Modify | `internal/adapters/clients/dh/inventory.go` | Add `UpdateInventory`, `SyncChannels`, `DelistChannels` methods |
| Modify | `internal/adapters/clients/dh/inventory_test.go` | Tests for all new/modified methods |
| Modify | `internal/adapters/httpserver/handlers/dh_handler.go` | Add `DHInventoryPusher` interface, inject into handler |
| Modify | `internal/adapters/httpserver/handlers/dh_match_handler.go` | Push matched cards to DH as `in_stock` after bulk match |
| Modify | `internal/adapters/httpserver/handlers/campaigns.go` | Add `DHInventoryLister` interface, inject into handler |
| Modify | `internal/adapters/httpserver/handlers/campaigns_imports.go` | Trigger `listed` + channel sync on cert import |
| Modify | `internal/adapters/scheduler/dh_inventory_poll.go` | Handle `in_stock`/`listed` statuses, remove hardcoded `"active"` filter |
| Modify | `internal/adapters/scheduler/dh_inventory_poll_test.go` | Update test expectations for new status values |

---

### Task 1: Add inventory status constants and update types

**Files:**
- Modify: `internal/adapters/clients/dh/types_v2.go`

- [ ] **Step 1: Add inventory status constants**

Add after the cert status constants block (line 9):

```go
// --- Inventory Status Constants ---

const (
	InventoryStatusInStock = "in_stock"
	InventoryStatusListed  = "listed"
)
```

- [ ] **Step 2: Add `Status` field to `InventoryItem`**

Update the `InventoryItem` struct (line 76-82) to include the optional status field:

```go
type InventoryItem struct {
	DHCardID       int     `json:"dh_card_id"`
	CertNumber     string  `json:"cert_number"`
	GradingCompany string  `json:"grading_company"`
	Grade          float64 `json:"grade"`
	CostBasisCents int     `json:"cost_basis_cents"`
	Status         string  `json:"status,omitempty"` // "in_stock" (default) or "listed"
}
```

- [ ] **Step 3: Add `Status` field to `InventoryUpdate`**

Update the `InventoryUpdate` struct (line 141-144):

```go
type InventoryUpdate struct {
	Status         string `json:"status,omitempty"`
	CostBasisCents *int   `json:"cost_basis_cents,omitempty"`
}
```

Note: `CostBasisCents` changes to `*int` so we can omit it when only updating status (zero value would set cost to 0). The `omitempty` tag on both fields means the API only receives fields we intend to update.

- [ ] **Step 4: Update `InventoryResult.Status` comment**

Update the comment on `InventoryResult.Status` (line 99):

```go
Status             string                   `json:"status"` // "in_stock", "listed", "failed"
```

- [ ] **Step 5: Update `InventoryChannelStatus.Status` comment**

Update the comment on `InventoryChannelStatus.Status` (line 92):

```go
Status string `json:"status"` // "pending", "active", "error"
```

- [ ] **Step 6: Add channel sync request/response types**

Add after the `InventoryUpdate` struct:

```go
// ChannelSyncRequest is the request body for POST /inventory/:id/sync.
type ChannelSyncRequest struct {
	Channels []string `json:"channels"`
}

// ChannelSyncResponse is the response from POST /inventory/:id/sync.
type ChannelSyncResponse struct {
	DHInventoryID int                      `json:"dh_inventory_id"`
	Status        string                   `json:"status"`
	Channels      []InventoryChannelStatus `json:"channels"`
}

// ChannelDelistRequest is the request body for DELETE /inventory/:id/sync.
type ChannelDelistRequest struct {
	Channels []string `json:"channels,omitempty"` // empty = delist from all
}
```

- [ ] **Step 7: Build and verify compilation**

Run: `cd /workspace/.worktrees/guarzo-workflow && go build ./...`
Expected: no errors

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/clients/dh/types_v2.go
git commit -m "feat(dh): add inventory status constants and channel sync types"
```

---

### Task 2: Add `patchEnterprise` and `deleteEnterprise` HTTP helpers

**Files:**
- Modify: `internal/adapters/clients/dh/client.go`

- [ ] **Step 1: Add `patchEnterprise` method**

Add after `postEnterprise` (line 237):

```go
// patchEnterprise performs a PATCH request with Bearer auth for the enterprise API.
func (c *Client) patchEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
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
		enterpriseAuthHeader: "Bearer " + c.enterpriseKey,
		"Content-Type":       "application/json",
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Do(ctx, httpx.Request{
		Method:  "PATCH",
		URL:     fullURL,
		Headers: headers,
		Body:    bodyBytes,
		Timeout: c.timeout,
	})
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
```

- [ ] **Step 2: Add `deleteEnterprise` method**

Add after `patchEnterprise`:

```go
// deleteEnterprise performs a DELETE request with Bearer auth for the enterprise API.
// body may be nil for bodyless deletes.
func (c *Client) deleteEnterprise(ctx context.Context, fullURL string, body any, dest any) error {
	if !c.EnterpriseAvailable() {
		return apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
	}

	if err := c.limiter.Wait(ctx); err != nil {
		if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apperrors.ProviderUnavailable(providerName, err)
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return apperrors.ProviderInvalidRequest(providerName, err)
		}
	}

	headers := map[string]string{
		enterpriseAuthHeader: "Bearer " + c.enterpriseKey,
		"Content-Type":       "application/json",
		"Accept":             "application/json",
	}

	resp, err := c.httpClient.Do(ctx, httpx.Request{
		Method:  "DELETE",
		URL:     fullURL,
		Headers: headers,
		Body:    bodyBytes,
		Timeout: c.timeout,
	})
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
```

- [ ] **Step 3: Add httpx import**

Add `"github.com/guarzo/slabledger/internal/adapters/clients/httpx"` to the import block. (It's already imported indirectly but needs a direct reference now for `httpx.Request`.)

- [ ] **Step 4: Build and verify compilation**

Run: `cd /workspace/.worktrees/guarzo-workflow && go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/dh/client.go
git commit -m "feat(dh): add patchEnterprise and deleteEnterprise HTTP helpers"
```

---

### Task 3: Add `UpdateInventory`, `SyncChannels`, `DelistChannels` methods

**Files:**
- Modify: `internal/adapters/clients/dh/inventory.go`
- Modify: `internal/adapters/clients/dh/inventory_test.go`

- [ ] **Step 1: Write failing test for `UpdateInventory`**

Add to `inventory_test.go`:

```go
func TestClient_UpdateInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "PATCH", r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory/98765", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

		var req InventoryUpdate
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, InventoryStatusListed, req.Status)

		resp := InventoryResult{
			DHInventoryID: 98765,
			Status:        InventoryStatusListed,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	result, err := c.UpdateInventory(context.Background(), 98765, InventoryUpdate{
		Status: InventoryStatusListed,
	})
	require.NoError(t, err)
	require.Equal(t, 98765, result.DHInventoryID)
	require.Equal(t, InventoryStatusListed, result.Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -run TestClient_UpdateInventory -v`
Expected: FAIL — `c.UpdateInventory` is undefined

- [ ] **Step 3: Implement `UpdateInventory`**

Add to `inventory.go`:

```go
// UpdateInventory updates an inventory item on DH (status and/or cost basis).
func (c *Client) UpdateInventory(ctx context.Context, inventoryID int, update InventoryUpdate) (*InventoryResult, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d", c.baseURL, inventoryID)

	var resp InventoryResult
	if err := c.patchEnterprise(ctx, fullURL, update, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -run TestClient_UpdateInventory -v`
Expected: PASS

- [ ] **Step 5: Write failing test for `SyncChannels`**

Add to `inventory_test.go`:

```go
func TestClient_SyncChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory/98765/sync", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

		var req ChannelSyncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, []string{"ebay", "shopify"}, req.Channels)

		resp := ChannelSyncResponse{
			DHInventoryID: 98765,
			Status:        InventoryStatusListed,
			Channels: []InventoryChannelStatus{
				{Name: "ebay", Status: "pending"},
				{Name: "shopify", Status: "pending"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.SyncChannels(context.Background(), 98765, []string{"ebay", "shopify"})
	require.NoError(t, err)
	require.Equal(t, 98765, resp.DHInventoryID)
	require.Equal(t, InventoryStatusListed, resp.Status)
	require.Len(t, resp.Channels, 2)
	require.Equal(t, "ebay", resp.Channels[0].Name)
	require.Equal(t, "pending", resp.Channels[0].Status)
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -run TestClient_SyncChannels -v`
Expected: FAIL — `c.SyncChannels` is undefined

- [ ] **Step 7: Implement `SyncChannels`**

Add to `inventory.go`:

```go
// SyncChannels pushes a listed inventory item to external sales channels.
func (c *Client) SyncChannels(ctx context.Context, inventoryID int, channels []string) (*ChannelSyncResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/sync", c.baseURL, inventoryID)
	body := ChannelSyncRequest{Channels: channels}

	var resp ChannelSyncResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -run TestClient_SyncChannels -v`
Expected: PASS

- [ ] **Step 9: Write failing test for `DelistChannels`**

Add to `inventory_test.go`:

```go
func TestClient_DelistChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "DELETE", r.Method)
		require.Equal(t, "/api/v1/enterprise/inventory/98765/sync", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

		var req ChannelDelistRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, []string{"ebay"}, req.Channels)

		resp := ChannelSyncResponse{
			DHInventoryID: 98765,
			Status:        InventoryStatusListed,
			Channels: []InventoryChannelStatus{
				{Name: "shopify", Status: "active"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.DelistChannels(context.Background(), 98765, []string{"ebay"})
	require.NoError(t, err)
	require.Equal(t, 98765, resp.DHInventoryID)
	require.Len(t, resp.Channels, 1)
	require.Equal(t, "shopify", resp.Channels[0].Name)
}
```

- [ ] **Step 10: Run test to verify it fails**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -run TestClient_DelistChannels -v`
Expected: FAIL — `c.DelistChannels` is undefined

- [ ] **Step 11: Implement `DelistChannels`**

Add to `inventory.go`:

```go
// DelistChannels removes a listed inventory item from specific external channels.
// If channels is empty, delists from all channels.
func (c *Client) DelistChannels(ctx context.Context, inventoryID int, channels []string) (*ChannelSyncResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/sync", c.baseURL, inventoryID)

	var body *ChannelDelistRequest
	if len(channels) > 0 {
		body = &ChannelDelistRequest{Channels: channels}
	}

	var resp ChannelSyncResponse
	if err := c.deleteEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 12: Run all DH client tests**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -v`
Expected: all tests PASS

- [ ] **Step 13: Update existing `TestClient_PushInventory` for status field**

Update `TestClient_PushInventory` in `inventory_test.go` to include the `Status` field in the push request. In the server handler, add an assertion:

```go
require.Equal(t, InventoryStatusInStock, item.Status)
```

And in the test's item construction, add:

```go
Status: InventoryStatusInStock,
```

Also update the response status from `"active"` to `InventoryStatusInStock`:

```go
Status: InventoryStatusInStock,
```

- [ ] **Step 14: Run all DH client tests again**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./internal/adapters/clients/dh/ -v`
Expected: all tests PASS

- [ ] **Step 15: Commit**

```bash
git add internal/adapters/clients/dh/inventory.go internal/adapters/clients/dh/inventory_test.go
git commit -m "feat(dh): add UpdateInventory, SyncChannels, DelistChannels methods"
```

---

### Task 4: Wire inventory push into bulk match handler

After a successful bulk match, push matched cards to DH as `in_stock` so they start getting pricing data and intelligence.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_handler.go`
- Modify: `internal/adapters/httpserver/handlers/dh_match_handler.go`

- [ ] **Step 1: Add `DHInventoryPusher` interface to `dh_handler.go`**

Add after the `DHPurchaseLister` interface (line 31):

```go
// DHInventoryPusher pushes inventory items to DH.
type DHInventoryPusher interface {
	PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}
```

Add `dh` import: `"github.com/guarzo/slabledger/internal/adapters/clients/dh"`

- [ ] **Step 2: Add `inventoryPusher` field to `DHHandler`**

Add to the `DHHandler` struct (line 46):

```go
inventoryPusher DHInventoryPusher // optional: pushes matched cards to DH inventory
```

- [ ] **Step 3: Update `NewDHHandler` signature**

Add the parameter to `NewDHHandler` (insert after `purchaseLister`):

```go
func NewDHHandler(
	matchClient DHMatchClient,
	cardIDSaver DHCardIDSaver,
	purchaseLister DHPurchaseLister,
	inventoryPusher DHInventoryPusher,  // NEW
	intelRepo intelligence.Repository,
	suggestionsRepo intelligence.SuggestionsRepository,
	intelCounter DHIntelligenceCounter,
	suggestCounter DHSuggestionsCounter,
	logger observability.Logger,
	baseCtx context.Context,
) *DHHandler {
```

And assign it inside the constructor:

```go
inventoryPusher: inventoryPusher,
```

- [ ] **Step 4: Add inventory push to `runBulkMatch`**

In `dh_match_handler.go`, after the bulk match loop completes (after the existing logger.Info at line 101), add an inventory push step. Replace the final `h.logger.Info(ctx, "bulk match completed", ...)` block with:

```go
	h.logger.Info(ctx, "bulk match completed",
		observability.Int("total", len(identities)),
		observability.Int("matched", matched),
		observability.Int("skipped", skipped),
		observability.Int("low_confidence", lowConf),
		observability.Int("failed", failed))

	// Push newly matched cards to DH as in_stock for early pricing/intelligence.
	if h.inventoryPusher != nil && len(pushItems) > 0 {
		h.pushMatchedInventory(ctx, pushItems)
	}
```

Before the loop, declare the `pushItems` slice:

```go
var pushItems []dhPushCandidate
```

Inside the loop, after the successful `SaveExternalID` call (after `matched++`, line 98), collect push candidates:

```go
		pushItems = append(pushItems, dhPushCandidate{
			dhCardID:   matchResp.CardID,
			certNumber: "", // will be filled per-purchase below
		})
```

Wait — on reflection, `runBulkMatch` works with deduplicated `CardIdentity` structs (card name + set + number), not individual purchases. A single card identity can map to multiple cert numbers. The push needs cert-level data (cert number, grade, cost). So the push should happen at the purchase level, not the identity level.

A cleaner approach: collect the newly matched DH card IDs, then after the match loop, query purchases that have those card identities, and push each one.

- [ ] **Step 4 (revised): Collect matched identities, then push purchases**

In `dh_match_handler.go`, update `runBulkMatch` to collect matched card identities and push their purchases:

After the match loop's final log statement, add:

```go
	// Push newly matched cards to DH inventory as in_stock.
	if h.inventoryPusher != nil && len(matchedIdentities) > 0 {
		h.pushMatchedToDH(ctx, matchedIdentities)
	}
```

Before the loop, declare:

```go
var matchedIdentities []campaigns.CardIdentity
```

Inside the loop, after `matched++` (line 98), collect the identity:

```go
		matchedIdentities = append(matchedIdentities, ci)
```

- [ ] **Step 5: Implement `pushMatchedToDH` method**

Add to `dh_match_handler.go`:

```go
// pushMatchedToDH pushes purchases for newly matched card identities to DH as in_stock.
func (h *DHHandler) pushMatchedToDH(ctx context.Context, matched []campaigns.CardIdentity) {
	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "push to DH: list purchases failed", observability.Err(err))
		return
	}

	// Build lookup of matched identities
	matchedSet := make(map[string]bool, len(matched))
	for _, ci := range matched {
		matchedSet[dhCardKey(ci.CardName, ci.SetName, ci.CardNumber)] = true
	}

	// Collect pushable purchases: matched identity, has DH card ID, has cert, no existing DH inventory ID
	var items []dh.InventoryItem
	for _, p := range purchases {
		key := dhCardKey(p.CardName, p.SetName, p.CardNumber)
		if !matchedSet[key] {
			continue
		}
		if p.DHCardID == 0 || p.CertNumber == "" || p.DHInventoryID != 0 {
			continue
		}
		items = append(items, dh.InventoryItem{
			DHCardID:       p.DHCardID,
			CertNumber:     p.CertNumber,
			GradingCompany: "psa",
			Grade:          p.GradeValue,
			CostBasisCents: p.BuyCostCents,
			Status:         dh.InventoryStatusInStock,
		})
	}

	if len(items) == 0 {
		return
	}

	resp, err := h.inventoryPusher.PushInventory(ctx, items)
	if err != nil {
		h.logger.Error(ctx, "push to DH failed",
			observability.Int("items", len(items)), observability.Err(err))
		return
	}

	pushed := 0
	for _, r := range resp.Results {
		if r.Status != "failed" {
			pushed++
		}
	}
	h.logger.Info(ctx, "pushed matched inventory to DH",
		observability.Int("pushed", pushed),
		observability.Int("total", len(items)))
}
```

- [ ] **Step 6: Update main.go `NewDHHandler` call**

Find the `NewDHHandler` call in `cmd/slabledger/main.go` and add `dhClient` as the `inventoryPusher` parameter (insert after `campaignsRepo`):

```go
dhHandler = handlers.NewDHHandler(
	dhClient,           // DHMatchClient
	cardIDMappingRepo,  // DHCardIDSaver
	campaignsRepo,      // DHPurchaseLister
	dhClient,           // DHInventoryPusher  ← NEW
	intelRepo,          // ...
	// ... rest unchanged
)
```

- [ ] **Step 7: Build and verify compilation**

Run: `cd /workspace/.worktrees/guarzo-workflow && go build ./...`
Expected: no errors

- [ ] **Step 8: Run all tests**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./... 2>&1 | tail -30`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_handler.go internal/adapters/httpserver/handlers/dh_match_handler.go cmd/slabledger/main.go
git commit -m "feat(dh): push matched cards to DH as in_stock after bulk match"
```

---

### Task 5: Wire `listed` + channel sync into cert import

When cards physically arrive (cert import), flip their DH status to `listed` and trigger channel sync to eBay/Shopify.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns.go`
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports.go`

- [ ] **Step 1: Add DH listing interface to `campaigns.go`**

Add after the imports:

```go
// DHInventoryLister transitions DH inventory items to listed and syncs channels.
type DHInventoryLister interface {
	UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
}
```

Add `dh` import: `"github.com/guarzo/slabledger/internal/adapters/clients/dh"`

- [ ] **Step 2: Add `dhLister` field to `CampaignsHandler`**

Add to the struct:

```go
dhLister DHInventoryLister // optional: lists cards on DH after cert import
```

- [ ] **Step 3: Update `NewCampaignsHandler` signature**

```go
func NewCampaignsHandler(
	service campaigns.Service,
	logger observability.Logger,
	discoverer CardDiscoverer,
	dhLister DHInventoryLister,  // NEW
	baseCtx context.Context,
) *CampaignsHandler {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &CampaignsHandler{
		service:    service,
		logger:     logger,
		discoverer: discoverer,
		dhLister:   dhLister,
		baseCtx:    baseCtx,
	}
}
```

- [ ] **Step 4: Add `triggerDHListing` method to `campaigns_imports.go`**

Add after `triggerCardDiscovery`:

```go
// triggerDHListing transitions DH inventory items to "listed" and syncs to sales channels
// in a background goroutine so it doesn't delay the HTTP response.
func (h *CampaignsHandler) triggerDHListing(certNumbers []string) {
	if h.dhLister == nil || len(certNumbers) == 0 {
		return
	}

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error(h.baseCtx, "panic in triggerDHListing",
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		ctx, cancel := context.WithTimeout(h.baseCtx, 5*time.Minute)
		defer cancel()

		// Look up purchases by cert to find their DH inventory IDs
		for _, cert := range certNumbers {
			purchases, err := h.service.GetPurchasesByCert(ctx, cert)
			if err != nil {
				h.logger.Warn(ctx, "dh listing: cert lookup failed",
					observability.String("cert", cert), observability.Err(err))
				continue
			}

			for _, p := range purchases {
				if p.DHInventoryID == 0 {
					continue // not yet pushed to DH
				}

				// Step 1: Flip to listed
				_, err := h.dhLister.UpdateInventory(ctx, p.DHInventoryID, dh.InventoryUpdate{
					Status: dh.InventoryStatusListed,
				})
				if err != nil {
					h.logger.Warn(ctx, "dh listing: status update failed",
						observability.String("cert", cert),
						observability.Int("inventoryID", p.DHInventoryID),
						observability.Err(err))
					continue
				}

				// Step 2: Sync to channels
				_, err = h.dhLister.SyncChannels(ctx, p.DHInventoryID, []string{"ebay", "shopify"})
				if err != nil {
					h.logger.Warn(ctx, "dh listing: channel sync failed",
						observability.String("cert", cert),
						observability.Int("inventoryID", p.DHInventoryID),
						observability.Err(err))
					continue
				}

				h.logger.Info(ctx, "dh listing: card listed and synced",
					observability.String("cert", cert),
					observability.Int("inventoryID", p.DHInventoryID))
			}
		}
	}()
}
```

- [ ] **Step 5: Check if `GetPurchasesByCert` exists on campaigns.Service**

Search for this method — if it doesn't exist, we need to use the repository method `GetPurchasesByGraderAndCertNumbers` instead. The simpler approach: since `HandleImportCerts` already calls `h.service.ImportCerts()` which returns the result with imported cert numbers, we can pass those certs and use the existing `GetPurchasesByGraderAndCertNumbers` repo method through the service.

Alternative simpler approach: since the import result tells us which certs were successfully imported or already existed (and got flagged), we can look up those purchases from the import result. But the handler doesn't have access to the repo directly.

Simplest approach: add a `ListPurchasesByCerts` method to the `campaigns.Service` interface and implement it. But that's adding a new domain method.

Actually — let me reconsider. The cert import handler already gets back a `CertImportResult` with the imported cert numbers. The purchases for those certs were either just created (new imports, `DHInventoryID` = 0, no DH push yet) or already existed (already flagged). For new imports, they won't have `DHInventoryID` yet — they need to go through bulk match first.

For already-existing purchases that DO have a `DHInventoryID` (pushed as `in_stock` from a previous bulk match), cert import is the signal to list them.

So the flow is:
1. Cert import processes certs (some new, some existing)
2. For existing certs that already have `DHInventoryID` > 0: trigger listing

The simplest approach: pass the result to `triggerDHListing` and let it filter by `AlreadyExisted` certs. But we need purchase data (specifically `DHInventoryID`) which the result doesn't carry.

Best approach: add a `GetPurchasesByCertNumbers` method to the service that returns purchases for a set of cert numbers.

- [ ] **Step 5 (revised): Add `GetPurchasesByCertNumbers` to campaigns service**

Add to `internal/domain/campaigns/repository.go` in the `PurchaseRepository` interface:

This method already exists as `GetPurchasesByGraderAndCertNumbers`. We can use the existing method. But the service doesn't expose it. Add a thin wrapper to the service.

Add to the `Service` interface in `internal/domain/campaigns/service.go`:

```go
GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) ([]Purchase, error)
```

Implement in a new or existing service file:

```go
func (s *service) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) ([]Purchase, error) {
	m, err := s.repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", certNumbers)
	if err != nil {
		return nil, err
	}
	purchases := make([]Purchase, 0, len(m))
	for _, p := range m {
		purchases = append(purchases, p)
	}
	return purchases, nil
}
```

- [ ] **Step 6: Revise `triggerDHListing` to use `GetPurchasesByCertNumbers`**

Replace the per-cert lookup loop with a batch lookup:

```go
func (h *CampaignsHandler) triggerDHListing(certNumbers []string) {
	if h.dhLister == nil || len(certNumbers) == 0 {
		return
	}

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error(h.baseCtx, "panic in triggerDHListing",
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		ctx, cancel := context.WithTimeout(h.baseCtx, 5*time.Minute)
		defer cancel()

		purchases, err := h.service.GetPurchasesByCertNumbers(ctx, certNumbers)
		if err != nil {
			h.logger.Warn(ctx, "dh listing: batch cert lookup failed", observability.Err(err))
			return
		}

		listed, synced := 0, 0
		for _, p := range purchases {
			if p.DHInventoryID == 0 {
				continue // not yet pushed to DH
			}

			_, err := h.dhLister.UpdateInventory(ctx, p.DHInventoryID, dh.InventoryUpdate{
				Status: dh.InventoryStatusListed,
			})
			if err != nil {
				h.logger.Warn(ctx, "dh listing: status update failed",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(err))
				continue
			}
			listed++

			_, err = h.dhLister.SyncChannels(ctx, p.DHInventoryID, []string{"ebay", "shopify"})
			if err != nil {
				h.logger.Warn(ctx, "dh listing: channel sync failed",
					observability.String("cert", p.CertNumber),
					observability.Int("inventoryID", p.DHInventoryID),
					observability.Err(err))
				continue
			}
			synced++
		}

		if listed > 0 || synced > 0 {
			h.logger.Info(ctx, "dh listing completed",
				observability.Int("listed", listed),
				observability.Int("synced", synced),
				observability.Int("certs", len(certNumbers)))
		}
	}()
}
```

- [ ] **Step 7: Call `triggerDHListing` from `HandleImportCerts`**

In `campaigns_imports.go`, update `HandleImportCerts` (line 278-300). After the `writeJSON` call, trigger DH listing for all processed certs. Add before `writeJSON`:

```go
	// Trigger DH listing for certs that may have DH inventory (existing certs with in_stock status).
	h.triggerDHListing(cleaned)
```

Wait — `cleaned` is inside the service method, not the handler. The handler receives `req.CertNumbers`. The result has counts but not the cert list. We need to pass the certs. Actually, looking at the handler again (line 279-300), `req.CertNumbers` is the raw input. We should pass all cert numbers and let `triggerDHListing` filter by `DHInventoryID > 0`.

Add after line 297 (after the error check, before writeJSON):

```go
	// Trigger DH listing for certs that already have DH inventory items as in_stock.
	h.triggerDHListing(req.CertNumbers)
```

- [ ] **Step 8: Update main.go `NewCampaignsHandler` call**

Find the `NewCampaignsHandler` call and add `dhClient` as the `dhLister` parameter:

```go
campaignsHandler := handlers.NewCampaignsHandler(
	campaignsService,
	logger,
	discoverer,
	dhClient,    // DHInventoryLister ← NEW
	ctx,
)
```

If `dhClient` is nil, pass `nil` — the handler's nil check handles it.

- [ ] **Step 9: Build and verify compilation**

Run: `cd /workspace/.worktrees/guarzo-workflow && go build ./...`
Expected: no errors

- [ ] **Step 10: Run all tests**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./... 2>&1 | tail -30`
Expected: all PASS (existing tests may need mock updates for the new constructor parameter — fix any that fail)

- [ ] **Step 11: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns.go internal/adapters/httpserver/handlers/campaigns_imports.go internal/domain/campaigns/service.go cmd/slabledger/main.go
git commit -m "feat(dh): trigger listed + channel sync on cert import"
```

---

### Task 6: Update inventory poll scheduler for new status values

The inventory poll currently hardcodes `Status: "active"` in its filter. DH is changing `active` → `listed` and adding `in_stock`. Update the poll to fetch both statuses.

**Files:**
- Modify: `internal/adapters/scheduler/dh_inventory_poll.go`

- [ ] **Step 1: Update `fetchAllPages` status filter**

In `dh_inventory_poll.go` line 181, change the hardcoded `"active"` status to empty string (fetch all statuses, let the poll handle both `in_stock` and `listed`):

```go
resp, err := s.client.ListInventory(ctx, dh.InventoryFilters{
	UpdatedSince: since,
	Page:         page,
	PerPage:      100,
})
```

Remove the `Status: "active"` line entirely — we want updates for all inventory items regardless of status.

- [ ] **Step 2: Add DH inventory status to the `DHFieldsUpdate`**

In `internal/domain/campaigns/repository.go`, add a `Status` field to `DHFieldsUpdate`:

```go
type DHFieldsUpdate struct {
	CardID            int
	InventoryID       int
	CertStatus        string
	ListingPriceCents int
	ChannelsJSON      string
	DHStatus          string // "in_stock" or "listed"
}
```

- [ ] **Step 3: Pass inventory status through in the poll**

In `dh_inventory_poll.go` line 141, add the DH item status to the update:

```go
if updateErr := s.updater.UpdatePurchaseDHFields(ctx, purchaseID, campaigns.DHFieldsUpdate{
	CardID:            item.DHCardID,
	InventoryID:       item.DHInventoryID,
	CertStatus:        dh.CertStatusMatched,
	ListingPriceCents: item.ListingPriceCents,
	ChannelsJSON:      channelsJSON,
	DHStatus:          item.Status,
}); updateErr != nil {
```

- [ ] **Step 4: Update SQLite repo to persist DHStatus**

Find the `UpdatePurchaseDHFields` implementation in the SQLite storage layer and add `dh_status` to the UPDATE query. This requires checking the existing migration — if a `dh_status` column doesn't exist, create a migration.

Search for the column first:

Run: `grep -r "dh_status" internal/adapters/storage/sqlite/`

If not found, create migration `000031_add_dh_status.up.sql`:

```sql
ALTER TABLE purchases ADD COLUMN dh_status TEXT NOT NULL DEFAULT '';
```

And `000031_add_dh_status.down.sql`:

```sql
ALTER TABLE purchases DROP COLUMN dh_status;
```

Update the `UpdatePurchaseDHFields` SQL query to include `dh_status = ?`.

- [ ] **Step 5: Add `DHStatus` to Purchase domain type**

In `internal/domain/campaigns/types.go`, add to the Purchase struct (after the existing DH fields):

```go
DHStatus            string    // "in_stock" or "listed" — DH inventory status
```

- [ ] **Step 6: Build and verify compilation**

Run: `cd /workspace/.worktrees/guarzo-workflow && go build ./...`
Expected: no errors

- [ ] **Step 7: Run all tests**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./... 2>&1 | tail -30`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/scheduler/dh_inventory_poll.go internal/domain/campaigns/repository.go internal/domain/campaigns/types.go internal/adapters/storage/sqlite/
git commit -m "feat(dh): update inventory poll for in_stock/listed statuses, add dh_status tracking"
```

---

### Task 7: Update existing test expectations

Existing tests reference `"active"` and `"pending"` status values. Update them to use the new constants.

**Files:**
- Modify: `internal/adapters/clients/dh/inventory_test.go`
- Modify: `internal/adapters/scheduler/dh_inventory_poll_test.go` (if exists)

- [ ] **Step 1: Update `TestClient_ListInventory` status values**

In `inventory_test.go`, update the `"active inventory with channels"` test case:
- Change `filters: InventoryFilters{Status: "active"}` → `filters: InventoryFilters{Status: InventoryStatusListed}`
- Change `"status": "active"` in the JSON → `"status": "listed"`
- Change `Status: "active"` in `wantFirst` → `Status: InventoryStatusListed`
- Update the server handler assertion: `require.Equal(t, "active", ...)` → `require.Equal(t, InventoryStatusListed, ...)`

- [ ] **Step 2: Check for and update inventory poll scheduler tests**

Run: `ls internal/adapters/scheduler/dh_inventory_poll_test.go`

If it exists, update any `"active"` status references to use `dh.InventoryStatusListed`.

- [ ] **Step 3: Run all tests**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test ./... 2>&1 | tail -30`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/clients/dh/inventory_test.go internal/adapters/scheduler/
git commit -m "test(dh): update status value expectations from active/pending to listed/in_stock"
```

---

### Task 8: Compile-time interface checks and final verification

Add compile-time checks that `dh.Client` satisfies the new interfaces, and run the full test suite.

**Files:**
- Modify: `internal/adapters/scheduler/dh_inventory_poll.go` (already has one)
- Modify: `internal/adapters/httpserver/handlers/dh_handler.go`
- Modify: `internal/adapters/httpserver/handlers/campaigns.go`

- [ ] **Step 1: Add compile-time checks**

In `dh_handler.go`, add at the bottom:

```go
// Compile-time checks.
var _ DHInventoryPusher = (*dh.Client)(nil)
```

In `campaigns.go` (or `campaigns_imports.go`), add:

```go
// Compile-time checks.
var _ DHInventoryLister = (*dh.Client)(nil)
```

- [ ] **Step 2: Run full test suite with race detection**

Run: `cd /workspace/.worktrees/guarzo-workflow && go test -race -timeout 10m ./... 2>&1 | tail -30`
Expected: all PASS, no races

- [ ] **Step 3: Run quality checks**

Run: `cd /workspace/.worktrees/guarzo-workflow && make check`
Expected: PASS (lint, imports, file sizes)

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_handler.go internal/adapters/httpserver/handlers/campaigns.go
git commit -m "chore(dh): add compile-time interface satisfaction checks"
```
