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
│    │   ├── cardhedger/     (supplementary pricing) │
│    │   ├── cardutil/       (card utilities)     │
│    │   ├── fusionprice/    (multi-source fusion)│
│    │   ├── google/         (Google OAuth)       │
│    │   ├── httpx/          (shared HTTP client) │
│    │   ├── pokemonprice/   (primary graded prices) │
│    │   ├── tcgdex/         (card metadata)      │
│    │   ├── pricecharting/  (graded prices + market) │
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
│    ├── auth/           (authentication)         │
│    ├── campaigns/      (campaign tracking, P&L) │
│    ├── cards/          (card interfaces)        │
│    ├── constants/      (shared constants)       │
│    ├── favorites/      (favorites management)   │
│    ├── fusion/         (price fusion interfaces)│
│    ├── mathutil/       (math utilities)         │
│    ├── observability/  (logger interfaces)      │
│    ├── pricing/        (price interfaces/models)│
│    │   └── analysis/   (pricing analysis)       │
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
| `campaigns/` | Campaign tracking, purchases, sales, P&L, analytics, CSV import |
| `cards/` | `CardRepository` interface for card metadata |
| `constants/` | Shared application constants |
| `favorites/` | Favorites management |
| `fusion/` | Price fusion interfaces |
| `mathutil/` | Math utility functions |
| `observability/` | Logger, MetricsRecorder interfaces |
| `pricing/` | `PriceProvider` interface, graded prices, market data models |
| `pricing/analysis/` | Pricing analysis logic |
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
package pricecharting

type Client struct { ... }

func (c *Client) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    // API call implementation
}

// 3. Wire in main.go
priceClient := pricecharting.NewClient(...)
service := someservice.NewService(priceClient) // Inject interface
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
│   ├── cardhedger/      # CardHedger supplementary pricing
│   ├── cardutil/        # Card utility functions
│   ├── fusionprice/     # Multi-source price fusion (PokemonPrice + CardHedger)
│   ├── google/          # Google OAuth client
│   ├── httpx/           # Shared HTTP client (retry, circuit breaker)
│   ├── pokemonprice/    # PokemonPrice primary graded price source
│   ├── tcgdex/          # TCGdex.dev card/set metadata (EN + JA)
│   ├── pricecharting/   # PriceCharting graded prices + market data
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
package pricecharting

import "github.com/guarzo/slabledger/internal/domain/pricing"

// Client implements pricing.PriceProvider interface
type Client struct {
    httpClient *httpx.Client
    apiToken   string
    logger     observability.Logger
}

var _ pricing.PriceProvider = (*Client)(nil) // Compile-time interface check

func (c *Client) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
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
// internal/domain/campaigns/service.go
import "github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"

type Service struct {
    priceClient *pricecharting.Client // ❌ Direct dependency on adapter
}
```

**Good**:
```go
// internal/domain/campaigns/service.go
import "github.com/guarzo/slabledger/internal/domain/pricing"

type Service struct {
    priceProvider pricing.PriceProvider // ✅ Depends on interface
}
```

---

## Testing Strategy by Layer

### Domain Layer Testing
```go
// internal/domain/campaigns/service_test.go
func TestService_GetPNL(t *testing.T) {
    // ✅ Use mock providers (no real API calls)
    mockPrice := &mocks.MockPriceProvider{
        GetPriceFunc: func(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
            return &pricing.Price{RawUSD: 10}, nil
        },
    }

    service := campaigns.NewService(mockPrice)
    pnl, err := service.GetPNL(context.Background(), campaignID)
    assert.NoError(t, err)
}
```

### Adapter Layer Testing
```go
// internal/adapters/clients/pricecharting/client_test.go
func TestClient_GetPrice(t *testing.T) {
    // ✅ Use mock HTTP client (no real network calls)
    mockHTTP := mocks.NewMockHTTPClientWithResponse(`{"price": 100}`)

    client := pricecharting.NewClientWithHTTP(mockHTTP, "test-token")

    price, err := client.GetPrice(context.Background(), testCard)
    assert.NoError(t, err)
}
```

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
