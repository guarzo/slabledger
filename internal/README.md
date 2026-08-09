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
│    │   ├── dhprice/        (DH pricing)         │
│    │   ├── google/         (Google OAuth)       │
│    │   ├── httpx/          (shared HTTP client) │
│    │   └── psa/            (PSA data)           │
│    ├── scheduler/       (background jobs)       │
│    └── storage/postgres/ (Postgres persistence) │
└───────────────────┬─────────────────────────────┘
                    │ (implements interfaces)
                    ▼
┌─────────────────────────────────────────────────┐
 │         DOMAIN (Business Logic)                 │
 │    internal/domain/                             │
 │    ├── inventory/      (campaigns, purchases)   │
 │    ├── arbitrage/      (acquisition targets, EV) │
 │    ├── auth/           (authentication)         │
 │    ├── constants/      (shared constants)       │
 │    ├── export/         (sell sheet generation)  │
 │    ├── finance/        (invoices, cashflow)     │
 │    ├── intelligence/   (DH market data)         │
 │    ├── mathutil/       (math utilities)         │
 │    ├── observability/  (logger interfaces)      │
 │    ├── pricing/        (price interfaces/models)│
 │    └── storage/        (storage interfaces)     │
└───────────────────┬─────────────────────────────┘
                    │ (uses)
                    ▼
┌─────────────────────────────────────────────────┐
│      PLATFORM (Infrastructure)                  │
│    internal/platform/                           │
│    ├── canonjson/      (canonical JSON encoding)│
│    ├── cardutil/       (card normalization)     │
│    ├── config/         (configuration)          │
│    ├── crypto/         (AES encryption)         │
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

**Packages** (partial — run `go list ./internal/domain/...` for the current set):

| Package | Purpose |
|---------|---------|
| `auth/` | Authentication interfaces |
| `inventory/` | Campaign tracking, purchases, sales, P&L, analytics |
| `arbitrage/` | Acquisition targets, expected value, Monte Carlo |
| `constants/` | Shared application constants |
| `csvimport/` | CSV import parsing (eBay, Shopify, orders) |
| `export/` | Sell sheet generation |
| `finance/` | Invoices, cashflow, capital tracking, revocation flags |
| `intelligence/` | DH market intelligence repository and types |
| `mathutil/` | Math utility functions |
| `observability/` | Logger, MetricsRecorder interfaces |
| `pricing/` | `PriceProvider` interface, graded prices, market data models |
| `storage/` | Storage interfaces |

**Rules**:
- ✅ Define interfaces for external dependencies
- ✅ Implement business logic using only domain types
- ❌ NO imports from `internal/adapters`
- ❌ NO direct API calls or database queries
- ❌ NO framework dependencies (gin, echo, etc.)

#### Leaf and non-leaf packages

Which domain packages may import which is decided by one derived test (SLA-48):

> A package under `internal/domain/` is a **leaf** if its transitive import closure within
> `internal/domain/` excludes `internal/domain/inventory`. It is **non-leaf** otherwise —
> the hub itself, plus every package that depends on it directly or transitively.
>
> Any domain package may import a **leaf**, or the **hub**. Importing any other non-leaf
> is a violation.

Derived, never listed — a hardcoded roster is the failure mode SLA-12 removed. To see the
current partition:

```bash
for p in $(go list ./internal/domain/...); do
  go list -deps "$p" | grep -q '/internal/domain/inventory$' \
    && echo "non-leaf: $p" || echo "leaf:     $p"
done
```

**This is already enforced; no separate check exists or is needed.** `scripts/check-imports.sh`
derives its governed set as the *direct* hub importers, while the taxonomy's non-leaf set is
the *transitive* ones. The two differ only for a package reaching the hub through another
non-leaf — and that package imports a governed sibling, which the checker already flags. So
on any tree that passes, `governed sibling ≡ non-leaf minus the hub`, and the target-side
scan is exactly "no domain package may import a non-leaf other than the hub."

A corollary worth knowing before you propose strengthening the checker: making membership
transitive is a **no-op**. Closure can only add a package that imports an already-governed
one, which is itself a violation, so the fixed point is reached at iteration zero wherever
the checker passes.

Edges that look like exceptions but are not — each target is a leaf, so all are legal:
`csvimport → mathutil`, `csvimport → dhevents`, `dhlisting → dhevents`,
`pricing/lookup → pricing`, `inventory → dhevents`, `inventory → intelligence`.

Full reasoning: `docs/specs/2026-08-08-domain-leaf-taxonomy-design.md`.

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
│   ├── dhprice/         # DH (DoubleHolo) pricing
│   ├── google/          # Google OAuth client
│   ├── httpx/           # Shared HTTP client (retry, circuit breaker)
│   └── psa/             # PSA data client
├── scheduler/           # Background jobs (price refresh, session cleanup)
└── storage/postgres/    # Postgres persistence + migrations
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
- Card name/set normalization
- Canonical JSON encoding
- Resilience (retry + circuit breaker)
- Encryption for auth tokens

**Structure**:
```
internal/platform/
├── canonjson/      # Canonical JSON encoding
├── cardutil/       # Card name/set normalization
├── config/         # Configuration loading and validation
├── crypto/         # AES encryption for auth tokens
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
// Most mocks use the Fn-field pattern: set the field for the method you care about.
mock := &mocks.CampaignRepositoryMock{
    GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
        return &inventory.Campaign{ID: id, Name: "Test Campaign"}, nil
    },
}
```

See [testutil/mocks/README.md](testutil/mocks/README.md) for the full guide, including the
mocks that are built by a constructor because they carry initialized state.

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
│   ├─ Persistence (DB)?   → /internal/adapters/storage/postgres/
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

**Step 3**: Add a `buildMyWorkerScheduler` helper in `internal/adapters/scheduler/builder_schedulers.go` that returns `nil` when the worker's prerequisites are unmet, then call it from `BuildGroup` in `builder.go`
```go
// internal/adapters/scheduler/builder_schedulers.go
func buildMyWorkerScheduler(cfg *config.Config, deps BuildDeps) *MyWorker {
    if !cfg.MyWorkerEnabled || deps.MyService == nil {
        return nil
    }
    return &MyWorker{
        service: deps.MyService,
        logger:  deps.Logger,
        cfg:     MyWorkerConfig{Interval: cfg.MyWorkerInterval},
    }
}

// internal/adapters/scheduler/builder.go, inside BuildGroup
if s := buildMyWorkerScheduler(cfg, deps); s != nil {
    schedulers = append(schedulers, s)
}
```
Return the concrete `*MyWorker`, not `Scheduler` — a nil `*MyWorker` stored in a `Scheduler` interface compares non-nil, and the group would then call `Start` on it. If callers outside the group need the instance (as PSA sync and cert enrichment do), also add a field for it on `BuildResult`.

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

**Scenario**: Add a schema change to Postgres.

**Step 1**: Check the highest existing migration number:
```bash
ls internal/adapters/storage/postgres/migrations/ | sort -n | tail -2
```

**Step 2**: Create the pair (zero-pad to 6 digits):
```bash
touch internal/adapters/storage/postgres/migrations/000026_description.up.sql
touch internal/adapters/storage/postgres/migrations/000026_description.down.sql
```

**Step 3**: Write the SQL. The `.up.sql` applies the change, `.down.sql` reverts it.

**Step 4**: Update `docs/SCHEMA.md` with the new table/column.

**Step 5**: Update the migration count in `CLAUDE.md`'s Database section.

**Step 6**: Verify with `make test-postgres`.

#### Editing an existing migration file

Add a new numbered migration instead, whenever the change has already shipped —
golang-migrate records versions, not file contents, so an edit to a migration a
database has already applied never runs there. Any environment past that version
keeps the old schema silently, and `schema_migrations` still reports success.

Editing in place is only safe for a migration that has not yet merged. Even then,
the local test database has probably already recorded the version from an earlier
run. `TestMain` in `internal/adapters/storage/postgres/testhelper_test.go` drops
and re-migrates the schema before every package run precisely so this cannot
produce a false green — do not weaken it. If you apply migrations by hand
anywhere else, drop the schema first:

```bash
psql "$POSTGRES_TEST_URL" -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
```

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
    // ✅ Use the in-memory store (no real database)
    store := mocks.NewInMemoryCampaignStore()
    store.Campaigns[campaignID] = &inventory.Campaign{ID: campaignID}

    svc := inventory.NewService(store, store, store, store, ...)
    pnl, err := svc.GetCampaignPNL(context.Background(), campaignID)
    assert.NoError(t, err)
}
```

### Adapter Layer Testing
```go
// internal/adapters/clients/dhprice/provider_test.go
func TestGetPrice(t *testing.T) {
    // ✅ Inject mock collaborators (no real network calls)
    client := &mocks.MockDHMarketDataClient{
        CardLookupFn: func(ctx context.Context, cardID int) (*dh.CardLookupResponse, error) {
            return &dh.CardLookupResponse{}, nil
        },
    }
    lookup := &mocks.MockDHCardIDLookup{
        GetExternalIDFn: func(context.Context, string, string, string, string) (string, error) {
            return "12345", nil
        },
    }

    p := dhprice.New(client, lookup)

    price, err := p.GetPrice(context.Background(), testCard)
    assert.NoError(t, err)
}
```

---

## Large File Awareness

Several files in this codebase exceed 500 lines of code. Before adding code to any of them, consider whether the new logic belongs in a separate file. `scripts/check-file-size.sh` warns at 500 lines and fails at 600.

The table below is a snapshot, not the source of truth — regenerate it rather than
trusting it:

```bash
find internal/ cmd/ -name '*.go' ! -name '*_test.go' ! -path '*/testutil/*' \
  ! -path 'cmd/slabledger/main.go' -type f -exec wc -l {} + \
  | sort -rn | awk '$1>500 && $2!="total"'
```

As of 2026-08-08:

| File | LOC | Why it's large |
|------|-----|----------------|
| `domain/inventory/core_types.go` | 562 | Core domain types: Campaign, Purchase, Sale, Phase, DH status |
| `domain/portfolio/service.go` | 540 | Portfolio health, insights, capital timeline, weekly review |
| `adapters/scheduler/cardladder_refresh.go` | 525 | CardLadder refresh job: options, wiring, single-purchase pricing |
| `adapters/httpserver/handlers/campaigns_purchases.go` | 520 | Purchase and sale CRUD handlers, price overrides, cert lookup |

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
