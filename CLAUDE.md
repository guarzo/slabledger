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

# Screenshots (all pages, real backend + local Postgres)
make screenshots
# Output: web/screenshots/*.png (desktop) + web/screenshots/mobile/*.png (mobile)

# Quality
make check                                 # Full quality check (see "Quality Checks" below)
```

## Architecture

**Hexagonal (Clean Architecture)** - domain defines interfaces, adapters implement them.

```
internal/
  domain/     # Pure business logic (NO external deps) — defines the interfaces
  adapters/   # Interface implementations (HTTP, clients, storage, schedulers)
  platform/   # Cross-cutting concerns (cache, config, crypto, resilience, telemetry)
```

**Package rosters are derived, not listed here.** Any snapshot of the package tree
goes stale silently and then misleads every session that loads this file. Get the
current set from the tree:

```bash
go list ./internal/domain/...            # domain packages
ls internal/adapters internal/adapters/clients internal/platform
```

Orientation for the packages whose purpose is not obvious from the name:

- `domain/inventory` — the hub: campaigns, purchases, sales. See "Inventory Domain" below.
- `domain/pricing` — `PriceProvider` interface, graded prices, market data.
  `domain/pricing/lookup` is the `PriceLookup` adapter over it.
- `domain/intelligence` — market intelligence repository and types (DH Tier 3 data).
- `domain/liquidation` — liquidation candidates and comp pricing.
- `domain/dhevents` — DH event stream types.
- `domain/storage` — cache and storage interfaces.
- `domain/{constants,errors,llmutil,mathutil,observability,timeutil}` — leaf utilities.
- `adapters/clients/dh` — the DH API client; `adapters/clients/dhprice` wraps it as a
  `PriceProvider`; `adapters/clients/dhlisting` handles listing pushes.
- `adapters/clients/cardladder` — CardLadder valuations (separate from `PriceProvider`).
- `adapters/clients/psa`, `adapters/clients/psaportal` — PSA APIs and the portal session.
- `adapters/clients/httpx` — unified HTTP client (retry + circuit breaker), used by the rest.
- `adapters/storage/postgres` — persistence + embedded migrations.
- `adapters/scheduler` — background jobs (price refresh, DH polling/push, CardLadder
  refresh, PSA sync, session cleanup, snapshots).

**Key Principle**: Domain code depends ONLY on interfaces, never concrete implementations.

## Inventory Domain

The inventory domain (`internal/domain/inventory/`) is the core campaigns and inventory tracking feature.

### Core inventory package
- **Types**: Campaign, Purchase, Sale, Phase, SaleChannel (`core_types.go`)
- **8 repository interfaces**: CampaignRepository, PurchaseRepository, SaleRepository,
  AnalyticsRepository, FinanceRepository, PricingRepository, DHRepository,
  PendingItemRepository. They are split by concern, not by size — PurchaseRepository
  alone carries more methods than the other seven combined, so do not assume a new
  purchase-shaped method belongs elsewhere just to keep it small.
- **Service**: CRUD + imports + analytics; delegates computation to sibling sub-packages
- **PriceLookup**: Optional interface for market signal computation (injected via `WithPriceLookup` functional option)
- **Import**: CSV parsing lives directly in the `inventory` package
  (`ls internal/domain/inventory/parse_*.go` for the current set — eBay, Shopify, orders)
- **Channel fees**: eBay/TCGPlayer use campaign's `ebayFeePct`; local/other = 0%

### Sibling sub-packages (flat siblings under `internal/domain/`, no cross-imports between them)

Membership and enforcement are separate, and the split matters:

- **Membership is derived, not listed**: a sibling is any directory under
  `internal/domain/` with a non-test `.go` file importing `internal/domain/inventory`.
  Adding a new inventory-importing package puts it under the rule automatically —
  no list to update here or in the script.
- **Enforcement is target-based**: `scripts/check-imports.sh` flags *any* importer of
  a governed sibling, anywhere under `internal/domain/`, whether or not that importer
  imports the hub itself. A package cannot leave enforcement by dropping its own hub
  import (SLA-45).

The checker computes both from the tree on every `make check`, and
`scripts/check-imports-test.sh` tests the checker.

Siblings may depend on `inventory` (the hub) and on leaf packages such as `errors`
and `observability`, but never on each other. "Leaf" is defined precisely — a package
whose transitive closure under `internal/domain/` excludes the hub — in
[internal/README.md](internal/README.md#leaf-and-non-leaf-packages) (SLA-48). That section
also explains why the existing target-side check already enforces the leaf rule, and why
making membership transitive would be a no-op. To see the current set:

```bash
grep -rl --include='*.go' 'internal/domain/inventory' internal/domain \
  | grep -v _test.go | xargs -n1 dirname | sort -u
```

The roster is deliberately not written out here. It has now drifted twice (SLA-46,
SLA-48) because a dated list goes stale silently on the next package add or delete,
while the command above cannot.

## Database

Postgres via `jackc/pgx/v5/stdlib` (Supabase in prod, local Postgres in the devcontainer).
All monetary values in **cents**. Migrations managed by `golang-migrate/migrate/v4` and
embedded in the binary via `embed.FS`. Migrations run automatically on startup.

Migration files: `internal/adapters/storage/postgres/migrations/`. `000001_initial_schema`
represents the final-state schema after cutover from SQLite; every later migration is
incremental. **Do not trust a prose list of migrations here** — run
`ls internal/adapters/storage/postgres/migrations/` for the current set and read the file
itself for what it does. The narratives below are only the ones carrying semantics you
would not infer from the SQL:

- `campaign_targeting_axes` (000024) — replaced the inclusion/exclusion model with four
  `campaigns` columns: a multi-valued language axis (`target_languages` JSONB, **empty =
  open net**), subject-mode, subject-list, and denied-spec. Its legacy subject backfill
  marks unreconciled rows with a **negative sentinel id**.
- The RLS retrofits (000027, 000028, 000029) — 000003's blanket pass wrote `USING (true)`
  policies with no `TO` clause, which defaults to `TO PUBLIC` and therefore admits
  `anon`/`authenticated`. These three migrations bring every table under
  `TO service_role` + REVOKE. Role-dependent statements are guarded on `pg_roles` so they
  also apply to a local Postgres, where Supabase's roles do not exist. This denies a
  PostgREST writer, **not** a holder of `service_role` or direct database access — the
  claim/`MarkResult` guard in code and SLA-44 cover that.

Connection is configured via `DATABASE_URL`. The transaction pooler is used for the app
runtime; DDL works the same because `db.go` uses `pgx.QueryExecModeExec` (simple protocol).

See [internal/README.md](internal/README.md) for step-by-step migration creation.

## Environment Variables

See `.env.example` for the complete list with descriptions. Key groups:

- **Required**: none (all features optional or DH-keyed)
- **DH**: `DH_API_BASE_URL`, `DH_ENTERPRISE_API_KEY`
- **Auth**: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ENCRYPTION_KEY`
- **CardLadder**: `CARDLADDER_REFRESH_ENABLED`, `CARDLADDER_REFRESH_HOUR`
- **Schedulers**: `PRICE_REFRESH_ENABLED`

## Pricing Pipeline

DH (DoubleHolo) is the sole implementation of the `pricing.PriceProvider` interface, via
`DHPriceProvider` (`internal/adapters/clients/dhprice/`). It is not the only valuation
source in the system: **CardLadder** is wired separately in
`cmd/slabledger/init_services.go` (`initializeCardLadder`) and feeds
`internal/domain/liquidation` comp pricing plus its own scheduler jobs. When a change
touches "prices," check which of the two it means.

Prices are computed in-memory from DH API calls — there is no `price_history` table
(dropped before the Postgres cutover). The price refresh scheduler warms the DH card ID
cache by iterating unsold inventory from `campaign_purchases`. The `DBTracker` struct
(`internal/adapters/storage/postgres/prices.go`) provides API tracking, access tracking,
and health checks. Previous pricing sources (PriceCharting, CardHedger, JustTCG, fusion
engine) were removed on 2026-04-06.

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

- `make check` — runs lint + architecture import check + file size check + Playwright version check
- `scripts/check-imports.sh` — fails if domain packages import adapter packages (hexagonal invariant); also enforces the flat sibling rule, deriving membership from the tree and enforcing on the target side, and fails closed if fewer than two siblings are derived or if the scan performs an unexpected number of checks
- `scripts/check-imports-test.sh` — self-test for the above; five fixture cases, run first by `make check`
- `scripts/check-file-size.sh` — warns at 500 lines, fails at 600 lines (excludes test files and mocks)
- `scripts/check-playwright-version.sh` — keeps the Playwright package and browser image in step

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

- **Dev proxy**: Vite proxies `/api/*` and `/auth/*` → `http://localhost:8081`
  (see `web/vite.config.js`). The proxy is disabled when `PLAYWRIGHT_TEST` is set, so
  Playwright specs can mock routes.
- **Type sync**: Frontend types in `web/src/types/` are manually maintained to match Go struct JSON tags. When modifying Go response structs, update corresponding TS interfaces.
- **API client**: `web/src/js/api/client.ts` — singleton with retry, 30s timeout (5min for uploads), credential inclusion

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
- [Schedulers](docs/SCHEDULERS.md) - Every background job, its cadence and its env gate
- [Operations](docs/OPERATIONS.md) - Deploy, monitoring, incident handling; see also `docs/runbooks/`
- [Loop](docs/LOOP.md) - The acquisition/liquidation loop this system exists to run
- [DH Inventory](docs/DH_INVENTORY.md) - DH listing and inventory sync behavior

`ls docs/` for the rest — the list above is the durable set, not an inventory.

## Key Reference Files

- `internal/README.md` — Architecture rules, decision tree for code placement, anti-patterns, recipes
- `internal/testutil/mocks/README.md` — Mock patterns with examples
- `docs/API.md` — All endpoint request/response shapes
- `docs/SCHEMA.md` — Full database schema with indexes
- `.env.example` — All environment variables with comments

## Worktrees

Use `.worktrees/` in the project root for git worktrees.
