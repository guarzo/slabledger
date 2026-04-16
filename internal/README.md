# Internal Package Structure

This document describes the organization of packages within `/internal` and provides guidelines for where new code should be added.

---

## Architecture Overview

This codebase follows **Hexagonal Architecture** (also known as Ports and Adapters or Clean Architecture). The key principle is **dependency inversion**: domain logic defines interfaces (ports), and external adapters implement those interfaces.

```
┌─────────────────────────────────────────────────┐
│              Entry Points (Server)               │
│         cmd/slabledger/main.go                  │
└───────────────────┬─────────────────────────────┘
                    │ (wires dependencies)
                    ▼
┌─────────────────────────────────────────────────┐
│         ADAPTERS (External World)               │
│    internal/adapters/                           │
│    ├── httpserver/      (inbound: web API)      │
│    ├── clients/         (outbound: APIs)        │
│    │   ├── cardutil/       (card utilities)     │
│    │   ├── dhprice/        (DH pricing)         │
│    │   ├── google/         (Google OAuth)       │
│    │   ├── httpx/          (shared HTTP client) │
│    │   ├── tcgdex/         (card metadata)      │
│    │   ├── pricelookup/    (PriceLookup adapter)│
│    │   └── psa/            (PSA data)           │
│    ├── scheduler/       (background jobs)       │
│    └── storage/sqlite/  (SQLite persistence)    │
└───────────────────┬─────────────────────────────┘
                    │ (implements interfaces)
                    ▼
┌─────────────────────────────────────────────────┐
 │         DOMAIN (Business Logic)                 │
 │    internal/domain/                             │
 │    ├── inventory/      (campaigns, purchases)   │
 │    ├── arbitrage/      (crack candidates, EV)   │
 │    ├── auth/           (authentication)         │
 │    ├── cards/          (card interfaces)        │
 │    ├── constants/      (shared constants)       │
 │    ├── export/         (sell sheet, eBay CSV)   │
 │    ├── favorites/      (favorites management)   │
 │    ├── finance/        (invoices, cashflow)     │
 │    ├── intelligence/   (DH market data)         │
 │    ├── mathutil/       (math utilities)         │
 │    ├── observability/  (logger interfaces)      │
 │    ├── picks/          (acquisition watchlist)  │
 │    ├── pricing/        (price interfaces/models)│
 │    ├── scoring/        (price scoring factors)  │
 │    └── storage/        (storage interfaces)     │
└───────────────────┬─────────────────────────────┘
                    │ (uses)
                    ▼
┌─────────────────────────────────────────────────┐
│      PLATFORM (Infrastructure)                  │
│    internal/platform/                           │
│    ├── cache/          (type-safe caching)      │
│    ├── config/         (configuration)          │
│    ├── crypto/         (AES encryption)         │
│    ├── errors/         (error types)            │
│    ├── resilience/     (retry + circuit breaker)│
│    ├── storage/        (file store)             │
│    └── telemetry/      (slog logging)           │
└─────────────────────────────────────────────────┘
```

**Dependency Rule**: Dependencies flow **inward only**:
- ✅ `adapters` → `domain` (implements domain interfaces)
- ✅ `domain` → `platform` (uses infrastructure)
- ❌ `domain` → `adapters` (NEVER - violates dependency inversion)

---

## Core Hexagonal Packages

### `/internal/domain` - Business Logic

**Purpose**: Pure business logic with no external dependencies.

**Contains**:
- Domain entities (data structures)
- Domain interfaces (ports)
- Business rules and algorithms

**Packages**:

| Package | Purpose |
|---------|---------|
| `auth/` | Authentication interfaces |
| `inventory/` | Campaign tracking, purchases, sales, P&L, analytics, CSV import |
| `arbitrage/` | Crack candidates, acquisition targets, expected value, Monte Carlo |
| `cards/` | `CardRepository` interface for card metadata |
| `constants/` | Shared application constants |
| `export/` | Sell sheet generation, eBay CSV, Shopify price sync |
| `favorites/` | Favorites management |
| `finance/` | Invoices, cashflow, capital tracking, revocation flags |
| `intelligence/` | DH market intelligence repository and types |
| `mathutil/` | Math utility functions |
| `observability/` | Logger, MetricsRecorder interfaces |
| `picks/` | AI-driven acquisition watchlist service |
| `pricing/` | `PriceProvider` interface, graded prices, market data models |
| `scoring/` | Price scoring factors and profiles |
| `storage/` | Storage interfaces |

**Rules**:
- ✅ Define interfaces for external dependencies
- ✅ Implement business logic using only domain types
- ❌ NO imports from `internal/adapters`
- ❌ NO direct API calls or database queries
- ❌ NO framework dependencies (gin, echo, etc.)

**Adding New Domain Logic**:
```go
// 1. Define interface in domain layer
package pricing

type PriceProvider interface {
    GetPrice(ctx context.Context, card Card) (*Price, error)
}

// 2. Implement in adapter layer
package dhprice

type Provider struct { ... }

func (p *Provider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    // API call implementation
}

// 3. Wire in main.go
dhProvider := dhprice.NewProvider(...)
service := someservice.NewService(dhProvider) // Inject interface
```

---

### `/internal/adapters` - External Integrations

**Purpose**: Implements domain interfaces by talking to the external world.

**Contains**:
- **Inbound adapters**: HTTP handlers (receive requests)
- **Outbound adapters**: API clients, database repositories (fetch/store data)
- **Background jobs**: Schedulers for periodic tasks

**Structure**:
```
internal/adapters/
├── httpserver/          # Inbound: Web API
│   ├── handlers/        # HTTP request handlers
│   ├── middleware/       # Authentication, CORS, etc.
│   └── router.go        # Route configuration
├── clients/             # Outbound: External APIs
│   ├── cardutil/        # Card utility functions
│   ├── dhprice/         # DH (DoubleHolo) pricing
│   ├── google/          # Google OAuth client
│   ├── httpx/           # Shared HTTP client (retry, circuit breaker)
│   ├── tcgdex/          # TCGdex.dev card/set metadata (EN + JA)
│   ├── pricelookup/     # PriceLookup adapter (wraps PriceProvider for campaigns)
│   └── psa/             # PSA data client
├── scheduler/           # Background jobs (price refresh, session cleanup)
└── storage/sqlite/      # SQLite persistence + migrations
```

**Rules**:
- ✅ Implement domain interfaces
- ✅ Handle external API/database interactions
- ✅ Convert between external formats and domain models
- ❌ NO business logic (put in domain layer)

**Example Adapter**:
```go
package dhprice

import "github.com/guarzo/slabledger/internal/domain/pricing"

// Provider implements pricing.PriceProvider interface
type Provider struct {
    httpClient *httpx.Client
    apiKey     string
    logger     observability.Logger
}

var _ pricing.PriceProvider = (*Provider)(nil) // Compile-time interface check

func (p *Provider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    // 1. Build API request
    // 2. Make HTTP call
    // 3. Parse response
    // 4. Convert to domain.Price
    // 5. Return
}
```

---

### `/internal/platform` - Cross-Cutting Infrastructure

**Purpose**: Provides infrastructure services used across all layers.

**Contains**:
- Configuration management
- Observability (logging)
- Error handling
- Caching infrastructure
- Resilience (retry + circuit breaker)
- Encryption for auth tokens

**Structure**:
```
internal/platform/
├── cache/          # Type-safe caching (LRU, file store)
├── config/         # Configuration loading and validation
├── crypto/         # AES encryption for auth tokens
├── errors/         # Custom error types
├── resilience/     # Retry + circuit breaker
├── storage/        # File store
└── telemetry/      # slog logging implementation
```

**Rules**:
- ✅ Provide infrastructure via interfaces
- ✅ No business logic
- ✅ Vendor-agnostic (can swap implementations)

---

### `/internal/testutil` - Test Utilities

**Purpose**: Shared test helpers, mocks, and fixtures.

**Structure**:
```
internal/testutil/
└── mocks/          # Mock implementations of domain interfaces
```

**Example**:
```go
package mocks

type MockPriceProvider struct {
    GetPriceFunc func(ctx context.Context, card pricing.Card) (*pricing.Price, error)
}

func (m *MockPriceProvider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    return m.GetPriceFunc(ctx, card)
}
```

---

## Guidelines for Adding New Code

### Where should new code go?

**Use this decision tree**:

```
Is it business logic?
├─ YES → /internal/domain/
│   ├─ Defines what the system does
│   ├─ No external dependencies
│   └─ Example: campaign analytics, pricing models
│
├─ Is it an external integration?
│   ├─ Inbound (HTTP)?     → /internal/adapters/httpserver/
│   ├─ Outbound (API)?     → /internal/adapters/clients/
│   ├─ Persistence (DB)?   → /internal/adapters/storage/sqlite/
│   └─ Background job?     → /internal/adapters/scheduler/
│
├─ Is it infrastructure?
│   └─ YES → /internal/platform/
│       ├─ Config, logging
│       ├─ Caching, resilience
│       └─ Error handling, encryption
│
└─ Is it a test helper?
    └─ YES → /internal/testutil/
```

---

### Example: Adding a New Data Source

**Scenario**: Add a new pricing provider.

**Step 1**: Define interface in domain layer (if not already covered by existing interfaces)
```go
// internal/domain/pricing/provider.go
package pricing

type PriceProvider interface {
    GetPrice(ctx context.Context, card Card) (*Price, error)
}
```

**Step 2**: Implement interface in adapter layer
```go
// internal/adapters/clients/newprovider/client.go
package newprovider

import "github.com/guarzo/slabledger/internal/domain/pricing"

type Client struct {
    httpClient *httpx.Client
    apiKey     string
}

var _ pricing.PriceProvider = (*Client)(nil)

func (c *Client) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    // API implementation
}
```

**Step 3**: Wire in main.go
```go
// cmd/slabledger/main.go
newClient := newprovider.NewClient(httpClient, cfg.NewProviderAPIKey)
```

---

### Example: Adding a New HTTP Handler

**Scenario**: Add a new endpoint to an existing or new handler struct.

**Step 1**: Create the handler method on the appropriate handler struct in `internal/adapters/httpserver/handlers/`
```go
// internal/adapters/httpserver/handlers/snapshots.go
func (h *SnapshotHandler) GetSummary(c *gin.Context) {
    ctx := c.Request.Context()
    summary, err := h.snapshotService.GetSummary(ctx)
    if err != nil {
        h.logger.Error("get summary failed", observability.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
        return
    }
    c.JSON(http.StatusOK, summary)
}
```

**Step 2**: Define request/response types in the handler file (or a `types.go` in the same package)
```go
// internal/adapters/httpserver/handlers/snapshots.go
type SnapshotSummaryResponse struct {
    TotalCount int     `json:"total_count"`
    TotalValue float64 `json:"total_value"`
}
```

**Step 3**: Register the route in `router.go` using the correct middleware chain
```go
// internal/adapters/httpserver/router.go
// Use authRoute() for endpoints that require an authenticated session.
authRoute(rg, "GET", "/snapshots/summary", handlers.Snapshot.GetSummary)
```

**Step 4**: If a new handler struct is needed, wire its dependencies in `main.go`
```go
// cmd/slabledger/main.go
snapshotHandler := handlers.NewSnapshotHandler(snapshotService, logger)
```

**Step 5**: If the response shape is new, add matching TypeScript types in `web/src/types/` to keep the frontend in sync with the Go JSON tags.

---

### Example: Adding a New Scheduler

**Scenario**: Add a background job that runs on a fixed interval.

**Step 1**: Create a scheduler file in `internal/adapters/scheduler/`
```go
// internal/adapters/scheduler/myworker.go
package scheduler

type MyWorker struct {
    service domain.MyService
    logger  observability.Logger
    cfg     MyWorkerConfig
}
```

**Step 2**: Define a config struct and implement the `Run(ctx)` loop using `RunLoop`
```go
// internal/adapters/scheduler/myworker.go
type MyWorkerConfig struct {
    Interval time.Duration
}

func (w *MyWorker) Run(ctx context.Context) {
    RunLoop(ctx, w.logger, "myworker", w.cfg.Interval, func() {
        if err := w.service.DoWork(ctx); err != nil {
            w.logger.Error("myworker failed", observability.Error(err))
        }
    })
}
```

**Step 3**: Register the worker in `BuildGroup()` with any prerequisite checks
```go
// internal/adapters/scheduler/group.go
if cfg.MyWorkerEnabled {
    g.workers = append(g.workers, &MyWorker{
        service: svc,
        logger:  logger,
        cfg:     MyWorkerConfig{Interval: cfg.MyWorkerInterval},
    })
}
```

**Step 4**: If the scheduler needs a domain type that doesn't match an existing adapter directly, add a thin wrapper in `main.go` to convert between types before passing the service in.

**Step 5**: Configure the startup delay in the group timing sequence so the new worker doesn't race with database migrations or other workers that must finish first.

**Step 6**: Document the new scheduler (env vars, interval, purpose) in `docs/SCHEDULERS.md`.

---

### Example: Adding a New Domain Error

**Scenario**: Add a custom error to a domain package.

**Step 1**: Add error code in `internal/domain/<package>/errors.go`:
```go
ErrCodeMyError errors.ErrorCode = "ERR_MY_ERROR"
```

**Step 2**: Add sentinel error:
```go
var ErrMyError = errors.NewAppError(ErrCodeMyError, "description")
```

**Step 3**: Add predicate:
```go
func IsMyError(err error) bool { return errors.HasErrorCode(err, ErrCodeMyError) }
```

**Step 4**: Test with `errors.Is(err, ErrMyError)` in callers.

---

### Example: Adding a New Migration

**Scenario**: Add a schema change to SQLite.

**Step 1**: Check the highest existing migration number:
```bash
ls internal/adapters/storage/sqlite/migrations/ | sort -n | tail -2
```

**Step 2**: Create the pair (zero-pad to 6 digits):
```bash
touch internal/adapters/storage/sqlite/migrations/000022_description.up.sql
touch internal/adapters/storage/sqlite/migrations/000022_description.down.sql
```

**Step 3**: Write the SQL. The `.up.sql` applies the change, `.down.sql` reverts it.

**Step 4**: Update `docs/SCHEMA.md` with the new table/column.

**Step 5**: Update the migration count in `CLAUDE.md`'s Database section.

---

## Common Anti-Patterns to Avoid

### ❌ Anti-Pattern 1: Business Logic in Adapters

**Bad**:
```go
// internal/adapters/httpserver/handlers/campaigns.go
func (h *Handler) GetPNL(c *gin.Context) {
    // ❌ Business logic in HTTP handler
    pnl := calculatePNL(purchases, sales)
    c.JSON(200, pnl)
}
```

**Good**:
```go
// internal/adapters/httpserver/handlers/campaigns.go
func (h *Handler) GetPNL(c *gin.Context) {
    // ✅ Delegate to domain service
    pnl, err := h.campaignService.GetPNL(ctx, campaignID)
    c.JSON(200, pnl)
}
```

---

### ❌ Anti-Pattern 2: Domain Depending on Adapters

**Bad**:
```go
// internal/domain/inventory/service.go
import "github.com/guarzo/slabledger/internal/adapters/clients/dhprice"

type Service struct {
    priceClient *dhprice.Provider // ❌ Direct dependency on adapter
}
```

**Good**:
```go
// internal/domain/inventory/service.go
import "github.com/guarzo/slabledger/internal/domain/pricing"

type Service struct {
    priceProvider pricing.PriceProvider // ✅ Depends on interface
}
```

---

## Testing Strategy by Layer

### Domain Layer Testing
```go
// internal/domain/inventory/service_test.go
func TestService_GetCampaignPNL(t *testing.T) {
    // ✅ Use mock providers (no real API calls)
    mockAnalytics := &mocks.AnalyticsRepositoryMock{
        GetCampaignPNLFn: func(ctx context.Context, id string) (*inventory.CampaignPNL, error) {
            return &inventory.CampaignPNL{}, nil
        },
    }

    svc := inventory.NewService(mockCampaigns, mockPurchases, mockSales, mockAnalytics, ...)
    pnl, err := svc.GetCampaignPNL(context.Background(), campaignID)
    assert.NoError(t, err)
}
```

### Adapter Layer Testing
```go
// internal/adapters/clients/dhprice/provider_test.go
func TestProvider_GetPrice(t *testing.T) {
    // ✅ Use mock HTTP client (no real network calls)
    mockHTTP := mocks.NewMockHTTPClientWithResponse(`{"price": 100}`)

    provider := dhprice.NewProviderWithHTTP(mockHTTP, "test-key")

    price, err := provider.GetPrice(context.Background(), testCard)
    assert.NoError(t, err)
}
```

---

## Large File Awareness

Several files in this codebase exceed 500 lines of code. Before adding code to any of them, consider whether the new logic belongs in a separate file.

| File | LOC | Why it's large |
|------|-----|----------------|
| `domain/arbitrage/service.go` | 530 | Crack, acquisition, EV, Monte Carlo computation |
| `domain/inventory/service_analytics.go` | ~330 | Campaign analytics computations |

---

## References

- [User Guide](../docs/USER_GUIDE.md) - End-user documentation
- [Architecture](../docs/ARCHITECTURE.md) - System design and key decisions
- [Development](../docs/DEVELOPMENT.md) - Caching, rate limiting, API integrations

---

## Questions?

If you're unsure where to add new code, ask yourself:

1. **Does it contain business logic?** → Domain layer
2. **Does it talk to external systems?** → Adapter layer
3. **Is it infrastructure (logging, config)?** → Platform layer
4. **Is it for testing?** → testutil package

**Remember**: When in doubt, favor the domain layer. It's easier to extract an adapter from domain code than to extract business logic from adapter code.
