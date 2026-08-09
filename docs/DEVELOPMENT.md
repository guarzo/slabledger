# Development Guide

Operational reference for caching, rate limiting, API integrations, and common gotchas.

## Price Units (Critical)

**Money crosses the API as cents (integers). The frontend formats it for display.**

### Convention

| Layer | Format | Example |
|-------|--------|---------|
| Backend internal | Cents (int) | `355239` |
| Campaign API responses | Cents (int) | `355239` |
| Frontend display | Dollars | `$3,552.39` |

Campaign endpoints (`/api/campaigns/*`) return cents. The frontend converts for display.

Key file:
- `web/src/react/utils/formatters.ts` — `formatCents()` helper for campaign data

---

## Campaign Domain

### Key Files

| File | Purpose |
|------|---------|
| `internal/domain/inventory/core_types.go` | Campaign, Purchase, Sale structs |
| `internal/domain/inventory/service.go` | Business logic, PriceLookup interface |
| `internal/domain/inventory/channel_fees.go` | CalculateSaleFee, CalculateNetProfit |
| `internal/domain/inventory/repository_campaign.go` | Repository interfaces (one `repository_*.go` per concern) |
| `internal/adapters/storage/postgres/campaign_store.go` | Postgres implementation |
| `internal/adapters/httpserver/handlers/campaigns.go` | HTTP handlers |
| `internal/domain/pricing/lookup/adapter.go` | PriceLookup adapter |

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
adapter := lookup.NewAdapter(priceProvider)
svc := inventory.NewService(repos, inventory.WithPriceLookup(adapter))
```

### Database Migrations

Migrations are in `internal/adapters/storage/postgres/migrations/`. After the April 2026 cutover from SQLite the tree was reset to a single `000001_initial_schema` representing the final-state schema; future changes are additive from `000002` onward. See [docs/SCHEMA.md](SCHEMA.md) for the full schema and [internal/README.md](../internal/README.md) for step-by-step migration creation instructions.

### Postgres adapter tests

Tests in `internal/adapters/storage/postgres/` drop schemas and truncate tables.
They skip unless `POSTGRES_TEST_URL` is set — there is deliberately no default,
because the devcontainer's `DATABASE_URL` points at your development database.

Run them against a dedicated throwaway database:

    make test-postgres

This creates `slabledger_test` on first use and points the tests at it. CI sets
`POSTGRES_TEST_URL` explicitly (`.github/workflows/test.yml`).

### Integration tests (`-tags integration`)

Tests behind the `integration` build tag call live third-party APIs, so they are
excluded from `go test ./...`. Each one `t.Skip`s with a message naming what it
needs, which means a missing variable looks like a pass — check for `SKIP` in the
output rather than trusting a green run.

CI does not run them on push. The `integration` job in `.github/workflows/test.yml`
covers `./internal/integration/...` only on a weekly schedule (Mondays 06:00 UTC)
or via `workflow_dispatch`, and it supplies only `DH_ENTERPRISE_API_KEY` — so the
Card Ladder tests below skip there as well, and the PSA portal test is outside the
path it runs entirely. In practice a developer machine is the only place the
`CL_*` tests execute.

These variables are **not** in `.env.example` and deliberately so: they are test
credentials, not application config, and `.env.example` is the template
developers copy. This table is their only home — do not duplicate it there.
`POSTGRES_TEST_URL` above follows the same convention.

**Export these into your environment** — do not put them in the repo-root `.env`.
The tests use `godotenv/autoload`, which reads `.env` from the *package* directory,
and `go test` runs each binary with its cwd set there. So the root `.env` is never
consulted, and the skip message in `cardladder_test.go` that says "add to .env" is
misleading. Verified 2026-08-08: unsetting `CL_EMAIL` with the root `.env` in place
still skips; dropping a `.env` into `internal/integration/` makes it take effect.
`psaportal/live_test.go:25` reads the environment directly and has no `.env`
support at all.

Either export them in your shell (what the devcontainer does), or place a `.env`
in `internal/integration/`.

| Variable | Used by | Purpose |
|----------|---------|---------|
| `CL_EMAIL` | `internal/integration/cardladder_test.go` | Card Ladder login |
| `CL_PASSWORD` | same | Card Ladder login |
| `CL_FIREBASE_API_KEY` | same | Card Ladder login |
| `CL_TEST_CERT` | same | Cert number used as the lookup fixture |
| `CL_TEST_WRITE` | same | **Set to `true` to enable a test that writes to Firestore.** Off by default; the only destructive knob here |
| `CL_COLLECTION_ID` | same | Target collection for that write test |
| `PSA_PORTAL_TEST_TOKEN` | `internal/adapters/clients/psaportal/live_test.go` | Portal `accessToken` cookie value |

`internal/integration/dh_enterprise_test.go` also runs under this tag, but reads
`DH_API_BASE_URL` and `DH_ENTERPRISE_API_KEY` — production config already
documented in `.env.example`, so nothing extra is needed for it.

Run them explicitly:

    go test ./internal/integration/ -tags integration -v -run TestCardLadder -timeout 2m
    go test -tags integration ./internal/adapters/clients/psaportal/ -run TestLiveSnapshotChain -v

`TestLiveSnapshotChain` must run from an IP Cloudflare trusts — the devcontainer
qualifies, datacenter IPs do not.

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

Code: `internal/domain/pricing/lookup/adapter.go`

---

## Monitoring

```bash
# Health check (unauthenticated)
curl http://localhost:8081/api/health

# API usage status (per-provider call counts, success rates, latency).
# Admin-gated: send an authenticated admin session cookie.
curl --cookie "$SESSION_COOKIE" http://localhost:8081/api/admin/api-usage

# Request timing metrics (admin-gated)
curl --cookie "$SESSION_COOKIE" http://localhost:8081/api/admin/metrics
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

---

## Deploying to Fly

Production runs on a single Fly Machine in `iad`, backed by Supabase Postgres (`aws-1-us-east-2`).

- **Secrets**: managed via `fly secrets set`. Pull the list with `fly secrets list --app slabledger`. `DATABASE_URL` points at the Supabase transaction pooler (port 6543, `?sslmode=require`). `ENCRYPTION_KEY` must remain stable — rotating it makes existing OAuth tokens undecryptable.
- **Deploy**: auto-deploy on push to `main` is configured in Fly itself (GitHub App integration), not by a workflow in this repository — there is no `.github/workflows/` deploy pipeline to inspect. Manual deploy: `flyctl deploy --remote-only` from the repo root.
- **Logs**: `flyctl logs --app slabledger`. `flyctl status` for machine / health state.
- **Rollback**: `flyctl releases list` → `flyctl deploy --image <previous-image>` or redeploy an earlier commit.

## Database (Supabase)

- **Prod connection**: transaction pooler at `aws-1-us-east-2.pooler.supabase.com:6543`. `pgx.QueryExecModeExec` (simple protocol) keeps it PgBouncer-compatible.
- **Local dev**: Postgres 17 container in the devcontainer, host port `5434` / internal `postgres:5432`.
- **Backups**: automatic daily, managed by Supabase. Dashboard → Database → Backups.
- **Dump / restore**: `make db-pull` and `make db-push` wrap `pg_dump` / `pg_restore` in custom format. `db-push` requires typing `yes` (not `y`) and writes a timestamped remote backup file before overwriting prod.
