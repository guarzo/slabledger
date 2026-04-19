# Scheduler Architecture

Background schedulers run periodic jobs (price refresh, cache warmup, cleanup, etc.). All schedulers live in `internal/adapters/scheduler/` and share a common infrastructure.

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

`BuildGroup` (`builder.go`) is the single construction point. It reads from `config.Config`, wires all dependencies via `BuildDeps`, and returns a `BuildResult` containing the `Group` and an optional `CardLadderRefresh` reference.

Schedulers are conditionally included based on:
- Config flags (e.g. `cfg.PriceRefresh.Enabled`)
- Dependency availability (e.g. `deps.AuthService != nil`)
- Builder-level gates (e.g. access log cleanup checks both `Enabled` and `RetentionDays > 0`)

## Schedulers

### Price Refresh

**File:** `price_refresh.go`
**Purpose:** Refreshes stale card prices by calling the DH price provider.

Fetches cards with the oldest prices (prioritized by value-based staleness thresholds), groups them by provider, respects per-provider rate limits and hourly call caps, then logs daily API budget usage.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `PRICE_REFRESH_ENABLED` | `true` | Enable/disable |
| `RefreshInterval` | `PRICE_REFRESH_INTERVAL` | `1h` | How often to run |
| `BatchSize` | `PRICE_BATCH_SIZE` | `50` | Max cards per batch |
| `BatchDelay` | `PRICE_BATCH_DELAY` | `1s` | Delay between API calls |
| `MaxBurstCalls` | `PRICE_MAX_BURST_CALLS` | `10` | Calls before burst pause |
| `MaxCallsPerHour` | `PRICE_MAX_CALLS_PER_HOUR` | `50` | Hourly rate limit per provider |
| `BurstPauseDuration` | `PRICE_BURST_PAUSE_DURATION` | `30s` | Pause after burst limit |

**Additional features:** `Wait()` method for clean shutdown synchronization, `Health()` for liveness checks.

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

### Cache Warmup

**File:** `cache_warmup.go`
**Purpose:** Populates the persistent card cache by fetching all sets and their cards from TCGdex.

Skips sets already finalized on disk (via `NewSetIDsProvider`), aborts after 5 consecutive API errors, and rate-limits between set fetches.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `CACHE_WARMUP_ENABLED` | `true` | Enable/disable |
| `Interval` | `CACHE_WARMUP_INTERVAL` | `24h` | How often to run |
| `RateLimitDelay` | `CACHE_WARMUP_RATE_LIMIT_DELAY` | `2s` | Delay between GetCards calls |

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

### Session Cleanup

**File:** `session_cleanup.go`
**Purpose:** Deletes expired user sessions from the database.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `SESSION_CLEANUP_ENABLED` | `true` | Enable/disable |
| `Interval` | `SESSION_CLEANUP_INTERVAL` | `1h` | How often to run |

### DH Inventory Reconciliation

**File:** `dh_reconcile.go`
**Purpose:** Hourly drift scan that diffs local DH linkage against a fresh DH inventory snapshot. Purchases whose `dh_inventory_id` is no longer present on DH have their local DH fields cleared so the push scheduler re-enrolls them as `in_stock` on its next tick.

| Config | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `Enabled` | `DH_RECONCILE_ENABLED` | `true` | Enable/disable |
| `Interval` | `DH_RECONCILE_INTERVAL` | `1h` | How often to run |

A manual trigger is available at `POST /api/admin/dh-reconcile/trigger` (admin-only).

## Startup Timing

Schedulers coordinate their startup to avoid conflicts:

```
T=0s     All schedulers start
T=0s     Price refresh, inventory refresh, cache warmup,
         access log cleanup, session cleanup run immediately
```

## Shutdown

1. Application cancels the context passed to `StartAll`
2. Application calls `group.StopAll()` — closes each scheduler's `stopChan`
3. Application calls `group.Wait()` — blocks until all goroutines exit
4. Schedulers with their own `Wait()` (PriceRefresh) can also be waited on individually

## File Layout

```
internal/adapters/scheduler/
├── stop_handle.go           # StopHandle (embedded stop/wait infrastructure)
├── loop.go                  # RunLoop helper + LoopConfig
├── loop_test.go             # RunLoop unit tests
├── group.go                 # Scheduler interface, Group
├── builder.go               # BuildGroup, BuildDeps, BuildResult
├── config.go                # PriceRefresh-specific Config struct
├── price_refresh.go         # Price refresh scheduler
├── inventory_refresh.go     # Inventory snapshot refresh scheduler
├── cache_warmup.go          # Card cache warmup scheduler
├── access_log_cleanup.go    # Access log cleanup scheduler
└── session_cleanup.go       # Session cleanup scheduler
```
