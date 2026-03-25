# CLAUDE.md

Developer reference for Claude Code. Go 1.26, hexagonal architecture.

## Quick Commands

```bash
# Build and run
go build -o slabledger ./cmd/slabledger
./slabledger                              # Start web server on :8081

# Testing
go test ./...                              # Run all tests
go test -race -timeout 10m ./...           # With race detection (CI)
make test                                  # Via Makefile

# Frontend
cd web && npm install && npm run dev       # Dev server on :5173
npm run build                              # Production build
npm test                                   # Run tests
```

## Architecture

**Hexagonal (Clean Architecture)** - domain defines interfaces, adapters implement them.

```
internal/
  domain/           # Pure business logic (NO external deps)
    advisor/        # AI advisor interfaces and types
    auth/           # Authentication interfaces
    campaigns/      # Campaign tracking, purchases, sales, P&L, analytics, CSV import
    cards/          # CardRepository interface
    constants/      # Shared constants
    errors/         # Error types
    favorites/      # Favorites management
    fusion/         # Price fusion interfaces
    mathutil/       # Math utility functions
    observability/  # Logger, MetricsRecorder interfaces
    pricing/        # PriceProvider interface, graded prices, market data
    social/         # Social content generation domain
    storage/        # Storage interfaces
  adapters/         # Interface implementations
    httpserver/     # HTTP handlers, middleware, router
    clients/        # External API clients
      fusionprice/  # Multi-source price fusion (PokemonPrice + CardHedger + PriceCharting)
      pricelookup/  # PriceLookup adapter (wraps PriceProvider for campaigns)
      tcgdex/       # TCGdex.dev card/set metadata (EN + JA, no API key)
      pricecharting/ # PriceCharting graded prices + market data
      pokemonprice/ # PokemonPrice primary graded price source
      google/       # Google OAuth
      httpx/        # Unified HTTP client (retry + circuit breaker)
      cardutil/     # Card utility functions
      cardhedger/ # CardHedger supplementary pricing (unlimited plan)
      instagram/  # Instagram OAuth + carousel publishing
      azureai/    # Azure AI completions
    storage/sqlite/ # SQLite persistence + migrations
    scheduler/      # Background jobs (price refresh, session cleanup, social content, advisor, snapshots)
  platform/         # Cross-cutting concerns
    cache/          # Type-safe cache
    config/         # Configuration
    crypto/         # AES encryption for auth tokens
    errors/         # Error types
    resilience/     # Retry + circuit breaker
    telemetry/      # slog logging
```

**Key Principle**: Domain code depends ONLY on interfaces, never concrete implementations.

## Campaigns Domain

The campaigns package (`internal/domain/campaigns/`) is the core business feature:

- **Types**: Campaign, Purchase, Sale, Phase, SaleChannel
- **Service**: CRUD + ArchiveCampaign, ImportPurchases, analytics (PNL, channel breakdown, fill rate, days-to-sell, inventory aging with market signals)
- **PriceLookup**: Optional interface for market signal computation (injected via `WithPriceLookup` functional option)
- **Import**: CSV import with `ExtractGrade` for PSA grade extraction from card titles
- **Channel fees**: eBay/TCGPlayer use campaign's `ebayFeePct`; local/other = 0%

## Database

SQLite with WAL mode. Migrations managed by `golang-migrate/migrate/v4`
and embedded in the binary via `embed.FS`. Migrations run automatically
on startup — no manual step required.

Migration files: `internal/adapters/storage/sqlite/migrations/` (17 pairs, 000001–000017)

**Creating a new migration:**
```bash
# Check highest migration number:
ls internal/adapters/storage/sqlite/migrations/ | sort -n | tail -2
# Create pair (zero-pad to 6 digits):
touch internal/adapters/storage/sqlite/migrations/000018_description.up.sql
touch internal/adapters/storage/sqlite/migrations/000018_description.down.sql
```

**Rollback (manual):** Stop the app, apply `.down.sql` with `sqlite3`, update `schema_migrations` table version, restart.

## Environment Variables

```bash
# Required
PRICECHARTING_TOKEN="..."    # Graded prices + sales data

# Optional
LOG_LEVEL="info"             # debug, info, warn, error
ADMIN_EMAILS="a@b.com,c@d.com" # Comma-separated admin email addresses
CARD_HEDGER_API_KEY="..."    # Supplementary pricing source
CARD_HEDGER_CLIENT_ID="..."  # Card request submission token
CARD_HEDGER_POLL_INTERVAL    # Delta poll interval (default: 1h)
CARD_HEDGER_BATCH_INTERVAL   # Batch interval (default: 24h)
CARD_HEDGER_MAX_CARDS_PER_RUN # Max cards per batch (default: 200)
LOCAL_API_TOKEN="..."        # Bearer token for CLI/curl access without browser OAuth
PRICE_REFRESH_ENABLED="true" # Enable/disable price refresh scheduler
SESSION_CLEANUP_ENABLED="true" # Enable/disable session cleanup scheduler
SNAPSHOT_ENRICH_RETRY_INTERVAL # Retry interval for failed snapshots (default: 30m)
SNAPSHOT_ENRICH_MAX_RETRIES    # Max retries before marking exhausted (default: 5)
INSTAGRAM_APP_ID="..."        # Instagram OAuth app ID
INSTAGRAM_APP_SECRET="..."    # Instagram OAuth app secret
INSTAGRAM_REDIRECT_URI="..."  # Instagram OAuth redirect URI
SOCIAL_CONTENT_ENABLED="true" # Enable/disable social content scheduler
SOCIAL_CONTENT_INTERVAL="24h" # Social content detection interval
AZURE_AI_ENDPOINT="..."       # Azure OpenAI endpoint URL
AZURE_AI_API_KEY="..."        # Azure OpenAI API key
AZURE_AI_DEPLOYMENT="..."     # Azure OpenAI deployment name
```

## Pricing Pipeline

Card names flow through a multi-stage normalization and matching pipeline:

**PriceCharting lookup** (6 strategies): hint → cache → UPC → API → fuzzy → consoleMismatchFallback.
- `normalizeSetName()` in `pc_query_helpers.go` expands era codes (SWSH→Sword Shield), strips PSA codes, handles Chinese sets
- `normalizeCardName()` in `pc_query_helpers.go` expands abbreviations (SP.DELIVERY, REV.FOIL), strips PSA boilerplate
- `VerifyProductMatch` in `pc_verify.go` validates card numbers with `NumericOnly` fallback for prefix mismatches (PSA "075" vs PC "SWSH075")
- Promo sets: PriceCharting uses "Pokemon Promo" for ALL eras — `tryAPI` detects when both sides are promo and bypasses set-token comparison
- Chinese sets: `isChineseSet()` + `mapChineseNumber()` translate PSA printed numbers to PC species-based numbers (CBB1=700+n, CBB2=600+n). Unknown volumes fall back to number-less search.

**CardHedger `details-by-certs`** (`cert_resolver.go`): Batch cert→card_id resolution. API returns nested objects (`cert_info` + `card`), not flat fields. `Card` is null when cert's card isn't in CardHedger's DB. Types: `CertDetailResult` → `CertInfo` + `*CardDetail`.

**CardHedger lookup** (`resolveCardID` in `source_adapters.go`): 3-tier query fallback:
1. Full normalized query: `NormalizeSetNameForSearch(set) + SimplifyForSearch(NormalizePurchaseName(name)) + number`
2. Minimal query: `truncateAtVariant(name) + eraPrefix + number` (strips variant noise like "Holo CRZ")
3. Raw PSA title (stripped of grade suffix) — lets CardHedger's LLM parse natural language directly

**Shared normalization** (`cardutil/normalize.go`):
- `NormalizePurchaseName`: expands PSA abbreviations (-HOLO, -REV.FOIL, SP.DELIVERY)
- `SimplifyForSearch`: deduplicates prefix, truncates after type suffix (ex, GX), strips trailing noise words
- `NormalizeSetNameForSearch`: strips PSA codes (PRE EN-, M24 EN-), handles Chinese/Japanese prefixes
- `MissingSetTokens`: compares set token overlap; callers normalize expected set first rather than maintaining exclusion lists

**Integration tests** (`internal/integration/`): 34 real inventory items with actual PSA cert numbers, run against live APIs with `-tags integration`.

## Testing

- **Unit tests**: Mock all external deps, use `internal/testutil/mocks`
- **Integration tests**: `internal/integration/` with `-tags integration` flag, requires API keys in `.env`
- Always run `go test -race` before committing

## Code Style

- Use structured logging: `logger.Info("msg", observability.String("key", val))`
- Backend uses cents internally, API responses use USD (dollars)
- Context propagation: always pass `ctx` as first parameter
- Avoid over-engineering: only make changes directly requested
- Cost/prior calculations use simple functions, not manager structs
- Use builtin min/max (Go 1.21+), not custom implementations
- Functional options pattern for optional dependencies (e.g. `WithPriceLookup`)

## Resilience Patterns

- **Retry**: Exponential backoff with jitter (`platform/resilience/retry.go`), used by `httpx.Client`
- **Circuit breaker**: Per-provider via `sony/gobreaker` in `httpx/`. States: closed → open (after N failures) → half-open
- **Rate limits**: PriceCharting 1 req/sec, CardHedger 100 req/min + 700ms pause, pricing API 60 req/min, auth 10 req/sec
- **429 handling**: `APITracker.UpdateRateLimit` blocks provider-level requests until expiry

## Adding a New API Client

1. Define domain interface in `internal/domain/<package>/`
2. Create adapter at `internal/adapters/clients/<name>/` using `httpx.Client` (gets retry + circuit breaker)
3. Wire in `cmd/slabledger/main.go` via functional option (`WithMyProvider(client)`)
4. Update this file with the new package and env vars

Simplest reference: `internal/adapters/clients/pokemonprice/`

## Frontend-Backend Integration

- **Dev proxy**: Vite proxies `/api/*` → `http://localhost:8081` (see `web/vite.config.ts`)
- **Type sync**: Frontend types in `web/src/types/` are manually maintained to match Go struct JSON tags. When modifying Go response structs, update corresponding TS interfaces.
- **API client**: `web/src/js/api.ts` — singleton with retry, 30s timeout (5min for uploads), credential inclusion

## Troubleshooting

| Error | Likely Cause | Fix |
|-------|-------------|-----|
| `database is locked` | WAL mode issue or concurrent write contention | Check `PRAGMA journal_mode=wal;` runs on startup |
| `429 rate limited` on PriceCharting | Exceeded 1 req/sec | Wait for block expiry; check `rate_limiter.go` |
| `mock does not implement interface` | Repository interface changed | Add missing method to both mocks (`testutil/mocks/` and `domain/campaigns/mock_repo_test.go`) |
| Frontend proxy 502 | Backend not running on :8081 | Start backend: `go run ./cmd/slabledger` |
| `migration: dirty database` | Failed migration left dirty state | Fix version in `schema_migrations` table |
| CardHedger `Card is null` | Cert's card not in CardHedger DB | Expected for new/rare cards; null handling in `cert_resolver.go` |
| Chinese set number mapping unknown | New CBB volume not in `mapChineseNumber` | Add volume mapping; falls back to number-less search |

## Campaign Analysis Command

`/campaign-analysis` — conversational analysis of campaign performance. Requires the web server running on `:8081`. Fetches live data from the API and cross-references against `docs/private/CAMPAIGN_STRATEGY.md`.

```
/campaign-analysis                  # Full portfolio overview
/campaign-analysis health           # Quick health check (traffic-light per campaign)
/campaign-analysis weekly           # Monday review cadence
/campaign-analysis tuning           # Parameter adjustment discussion
/campaign-analysis campaign 3       # Deep dive on a specific campaign by ID
```

Command definition: `.claude/commands/campaign-analysis.md`

## Documentation

- [Internal Package Guide](internal/README.md) - Dependency rules, anti-patterns, step-by-step examples for adding code
- [User Guide](docs/USER_GUIDE.md) - End-user documentation
- [Architecture](docs/ARCHITECTURE.md) - System design and key decisions
- [Development](docs/DEVELOPMENT.md) - Caching, rate limiting, API integrations
- [Roadmap](docs/ROADMAP.md) - Development roadmap (EV calculator, Monte Carlo, crack arbitrage, capital visibility)
- [Campaign Analysis Plan](docs/CAMPAIGN_ANALYSIS_SKILL_PLAN.md) - Design rationale for the /campaign-analysis command
- [Campaign Strategy](docs/private/CAMPAIGN_STRATEGY.md) - Business strategy (private, not tracked in git)
