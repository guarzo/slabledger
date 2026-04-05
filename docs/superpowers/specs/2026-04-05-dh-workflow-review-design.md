# DH Workflow Review — Before/After Integration Analysis

**Date:** 2026-04-05
**Status:** Ready for implementation — DH API changes confirmed

## Problem Statement

The DH (Double Holo) enterprise integration was added but the user workflow was never fully mapped end-to-end. Two issues surfaced:

1. **Missing steps** — inventory is never pushed to DH (the `PushInventory` client method exists but is never called in any handler, scheduler, or service)
2. **Redundant steps** — several manual CSV round-trips and background pricing jobs may be replaceable by DH capabilities

## Old Workflow (Pre-DH)

### Card Acquisition & Import
1. **Download PSA CSV** — communication spreadsheet with all purchased cards (including not-yet-arrived)
2. **Export to Card Ladder format** — `GET /api/purchases/export-cl` generates CL-compatible CSV
3. **Import into Card Ladder** — manual upload into Card Ladder web app
4. **Export from Card Ladder** — download CL export with CL prices and metadata
5. **Upload CL export back** — `POST /api/purchases/import-cl` brings CL values into SlabLedger

### Listing & Selling
6. **Card physically arrives** — `POST /api/purchases/import-certs` (cert lookup, flags for eBay export)
7. **Export eBay CSV** — `POST /api/purchases/export-ebay/generate` creates eBay bulk import CSV
8. **Upload to Shopify** — manual upload (Shopify syncs with eBay)
9. **Shopify price sync** — `POST /api/shopify/price-sync` matches Shopify products to inventory, returns updated prices
10. **Export updated prices** — manual CSV export for price updates

### Background Pricing (5 sources)
- **Card Ladder daily sync** — `CardLadderRefreshScheduler` (Firebase auth, collection sync, sales comps)
- **CardHedger batch** — `CardHedgerBatchScheduler` (cert discovery, all-grade pricing)
- **JustTCG refresh** — `JustTCGRefreshScheduler` (NM prices, 2000/day budget)
- **PriceCharting** — `PriceRefreshScheduler` (generic provider)
- **Fusion engine** — aggregates all sources with confidence weighting

## Current DH Integration (What's Built)

### Client (`internal/adapters/clients/dh/`)
- `Search()`, `Match()` — catalog search and AI-powered card matching
- `MarketData()` — tier 3 market data (price history, sales, population, sentiment, forecasts)
- `Suggestions()` — daily buy/sell recommendations
- `ResolveCert()`, `ResolveCertsBatch()` — PSA cert to DH card ID resolution
- `PushInventory()` — **exists but NEVER CALLED outside tests**
- `ListInventory()` — read inventory status from DH
- `GetOrders()` — completed sales from DH

### Schedulers (`internal/adapters/scheduler/`)
- `DHInventoryPollScheduler` (2h) — polls `ListInventory`, updates purchase DH fields
- `DHOrdersPollScheduler` (30m) — polls `GetOrders`, auto-confirms sales
- `DHIntelligenceRefreshScheduler` (1h) — refreshes stale market data (24h TTL)
- `DHSuggestionsScheduler` (6h) — fetches buy/sell recommendations

### Handlers (`internal/adapters/httpserver/handlers/`)
- `POST /api/dh/match` — async bulk match (0.90 confidence threshold)
- `GET /api/dh/unmatched` — list unmatched cards
- `GET /api/dh/export-unmatched` — CSV of unmatched cards
- `GET /api/dh/intelligence` — market intelligence for a card
- `GET /api/dh/suggestions` — latest suggestions
- `GET /api/dh/suggestions/inventory-alerts` — cross-ref vs inventory
- `GET /api/dh/status` — admin dashboard stats

### Pricing Integration
- `DHAdapter` in fusion engine as `SecondaryPriceSource` (0.90 confidence)
- Converts DH market data recent sales to grade-keyed prices

## What DH Can Replace

With DH handling pricing and Shopify/eBay integration:

| Old Step | Can DH Replace? | Notes |
|----------|----------------|-------|
| CardHedger batch pricing | Likely yes | DH has its own pricing data |
| JustTCG NM refresh | Likely yes | DH market data covers this |
| PriceCharting refresh | Likely yes | DH market data covers this |
| eBay CSV export + Shopify upload | Yes | DH pushes to Shopify/eBay via channels |
| Shopify price sync | Yes | DH manages listing prices |
| Manual price CSV exports | Yes | DH handles price updates |

| Old Step | Still Needed | Notes |
|----------|-------------|-------|
| PSA CSV download + import | Yes | PSA is the source of truth for purchased cards |
| Card Ladder export/import round-trip | Yes | DH doesn't have CL as a data source |
| Card Ladder daily sync | Yes | CL values still needed for non-DH pricing |
| Cert import (physical arrival) | Yes | But role changes — becomes the "list now" trigger |

## DH API Changes (Confirmed 2026-04-05)

DH agreed to all requested changes plus additional channel sync control.

### Updated Endpoints

**`POST /inventory` (upsert)** — new optional `status` field per item:
- `in_stock` (default): tracked, gets pricing/intelligence, NOT listed for sale
- `listed`: current behavior — creates MarketOrder + auto-prices on DH marketplace

**`PATCH /inventory/:id` (update)** — `status` now updatable alongside `cost_basis_cents`:
- `in_stock` → `listed`: creates MarketOrder, item goes live on DH marketplace
- `listed` → `in_stock`: cancels MarketOrder, delists from all channels, clears price

**`POST /inventory/:id/sync` (NEW)** — push to external channels:
- Request: `{"channels": ["ebay", "shopify"]}`
- Requires `listed` status (422 if `in_stock`)
- Returns channel statuses (each initially `pending`)

**`DELETE /inventory/:id/sync` (NEW)** — delist from specific channels:
- Request: `{"channels": ["ebay"]}` (or empty body for all)
- Item stays `listed` on DH, only removes from external channels

### Key Design Detail: Decoupled Auto-Sync

For enterprise API inventory, `listed` does NOT auto-push to external channels. Channel sync only happens via explicit `/sync` call. Non-enterprise flows (DH UI) keep auto-sync.

### Terminology Change

DH is updating their serializer: `pending`/`active` → `in_stock`/`listed`. No backward compatibility concern (no existing consumers).

## Ideal Workflow (Post-DH)

### Card Acquisition
1. Download PSA CSV → import into SlabLedger (unchanged)
2. Export to CL format → import into CL → export with prices → import back (unchanged)
3. **NEW: Push to DH as `in_stock`** — gets pricing data, intelligence, suggestions, but NO listings

### Card Arrives (Physical Possession)
4. Cert import (unchanged trigger)
5. **NEW: `PATCH status: listed`** — item goes live on DH marketplace
6. **NEW: `POST /sync channels: [ebay, shopify]`** — auto-triggered, pushes to external channels
7. No more eBay CSV export, no more Shopify upload, no more price sync

### Background Pricing (Slimmed Down)
- **Keep:** Card Ladder daily sync (DH doesn't have CL data)
- **Keep:** DH intelligence + suggestions schedulers
- **Keep:** DH inventory poll + orders poll
- **Evaluate removal:** CardHedger, JustTCG, PriceCharting (DH market data may fully replace these)
- **Keep but simplify:** Fusion engine (fewer sources)

## Open Questions (Remaining)

1. Can we fully drop CardHedger/JustTCG/PriceCharting, or do they provide data DH doesn't?
2. Should the eBay CSV export flow be kept as a fallback, or removed entirely?
3. What happens to the Shopify price sync endpoint — deprecate or repurpose?

## Implementation Plan

### DH Client Changes (`internal/adapters/clients/dh/`)
1. **`PushInventory`** — add optional `Status` field to request item struct (default `in_stock`)
2. **`UpdateInventory`** (PATCH) — add `Status` field alongside `CostBasisCents`
3. **New: `SyncChannels(ctx, inventoryID, channels []string)`** — `POST /inventory/:id/sync`
4. **New: `DelistChannels(ctx, inventoryID, channels []string)`** — `DELETE /inventory/:id/sync`
5. **Response types** — update `pending`/`active` → `in_stock`/`listed`

### Workflow Wiring
6. **Purchase import** — after DH match, call `PushInventory` with `status: in_stock`
7. **Cert import (card arrives)** — `PATCH status: listed` + auto-trigger `SyncChannels([ebay, shopify])`
8. **Inventory poll scheduler** — handle new status values in responses

### Future Cleanup (After Validation)
9. Evaluate removing CardHedger/JustTCG/PriceCharting schedulers + clients
10. Simplify fusion engine to fewer sources
11. Deprecate eBay CSV export and Shopify price sync endpoints
                                                              