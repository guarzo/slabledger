# Architecture

## Overview

slabledger is a graded card portfolio tracker and pricing tool using Hexagonal Architecture. The system manages PSA grading campaigns, tracks multi-channel sales (eBay, TCGPlayer, local), computes P&L analytics, and provides market direction signals via fused pricing from multiple sources.

**Stack**: Go 1.26 | SQLite (WAL) | stdlib net/http mux | slog logging | React + TypeScript + Vite + Tailwind

## Hexagonal Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        ADAPTERS LAYER                       │
│  Inbound:                    Outbound:                      │
│  • HTTP Handlers             • PriceCharting, CardHedger,   │
│  • Web Server                • TCGdex.dev                   │
│                              • TCGdex.dev                   │
│                              • Google OAuth                 │
│                              • SQLite Storage               │
│                              • PriceLookup (market signals) │
└─────────────────────────────────────────────────────────────┘
                              ↓ Interfaces (defined by domain)
┌─────────────────────────────────────────────────────────────┐
│                         DOMAIN LAYER                        │
│          (Pure Business Logic - NO external deps)           │
│                                                             │
│  • Campaigns Service       • P&L Analytics                  │
│  • Price Fusion            • Market Direction Signals       │
│  • Favorites               • CSV Import                     │
│  • Authentication          • Channel Fee Calculation        │
│                                                             │
│  Interfaces: PriceProvider, CardRepository, PriceLookup     │
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
    campaigns/              # Campaign tracking, purchases, sales, analytics
      types.go              # Campaign, Purchase, Sale, Phase, SaleChannel
      repository.go         # Repository interface (CRUD + analytics queries)
      service.go            # Business logic, PriceLookup interface, ServiceOption
      validation.go         # Input validation
      channel_fees.go       # CalculateSaleFee, CalculateNetProfit
      analytics_types.go    # CampaignPNL, ChannelPNL, DailySpend, AgingItem, etc.
      import.go             # ImportRow, ImportResult, ExtractGrade
      errors.go             # Sentinel errors (ErrCampaignNotFound, etc.)
    cards/                  # CardRepository interface
    favorites/              # Favorites service
    observability/          # Logger, MetricsRecorder interfaces
    pricing/                # PriceProvider, Price, GradedPrices, LastSoldByGrade
    fusion/                 # Price fusion interfaces
    mathutil/               # CalculateTrend, CalculatePercentChange, etc.

  adapters/                 # Interface implementations
    httpserver/             # Inbound HTTP
      handlers/             # CampaignsHandler, AuthHandlers, FavoritesHandlers, etc.
      middleware/           # Auth, CORS, rate limiting, recovery
      router.go             # Route registration with auth gating
    clients/
      fusionprice/          # Multi-source price fusion (CardHedger + PriceCharting market data)
      pricelookup/          # PriceLookup adapter (wraps PriceProvider for campaigns)
      pricecharting/        # PriceCharting API client
      cardhedger/           # CardHedger secondary pricing
      tcgdex/               # TCGdex.dev card/set metadata (EN + JA)
      google/               # Google OAuth service
      httpx/                # Unified HTTP client with retry + circuit breaker
      cardutil/             # Card name normalization
    storage/sqlite/         # SQLite repository implementations + migrations + card_id_mappings + sync_state
    scheduler/              # Background jobs (price refresh, session cleanup)

  platform/                 # Cross-cutting concerns
    cache/                  # Type-safe cache (memory + file)
    config/                 # Configuration loading
    crypto/                 # AES encryption for auth tokens
    errors/                 # Platform error types (AppError)
    resilience/             # Retry and circuit breaker
    telemetry/              # slog-based structured logging
```

## Key Interfaces

```go
// Pricing
type PriceProvider interface {
    GetPrice(ctx context.Context, card Card) (*Price, error)
    Available() bool
    Name() string
    Close() error
    LookupCard(ctx context.Context, setName string, card Card) (*Price, error)
    GetStats(ctx context.Context) *ProviderStats
}

// Card metadata
type CardRepository interface {
    GetCards(ctx context.Context, setName string) ([]Card, error)
    SearchCard(ctx context.Context, name string) (*Card, error)
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

// Fusion domain interfaces (internal/domain/fusion/source.go)
type SecondaryPriceSource interface {
    FetchFusionData(ctx, setName, cardName, cardNumber string) (*FetchResult, int, http.Header, error)
    Available() bool
    Name() string
}

type CardIDResolver interface {
    GetExternalID(ctx, cardName, setName, provider string) (string, error)
    SaveExternalID(ctx, cardName, setName, provider, externalID string) error
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

### Price Fusion
```
1. CardHedger → graded price estimates with confidence ranges
2. PriceCharting → market data (active listings, sales velocity, grade prices)
3. Fusion provider merges, detects outliers, computes confidence
4. Result cached in SQLite + memory with configurable TTL
```

## Dependency Injection

All dependencies injected via constructors in `main.go`:

```go
// Secondary sources (each implements fusion.SecondaryPriceSource)
secondarySources := []fusion.SecondaryPriceSource{ppSource, chSource}
priceProvImpl := fusionprice.NewFusionProviderWithRepo(pcProvider, secondarySources, ...)

// Campaigns with optional market signals
priceLookupAdapter := pricelookup.NewAdapter(priceProvImpl)
campaignsService := campaigns.NewService(campaignsRepo, campaigns.WithPriceLookup(priceLookupAdapter))

// HTTP server
deps := ServerDependencies{CampaignsService: campaignsService, ...}
```

## Adding New Features

1. Define interface in `internal/domain/{feature}/`
2. Implement in `internal/adapters/clients/{provider}/` or `internal/adapters/storage/`
3. Wire in `main.go`
4. Add compile-time check: `var _ campaigns.Repository = (*CampaignsRepository)(nil)`
5. Use functional options for optional dependencies: `WithFoo(impl) ServiceOption`

### Access Control Model

SlabLedger uses a single-tenant model: one instance per user/household.
The email allowlist (`ADMIN_EMAILS` env var) controls who can authenticate,
but all authenticated users share the same campaign data. There is no
per-user data isolation by design.

If multi-tenant support is ever needed, add a `user_id` column to all data
tables and filter queries accordingly.

---

## Access Control Model

SlabLedger is a **single-tenant** application. An email allowlist (`ADMIN_EMAILS` environment variable) gates authentication via Google OAuth. All authenticated users share the same campaign data, pricing caches, and favorites. There is no per-user data isolation, row-level security, or role hierarchy beyond the admin flag.

To support multi-tenant usage, the following changes would be required:

1. Add a `user_id` foreign key to campaigns, purchases, sales, and favorites tables.
2. Enforce tenant scoping in every repository query.
3. Introduce a `tenant_id` or `org_id` concept if multiple users should share data within an organization.
4. Add authorization middleware that validates resource ownership on each request.

---

## Key Design Decisions

### Campaign Tracking Pivot (Feb 2026)

**Problem**: Original app focused on finding raw cards to buy and grade. Business pivoted to PSA Direct Buy campaigns where PSA sources already-graded cards.

**Decision**: New `campaigns/` domain package with purchase/sale tracking, multi-channel P&L, market direction signals. Removed unused scoring, opportunity detection, eBay deal detection, PSA population analysis.

**Result**: Clean separation between campaign tracking (new core feature) and card pricing (retained for market signals and favorites).

### PriceLookup Interface (Dependency Inversion)

**Problem**: Campaign domain needs market price data for signals, but shouldn't import the pricing package directly.

**Decision**: Define `PriceLookup` interface in campaigns domain. Adapter in `clients/pricelookup/` wraps `PriceProvider`. Injected via functional option `WithPriceLookup`.

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

**Result**: Cache latency 30us, memory stable, high test coverage, single circuit breaker implementation.

### Cache Performance (Oct 2025)

**Decision**: Remove JSON from hot path, use direct type assertions with `TypedCache[T]` generics. Cache Get() <50us.

### FetchResult Pattern (Mar 2026)

**Problem**: Secondary price sources used shared mutable maps for side-channel data, causing data races in concurrent fusion calls.

**Decision**: Introduced `fusion.FetchResult` struct that bundles grade data with optional per-grade details. Each `FetchFusionData` call returns its own result, eliminating shared state.

**Result**: No shared mutable state between concurrent calls. Domain interfaces (`SecondaryPriceSource`, `CardIDResolver`) moved from adapter to `internal/domain/fusion/source.go`.

### CardHedger Integration (Mar 2026)

**Problem**: Needed a reliable secondary price source for fusion confidence.

**Decision**: Added CardHedger as the secondary fusion source with dedicated background schedulers (delta poll + daily batch), card ID mapping cache in SQLite, and configurable scheduler intervals via environment variables.

**Result**: Two-source fusion (CardHedger + PriceCharting market data) with confidence scoring. CardHedger usage tracked via atomic counters and CAS-based daily reset with 429 monitoring.

### Codebase Simplification (Feb 2026)

**Decision**: Removed dead code (scoring engine, opportunity detection, eBay deals, PSA population, marketplace timing, monitoring/alerts), simplified managers to plain functions, consolidated duplicate endpoints.

---

## Domain Interfaces

| Package | Interface | File | Methods | Purpose |
|---------|-----------|------|---------|---------|
| `campaigns` | `Service` | `service.go` | ~40 | Full campaign business logic |
| `campaigns` | `Repository` | `repository.go` | composed | Composed: CRUD + Purchase + Sale + Analytics + Finance + Revocation |
| `campaigns` | `PriceLookup` | `service.go` | 2 | Market signals for inventory aging |
| `campaigns` | `CertLookup` | `service.go` | 1 | PSA cert → card details |
| `campaigns` | `CardIDResolver` | `service.go` | 1 | Batch cert → external card ID |
| `pricing` | `PriceProvider` | `provider.go` | 5 | Card price lookup (PriceCharting) |
| `pricing` | `PriceRepository` | `repository.go` | ~10 | Price history persistence |
| `pricing` | `APITracker` | `repository.go` | 3 | Rate limit state tracking |
| `pricing` | `AccessTracker` | `repository.go` | 1 | Card access log |
| `pricing` | `HealthChecker` | `repository.go` | 1 | Provider health |
| `pricing` | `DiscoveryFailureTracker` | `repository.go` | 3 | Failed discovery persistence |
| `pricing` | `PricingDiagnosticsProvider` | `repository.go` | 1 | Diagnostics data |
| `auth` | `Service` | `service.go` | 14 | OAuth flow, session management, allowlist |
| `auth` | `Repository` | `repository.go` | ~14 | Auth persistence |
| `social` | `Service` | `service.go` | 8 | Social post generation and publishing |
| `social` | `Publisher` | `service.go` | 1 | Instagram carousel publish |
| `social` | `InstagramTokenProvider` | `service.go` | 1 | Instagram credentials |
| `social` | `Repository` | `repository.go` | ~8 | Social post persistence |
| `advisor` | `Service` | `service.go` | 6 | AI advisor analysis (streaming) |
| `advisor` | `CacheStore` | `cache.go` | 5 | Advisor result persistence |
| `ai` | `LLMProvider` | `llm.go` | 1 | LLM completion (Azure AI) |
| `ai` | `AICallTracker` | `tracking.go` | 1 | AI call metrics |
| `ai` | `ToolExecutor` | `tools.go` | 1 | Tool call execution |
| `ai` | `FilteredToolExecutor` | `tools.go` | 1 | Subset tool execution |
| `cards` | `CardProvider` | `provider.go` | 5 | Card/set search (TCGdex) |
| `cards` | `NewSetIDsProvider` | `provider.go` | 1 | New set discovery |
| `fusion` | `SecondaryPriceSource` | `source.go` | 3 | Price fusion data (CardHedger) |
| `fusion` | `CardIDResolver` | `source.go` | 3 | External ID cache |
| `fusion` | `PriceHintResolver` | `source.go` | 4 | User-provided price hints |
| `favorites` | `Service` | `service.go` | 6 | Favorites CRUD |
| `favorites` | `Repository` | `repository.go` | ~6 | Favorites persistence |
| `observability` | `Logger` | `logger.go` | 5 | Structured logging |
