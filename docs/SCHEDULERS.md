# Scheduler Architecture

Background schedulers run periodic jobs (price refresh, DH sync, cleanup, etc.). All
schedulers live in `internal/adapters/scheduler/` and share a common infrastructure.

The roster below is derived from `builder_schedulers.go`. When it disagrees with that
file, the file wins — get the current set with:

```bash
grep -n '^func build' internal/adapters/scheduler/builder_schedulers.go
```

## Core Abstractions

### `Scheduler` Interface

Every scheduler implements this two-method interface:

```go
type Scheduler interface {
    Start(ctx context.Context)
    Stop()
}
```

`Start` blocks until the context is cancelled or `Stop` is called. `Stop` is idempotent — safe to call multiple times.

### `StopHandle` (Embedded)

Every scheduler embeds a `StopHandle` (defined in `stop_handle.go`), which provides:

- `Stop()` — idempotent; closes the stop channel
- `Wait()` — blocks until WG-tracked goroutines finish
- `WG()` — returns a `*sync.WaitGroup` for use with `RunLoop`
- `C` — the stop channel, passed to `RunLoop` as `StopChan`

Constructors initialize it with `NewStopHandle()`.

### `RunLoop` Helper

All schedulers delegate their tick/stop/context loop to `RunLoop` (defined in `loop.go`), which eliminates the duplicated select-loop boilerplate. Configuration is passed via `LoopConfig`:

| Field          | Type                  | Description                                      |
|----------------|-----------------------|--------------------------------------------------|
| `Name`         | `string`              | Used in log messages (e.g. `"price-refresh"`)    |
| `Interval`     | `time.Duration`       | Ticker interval                                  |
| `InitialDelay` | `time.Duration`       | Delay before first run (0 = run immediately)     |
| `WG`           | `*sync.WaitGroup`     | Optional — enables `Wait()` on the scheduler     |
| `StopChan`     | `<-chan struct{}`      | Receives stop signal from `Stop()`               |
| `Logger`       | `observability.Logger` | Structured logger                               |
| `LogFields`    | `[]observability.Field`| Extra fields logged at startup                  |

`RunLoop` handles:
1. Optional `WaitGroup` tracking (`Add`/`Done`)
2. Startup log with interval + custom fields
3. Initial execution (immediate or after `InitialDelay`)
4. Standard `for/select` loop: context cancellation, stop signal, or ticker
5. Shutdown log messages

### `Group` and `BuildGroup`

`Group` (`group.go`) manages multiple schedulers as a unit:

```go
group := scheduler.NewGroup(s1, s2, s3)
group.StartAll(ctx)   // launches each in its own goroutine
group.StopAll()       // signals all to stop
group.Wait()          // blocks until all have exited
```

`BuildGroup` (`builder.go`) is the near-single construction point. It reads from
`config.Config`, wires all dependencies via `BuildDeps`, and returns a `BuildResult`
holding the `Group` plus direct references to the schedulers callers need to reach
individually: `CardLadderRefresh`, `PSASync`, `CertEnrichJob`, `DHOrdersPoll`, and
`DHReconcile` (each nil when unconfigured). Those references exist because admin
endpoints trigger those schedulers on demand, and because the pricing-enrichment job
needs `CardLadderRefresh` as a pricer.

Each scheduler is built by its own `buildXScheduler` helper in `builder_schedulers.go`,
which returns a **concrete pointer type**, not the `Scheduler` interface. That is
deliberate: a nil `*T` assigned to an interface compares non-nil, so returning the
interface would let a nil scheduler into the group.

Schedulers are conditionally included based on:
- Config flags (e.g. `cfg.PriceRefresh.Enabled`)
- Dependency availability (e.g. `deps.AuthService != nil`)
- Builder-level gates (e.g. access log cleanup checks both `Enabled` and `RetentionDays > 0`)

## Roster

`BuildGroup` builds 19 schedulers; `cmd/slabledger/init_schedulers.go` adds a 20th
(pricing enrichment) after the Card Ladder scheduler exists to serve as its pricer.

| Scheduler | Env gate | Cadence | Also requires |
|-----------|----------|---------|---------------|
| Price refresh | `PRICE_REFRESH_ENABLED` | `1h` | — (the one unconditional scheduler) |
| Session cleanup | `SESSION_CLEANUP_ENABLED` | `1h` | auth configured |
| Access log cleanup | `ACCESS_LOG_CLEANUP_ENABLED` | `24h` | `ACCESS_LOG_RETENTION_DAYS > 0` |
| DH event cleanup | `DH_EVENT_CLEANUP_ENABLED` | `24h` | `DH_EVENT_RETENTION_DAYS > 0`, event pruner |
| Inventory refresh | `INVENTORY_REFRESH_ENABLED` | `1h` | inventory lister + snapshot refresher |
| Snapshot enrich | `SNAPSHOT_ENRICH_ENABLED` | `15s` | snapshot enrich service |
| Card Ladder refresh | `CARDLADDER_REFRESH_ENABLED` | daily at `CARDLADDER_REFRESH_HOUR` | CL store + purchase lister + value updater |
| Pricing enrich | — (queue-driven) | on enqueue | pre-built job wired into `inventory.Service` |
| DH intelligence refresh | `DH_ENABLED` | `1h` | enterprise DH key, intelligence repo |
| DH analytics refresh | `DH_ANALYTICS_REFRESH_ENABLED` | daily at `DH_ANALYTICS_REFRESH_HOUR` | enterprise DH key, demand repo |
| Card trajectory refresh | `DH_ENABLED` | `7d` | enterprise DH key, trajectory repo, seed lister |
| DH suggestions | `DH_ENABLED` | `6h` | enterprise DH key, suggestions repo |
| DH orders poll | `DH_ENABLED` | `DH_ORDERS_POLL_INTERVAL` (`30m`) | orders client, sync state, orders importer |
| DH inventory poll | `DH_ENABLED` | `DH_INVENTORY_POLL_INTERVAL` (`2h`) | inventory list client, sync state, cert lookup |
| DH sold reconciler | `DH_SOLD_RECONCILER_ENABLED` | `1h` | purchase repo |
| DH reconcile | `DH_RECONCILE_ENABLED` | `1h` | DH reconciler |
| DH price sync | `DH_PRICE_SYNC_ENABLED` | `15m` | DH price-sync service |
| DH push | `DH_ENABLED` | `DH_PUSH_INTERVAL` (`5m`) | enterprise DH key, pending lister, status updater |
| PSA sync | `PSA_SYNC_ENABLED` (default `false`) | daily at `PSA_SYNC_HOUR` | PSA row provider + importer |
| Cert enrich | — (queue-driven) | on enqueue | cert lookup + purchase repo |

Two env gates named in `.env.example` are **not** app schedulers: `PSA_PORTAL_ENABLED`
turns on the portal token reader, and `PSA_CAMPAIGN_SYNC_ENABLED` is read by the
separate `cmd/psa-harvest` job (see [psa-harvester.md](psa-harvester.md)).

## Schedulers

### Price Refresh

**File:** `price_refresh.go`
**Purpose:** Refreshes stale card prices by calling the DH price provider.

Fetches cards with the oldest prices (prioritized by value-based staleness thresholds), groups them by provider, respects per-provider rate limits and hourly call caps, then logs daily API budget usage.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `PRICE_REFRESH_ENABLED` | `true` | Enable/disable |
| `RefreshInterval` | `PRICE_REFRESH_INTERVAL` | `1h` | How often to run |
| `BatchSize` | `PRICE_BATCH_SIZE` | `100` | Max cards per batch |
| `BatchDelay` | `PRICE_BATCH_DELAY` | `200ms` | Delay between API calls |
| `MaxBurstCalls` | `PRICE_MAX_BURST_CALLS` | `50` | Calls before burst pause |
| `MaxCallsPerHour` | `PRICE_MAX_CALLS_PER_HOUR` | `400` | Hourly rate limit per provider |
| `BurstPauseDuration` | `PRICE_BURST_PAUSE_DURATION` | `10s` | Pause after burst limit |

**Additional features:** `Wait()` method for clean shutdown synchronization, `Health()` for liveness checks.

### Session Cleanup

**File:** `session_cleanup.go`
**Purpose:** Deletes expired user sessions from the database.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `SESSION_CLEANUP_ENABLED` | `true` | Enable/disable |
| `Interval` | `SESSION_CLEANUP_INTERVAL` | `1h` | How often to run |

### Access Log Cleanup

**File:** `access_log_cleanup.go`
**Purpose:** Deletes old card access log entries to prevent unbounded table growth.

Runs a `DELETE FROM card_access_log WHERE accessed_at < ...` query using the `accessed_at` index for efficient cleanup.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `ACCESS_LOG_CLEANUP_ENABLED` | `true` | Enable/disable |
| `Interval` | `ACCESS_LOG_CLEANUP_INTERVAL` | `24h` | How often to run |
| `RetentionDays` | `ACCESS_LOG_RETENTION_DAYS` | `30` | Days of logs to keep |

**Note:** Enabled check is handled in `BuildGroup` rather than in `Start()`.

### DH Event Cleanup

**File:** `dh_event_cleanup.go`
**Purpose:** Deletes old `dh_state_events` rows to prevent unbounded table growth.

The table is append-only — every push, poll, reconcile, and manual match writes a row,
and no other code path deletes one — so this scheduler is the only bound on its size.
Runs a `DELETE FROM dh_state_events WHERE event_at < ...`.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_EVENT_CLEANUP_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_EVENT_CLEANUP_INTERVAL` | `24h` | How often to run |
| `RetentionDays` | `DH_EVENT_RETENTION_DAYS` | `90` | Days of events to keep |

Retention is deliberately longer than the access log's 30 days: this is a diagnostic
trail for a pipeline whose failures surface over weeks, so the history has to outlive
the gap between a failure and someone looking for it (`GET /api/dh/events`).

**Note:** Enabled check is handled in `BuildGroup` rather than in `Start()`; the builder
also returns nil when no pruner is wired.

### Inventory Refresh

**File:** `inventory_refresh.go`
**Purpose:** Refreshes market snapshots on unsold inventory purchases.

Lists unsold purchases, filters to those with stale or missing snapshots, sorts by value (highest first), and refreshes up to `BatchSize` per cycle with rate limiting between calls.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `INVENTORY_REFRESH_ENABLED` | `true` | Enable/disable |
| `Interval` | `INVENTORY_REFRESH_INTERVAL` | `1h` | How often to run |
| `StaleThreshold` | `INVENTORY_REFRESH_STALE_THRESHOLD` | `12h` | Age at which snapshots are stale |
| `BatchSize` | `INVENTORY_REFRESH_BATCH_SIZE` | `20` | Max purchases per cycle |
| `BatchDelay` | `INVENTORY_REFRESH_BATCH_DELAY` | `2s` | Delay between API calls |

### Snapshot Enrichment

**File:** `snapshot_enrich.go`
**Purpose:** Drains the pending-snapshot queue produced by async CSV/order imports.

Runs on a short tick because it exists to close the gap between an import completing
and its rows showing market data. Failed snapshots retry on a separate, much slower
interval until `MaxRetries` is exhausted.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `SNAPSHOT_ENRICH_ENABLED` | `true` | Enable/disable |
| `Interval` | `SNAPSHOT_ENRICH_INTERVAL` | `15s` | Pending-queue tick |
| `RetryInterval` | `SNAPSHOT_ENRICH_RETRY_INTERVAL` | `30m` | Retry tick for failed snapshots |
| `BatchSize` | `SNAPSHOT_ENRICH_BATCH_SIZE` | `3` | Max purchases per tick |
| `MaxRetries` | `SNAPSHOT_ENRICH_MAX_RETRIES` | `5` | Attempts before marking exhausted |

### Card Ladder Refresh

**File:** `cardladder_refresh.go` (plus the `cardladder_*.go` helpers)
**Purpose:** Daily Card Ladder value sync for unsold inventory, and the pricer backing
the pricing-enrichment job.

Card Ladder is a valuation source wired separately from the `pricing.PriceProvider`
pipeline — see [ARCHITECTURE.md](ARCHITECTURE.md). The scheduler is constructed
whenever the store and purchase interfaces exist, **even if no CL client is configured
yet**: `SetClient` is called by the credentials handler when credentials are first
saved, activating it without a server restart.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `CARDLADDER_REFRESH_ENABLED` | `true` | Enable/disable |
| `RefreshHour` | `CARDLADDER_REFRESH_HOUR` | `4` | UTC hour for the daily run |
| `Interval` | — | `24h` | Not env-configurable |

### Pricing Enrichment

**File:** `pricing_enrich.go`
**Purpose:** Prices a single newly-imported purchase on demand, rather than waiting for
the next price-refresh tick.

Queue-driven, not interval-driven: `inventory.Service` enqueues a cert number, and a
worker pool prices it. Its pricers are attached in `init_schedulers.go` after
`BuildGroup` returns, because the Card Ladder scheduler is one of them — which is why
this is the one group member `BuildGroup` does not add itself.

### DH Intelligence Refresh

**File:** `dh_intelligence_refresh.go`
**Purpose:** Refills `market_intelligence` from DH Enterprise, seeded by our own
inventory.

Runs hourly with a per-run cap of 50 cards. Cards DH repeatedly 404s on are recorded in
the tombstone repo so they are not retried forever.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ENABLED` | `true` | Enable/disable |
| `CacheTTL` | `DH_CACHE_TTL_HOURS` | `24` | Age at which an entry is refreshed |
| `Interval` | — | `1h` | Not env-configurable |
| `MaxPerRun` | — | `50` | Not env-configurable |

### DH Analytics Refresh

**File:** `dh_analytics_refresh.go` (plus `_steps.go`, `_attribution.go`)
**Purpose:** Caches per-card and per-character demand, velocity, and saturation signals
that back the niche-opportunity leaderboard and campaign signals.

Scheduled for 04:00 UTC by default because DH's own nightly analytics rollup lands at
03:15 UTC — running earlier reads yesterday's numbers.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ANALYTICS_REFRESH_ENABLED` | `true` | Enable/disable |
| `RefreshHour` | `DH_ANALYTICS_REFRESH_HOUR` | `4` | UTC hour for the daily run |
| `Window` | `DH_ANALYTICS_REFRESH_WINDOW` | `30d` | Demand signal window passed to DH |

### Card Trajectory Refresh

**File:** `card_trajectory_refresh.go`
**Purpose:** Builds weekly price-trajectory buckets from DH's graded-sales-analytics
`recent_sales` array.

Weekly rather than daily: the buckets are weekly, so a faster cadence would spend DH
budget to recompute the same numbers.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ENABLED` | `true` | Enable/disable |
| `Interval` | — | `7d` | Not env-configurable |

### DH Suggestions

**File:** `dh_suggestions.go`
**Purpose:** Refreshes the DH acquisition-suggestions cache.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ENABLED` | `true` | Enable/disable |
| `Interval` | — | `6h` | Not env-configurable |

### DH Orders Poll

**File:** `dh_orders_poll.go`
**Purpose:** Polls the DH v2 orders endpoint and lands new orders as sales.

Cursor state lives in the shared `SyncStateStore`, so a restart resumes rather than
re-importing. Import goes through `OrdersImporter` rather than `CampaignService`
because CSV/orders intake lives in its own package.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_ORDERS_POLL_INTERVAL` | `30m` | How often to poll |

### DH Inventory Poll

**File:** `dh_inventory_poll.go`
**Purpose:** Syncs listing status back from DH onto local purchases, matched by cert.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_INVENTORY_POLL_INTERVAL` | `2h` | How often to poll |

### DH Sold Reconciler

**File:** `dh_sold_reconciler.go`
**Purpose:** Repairs purchases that have a linked sale but whose `dh_status` never
advanced to `sold`, and records those sales properly on DH.

A safety net for the best-effort `dh_status` update inside `CreateSale` — that update
is deliberately non-fatal to the sale, so something has to catch the misses. Beyond the
local column repair it runs two DH-side passes: a sweep that records a real sale via the
DH sale endpoint for anything DH still lists that we have already sold, and a
handle-recovery pass that finds sales DH already accepted whose `dh_sale_id` we failed
to persist. See `docs/specs/2026-08-20-dh-record-sale-design.md`.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_SOLD_RECONCILER_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_SOLD_RECONCILER_INTERVAL` | `1h` | How often to run |

### DH Inventory Reconciliation

**File:** `dh_reconcile.go`
**Purpose:** Hourly drift scan that diffs local DH linkage against a fresh DH inventory snapshot. Purchases whose `dh_inventory_id` is no longer present on DH have their local DH fields cleared so the push scheduler re-enrolls them as `in_stock` on its next tick.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_RECONCILE_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_RECONCILE_INTERVAL` | `1h` | How often to run |

A manual trigger is available at `POST /api/admin/dh-reconcile/trigger` (admin-only).

### DH Price Sync

**File:** `dh_price_sync.go`
**Purpose:** Reconciles drift between `reviewed_price_cents` and
`dh_listing_price_cents` for items already on DH.

Also runs inline on every review-price edit; the scheduler is the safety net for failed
or missed inline syncs.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_PRICE_SYNC_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_PRICE_SYNC_INTERVAL` | `15m` | How often to run |

### DH Push

**File:** `dh_push.go` (plus `dh_psa_import.go`)
**Purpose:** Matches pending purchases and pushes them to DH inventory as listings.

The fastest DH scheduler, because it is the one on the path between "item acquired" and
"item listed for sale." See [DH_INVENTORY.md](DH_INVENTORY.md) for the push state
machine and hold semantics.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_PUSH_INTERVAL` | `5m` | How often to run |

### PSA Sync

**File:** `psa_sync.go`
**Purpose:** Daily import of PSA Buyer Campaign Manager rows.

Off by default. The app side only reads the encrypted portal token from Postgres; the
`cmd/psa-harvest` job is what logs in with a real browser and writes a fresh one. See
[psa-harvester.md](psa-harvester.md).

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `PSA_SYNC_ENABLED` | `false` | Enable/disable |
| `SyncHour` | `PSA_SYNC_HOUR` | `10` | UTC hour for the daily run (`-1` = use `InitialDelay`) |
| `Interval` | — | `24h` | Not env-configurable |
| `InitialDelay` | — | `5m` | Not env-configurable |

### Cert Enrichment

**File:** `cert_enrich.go`
**Purpose:** Fills in PSA cert details and slab images for a purchase on demand.

Queue-driven like pricing enrichment. The job is built *before* service construction so
the same instance can be injected into `inventory.Service` via `WithCertEnrichEnqueuer`
— `buildCertEnrichJob` prefers that pre-built instance over creating its own.

Setting `BACKFILL_IMAGES=true` enqueues every unsold PSA purchase with empty image URLs
once at startup. It draws on the PSA daily budget, so set it back to `false` after one
run.

## Startup Timing

There is no staggering. `StartAll` launches every scheduler at `T=0`, and each one's
first execution is governed by its own `InitialDelay` (`0` = run immediately). The
daily schedulers — Card Ladder refresh, DH analytics refresh, PSA sync — compute their
own delay to the next occurrence of their configured UTC hour instead.

## Shutdown

1. Application cancels the context passed to `StartAll`
2. Application calls `group.StopAll()` — closes each scheduler's `stopChan`
3. Application calls `group.Wait()` — blocks until all goroutines exit
4. Schedulers with their own `Wait()` (PriceRefresh) can also be waited on individually

## File Layout

`internal/adapters/scheduler/` holds roughly 37 non-test files, so an exhaustive tree
here would go stale silently. The shared infrastructure is:

```
internal/adapters/scheduler/
├── stop_handle.go           # StopHandle (embedded stop/wait infrastructure)
├── loop.go                  # RunLoop helper + LoopConfig
├── group.go                 # Scheduler interface, Group
├── builder.go               # BuildGroup, BuildDeps, BuildResult
├── builder_schedulers.go    # One buildXScheduler helper per scheduler
├── config.go                # PriceRefresh-specific Config struct
├── shared_types.go          # Interfaces shared across schedulers
└── timeutil.go              # Next-daily-run helpers
```

Everything else is a scheduler implementation named after its entry in the roster table
above, occasionally split across several files (`cardladder_*.go`,
`dh_analytics_refresh*.go`). For the current set:

```bash
ls internal/adapters/scheduler/*.go | grep -v _test.go
```
