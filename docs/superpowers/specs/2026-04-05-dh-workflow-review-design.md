# DH Workflow Review — Before/After Integration Analysis

**Date:** 2026-04-05
**Status:** In Progress — Awaiting DH API change request

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

## Gap: Inventory Push & Listing Control

### The Problem
`PushInventory` is never called. Cards never get sent to DH. Even if we wire it up, we need to control WHEN listings go live.

### DH API Status Enum (from `enterprise-api.yaml`)
The inventory `status` field supports: `in_stock`, `listed`, `reserved`, `sold`, `traded_out`, `consigned_out`, `lost`, `damaged`, `returned`

- `in_stock` = in possession, NOT listed for sale
- `listed` = actively for sale on channels

### API Limitation
The current `POST /inventory` (upsert) request body only accepts:
- `dh_card_id` (required)
- `cert_number` (required)
- `grading_company` (required)
- `grade` (required)
- `cost_basis_cents` (required)

**No `status` field.** We cannot specify `in_stock` vs `listed` on push.

The `PATCH /inventory/{id}` only allows updating `cost_basis_cents`. **No `status` field either.**

### Required API Changes
We need DH to add `status` control to:
1. `POST /inventory` — optional `status` field per item (default: `in_stock` or current behavior)
2. `PATCH /inventory/{id}` — allow updating `status` (so we can flip `in_stock` -> `listed` when card arrives)

## Ideal Workflow (Post-DH, Assuming API Changes)

### Card Acquisition
1. Download PSA CSV -> import into SlabLedger (unchanged)
2. Export to CL format -> import into CL -> export with prices -> import back (unchanged)
3. **NEW: Push to DH as `in_stock`** — gets pricing data, intelligence, suggestions, but NO listings created

### Card Arrives (Physical Possession)
4. Cert import (unchanged trigger)
5. **NEW: Update DH status to `listed`** — DH creates Shopify/eBay listings automatically
6. No more eBay CSV export, no more Shopify upload, no more price sync

### Background Pricing (Slimmed Down)
- **Keep:** Card Ladder daily sync (DH doesn't have CL data)
- **Keep:** DH intelligence + suggestions schedulers
- **Keep:** DH inventory poll + orders poll
- **Evaluate removal:** CardHedger, JustTCG, PriceCharting (DH market data may fully replace these)
- **Keep but simplify:** Fusion engine (fewer sources)

## Open Questions

1. Will DH add `status` to upsert and patch endpoints?
2. What is DH's default behavior when inventory is pushed — does it auto-list or stay `in_stock`?
3. Can we fully drop CardHedger/JustTCG/PriceCharting, or do they provide data DH doesn't?
4. Should the eBay CSV export flow be kept as a fallback, or removed entirely?
5. What happens to the Shopify price sync endpoint — deprecate or repurpose?


  Subject: Enterprise API — Add status field to inventory upsert and patch endpoints

  We're integrating inventory push from our system and need to control when items go live on sales channels. Our workflow has two
  distinct phases:

  1. Card purchased but not yet in hand — we want to push to DH early so we get pricing data, market intelligence, and suggestions, but
   the card should NOT be listed for sale
  2. Card physically arrives — flip to active listing on Shopify/eBay

  The inventory list endpoint already returns a status field with values like in_stock and listed, but we can't set it:

  Request 1 — POST /api/v1/enterprise/inventory (upsert):
  Add an optional status field to each item in the request body. Suggested default: in_stock (safe — nothing gets listed until
  explicitly requested).

  status:
    type: string
    enum: [in_stock, listed]
    default: in_stock
    description: Initial inventory status. "in_stock" = tracked but not listed. "listed" = active on sales channels.

  Request 2 — PATCH /api/v1/enterprise/inventory/{id} (update):
  Add status as an updatable field alongside cost_basis_cents, so we can transition in_stock -> listed when we're ready to sell.

  status:
    type: string
    enum: [in_stock, listed]
    description: Update inventory status. Set to "listed" to activate sales channel listings.

  This would let us push inventory early for data enrichment without prematurely creating listings. Let us know if this is feasible or
  if there's an existing mechanism we're missing.
                                                              