# DH (DoubleHolo) Inventory Pipeline

How cards flow from local inventory to DH's marketplace.

## Overview

```
PSA Cert Import → CL Value Lookup → Cert Resolution → Inventory Push → Channel Sync
     (CSV)         (CardLadder)        (DH API)          (DH API)       (eBay/Shopify)
```

## Pipeline States (`dh_push_status`)

| Status | Meaning | Next step |
|--------|---------|-----------|
| *(empty)* | Not yet enrolled | `NeedsDHPush()` checks eligibility |
| `pending` | Queued for push | Scheduler picks up in next cycle |
| `matched` | Successfully pushed to DH | Done (unless CL value changes) |
| `unmatched` | Cert couldn't be resolved | Manual fix via URL or candidate selection |
| `manual` | User manually resolved | Done |

## Step-by-Step Flow

### 1. Enrollment

A purchase becomes eligible for DH push when:
- It has no `DHInventoryID` (not already on DH)
- Its `dh_push_status` is not already `pending`, `unmatched`, or `manual`

**Triggers:**
- CL import/refresh sets status to `pending` when `NeedsDHPush()` returns true
- CL value change on an already-pushed item re-enrolls it as `pending` (re-push)

**Guard:** Items with `CLValueCents == 0` are skipped by the push scheduler (left as `pending` until CL value arrives).

### 2. Cert Resolution

The DH push scheduler (`dh_push.go`, every 5 min) resolves PSA certs to DH card IDs:

```
POST /api/v1/enterprise/certs/resolve
{
  "cert": {
    "cert_number": "12345678",
    "gemrate_id": "abc123...",       // CL gemRateID (optional, enables direct lookup)
    "card_name": "Charizard",
    "set_name": "Base Set",
    "card_number": "4",
    "year": "1999",
    "variant": "holo"
  }
}
```

**Resolution outcomes:**
- `matched` → Got a DH card ID, proceed to push
- `ambiguous` → Multiple candidates; attempt card-number disambiguation
- `not_found` → No match, mark as `unmatched`

**Optimizations:**
- Card ID mappings are cached in `card_id_mapping` table (avoids redundant API calls for same card)
- Re-pushes (items with existing `DHCardID`) skip cert resolution entirely
- PSA API key rotation on rate limit errors (`X-PSA-API-Key` header)

### 3. Inventory Push

```
POST /api/v1/enterprise/inventory
{
  "items": [{
    "dh_card_id": 12345,
    "cert_number": "98765432",
    "grading_company": "psa",
    "grade": 9,
    "cost_basis_cents": 15000,
    "market_value_cents": 45000,    // CLValueCents — DH uses this for pricing
    "status": "in_stock"
  }]
}
```

- **Upsert semantics** — same cert number updates the existing item
- `market_value_cents` is critical: without it, DH falls back to sparse internal pricing and items may get $0.00 prices
- Response includes `dh_inventory_id` and `assigned_price_cents`
- Local purchase is updated with `DHCardID`, `DHInventoryID`, `DHCertStatus`, etc.

### 4. Channel Sync (Listing)

After push, items are `in_stock`. To list for sale:

```
POST /api/v1/enterprise/inventory/{id}/sync
{ "channels": ["ebay", "shopify"] }
```

This is triggered when a user marks a card for listing in the UI.

## Push Sites

There are 5 places that push inventory to DH:

| Site | File | When |
|------|------|------|
| Push scheduler | `scheduler/dh_push.go` | Every 5 min, processes `pending` items |
| Bulk match | `handlers/dh_match_handler.go` | User-triggered bulk match + push |
| Inline match | `handlers/campaigns_dh_listing.go` | On-demand when listing a pending card |
| Select match | `handlers/dh_select_match_handler.go` | User picks from ambiguous candidates |
| Fix match | `handlers/dh_fix_match_handler.go` | User pastes a DH URL for manual match |

All sites send `CLValueCents` as both `cost_basis_cents` and `market_value_cents`.

## Re-Push on Price Change

When CL value changes on an already-pushed item:

1. **CL import service** (`service_import_cl.go`): detects `DHInventoryID != 0 && newCL != oldCL`, sets status back to `pending`
2. **CL refresh scheduler** (`cardladder_refresh.go`): same logic for API-driven value updates
3. **Push scheduler** picks it up, sees `DHCardID` already set, skips cert resolution, pushes with updated `market_value_cents`
4. DH's upsert semantics update the existing inventory item

## Manual Resolution

For `unmatched` cards:

- **Fix match**: User pastes a `doubleholo.com/card/{id}` URL → parses card ID → pushes
- **Select match**: User picks from stored disambiguation candidates → pushes
- Both set status to `manual`

## DH Status Dashboard

`GET /api/dh/status` returns:

| Field | Source |
|-------|--------|
| `pending_count` | DB: `dh_push_status = "pending"` |
| `unmatched_count` | DB: `dh_push_status = "unmatched"` |
| `mapped_count` | DB: `dh_push_status IN ("matched", "manual")` |
| `dh_inventory_count` | DH API: `GET /inventory` total count |
| `dh_listings_count` | DH API: `GET /inventory?status=listed` total count |
| `dh_orders_count` | DH API: `GET /orders` total count |
| `api_health` | In-memory 7-day rolling success rate |

## Key Files

| Component | File |
|-----------|------|
| Push scheduler | `internal/adapters/scheduler/dh_push.go` |
| DH API client | `internal/adapters/clients/dh/` |
| API types | `internal/adapters/clients/dh/types_v2.go` |
| Card name cleaning | `internal/domain/campaigns/dh_helpers.go` |
| Push status DB ops | `internal/adapters/storage/sqlite/purchases_repository_dh.go` |
| Card ID mapping cache | `internal/adapters/storage/sqlite/card_id_mapping_repository.go` |
| HTTP handlers | `internal/adapters/httpserver/handlers/dh_*.go` |
| CL refresh (re-push) | `internal/adapters/scheduler/cardladder_refresh.go` |
| CL import (re-push) | `internal/domain/campaigns/service_import_cl.go` |
| Scheduler wiring | `internal/adapters/scheduler/builder.go` |
