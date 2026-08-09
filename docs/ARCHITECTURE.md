# Architecture

## Overview

slabledger is a graded card portfolio tracker and pricing tool using Hexagonal Architecture. The system manages PSA grading campaigns, tracks multi-channel sales (eBay, TCGPlayer, local), computes P&L analytics, and provides market direction signals via DH (DoubleHolo) pricing.

**Stack**: Go 1.26 | Postgres (pgx) | stdlib net/http mux | slog logging | React + TypeScript + Vite + Tailwind

## Hexagonal Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        ADAPTERS LAYER                       │
│  Inbound:                    Outbound:                      │
│  • HTTP Handlers             • DH (DoubleHolo) Pricing      │
│  • Web Server                • CardLadder Valuations        │
│                              • Google OAuth                 │
│                              • Postgres Storage             │
│                              • PriceLookup (market signals) │
└─────────────────────────────────────────────────────────────┘
                              ↓ Interfaces (defined by domain)
┌─────────────────────────────────────────────────────────────┐
│                         DOMAIN LAYER                        │
│          (Pure Business Logic - NO external deps)           │
│                                                             │
│  • Inventory Service       • P&L Analytics                  │
│  • DH Pricing              • Market Direction Signals       │
│  • Portfolio Health        • CSV Import                     │
│  • Authentication          • Channel Fee Calculation        │
│                                                             │
│  Interfaces: PriceProvider, PriceLookup                     │
└─────────────────────────────────────────────────────────────┘
                              ↑
┌─────────────────────────────────────────────────────────────┐
│                       PLATFORM LAYER                        │
│  • Configuration            • Telemetry (slog)              │
│  • HTTP Client (retry+breaker) • Error Types                │
│  • Cache                    • Rate Limiting                 │
│  • Crypto (AES)             • Resilience                    │
└─────────────────────────────────────────────────────────────┘
```

**Key Principle**: Dependencies point inward. Domain defines interfaces, adapters implement them.

## Package Structure

```
internal/
  domain/                   # Pure business logic
    auth/                   # Authentication service interface
    inventory/              # Campaign tracking, purchases, sales, analytics
      core_types.go         # Campaign, Purchase, Sale, Phase, SaleChannel
      repository_*.go       # The focused repository interfaces (plus pending_items.go)
      service_interfaces.go # Service-side interfaces (Analytics, Cert, Pricing, DH, Snapshot)
      service.go            # Business logic, PriceLookup interface, ServiceOption
      channel_fees.go       # CalculateSaleFee, CalculateNetProfit
      errors.go             # Sentinel errors (ErrCampaignNotFound, etc.)
    arbitrage/              # Crack candidates, acquisition targets, EV, Monte Carlo
    portfolio/              # Inventory aging, price signals, portfolio health
    tuning/                 # Campaign parameter optimization and analytics
    finance/                # Invoices, cashflow, capital tracking, revocation flags
    export/                 # Sell sheet generation
    dhlisting/              # DH listing push pipeline coordination
    observability/          # Logger, MetricsRecorder interfaces
    pricing/                # PriceProvider, Price, GradedPrices, LastSoldByGrade
    mathutil/               # CalculateTrend, CalculatePercentChange, etc.

  adapters/                 # Interface implementations
    httpserver/             # Inbound HTTP
      handlers/             # CampaignsHandler, AuthHandlers, etc.
      middleware/           # Auth, CORS, rate limiting, recovery
      router.go             # Route registration with auth gating
    clients/
      dh/                   # DH (DoubleHolo) API client
      dhprice/              # DH pricing (PriceProvider implementation)
      dhlisting/            # DH listing pushes
      cardladder/           # CardLadder valuations
      psa/                  # PSA APIs
      psaportal/            # PSA portal session
      google/               # Google OAuth service
      httpx/                # Unified HTTP client with retry + circuit breaker
    storage/postgres/       # Postgres repository implementations + migrations
    scheduler/              # Background jobs (price refresh, session cleanup)

  platform/                 # Cross-cutting concerns
    canonjson/              # Canonical JSON encoding (stable key order)
    cardutil/               # Card name/set normalization
    config/                 # Configuration loading
    crypto/                 # AES encryption for auth tokens
    resilience/             # Retry and circuit breaker
    storage/                # File-based persistence for structured data
    telemetry/              # slog-based structured logging
```

The domain listing above is orientation, not a roster — it names the packages whose purpose
is not obvious from the directory name. For the current set, run `go list ./internal/domain/...`
and `ls internal/adapters internal/adapters/clients internal/platform`.

## Key Interfaces

```go
// Pricing
type PriceProvider interface {
    GetPrice(ctx context.Context, card Card) (*Price, error)
    Available() bool
    Name() string
    LookupCard(ctx context.Context, setName string, card Card) (*Price, error)
}

// Market signals for campaigns (dependency inversion)
type PriceLookup interface {
    GetLastSoldCents(ctx context.Context, cardName string, grade int) (int, error)
}

// Campaign persistence
type Repository interface {
    // Campaign CRUD
    CreateCampaign(ctx, *Campaign) error
    GetCampaign(ctx, id) (*Campaign, error)
    ListCampaigns(ctx, includeArchived bool) ([]Campaign, error)
    UpdateCampaign(ctx, *Campaign) error
    ArchiveCampaign(ctx, id) error
    // Purchase + Sale CRUD
    // Analytics queries (PNL, channel breakdown, fill rate, etc.)
}
```

## Data Flow

### Campaign P&L
```
1. User creates campaign with buy parameters (CL%, grade range, spend cap)
2. Purchases recorded with cert number, grade, cost, CL value
3. Sales recorded with channel (eBay/TCGPlayer/local/other)
4. Service computes: sale fee (channel-aware), days-to-sell, net profit
5. Analytics: aggregate P&L, channel breakdown, fill rate, inventory aging
```

### Market Direction Signals
```
1. GetInventoryAging() called for unsold cards
2. For each card, PriceLookup.GetLastSoldCents() fetches real-time sold price
3. Compare against recorded CL valuation (clValueCents)
4. Delta > +5%: "rising" → suggest eBay/TCGPlayer
5. Delta < -5%: "falling" → suggest local (lock in before CL drops)
6. Within ±5%: "stable" → either channel works
```

### Pricing
```
1. DH (DoubleHolo) → graded price estimates, market data, sales history
2. Results cached in Postgres + memory with configurable TTL
```

## Dependency Injection

All dependencies injected via constructors in `main.go`:

```go
// Pricing via DH
priceProvImpl := dhprice.NewProvider(dhClient, ...)

// Inventory service with optional market signals
priceLookupAdapter := lookup.NewAdapter(priceProvImpl)
inventoryService := inventory.NewService(repos, inventory.WithPriceLookup(priceLookupAdapter))

// HTTP server
deps := ServerDependencies{InventoryService: inventoryService, ...}
```

## Adding New Features

1. Define interface in `internal/domain/{feature}/`
2. Implement in `internal/adapters/clients/{provider}/` or `internal/adapters/storage/`
3. Wire in `main.go`
4. Add compile-time check: `var _ inventory.Repository = (*InventoryRepository)(nil)`
5. Use functional options for optional dependencies: `WithFoo(impl) ServiceOption`

### Access Control Model

SlabLedger is a **single-tenant** application. An email allowlist (`ADMIN_EMAILS` environment variable) gates authentication via Google OAuth. All authenticated users share the same campaign data and pricing caches. There is no per-user data isolation, row-level security, or role hierarchy beyond the admin flag.

To support multi-tenant usage, the following changes would be required:

1. Add a `user_id` foreign key to the campaigns, purchases, and sales tables.
2. Enforce tenant scoping in every repository query.
3. Introduce a `tenant_id` or `org_id` concept if multiple users should share data within an organization.
4. Add authorization middleware that validates resource ownership on each request.

---

## Key Design Decisions

### Campaign Tracking Pivot (Feb 2026)

**Problem**: Original app focused on finding raw cards to buy and grade. Business pivoted to PSA Direct Buy campaigns where PSA sources already-graded cards.

**Decision**: New `inventory/` domain package with purchase/sale tracking, multi-channel P&L, market direction signals. Removed unused scoring, opportunity detection, eBay deal detection, PSA population analysis.

**Result**: Clean separation between campaign tracking (new core feature) and card pricing (retained for market signals).

### PriceLookup Interface (Dependency Inversion)

**Problem**: Campaign domain needs market price data for signals, but shouldn't import the pricing package directly.

**Decision**: Define `PriceLookup` interface in inventory domain. Adapter in `domain/pricing/lookup/` wraps `PriceProvider` — it lives in the domain rather than under `adapters/` because grade interpolation and fallback are domain computations on pricing data, not external API concerns. Injected via functional option `WithPriceLookup`.

**Result**: Domain stays pure, market signals work when price provider is available, gracefully degrade when not.

### Channel-Aware Fee Calculation

**Problem**: Different sell channels have different fee structures.

**Decision**: `CalculateSaleFee` uses campaign's `ebayFeePct` for eBay/TCGPlayer, 0 for local/other. Net profit = sale price - buy cost - sourcing fee - sale fee.

### Cents Everywhere

**Problem**: Floating point rounding in financial calculations.

**Decision**: All monetary values stored as integer cents in backend. API responses convert to dollars at the boundary. Frontend displays dollars.

### Hexagonal Architecture (Oct 2025)

**Problem**: Scattered resilience patterns, poor testability.

**Decision**: Strict dependency inversion. Domain defines interfaces, adapters implement.

**Result**: Domain stays pure, high test coverage, single circuit breaker implementation.

### Cache Removal (Aug 2026, SLA-47)

**Problem**: `platform/cache` (`TypedCache[T]`, memory + file backends) had exactly one
importer tree-wide — an integration-gated test helper with no callers of its own — and
`cache.New` had no non-test callers at all. SLA-19 had already removed the live half of
the plumbing (`Config.Cache.Path`, the `-cache` flag), leaving the package dead.

**Decision**: Delete the package, along with the container plumbing that provisioned a
cache directory nothing wrote (`/app/cache` in the Dockerfile, the `./cache` bind mount
in both compose files).

**Note**: This supersedes an earlier "Cache Performance" entry that recorded tuning
`TypedCache[T]` down to sub-50us gets. That work went with the package; the numbers are
removed rather than left to read as current.

### Codebase Simplification (Feb 2026)

**Decision**: Removed dead code (scoring engine, opportunity detection, eBay deals, PSA population, marketplace timing, monitoring/alerts), simplified managers to plain functions, consolidated duplicate endpoints.

---

## Domain Interfaces

| Package | Interface | File | Methods | Purpose |
|---------|-----------|------|---------|---------|
| `inventory` | `Service` | `service.go` | ~40 | Full campaign/inventory business logic |
| `inventory` | `CampaignRepository` | `repository_campaign.go` | 6 | Campaign CRUD |
| `inventory` | `PurchaseRepository` | `repository_purchase.go` | 55 | Composite of the six focused purchase ports below |
| `inventory` | `PurchaseCoreRepository` | `repository_purchase.go` | 11 | Purchase CRUD and listing |
| `inventory` | `PurchaseFieldRepository` | `repository_purchase.go` | 14 | Single-field updates |
| `inventory` | `PurchasePricingRepository` | `repository_purchase.go` | 5 | Price and review-price writes |
| `inventory` | `PurchaseEbayExportRepository` | `repository_purchase.go` | 3 | eBay export state |
| `inventory` | `PurchaseSnapshotRepository` | `repository_purchase.go` | 2 | Market snapshot writes |
| `inventory` | `PurchaseDHRepository` | `repository_purchase_dh.go` | 20 | DH linkage, push state, listing prices |
| `inventory` | `SaleRepository` | `repository_sale.go` | 7 | Sale CRUD |
| `inventory` | `AnalyticsRepository` | `repository_analytics.go` | 10 | PNL, aging, channel analytics |
| `inventory` | `FinanceRepository` | `repository_finance.go` | 14 | Capital, cashflow, invoices |
| `inventory` | `PricingRepository` | `repository_pricing.go` | 8 | Cached pricing reads/writes |
| `inventory` | `DHRepository` | `repository_dh.go` | 2 | DH config persistence |
| `inventory` | `PendingItemRepository` | `pending_items.go` | 6 | Pending-item queue |
| `inventory` | `PriceLookup` | `service.go` | 2 | Market signals for inventory aging |
| `pricing` | `PriceProvider` | `provider.go` | 5 | Card price lookup (DH) |
| `pricing` | `APITracker` | `repository.go` | 3 | Rate limit state tracking |
| `pricing` | `AccessTracker` | `repository.go` | 1 | Card access log |
| `pricing` | `HealthChecker` | `repository.go` | 1 | Provider health |
| `auth` | `Service` | `service.go` | 14 | OAuth flow, session management, allowlist |

`PurchaseRepository` is the composite (SLA-37); depend on the narrowest focused port that
covers your use. The Postgres `PurchaseStore` satisfies all of them, so taking the
composite buys nothing but a wider mock. `inventory.Service` is the one legitimate holder
of the composite — it genuinely spans every concern.
| `pricing` | `PriceProvider` | `provider.go` | 5 | Card price lookup (DH) |
| `pricing` | `APITracker` | `repository.go` | 3 | Rate limit state tracking |
| `pricing` | `AccessTracker` | `repository.go` | 1 | Card access log |
| `pricing` | `HealthChecker` | `repository.go` | 1 | Provider health |
| `auth` | `Service` | `service.go` | 14 | OAuth flow, session management, allowlist |
| `auth` | `Repository` | `repository.go` | ~14 | Auth persistence |
| `observability` | `Logger` | `logger.go` | 5 | Structured logging |
