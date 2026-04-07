# Campaigns Architecture Notes (2026-04-07)

## Current State

The `internal/domain/campaigns/` package has grown to 55 source files / 18K LOC.
The `Service` interface defines ~55 methods covering CRUD, imports, analytics,
sell sheets, tuning, projections, cert operations, eBay export, and more.

The `service` struct also manages two background goroutines:
- `certEnrichWorker` — processes cert enrichment via a bounded channel
- `crackCacheWorker` — refreshes crack candidate cache periodically

These are lifecycle concerns (the service has a `Close()` method) that would
typically live in the scheduler layer rather than the domain.

## Recommended Direction

### Phase 1: Extract background workers (medium effort)
Move `certEnrichWorker` and `crackCacheWorker` to `internal/adapters/scheduler/`.
The service exposes the business logic methods they need; the scheduler owns the
goroutine lifecycle. This removes `Close()`, `sync.WaitGroup`, channels, and
cancel functions from the domain service.

### Phase 2: Split the Service interface (high effort)
Break the monolithic `Service` into focused interfaces:
- `CampaignService` — CRUD + archive
- `PurchaseService` — purchase CRUD + field updates
- `ImportService` — CL, PSA, external, orders imports
- `AnalyticsService` — PNL, aging, velocity, tuning, projections
- `SellSheetService` — sell sheet generation
- `CertService` — cert lookup, scan, import

Each handler file would accept only the interface it needs. This makes
dependencies explicit and testing simpler.

### Phase 3: Sub-package extraction (optional, high effort)
If the package continues to grow, consider extracting:
- `campaigns/imports/` — all parsing and import logic
- `campaigns/analytics/` — PNL, aging, velocity calculations
- `campaigns/arbitrage/` — crack + acquisition logic

The current file naming (service_import_cl.go, service_analytics.go, etc.)
already reflects this structure, so the split would be mechanical.

## Why not now?
These changes touch nearly every handler, test, and wiring point. The risk of
regressions outweighs the maintainability benefit unless accompanied by
comprehensive test coverage improvements (current: 63.1%).
