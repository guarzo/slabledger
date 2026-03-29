# Codebase Maintainability & AI Agent Experience

**Date:** 2026-03-26
**Scope:** Documentation improvements, code splits, test coverage, linter config
**Approach:** Docs-first (Phase 1), then code changes (Phase 2), then tests (Phase 3)

## Context

SlabLedger is a well-architected hexagonal Go codebase with strong existing documentation. This spec addresses gaps that hurt maintainability and make it harder for AI agents to navigate and modify the code confidently.

**Key findings from exploration:**
- 6 production files exceed 600 LOC with tangled responsibilities
- `main.go` (704 LOC) does all wiring in a single `runServer()` function
- `campaigns_imports.go` (785 LOC) mixes HTTP concerns with CSV parsing business logic
- 3 subsystems have zero tests: `advisor/` (672 LOC), `advisortool/` (545 LOC), `instagram/` (7 files)
- No database schema reference, API endpoint catalog, or `.env.example`
- No golangci-lint config checked in (non-deterministic linting)
- CLAUDE.md is good but missing interface catalog, config flow, and recipes

## Phase 1: Documentation & AI Agent Experience

Goal: Give AI agents (and humans) the reference material needed to navigate the codebase without guessing.

### 1.1 CLAUDE.md Additions

Add the following sections to the existing CLAUDE.md. These are compact, scannable references — not exhaustive docs (those go in dedicated files).

#### API Route Summary Table

Add a grouped route table covering all ~80 endpoints. Format:

```
| Route | Method | Auth | Handler |
```

Grouped by: Auth, Health/Admin, Favorites, Cards/Pricing, Campaigns CRUD, Campaigns Analytics, Portfolio, Purchases (global), AI Advisor, Social, Instagram, Pricing API (v1).

Include middleware notes: global middleware stack order, auth rate limiter (10 req/sec), pricing API rate limiter (60 req/min global).

#### Database Schema Overview

Add a brief schema section with one line per table (22 tables total):

```
| Table | Purpose | Key Columns | Foreign Keys |
```

Link to `docs/SCHEMA.md` for full reference. Note: 8 views also exist (stale_prices, api_usage_summary, etc.).

#### Domain Interface Catalog

Add a grouped summary:

```
| Package | Interface | File | Methods | Purpose |
```

9 domain packages, 31 interfaces, 298 total methods. Group by: campaigns (14), pricing (6), auth (2), social (2), advisor (2), ai (3), cards (2), fusion (3), favorites (2), observability (1).

#### "How to..." Recipes

Add concise 3-5 step recipes for common tasks:

**Add a new API endpoint:**
1. Add handler method in `internal/adapters/httpserver/handlers/`
2. Register route in `router.go` with appropriate auth middleware
3. Update `docs/API.md` with request/response contract
4. If new handler struct needed, wire in `main.go`

**Add a new scheduler:**
1. Create scheduler file in `internal/adapters/scheduler/`
2. Implement the `scheduler.Scheduler` interface (or use `RunLoop`)
3. Add to `BuildGroup()` in `scheduler/group.go`
4. Wire dependencies in `main.go` `initializeSchedulers()`
5. Document in `docs/SCHEDULERS.md`

**Add a new domain interface:**
1. Define interface in `internal/domain/<package>/`
2. Create adapter implementation in `internal/adapters/`
3. Wire in `main.go` via constructor or functional option
4. Add mock in test file or `testutil/mocks/` if widely used

**Add a new migration:**
1. Check highest number: `ls internal/adapters/storage/sqlite/migrations/ | sort -n | tail -2`
2. Create up/down pair: `000NNN_description.up.sql` / `000NNN_description.down.sql`
3. Migrations run automatically on startup via embedded `embed.FS`
4. Update `docs/SCHEMA.md` with new tables/columns

#### Config Loading Flow

Add brief description:
- Precedence: CLI flags > environment variables > `.env` file > defaults
- Loaded in `internal/platform/config/loader.go`
- Validated in `config/validation.go`
- OAuth configs: separate helpers for Google (`google_oauth.go`) and Instagram (`instagram_oauth.go`)
- Reference `.env.example` for all available variables

### 1.2 New File: `.env.example`

Create at repository root. All env vars with descriptions, grouped by feature:

```bash
# === Required ===
PRICECHARTING_TOKEN=""           # PriceCharting API key for graded prices + sales data

# === Server ===
LOG_LEVEL="info"                 # debug, info, warn, error
WEB_PORT="8081"                  # HTTP server port

# === Authentication ===
GOOGLE_CLIENT_ID=""              # Google OAuth client ID
GOOGLE_CLIENT_SECRET=""          # Google OAuth client secret
GOOGLE_REDIRECT_URL=""           # Google OAuth redirect URL
ADMIN_EMAILS=""                  # Comma-separated admin email addresses
LOCAL_API_TOKEN=""               # Bearer token for CLI/curl access without browser OAuth

# === Pricing Sources ===
CARD_HEDGER_API_KEY=""           # CardHedger supplementary pricing (unlimited plan)
CARD_HEDGER_CLIENT_ID=""         # CardHedger card request submission token
CARD_HEDGER_POLL_INTERVAL="1h"  # Delta poll interval
CARD_HEDGER_BATCH_INTERVAL="24h" # Batch refresh interval
CARD_HEDGER_MAX_CARDS_PER_RUN="200" # Max cards per batch

# === Schedulers ===
PRICE_REFRESH_ENABLED="true"     # Enable/disable price refresh scheduler
SESSION_CLEANUP_ENABLED="true"   # Enable/disable session cleanup scheduler

# === Snapshot Enrichment ===
SNAPSHOT_ENRICH_RETRY_INTERVAL="30m" # Retry interval for failed snapshots
SNAPSHOT_ENRICH_MAX_RETRIES="5"      # Max retries before marking exhausted

# === Instagram Integration ===
INSTAGRAM_APP_ID=""              # Instagram OAuth app ID
INSTAGRAM_APP_SECRET=""          # Instagram OAuth app secret
INSTAGRAM_REDIRECT_URI=""        # Instagram OAuth redirect URI
SOCIAL_CONTENT_ENABLED="true"   # Enable/disable social content scheduler
SOCIAL_CONTENT_INTERVAL="24h"   # Social content detection interval

# === AI Advisor ===
AZURE_AI_ENDPOINT=""             # Azure OpenAI endpoint URL
AZURE_AI_API_KEY=""              # Azure OpenAI API key
AZURE_AI_DEPLOYMENT=""           # Azure OpenAI deployment name
```

### 1.3 New File: `docs/SCHEMA.md`

Curated database schema reference. One section per table (22 tables), each containing:

- Table name and one-line purpose
- Column table: name, type, constraints, notes
- Indexes (name, columns, conditions)
- Foreign keys with cascade behavior
- Related tables

Also document the 8 views: `stale_prices`, `api_usage_summary`, `api_hourly_distribution`, `api_daily_summary`, `active_sessions`, `expired_sessions`, `ai_usage_summary`, `ai_usage_by_operation`.

End with a relationship summary showing the FK dependency graph.

Source data: all 17 migration pairs in `internal/adapters/storage/sqlite/migrations/`.

### 1.4 New File: `docs/API.md`

Detailed endpoint reference. For each endpoint group:

- Route, method, auth requirement
- Request body / query params with types
- Response shape (JSON structure with field types)
- Error responses
- Example (where non-obvious)

Grouped by feature area matching the route table in CLAUDE.md. This is the "drill-down" doc agents use when they need payload details.

### 1.5 `internal/README.md` Enhancements

Add two new walkthrough sections (matching the existing "Adding a New Data Source" pattern):

**"Adding a New HTTP Handler"** walkthrough:
1. Create handler method on appropriate handler struct
2. Define request/response types in handler file
3. Register in `router.go` with correct middleware chain
4. Wire handler dependencies in `main.go` if new struct
5. Add frontend types in `web/src/types/` if new response shape

**"Adding a New Scheduler"** walkthrough:
1. Create file in `internal/adapters/scheduler/`
2. Define config struct, implement `Run(ctx)` loop
3. Register in `BuildGroup()` with prerequisite checks
4. Add adapter wrappers in `main.go` if domain type conversion needed
5. Configure startup delay in group timing sequence

**Large file awareness note:**
Brief note listing files >500 LOC and why they're large, so contributors don't accidentally add more responsibilities to already-large files.

---

## Phase 2: Code Maintainability

Goal: Split files with tangled responsibilities along natural boundaries. Only split where there's a clear responsibility boundary — not just because a file is large.

### 2.1 Split `cmd/slabledger/main.go`

**Current:** 704 LOC, single `runServer()` function handling all initialization.

**Target:** ~300 LOC in main.go + init helpers in same package.

Extract these functions (all in `cmd/slabledger/`):

#### `initializePriceProviders()`
```go
func initializePriceProviders(
    cfg *config.Config,
    appCache cache.Cache,
    logger observability.Logger,
    cardProv *tcgdex.TCGdex,
) (*fusionprice.FusionProvider, *cardhedger.Client, error)
```
Covers: PriceCharting client, CardHedger client, FusionProvider assembly with secondary sources.

#### `initializeCampaignsService()`
```go
func initializeCampaignsService(
    cfg *config.Config,
    logger observability.Logger,
    db *sqlite.DB,
    priceProv *fusionprice.FusionProvider,
    cardHedgerClient *cardhedger.Client,
    priceRepo *sqlite.PriceRepository,
) (*campaigns.ServiceImpl, *sqlite.CardRequestRepository, error)
```
Covers: Campaigns repository, PriceLookup adapter wiring, CertLookup, CardIDResolver, functional options.

#### `initializeAdvisorService()`
```go
func initializeAdvisorService(
    cfg *config.Config,
    logger observability.Logger,
    db *sqlite.DB,
    aiCallRepo *sqlite.AICallRepository,
    campaignsService campaigns.Service,
) (*azureai.Client, *advisor.ServiceImpl, *sqlite.AdvisorCacheRepository, error)
```
All three return values may be nil (Azure AI credentials are optional). Covers: Azure AI client, advisor tool executor, advisor service, cache repo.

#### `initializeSocialService()`
```go
func initializeSocialService(
    cfg *config.Config,
    logger observability.Logger,
    db *sqlite.DB,
    azureAIClient *azureai.Client,
    aiCallRepo *sqlite.AICallRepository,
) (*social.ServiceImpl, *igclient.Client, *sqlite.InstagramStore, error)
```
Covers: Social repository, Instagram client (with encryption), LLM integration, service assembly.

#### `initializeSchedulers()`
Uses a deps struct to avoid 20+ parameters:
```go
type schedulerDeps struct {
    cfg              *config.Config
    logger           observability.Logger
    db               *sqlite.DB
    priceProvider    *fusionprice.FusionProvider
    campaignsService campaigns.Service
    // ... remaining deps
}

func initializeSchedulers(ctx context.Context, deps schedulerDeps) (*scheduler.Group, context.CancelFunc, error)
```

**What stays in main.go:**
- Adapter wrapper types (6 DTO types, ~120 LOC) — they're wiring glue
- `runServer()` as a readable sequence of init calls
- Signal handling, graceful shutdown
- `main()`, `runAdmin()`, `printHelp()`

**New file:** `cmd/slabledger/init.go` — contains all extracted init functions and `schedulerDeps`.

### 2.2 Extract Business Logic from `campaigns_imports.go`

**Current:** 785 LOC HTTP handler mixing multipart upload handling with CSV parsing, normalization, and domain object construction.

**Target:** ~350 LOC handler (HTTP only) + domain parsing functions.

#### New domain files:

**`internal/domain/campaigns/parse_cl.go`**
```go
// ParseCLExportRows parses Card Ladder CSV export records into CLExportRow structs.
// Handles header detection, field extraction, currency/date parsing, normalization.
func ParseCLExportRows(records [][]string) ([]CLExportRow, []ParseError)
```

**`internal/domain/campaigns/parse_psa.go`**
```go
// ParsePSAExportRows parses PSA cert CSV records into PSAExportRow structs.
// Handles header detection, cert normalization, grade extraction, date parsing.
func ParsePSAExportRows(records [][]string) ([]PSAExportRow, []ParseError)

// NormalizePSACert strips non-numeric characters from cert numbers.
func NormalizePSACert(raw string) string
```

**`internal/domain/campaigns/parse_shopify.go`**
```go
// ParseShopifyExportRows parses Shopify product CSV records into ShopifyExportRow structs.
// Handles handle-based product consolidation, tag parsing, card extraction.
func ParseShopifyExportRows(records [][]string) ([]ShopifyExportRow, []ParseError)
```

**`internal/domain/campaigns/parse_helpers.go`**
```go
// Shared CSV parsing utilities used by all parsers.
func buildHeaderMap(headers []string) map[string]int
func findHeaderRow(records [][]string, requiredHeaders []string) (int, map[string]int, error)
func parseCurrencyString(s string) (int, error)
```

**`internal/domain/campaigns/parse_error.go`**
```go
// ParseError represents a non-fatal parsing issue in a specific row.
type ParseError struct {
    Row     int
    Field   string
    Message string
}
```

#### What stays in the handler:

- `parseGlobalCSVUpload()` — multipart form handling, file size validation
- `csv.NewReader()` instantiation and `ReadAll()`
- Calling domain `Parse*Rows()` functions
- Passing results to existing service methods
- HTTP response writing
- Background `triggerCardDiscovery()` goroutine (keep goroutine management in handler, extract dedup logic to domain)

### 2.3 Split `purchases_repository.go`

**Current:** 802 LOC with CRUD and complex batch queries in one file.

**Target:** Two files in the same package, same receiver type.

**`purchases_repository.go`** (~400 LOC) — CRUD operations:
- `CreatePurchase`, `GetPurchase`, `DeletePurchase`
- `ListPurchasesByCampaign`, `ListUnsoldPurchases`, `ListAllUnsoldPurchases`
- `CountPurchasesByCampaign`
- Single-field updates: `UpdatePurchaseCLValue`, `UpdatePurchaseCardMetadata`, `UpdatePurchaseGrade`, `UpdatePurchaseCampaign`, `UpdatePurchaseCardYear`

**`purchases_repository_queries.go`** (~400 LOC) — complex queries:
- Batch lookups: `GetPurchaseByCertNumber`, `GetPurchasesByGraderAndCertNumbers`, `GetPurchasesByCertNumbers`, `GetPurchasesByIDs`
- Market data: `UpdatePurchaseMarketSnapshot`, `UpdatePurchaseSnapshotStatus`, `ListSnapshotPurchasesByStatus`
- eBay export: `SetEbayExportFlag`, `ClearEbayExportFlags`, `ListEbayFlaggedPurchases`
- Price overrides: `UpdatePurchasePriceOverride`, `UpdatePurchaseAISuggestion`, `ClearPurchaseAISuggestion`, `AcceptAISuggestion`, `GetPriceOverrideStats`
- External/PSA fields: `UpdateExternalPurchaseFields`, `UpdatePurchasePSAFields`

### 2.4 Add `.golangci.yml`

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
      - hugeParam          # false positives on large structs passed by value intentionally
  revive:
    rules:
      - name: exported
        disabled: true     # too noisy for iterative development
      - name: unexported-return
        disabled: true

issues:
  exclude-dirs:
    - web/node_modules
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck          # test assertions handle errors differently
        - gocritic
    - path: testutil/mocks/
      linters:
        - unused
        - revive
  max-issues-per-linter: 0
  max-same-issues: 0
```

---

## Phase 3: Test Coverage

Goal: Add tests for untested subsystems. Write tests after Phase 2 code splits so test targets are stable.

### 3.1 Advisor Service Tests

**File:** `internal/domain/advisor/service_test.go`

**Mock types (defined inline in test file):**
- `mockLLMProvider` — implements `ai.LLMProvider`, records calls, returns configurable responses
- `mockToolExecutor` — implements `ai.ToolExecutor`, returns predefined tool results
- `mockCacheStore` — implements `advisor.CacheStore`, tracks state transitions

**Test cases:**

| Test | What it verifies |
|------|-----------------|
| `TestGenerateDigest_Success` | Prompt construction, streaming output, tool call handling |
| `TestGenerateDigest_LLMError` | Error propagation from LLM provider |
| `TestGenerateDigest_ToolCallError` | Graceful handling of tool execution failure |
| `TestAnalyzeCampaign_Success` | Campaign ID threading, tool subset filtering |
| `TestAnalyzeCampaign_InvalidID` | Error when campaign not found |
| `TestAnalyzeLiquidation_Success` | Tool definitions passed, streaming output |
| `TestAssessPurchase_Success` | Request validation, prompt assembly |
| `TestAssessPurchase_MissingFields` | Validation error for incomplete request |
| `TestCollectDigest_Success` | Non-streaming collection returns full content |
| `TestCacheStore_AcquireRefresh` | Lease acquisition, stale threshold |
| `TestCacheStore_SaveResult` | Content persistence after analysis |

### 3.2 Advisor Tool Executor Tests

**File:** `internal/adapters/advisortool/executor_test.go`

**Mock types (defined inline):**
- `mockCampaignsService` — implements campaigns.Service methods used by tools

**Test cases:**

| Test | What it verifies |
|------|-----------------|
| `TestDefinitions_ReturnsAllTools` | All registered tools returned with schemas |
| `TestDefinitionsFor_FiltersByName` | Subset selection works correctly |
| `TestExecute_UnknownTool` | Returns error for unregistered tool name |
| `TestExecute_InvalidJSON` | Returns error for malformed arguments |
| `TestExecute_PortfolioHealth` | Routes to correct handler, returns formatted result |
| `TestExecute_CampaignPNL` | Campaign ID extracted from args, service called |
| `TestExecute_Inventory` | Inventory tool returns expected data shape |
| `TestExecute_SellSheet` | Sell sheet generation with purchase IDs |
| `TestExecute_ServiceError` | Service errors propagated correctly |

### 3.3 Instagram Client Tests

**File:** `internal/adapters/clients/instagram/client_test.go`

Uses `httptest.NewServer` for HTTP mocking (consistent with `httpx/client_test.go` pattern).

**Test cases:**

| Test | What it verifies |
|------|-----------------|
| `TestPublishCarousel_Success` | Full publish flow: container create → carousel create → publish → status poll |
| `TestPublishCarousel_ContainerError` | API error on container creation |
| `TestPublishCarousel_PublishTimeout` | Status polling timeout handling |
| `TestPublishCarousel_InvalidToken` | 401 response handling |
| `TestPublishCarousel_RateLimit` | 429 response handling |
| `TestExchangeLongLivedToken_Success` | Token exchange flow |
| `TestExchangeLongLivedToken_Error` | Token exchange failure |

### 3.4 Domain Parser Tests (from Phase 2 extraction)

Tests for the newly extracted CSV parsing functions:

**`internal/domain/campaigns/parse_cl_test.go`**
- Happy path with standard CL export format
- Missing required columns
- Currency parsing edge cases (negative, commas, dollar signs)
- Date format variations

**`internal/domain/campaigns/parse_psa_test.go`**
- Happy path with standard PSA export format
- Cert number normalization (leading zeros, non-numeric chars)
- Grade extraction from description
- Duplicate cert handling

**`internal/domain/campaigns/parse_shopify_test.go`**
- Happy path with standard Shopify export
- Handle-based product consolidation (multiple rows same handle)
- Tag parsing for card metadata
- Variant-only rows (no product title)

**`internal/domain/campaigns/parse_helpers_test.go`**
- `buildHeaderMap` with various header formats
- `findHeaderRow` with offset headers (non-zero start row)
- `parseCurrencyString` edge cases

---

## File Change Summary

### New Files
| File | Phase | Purpose |
|------|-------|---------|
| `.env.example` | 1 | Environment variable reference |
| `docs/SCHEMA.md` | 1 | Database schema reference |
| `docs/API.md` | 1 | API endpoint documentation |
| `cmd/slabledger/init.go` | 2 | Extracted init functions from main.go |
| `internal/domain/campaigns/parse_cl.go` | 2 | CL CSV parsing (domain) |
| `internal/domain/campaigns/parse_psa.go` | 2 | PSA CSV parsing (domain) |
| `internal/domain/campaigns/parse_shopify.go` | 2 | Shopify CSV parsing (domain) |
| `internal/domain/campaigns/parse_helpers.go` | 2 | Shared CSV utilities (domain) |
| `internal/domain/campaigns/parse_error.go` | 2 | Parse error type |
| `internal/adapters/storage/sqlite/purchases_repository_queries.go` | 2 | Complex purchase queries |
| `.golangci.yml` | 2 | Linter configuration |
| `internal/domain/advisor/service_test.go` | 3 | Advisor service tests |
| `internal/adapters/advisortool/executor_test.go` | 3 | Tool executor tests |
| `internal/adapters/clients/instagram/client_test.go` | 3 | Instagram client tests |
| `internal/domain/campaigns/parse_cl_test.go` | 3 | CL parser tests |
| `internal/domain/campaigns/parse_psa_test.go` | 3 | PSA parser tests |
| `internal/domain/campaigns/parse_shopify_test.go` | 3 | Shopify parser tests |
| `internal/domain/campaigns/parse_helpers_test.go` | 3 | Parser helper tests |

### Modified Files
| File | Phase | Change |
|------|-------|--------|
| `CLAUDE.md` | 1 | Add API routes, schema overview, interface catalog, recipes, config flow |
| `internal/README.md` | 1 | Add handler/scheduler walkthroughs, large file note |
| `cmd/slabledger/main.go` | 2 | Extract init functions to init.go, simplify runServer() |
| `internal/adapters/httpserver/handlers/campaigns_imports.go` | 2 | Remove business logic, call domain parsers |
| `internal/adapters/storage/sqlite/purchases_repository.go` | 2 | Move complex queries to purchases_repository_queries.go |

### Unchanged (explicitly NOT touched)
- `internal/adapters/clients/pricecharting/domain_adapter.go` (637 LOC) — large but cohesive strategy pipeline
- `internal/adapters/clients/fusionprice/fusion_provider.go` (635 LOC) — large but single-purpose
- `internal/domain/social/service_impl.go` (731 LOC) — assess after advisor tests reveal patterns
- Mock generation strategy — keeping hand-written mocks
- Domain interface definitions — no changes needed

## Success Criteria

1. **AI agents can answer "where does X go?"** by reading CLAUDE.md alone (route table, schema overview, interface catalog, recipes)
2. **`golangci-lint run`** passes cleanly on current code and provides deterministic feedback
3. **`go test ./...`** passes with new tests covering advisor, advisortool, and instagram
4. **No file in `cmd/slabledger/`** exceeds 400 LOC
5. **`campaigns_imports.go`** contains only HTTP concerns (under 400 LOC)
6. **All CSV parsing logic** is testable independently of HTTP (in domain layer)
7. **`.env.example`** documents every environment variable the app reads

## Non-Goals

- Splitting pricing pipeline files (cohesive, read top-to-bottom)
- Introducing mock generation tooling
- Adding OpenAPI spec generation
- Refactoring the campaigns domain package structure
- Changing the frontend architecture
- Adding deployment documentation
