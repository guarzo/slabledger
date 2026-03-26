# Maintainability & AI Agent Experience — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve codebase maintainability and AI agent experience through documentation, code splits, linter config, and test coverage.

**Architecture:** Docs-first (Phase 1), then code refactoring (Phase 2), then tests (Phase 3). Each phase builds on the prior — Phase 1 docs inform Phase 2 splits, Phase 2 splits create stable targets for Phase 3 tests.

**Tech Stack:** Go 1.26, SQLite, React/TypeScript, Vite, golangci-lint

**Spec:** `docs/2026-03-26-maintainability-ai-agent-design.md`

---

## Phase 1: Documentation & AI Agent Experience

### Task 1: Create `.env.example`

**Files:**
- Create: `.env.example`

- [ ] **Step 1: Write `.env.example`**

Create `.env.example` at repo root with ALL environment variables the app reads (51 total from `internal/platform/config/loader.go`). Group by feature area. Include every env var from the config loader — not just the ones already in CLAUDE.md.

The full variable list by category:

**Required:** `PRICECHARTING_TOKEN`

**Server/Logging:** `LOG_LEVEL`, `LOG_JSON`, `HTTP_LISTEN_ADDR`, `HTTP_READ_TIMEOUT`, `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT`, `RATE_LIMIT_REQUESTS`, `RATE_LIMIT_TRUST_PROXY`

**Database:** `DATABASE_PATH`, `MIGRATIONS_PATH`

**Auth:** `ENCRYPTION_KEY`, `ADMIN_EMAILS`, `LOCAL_API_TOKEN` (from `main.go` env read)

**Google OAuth:** `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL` (from `config.LoadGoogleOAuthConfig()`)

**Pricing Sources:** `POKEMONPRICE_TRACKER_API_KEY`, `CARD_HEDGER_API_KEY`, `CARD_HEDGER_CLIENT_ID`, `PSA_ACCESS_TOKEN`, `PAO_API`, `PRICING_API_KEY`

**Price Refresh Scheduler:** `PRICE_REFRESH_ENABLED`, `PRICE_REFRESH_INTERVAL`, `PRICE_BATCH_SIZE`, `PRICE_BATCH_DELAY`, `PRICE_MAX_BURST_CALLS`, `PRICE_MAX_CALLS_PER_HOUR`, `PRICE_BURST_PAUSE_DURATION`

**CardHedger Scheduler:** `CARD_HEDGER_ENABLED`, `CARD_HEDGER_POLL_INTERVAL`, `CARD_HEDGER_BATCH_INTERVAL`, `CARD_HEDGER_MAX_CARDS_PER_RUN`

**Inventory Refresh:** `INVENTORY_REFRESH_ENABLED`, `INVENTORY_REFRESH_INTERVAL`, `INVENTORY_REFRESH_STALE_THRESHOLD`, `INVENTORY_REFRESH_BATCH_SIZE`, `INVENTORY_REFRESH_BATCH_DELAY`

**Cache Warmup:** `CACHE_WARMUP_ENABLED`, `CACHE_WARMUP_INTERVAL`, `CACHE_WARMUP_RATE_LIMIT_DELAY`

**Session Cleanup:** `SESSION_CLEANUP_ENABLED`, `SESSION_CLEANUP_INTERVAL`

**Access Log:** `ACCESS_LOG_RETENTION_DAYS`, `ACCESS_LOG_CLEANUP_INTERVAL`, `ACCESS_LOG_CLEANUP_ENABLED`

**Fusion:** `FUSION_CACHE_TTL`, `FUSION_PRICECHARTING_TIMEOUT`, `FUSION_SECONDARY_TIMEOUT`

**Snapshot Enrichment:** `SNAPSHOT_ENRICH_ENABLED`, `SNAPSHOT_ENRICH_INTERVAL`, `SNAPSHOT_ENRICH_BATCH_SIZE`, `SNAPSHOT_ENRICH_RETRY_INTERVAL`, `SNAPSHOT_ENRICH_MAX_RETRIES`

**Snapshot History:** `SNAPSHOT_HISTORY_ENABLED`, `SNAPSHOT_HISTORY_INTERVAL`

**Advisor Refresh:** `ADVISOR_REFRESH_ENABLED`, `ADVISOR_REFRESH_INTERVAL`, `ADVISOR_REFRESH_INITIAL_DELAY`

**AI:** `AZURE_AI_ENDPOINT`, `AZURE_AI_API_KEY`, `AZURE_AI_DEPLOYMENT`

**Instagram:** `INSTAGRAM_APP_ID`, `INSTAGRAM_APP_SECRET`, `INSTAGRAM_REDIRECT_URI`

**Social Content:** `SOCIAL_CONTENT_ENABLED`, `SOCIAL_CONTENT_INTERVAL`, `SOCIAL_CONTENT_INITIAL_DELAY`

**Media:** `MEDIA_DIR`, `BASE_URL` (read via `os.Getenv` in main.go)

Format: Each var with `=""` default (or actual default if known), inline comment explaining purpose. Read `internal/platform/config/defaults.go` for default values.

- [ ] **Step 2: Verify no secrets in file**

Run: `grep -i "secret\|password\|token" .env.example | grep -v '=""'`
Expected: No output (all secret values should be empty strings)

- [ ] **Step 3: Commit**

```bash
git add .env.example
git commit -m "docs: add .env.example with all 51 environment variables"
```

---

### Task 2: Create `docs/SCHEMA.md`

**Files:**
- Create: `docs/SCHEMA.md`

- [ ] **Step 1: Write schema doc**

Read all migration files in `internal/adapters/storage/sqlite/migrations/` (000001-000017) and create a curated schema reference.

Structure for each of the 22 tables:
```markdown
### `table_name`
One-line purpose.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
...

**Indexes:** list with columns and WHERE conditions
**Foreign Keys:** target table + cascade behavior
```

Tables to document (in dependency order):
1. `users` — Google OAuth users
2. `user_sessions` — Session management (FK → users)
3. `user_tokens` — OAuth tokens (FK → users, user_sessions)
4. `oauth_states` — CSRF state tokens
5. `allowed_emails` — Email allowlist
6. `favorites` — User card favorites (FK → users)
7. `price_history` — Historical price data
8. `api_calls` — API call tracking
9. `api_rate_limits` — Per-provider rate limit state
10. `price_refresh_queue` — Price refresh scheduling (FK → api_rate_limits)
11. `card_access_log` — Card access tracking
12. `campaigns` — Campaign definitions
13. `campaign_purchases` — Purchase records (FK → campaigns)
14. `campaign_sales` — Sale records (FK → campaign_purchases)
15. `invoices` — Invoice tracking
16. `cashflow_config` — Singleton cashflow settings
17. `card_id_mappings` — External provider ID cache
18. `sync_state` — Delta poll state
19. `revocation_flags` — Revocation tracking
20. `card_request_submissions` — CardHedger card requests
21. `discovery_failures` — Failed card discovery tracking
22. `market_snapshot_history` — Daily market snapshots
23. `population_history` — Grade population tracking
24. `cl_value_history` — Card Ladder value history
25. `advisor_cache` — AI advisor analysis cache
26. `ai_calls` — AI call metrics
27. `social_posts` — Social media posts
28. `social_post_cards` — Post-to-card mapping (FK → social_posts)
29. `instagram_config` — Singleton Instagram settings

Also document the 8 views: `stale_prices`, `api_usage_summary`, `api_hourly_distribution`, `api_daily_summary`, `active_sessions`, `expired_sessions`, `ai_usage_summary`, `ai_usage_by_operation`.

End with FK dependency graph.

- [ ] **Step 2: Commit**

```bash
git add docs/SCHEMA.md
git commit -m "docs: add database schema reference (22 tables, 8 views)"
```

---

### Task 3: Create `docs/API.md`

**Files:**
- Create: `docs/API.md`

- [ ] **Step 1: Write API doc**

Read `internal/adapters/httpserver/router.go` for all route registrations and handler files for request/response types. Create a detailed endpoint reference grouped by feature area.

For each endpoint group, document:
- Route, method, auth requirement (None/RequireAuth/RequireAdmin/RequireAPIKey)
- Request body or query parameters with types
- Response shape (JSON field names and types, read from handler `writeJSON` calls and domain types)
- Notable error responses

Groups (matching CLAUDE.md route table order):
1. Authentication (4 routes)
2. Health & Status (5 routes)
3. Favorites (5 routes)
4. Cards & Pricing (2 routes)
5. Price Hints (1 route)
6. Admin (10 routes)
7. Campaign CRUD (5 routes)
8. Campaign Purchases (4 routes)
9. Campaign Sales (2 routes)
10. Campaign Analytics (12 routes)
11. Global Purchases (9 routes)
12. Price Override & AI (4 routes)
13. Credit & Invoices (5 routes)
14. Portfolio (7 routes)
15. Utilities (2 routes)
16. AI Advisor (6 routes)
17. Social Content (8 routes)
18. Instagram (5 routes)
19. Pricing API v1 (3 routes)

Read the handler Go files for each group to get exact request/response JSON shapes. Reference domain types from `internal/domain/campaigns/types.go`, `internal/domain/pricing/types.go`, etc.

- [ ] **Step 2: Commit**

```bash
git add docs/API.md
git commit -m "docs: add API endpoint reference (~90 routes)"
```

---

### Task 4: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add API Route Summary Table**

After the "Frontend-Backend Integration" section and before "Troubleshooting", add a new `## API Routes` section with a compact grouped table. Format:

```markdown
## API Routes

See [docs/API.md](docs/API.md) for detailed request/response shapes.

**Middleware stack** (all routes): CORS → Gzip → Logging → Timing → Security Headers → Recovery → Rate Limiter

### Authentication
| Route | Method | Auth | Handler |
|-------|--------|------|---------|
| `/auth/google/login` | GET/POST | AuthRateLimit | `AuthHandlers.HandleGoogleLogin` |
...
```

Include all ~90 routes grouped into the same categories as docs/API.md.

- [ ] **Step 2: Add Database Schema Overview**

After API Routes, add `## Database Schema` with one-line-per-table summary:

```markdown
## Database Schema

See [docs/SCHEMA.md](docs/SCHEMA.md) for full column definitions and indexes.

| Table | Purpose | Key FKs |
|-------|---------|---------|
| `users` | Google OAuth users | — |
| `user_sessions` | Session management | → users |
...
```

List all 22 tables + note about 8 views.

- [ ] **Step 3: Add Domain Interface Catalog**

After Database Schema, add `## Domain Interfaces`:

```markdown
## Domain Interfaces

| Package | Interface | File | Methods | Purpose |
|---------|-----------|------|---------|---------|
| campaigns | CampaignCRUD | repository.go | 5 | Campaign lifecycle |
| campaigns | PurchaseRepository | repository.go | 27 | Purchase persistence |
...
```

All 31 interfaces across 9 packages.

- [ ] **Step 4: Add "How to..." Recipes**

After Domain Interfaces, add `## Common Recipes` with the 4 recipes from the spec (new endpoint, new scheduler, new domain interface, new migration). Each 3-5 steps. Keep concise — detailed walkthroughs are in `internal/README.md`.

- [ ] **Step 5: Add Config Loading Flow**

After Common Recipes, add `## Configuration`:

```markdown
## Configuration

**Precedence:** CLI flags > environment variables > `.env` file > defaults

- Loaded in `internal/platform/config/loader.go`
- Validated in `internal/platform/config/validation.go`
- OAuth: `config.LoadGoogleOAuthConfig()`, `config.LoadInstagramOAuthConfig()`
- See `.env.example` for all 51 environment variables with defaults
```

- [ ] **Step 6: Update Documentation section**

Add links to the new docs:
```markdown
- [Database Schema](docs/SCHEMA.md) - Table definitions, indexes, relationships
- [API Reference](docs/API.md) - All endpoints with request/response shapes
```

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add API routes, schema overview, interface catalog, recipes to CLAUDE.md"
```

---

### Task 5: Update `internal/README.md`

**Files:**
- Modify: `internal/README.md`

- [ ] **Step 1: Add "Adding a New HTTP Handler" walkthrough**

After the existing "Example: Adding a New Data Source" section (ends around line 323), add:

```markdown
### Example: Adding a New HTTP Handler

**Scenario**: Add a new API endpoint.

**Step 1**: Create handler method on the appropriate handler struct
\```go
// internal/adapters/httpserver/handlers/myfeature.go
func (h *MyHandler) HandleGetThing(w http.ResponseWriter, r *http.Request) {
    result, err := h.service.GetThing(r.Context())
    if err != nil {
        writeError(w, http.StatusInternalServerError, "Internal server error")
        return
    }
    writeJSON(w, http.StatusOK, result)
}
\```

**Step 2**: Register route in `router.go` with appropriate middleware
\```go
// internal/adapters/httpserver/router.go
mux.HandleFunc("GET /api/things", authRoute(myHandler.HandleGetThing))
\```

**Step 3**: Wire handler dependencies in `main.go` if new handler struct
\```go
myHandler := handlers.NewMyHandler(service, logger)
\```

**Step 4**: Add frontend types in `web/src/types/` if new response shape

**Step 5**: Update `docs/API.md` with request/response contract
```

- [ ] **Step 2: Add "Adding a New Scheduler" walkthrough**

```markdown
### Example: Adding a New Scheduler

**Scenario**: Add a background job that runs periodically.

**Step 1**: Create scheduler file
\```go
// internal/adapters/scheduler/my_job.go
func myJobLoop(ctx context.Context, deps MyJobDeps, logger observability.Logger) {
    // Job logic here
}
\```

**Step 2**: Register in `BuildGroup()` in `scheduler/group.go`
\```go
if cfg.MyJob.Enabled {
    group.Add("my-job", scheduler.RunLoop(scheduler.LoopConfig{
        Name:     "my-job",
        Interval: cfg.MyJob.Interval,
    }, func(ctx context.Context) { myJobLoop(ctx, deps, logger) }))
}
\```

**Step 3**: Add adapter wrappers in `main.go` if domain type conversion needed

**Step 4**: Document in `docs/SCHEDULERS.md`
```

- [ ] **Step 3: Add large file awareness note**

After the anti-patterns section, add:

```markdown
## Large File Awareness

These files exceed 500 LOC. Before adding code to them, consider whether the new code belongs in a separate file:

| File | LOC | Why it's large |
|------|-----|----------------|
| `adapters/storage/sqlite/purchases_repository.go` | ~400 | Purchase CRUD (split from original 802 LOC) |
| `adapters/storage/sqlite/purchases_repository_queries.go` | ~400 | Complex purchase queries |
| `adapters/clients/pricecharting/domain_adapter.go` | 637 | 6-strategy matching pipeline (cohesive) |
| `adapters/clients/fusionprice/fusion_provider.go` | 635 | Multi-source fusion (single purpose) |
| `domain/social/service_impl.go` | 731 | Social content orchestration |
| `domain/campaigns/service_analytics.go` | 609 | Campaign analytics computations |
```

Note: Update LOC numbers after Phase 2 splits.

- [ ] **Step 4: Commit**

```bash
git add internal/README.md
git commit -m "docs: add handler/scheduler walkthroughs and large file guide to internal README"
```

---

## Phase 2: Code Maintainability

### Task 6: Add `.golangci.yml`

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Write linter config**

Create `.golangci.yml` with the config from the spec:

```yaml
run:
  timeout: 5m
  tests: true

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - ineffassign
    - gosimple
    - gocritic
    - misspell
    - revive

linters-settings:
  gocritic:
    enabled-tags:
      - diagnostic
      - style
    disabled-checks:
      - hugeParam
  revive:
    rules:
      - name: exported
        disabled: true
      - name: unexported-return
        disabled: true

issues:
  exclude-dirs:
    - web/node_modules
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
        - gocritic
    - path: testutil/mocks/
      linters:
        - unused
        - revive
  max-issues-per-linter: 0
  max-same-issues: 0
```

- [ ] **Step 2: Run linter and fix any issues**

Run: `golangci-lint run ./...`

If there are failures, fix them. Focus on real issues (misspellings, unchecked errors at system boundaries). If a linter rule is too noisy for existing code, add a targeted exclusion rather than fixing hundreds of false positives.

Expected: Clean pass (or exclusions added for existing patterns).

- [ ] **Step 3: Commit**

```bash
git add .golangci.yml
# Include any source fixes if linter found real issues
git commit -m "chore: add golangci-lint config with curated ruleset"
```

---

### Task 7: Split `purchases_repository.go`

**Files:**
- Modify: `internal/adapters/storage/sqlite/purchases_repository.go`
- Create: `internal/adapters/storage/sqlite/purchases_repository_queries.go`

- [ ] **Step 1: Create `purchases_repository_queries.go`**

Move these methods from `purchases_repository.go` to a new `purchases_repository_queries.go` file (same package, same `*CampaignsRepository` receiver):

**Methods to move** (cut from purchases_repository.go, paste into new file):
- `GetPurchaseByCertNumber` (line 183)
- `GetPurchasesByGraderAndCertNumbers` (line 198)
- `GetPurchasesByCertNumbers` (line 246)
- `GetPurchasesByIDs` (line 302)
- `UpdateExternalPurchaseFields` (line 399)
- `UpdatePurchaseMarketSnapshot` (line 428)
- `ListSnapshotPurchasesByStatus` (line 471)
- `UpdatePurchaseSnapshotStatus` (line 484)
- `UpdatePurchasePSAFields` (line 502)
- `ListPurchasesMissingImages` (line 525)
- `UpdatePurchaseImageURLs` (line 558)
- `UpdatePurchasePriceOverride` (line 593)
- `UpdatePurchaseAISuggestion` (line 616)
- `GetPriceOverrideStats` (line 635)
- `ClearPurchaseAISuggestion` (line 667)
- `AcceptAISuggestion` (line 685)
- `SetEbayExportFlag` (line 722)
- `ClearEbayExportFlags` (line 739)
- `ListEbayFlaggedPurchases` (line 769)

New file needs the same package declaration and imports used by those methods:
```go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)
```

**Methods that stay** in `purchases_repository.go`:
- `CreatePurchase`
- `GetPurchase`
- `ListPurchasesByCampaign`
- `ListUnsoldPurchases`
- `ListAllUnsoldPurchases`
- `ListUnsoldCards` (+ `UnsoldCardInfo` type)
- `CountPurchasesByCampaign`
- `UpdatePurchaseCLValue`
- `UpdatePurchaseCardMetadata`
- `UpdatePurchaseGrade`
- `UpdatePurchaseCampaign`
- `UpdatePurchaseCardYear`
- `DeletePurchase` (if exists, check for it)

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/adapters/storage/sqlite/...`
Expected: Clean build

- [ ] **Step 3: Run tests**

Run: `go test ./internal/adapters/storage/sqlite/...`
Expected: All tests pass (no behavior change, just file reorganization)

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/purchases_repository.go internal/adapters/storage/sqlite/purchases_repository_queries.go
git commit -m "refactor: split purchases_repository into CRUD and complex queries"
```

---

### Task 8: Extract CSV parsing to domain layer

**Files:**
- Create: `internal/domain/campaigns/parse_error.go`
- Create: `internal/domain/campaigns/parse_helpers.go`
- Create: `internal/domain/campaigns/parse_cl.go`
- Create: `internal/domain/campaigns/parse_psa.go`
- Create: `internal/domain/campaigns/parse_shopify.go`
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports.go`

This is the most complex refactoring task. The approach: create the new domain files first, then update the handler to call them.

- [ ] **Step 1: Create `parse_error.go`**

```go
package campaigns

// ParseError represents a non-fatal parsing issue in a specific CSV row.
type ParseError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}
```

- [ ] **Step 2: Create `parse_helpers.go`**

Move `buildHeaderMap`, `findHeaderRow`, and `parseCurrencyString` from the handler into domain. Also move `normalizePSACert` and its regex patterns.

```go
package campaigns

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// psaCertFromSKU extracts a PSA cert number from a SKU like "PSA-192060238".
var psaCertFromSKU = regexp.MustCompile(`(?i)^PSA-(\d+)$`)

// digitsOnly matches a string that is entirely digits.
var digitsOnly = regexp.MustCompile(`^\d+$`)

// NormalizePSACert returns a digits-only cert number from a raw field value.
func NormalizePSACert(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if digitsOnly.MatchString(s) {
		return s
	}
	if m := psaCertFromSKU.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return ""
}

// BuildHeaderMap creates a lowercase header name -> column index map.
func BuildHeaderMap(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, col := range header {
		m[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return m
}

// FindPSAHeaderRow scans the first few rows for known PSA column names.
// Returns the header row index, or -1 if not found.
func FindPSAHeaderRow(rows [][]string) int {
	knownColumns := map[string]bool{
		"cert number":   true,
		"listing title": true,
		"grade":         true,
		"price paid":    true,
	}
	for i, row := range rows {
		if i > 5 {
			break
		}
		headerMap := BuildHeaderMap(row)
		matches := 0
		for col := range knownColumns {
			if _, ok := headerMap[col]; ok {
				matches++
			}
		}
		if matches >= 3 {
			return i
		}
	}
	return -1
}

// ParseCurrencyString parses a currency string like "$1,234.56" into a float64.
func ParseCurrencyString(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, fmt.Errorf("empty currency string")
	}
	return strconv.ParseFloat(s, 64)
}
```

Note: Check if `parseCurrencyString` already exists in the handler file or elsewhere — the handler uses it but it may be defined in a different handler file. Search for it with `grep -rn "func parseCurrencyString" internal/`.

- [ ] **Step 3: Create `parse_cl.go`**

Extract the CL CSV parsing logic from `HandleGlobalRefreshCL` and `HandleGlobalImportCL` handlers:

```go
package campaigns

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseCLRefreshRows parses Card Ladder CSV records for CL value refresh.
// Returns parsed rows and any non-fatal parse errors.
func ParseCLRefreshRows(records [][]string) ([]CLExportRow, []ParseError, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])
	if _, exists := headerMap["slab serial #"]; !exists {
		return nil, nil, fmt.Errorf("missing required column: \"slab serial #\"")
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	getField := func(rec []string, idx int) string {
		if idx >= 0 && idx < len(rec) {
			return strings.TrimSpace(rec[idx])
		}
		return ""
	}

	var rows []CLExportRow
	var errs []ParseError
	for i, rec := range records[1:] {
		slabSerial := getField(rec, colIdx("slab serial #"))
		if slabSerial == "" {
			continue
		}

		cvStr := getField(rec, colIdx("current value"))
		if cvStr == "" {
			errs = append(errs, ParseError{Row: i + 2, Field: "current value",
				Message: fmt.Sprintf("missing current value for slab serial %s", slabSerial)})
			continue
		}
		currentValue, err := strconv.ParseFloat(cvStr, 64)
		if err != nil {
			errs = append(errs, ParseError{Row: i + 2, Field: "current value",
				Message: fmt.Sprintf("invalid current value %q for slab serial %s", cvStr, slabSerial)})
			continue
		}

		var population int
		if pop := getField(rec, colIdx("population")); pop != "" {
			population, _ = strconv.Atoi(pop)
		}

		rows = append(rows, CLExportRow{
			SlabSerial:   slabSerial,
			Card:         getField(rec, colIdx("card")),
			Set:          getField(rec, colIdx("set")),
			Number:       getField(rec, colIdx("number")),
			CurrentValue: currentValue,
			Population:   population,
		})
	}
	return rows, errs, nil
}

// ParseCLImportRows parses Card Ladder CSV records for full import (with investment + date).
func ParseCLImportRows(records [][]string) ([]CLExportRow, []ParseError, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])
	required := []string{"slab serial #", "investment", "current value"}
	for _, hdr := range required {
		if _, ok := headerMap[hdr]; !ok {
			return nil, nil, fmt.Errorf("missing required column: %q", hdr)
		}
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	getField := func(rec []string, idx int) string {
		if idx >= 0 && idx < len(rec) {
			return strings.TrimSpace(rec[idx])
		}
		return ""
	}

	var rows []CLExportRow
	var errs []ParseError
	for i, rec := range records[1:] {
		rowNum := i + 2
		slabSerial := getField(rec, colIdx("slab serial #"))
		if slabSerial == "" {
			errs = append(errs, ParseError{Row: rowNum, Field: "slab serial #", Message: "missing Slab Serial #"})
			continue
		}

		investment, err := strconv.ParseFloat(getField(rec, colIdx("investment")), 64)
		if err != nil {
			errs = append(errs, ParseError{Row: rowNum, Field: "investment",
				Message: fmt.Sprintf("invalid Investment %q", getField(rec, colIdx("investment")))})
			continue
		}

		cvStr := getField(rec, colIdx("current value"))
		if cvStr == "" {
			errs = append(errs, ParseError{Row: rowNum, Field: "current value", Message: "missing Current Value"})
			continue
		}
		currentValue, err := strconv.ParseFloat(cvStr, 64)
		if err != nil {
			errs = append(errs, ParseError{Row: rowNum, Field: "current value",
				Message: fmt.Sprintf("invalid Current Value %q", cvStr)})
			continue
		}

		var population int
		if pop := getField(rec, colIdx("population")); pop != "" {
			population, _ = strconv.Atoi(pop)
		}

		datePurchased := getField(rec, colIdx("date purchased"))
		if datePurchased != "" {
			converted, dateErr := ParseCLDate(datePurchased)
			if dateErr != nil {
				errs = append(errs, ParseError{Row: rowNum, Field: "date purchased",
					Message: fmt.Sprintf("invalid Date Purchased %q: expected M/D/YYYY", datePurchased)})
				continue
			}
			datePurchased = converted
		}

		rows = append(rows, CLExportRow{
			DatePurchased: datePurchased,
			Card:          getField(rec, colIdx("card")),
			Player:        getField(rec, colIdx("player")),
			Set:           getField(rec, colIdx("set")),
			Number:        getField(rec, colIdx("number")),
			Condition:     getField(rec, colIdx("condition")),
			Investment:    investment,
			CurrentValue:  currentValue,
			SlabSerial:    slabSerial,
			Population:    population,
		})
	}
	return rows, errs, nil
}
```

- [ ] **Step 4: Create `parse_psa.go`**

Extract PSA CSV parsing from `HandleGlobalImportPSA`:

```go
package campaigns

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePSAExportRows parses PSA communication spreadsheet CSV records.
func ParsePSAExportRows(records [][]string) ([]PSAExportRow, []ParseError, error) {
	headerIdx := FindPSAHeaderRow(records)
	if headerIdx < 0 {
		return nil, nil, fmt.Errorf("could not find PSA header row (expected columns: cert number, listing title, grade)")
	}

	headerMap := BuildHeaderMap(records[headerIdx])
	dataRows := records[headerIdx+1:]

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	getField := func(rec []string, idx int) string {
		if idx >= 0 && idx < len(rec) {
			return strings.TrimSpace(rec[idx])
		}
		return ""
	}

	var rows []PSAExportRow
	var errs []ParseError
	for i, rec := range dataRows {
		rowNum := headerIdx + 2 + i

		certNumber := getField(rec, colIdx("cert number"))
		if certNumber == "" {
			continue
		}

		var pricePaid float64
		if pp := getField(rec, colIdx("price paid")); pp != "" {
			v, parseErr := ParseCurrencyString(pp)
			if parseErr != nil {
				errs = append(errs, ParseError{Row: rowNum, Field: "price paid",
					Message: fmt.Sprintf("invalid price paid %q: %v", pp, parseErr)})
				continue
			}
			pricePaid = v
		}

		var grade float64
		if g := getField(rec, colIdx("grade")); g != "" {
			v, parseErr := strconv.ParseFloat(g, 64)
			if parseErr != nil {
				errs = append(errs, ParseError{Row: rowNum, Field: "grade",
					Message: fmt.Sprintf("invalid grade %q: %v", g, parseErr)})
				continue
			}
			grade = v
		}

		dateStr := getField(rec, colIdx("date"))
		purchaseDate := ""
		if dateStr != "" {
			converted, dateErr := ParsePSADate(dateStr)
			if dateErr != nil {
				errs = append(errs, ParseError{Row: rowNum, Field: "date",
					Message: fmt.Sprintf("invalid date %q: %v", dateStr, dateErr)})
				continue
			}
			purchaseDate = converted
		}

		invoiceDateStr := getField(rec, colIdx("invoice date"))
		invoiceDate := ""
		if invoiceDateStr != "" {
			converted, dateErr := ParsePSADate(invoiceDateStr)
			if dateErr != nil {
				errs = append(errs, ParseError{Row: rowNum, Field: "invoice date",
					Message: fmt.Sprintf("invalid invoice date %q: %v", invoiceDateStr, dateErr)})
				continue
			}
			invoiceDate = converted
		}

		wasRefunded := false
		refundedStr := strings.ToLower(getField(rec, colIdx("was refunded?")))
		if refundedStr == "yes" || refundedStr == "true" || refundedStr == "1" {
			wasRefunded = true
		}

		rows = append(rows, PSAExportRow{
			Date:           purchaseDate,
			Category:       getField(rec, colIdx("category")),
			CertNumber:     certNumber,
			ListingTitle:   getField(rec, colIdx("listing title")),
			Grade:          grade,
			PricePaid:      pricePaid,
			PurchaseSource: getField(rec, colIdx("purchase source")),
			VaultStatus:    getField(rec, colIdx("vault status")),
			InvoiceDate:    invoiceDate,
			WasRefunded:    wasRefunded,
			FrontImageURL:  getField(rec, colIdx("front image url")),
			BackImageURL:   getField(rec, colIdx("back image url")),
		})
	}

	if len(rows) == 0 && len(errs) == 0 {
		return nil, nil, fmt.Errorf("no valid PSA data rows found in CSV")
	}

	return rows, errs, nil
}
```

- [ ] **Step 5: Create `parse_shopify.go`**

Extract Shopify CSV parsing from `HandleGlobalImportExternal`. This is the most complex parser due to handle-based consolidation:

```go
package campaigns

import (
	"fmt"
	"strings"
)

// ParseShopifyExportRows parses Shopify product export CSV records.
// Handles multi-row products by Handle+CertNumber consolidation.
func ParseShopifyExportRows(records [][]string) ([]ShopifyExportRow, []ParseError, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])
	if _, hasHandle := headerMap["handle"]; !hasHandle {
		return nil, nil, fmt.Errorf("CSV is missing required column: handle")
	}
	if _, hasTitle := headerMap["title"]; !hasTitle {
		return nil, nil, fmt.Errorf("CSV is missing required column: title")
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	getField := func(rec []string, idx int) string {
		if idx >= 0 && idx < len(rec) {
			return strings.TrimSpace(rec[idx])
		}
		return ""
	}

	type product struct {
		row ShopifyExportRow
	}
	products := make(map[string]*product)
	backImages := make(map[string]string)
	var order []string
	var errs []ParseError

	for rowIdx, rec := range records[1:] {
		handle := getField(rec, colIdx("handle"))
		if handle == "" {
			continue
		}

		title := getField(rec, colIdx("title"))
		imageURL := getField(rec, colIdx("image src"))

		if title == "" {
			if imageURL != "" {
				if _, exists := backImages[handle]; !exists {
					backImages[handle] = imageURL
				}
			}
			continue
		}

		certNumber := NormalizePSACert(getField(rec, colIdx("cert number")))
		if certNumber == "" {
			certNumber = NormalizePSACert(getField(rec, colIdx("cert")))
		}
		if certNumber == "" {
			certNumber = NormalizePSACert(getField(rec, colIdx("sku")))
		}
		if certNumber == "" {
			continue
		}

		productKey := handle + "|" + certNumber
		if _, exists := products[productKey]; exists {
			if imageURL != "" {
				if _, hasBack := backImages[handle]; !hasBack {
					backImages[handle] = imageURL
				}
			}
			continue
		}

		tags := getField(rec, colIdx("tags"))
		cardName, cardNumber, setName, _, tagErr := ParseShopifyTags(tags)
		if tagErr != nil {
			// Fall through — tags are optional
			_ = tagErr
		}

		if cardName == "" {
			cardName = ExtractCardNameFromTitle(title)
		}

		grader, gradeValue := ExtractGraderAndGrade(title)
		if grader == "" {
			grader = "PSA"
		}

		var variantPrice float64
		priceField := getField(rec, colIdx("variant price"))
		if priceField == "" {
			priceField = getField(rec, colIdx("price"))
		}
		if priceField != "" {
			v, err := ParseCurrencyString(priceField)
			if err != nil {
				errs = append(errs, ParseError{Row: rowIdx + 2, Field: "price",
					Message: fmt.Sprintf("invalid price %q for handle %s: %v", priceField, handle, err)})
				continue
			}
			variantPrice = v
		}

		var costPerItem float64
		if cp := getField(rec, colIdx("cost per item")); cp != "" {
			v, err := ParseCurrencyString(cp)
			if err != nil {
				errs = append(errs, ParseError{Row: rowIdx + 2, Field: "cost per item",
					Message: fmt.Sprintf("invalid cost per item %q for handle %s: %v", cp, handle, err)})
				continue
			}
			costPerItem = v
		}

		products[productKey] = &product{
			row: ShopifyExportRow{
				Handle:        handle,
				CertNumber:    certNumber,
				Title:         title,
				CardName:      cardName,
				CardNumber:    cardNumber,
				SetName:       setName,
				Grader:        grader,
				GradeValue:    gradeValue,
				VariantPrice:  variantPrice,
				CostPerItem:   costPerItem,
				FrontImageURL: imageURL,
			},
		}
		order = append(order, productKey)
	}

	var rows []ShopifyExportRow
	for _, key := range order {
		p := products[key]
		if img, ok := backImages[p.row.Handle]; ok {
			p.row.BackImageURL = img
		}
		rows = append(rows, p.row)
	}

	return rows, errs, nil
}
```

- [ ] **Step 6: Update handler to use domain parsers**

Modify `campaigns_imports.go` to:
1. Remove `buildHeaderMap`, `findHeaderRow`, `normalizePSACert`, `psaCertFromSKU`, `digitsOnly` — now in domain
2. Replace inline parsing in each `Handle*` method with calls to domain `Parse*Rows` functions
3. Keep `parseGlobalCSVUpload`, HTTP response writing, `triggerCardDiscovery`, `CardDiscoverer` interface

For the error handling change: the old handlers returned HTTP errors for parse failures. The new domain parsers return `[]ParseError` and `error`. The handler should check `error` for fatal issues (missing required columns) and convert to HTTP 400 responses. Non-fatal `ParseError`s can be logged or returned alongside results.

**Important**: The CL refresh handler currently returns HTTP 400 on *any* row parse error (stopping on first error). The domain parser collects all errors and continues. The handler needs to preserve the same behavior for backwards compatibility — check if `len(errs) > 0` and return the first error as HTTP 400, or change to a softer approach. **Decision: preserve the stop-on-first-error behavior** by having the handler check `errs` from the domain parser and return the first one as HTTP 400. This is a behavior-preserving refactoring.

- [ ] **Step 7: Verify compilation**

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 8: Run all tests**

Run: `go test ./...`
Expected: All existing tests pass

- [ ] **Step 9: Commit**

```bash
git add internal/domain/campaigns/parse_*.go internal/adapters/httpserver/handlers/campaigns_imports.go
git commit -m "refactor: extract CSV parsing from handler to domain layer"
```

---

### Task 9: Split `cmd/slabledger/main.go`

**Files:**
- Create: `cmd/slabledger/init.go`
- Modify: `cmd/slabledger/main.go`

- [ ] **Step 1: Create `init.go` with init functions**

Create `cmd/slabledger/init.go` containing 5 extracted functions. Each function is a cut-paste of the corresponding section from `runServer()`, wrapped in a function signature.

**Package and imports**: Same `package main`, with the imports that the extracted code needs (subset of main.go imports).

**Functions to create:**

1. `initializePriceProviders()` — lines 376-418 of current main.go
2. `initializeCampaignsService()` — lines 420-453
3. `initializeAdvisorService()` — lines 461-486
4. `initializeSocialService()` — lines 494-527
5. `initializeSchedulers()` — lines 529-560, using a `schedulerDeps` struct

For `initializeSchedulers`, define:
```go
type schedulerDeps struct {
	cfg                  *config.Config
	logger               observability.Logger
	priceRepo            *sqlite.PriceRepository
	priceProvider        *fusionprice.FusionProvider
	cardProvider         *tcgdex.TCGdex
	authService          auth.Service
	cardHedgerClient     *cardhedger.Client
	syncStateRepo        *sqlite.SyncStateRepository
	cardIDMappingRepo    *sqlite.CardIDMappingRepository
	discoveryFailureRepo *sqlite.DiscoveryFailureRepository
	favoritesRepo        *sqlite.FavoritesRepository
	campaignsRepo        *sqlite.CampaignsRepository
	campaignsService     campaigns.Service
	advisorService       advisor.Service
	advisorCacheRepo     *sqlite.AdvisorCacheRepository
	aiCallRepo           *sqlite.AICallRepository
	socialService        social.Service
	igTokenRefresher     scheduler.InstagramTokenRefresher
}
```

- [ ] **Step 2: Update `runServer()` to call init functions**

Replace the extracted code blocks in `runServer()` with calls:

```go
// Initialize price providers
priceProvImpl, pokemonPriceClientImpl, cardHedgerClientImpl, pcProvider, err := initializePriceProviders(cfg, appCache, logger, cardProvImpl)
if err != nil {
    return err
}
defer func() {
    if err := pcProvider.Close(); err != nil {
        logger.Warn(ctx, "failed to close PriceCharting provider", observability.Err(err))
    }
}()

// Initialize campaigns
campaignsService, cardRequestRepo, err := initializeCampaignsService(cfg, logger, db, priceProvImpl, cardHedgerClientImpl, priceRepo)
if err != nil {
    return err
}

// Initialize advisor
azureAIClient, advisorService, advisorCacheRepo, err := initializeAdvisorService(cfg, logger, db, aiCallRepo, campaignsService)
if err != nil {
    return err
}

// Initialize social
socialService, igClient, igStore, igTokenRefresher, err := initializeSocialService(cfg, logger, db, azureAIClient, aiCallRepo)
if err != nil {
    return err
}

// Build and start schedulers
schedulerResult, cancelScheduler, err := initializeSchedulers(ctx, schedulerDeps{...})
if err != nil {
    return err
}
```

Adjust return types of each init function to match what `runServer()` needs downstream. The exact signatures will depend on reading the code carefully — the key is that `runServer()` becomes a readable sequence of init calls.

- [ ] **Step 3: Verify compilation**

Run: `go build ./cmd/slabledger/...`
Expected: Clean build

- [ ] **Step 4: Run server smoke test**

Run: `go build -o /tmp/slabledger ./cmd/slabledger && /tmp/slabledger --help`
Expected: Help text prints correctly

- [ ] **Step 5: Verify line counts**

Run: `wc -l cmd/slabledger/main.go cmd/slabledger/init.go`
Expected: main.go under 400 LOC, init.go contains the rest

- [ ] **Step 6: Commit**

```bash
git add cmd/slabledger/main.go cmd/slabledger/init.go
git commit -m "refactor: extract init functions from main.go to init.go"
```

---

## Phase 3: Test Coverage

### Task 10: Advisor service tests

**Files:**
- Create: `internal/domain/advisor/service_test.go`

- [ ] **Step 1: Write test file with mocks and tests**

Create `internal/domain/advisor/service_test.go` with inline mocks and test cases.

The advisor service has these key behaviors to test:
- `NewService` creates a service with LLM provider and tool executor
- `GenerateDigest` calls `llm.StreamCompletion` with digest system prompt and tool definitions
- `AnalyzeCampaign` formats campaign ID into user prompt
- `AssessPurchase` formats purchase details into user prompt
- `CollectDigest` returns full content string (non-streaming)
- Tool calls: when LLM returns tool calls, executor is called, results fed back
- Error handling: LLM errors propagate, tool errors are wrapped in JSON error objects
- Max tool rounds: exceeded rounds returns error

Key mocks needed:
- `mockLLMProvider` implementing `ai.LLMProvider` — `StreamCompletion` that calls the callback with configurable chunks
- `mockToolExecutor` implementing `ai.FilteredToolExecutor` — returns predefined definitions and results
- `mockCacheStore` implementing `advisor.CacheStore` — stores/retrieves cached analysis

Reference the source at `internal/domain/advisor/service_impl.go` for exact behavior.

Test cases:
- `TestGenerateDigest_NoToolCalls` — LLM returns content without tool calls, verify stream events emitted
- `TestGenerateDigest_WithToolCalls` — LLM returns tool calls first round, content second round
- `TestGenerateDigest_LLMError` — LLM returns error, verify it propagates
- `TestAnalyzeCampaign_FormatsPrompt` — Verify campaign ID appears in user prompt
- `TestAssessPurchase_FormatsPrompt` — Verify card details appear in user prompt
- `TestCollectDigest_ReturnsContent` — Non-streaming path returns full string
- `TestMaxToolRounds_Exceeded` — Set maxToolRounds=1, LLM always returns tool calls, verify error
- `TestToolExecutionError_WrappedInJSON` — Tool returns error, verify JSON error sent to LLM

- [ ] **Step 2: Run tests**

Run: `go test -v ./internal/domain/advisor/...`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/domain/advisor/service_test.go
git commit -m "test: add advisor service tests (8 test cases)"
```

---

### Task 11: Advisor tool executor tests

**Files:**
- Create: `internal/adapters/advisortool/executor_test.go`

- [ ] **Step 1: Write test file**

Create `internal/adapters/advisortool/executor_test.go`.

The `CampaignToolExecutor` has 21 registered tools. Tests should verify:
- `Definitions()` returns all 21 tools
- `DefinitionsFor()` filters correctly
- `Execute()` routes to correct handler
- Unknown tool name returns error
- Invalid JSON arguments return error
- Service errors propagate

Mock: `mockCampaignsService` — only needs to implement the methods actually called by the tools being tested. Use a struct with function fields (same pattern as `testutil/mocks/`).

Test cases:
- `TestDefinitions_Count` — verify 21 tools registered
- `TestDefinitionsFor_Subset` — request 3 names, get 3 definitions
- `TestDefinitionsFor_Empty` — request 0 names, get 0 definitions
- `TestExecute_UnknownTool` — returns "unknown tool: xyz" error
- `TestExecute_InvalidJSON` — returns "invalid arguments" error
- `TestExecute_ListCampaigns` — calls svc.ListCampaigns, returns JSON
- `TestExecute_GetCampaignPNL` — extracts campaignId, calls svc.GetCampaignPNL
- `TestExecute_GetPortfolioHealth` — no args needed, calls svc.GetPortfolioHealth
- `TestExecute_ServiceError` — service returns error, execute returns error

- [ ] **Step 2: Run tests**

Run: `go test -v ./internal/adapters/advisortool/...`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/advisortool/executor_test.go
git commit -m "test: add advisor tool executor tests (9 test cases)"
```

---

### Task 12: Instagram client tests

**Files:**
- Create: `internal/adapters/clients/instagram/client_test.go`

- [ ] **Step 1: Write test file**

Create `internal/adapters/clients/instagram/client_test.go` using `httptest.NewServer` to mock the Instagram Graph API.

The client makes HTTP calls to:
- `POST /oauth/access_token` — short-lived token exchange
- `GET /access_token` — long-lived token exchange
- `GET /{userID}?fields=username` — get username
- `POST /{igUserID}/media` — create item containers and carousel containers
- `GET /{containerID}?fields=status_code` — poll container status
- `POST /{igUserID}/media_publish` — publish

Test server should route by path and return JSON responses.

Test cases:
- `TestPublishCarousel_Success` — 2 images: creates 2 item containers, 1 carousel container, polls FINISHED, publishes
- `TestPublishCarousel_SingleImage` — 1 image delegates to PublishSingleImage
- `TestPublishCarousel_ContainerCreateError` — API returns error JSON on item container creation
- `TestPublishCarousel_StatusError` — poll returns ERROR status
- `TestRefreshToken_Success` — returns new token with expiry
- `TestRefreshToken_Error` — API returns error

- [ ] **Step 2: Run tests**

Run: `go test -v ./internal/adapters/clients/instagram/...`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/clients/instagram/client_test.go
git commit -m "test: add Instagram client tests (6 test cases)"
```

---

### Task 13: CSV parser tests

**Files:**
- Create: `internal/domain/campaigns/parse_helpers_test.go`
- Create: `internal/domain/campaigns/parse_cl_test.go`
- Create: `internal/domain/campaigns/parse_psa_test.go`
- Create: `internal/domain/campaigns/parse_shopify_test.go`

- [ ] **Step 1: Write `parse_helpers_test.go`**

```go
package campaigns

import "testing"

func TestBuildHeaderMap(t *testing.T) {
	headers := []string{"Name", " Set ", "NUMBER"}
	m := BuildHeaderMap(headers)
	if m["name"] != 0 { t.Errorf("expected name=0, got %d", m["name"]) }
	if m["set"] != 1 { t.Errorf("expected set=1, got %d", m["set"]) }
	if m["number"] != 2 { t.Errorf("expected number=2, got %d", m["number"]) }
}

func TestFindPSAHeaderRow_Found(t *testing.T) {
	rows := [][]string{
		{"some", "junk", "row"},
		{"Cert Number", "Listing Title", "Grade", "Price Paid"},
		{"12345", "Charizard", "10", "$100"},
	}
	idx := FindPSAHeaderRow(rows)
	if idx != 1 { t.Errorf("expected 1, got %d", idx) }
}

func TestFindPSAHeaderRow_NotFound(t *testing.T) {
	rows := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
	idx := FindPSAHeaderRow(rows)
	if idx != -1 { t.Errorf("expected -1, got %d", idx) }
}

func TestNormalizePSACert(t *testing.T) {
	tests := []struct{ in, want string }{
		{"12345", "12345"},
		{"PSA-12345", "12345"},
		{"psa-99999", "99999"},
		{" 12345 ", "12345"},
		{"", ""},
		{"abc", ""},
	}
	for _, tt := range tests {
		got := NormalizePSACert(tt.in)
		if got != tt.want { t.Errorf("NormalizePSACert(%q) = %q, want %q", tt.in, got, tt.want) }
	}
}

func TestParseCurrencyString(t *testing.T) {
	tests := []struct{ in string; want float64; wantErr bool }{
		{"$1,234.56", 1234.56, false},
		{"100.00", 100.00, false},
		{"$0.99", 0.99, false},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseCurrencyString(tt.in)
		if (err != nil) != tt.wantErr { t.Errorf("ParseCurrencyString(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr) }
		if !tt.wantErr && got != tt.want { t.Errorf("ParseCurrencyString(%q) = %v, want %v", tt.in, got, tt.want) }
	}
}
```

- [ ] **Step 2: Write `parse_cl_test.go`**

Test `ParseCLRefreshRows` and `ParseCLImportRows` with:
- Happy path with valid data
- Missing required column returns error
- Invalid current value row collected in parse errors
- Empty slab serial rows skipped

- [ ] **Step 3: Write `parse_psa_test.go`**

Test `ParsePSAExportRows` with:
- Happy path with header on row 0
- Header on row 2 (offset detection)
- Invalid grade collected in parse errors
- No valid rows returns error

- [ ] **Step 4: Write `parse_shopify_test.go`**

Test `ParseShopifyExportRows` with:
- Happy path with single product
- Handle consolidation (two rows same handle, different certs)
- Back image merging from variant-only rows
- Missing handle column returns error
- No PSA cert rows all skipped

- [ ] **Step 5: Run all parser tests**

Run: `go test -v ./internal/domain/campaigns/... -run "Parse|Normalize|Build|Find|Currency"`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/parse_*_test.go
git commit -m "test: add CSV parser tests (helpers, CL, PSA, Shopify)"
```

---

### Task 14: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test -race -timeout 10m ./...`
Expected: All tests pass including new ones

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: Clean pass

- [ ] **Step 3: Run frontend tests**

Run: `cd web && npm test`
Expected: All tests pass (no frontend changes, but verify no breakage)

- [ ] **Step 4: Verify line counts on split files**

Run: `wc -l cmd/slabledger/main.go cmd/slabledger/init.go internal/adapters/httpserver/handlers/campaigns_imports.go internal/adapters/storage/sqlite/purchases_repository.go internal/adapters/storage/sqlite/purchases_repository_queries.go`

Expected:
- `main.go` < 400 LOC
- `campaigns_imports.go` < 400 LOC
- `purchases_repository.go` < 500 LOC
- `purchases_repository_queries.go` < 500 LOC

- [ ] **Step 5: Commit any fixes**

If any tests or lints failed, fix and commit.

---

## Task Dependency Graph

```
Task 1 (.env.example) ─────┐
Task 2 (SCHEMA.md) ────────┤
Task 3 (API.md) ───────────┼── Task 4 (CLAUDE.md) ── Task 5 (README.md)
                            │
Task 6 (.golangci.yml) ─────┤
Task 7 (repo split) ────────┤
Task 8 (CSV extract) ───────┼── Task 13 (parser tests)
Task 9 (main.go split) ─────┤
                            │
Task 10 (advisor tests) ────┤
Task 11 (executor tests) ───┼── Task 14 (final verification)
Task 12 (instagram tests) ──┘
```

Tasks 1-3 are independent and can run in parallel.
Task 4 depends on 1-3 (references their output).
Tasks 6-9 are independent code changes.
Tasks 10-13 are independent test additions.
Task 14 is the final gate.
