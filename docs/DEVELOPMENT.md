# Development Guide

Operational reference for caching, rate limiting, API integrations, and common gotchas.

## Price Units (Critical)

**Campaign API endpoints use cents (integers). Pricing/card API endpoints use dollars (floats).**

### Convention

| Layer | Format | Example |
|-------|--------|---------|
| Backend internal | Cents (int) | `355239` |
| Campaign API responses | Cents (int) | `355239` |
| Pricing API responses | Dollars (float64) | `3552.39` |
| Frontend display | Dollars | `$3,552.39` |

Campaign endpoints (`/api/campaigns/*`) return cents. The frontend converts with `(cents / 100).toFixed(2)`.

Pricing endpoints (`/api/cards/search`) return dollars. Display directly.

Key files:
- `internal/adapters/httpserver/handlers/formatter.go` — converts cents to dollars for pricing endpoints
- `web/src/react/pages/CampaignDetailPage.tsx` — `formatCents()` helper for campaign data

---

## Campaign Domain

### Key Files

| File | Purpose |
|------|---------|
| `internal/domain/campaigns/types.go` | Campaign, Purchase, Sale structs |
| `internal/domain/campaigns/service.go` | Business logic, PriceLookup interface |
| `internal/domain/campaigns/channel_fees.go` | CalculateSaleFee, CalculateNetProfit |
| `internal/domain/campaigns/import.go` | ImportRow, ExtractGrade |
| `internal/domain/campaigns/analytics_types.go` | CampaignPNL, ChannelPNL, AgingItem, etc. |
| `internal/adapters/storage/sqlite/campaigns_repository.go` | SQLite implementation |
| `internal/adapters/httpserver/handlers/campaigns.go` | HTTP handlers |
| `internal/adapters/clients/pricelookup/adapter.go` | PriceLookup adapter |

### Functional Options Pattern

Optional dependencies use functional options:

```go
// Domain defines interface
type PriceLookup interface {
    GetLastSoldCents(ctx context.Context, cardName string, grade int) (int, error)
}

// Service accepts options
type ServiceOption func(*service)
func WithPriceLookup(pl PriceLookup) ServiceOption { ... }
func NewService(repo Repository, opts ...ServiceOption) Service { ... }

// Wiring in main.go
adapter := pricelookup.NewAdapter(priceProvider)
svc := campaigns.NewService(repo, campaigns.WithPriceLookup(adapter))
```

### Database Migrations

Migrations are in `internal/adapters/storage/sqlite/migrations/`. Current migrations:

| Migration | Description |
|-----------|-------------|
| 000001 | Complete initial schema (campaigns, purchases, sales, price_history, price_refresh_queue, favorites, sessions, api_calls, etc.) |
| 000002 | Card ID mappings table + sync state (for CardHedger external ID caching and delta poll state) |
| 000003 | API daily summary view (aggregates api_calls by provider and date for status endpoint) |

---

## Cache System

### Cache Types

| Type | TTL | Use Case |
|------|-----|----------|
| Memory | 2-4 hours | Hot data, frequently accessed |
| File | 24 hours | Persistence across restarts |
| SQLite | 24+ hours | Price persistence with background refresh |

### Recommended TTLs

| Data | TTL | Rationale |
|------|-----|-----------|
| Sets | 24 hours | Rarely changes |
| Cards | 6 hours | Occasional updates |
| Prices | 1-4 hours | Changes frequently |

### Type-Safe Cache API

```go
cardCache := cache.NewCardSliceCache(baseCache)

// GetOrLoad pattern (cache-aside)
cards, err := cardCache.GetOrLoad(ctx, key, func() ([]model.Card, error) {
    return fetchFromAPI()
}, 6*time.Hour)
```

---

## Rate Limiting

| Provider | Limit | Notes |
|----------|-------|-------|
| PriceCharting | 60/min, 20k/day | Market data (listings, sales velocity) |
| PokemonPriceTracker | 60/min | Primary graded price source (2 credits with eBay data) |
| CardHedger | 60/min (unlimited plan) | Supplementary pricing estimates (429-monitored) |

---

## Circuit Breaker

Prevents cascading failures when APIs are down.

**States**: Closed (normal) -> Open (fast-fail) -> Half-Open (testing recovery)

```go
CircuitBreakerConfig{
    FailureRatio:     0.6,              // 60% failures trips breaker
    Timeout:          60 * time.Second, // Wait before half-open
    SuccessThreshold: 2,                // Successes to close
}
```

---

## Retry Policy

- Max retries: 3
- Initial backoff: 1s, max: 30s, factor: 2x
- Retryable: timeouts, connection reset, 429, 502, 503, 504
- Non-retryable: 400, 401, 404, JSON parse errors

---

## API Integrations

### PriceCharting

```bash
export PRICECHARTING_TOKEN="your_token"
```

**Price field mapping** (all returned in cents):

| API Field | Grade |
|-----------|-------|
| `loose-price` | Raw/Ungraded |
| `new-price` | PSA 8 |
| `graded-price` | PSA 9 |
| `manual-only-price` | PSA 10 |
| `bgs-10-price` | BGS 10 Black Label |

Code: `internal/adapters/clients/pricecharting/`

### TCGdex.dev

No API key required. Provides card metadata (name, set, number, image URL) for English and Japanese cards.

Code: `internal/adapters/clients/tcgdex/`

### CardHedger

```bash
export CARD_HEDGER_API_KEY="your_key"  # Supplementary pricing (unlimited plan)
```

Provides multi-platform price estimates with confidence ranges. Used as a secondary fusion source alongside PokemonPrice.
- Daily limit tracked via atomic counter with CAS-based daily reset
- Card ID mapping cached in SQLite (`card_id_mappings` table)
- Background schedulers: delta poll (1h) + daily batch refresh

Code: `internal/adapters/clients/cardhedger/`

### Fusion Price Provider

Merges data from PokemonPrice (primary graded prices), CardHedger (supplementary estimates), and PriceCharting (market data) into a single `Price` struct with confidence scores and fusion metadata. Uses `fusion.FetchResult` pattern to avoid shared mutable state.

Code: `internal/adapters/clients/fusionprice/`

### PriceLookup Adapter

Wraps `PriceProvider` to implement the campaigns domain's `PriceLookup` interface. Extracts per-grade last-sold prices from `Price.LastSoldByGrade`:

```go
// Grade mapping
10 -> PSA10.LastSoldPrice
9  -> PSA9.LastSoldPrice
8  -> PSA8.LastSoldPrice
*  -> Raw.LastSoldPrice
```

Code: `internal/adapters/clients/pricelookup/adapter.go`

---

## Monitoring

```bash
# Health check
curl http://localhost:8081/api/health

# Cache statistics
./slabledger admin cache-stats

# API usage status (per-provider call counts, success rates, latency)
curl http://localhost:8081/api/status/api-usage
```

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Low cache hit rate (<50%) | Increase `CACHE_MAX_ENTRIES` or TTLs |
| High memory usage | Decrease cache limits, clear `data/cache/*` |
| Circuit breaker stuck open | Wait 60s, check API status, review logs |
| Rate limit errors | Reduce concurrent workers, check quotas |
| Market signals missing | Verify PriceCharting token is set; check `PriceLookup` wiring |
| CSV import skipping all rows | Check CSV format: 3 columns, header row required |
| Duplicate cert errors | Certificate numbers are unique across all campaigns |
| CardHedger 429 errors | Unlimited plan; 429s indicate actual rate limiting. Check via /api/status/api-usage |

---

## Domain Simplifications

- **Cost calculations**: Use `CalculateSaleFee()` and `CalculateNetProfit()` functions (no manager pattern)
- **Grade extraction**: `ExtractGrade(title)` parses PSA grade from card title strings
- **Object allocation**: Standard `new()` / `make()` — no sync.Pool
- **Optional deps**: Functional options (`WithPriceLookup`) instead of required constructor params
