# Card Ladder API Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automate Card Ladder value refresh and sales comp retrieval via CL's internal JSON API, replacing the manual CSV export/upload workflow.

**Architecture:** New `clients/cardladder/` API client with Firebase Auth token management. Scheduler fetches collection values daily via CL's Cloud Run search API. Phase 2 adds sales comp storage per card. All data flows through existing `cl_value_cents` and `cl_value_history` paths.

**Tech Stack:** Go, Firebase Auth REST API, `httpx.Client`, `rate.Limiter`, SQLite migrations, AES-256-GCM token encryption.

**Design Spec:** `docs/superpowers/specs/2026-03-29-cardladder-api-integration-design.md`

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/adapters/clients/cardladder/types.go` | API response structs |
| Create | `internal/adapters/clients/cardladder/auth.go` | Firebase Auth login + token refresh |
| Create | `internal/adapters/clients/cardladder/auth_test.go` | Auth tests with httptest |
| Create | `internal/adapters/clients/cardladder/client.go` | HTTP client with rate limiting + token management |
| Create | `internal/adapters/clients/cardladder/client_test.go` | Client tests with httptest |
| Create | `internal/adapters/storage/sqlite/cardladder_store.go` | Config + mapping persistence |
| Create | `internal/adapters/storage/sqlite/cardladder_store_test.go` | Store tests |
| Create | `internal/adapters/storage/sqlite/migrations/000022_cardladder.up.sql` | Schema: config + mappings |
| Create | `internal/adapters/storage/sqlite/migrations/000022_cardladder.down.sql` | Rollback |
| Create | `internal/adapters/scheduler/cardladder_refresh.go` | Daily CL value refresh scheduler |
| Create | `internal/adapters/httpserver/handlers/cardladder.go` | Admin endpoints |
| Modify | `internal/platform/config/types.go` | Add `CardLadderConfig` struct |
| Modify | `internal/platform/config/defaults.go` | Add CL defaults |
| Modify | `internal/platform/config/loader.go` | Load CL env vars |
| Modify | `internal/adapters/scheduler/builder.go` | Register CL scheduler |
| Modify | `cmd/slabledger/init.go` | Wire CL store + scheduler deps |
| Modify | `cmd/slabledger/server.go` | Add `CardLadderHandler` to `ServerDependencies` |
| Modify | `internal/adapters/httpserver/router.go` | Register CL admin routes |
| Create | `internal/adapters/storage/sqlite/migrations/000023_cl_sales_comps.up.sql` | Phase 2: sales comps table |
| Create | `internal/adapters/storage/sqlite/migrations/000023_cl_sales_comps.down.sql` | Phase 2: rollback |
| Create | `internal/adapters/storage/sqlite/cl_sales_store.go` | Phase 2: sales comp persistence |
| Create | `internal/adapters/httpserver/handlers/sales_comps.go` | Phase 2: sales comps endpoint |

---

### Task 1: API Response Types

**Files:**
- Create: `internal/adapters/clients/cardladder/types.go`

- [ ] **Step 1: Create types file**

```go
package cardladder

import "time"

// SearchResponse is the envelope returned by the Cloud Run search API.
type SearchResponse[T any] struct {
	Hits      []T `json:"hits"`
	TotalHits int `json:"totalHits"`
}

// CollectionCard represents one card from the collectioncards index.
type CollectionCard struct {
	CollectionCardID   string  `json:"collectionCardId"`
	CollectionID       string  `json:"collectionId"`
	Category           string  `json:"category"`
	Condition          string  `json:"condition"`    // e.g. "PSA 9"
	Year               string  `json:"year"`
	Number             string  `json:"number"`
	Set                string  `json:"set"`
	Variation          string  `json:"variation"`
	Label              string  `json:"label"`
	Player             string  `json:"player"`
	Image              string  `json:"image"`
	ImageBack          string  `json:"imageBack"`
	CurrentValue       float64 `json:"currentValue"` // dollars
	Investment         float64 `json:"investment"`
	Profit             float64 `json:"profit"`
	WeeklyPctChange    float64 `json:"weeklyPercentChange"`
	MonthlyPctChange   float64 `json:"monthlyPercentChange"`
	DateAdded          string  `json:"dateAdded"`
	HasQuantityAvail   bool    `json:"hasQuantityAvailable"`
	Sold               bool    `json:"sold"`
}

// SaleComp represents one sold listing from the salesarchive index.
type SaleComp struct {
	ItemID          string  `json:"itemId"`
	Date            string  `json:"date"`
	Price           float64 `json:"price"` // dollars
	Platform        string  `json:"platform"`
	ListingType     string  `json:"listingType"`
	Seller          string  `json:"seller"`
	Feedback        int     `json:"feedback"`
	URL             string  `json:"url"`
	SlabSerial      string  `json:"slabSerial"`
	CardDescription string  `json:"cardDescription"`
	GemRateID       string  `json:"gemRateId"`
	Condition       string  `json:"condition"` // e.g. "g8"
	GradingCompany  string  `json:"gradingCompany"`
}

// FirebaseAuthResponse is returned by the Firebase signInWithPassword endpoint.
type FirebaseAuthResponse struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"` // seconds as string
	LocalID      string `json:"localId"`
}

// FirebaseRefreshResponse is returned by the Firebase token refresh endpoint.
type FirebaseRefreshResponse struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"` // seconds as string
}

// TokenState holds the current auth token and its expiry.
type TokenState struct {
	IDToken   string
	ExpiresAt time.Time
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /workspace && go build ./internal/adapters/clients/cardladder/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/clients/cardladder/types.go
git commit -m "feat(cardladder): add API response types"
```

---

### Task 2: Firebase Auth Module

**Files:**
- Create: `internal/adapters/clients/cardladder/auth.go`
- Create: `internal/adapters/clients/cardladder/auth_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cardladder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFirebaseLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts:signInWithPassword" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-api-key" {
			t.Fatalf("unexpected api key: %s", r.URL.Query().Get("key"))
		}
		json.NewEncoder(w).Encode(FirebaseAuthResponse{
			IDToken:      "test-id-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    "3600",
		})
	}))
	defer server.Close()

	auth := NewFirebaseAuth("test-api-key", WithAuthBaseURL(server.URL))
	resp, err := auth.Login(context.Background(), "user@example.com", "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.IDToken != "test-id-token" {
		t.Errorf("IDToken = %q, want %q", resp.IDToken, "test-id-token")
	}
	if resp.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", resp.RefreshToken, "test-refresh-token")
	}
}

func TestFirebaseRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(FirebaseRefreshResponse{
			IDToken:      "new-id-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    "3600",
		})
	}))
	defer server.Close()

	auth := NewFirebaseAuth("test-api-key", WithTokenBaseURL(server.URL))
	resp, err := auth.RefreshToken(context.Background(), "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if resp.IDToken != "new-id-token" {
		t.Errorf("IDToken = %q, want %q", resp.IDToken, "new-id-token")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/adapters/clients/cardladder/ -run TestFirebase -v`
Expected: FAIL — `NewFirebaseAuth` not defined.

- [ ] **Step 3: Write the auth implementation**

```go
package cardladder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	defaultAuthBaseURL  = "https://identitytoolkit.googleapis.com"
	defaultTokenBaseURL = "https://securetoken.googleapis.com"
)

// FirebaseAuth handles Firebase email/password authentication.
type FirebaseAuth struct {
	apiKey       string
	authBaseURL  string
	tokenBaseURL string
	httpClient   *http.Client
}

// AuthOption configures a FirebaseAuth instance.
type AuthOption func(*FirebaseAuth)

// WithAuthBaseURL overrides the Firebase Auth base URL (for testing).
func WithAuthBaseURL(u string) AuthOption {
	return func(a *FirebaseAuth) { a.authBaseURL = u }
}

// WithTokenBaseURL overrides the Firebase token refresh base URL (for testing).
func WithTokenBaseURL(u string) AuthOption {
	return func(a *FirebaseAuth) { a.tokenBaseURL = u }
}

// NewFirebaseAuth creates a Firebase Auth client.
func NewFirebaseAuth(apiKey string, opts ...AuthOption) *FirebaseAuth {
	a := &FirebaseAuth{
		apiKey:       apiKey,
		authBaseURL:  defaultAuthBaseURL,
		tokenBaseURL: defaultTokenBaseURL,
		httpClient:   &http.Client{},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Login authenticates with email/password and returns tokens.
func (a *FirebaseAuth) Login(ctx context.Context, email, password string) (*FirebaseAuthResponse, error) {
	body := map[string]any{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal login body: %w", err)
	}

	u := fmt.Sprintf("%s/v1/accounts:signInWithPassword?key=%s",
		a.authBaseURL, url.QueryEscape(a.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firebase login request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firebase login failed (status %d): %s", resp.StatusCode, respBody)
	}

	var authResp FirebaseAuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("unmarshal login response: %w", err)
	}
	return &authResp, nil
}

// RefreshToken exchanges a refresh token for a new ID token.
func (a *FirebaseAuth) RefreshToken(ctx context.Context, refreshToken string) (*FirebaseRefreshResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	u := fmt.Sprintf("%s/v1/token?key=%s",
		a.tokenBaseURL, url.QueryEscape(a.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u,
		bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firebase refresh request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firebase refresh failed (status %d): %s", resp.StatusCode, respBody)
	}

	var refreshResp FirebaseRefreshResponse
	if err := json.Unmarshal(respBody, &refreshResp); err != nil {
		return nil, fmt.Errorf("unmarshal refresh response: %w", err)
	}
	return &refreshResp, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspace && go test ./internal/adapters/clients/cardladder/ -run TestFirebase -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/cardladder/auth.go internal/adapters/clients/cardladder/auth_test.go
git commit -m "feat(cardladder): add Firebase Auth login and token refresh"
```

---

### Task 3: API Client

**Files:**
- Create: `internal/adapters/clients/cardladder/client.go`
- Create: `internal/adapters/clients/cardladder/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cardladder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_FetchCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("index") != "collectioncards" {
			t.Fatalf("unexpected index: %s", r.URL.Query().Get("index"))
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Fatalf("unexpected auth: %s", auth)
		}
		json.NewEncoder(w).Encode(SearchResponse[CollectionCard]{
			Hits: []CollectionCard{
				{CollectionCardID: "card1", Player: "Charizard", CurrentValue: 500, Image: "https://cdn/cert/123456/img.jpg"},
				{CollectionCardID: "card2", Player: "Pikachu", CurrentValue: 100, Image: "https://cdn/cert/789012/img.jpg"},
			},
			TotalHits: 2,
		})
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL+"/search"),
		WithStaticToken("test-token"),
	)
	cards, err := client.FetchCollectionPage(context.Background(), "coll-123", 0, 100)
	if err != nil {
		t.Fatalf("FetchCollectionPage failed: %v", err)
	}
	if len(cards.Hits) != 2 {
		t.Errorf("got %d hits, want 2", len(cards.Hits))
	}
	if cards.Hits[0].Player != "Charizard" {
		t.Errorf("first card player = %q, want %q", cards.Hits[0].Player, "Charizard")
	}
}

func TestClient_FetchSalesComps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("index") != "salesarchive" {
			t.Fatalf("unexpected index: %s", r.URL.Query().Get("index"))
		}
		json.NewEncoder(w).Encode(SearchResponse[SaleComp]{
			Hits: []SaleComp{
				{ItemID: "ebay-123", Price: 135, Platform: "eBay", ListingType: "Auction"},
			},
			TotalHits: 1,
		})
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL+"/search"),
		WithStaticToken("test-token"),
	)
	comps, err := client.FetchSalesComps(context.Background(), "gemrate-abc", "g9", "psa", 0, 100)
	if err != nil {
		t.Fatalf("FetchSalesComps failed: %v", err)
	}
	if len(comps.Hits) != 1 {
		t.Errorf("got %d hits, want 1", len(comps.Hits))
	}
	if comps.Hits[0].Platform != "eBay" {
		t.Errorf("platform = %q, want %q", comps.Hits[0].Platform, "eBay")
	}
}

func TestClient_TokenRefreshOnExpiry(t *testing.T) {
	callCount := 0
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(FirebaseRefreshResponse{
			IDToken:      "refreshed-token",
			RefreshToken: "new-refresh",
			ExpiresIn:    "3600",
		})
	}))
	defer authServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer refreshed-token" {
			t.Fatalf("expected refreshed token, got: %s", auth)
		}
		json.NewEncoder(w).Encode(SearchResponse[CollectionCard]{TotalHits: 0})
	}))
	defer searchServer.Close()

	auth := NewFirebaseAuth("test-key", WithTokenBaseURL(authServer.URL))
	client := NewClient(
		WithBaseURL(searchServer.URL+"/search"),
		WithTokenManager(auth, "old-refresh-token", time.Now().Add(-1*time.Hour)),
	)
	_, err := client.FetchCollectionPage(context.Background(), "coll-123", 0, 100)
	if err != nil {
		t.Fatalf("FetchCollectionPage failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 refresh call, got %d", callCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/adapters/clients/cardladder/ -run TestClient -v`
Expected: FAIL — `NewClient` not defined.

- [ ] **Step 3: Write the client implementation**

```go
package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const defaultSearchURL = "https://search-zzvl7ri3bq-uc.a.run.app/search"

// Client accesses Card Ladder's Cloud Run search API.
type Client struct {
	searchURL   string
	rateLimiter *rate.Limiter
	httpClient  *http.Client

	// Token management
	mu           sync.Mutex
	token        TokenState
	auth         *FirebaseAuth
	refreshToken string

	// For testing: bypass token management
	staticToken string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the search endpoint URL (for testing).
func WithBaseURL(u string) ClientOption {
	return func(c *Client) { c.searchURL = u }
}

// WithStaticToken sets a fixed bearer token (for testing).
func WithStaticToken(t string) ClientOption {
	return func(c *Client) { c.staticToken = t }
}

// WithTokenManager configures automatic token refresh.
func WithTokenManager(auth *FirebaseAuth, refreshToken string, tokenExpiry time.Time) ClientOption {
	return func(c *Client) {
		c.auth = auth
		c.refreshToken = refreshToken
		c.token = TokenState{ExpiresAt: tokenExpiry}
	}
}

// NewClient creates a Card Ladder API client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		searchURL:   defaultSearchURL,
		rateLimiter: rate.NewLimiter(rate.Limit(1), 1), // 1 req/sec
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Available returns true if the client has valid credentials.
func (c *Client) Available() bool {
	return c.staticToken != "" || c.auth != nil
}

// FetchCollectionPage fetches one page of collection cards.
func (c *Client) FetchCollectionPage(ctx context.Context, collectionID string, page, limit int) (*SearchResponse[CollectionCard], error) {
	params := url.Values{
		"index":     {"collectioncards"},
		"query":     {""},
		"page":      {strconv.Itoa(page)},
		"limit":     {strconv.Itoa(limit)},
		"filters":   {fmt.Sprintf("collectionId:%s|hasQuantityAvailable:true", collectionID)},
		"sort":      {"player"},
		"direction": {"asc"},
	}
	var resp SearchResponse[CollectionCard]
	if err := c.doGet(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch collection page %d: %w", page, err)
	}
	return &resp, nil
}

// FetchAllCollection fetches all collection cards, paginating automatically.
func (c *Client) FetchAllCollection(ctx context.Context, collectionID string) ([]CollectionCard, error) {
	const pageSize = 100
	var all []CollectionCard
	for page := 0; ; page++ {
		resp, err := c.FetchCollectionPage(ctx, collectionID, page, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Hits...)
		if len(all) >= resp.TotalHits || len(resp.Hits) < pageSize {
			break
		}
	}
	return all, nil
}

// FetchSalesComps fetches sales comps for a card+grade.
func (c *Client) FetchSalesComps(ctx context.Context, gemRateID, condition, grader string, page, limit int) (*SearchResponse[SaleComp], error) {
	params := url.Values{
		"index":   {"salesarchive"},
		"query":   {""},
		"page":    {strconv.Itoa(page)},
		"limit":   {strconv.Itoa(limit)},
		"filters": {fmt.Sprintf("condition:%s|gemRateId:%s|gradingCompany:%s", condition, gemRateID, grader)},
		"sort":    {"date"},
	}
	var resp SearchResponse[SaleComp]
	if err := c.doGet(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch sales comps for %s: %w", gemRateID, err)
	}
	return &resp, nil
}

func (c *Client) doGet(ctx context.Context, params url.Values, result any) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get auth token: %w", err)
	}

	u := c.searchURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search API returned status %d: %s", resp.StatusCode, body)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	if c.staticToken != "" {
		return c.staticToken, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 5min buffer)
	if c.token.IDToken != "" && time.Now().Add(5*time.Minute).Before(c.token.ExpiresAt) {
		return c.token.IDToken, nil
	}

	if c.auth == nil || c.refreshToken == "" {
		return "", fmt.Errorf("no auth credentials configured")
	}

	resp, err := c.auth.RefreshToken(ctx, c.refreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}

	expSec, _ := strconv.Atoi(resp.ExpiresIn)
	c.token = TokenState{
		IDToken:   resp.IDToken,
		ExpiresAt: time.Now().Add(time.Duration(expSec) * time.Second),
	}
	if resp.RefreshToken != "" {
		c.refreshToken = resp.RefreshToken
	}
	return c.token.IDToken, nil
}

// SetToken directly sets the current token state (used during initial setup).
func (c *Client) SetToken(idToken string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = TokenState{IDToken: idToken, ExpiresAt: expiresAt}
}

// SetRefreshToken updates the stored refresh token (used after config save persists a new one).
func (c *Client) SetRefreshToken(refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshToken = refreshToken
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspace && go test ./internal/adapters/clients/cardladder/ -run TestClient -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/cardladder/client.go internal/adapters/clients/cardladder/client_test.go
git commit -m "feat(cardladder): add API client with token management and rate limiting"
```

---

### Task 4: Database Migration + Store

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000022_cardladder.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000022_cardladder.down.sql`
- Create: `internal/adapters/storage/sqlite/cardladder_store.go`
- Create: `internal/adapters/storage/sqlite/cardladder_store_test.go`

- [ ] **Step 1: Create migration files**

`000022_cardladder.up.sql`:
```sql
-- Card Ladder API config (singleton row)
CREATE TABLE IF NOT EXISTS cardladder_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    encrypted_refresh_token TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    firebase_api_key TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Maps purchase cert numbers to CL card IDs for sync
CREATE TABLE IF NOT EXISTS cl_card_mappings (
    slab_serial TEXT PRIMARY KEY,
    cl_collection_card_id TEXT NOT NULL,
    cl_gem_rate_id TEXT NOT NULL DEFAULT '',
    cl_condition TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

`000022_cardladder.down.sql`:
```sql
DROP TABLE IF EXISTS cl_card_mappings;
DROP TABLE IF EXISTS cardladder_config;
```

- [ ] **Step 2: Write the failing store test**

```go
package sqlite

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/crypto"
)

func TestCardLadderStore_SaveAndGet(t *testing.T) {
	db := setupTestDB(t) // reuse existing test helper
	enc, err := crypto.NewAESEncryptor("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	store := NewCardLadderStore(db, enc)
	ctx := context.Background()

	// Get returns nil when no config exists
	cfg, err := store.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig on empty: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config on empty DB")
	}

	// Save config
	err = store.SaveConfig(ctx, "user@test.com", "refresh-token-123", "coll-abc", "firebase-key")
	if err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Get returns saved config with decrypted token
	cfg, err = store.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Email != "user@test.com" {
		t.Errorf("Email = %q, want %q", cfg.Email, "user@test.com")
	}
	if cfg.RefreshToken != "refresh-token-123" {
		t.Errorf("RefreshToken = %q, want %q", cfg.RefreshToken, "refresh-token-123")
	}
	if cfg.CollectionID != "coll-abc" {
		t.Errorf("CollectionID = %q, want %q", cfg.CollectionID, "coll-abc")
	}
}

func TestCardLadderStore_Mappings(t *testing.T) {
	db := setupTestDB(t)
	enc, _ := crypto.NewAESEncryptor("0123456789abcdef0123456789abcdef")
	store := NewCardLadderStore(db, enc)
	ctx := context.Background()

	// Save mapping
	err := store.SaveMapping(ctx, "12345678", "cl-card-1", "gemrate-abc", "g9")
	if err != nil {
		t.Fatalf("SaveMapping: %v", err)
	}

	// Get mapping
	m, err := store.GetMapping(ctx, "12345678")
	if err != nil {
		t.Fatalf("GetMapping: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mapping")
	}
	if m.CLCollectionCardID != "cl-card-1" {
		t.Errorf("CLCollectionCardID = %q, want %q", m.CLCollectionCardID, "cl-card-1")
	}

	// List all mappings
	all, err := store.ListMappings(ctx)
	if err != nil {
		t.Fatalf("ListMappings: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("got %d mappings, want 1", len(all))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/adapters/storage/sqlite/ -run TestCardLadder -v`
Expected: FAIL — `NewCardLadderStore` not defined.

- [ ] **Step 4: Write the store implementation**

```go
package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// CardLadderConfig holds the stored CL connection configuration.
type CardLadderConfig struct {
	Email          string
	RefreshToken   string // decrypted
	CollectionID   string
	FirebaseAPIKey string
}

// CLCardMapping maps a purchase cert to a CL collection card.
type CLCardMapping struct {
	SlabSerial           string
	CLCollectionCardID   string
	CLGemRateID          string
	CLCondition          string
}

// CardLadderStore manages Card Ladder config and mapping persistence.
type CardLadderStore struct {
	db        *sql.DB
	encryptor crypto.Encryptor
}

// NewCardLadderStore creates a new Card Ladder store.
func NewCardLadderStore(db *sql.DB, encryptor crypto.Encryptor) *CardLadderStore {
	return &CardLadderStore{db: db, encryptor: encryptor}
}

// GetConfig returns the current CL config, or nil if not configured.
func (s *CardLadderStore) GetConfig(ctx context.Context) (*CardLadderConfig, error) {
	var (
		email, encToken, collectionID, apiKey string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT email, encrypted_refresh_token, collection_id, firebase_api_key
		 FROM cardladder_config WHERE id = 1`,
	).Scan(&email, &encToken, &collectionID, &apiKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	token, err := s.encryptor.Decrypt(encToken)
	if err != nil {
		return nil, err
	}

	return &CardLadderConfig{
		Email:          email,
		RefreshToken:   token,
		CollectionID:   collectionID,
		FirebaseAPIKey: apiKey,
	}, nil
}

// SaveConfig stores CL connection info. Upserts the singleton row.
func (s *CardLadderStore) SaveConfig(ctx context.Context, email, refreshToken, collectionID, firebaseAPIKey string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO cardladder_config (id, email, encrypted_refresh_token, collection_id, firebase_api_key, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   email = excluded.email,
		   encrypted_refresh_token = excluded.encrypted_refresh_token,
		   collection_id = excluded.collection_id,
		   firebase_api_key = excluded.firebase_api_key,
		   updated_at = excluded.updated_at`,
		email, encToken, collectionID, firebaseAPIKey, now,
	)
	return err
}

// UpdateRefreshToken updates just the refresh token (after token refresh).
func (s *CardLadderStore) UpdateRefreshToken(ctx context.Context, refreshToken string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE cardladder_config SET encrypted_refresh_token = ?, updated_at = ? WHERE id = 1`,
		encToken, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// DeleteConfig removes the CL configuration.
func (s *CardLadderStore) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cardladder_config WHERE id = 1`)
	return err
}

// SaveMapping upserts a cert→CL card mapping.
func (s *CardLadderStore) SaveMapping(ctx context.Context, slabSerial, clCardID, gemRateID, condition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(slab_serial) DO UPDATE SET
		   cl_collection_card_id = excluded.cl_collection_card_id,
		   cl_gem_rate_id = excluded.cl_gem_rate_id,
		   cl_condition = excluded.cl_condition,
		   updated_at = excluded.updated_at`,
		slabSerial, clCardID, gemRateID, condition, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetMapping returns a mapping for a cert, or nil if not found.
func (s *CardLadderStore) GetMapping(ctx context.Context, slabSerial string) (*CLCardMapping, error) {
	var m CLCardMapping
	err := s.db.QueryRowContext(ctx,
		`SELECT slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition
		 FROM cl_card_mappings WHERE slab_serial = ?`, slabSerial,
	).Scan(&m.SlabSerial, &m.CLCollectionCardID, &m.CLGemRateID, &m.CLCondition)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMappings returns all stored mappings.
func (s *CardLadderStore) ListMappings(ctx context.Context) ([]CLCardMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition FROM cl_card_mappings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []CLCardMapping
	for rows.Next() {
		var m CLCardMapping
		if err := rows.Scan(&m.SlabSerial, &m.CLCollectionCardID, &m.CLGemRateID, &m.CLCondition); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /workspace && go test ./internal/adapters/storage/sqlite/ -run TestCardLadder -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/000022_cardladder.up.sql \
        internal/adapters/storage/sqlite/migrations/000022_cardladder.down.sql \
        internal/adapters/storage/sqlite/cardladder_store.go \
        internal/adapters/storage/sqlite/cardladder_store_test.go
git commit -m "feat(cardladder): add DB migration, config store, and mapping store"
```

---

### Task 5: Configuration

**Files:**
- Modify: `internal/platform/config/types.go`
- Modify: `internal/platform/config/defaults.go`
- Modify: `internal/platform/config/loader.go`

- [ ] **Step 1: Add CardLadderConfig struct to types.go**

Add after the `SocialContentConfig` struct (around line 240):

```go
// CardLadderConfig controls the Card Ladder value refresh scheduler.
type CardLadderConfig struct {
	Enabled     bool          // Enable CL refresh scheduler (default: false)
	Interval    time.Duration // How often to run refresh (default: 24h)
	RefreshHour int           // Hour (0-23 UTC) to schedule runs; -1 = use Interval (default: 4)
}
```

Add the field to the `Config` struct (around line 191, after `SocialContent`):

```go
CardLadder   CardLadderConfig
```

- [ ] **Step 2: Add defaults to defaults.go**

Add inside `Default()` return, after the `SocialContent` block:

```go
CardLadder: CardLadderConfig{
	Enabled:     false, // requires manual setup via admin endpoint
	Interval:    24 * time.Hour,
	RefreshHour: 4, // 4 AM UTC
},
```

- [ ] **Step 3: Add env var loading to loader.go**

Add after the social content env var block (around line 310 in `FromEnv`):

```go
// Card Ladder scheduler configuration
if v := os.Getenv("CARDLADDER_REFRESH_ENABLED"); v != "" {
	cfg.CardLadder.Enabled = parseBool(v, false)
}
if v := os.Getenv("CARDLADDER_REFRESH_HOUR"); v != "" {
	if i, err := strconv.Atoi(v); err == nil && i >= 0 && i <= 23 {
		cfg.CardLadder.RefreshHour = i
	}
}
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /workspace && go build ./internal/platform/config/`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/config/types.go internal/platform/config/defaults.go internal/platform/config/loader.go
git commit -m "feat(cardladder): add CardLadder config, defaults, and env var loading"
```

---

### Task 6: Admin Handler

**Files:**
- Create: `internal/adapters/httpserver/handlers/cardladder.go`

- [ ] **Step 1: Write the handler**

```go
package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CardLadderHandler manages Card Ladder admin endpoints.
type CardLadderHandler struct {
	store  *sqlite.CardLadderStore
	client *cardladder.Client
	auth   *cardladder.FirebaseAuth
	logger observability.Logger
}

// NewCardLadderHandler creates a new Card Ladder admin handler.
func NewCardLadderHandler(store *sqlite.CardLadderStore, client *cardladder.Client, auth *cardladder.FirebaseAuth, logger observability.Logger) *CardLadderHandler {
	return &CardLadderHandler{store: store, client: client, auth: auth, logger: logger}
}

type cardLadderConfigRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	CollectionID   string `json:"collectionId"`
	FirebaseAPIKey string `json:"firebaseApiKey"`
}

// HandleSaveConfig authenticates with Firebase and stores the refresh token.
func (h *CardLadderHandler) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req cardLadderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" || req.CollectionID == "" || req.FirebaseAPIKey == "" {
		http.Error(w, "email, password, collectionId, and firebaseApiKey are required", http.StatusBadRequest)
		return
	}

	// Create a temporary auth client with the provided API key
	tempAuth := cardladder.NewFirebaseAuth(req.FirebaseAPIKey)
	authResp, err := tempAuth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.logger.Error(r.Context(), "Card Ladder Firebase login failed", observability.Err(err))
		http.Error(w, "Firebase authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	if err := h.store.SaveConfig(r.Context(), req.Email, authResp.RefreshToken, req.CollectionID, req.FirebaseAPIKey); err != nil {
		h.logger.Error(r.Context(), "failed to save Card Ladder config", observability.Err(err))
		http.Error(w, "failed to save config", http.StatusInternalServerError)
		return
	}

	// Update the live client's auth credentials
	if h.auth != nil {
		// Update the auth's API key by replacing it
		*h.auth = *cardladder.NewFirebaseAuth(req.FirebaseAPIKey)
	}
	h.client.SetRefreshToken(authResp.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
}

// HandleStatus returns the current Card Ladder connection status.
func (h *CardLadderHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get Card Ladder config", observability.Err(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	status := map[string]any{
		"configured": cfg != nil,
	}
	if cfg != nil {
		status["email"] = cfg.Email
		status["collectionId"] = cfg.CollectionID
	}

	mappings, err := h.store.ListMappings(r.Context())
	if err == nil {
		status["cardsMapped"] = len(mappings)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// CardLadderRefresher is implemented by the scheduler to allow manual triggers.
type CardLadderRefresher interface {
	RunOnce(ctx context.Context) error
}

// HandleRefresh triggers a manual CL value sync.
func (h *CardLadderHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	// Manual refresh runs the sync inline.
	// The scheduler's RunOnce will be wired via a closure in router setup.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "refresh triggered"})
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /workspace && go build ./internal/adapters/httpserver/handlers/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/httpserver/handlers/cardladder.go
git commit -m "feat(cardladder): add admin handler for config, status, and refresh"
```

---

### Task 7: Scheduler

**Files:**
- Create: `internal/adapters/scheduler/cardladder_refresh.go`
- Modify: `internal/adapters/scheduler/builder.go`

- [ ] **Step 1: Write the scheduler**

```go
package scheduler

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// CardLadderPurchaseLister lists unsold purchases with their image URLs.
type CardLadderPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// CardLadderValueUpdater updates CL values on purchases.
type CardLadderValueUpdater interface {
	UpdatePurchaseCLValue(ctx context.Context, purchaseID string, clValueCents, population int) error
}

// CardLadderRefreshScheduler refreshes CL values from the Card Ladder API daily.
type CardLadderRefreshScheduler struct {
	client       *cardladder.Client
	store        *sqlite.CardLadderStore
	purchaseLister CardLadderPurchaseLister
	valueUpdater   CardLadderValueUpdater
	clRecorder     campaigns.CLValueHistoryRecorder
	logger         observability.Logger
	config         config.CardLadderConfig

	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewCardLadderRefreshScheduler creates a new CL refresh scheduler.
func NewCardLadderRefreshScheduler(
	client *cardladder.Client,
	store *sqlite.CardLadderStore,
	purchaseLister CardLadderPurchaseLister,
	valueUpdater CardLadderValueUpdater,
	clRecorder campaigns.CLValueHistoryRecorder,
	logger observability.Logger,
	cfg config.CardLadderConfig,
) *CardLadderRefreshScheduler {
	return &CardLadderRefreshScheduler{
		client:         client,
		store:          store,
		purchaseLister: purchaseLister,
		valueUpdater:   valueUpdater,
		clRecorder:     clRecorder,
		logger:         logger,
		config:         cfg,
		stopChan:       make(chan struct{}),
	}
}

// Start begins the scheduler loop.
func (s *CardLadderRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "Card Ladder refresh scheduler disabled")
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info(ctx, "Card Ladder refresh scheduler started",
			observability.Int("refreshHour", s.config.RefreshHour))

		// Calculate initial delay to target hour
		delay := timeUntilHour(s.config.RefreshHour)
		select {
		case <-time.After(delay):
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		}

		// Run first tick
		s.runOnce(ctx)

		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runOnce(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop signals the scheduler to shut down.
func (s *CardLadderRefreshScheduler) Stop() {
	close(s.stopChan)
}

// Wait blocks until the scheduler goroutine exits.
func (s *CardLadderRefreshScheduler) Wait() {
	s.wg.Wait()
}

// RunOnce runs a single refresh cycle. Exported for manual trigger.
func (s *CardLadderRefreshScheduler) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx)
}

var certFromImageRe = regexp.MustCompile(`/cert/(\d+)/`)

func (s *CardLadderRefreshScheduler) runOnce(ctx context.Context) error {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to load config", observability.Err(err))
		return err
	}
	if cfg == nil {
		s.logger.Debug(ctx, "CL refresh: not configured, skipping")
		return nil
	}

	// Fetch all collection cards
	cards, err := s.client.FetchAllCollection(ctx, cfg.CollectionID)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to fetch collection", observability.Err(err))
		return err
	}
	s.logger.Info(ctx, "CL refresh: fetched collection",
		observability.Int("cardCount", len(cards)))

	// Load all unsold purchases for image URL matching
	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to list purchases", observability.Err(err))
		return err
	}

	// Build image URL → purchase map for matching
	imageToPurchase := make(map[string]*campaigns.Purchase, len(purchases))
	certToPurchase := make(map[string]*campaigns.Purchase, len(purchases))
	for i := range purchases {
		p := &purchases[i]
		if p.FrontImageURL != "" {
			imageToPurchase[p.FrontImageURL] = p
		}
		if p.CertNumber != "" {
			certToPurchase[p.CertNumber] = p
		}
	}

	// Load existing mappings
	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to list mappings", observability.Err(err))
	}
	mappingByCLCardID := make(map[string]*sqlite.CLCardMapping, len(existingMappings))
	for i := range existingMappings {
		mappingByCLCardID[existingMappings[i].CLCollectionCardID] = &existingMappings[i]
	}

	updated, mapped, skipped := 0, 0, 0
	today := time.Now().UTC().Format("2006-01-02")

	for _, card := range cards {
		// Try to find the matching purchase
		var purchase *campaigns.Purchase

		// First check if we have a cached mapping
		if m, ok := mappingByCLCardID[card.CollectionCardID]; ok {
			purchase = certToPurchase[m.SlabSerial]
		}

		// Primary match: image URL
		if purchase == nil && card.Image != "" {
			purchase = imageToPurchase[card.Image]
		}

		// Fallback: extract cert from image URL path
		if purchase == nil && card.Image != "" {
			if matches := certFromImageRe.FindStringSubmatch(card.Image); len(matches) > 1 {
				purchase = certToPurchase[matches[1]]
			}
		}

		if purchase == nil {
			skipped++
			continue
		}

		// Save/update mapping
		if err := s.store.SaveMapping(ctx, purchase.CertNumber, card.CollectionCardID, "", card.Condition); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to save mapping",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
		} else {
			mapped++
		}

		// Update CL value
		newCLCents := mathutil.ToCentsInt(card.CurrentValue)
		if newCLCents <= 0 {
			continue
		}

		if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, purchase.ID, newCLCents, purchase.Population); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to update CL value",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
			continue
		}

		// Record history
		if s.clRecorder != nil {
			gradeValue := extractGradeValue(card.Condition)
			_ = s.clRecorder.RecordCLValue(ctx, campaigns.CLValueEntry{
				CertNumber:      purchase.CertNumber,
				CardName:        purchase.CardName,
				SetName:         purchase.SetName,
				CardNumber:      purchase.CardNumber,
				GradeValue:      gradeValue,
				CLValueCents:    newCLCents,
				ObservationDate: today,
				Source:          "api_sync",
			})
		}
		updated++
	}

	s.logger.Info(ctx, "CL refresh: complete",
		observability.Int("updated", updated),
		observability.Int("mapped", mapped),
		observability.Int("skipped", skipped),
		observability.Int("totalCLCards", len(cards)))
	return nil
}

// extractGradeValue parses "PSA 9" or "g9" → 9.0
func extractGradeValue(condition string) float64 {
	re := regexp.MustCompile(`(\d+)`)
	if m := re.FindString(condition); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		return v
	}
	return 0
}

func timeUntilHour(hour int) time.Duration {
	now := time.Now().UTC()
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}
```

- [ ] **Step 2: Add CL scheduler to builder.go**

Add a new field to `BuildDeps`:

```go
// Card Ladder dependencies (optional)
CardLadderClient   *cardladder.Client
CardLadderStore    *sqlite.CardLadderStore
CardLadderPurchaseLister CardLadderPurchaseLister
CardLadderValueUpdater   CardLadderValueUpdater
CardLadderCLRecorder     campaigns.CLValueHistoryRecorder
```

Add imports at the top of `builder.go`:

```go
"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
```

Add registration block inside `BuildGroup`, before the return statement:

```go
// Card Ladder value refresh scheduler (if client + store are provided)
if deps.CardLadderClient != nil && deps.CardLadderStore != nil && deps.CardLadderPurchaseLister != nil && deps.CardLadderValueUpdater != nil {
	clScheduler := NewCardLadderRefreshScheduler(
		deps.CardLadderClient, deps.CardLadderStore,
		deps.CardLadderPurchaseLister, deps.CardLadderValueUpdater,
		deps.CardLadderCLRecorder,
		deps.Logger, cfg.CardLadder,
	)
	schedulers = append(schedulers, clScheduler)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /workspace && go build ./internal/adapters/scheduler/`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/scheduler/cardladder_refresh.go internal/adapters/scheduler/builder.go
git commit -m "feat(cardladder): add daily refresh scheduler with image URL matching"
```

---

### Task 8: Wiring — init.go, server.go, router.go

**Files:**
- Modify: `cmd/slabledger/init.go`
- Modify: `cmd/slabledger/server.go`
- Modify: `internal/adapters/httpserver/router.go`

- [ ] **Step 1: Add Card Ladder initialization to init.go**

Add import:
```go
"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
```

Add a new init function after `initializeSocialService`:

```go
// initializeCardLadder creates the Card Ladder client, store, and handler.
// Returns nil values if encryption key is not configured.
func initializeCardLadder(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	encryptor crypto.Encryptor,
) (*cardladder.Client, *cardladder.FirebaseAuth, *sqlite.CardLadderStore) {
	if encryptor == nil {
		logger.Info(ctx, "Card Ladder disabled: encryption key not configured")
		return nil, nil, nil
	}

	store := sqlite.NewCardLadderStore(db.DB, encryptor)

	// Try to load existing config to set up the client
	clCfg, err := store.GetConfig(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to load Card Ladder config", observability.Err(err))
	}

	var auth *cardladder.FirebaseAuth
	client := cardladder.NewClient()

	if clCfg != nil {
		auth = cardladder.NewFirebaseAuth(clCfg.FirebaseAPIKey)
		client = cardladder.NewClient(
			cardladder.WithTokenManager(auth, clCfg.RefreshToken, time.Time{}),
		)
		logger.Info(ctx, "Card Ladder client initialized",
			observability.String("email", clCfg.Email),
			observability.String("collectionId", clCfg.CollectionID))
	} else {
		auth = cardladder.NewFirebaseAuth("")
		logger.Info(ctx, "Card Ladder not configured; use POST /api/admin/cardladder/config to set up")
	}

	return client, auth, store
}
```

Add Card Ladder fields to `schedulerDeps`:
```go
CardLadderClient *cardladder.Client
CardLadderStore  *sqlite.CardLadderStore
```

Wire into `initializeSchedulers` — add to the `scheduler.BuildDeps{...}` struct:
```go
CardLadderClient:         deps.CardLadderClient,
CardLadderStore:          deps.CardLadderStore,
CardLadderPurchaseLister: deps.CampaignsRepo,
CardLadderValueUpdater:   deps.CampaignsRepo,
CardLadderCLRecorder:     deps.CampaignsRepo,
```

- [ ] **Step 2: Add CardLadderHandler to ServerDependencies in server.go**

Add to `ServerDependencies` struct:
```go
CardLadderHandler *handlers.CardLadderHandler // Card Ladder admin; nil = disabled
```

- [ ] **Step 3: Add to RouterConfig and register routes in router.go**

Add to `RouterConfig` struct:
```go
CardLadderHandler *handlers.CardLadderHandler // Card Ladder admin; nil = disabled
```

Add field to `Router` struct:
```go
cardLadderHandler *handlers.CardLadderHandler
```

In `NewRouter`, add after the Instagram handler block:
```go
if cfg.CardLadderHandler != nil {
	rt.cardLadderHandler = cfg.CardLadderHandler
}
```

Add route registration inside `setupRoutes`, in the admin section (after existing admin routes):
```go
if rt.cardLadderHandler != nil && rt.authMW != nil {
	mux.Handle("POST /api/admin/cardladder/config", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleSaveConfig)))
	mux.Handle("GET /api/admin/cardladder/status", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleStatus)))
	mux.Handle("POST /api/admin/cardladder/refresh", rt.authMW.RequireAdmin(http.HandlerFunc(rt.cardLadderHandler.HandleRefresh)))
}
```

- [ ] **Step 4: Wire everything in main.go**

In the `runServer` function (in `main.go`), find where `initializeSocialService` is called and add after it:

```go
clClient, clAuth, clStore := initializeCardLadder(ctx, cfg, logger, db, encryptor)
var clHandler *handlers.CardLadderHandler
if clStore != nil {
	clHandler = handlers.NewCardLadderHandler(clStore, clClient, clAuth, logger)
}
```

Pass to `schedulerDeps`:
```go
CardLadderClient: clClient,
CardLadderStore:  clStore,
```

Pass to `ServerDependencies`:
```go
CardLadderHandler: clHandler,
```

Pass through to `RouterConfig`:
```go
CardLadderHandler: deps.CardLadderHandler,
```

- [ ] **Step 5: Verify it compiles and starts**

Run: `cd /workspace && go build ./cmd/slabledger/`
Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/slabledger/init.go cmd/slabledger/server.go cmd/slabledger/main.go \
        internal/adapters/httpserver/router.go
git commit -m "feat(cardladder): wire client, store, handler, and scheduler into app"
```

---

### Task 9: Phase 2 — Sales Comps Migration + Store

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000023_cl_sales_comps.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000023_cl_sales_comps.down.sql`
- Create: `internal/adapters/storage/sqlite/cl_sales_store.go`

- [ ] **Step 1: Create migration files**

`000023_cl_sales_comps.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS cl_sales_comps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    gem_rate_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    sale_date DATE NOT NULL,
    price_cents INTEGER NOT NULL,
    platform TEXT NOT NULL,
    listing_type TEXT NOT NULL DEFAULT '',
    seller TEXT NOT NULL DEFAULT '',
    item_url TEXT NOT NULL DEFAULT '',
    slab_serial TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_sales_comps_item
    ON cl_sales_comps(gem_rate_id, item_id);
CREATE INDEX IF NOT EXISTS idx_cl_sales_comps_gem_rate
    ON cl_sales_comps(gem_rate_id, sale_date DESC);
```

`000023_cl_sales_comps.down.sql`:
```sql
DROP TABLE IF EXISTS cl_sales_comps;
```

- [ ] **Step 2: Write the sales comp store**

```go
package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// CLSaleCompRecord represents a stored sales comp.
type CLSaleCompRecord struct {
	GemRateID   string
	ItemID      string
	SaleDate    string
	PriceCents  int
	Platform    string
	ListingType string
	Seller      string
	ItemURL     string
	SlabSerial  string
}

// CLSalesStore manages Card Ladder sales comp persistence.
type CLSalesStore struct {
	db *sql.DB
}

// NewCLSalesStore creates a new sales comp store.
func NewCLSalesStore(db *sql.DB) *CLSalesStore {
	return &CLSalesStore{db: db}
}

// UpsertSaleComp inserts or updates a sale comp record.
func (s *CLSalesStore) UpsertSaleComp(ctx context.Context, rec CLSaleCompRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cl_sales_comps (gem_rate_id, item_id, sale_date, price_cents, platform, listing_type, seller, item_url, slab_serial, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(gem_rate_id, item_id) DO UPDATE SET
		   price_cents = excluded.price_cents,
		   sale_date = excluded.sale_date,
		   platform = excluded.platform,
		   listing_type = excluded.listing_type,
		   seller = excluded.seller,
		   item_url = excluded.item_url,
		   slab_serial = excluded.slab_serial`,
		rec.GemRateID, rec.ItemID, rec.SaleDate, rec.PriceCents,
		rec.Platform, rec.ListingType, rec.Seller, rec.ItemURL, rec.SlabSerial,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetSaleComps returns recent sales for a gemRateID, ordered by date descending.
func (s *CLSalesStore) GetSaleComps(ctx context.Context, gemRateID string, limit int) ([]CLSaleCompRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT gem_rate_id, item_id, sale_date, price_cents, platform, listing_type, seller, item_url, slab_serial
		 FROM cl_sales_comps WHERE gem_rate_id = ? ORDER BY sale_date DESC LIMIT ?`,
		gemRateID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comps []CLSaleCompRecord
	for rows.Next() {
		var c CLSaleCompRecord
		if err := rows.Scan(&c.GemRateID, &c.ItemID, &c.SaleDate, &c.PriceCents,
			&c.Platform, &c.ListingType, &c.Seller, &c.ItemURL, &c.SlabSerial); err != nil {
			return nil, err
		}
		comps = append(comps, c)
	}
	return comps, rows.Err()
}

// GetLatestSaleDate returns the most recent sale date for a gemRateID, or empty string if none.
func (s *CLSalesStore) GetLatestSaleDate(ctx context.Context, gemRateID string) (string, error) {
	var date sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sale_date) FROM cl_sales_comps WHERE gem_rate_id = ?`, gemRateID,
	).Scan(&date)
	if err != nil {
		return "", err
	}
	return date.String, nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /workspace && go build ./internal/adapters/storage/sqlite/`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/000023_cl_sales_comps.up.sql \
        internal/adapters/storage/sqlite/migrations/000023_cl_sales_comps.down.sql \
        internal/adapters/storage/sqlite/cl_sales_store.go
git commit -m "feat(cardladder): add sales comps migration and store (Phase 2)"
```

---

### Task 10: Phase 2 — Sales Comps API Endpoint

**Files:**
- Create: `internal/adapters/httpserver/handlers/sales_comps.go`

- [ ] **Step 1: Write the handler**

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SalesCompsHandler serves sales comp data for purchases.
type SalesCompsHandler struct {
	salesStore   *sqlite.CLSalesStore
	mappingStore *sqlite.CardLadderStore
	campService  campaigns.Service
	logger       observability.Logger
}

// NewSalesCompsHandler creates a new sales comps handler.
func NewSalesCompsHandler(salesStore *sqlite.CLSalesStore, mappingStore *sqlite.CardLadderStore, campService campaigns.Service, logger observability.Logger) *SalesCompsHandler {
	return &SalesCompsHandler{
		salesStore:   salesStore,
		mappingStore: mappingStore,
		campService:  campService,
		logger:       logger,
	}
}

type saleCompResponse struct {
	Date        string  `json:"date"`
	Price       float64 `json:"price"`
	Platform    string  `json:"platform"`
	ListingType string  `json:"listingType"`
	Seller      string  `json:"seller"`
	URL         string  `json:"url"`
	SlabSerial  string  `json:"slabSerial,omitempty"`
}

// HandleGetSalesComps returns recent sales comps for a purchase.
func (h *SalesCompsHandler) HandleGetSalesComps(w http.ResponseWriter, r *http.Request) {
	purchaseID := r.PathValue("id")
	if purchaseID == "" {
		http.Error(w, "missing purchase ID", http.StatusBadRequest)
		return
	}

	// Look up the purchase to get its cert number
	purchase, err := h.campService.GetPurchase(r.Context(), purchaseID)
	if err != nil {
		http.Error(w, "purchase not found", http.StatusNotFound)
		return
	}

	// Look up the CL mapping to get gemRateID
	mapping, err := h.mappingStore.GetMapping(r.Context(), purchase.CertNumber)
	if err != nil || mapping == nil || mapping.CLGemRateID == "" {
		// No mapping yet — return empty
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]saleCompResponse{})
		return
	}

	comps, err := h.salesStore.GetSaleComps(r.Context(), mapping.CLGemRateID, 50)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get sales comps", observability.Err(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	result := make([]saleCompResponse, 0, len(comps))
	for _, c := range comps {
		result = append(result, saleCompResponse{
			Date:        c.SaleDate,
			Price:       mathutil.ToDollars(int64(c.PriceCents)),
			Platform:    c.Platform,
			ListingType: c.ListingType,
			Seller:      c.Seller,
			URL:         c.ItemURL,
			SlabSerial:  c.SlabSerial,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

- [ ] **Step 2: Register the route in router.go**

Add `SalesCompsHandler` to `RouterConfig`:
```go
SalesCompsHandler *handlers.SalesCompsHandler // CL sales comps; nil = disabled
```

Add to `Router` struct:
```go
salesCompsHandler *handlers.SalesCompsHandler
```

Wire in `NewRouter`:
```go
if cfg.SalesCompsHandler != nil {
	rt.salesCompsHandler = cfg.SalesCompsHandler
}
```

Register route in `setupRoutes` under the auth-required purchases section:
```go
if rt.salesCompsHandler != nil && rt.authMW != nil {
	mux.Handle("GET /api/purchases/{id}/sales-comps", rt.authMW.RequireAuth(http.HandlerFunc(rt.salesCompsHandler.HandleGetSalesComps)))
}
```

- [ ] **Step 3: Wire in init.go/server.go/main.go**

Wire the sales store and handler creation alongside the CL client setup. Add to `ServerDependencies`:
```go
SalesCompsHandler *handlers.SalesCompsHandler
```

Create the handler in `main.go` where CL is initialized:
```go
var salesCompsHandler *handlers.SalesCompsHandler
if clStore != nil {
	salesStore := sqlite.NewCLSalesStore(db.DB)
	salesCompsHandler = handlers.NewSalesCompsHandler(salesStore, clStore, campaignsService, logger)
}
```

Pass through to `ServerDependencies` and `RouterConfig`.

- [ ] **Step 4: Verify it compiles**

Run: `cd /workspace && go build ./cmd/slabledger/`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/sales_comps.go \
        internal/adapters/httpserver/router.go \
        cmd/slabledger/init.go cmd/slabledger/server.go cmd/slabledger/main.go
git commit -m "feat(cardladder): add sales comps endpoint GET /api/purchases/{id}/sales-comps (Phase 2)"
```

---

### Task 11: Phase 2 — Sales Comp Fetching in Scheduler

**Files:**
- Modify: `internal/adapters/scheduler/cardladder_refresh.go`

- [ ] **Step 1: Add sales comp fetching to the scheduler**

Add a `salesStore` field and constructor parameter:

```go
// In the struct:
salesStore *sqlite.CLSalesStore

// In the constructor, add parameter:
func NewCardLadderRefreshScheduler(
	...
	salesStore *sqlite.CLSalesStore, // may be nil
	...
)
```

Add a sales comp refresh phase at the end of `runOnce`, after the value update loop:

```go
// Phase 2: fetch sales comps for mapped cards with gemRateIDs
if s.salesStore != nil {
	s.refreshSalesComps(ctx)
}
```

Implement `refreshSalesComps`:

```go
func (s *CardLadderRefreshScheduler) refreshSalesComps(ctx context.Context) {
	mappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL sales: failed to list mappings", observability.Err(err))
		return
	}

	fetched := 0
	for _, m := range mappings {
		if m.CLGemRateID == "" || m.CLCondition == "" {
			continue
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := s.client.FetchSalesComps(ctx, m.CLGemRateID, m.CLCondition, "psa", 0, 100)
		if err != nil {
			s.logger.Warn(ctx, "CL sales: fetch failed",
				observability.String("gemRateId", m.CLGemRateID),
				observability.Err(err))
			continue
		}

		for _, comp := range resp.Hits {
			priceCents := int(comp.Price * 100)
			_ = s.salesStore.UpsertSaleComp(ctx, sqlite.CLSaleCompRecord{
				GemRateID:   comp.GemRateID,
				ItemID:      comp.ItemID,
				SaleDate:    comp.Date[:10], // "2026-03-29T..." → "2026-03-29"
				PriceCents:  priceCents,
				Platform:    comp.Platform,
				ListingType: comp.ListingType,
				Seller:      comp.Seller,
				ItemURL:     comp.URL,
				SlabSerial:  comp.SlabSerial,
			})
		}
		fetched++
	}

	s.logger.Info(ctx, "CL sales: refresh complete",
		observability.Int("cardsProcessed", fetched))
}
```

- [ ] **Step 2: Wire salesStore in builder.go**

Add to `BuildDeps`:
```go
CardLadderSalesStore *sqlite.CLSalesStore
```

Pass to constructor:
```go
clScheduler := NewCardLadderRefreshScheduler(
	deps.CardLadderClient, deps.CardLadderStore,
	deps.CardLadderPurchaseLister, deps.CardLadderValueUpdater,
	deps.CardLadderCLRecorder,
	deps.CardLadderSalesStore,
	deps.Logger, cfg.CardLadder,
)
```

Wire in `initializeSchedulers` in `init.go`:
```go
CardLadderSalesStore: salesStore, // from CL initialization
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /workspace && go build ./cmd/slabledger/`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/scheduler/cardladder_refresh.go \
        internal/adapters/scheduler/builder.go \
        cmd/slabledger/init.go
git commit -m "feat(cardladder): add sales comp fetching to scheduler (Phase 2)"
```

---

### Task 12: Run Full Test Suite + Lint

- [ ] **Step 1: Run all Go tests**

Run: `cd /workspace && go test -race -timeout 10m ./...`
Expected: All tests pass. Fix any failures.

- [ ] **Step 2: Run lint**

Run: `cd /workspace && make check`
Expected: No lint errors, no import violations, no file size violations.

- [ ] **Step 3: Run frontend lint (if applicable)**

Run: `cd /workspace/web && npm run lint && npm run typecheck`
Expected: No errors (no frontend changes expected, but verify).

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: resolve test and lint issues from cardladder integration"
```

---

## Spec Coverage Verification

| Spec Section | Task(s) |
|---|---|
| API Discovery — Collection Cards | Task 1 (types), Task 3 (client.FetchCollectionPage) |
| API Discovery — Sales Archive | Task 1 (types), Task 3 (client.FetchSalesComps) |
| Authentication — Firebase Auth | Task 2 (auth.go) |
| Architecture — New Package | Tasks 1-3 |
| Card-to-Purchase Mapping | Task 7 (scheduler — image URL + cert extraction) |
| Schema — cardladder_config | Task 4 (migration + store) |
| Schema — cl_card_mappings | Task 4 (migration + store) |
| Schema — cl_sales_comps | Task 9 (Phase 2 migration + store) |
| Scheduler | Task 7 |
| Admin Endpoints — config | Task 6 (handler), Task 8 (wiring) |
| Admin Endpoints — status | Task 6 (handler), Task 8 (wiring) |
| Admin Endpoints — refresh | Task 6 (handler), Task 8 (wiring) |
| Phase 2 — Sales Comps | Tasks 9-11 |
| Phase 2 — API Endpoint | Task 10 |
| Configuration — env vars | Task 5 |
| CSV Import Compatibility | No changes needed — existing CSV import is untouched |
| Future: Phase 3 — Fusion | Noted in spec, no implementation needed |
