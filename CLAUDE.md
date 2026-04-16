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

# Screenshots (all pages, mocked API — no backend needed)
cd web && npx playwright test tests/screenshot-all-pages.spec.ts --project=chromium
# Output: web/screenshots/*.png (desktop) + web/screenshots/mobile/*.png (iPhone 14)

# Quality
make check                                 # Full quality check (lint + architecture + file size)
```

## Architecture

**Hexagonal (Clean Architecture)** - domain defines interfaces, adapters implement them.

```
internal/
  domain/           # Pure business logic (NO external deps)
    inventory/      # Core inventory: campaigns, purchases, sales (8 focused repo interfaces)
    arbitrage/      # Crack candidates, acquisition targets, EV, Monte Carlo projection
    portfolio/      # Inventory aging, price signals, portfolio health analysis
    tuning/         # Campaign parameter optimization, tuning suggestions and analytics
    finance/        # Invoices, cashflow, capital tracking, revocation flags
    export/         # Sell sheet generation
    dhlisting/      # DH listing push pipeline coordination
    advisor/        # AI advisor interfaces, tool loop, tracking
    ai/             # LLM provider, image generation, tool executor interfaces
    auth/           # Authentication interfaces
    cards/          # CardRepository interface
    constants/      # Shared constants
    errors/         # Error types
    favorites/      # Favorites management
    intelligence/   # Market intelligence repository and types (DH Tier 3 data)
    llmutil/        # LLM response utilities (strip fences, etc.)
    mathutil/       # Math utility functions
    observability/  # Logger, MetricsRecorder interfaces
    pricing/        # PriceProvider interface, graded prices, market data
    scoring/        # Price scoring factors and profiles
    storage/        # Cache and storage interfaces
    timeutil/       # Time utility functions
  adapters/         # Interface implementations
    httpserver/     # HTTP handlers, middleware, router
    clients/        # External API clients
      dhprice/      # DH (DoubleHolo) price provider — sole price source
      pricelookup/  # PriceLookup adapter (wraps PriceProvider for campaigns)
      tcgdex/       # TCGdex.dev card/set metadata (EN + JA, no API key)
      google/       # Google OAuth
      httpx/        # Unified HTTP client (retry + circuit breaker)
      azureai/      # Azure AI completions
    storage/sqlite/ # SQLite persistence + migrations
    scheduler/      # Background jobs (price refresh, session cleanup, advisor, snapshots)
  platform/         # Cross-cutting concerns
    cache/          # Type-safe cache
    cardutil/       # Card name/set normalization (pure utility, no external deps)
    config/         # Configuration
    crypto/         # AES encryption for auth tokens
    errors/         # Error types
    resilience/     # Retry + circuit breaker
    telemetry/      # slog logging
```

**Key Principle**: Domain code depends ONLY on interfaces, never concrete implementations.

## Inventory Domain

The inventory domain (`internal/domain/inventory/`) is the core campaigns and inventory tracking feature.

### Core inventory package
- **Types**: Campaign, Purchase, Sale, Phase, SaleChannel
- **8 focused repository interfaces**: CampaignRepository, PurchaseRepository, SaleRepository, AnalyticsRepository, FinanceRepository, PricingRepository, DHRepository, SnapshotRepository
- **Service**: CRUD + imports + analytics; delegates computation to sibling sub-packages
- **PriceLookup**: Optional interface for market signal computation (injected via `WithPriceLookup` functional option)
- **Import**: CSV parsing lives directly in the `inventory` package (parse_cl.go, parse_psa.go, parse_mm.go, parse_shopify.go, parse_orders.go)
- **Channel fees**: eBay/TCGPlayer use campaign's `ebayFeePct`; local/other = 0%

### Sibling sub-packages (flat siblings under `internal/domain/`, no cross-imports between them)
- **arbitrage**: Crack detection, acquisition targets, expected value, Monte Carlo projection
- **portfolio**: Inventory aging, price signals, portfolio health analysis
- **tuning**: Campaign parameter optimization, tuning suggestions and analytics
- **finance**: Invoices, cashflow forecasting, capital tracking, revocation flags
- **export**: Sell sheet generation
- **dhlisting**: DH listing push pipeline coordination

## Database

SQLite with WAL mode. All monetary values in **cents**. Migrations managed by `golang-migrate/migrate/v4`
and embedded in the binary via `embed.FS`. Migrations run automatically on startup. 69 migration pairs (`000001`–`000069`).

Migration files: `internal/adapters/storage/sqlite/migrations/` (69 migration pairs)

See [internal/README.md](internal/README.md) for step-by-step migration creation.

## Environment Variables

See `.env.example` for the complete list with descriptions. Key groups:

- **Required**: none (all features optional or DH-keyed)
- **DH**: `DH_API_BASE_URL`, `DH_ENTERPRISE_API_KEY`
- **AI**: `AZURE_AI_ENDPOINT`, `AZURE_AI_API_KEY`, `AZURE_AI_DEPLOYMENT`
- **Auth**: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ENCRYPTION_KEY`
- **Schedulers**: `PRICE_REFRESH_ENABLED`, `ADVISOR_REFRESH_HOUR`

## Pricing Pipeline

DH (DoubleHolo) is the sole price source via `DHPriceProvider` (`internal/adapters/clients/dhprice/`). Prices are computed in-memory from DH API calls — there is no `price_history` table (dropped in migration 000038). The price refresh scheduler warms the DH card ID cache by iterating unsold inventory from `campaign_purchases`. The `DBTracker` struct (`internal/adapters/storage/sqlite/prices.go`) provides API tracking, access tracking, and health checks. Previous pricing sources (PriceCharting, CardHedger, JustTCG, fusion engine) were removed on 2026-04-06.

## Testing

- **Pattern**: Table-driven tests with `[]struct` for all test cases
- **Mocks**: Import from `internal/testutil/mocks/` — never create inline mocks
  - Uses Fn-field pattern: override any method by setting `mock.CreateCampaignFn = func(...) { ... }`
  - Separate focused mocks for each interface: `CampaignRepositoryMock`, `PurchaseRepositoryMock`, etc.
  - Service mocks for each sub-package: `MockArbitrageService`, `MockPortfolioService`, etc.
  - In-memory store for service-layer tests: `mocks.NewInMemoryCampaignStore()`
  - Full guide: `internal/testutil/mocks/README.md`
- **Error assertions**: Use `errors.Is(err, inventory.ErrCampaignNotFound)` with sentinel errors
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

## Quality Checks

- `make check` — runs lint + architecture import check + file size check
- `scripts/check-imports.sh` — fails if domain packages import adapter packages (hexagonal invariant); also enforces flat sibling rule between inventory sub-packages
- `scripts/check-file-size.sh` — warns at 500 lines, fails at 600 lines (excludes test files and mocks)

## Adding New Components

See [internal/README.md](internal/README.md) for detailed step-by-step examples:
- Adding a new data source / API client
- Adding a new HTTP handler / endpoint
- Adding a new scheduler
- Adding a new domain interface
- Adding a new domain error
- Adding a new migration

Simplest API client reference: `internal/adapters/clients/dhprice/`

## Frontend-Backend Integration

- **Dev proxy**: Vite proxies `/api/*` → `http://localhost:8081` (see `web/vite.config.ts`)
- **Type sync**: Frontend types in `web/src/types/` are manually maintained to match Go struct JSON tags. When modifying Go response structs, update corresponding TS interfaces.
- **API client**: `web/src/js/api.ts` — singleton with retry, 30s timeout (5min for uploads), credential inclusion

## Configuration

- **Precedence**: CLI flags > env vars > `.env` > defaults
- **Loaded in**: `internal/platform/config/loader.go` — `Default()` → `FromEnv()` → `FromFlags()` → `Validate()`
- **Validated in**: `internal/platform/config/validation.go`
- **All variables**: see `.env.example` for the complete list with comments

## API Routes & Middleware

See [docs/API.md](docs/API.md) for all endpoints with request/response shapes.

**Middleware stack:** CORS → Gzip → Logging → Timing → Security Headers → Recovery → Rate Limiter

## Documentation

- [Internal Package Guide](internal/README.md) - Dependency rules, anti-patterns, recipes for adding code
- [User Guide](docs/USER_GUIDE.md) - End-user documentation
- [Architecture](docs/ARCHITECTURE.md) - System design, key decisions, domain interfaces
- [Development](docs/DEVELOPMENT.md) - Caching, rate limiting, resilience, troubleshooting
- [Database Schema](docs/SCHEMA.md) - Table definitions, indexes, relationships
- [API Reference](docs/API.md) - All endpoints with request/response shapes
- [Campaign Strategy](docs/private/CAMPAIGN_STRATEGY.md) - Business strategy (private, not tracked in git)

## Key Reference Files

- `internal/README.md` — Architecture rules, decision tree for code placement, anti-patterns, recipes
- `internal/testutil/mocks/README.md` — Mock patterns with examples
- `docs/API.md` — All endpoint request/response shapes
- `docs/SCHEMA.md` — Full database schema with indexes
- `.env.example` — All environment variables with comments

## Worktrees

Use `.worktrees/` in the project root for git worktrees.
