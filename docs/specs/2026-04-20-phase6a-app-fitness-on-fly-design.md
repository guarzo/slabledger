# Phase 6a — App Fitness on Fly / Supabase

## Context

Slabledger was migrated from wanderer + SQLite to Fly.io + Supabase Postgres on 2026-04-20 (see `docs/private/FLY_MIGRATION_PLAN.md`). Phase 5 (data cutover) and Phase 6 (CI/CD swap, delete sqlite adapter) are complete. Production is serving traffic at `slabledger.dpao.la` via a single Fly Machine in `iad` against Supabase in `aws-1-us-east-2`.

Post-cutover observations driving this phase:
- The inventory page "loads quite slow" in prod (user-visible symptom). Local SQLite was sub-millisecond per query; Supabase round-trips are ~15ms each, so N+1 patterns that were invisible before are now painful.
- Migration-on-boot has no retry: if Supabase is unreachable when the Fly machine starts, the app crashloops.
- Observability today is structured logs + Fly's edge HTTP histogram. No in-process metrics. No visibility into DB pool saturation.

This spec covers a tightly-scoped fitness pass. It is **6a**; follow-ups 6b (CI/CD gating) and 6c (local dev ergonomics) are separate documents.

## Goals

1. Make the inventory page fast enough that it stops feeling broken.
2. Survive transient Supabase unavailability at boot without crashlooping.
3. Get `sql.DBStats` and Go runtime metrics into the Fly-managed Grafana so we can see pool saturation and memory behavior.

## Non-goals

- Per-endpoint latency histograms in-app (Fly's edge histogram already provides this per route).
- Alerting / dashboards-as-code.
- Custom business metrics (campaigns_created, purchases_imported, etc.).
- Tuning the connection pool sizes — we add visibility first; changes are reactive if the metrics show a problem.
- Scheduler audit, memory sizing, Fly volume semantics, or any other fitness concern not named above.

## Design

### 1. Inventory slowness — diagnosis and fix

**Suspected root cause:** `enrichCompSummaries` in `internal/domain/inventory/comp_enrichment.go:12` loops over unique `(gemRateID, grade)` keys and calls `s.compProv.GetCompSummary(ctx, key.gemRateID, certNumber)` serially. For a campaign with ~250 unsold purchases there are often ~100 unique keys; at ~15ms Supabase round-trip each, that is ~1.5s of serial DB work per inventory page load. Two other potential hotspots to rule out during diagnosis: `s.applyOpenFlags` (called from both `GetInventoryAging` and `GetGlobalInventoryAging`) and `buildCrackCandidateSet` (only on the global path).

**Step 1 — measure.** Add a structured log line at the end of `GetInventoryAging` and `GetGlobalInventoryAging` that records elapsed time for each phase: `purchases_ms`, `open_flags_ms`, `comp_summaries_ms`, `crack_set_ms` (global only). Ship. Load the inventory page in prod once. Read `fly logs` to confirm where time actually goes. If the dominant phase is not comp summaries, pivot the fix.

**Step 2 — fix the dominant phase.** Assuming comp summaries is the hotspot (most likely), replace the per-key loop with a single batched repository call:

- Add `GetCompSummariesByKeys(ctx context.Context, keys []CompKey) (map[CompKey]*CompSummary, error)` to the `CompSummaryProvider` interface, where `CompKey = (GemRateID, CertNumber)`.
- Implement it in the adapter (`internal/adapters/storage/postgres/cl_sales_store.go` or wherever `GetCompSummary` lives) with a single SQL query using `WHERE (gem_rate_id, condition) IN (...)` or an `ANY($1::text[])` pattern, then group results in Go.
- Rewrite the loop in `enrichCompSummaries` to call the batch method once and index into the returned map.
- Keep the existing `GetCompSummary` method for callers that only need one (or remove if no one else uses it — verify with grep).

**Step 3 — verify.** After deploy, check `fly_app_http_response_time_seconds` for the `/api/campaigns/{id}/inventory` route in fly-metrics.net. The p95 should drop noticeably (target: from whatever-it-is to sub-500ms). Also re-read the phase log line for a sanity sanity check.

**Scope guard:** if diagnosis reveals the time is in a different phase (e.g., `applyOpenFlags` making 1-query-per-item calls), fix that instead. Do not fix all three speculatively. The phase has the same structure — find the N+1, replace with a batch call, verify.

**Not doing:**
- In-memory caching of comp summaries (YAGNI — batch query should be fast enough).
- Async / streaming inventory responses (YAGNI — a working batched query returns in <200ms).
- Rewriting the domain/service split around aging computation.

### 2. Migration-on-boot safety

**Current behavior:** `cmd/slabledger/main.go:160` opens the DB, then calls `postgres.RunMigrations` (`internal/adapters/storage/postgres/migrations.go:82`, which runs `m.Up()`). Any error from either path causes `main.run()` to return, which causes the binary to exit non-zero, which causes Fly to restart the machine immediately. If the cause is a transient Supabase issue (pooler restart, DNS blip, maintenance window), the app crashloops indefinitely.

**Design:** retry the connection, not the migrations.

- In `postgres.Open()` (`internal/adapters/storage/postgres/db.go:22`), wrap the existing `db.PingContext(ctx)` call in a retry loop with exponential backoff: 10 attempts, initial delay 1s, doubling each attempt, capped at 30s per attempt, total budget ~60s. Log each failed attempt at `WARN` level with attempt number and error. If all attempts fail, return the error as today.
- `RunMigrations` semantics are unchanged. Once Open succeeds, the DB is reachable, so any migration error from that point is a real problem (bad migration SQL, dirty state, schema conflict) and deserves to fail loudly.
- Confirm the Fly health check's `grace_period` in `fly.toml` is at least 60s so the machine is not marked unhealthy while retrying.

**Not doing:**
- Retrying `m.Up()` itself.
- Starting the app without a DB and reconnecting lazily.
- Any automatic dirty-state recovery (manual intervention remains the correct response).

### 3. Baseline observability

**Design:** emit Go runtime and `sql.DBStats` metrics on a separate port so Fly's metrics scraper can pick them up. No dashboards, no alerts — just get the numbers into fly-metrics.net.

**Changes:**
1. **`fly.toml`** — add a `[metrics]` block:
   ```toml
   [metrics]
     port = 9091
     path = "/metrics"
   ```
2. **Go dependencies** — add `github.com/prometheus/client_golang/prometheus` and `.../promhttp`.
3. **Metrics server wiring** in `cmd/slabledger/main.go`:
   - Register `collectors.NewGoCollector()` and `collectors.NewProcessCollector(...)`.
   - Register a custom collector that wraps `sql.DBStats` (`OpenConnections`, `InUse`, `Idle`, `WaitCount`, `WaitDuration`, `MaxIdleClosed`, `MaxLifetimeClosed`) as gauges/counters. Scrape the stats from `db.Stats()` on each `Collect()` call.
   - Start a second `http.Server` on `:9091` serving `promhttp.Handler()` at `/metrics`. Bind to `0.0.0.0` so Fly's scraper can reach it.
   - Run the metrics server in a goroutine from `main`, shut it down on the same `ctx.Done()` signal as the main server.
4. **No changes to the main `:8081` server.** Metrics port is deliberately separate so the rate limiter / auth middleware does not interfere with scrapes.

**Verification:** after deploy, open fly-metrics.net, find the Go runtime dashboard, confirm series populate. Run a quick load burst against `/api/campaigns/{id}/inventory` and spot-check `go_sql_stats_in_use` (or whatever we end up naming it) to validate the gauge moves.

**Not doing:**
- Custom per-route HTTP histograms (Fly's edge handles this).
- Business metrics.
- OpenTelemetry (Fly scrapes Prometheus natively).
- Alert rules.

## Files to modify / create

- `fly.toml` — add `[metrics]` block.
- `go.mod` / `go.sum` — add `prometheus/client_golang`.
- `cmd/slabledger/main.go` — start second HTTP server for metrics; register collectors.
- `internal/adapters/storage/postgres/db.go` — add retry loop in `Open()`.
- `internal/adapters/storage/postgres/metrics.go` *(new)* — `sql.DBStats` custom collector.
- `internal/domain/inventory/comp_enrichment.go` — replace per-key loop with batch call.
- `internal/domain/inventory/service_interfaces.go` — add `GetCompSummariesByKeys` to the comp provider interface (exact interface name to confirm during implementation).
- `internal/adapters/storage/postgres/cl_sales_store.go` *(or equivalent)* — implement the batch method.
- `internal/domain/inventory/service_analytics.go` — add phase-timing log lines to `GetInventoryAging` and `GetGlobalInventoryAging` (kept after the fix as ongoing observability).
- `internal/testutil/mocks/` — extend the comp provider mock with the new method.

## Verification

- `go build ./...`, `go test -race ./...`, `golangci-lint run ./...`, `make check` — all green.
- New unit test for the batch repository method (table-driven, against local Postgres).
- Manual smoke: load `/api/campaigns/{id}/inventory` in the deployed prod UI; confirm edge-latency p95 for that route drops in fly-metrics.net.
- Manual smoke: open fly-metrics.net, confirm `go_*` and `db_*` metrics are flowing.
- Migration retry: not easily testable in prod without causing a real incident; verified via unit test that injects a mock connector that fails N times then succeeds.

## Rollback

- Inventory fix: the batch repository method is additive; reverting means keeping the new interface method but restoring the loop in `enrichCompSummaries`. Low risk.
- Retry loop: reverting is a one-commit revert.
- Metrics endpoint: reverting is a one-commit revert; the `[metrics]` block in `fly.toml` becomes inert if the app does not expose the port.

## Out of scope — tracked for 6b / 6c

- **6b (CI/CD gating):** no deploy-on-push gate today. Fly dashboard auto-deploys whatever lands on `main` regardless of CI state. Preview envs per PR. Migration preview in CI against a throwaway Postgres. Rollback procedure doc.
- **6c (local dev):** seed data for new contributors, schema reset workflow, Postgres test isolation, `make db-pull` end-to-end verification.
