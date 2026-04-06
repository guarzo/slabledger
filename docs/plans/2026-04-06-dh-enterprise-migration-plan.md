# DH Enterprise API Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all DH integration API calls with enterprise API equivalents, eliminating the integration API key dependency and fixing 100% unmatched items in production.

**Architecture:** The DH client (`internal/adapters/clients/dh/`) drops its integration-key auth layer (`get`/`post`/`apiKey`/`Available`) and uses enterprise Bearer auth for everything. Match callers switch from title-based `Match()` to structured `ResolveCert()`. MarketData callers switch to `CardLookup()`. Suggestions switches auth internally.

**Tech Stack:** Go 1.26, hexagonal architecture, table-driven tests

---

### Task 1: Add card name cleaning helper

**Files:**
- Create: `internal/domain/campaigns/dh_helpers.go`
- Create: `internal/domain/campaigns/dh_helpers_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/campaigns/dh_helpers_test.go
package campaigns

import "testing"

func TestCleanCardNameForDH(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVariant string
	}{
		{"GENGAR-HOLO", "Gengar", "Holo"},
		{"CHARIZARD-REVERSE HOLO", "Charizard", "Reverse Holo"},
		{"PIKACHU", "Pikachu", ""},
		{"DRAGONITE 1ST EDITION", "Dragonite 1st Edition", ""},
		{"VENUSAUR-HOLO CD PROMO", "Venusaur CD Promo", "Holo"},
		{"M GENGAR EX", "M Gengar Ex", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			name, variant := CleanCardNameForDH(tc.input)
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if variant != tc.wantVariant {
				t.Errorf("variant = %q, want %q", variant, tc.wantVariant)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/campaigns/ -run TestCleanCardNameForDH -v`
Expected: FAIL — `CleanCardNameForDH` not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/domain/campaigns/dh_helpers.go
package campaigns

import "strings"

// CleanCardNameForDH strips holo suffixes from PSA-style card names and
// returns the cleaned name (title-cased) plus a variant hint for DH cert resolution.
func CleanCardNameForDH(raw string) (name, variant string) {
	if raw == "" {
		return "", ""
	}

	s := raw
	switch {
	case strings.HasSuffix(s, "-REVERSE HOLO"):
		s = strings.TrimSuffix(s, "-REVERSE HOLO")
		variant = "Reverse Holo"
	case strings.HasSuffix(s, "-HOLO"):
		s = strings.TrimSuffix(s, "-HOLO")
		variant = "Holo"
	}

	name = strings.Join(strings.Fields(toTitleCase(s)), " ")
	return name, variant
}

// toTitleCase converts "DRAGONITE 1ST EDITION" → "Dragonite 1st Edition".
func toTitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/campaigns/ -run TestCleanCardNameForDH -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/dh_helpers.go internal/domain/campaigns/dh_helpers_test.go
git commit -m "feat(dh): add CleanCardNameForDH helper for enterprise cert resolution"
```

---

### Task 2: Add CardLookup method and enterprise Suggestions to DH client

**Files:**
- Modify: `internal/adapters/clients/dh/types.go` — add `CardLookupResponse` type
- Modify: `internal/adapters/clients/dh/client.go` — add `CardLookup`, change `Suggestions` to enterprise auth
- Modify: `internal/adapters/clients/dh/client_test.go` — add tests for new methods

- [ ] **Step 1: Add CardLookupResponse type to types.go**

Add after the existing `MarketDataResponse` type:

```go
// CardLookupResponse is returned from GET /enterprise/cards/lookup.
type CardLookupResponse struct {
	Card       CardLookupCard       `json:"card"`
	MarketData CardLookupMarketData `json:"market_data"`
}

// CardLookupCard is the card identity from enterprise lookup.
type CardLookupCard struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	SetName            string `json:"set_name"`
	Number             string `json:"number"`
	Rarity             string `json:"rarity"`
	Language           string `json:"language"`
	Era                string `json:"era"`
	Year               string `json:"year"`
	Artist             string `json:"artist"`
	ImageURL           string `json:"image_url"`
	Slug               string `json:"slug"`
	PriceChartingID    string `json:"pricecharting_id"`
	TCGPlayerProductID *int   `json:"tcgplayer_product_id"`
}

// CardLookupMarketData is the market data from enterprise lookup.
type CardLookupMarketData struct {
	BestBid     *float64 `json:"best_bid"`
	BestAsk     *float64 `json:"best_ask"`
	Spread      *float64 `json:"spread"`
	LastSale    *float64 `json:"last_sale"`
	LastSaleDate *string `json:"last_sale_date"`
	LowPrice    *float64 `json:"low_price"`
	MidPrice    *float64 `json:"mid_price"`
	HighPrice   *float64 `json:"high_price"`
	ActiveBids  int      `json:"active_bids"`
	ActiveAsks  int      `json:"active_asks"`
	Volume24h   int      `json:"24h_volume"`
	Change24h   *float64 `json:"24h_change"`
	Change7d    *float64 `json:"7d_change"`
	Change30d   *float64 `json:"30d_change"`
}
```

- [ ] **Step 2: Add CardLookup method to client.go**

Add after the existing `ResolveCert` method:

```go
// CardLookup returns card details and market data from the enterprise API.
func (c *Client) CardLookup(ctx context.Context, cardID int) (*CardLookupResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/lookup?card_id=%d", c.baseURL, cardID)

	var resp CardLookupResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 3: Change Suggestions to use enterprise auth**

Replace the existing `Suggestions` method body:

```go
// Suggestions returns AI-generated buy/sell suggestions via the enterprise API.
func (c *Client) Suggestions(ctx context.Context) (*SuggestionsResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/suggestions", c.baseURL)

	var resp SuggestionsResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 4: Add CardLookup unit test to client_test.go**

Add after the existing tests:

```go
func TestClient_CardLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get(enterpriseAuthHeader) != "Bearer test_api_key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get(enterpriseAuthHeader))
		}
		if r.URL.Path != "/api/v1/enterprise/cards/lookup" {
			t.Errorf("expected path /api/v1/enterprise/cards/lookup, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("card_id") != "247" {
			t.Errorf("expected card_id=247, got %q", r.URL.Query().Get("card_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"card": {
				"id": 247,
				"name": "Charizard [1st Edition]",
				"set_name": "Pokemon Base Set",
				"number": "4",
				"rarity": "Holo Rare",
				"language": "en",
				"era": "WOTC",
				"year": "1999",
				"artist": "Mitsuhiro Arita",
				"image_url": "https://example.com/charizard.png",
				"slug": "charizard-1st-edition",
				"pricecharting_id": "pc-123",
				"tcgplayer_product_id": null
			},
			"market_data": {
				"best_bid": 12000.00,
				"best_ask": 15000.00,
				"spread": 3000.00,
				"last_sale": 14000.00,
				"last_sale_date": "2026-04-01",
				"low_price": 11000.00,
				"mid_price": 13500.00,
				"high_price": 16000.00,
				"active_bids": 5,
				"active_asks": 8,
				"24h_volume": 2,
				"24h_change": 3.5,
				"7d_change": -1.2,
				"30d_change": 8.0
			}
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.CardLookup(context.Background(), 247)
	if err != nil {
		t.Fatalf("CardLookup() error = %v", err)
	}
	if resp.Card.ID != 247 {
		t.Errorf("Card.ID = %d, want 247", resp.Card.ID)
	}
	if resp.Card.Name != "Charizard [1st Edition]" {
		t.Errorf("Card.Name = %q, want Charizard [1st Edition]", resp.Card.Name)
	}
	if resp.MarketData.MidPrice == nil || *resp.MarketData.MidPrice != 13500.00 {
		t.Errorf("MarketData.MidPrice = %v, want 13500.00", resp.MarketData.MidPrice)
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/clients/dh/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/dh/types.go internal/adapters/clients/dh/client.go internal/adapters/clients/dh/client_test.go
git commit -m "feat(dh): add CardLookup method, switch Suggestions to enterprise auth"
```

---

### Task 3: Switch DH handler interfaces and bulk match to cert resolution

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_handler.go` — replace `DHMatchClient` with `DHCertResolver`
- Modify: `internal/adapters/httpserver/handlers/dh_match_handler.go` — rewrite `runBulkMatch` to use `ResolveCert`

- [ ] **Step 1: Replace DHMatchClient interface in dh_handler.go**

Replace lines 14-18:
```go
// DHCertResolver resolves PSA certs to DH card IDs via the enterprise API.
type DHCertResolver interface {
	ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}
```

Update the struct field at line 78:
```go
certResolver      DHCertResolver
```

Update the constructor parameter at line 103:
```go
certResolver DHCertResolver,
```

And the assignment at line 123:
```go
certResolver:      certResolver,
```

Remove the compile-time check at line 146 if it references `DHMatchClient`. Keep others.

- [ ] **Step 2: Rewrite runBulkMatch in dh_match_handler.go**

Replace the `uniqueCardIdentities` call and iteration. The new flow iterates over purchases directly (each has a unique cert number):

Replace `runBulkMatch` (lines 66-128):

```go
// runBulkMatch processes unsold purchases against DH cert resolution, logging results.
func (h *DHHandler) runBulkMatch(ctx context.Context, purchases []campaigns.Purchase, mappedSet map[string]string) {
	var matched, skipped, notFound, failed int
	var matchedCards []matchedCard

	for _, p := range purchases {
		if ctx.Err() != nil {
			break
		}

		key := p.DHCardKey()

		// Already mapped — skip.
		if mappedSet[key] != "" {
			skipped++
			continue
		}

		// Need a cert number for resolution.
		if p.CertNumber == "" {
			continue
		}

		cardName, variant := campaigns.CleanCardNameForDH(p.CardName)
		resp, err := h.certResolver.ResolveCert(ctx, dh.CertResolveRequest{
			CertNumber: p.CertNumber,
			CardName:   cardName,
			SetName:    p.SetName,
			CardNumber: p.CardNumber,
			Year:       p.CardYear,
			Variant:    variant,
		})
		if err != nil {
			h.logger.Warn(ctx, "bulk match: DH cert resolve failed",
				observability.String("cert", p.CertNumber), observability.Err(err))
			failed++
			continue
		}

		if resp.Status != dh.CertStatusMatched {
			notFound++
			if h.pushStatusUpdater != nil {
				if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); err != nil {
					h.logger.Warn(ctx, "bulk match: failed to set unmatched status",
						observability.String("purchaseID", p.ID), observability.Err(err))
				}
			}
			continue
		}

		externalID := strconv.Itoa(resp.DHCardID)
		if err := h.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Error(ctx, "bulk match: save external ID", observability.Err(err),
				observability.String("cert", p.CertNumber))
			failed++
			continue
		}
		matched++
		matchedCards = append(matchedCards, matchedCard{identity: p.ToCardIdentity(), dhCardID: resp.DHCardID})
		mappedSet[key] = externalID
	}

	h.logger.Info(ctx, "bulk match completed",
		observability.Int("total", len(purchases)),
		observability.Int("matched", matched),
		observability.Int("skipped", skipped),
		observability.Int("not_found", notFound),
		observability.Int("failed", failed))

	// Push newly matched cards to DH inventory as in_stock.
	if h.inventoryPusher != nil && len(matchedCards) > 0 {
		h.pushMatchedToDH(ctx, purchases, matchedCards)
	}
}
```

- [ ] **Step 3: Update HandleBulkMatch to pass purchases and mappedSet only**

Replace `HandleBulkMatch` to no longer call `uniqueCardIdentities` — pass purchases directly:

```go
func (h *DHHandler) HandleBulkMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}

	if !h.bulkMatchMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "already_running"})
		return
	}

	ctx := r.Context()

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.bulkMatchMu.Unlock()
		h.logger.Error(ctx, "bulk match: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	mappedSet, err := h.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		h.bulkMatchMu.Unlock()
		h.logger.Error(ctx, "bulk match: load mapped set", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}

	h.bulkMatchRunning.Store(true)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer h.bulkMatchMu.Unlock()
		defer h.bulkMatchRunning.Store(false)
		ctx, cancel := context.WithCancel(h.baseCtx)
		defer cancel()
		h.runBulkMatch(ctx, purchases, mappedSet)
	}()
}
```

Remove the `uniqueCardIdentities` method entirely — it's no longer used.

- [ ] **Step 4: Run tests and build**

Run: `go build ./...`
Expected: Build errors in main.go wiring (expected — we'll fix in Task 6)

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_handler.go internal/adapters/httpserver/handlers/dh_match_handler.go
git commit -m "feat(dh): switch bulk match from Match API to enterprise cert resolution"
```

---

### Task 4: Switch push scheduler and inline listing to cert resolution

**Files:**
- Modify: `internal/adapters/scheduler/dh_push.go` — replace `DHPushMatchClient` with cert resolver
- Modify: `internal/adapters/httpserver/handlers/campaigns_dh_listing.go` — replace inline match
- Modify: `internal/adapters/httpserver/handlers/campaigns.go` — rename option

- [ ] **Step 1: Update push scheduler interfaces and processPurchase in dh_push.go**

Replace `DHPushMatchClient` interface (lines 26-29):
```go
// DHPushCertResolver resolves PSA certs to DH card IDs.
type DHPushCertResolver interface {
	ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}
```

Rename the field in `DHPushScheduler` struct (line 53):
```go
certResolver  DHPushCertResolver
```

Update constructor parameter (line 65) and assignment:
```go
certResolver DHPushCertResolver,
```
```go
certResolver:  certResolver,
```

Replace the match block in `processPurchase` (lines 168-191):
```go
	if !alreadyMapped {
		// Call DH Cert Resolution API.
		cardName, variant := campaigns.CleanCardNameForDH(p.CardName)
		resp, err := s.certResolver.ResolveCert(ctx, dh.CertResolveRequest{
			CertNumber: p.CertNumber,
			CardName:   cardName,
			SetName:    p.SetName,
			CardNumber: p.CardNumber,
			Year:       p.CardYear,
			Variant:    variant,
		})
		if err != nil {
			s.logger.Warn(ctx, "dh push: cert resolve error, leaving as pending",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			return "skipped"
		}

		if resp.Status != dh.CertStatusMatched {
			s.logger.Debug(ctx, "dh push: cert not matched, marking unmatched",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.String("status", resp.Status))
			if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); updateErr != nil {
				s.logger.Warn(ctx, "dh push: failed to set unmatched status",
					observability.String("purchaseID", p.ID),
					observability.Err(updateErr))
			}
			return "unmatched"
		}

		dhCardID = resp.DHCardID

		// Persist the mapping so future runs skip the cert resolve call.
		externalID := strconv.Itoa(dhCardID)
		if saveErr := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); saveErr != nil {
			s.logger.Warn(ctx, "dh push: failed to save external ID mapping",
				observability.String("purchaseID", p.ID),
				observability.Err(saveErr))
		} else {
			mappedSet[key] = externalID
		}
	}
```

Update the compile-time checks at the bottom:
```go
var _ DHPushCertResolver = (*dh.Client)(nil)
var _ DHPushInventoryPusher = (*dh.Client)(nil)
```

- [ ] **Step 2: Update inlineMatchAndPush in campaigns_dh_listing.go**

Replace the `inlineMatchAndPush` method. Replace the field name reference from `h.dhMatchClient` to `h.dhCertResolver`:

```go
func (h *CampaignsHandler) inlineMatchAndPush(ctx context.Context, p *campaigns.Purchase) int {
	cardName, variant := campaigns.CleanCardNameForDH(p.CardName)

	resp, err := h.dhCertResolver.ResolveCert(ctx, dh.CertResolveRequest{
		CertNumber: p.CertNumber,
		CardName:   cardName,
		SetName:    p.SetName,
		CardNumber: p.CardNumber,
		Year:       p.CardYear,
		Variant:    variant,
	})
	if err != nil {
		h.logger.Warn(ctx, "inline dh cert resolve failed",
			observability.String("cert", p.CertNumber), observability.Err(err))
		return 0
	}

	if resp.Status != dh.CertStatusMatched {
		if h.pushStatusUpdater != nil {
			if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusUnmatched); err != nil {
				h.logger.Warn(ctx, "inline dh resolve: failed to set unmatched status",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}
		return 0
	}

	dhCardID := resp.DHCardID

	if h.dhCardIDSaver != nil {
		externalID := strconv.Itoa(dhCardID)
		if err := h.dhCardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Warn(ctx, "inline dh resolve: failed to save card mapping",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}

	item := dh.InventoryItem{
		DHCardID:       dhCardID,
		CertNumber:     p.CertNumber,
		GradingCompany: dh.GraderPSA,
		Grade:          p.GradeValue,
		CostBasisCents: p.CLValueCents,
		Status:         dh.InventoryStatusInStock,
	}

	pushResp, pushErr := h.dhPusher.PushInventory(ctx, []dh.InventoryItem{item})
	if pushErr != nil {
		h.logger.Warn(ctx, "inline dh push failed",
			observability.String("cert", p.CertNumber), observability.Err(pushErr))
		return 0
	}

	for _, r := range pushResp.Results {
		if r.Status == "failed" || r.DHInventoryID == 0 {
			continue
		}

		if h.dhFieldsUpdater != nil {
			if err := h.dhFieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, campaigns.DHFieldsUpdate{
				CardID:            dhCardID,
				InventoryID:       r.DHInventoryID,
				CertStatus:        dh.CertStatusMatched,
				ListingPriceCents: r.AssignedPriceCents,
				ChannelsJSON:      dh.MarshalChannels(r.Channels),
				DHStatus:          campaigns.DHStatus(r.Status),
			}); err != nil {
				h.logger.Warn(ctx, "inline dh push: failed to persist DH fields",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		if h.pushStatusUpdater != nil {
			if err := h.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusMatched); err != nil {
				h.logger.Warn(ctx, "inline dh push: failed to set matched status",
					observability.String("cert", p.CertNumber), observability.Err(err))
			}
		}

		return r.DHInventoryID
	}

	return 0
}
```

Also update the nil check in `triggerDHListing` (line 43):
```go
if h.dhCertResolver != nil && h.dhPusher != nil {
```

- [ ] **Step 3: Update campaigns.go field and option**

In `internal/adapters/httpserver/handlers/campaigns.go`, rename the field (line 25):
```go
dhCertResolver    DHCertResolver      // optional: resolves certs against DH
```

Rename the option function (lines 47-49):
```go
// WithDHCertResolver enables DH cert resolution for inline push.
func WithDHCertResolver(c DHCertResolver) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhCertResolver = c }
}
```

Note: `DHCertResolver` interface is defined in `dh_handler.go` (same package), so no import needed.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/scheduler/dh_push.go internal/adapters/httpserver/handlers/campaigns_dh_listing.go internal/adapters/httpserver/handlers/campaigns.go
git commit -m "feat(dh): switch push scheduler and inline listing to cert resolution"
```

---

### Task 5: Switch MarketData callers to CardLookup

**Files:**
- Modify: `internal/adapters/clients/fusionprice/dh_adapter.go` — change interface + mapping
- Modify: `internal/adapters/clients/fusionprice/dh_adapter_test.go` — update mock + tests
- Modify: `internal/adapters/scheduler/dh_intelligence_refresh.go` — change method call
- Modify: `internal/adapters/clients/dh/convert.go` — update to accept `CardLookupResponse`

- [ ] **Step 1: Update convert.go to accept CardLookupResponse**

The `ConvertToIntelligence` function currently takes `*MarketDataResponse`. The enterprise `CardLookupResponse` has a different shape — but we still get recent sales, sentiment, etc. from the _per-card_ enterprise endpoints (price-history, recent-sales, insights). For now, keep `ConvertToIntelligence` working with the existing `MarketDataResponse` type because the intelligence refresh scheduler calls dedicated per-card endpoints. The fusion adapter only needs `RecentSales` from market data, which the enterprise `/cards/lookup` does NOT provide.

**Decision:** The enterprise `cards/lookup` provides market_data (bid/ask/price) but NOT recent sales, sentiment, grading ROI, population, etc. Those come from separate enterprise endpoints (`/cards/{id}/recent-sales`, `/cards/{id}/insights`, etc.). For this migration:
- The **fusion adapter** needs recent sales → keep using the per-card enterprise endpoints. Change `MarketData()` to call `CardLookup()` for the basic market data check, but we need to also add a `RecentSales` call.
- The **intelligence refresh** scheduler uses `ConvertToIntelligence` which needs recent sales, sentiment, etc.

Actually, looking more carefully at the current `MarketDataResponse` — it bundles everything (price history, recent sales, sentiment, ROI, etc.) from the integration `market_data` endpoint. The enterprise API splits these across multiple endpoints. The simplest migration path is:

**For the fusion adapter:** The adapter only uses `RecentSales` from `MarketDataResponse`. The enterprise `cards/{card_id}/recent-sales` endpoint returns exactly this. So we change the adapter to call a new `RecentSales` method instead.

**For the intelligence refresh:** It calls `ConvertToIntelligence` which needs recent sales, sentiment, forecast, ROI, population, and insights — all available from separate enterprise endpoints. But refactoring this to call 5 separate endpoints is out of scope for this PR.

**Simpler approach:** Keep `MarketDataResponse` as a composite type, and have the client method assemble it from enterprise endpoints. This preserves the existing consumer code.

**Simplest approach:** The enterprise `cards/lookup` returns basic market data. Add `RecentSales` as a separate client method. The fusion adapter calls `RecentSales`. The intelligence refresh calls multiple endpoints internally.

**Actually simplest:** Check if the integration `market_data` endpoint still works (it uses the integration key which is still configured). If it does, we can defer this migration and focus on the matching fix which is the urgent production issue.

Let me re-examine — the user wants ALL integration endpoints removed. Let me check what the enterprise API provides per-card.

From the OpenAPI spec, these enterprise endpoints exist:
- `GET /enterprise/cards/{card_id}/recent-sales` — recent sales data
- `GET /enterprise/cards/{card_id}/insights` — AI insights
- `GET /enterprise/cards/{card_id}/price-history` — price history
- `GET /enterprise/cards/{card_id}/grading-roi` — grading ROI
- `GET /enterprise/cards/{card_id}/graded-sales-analytics` — graded analytics

The intelligence refresh needs all of these to populate `ConvertToIntelligence`. Instead of calling 5 endpoints per card, the better approach is to add a `FetchFullMarketData` method that calls the needed endpoints and assembles a `MarketDataResponse`.

But that's a lot of new code. For this task, let's:
1. Add a `RecentSales` client method for the fusion adapter (it only needs sales)
2. For the intelligence refresh, add a `FetchCardMarketData` method that calls the subset of endpoints needed

- [ ] **Step 1a: Add RecentSales client method**

Add to `client.go`:

```go
// RecentSales returns recent sales for a card from the enterprise API.
func (c *Client) RecentSales(ctx context.Context, cardID int) ([]RecentSale, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/%d/recent-sales", c.baseURL, cardID)

	var resp struct {
		Sales []RecentSale `json:"sales"`
	}
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return resp.Sales, nil
}
```

- [ ] **Step 1b: Update fusion adapter interface and implementation**

In `dh_adapter.go`, change the interface:

```go
// DHMarketDataClient is the subset of the dh.Client used by the adapter.
type DHMarketDataClient interface {
	RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}
```

Update `FetchFusionData` to use `RecentSales`:

Replace lines 98-127:
```go
	// Step 2: Fetch recent sales.
	cardIDInt, convErr := strconv.Atoi(dhCardID)
	if convErr != nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: invalid card ID %q: %w", dhCardID, convErr)
	}

	sales, err := a.client.RecentSales(ctx, cardIDInt)
	if err != nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: recent sales failed for card_id=%s: %w", dhCardID, err)
	}

	if len(sales) == 0 {
		return nil, &fusion.ResponseMeta{StatusCode: 200}, nil
	}

	// Step 3: Convert recent sales to grade data.
	gradeData := convertDHSalesToGradeData(sales)

	return &fusion.FetchResult{
		GradeData: gradeData,
	}, &fusion.ResponseMeta{StatusCode: 200}, nil
```

Add `"strconv"` to the imports. Remove the intelligence store step from the adapter — intelligence will be handled by its own scheduler.

- [ ] **Step 1c: Update fusion adapter tests**

Update the mock and tests in `dh_adapter_test.go`:

```go
type mockDHMarketDataClient struct {
	RecentSalesFn func(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}

func (m *mockDHMarketDataClient) RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
	return m.RecentSalesFn(ctx, cardID)
}
```

Update `TestDHAdapter_FetchFusionData_WithSales`:
```go
func TestDHAdapter_FetchFusionData_WithSales(t *testing.T) {
	client := &mockDHMarketDataClient{
		RecentSalesFn: func(_ context.Context, cardID int) ([]dh.RecentSale, error) {
			if cardID != 12345 {
				t.Fatalf("unexpected cardID: %d", cardID)
			}
			return []dh.RecentSale{
				{SoldAt: "2026-03-15T10:00:00Z", GradingCompany: "PSA", Grade: "10", Price: 500.00, Platform: "eBay"},
				{SoldAt: "2026-03-14T10:00:00Z", GradingCompany: "PSA", Grade: "10", Price: 480.00, Platform: "eBay"},
				{SoldAt: "2026-03-13T10:00:00Z", GradingCompany: "PSA", Grade: "9", Price: 250.00, Platform: "TCGPlayer"},
				{SoldAt: "2026-03-12T10:00:00Z", GradingCompany: "BGS", Grade: "10", Price: 700.00, Platform: "eBay"},
				{SoldAt: "2026-03-11T10:00:00Z", GradingCompany: "PSA", Grade: "8", Price: 120.00, Platform: "eBay"},
				{SoldAt: "2026-03-10T10:00:00Z", GradingCompany: "PSA", Grade: "7", Price: 80.00, Platform: "eBay"},
				{SoldAt: "2026-03-09T10:00:00Z", GradingCompany: "PSA", Grade: "6", Price: 50.00, Platform: "eBay"},
				{SoldAt: "2026-03-08T10:00:00Z", GradingCompany: "BGS", Grade: "9.5", Price: 300.00, Platform: "eBay"},
			}, nil
		},
	}

	idLookup := &mockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, provider string) (string, error) {
			if provider != pricing.SourceDH {
				t.Fatalf("unexpected provider: %s", provider)
			}
			return "12345", nil
		},
	}

	adapter := NewDHAdapter(client, idLookup, nil)
	// ... rest of assertions stay the same, minus intelligence store check
```

Update the other test mocks similarly (NoMapping, NoData, LookupError, SkipsZeroPriceSales).

- [ ] **Step 2: Update intelligence refresh scheduler**

In `dh_intelligence_refresh.go`, the scheduler calls `s.dhClient.MarketData(ctx, entry.DHCardID)`. The enterprise equivalent would need multiple calls. For now, the simplest approach is to add a `MarketDataEnterprise` method on the client that aggregates the enterprise endpoints.

Add to `client.go`:

```go
// MarketDataEnterprise fetches market data from multiple enterprise endpoints
// and assembles a MarketDataResponse compatible with the existing consumer code.
func (c *Client) MarketDataEnterprise(ctx context.Context, cardID int) (*MarketDataResponse, error) {
	// Fetch card lookup for basic data
	lookup, err := c.CardLookup(ctx, cardID)
	if err != nil {
		return nil, err
	}

	resp := &MarketDataResponse{
		HasData:   true,
		CardID:    lookup.Card.ID,
		CardTitle: lookup.Card.Name,
	}

	// Set current price from mid_price
	if lookup.MarketData.MidPrice != nil {
		resp.CurrentPrice = *lookup.MarketData.MidPrice
	}
	if lookup.MarketData.LowPrice != nil {
		resp.PeriodLow = *lookup.MarketData.LowPrice
	}
	if lookup.MarketData.HighPrice != nil {
		resp.PeriodHigh = *lookup.MarketData.HighPrice
	}

	// Fetch recent sales
	sales, err := c.RecentSales(ctx, cardID)
	if err == nil {
		resp.RecentSales = sales
	}

	return resp, nil
}
```

Update `dh_intelligence_refresh.go` line 94:
```go
cardIDInt, convErr := strconv.Atoi(entry.DHCardID)
if convErr != nil {
	s.logger.Warn(ctx, "invalid DH card ID",
		observability.String("dh_card_id", entry.DHCardID),
		observability.Err(convErr))
	failed++
	continue
}
resp, fetchErr := s.dhClient.MarketDataEnterprise(ctx, cardIDInt)
```

Add `"strconv"` to imports.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/adapters/clients/fusionprice/ -v`
Run: `go test ./internal/adapters/clients/dh/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/clients/dh/client.go internal/adapters/clients/fusionprice/dh_adapter.go internal/adapters/clients/fusionprice/dh_adapter_test.go internal/adapters/scheduler/dh_intelligence_refresh.go internal/adapters/clients/dh/convert.go
git commit -m "feat(dh): switch MarketData callers to enterprise API endpoints"
```

---

### Task 6: Config cleanup and wiring

**Files:**
- Modify: `internal/platform/config/types.go` — remove `DHKey`
- Modify: `internal/platform/config/loader.go` — remove `DH_INTEGRATION_API_KEY` loading
- Modify: `.env.example` — remove `DH_INTEGRATION_API_KEY`
- Modify: `internal/adapters/clients/dh/client.go` — remove `apiKey`, `Available()`, `get()`, `post()`, `apiKeyHeader`
- Modify: `cmd/slabledger/main.go` — fix wiring
- Modify: `cmd/slabledger/init.go` — fix wiring
- Modify: `cmd/slabledger/server.go` — rename `DHMatchClient` field
- Modify: `cmd/slabledger/admin_analyze.go` — fix DH client construction

- [ ] **Step 1: Remove integration key from config**

In `config/types.go`, remove the `DHKey` field (line 133):
```go
// Delete: DHKey string // DH_INTEGRATION_API_KEY
```

In `config/loader.go`, remove line 398:
```go
// Delete: cfg.Adapters.DHKey = os.Getenv("DH_INTEGRATION_API_KEY")
```

In `.env.example`, remove line 205:
```go
// Delete: DH_INTEGRATION_API_KEY=""
```

- [ ] **Step 2: Strip integration auth from DH client**

In `client.go`:
- Remove `apiKeyHeader` constant (line 22)
- Remove `apiKey` field from `Client` struct (line 53)
- Remove `apiKey` parameter from `NewClient` (line 64), update: `func NewClient(baseURL string, opts ...ClientOption)`
- Remove `Available()` method (lines 86-88)
- Remove `get()` method (lines 160-190)
- Remove `post()` method (lines 294-330)
- Remove `Match()` method (lines 123-136)
- Remove `Search()` method (lines 108-121)

Keep all `*Enterprise` methods, `getEnterprise`, `doEnterprise`, `postEnterprise`, `patchEnterprise`, `deleteEnterprise`.

- [ ] **Step 3: Update NewClient callers**

In `main.go` (lines 242-248), change:
```go
if cfg.Adapters.DHEnterpriseKey != "" && cfg.Adapters.DHBaseURL != "" {
	dhClient = dh.NewClient(
		cfg.Adapters.DHBaseURL,
		dh.WithLogger(logger),
		dh.WithRateLimitRPS(cfg.DH.RateLimitRPS),
		dh.WithEnterpriseKey(cfg.Adapters.DHEnterpriseKey),
	)
```

In `admin_analyze.go` (lines 118-124), same pattern — remove `cfg.Adapters.DHKey` from the condition and parameter.

- [ ] **Step 4: Fix Available() → EnterpriseAvailable() in wiring**

In `main.go` line 335:
```go
if dhClient != nil && dhClient.EnterpriseAvailable() {
```

In `init.go` line 70:
```go
if dhClient != nil && dhClient.EnterpriseAvailable() {
```

In `scheduler/builder.go` lines 340, 353, 395:
```go
if deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && ...
```

- [ ] **Step 5: Rename DHMatchClient → DHCertResolver in server.go**

In `server.go` line 55:
```go
DHCertResolver        handlers.DHCertResolver     // optional: inline DH cert resolution
```

Line 202-203:
```go
if deps.DHCertResolver != nil {
	opts = append(opts, handlers.WithDHCertResolver(deps.DHCertResolver))
}
```

In `main.go` line 519:
```go
deps.DHCertResolver = dhClient
```

- [ ] **Step 6: Build and test**

Run: `go build ./...`
Run: `go test ./... -count=1`
Expected: Build succeeds. Tests pass.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor(dh): remove integration API key, wire all callers to enterprise auth"
```

---

### Task 7: Dead code removal

**Files:**
- Modify: `internal/adapters/clients/dh/types.go` — remove `MatchRequest`, `MatchResponse`, `SearchResponse`, `SearchCard`
- Modify: `internal/adapters/clients/dh/client_test.go` — remove `TestClient_Match`, `TestClient_Search`, `TestClient_NotAvailable`, `TestClient_Suggestions` (now enterprise auth)
- Modify: `internal/domain/campaigns/types.go` — remove `DHMatchConfidenceThreshold`, `BuildDHMatchTitle`
- Delete: Anything else unused

- [ ] **Step 1: Remove dead types from types.go**

Remove `MatchRequest` (lines 19-23), `MatchResponse` (lines 26-32), `SearchResponse` (lines 5-7), `SearchCard` (lines 9-15).

- [ ] **Step 2: Remove dead constants and functions from campaigns/types.go**

Remove `DHMatchConfidenceThreshold` (line 213) and `BuildDHMatchTitle` (lines 216-230).

- [ ] **Step 3: Remove dead tests from client_test.go**

Remove `TestClient_Match`, `TestClient_Search`, `TestClient_NotAvailable`. Update `TestClient_Suggestions` to expect enterprise auth header instead of integration API key.

- [ ] **Step 4: Check for compile errors**

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -count=1 -race -timeout 10m`
Expected: All tests pass

- [ ] **Step 6: Run quality checks**

Run: `make check`
Expected: Lint, architecture checks, and file size checks pass

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor(dh): remove dead integration API types, methods, and tests"
```

---

### Task 8: Final verification

- [ ] **Step 1: Verify no integration API references remain**

Run: `grep -r "integrations/match\|integrations/catalog\|integrations/market_data\|integrations/suggestions\|DH_INTEGRATION_API_KEY\|apiKeyHeader\|X-Integration-API-Key" --include="*.go" internal/ cmd/`

Expected: No matches (except possibly comments/docs)

- [ ] **Step 2: Verify enterprise auth is used everywhere**

Run: `grep -r "Available()" --include="*.go" internal/adapters/clients/dh/`

Expected: Only `EnterpriseAvailable()` remains.

- [ ] **Step 3: Run full test suite one more time**

Run: `go test ./... -count=1 -race -timeout 10m`
Expected: All pass

- [ ] **Step 4: Run frontend checks**

Run: `cd web && npm run build && npm test`
Expected: No frontend impact (backend-only change)
