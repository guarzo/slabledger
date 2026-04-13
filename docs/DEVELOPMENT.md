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
| `internal/domain/inventory/types.go` | Campaign, Purchase, Sale structs |
| `internal/domain/inventory/service.go` | Business logic, PriceLookup interface |
| `internal/domain/inventory/channel_fees.go` | CalculateSaleFee, CalculateNetProfit |
| `internal/domain/inventory/service_interfaces.go` | 8 focused repository interfaces |
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
func NewService(repos Repositories, opts ...ServiceOption) Service { ... }

// Wiring in main.go
adapter := pricelookup.NewAdapter(priceProvider)
svc := inventory.NewService(repos, inventory.WithPriceLookup(adapter))
```

### Database Migrations

Migrations are in `internal/adapters/storage/sqlite/migrations/`. There are currently 60 migration pairs (`000001`–`000060`). See [docs/SCHEMA.md](SCHEMA.md) for the full schema and [internal/README.md](../internal/README.md) for step-by-step migration creation instructions.

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
| DH (DoubleHolo) | Enterprise plan | Graded pricing and market data |

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

### TCGdex.dev

No API key required. Provides card metadata (name, set, number, image URL) for English and Japanese cards.

Code: `internal/adapters/clients/tcgdex/`

### DH Price Provider

Provides graded card pricing via the DoubleHolo enterprise API. Returns price estimates, market data, and sales history for PSA-graded cards.

Code: `internal/adapters/clients/dhprice/`

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
| Market signals missing | Verify DH_ENTERPRISE_API_KEY is set; check `PriceLookup` wiring |
| CSV import skipping all rows | Check CSV format: 3 columns, header row required |
| Duplicate cert errors | Certificate numbers are unique across all campaigns |
| `database is locked` | WAL mode issue or concurrent write contention. Check `PRAGMA journal_mode=wal;` runs on startup |
| `mock does not implement interface` | Repository interface changed. Add missing method to both mocks (`testutil/mocks/` and `domain/inventory/mock_repo_test.go`) |
| Frontend proxy 502 | Backend not running on :8081. Start backend: `go run ./cmd/slabledger` |
| `migration: dirty database` | Failed migration left dirty state. Fix version in `schema_migrations` table |
| Chinese set number mapping unknown | New CBB volume not in `mapChineseNumber`. Add volume mapping; falls back to number-less search |

---

## Resilience Patterns

- **Retry**: Exponential backoff with jitter (`platform/resilience/retry.go`), used by `httpx.Client`
- **Circuit breaker**: Per-provider via `sony/gobreaker` in `httpx/`. States: closed → open (after N failures) → half-open
- **Rate limits**: DH enterprise (managed by provider), auth 10 req/sec
- **429 handling**: `APITracker.UpdateRateLimit` blocks provider-level requests until expiry

---

## Domain Simplifications

- **Cost calculations**: Use `CalculateSaleFee()` and `CalculateNetProfit()` functions (no manager pattern)
- **Grade extraction**: `ExtractGrade(title)` parses PSA grade from card title strings
- **Object allocation**: Standard `new()` / `make()` — no sync.Pool
- **Optional deps**: Functional options (`WithPriceLookup`) instead of required constructor params
