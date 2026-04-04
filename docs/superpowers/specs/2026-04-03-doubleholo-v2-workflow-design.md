# DoubleHolo v2 API — Workflow Simplification Spec

## Problem

The current card listing workflow requires 5 manual steps across 3 systems:

1. Upload PSA sheet to SlabLedger (creates purchases)
2. Export Card Ladder CSV from SlabLedger
3. Import CSV into Card Ladder (prices and creates listings)
4. Export eBay/Shopify file from Card Ladder
5. Import into Shopify/eBay

Sales recording is also manual: export orders CSV from Shopify, import into SlabLedger.

## Goal

Reduce the workflow to 2 steps with automated sales recording:

1. Upload PSA sheet to SlabLedger (creates purchases, batch-resolves certs via DoubleHolo)
2. Push inventory to DoubleHolo (DH auto-prices and syncs to Shopify/eBay)

Sales flow back automatically via scheduled polling — no manual export/import.

Card Ladder is removed from the critical listing path.

## DoubleHolo v2 API Endpoints (Confirmed)

Based on DH's accepted proposal. All monetary values are in **cents** (integer, suffixed `_cents`). Auth and rate limits unchanged from v1 (Bearer token, 100 req/min, 1,000 req/hour). Batch endpoints count as a single request.

### 1. Cert Resolution

Three endpoints covering sync single-cert lookup, async batch submission, and batch result polling. All resolve PSA cert numbers to DoubleHolo card identities via PSA's API using our PSA API key (configured in DH account).

**Prerequisite:** PSA API key must be configured in DH account. Endpoints return an error if not configured.

#### Shared Cert Object

All cert resolution endpoints accept the same cert object shape:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cert_number` | string | yes | PSA certification number |
| `card_name` | string | no | Hint to improve matching |
| `set_name` | string | no | Hint to improve matching |
| `card_number` | string | no | Card number within set |
| `year` | string | no | Release year hint |
| `variant` | string | no | e.g., "Holofoil", "1st Edition" |
| `language` | string | no | e.g., "English", "Japanese" |

#### Shared Result Shape

Each resolved cert returns one of three statuses:

| Status | Description |
|--------|-------------|
| `matched` | Single confident match. Full card details and market price included. |
| `ambiguous` | Multiple possible matches. Candidates returned for disambiguation. |
| `not_found` | No match in DH database. |

```json
[
  {
    "cert_number": "12345678",
    "status": "matched",
    "dh_card_id": 51942,
    "card_name": "Charizard",
    "set_name": "Base Set",
    "card_number": "4/102",
    "grade": 9,
    "image_url": "https://...",
    "current_market_price_cents": 45000
  },
  {
    "cert_number": "87654321",
    "status": "ambiguous",
    "candidates": [
      {
        "dh_card_id": 12001,
        "card_name": "Pikachu",
        "set_name": "Base Set",
        "card_number": "58/102",
        "image_url": "https://..."
      },
      {
        "dh_card_id": 12002,
        "card_name": "Pikachu",
        "set_name": "Base Set 2",
        "card_number": "87/130",
        "image_url": "https://..."
      }
    ]
  },
  {
    "cert_number": "99999999",
    "status": "not_found"
  }
]
```

#### 1a. Sync Single Cert

`POST /enterprise/certs/resolve`

Resolves a single cert synchronously. Calls PSA API once, returns immediately. Fast for one-off lookups and individual reconciliation.

**Request:** A single cert object (not an array).
```json
{
  "cert_number": "12345678",
  "card_name": "Charizard",
  "set_name": "Base Set"
}
```

**Response:** A single result object (same shape as the shared result above).

#### 1b. Async Batch Submit

`POST /enterprise/certs/resolve_batch`

Submits up to 500 certs for asynchronous resolution. Returns a job ID immediately.

**Limits:** Max 500 certs per request.

**Request:**
```json
{
  "certs": [
    { "cert_number": "12345678" },
    {
      "cert_number": "87654321",
      "card_name": "Charizard",
      "set_name": "Base Set",
      "card_number": "4/102",
      "year": "1999",
      "variant": "Holofoil",
      "language": "English"
    }
  ]
}
```

**Response:**
```json
{
  "job_id": "job_abc123",
  "status": "queued",
  "total_certs": 200
}
```

#### 1c. Batch Result Poll

`GET /enterprise/certs/resolve_batch/:job_id`

Returns the current status and any available results for a batch resolution job. Supports partial results — resolved certs appear as they complete.

**Response:**
```json
{
  "job_id": "job_abc123",
  "status": "completed",
  "total_certs": 200,
  "resolved_count": 185,
  "results": [ ... ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `job_id` | string | Job identifier |
| `status` | string | `queued`, `processing`, `completed`, `failed` |
| `total_certs` | integer | Number of certs submitted |
| `resolved_count` | integer | Number of certs with results so far |
| `results` | array | Available results (partial during `processing`, complete when `completed`) |

#### Usage Patterns

- **PSA import (batch):** Submit all certs via `resolve_batch` (up to 500 per job, so a typical PSA sheet fits in one job). Poll for results. A 200-cert sheet is a single submission.
- **Reconciliation (single):** User fixes one cert at a time — use the sync `resolve` endpoint with enriched hints (card name, set, number, year, variant, language) gathered from Card Ladder data or manual correction.
- **Reconciliation (batch):** Re-submit multiple unmatched/ambiguous certs with hints via `resolve_batch`.

### 2. Inventory Management

#### Create / Update (Upsert)

`POST /inventory`

Push inventory to DoubleHolo with upsert semantics — if a cert number already exists, updates cost basis rather than creating a duplicate. Works for both initial imports and backfill of existing inventory. DH auto-prices using pricing rules configured in the DH vendor profile and syncs to connected eBay/Shopify channels via background jobs.

**Limits:** Max 500 items per request.

**Request:**
```json
{
  "items": [
    {
      "dh_card_id": 51942,
      "cert_number": "12345678",
      "grading_company": "psa",
      "grade": 9.0,
      "cost_basis_cents": 5000
    }
  ]
}
```

**Response:**
```json
{
  "results": [
    {
      "dh_inventory_id": 98765,
      "cert_number": "12345678",
      "status": "active",
      "assigned_price_cents": 7500,
      "channels": [
        { "name": "ebay", "status": "active" },
        { "name": "shopify", "status": "pending" }
      ]
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `dh_inventory_id` | integer | Unique inventory item ID |
| `cert_number` | string | Echoed back for correlation |
| `status` | string | `active`, `pending`, or `failed` |
| `assigned_price_cents` | integer | Price set by auto-pricing (null if still pending) |
| `channels` | array | Per-channel sync status objects |
| `error` | string | Error message (only present if `failed`) |

Channel status values: `pending` (sync queued), `active` (live on channel), `error` (sync failed).

#### List Inventory

`GET /inventory`

| Param | Type | Description |
|-------|------|-------------|
| `status` | string | Filter: `active`, `sold`, `delisted`, `pending` |
| `cert_number` | string | Filter by specific cert |
| `updated_since` | string | ISO 8601 timestamp for incremental sync |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Results per page (default: 25, max: 100) |

**Response (per item):**

| Field | Type | Description |
|-------|------|-------------|
| `dh_inventory_id` | integer | Inventory item ID |
| `dh_card_id` | integer | DH card ID |
| `cert_number` | string | PSA cert number |
| `card_name` | string | Card name |
| `set_name` | string | Set name |
| `card_number` | string | Card number |
| `grading_company` | string | Grading company |
| `grade` | number | Grade |
| `status` | string | `active`, `sold`, `delisted`, `pending` |
| `listing_price_cents` | integer | Current listing price |
| `cost_basis_cents` | integer | Cost basis we provided |
| `channels` | array | Per-channel status objects |
| `created_at` | string | ISO 8601 |
| `updated_at` | string | ISO 8601 |

#### Update Item

`PATCH /inventory/:dh_inventory_id` — Update cost basis. Triggers re-price if pricing rules are cost-based.

#### Delist Item

`DELETE /inventory/:dh_inventory_id` — Remove from all channels.

### 3. Orders (Sales Feed)

`GET /orders`

Unified sales feed across DH marketplace, eBay, and Shopify.

| Param | Type | Description |
|-------|------|-------------|
| `since` | string | ISO 8601 timestamp (required) |
| `channel` | string | Filter: `dh`, `ebay`, `shopify` (optional) |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Results per page (default: 25, max: 100) |

**Response (using DH's field names):**
```json
{
  "orders": [
    {
      "order_id": "dh-12345",
      "cert_number": "12345678",
      "dh_card_id": 51942,
      "card_name": "Charizard",
      "set_name": "Base Set",
      "grade": 9,
      "sale_price_cents": 7500,
      "channel": "ebay",
      "fees": {
        "channel_fee_cents": 994,
        "commission_cents": null
      },
      "net_amount_cents": 6506,
      "sold_at": "2026-04-02T14:30:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 25,
    "total_count": 3
  }
}
```

**Field naming:** We adopt DH's field names directly — `order_id` (not `dh_order_id`), `sale_price_cents`, `sold_at` (not `sale_date`). Our Go types will mirror these names.

**Note:** Fee fields are `null` when exact amounts aren't available from the channel. `net_amount_cents` is only populated when all fee components are known.

**Requested addition:** Include `grading_company` on each order for completeness, even though PSA-only for now.

## DH Proposal Reconciliation

Discrepancies between our request and DH's confirmed proposal, with resolutions:

| # | Issue | Our Request | DH Proposal | Resolution |
|---|---|---|---|---|
| 1 | **Monetary values** | Mixed — `cost_basis_cents` (cents) but `sale_price`, `listing_price` etc. (dollars) | All values in cents with `_cents` suffix | **Use cents everywhere.** Aligns with our internal convention. Client converts to dollars at the API boundary for display. |
| 2 | **`dh_inventory_id` type** | String (`"inv_98765"`) | Integer | **Use integer.** Update our domain fields and DB columns to integer type. |
| 3 | **Channels format** | Simple array (`["ebay", "shopify"]`) | Structured array (`[{ "name": "ebay", "status": "active" }]`) | **Use structured format.** More useful — gives per-channel sync visibility. |
| 4 | **Orders `status` filter** | Query param defaulting to `completed` | Not supported — always returns completed sales | **Drop the param.** We only need completed sales anyway. |
| 5 | **Orders field naming** | `dh_order_id`, `sale_price`, `sale_date` | `order_id`, `sale_price_cents`, `sold_at` | **Use DH's field names directly.** No mapping — our Go types will use `order_id`, `sale_price_cents`, `sold_at` to match the API. |

### Suggestions & Match Migration

DH confirmed they are actively building enterprise versions of the suggestions and match endpoints. Once available, we can migrate those calls and fully retire the integration API client code.

## Migration: Integration API → Enterprise API

The existing DH client (`internal/adapters/clients/dh/client.go`) currently uses the **integration API** (`/api/v1/integrations/`). As part of v2, all calls should migrate to the **enterprise API** (`/api/v1/enterprise/`).

### Endpoint Mapping

| Current (Integration API) | Enterprise API Equivalent | Notes |
|---|---|---|
| `GET /api/v1/integrations/catalog/search` | `GET /search` | Query param changes: `q` → `query`, `limit` → `per_page` |
| `POST /api/v1/integrations/match` | `POST /enterprise/certs/resolve` (single) or `POST /enterprise/certs/resolve_batch` (batch) | Title/SKU matching replaced by cert-based resolution (sync for single lookups, async for batches). Match endpoint may still be useful as a fallback for non-cert scenarios — confirm with DH if it will exist on enterprise. |
| `GET /api/v1/integrations/market_data/{id}?tier=tier3` | `GET /cards/{cardId}/insights` | Enterprise returns a unified insights package (AI summary, forecast, sentiment, population, grading ROI) instead of the tiered market data response. Response schema changes significantly. |
| `GET /api/v1/integrations/suggestions` | **Not in enterprise spec** | Need to confirm with DH whether suggestions will be available on enterprise, or if this stays on the integration API. |

### Auth Change

| | Integration API | Enterprise API |
|---|---|---|
| Header | `X-Integration-API-Key: <key>` | `Authorization: Bearer <key>` |
| Rate limit | Self-imposed 1 RPS | 100 req/min, 1,000 req/hour (server-enforced) |

### Client Changes

- Update `apiKeyHeader` constant from `"X-Integration-API-Key"` to `"Authorization"` with `"Bearer "` prefix
- Update `baseURL` to point to enterprise endpoint
- Update `get`/`post` helpers to use Bearer auth
- Update rate limiter to align with enterprise limits (100 req/min instead of 1 RPS)
- Refactor `Search` to map to enterprise search params and response schema
- Refactor `MarketData` to consume the unified `CardInsightsResponse` instead of tiered market data
- Replace `Match` usage with `ResolveCert` (single) or `ResolveCertsBatch` (batch) where cert numbers are available
- Update `convert.go` to handle new response schemas for intelligence storage
- Confirm with DH: will `suggestions` and `match` endpoints be available on enterprise?

### Migration Strategy

This can be done as a separate task from the new v2 inventory/orders integration. The existing integration API presumably continues working during the transition. Recommended order:

1. Add new v2 client methods (cert resolution, inventory, orders) against enterprise API
2. Migrate existing methods (search, market data) to enterprise equivalents
3. Confirm suggestions endpoint availability, migrate or keep on integration API
4. Remove integration API auth code once fully migrated

## SlabLedger Integration Architecture

### Enhanced PSA Import Flow

After parsing the PSA sheet and creating purchases, the system submits all certs via `ResolveCertsBatch` (up to 500 per job). A typical 200-cert PSA sheet fits in a single batch submission. The system then polls `GetCertResolutionJob` for results.

**Polling strategy:** Poll every 3 seconds, max 2-minute timeout. Partial results are available during processing, enabling progressive UI updates (e.g., "Resolving certs... 150/200 matched"). On timeout, store whatever partial results are available and flag remaining certs as `unresolved`.

Matched certs enrich purchases with the DH card ID and canonical card metadata. This replaces the async CardHedger discovery as the primary card identity source for newly imported cards.

**Field handling:** When DH returns a match, the canonical DH card metadata (`card_name`, `set_name`, `card_number`) is written to the existing Purchase fields, overwriting the PSA sheet's title-parsed values. The raw PSA listing title is preserved in `PSAListingTitle` for reference. The `DHCardID` field stores the DH card identity for inventory push.

Ambiguous results are stored with their candidates so the user can disambiguate via UI.

### Push to DoubleHolo Action

A new handler/service method that takes unsold purchases with resolved DH card IDs, maps them to inventory items (cert, DH card ID, grade, cost basis), and calls `POST /inventory` (upsert).

Triggered:
- Automatically after PSA import (for new matched cards)
- Manually via UI action (for backfill of existing inventory)

The single upsert endpoint handles both cases — no need for separate create vs sync logic.

### Reconciliation Flow

For certs that returned `ambiguous` or `not_found`:
1. User disambiguates candidates (for ambiguous) or enriches card metadata (for not_found) via Card Ladder data, manual input, or other sources
2. Re-submit individually via `ResolveCert` (sync endpoint) with hints (card name, set name, card number, year, variant, language), or in bulk via `ResolveCertsBatch` (async endpoint)
3. Successfully resolved certs can then be pushed to inventory

### New DH Client Methods

Extending `internal/adapters/clients/dh/client.go`:
- `ResolveCert(ctx, CertResolveRequest) → CertResolution` — Sync single-cert resolution
- `ResolveCertsBatch(ctx, []CertResolveRequest) → CertResolutionJob` — Submit async batch (up to 500), returns job ID
- `GetCertResolutionJob(ctx, jobID string) → CertResolutionJobStatus` — Poll batch job for partial/complete results
- `PushInventory(ctx, []InventoryItem) → []InventoryResult`
- `ListInventory(ctx, InventoryFilters) → InventoryPage`
- `UpdateInventory(ctx, id int, InventoryUpdate) → error`
- `DelistInventory(ctx, id int) → error`
- `GetOrders(ctx, OrderFilters) → OrdersPage`

### New Schedulers

**Inventory Status Poll** (`dh_inventory_poll.go`):
- Interval: configurable (e.g. every 2 hours)
- Calls `GET /inventory?status=active&updated_since=<checkpoint>`
- Updates local records with current listing prices, status, and per-channel sync state
- Surfaces DH pricing decisions alongside cost basis in the UI

**Orders Poll** (`dh_orders_poll.go`):
- Interval: configurable (e.g. every 30 minutes)
- Calls `GET /orders?since=<checkpoint>`
- Converts DH order responses to `OrdersExportRow` and feeds through `ImportOrdersSales` for cert matching and validation
- **Auto-confirms** matched orders via `ConfirmOrdersSales` — no user review step. DH is the authoritative sales source, so manual confirmation is unnecessary. Orders that fail matching (`NotFound`, `AlreadySold`) are logged as warnings for the user to review in the UI, not silently dropped.
- **Fee mapping:** DH provides `channel_fee_cents` and `commission_cents`, which may be `null`. Sum all non-null fee components into `SaleFeeCents`. When all fees are null, fall back to the campaign's configured `ebayFeePct` for eBay/TCGPlayer channels (0% for local/other), consistent with the existing manual import behavior.
- Stores `order_id` on sale record for idempotency
- Updates checkpoint on success

### Domain Changes

Minimal:
- Add `OrderID` field to `Sale` for idempotency (matches DH's `order_id` field)
- Add `DHInventoryID` (integer), `ListingPriceCents`, and `DHCardID` fields to `Purchase` for tracking
- Add `DHCertStatus` field to `Purchase` for tracking resolution state (`matched`, `ambiguous`, `not_found`, `unresolved`, `resolving`)
- Add `DHChannelsJSON` text field to `Purchase` — stores per-channel sync status as a JSON blob (e.g., `[{"name":"ebay","status":"active"}]`). Updated by the inventory status poll. Parsed at the API boundary for display.
- New unique constraint on `order_id` in sales table
- **SaleChannel mapping:** DH uses `"dh"`, `"ebay"`, `"shopify"`. Map to existing `SaleChannel` values: `"ebay"` → eBay, `"shopify"` → TCGPlayer (our Shopify is TCGPlayer Direct), `"dh"` → new `SaleChannelDoubleHolo` constant. Confirm the Shopify mapping assumption with the business — if DH's Shopify channel is a separate storefront, it may need its own channel with its own fee structure.
- The existing order import / sale recording logic (`ImportOrdersSales`, `ConfirmOrdersSales`) is reused for cert matching and validation. The orders poll auto-confirms matched orders (see Orders Poll section above).

## Error Handling

**Cert resolution failures:** Unresolved certs still create purchases in SlabLedger with title-parsed metadata. They are flagged with `DHCertStatus` for reconciliation but do not block the import.

**Batch job failures:** If `resolve_batch` submission fails, all certs in the batch are flagged as `unresolved`. If polling times out (2 minutes), any partial results received are applied and remaining certs are flagged `unresolved`. If the job returns `failed` status, all certs without results are flagged `unresolved`. In all cases, the PSA import succeeds — cert resolution is best-effort enrichment.

**Ambiguous certs:** Candidates stored locally. User can select the correct match via UI, then re-submit via sync endpoint or directly push to inventory with the chosen DH card ID.

**Inventory push failures:** Per-item status reporting. Partial success is expected and handled — same pattern as existing PSA import results. Per-channel sync status lets us show granular progress.

**Order polling idempotency:** DH `order_id` values stored on sale records with unique constraint. Overlapping poll windows or retries cannot create duplicate sales.

**Polling state:** High-water mark timestamp stored in the existing `sync_state` table via the `SyncStateStore` interface (same mechanism used by the CardHedger refresh scheduler). Each poll cycle: fetch → match → record → update checkpoint.

**Reconciliation:** Periodic `GET /inventory` call verifies SlabLedger's view matches DH's view. Discrepancies (missed sales, manual delists, price changes) are flagged for the user.

**PSA API key not configured:** Cert resolution returns an error. The PSA import still works (creates purchases from CSV data) but without DH card matching. User prompted to configure their PSA API key in their DH account.

## Out of Scope

- **Webhooks** — Polling is sufficient. Can be added later if latency matters.
- **Pricing override** — We send cost basis; DH handles pricing via vendor profile rules. No mechanism to set list prices ourselves.
- **Listing customization** — No titles, descriptions, or photos from our side. DH generates everything.
- **Real-time price change notifications** — Inventory poll covers this at an acceptable frequency.
- **Non-PSA grading companies** — Cert resolution is PSA-only for now. BGS/CGC cards that sell through DH would still appear in the orders poll.

Existing v1 endpoints (insights, market data, suggestions, search, match) remain unchanged.
