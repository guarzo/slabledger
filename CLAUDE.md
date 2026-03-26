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

Migration files: `internal/adapters/storage/sqlite/migrations/` (19 pairs, 000001–000019)

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
ADVISOR_REFRESH_HOUR="4"     # Hour (0-23 UTC) to run advisor; -1 = use InitialDelay
SOCIAL_CONTENT_HOUR="5"      # Hour (0-23 UTC) to run social content; -1 = use InitialDelay
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

- **Pattern**: Table-driven tests with `[]struct` for all test cases
- **Mocks**: Import from `internal/testutil/mocks/` — never create inline mocks
  - Uses Fn-field pattern: override any method by setting `mock.CreateCampaignFn = func(...) { ... }`
  - Full guide: `internal/testutil/mocks/README.md`
- **Error assertions**: Use `errors.Is(err, campaigns.ErrCampaignNotFound)` with sentinel errors
- **Deterministic data**: Use fixed seeds for Monte Carlo, atomic counters for IDs
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
- Keep source files under 500 lines. If a file grows beyond this, look for natural split points (separate strategies, separate concerns, utilities)

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

## API Routes

See [docs/API.md](docs/API.md) for detailed request/response shapes.

| Group | Routes | Auth | Prefix |
|-------|--------|------|--------|
| Authentication | 4 | AuthRateLimit | `/auth/`, `/api/auth/` |
| Health & Admin | 10 | None/Admin | `/api/health`, `/api/admin/` |
| Favorites | 5 | Auth | `/api/favorites/` |
| Cards & Pricing | 3 | Auth/Admin | `/api/cards/`, `/api/price-hints` |
| Campaign CRUD | 5 | Auth | `/api/campaigns/` |
| Campaign Analytics | 12 | Auth | `/api/campaigns/{id}/` |
| Global Purchases | 11 | Auth | `/api/purchases/` |
| Price Override & AI | 4 | Auth | `/api/purchases/{id}/` |
| Credit & Invoices | 5 | Auth | `/api/credit/` |
| Portfolio | 9 | Auth | `/api/portfolio/` |
| Utilities | 2 | Auth | `/api/certs/`, `/api/shopify/` |
| AI Advisor | 6 | Auth | `/api/advisor/` |
| Social & Instagram | 14 | Admin | `/api/social/`, `/api/instagram/` |
| Pricing API v1 | 3 | APIKey | `/api/v1/` |

**Middleware stack:** CORS → Gzip → Logging → Timing → Security Headers → Recovery → Rate Limiter

## Database Schema

See [docs/SCHEMA.md](docs/SCHEMA.md) for full column definitions and indexes.

SQLite WAL mode. All monetary values in **cents**. 19 migration pairs (`000001`–`000019`).

| Table | Purpose | Key FKs |
|-------|---------|---------|
| `users` | Google OAuth accounts | — |
| `oauth_states` | Short-lived CSRF tokens for OAuth flow | — |
| `api_rate_limits` | Per-provider 429-block and rate state | — |
| `api_calls` | Outbound pricing API call log | — |
| `ai_calls` | Azure OpenAI call log with token usage | — |
| `sync_state` | Key-value scheduler checkpoints | — |
| `cashflow_config` | Singleton global cashflow parameters | — |
| `allowed_emails` | Login access allowlist | `added_by → users` |
| `revocation_flags` | Access revocation notices | — |
| `card_id_mappings` | Cached provider external IDs | — |
| `price_history` | Time-series card prices per source | — |
| `price_refresh_queue` | Background price-refresh work queue | `source → api_rate_limits` |
| `card_access_log` | Recent access log for staleness prioritization | — |
| `discovery_failures` | Failed card discovery records | — |
| `card_request_submissions` | Cards submitted to CardHedger | — |
| `market_snapshot_history` | Daily archive of unsold inventory snapshots | — |
| `population_history` | PSA population counts over time | — |
| `cl_value_history` | Card Ladder valuations per cert | — |
| `advisor_cache` | Cached AI advisor analysis results | — |
| `instagram_config` | Singleton Instagram account credentials | — |
| `invoices` | PSA Partner Offers purchase invoices | — |
| `campaigns` | Acquisition campaign definitions | — |
| `user_sessions` | Active browser sessions | `user_id → users` |
| `user_tokens` | OAuth tokens scoped to sessions | `user_id → users`, `session_id → user_sessions` |
| `favorites` | User-saved favorite cards | `user_id → users` |
| `campaign_purchases` | Individual graded cards per campaign | `campaign_id → campaigns` |
| `campaign_sales` | Sale records for purchased cards | `purchase_id → campaign_purchases` |
| `social_posts` | Instagram carousel draft posts | — |
| `social_post_cards` | Junction: posts ↔ purchases (slides) | `post_id → social_posts` |

**8 views**: `stale_prices`, `api_usage_summary`, `api_hourly_distribution`, `api_daily_summary`, `active_sessions`, `expired_sessions`, `ai_usage_summary`, `ai_usage_by_operation`

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
| `fusion` | `SecondaryPriceSource` | `source.go` | 3 | Price fusion data (PokemonPrice, CardHedger) |
| `fusion` | `CardIDResolver` | `source.go` | 3 | External ID cache |
| `fusion` | `PriceHintResolver` | `source.go` | 4 | User-provided price hints |
| `favorites` | `Service` | `service.go` | 6 | Favorites CRUD |
| `favorites` | `Repository` | `repository.go` | ~6 | Favorites persistence |
| `observability` | `Logger` | `logger.go` | 5 | Structured logging |

---

## Common Recipes

### Add a new API endpoint

1. Create a handler method on an existing handler struct in `internal/adapters/httpserver/handlers/`, or create a new handler file.
2. Register the route in `internal/adapters/httpserver/router.go` under the appropriate group (use Go 1.22 method+path patterns: `"GET /api/foo/{id}"`).
3. Wire any new handler dependencies through `RouterConfig` in `router.go` and `NewRouter`.
4. Update `docs/API.md` with the request/response shape and update the route table in this file.

### Add a new scheduler

1. Create a new file in `internal/adapters/scheduler/` following the existing pattern (a struct with `Run(ctx)` and `BuildGroup`).
2. Define any domain interface the scheduler needs in the relevant `internal/domain/<package>/` file.
3. Add the scheduler to the `BuildGroup` call in `cmd/slabledger/main.go` alongside the other schedulers.
4. Add any new env vars to `internal/platform/config/loader.go` (env overlay) and `internal/platform/config/` (struct + defaults).
5. Document the scheduler and its env vars in this file and `docs/DEVELOPMENT.md`.

### Add a new domain interface

1. Define the interface in `internal/domain/<package>/` — no external imports allowed in domain code.
2. Create a concrete adapter in `internal/adapters/clients/<name>/` or `internal/adapters/storage/sqlite/` that implements it.
3. Wire the adapter in `cmd/slabledger/main.go` via a functional option (`With<Name>(impl)`).
4. Add a mock in `internal/testutil/mocks/` and update any composed mocks in `domain/<package>/mock_repo_test.go`.

### Add a new domain error

1. Add error code in `internal/domain/<package>/errors.go`: `ErrCodeMyError errors.ErrorCode = "ERR_MY_ERROR"`
2. Add sentinel: `var ErrMyError = errors.NewAppError(ErrCodeMyError, "description")`
3. Add predicate: `func IsMyError(err error) bool { return errors.HasErrorCode(err, ErrCodeMyError) }`
4. Test with `errors.Is(err, ErrMyError)` in callers

### Add a new migration

1. Check the highest existing migration number: `ls internal/adapters/storage/sqlite/migrations/ | sort -n | tail -2`
2. Create the pair (zero-pad to 6 digits):
   ```bash
   touch internal/adapters/storage/sqlite/migrations/000020_description.up.sql
   touch internal/adapters/storage/sqlite/migrations/000020_description.down.sql
   ```
3. Update the table count in `internal/adapters/storage/sqlite/migrations/` and add the new table to `docs/SCHEMA.md`.
4. Update the migration count in the "## Database" section of this file.

---

## Configuration

- **Precedence**: CLI flags > env vars > `.env` > defaults
- **Loaded in**: `internal/platform/config/loader.go` — `Default()` → `FromEnv()` → `FromFlags()` → `Validate()`
- **Validated in**: `internal/platform/config/validation.go`
- **All variables**: see `.env.example` for the complete list with comments

---

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
- [Database Schema](docs/SCHEMA.md) - Table definitions, indexes, relationships
- [API Reference](docs/API.md) - All endpoints with request/response shapes
- [Roadmap](docs/ROADMAP.md) - Development roadmap (EV calculator, Monte Carlo, crack arbitrage, capital visibility)
- [Campaign Analysis Plan](docs/CAMPAIGN_ANALYSIS_SKILL_PLAN.md) - Design rationale for the /campaign-analysis command
- [Campaign Strategy](docs/private/CAMPAIGN_STRATEGY.md) - Business strategy (private, not tracked in git)

## Key Reference Files

- `internal/README.md` — Architecture rules, decision tree for code placement, anti-patterns
- `internal/testutil/mocks/README.md` — Mock patterns with examples
- `docs/API.md` — All endpoint request/response shapes
- `docs/SCHEMA.md` — Full database schema with indexes
- `.env.example` — All environment variables with comments
