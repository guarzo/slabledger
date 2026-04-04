# DoubleHolo Enterprise API v2 — Requested Endpoints

## Context

We're currently using the DoubleHolo Enterprise API v1 for market intelligence — card insights, sentiment, forecasts, grading ROI, and suggestions. This has been excellent for pricing decisions and portfolio analytics.

For v2, we'd like to request endpoints that would let us build a streamlined inventory-to-sale pipeline through DoubleHolo, taking advantage of your Shopify and eBay integrations.

## Current Workflow (What We Want to Simplify)

Today our users go through a multi-step process to get PSA-graded cards listed for sale:

1. Upload PSA communication sheet (cert numbers, grades, prices paid)
2. Export a file formatted for a third-party listing tool
3. Import into that tool for pricing and listing generation
4. Export marketplace-specific files from that tool
5. Import into Shopify/eBay

Sales recording is similarly manual — export order CSVs from Shopify, import back into our system.

## Proposed Workflow (With DoubleHolo v2)

1. Upload PSA sheet into our system
2. Batch-resolve cert numbers via DoubleHolo to get card identity
3. Push inventory to DoubleHolo with cost basis — DoubleHolo auto-prices and syncs to Shopify/eBay
4. Poll DoubleHolo for completed sales to automatically record them

This eliminates the intermediate listing tool entirely and automates sales recording.

## Requested Endpoints

### 1. Batch Cert Resolution

`POST /certs/resolve`

Resolve PSA certification numbers to DoubleHolo card identities. We'd send cert numbers immediately after a PSA sheet upload, and optionally re-submit unmatched certs later with additional identifying information to improve match rates.

**Request:**
```json
{
  "certs": [
    {
      "cert_number": "12345678"
    },
    {
      "cert_number": "87654321",
      "card_name": "Charizard",
      "set_name": "Base Set",
      "card_number": "4/102"
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cert_number` | string | yes | PSA certification number |
| `card_name` | string | no | Hint to improve matching |
| `set_name` | string | no | Hint to improve matching |
| `card_number` | string | no | Hint to improve matching |

**Expected response (per cert):**

| Field | Type | Description |
|-------|------|-------------|
| `cert_number` | string | Echo back for correlation |
| `status` | string | `matched`, `not_found`, or `ambiguous` |
| `dh_card_id` | integer | DoubleHolo card ID (when matched) |
| `card_name` | string | Canonical card name |
| `set_name` | string | Canonical set name |
| `card_number` | string | Card number within set |
| `grade` | number | Grade from PSA cert data |
| `image_url` | string | Card image URL |
| `current_market_price` | number | Current market price (USD) |

**Usage patterns:**
- **Initial import:** Cert number only (PSA sheets don't include structured card metadata)
- **Reconciliation:** Re-submit previously unmatched certs with card name, set name, and card number gathered from other sources

**Volume:** Typical batch is 50-300 certs. A limit of 500 per request would be sufficient.

### 2. Batch Inventory Creation

`POST /inventory`

Create listings on DoubleHolo from resolved cards. We provide the card identity, cert, grade, and our cost basis. DoubleHolo handles pricing and syncs to Shopify/eBay.

**Request:**
```json
{
  "items": [
    {
      "dh_card_id": 12345,
      "cert_number": "12345678",
      "grading_company": "psa",
      "grade": 9.0,
      "cost_basis_cents": 5000
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dh_card_id` | integer | yes | DoubleHolo card ID (from cert resolution) |
| `cert_number` | string | yes | PSA certification number |
| `grading_company` | string | yes | Grading company (e.g. `psa`) |
| `grade` | number | yes | Numeric grade |
| `cost_basis_cents` | integer | yes | Our cost basis in cents (for pricing decisions) |

**Expected response (per item):**

| Field | Type | Description |
|-------|------|-------------|
| `dh_inventory_id` | string | DoubleHolo inventory item ID |
| `cert_number` | string | Echo back for correlation |
| `status` | string | `active`, `pending`, or `failed` |
| `assigned_price` | number | Price DoubleHolo set for the listing (USD) |
| `channels` | array | Channels listed on (e.g. `["ebay", "shopify"]`) |
| `error` | string | Error message if failed |

### 3. Inventory Sync

`POST /inventory/sync`

Same request and response shape as batch inventory creation, but with upsert semantics. If a cert already exists in DoubleHolo inventory, update its cost basis rather than creating a duplicate.

We'd use this for:
- Pushing existing unsold inventory that predates the integration (backfill)
- Re-syncing after cost basis adjustments

### 4. Inventory List

`GET /inventory`

Retrieve current inventory with status and pricing. We'd use this both for on-demand reconciliation and for periodic polling to track DoubleHolo's auto-pricing decisions.

**Query parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `status` | string | Filter: `active`, `sold`, `delisted`, `pending` |
| `cert_number` | string | Filter by specific cert |
| `updated_since` | string | ISO 8601 timestamp for incremental sync |
| `page` | integer | Page number |
| `per_page` | integer | Results per page |

**Expected response (per item):**

| Field | Type | Description |
|-------|------|-------------|
| `dh_inventory_id` | string | DoubleHolo inventory item ID |
| `dh_card_id` | integer | DoubleHolo card ID |
| `cert_number` | string | PSA cert number |
| `card_name` | string | Card name |
| `set_name` | string | Set name |
| `card_number` | string | Card number |
| `grading_company` | string | Grading company |
| `grade` | number | Grade |
| `status` | string | `active`, `sold`, `delisted`, `pending` |
| `listing_price` | number | Current listing price (USD) |
| `cost_basis_cents` | integer | Cost basis we provided |
| `channels` | array | Where it's listed |
| `created_at` | string | ISO 8601 timestamp |
| `updated_at` | string | ISO 8601 timestamp |

### 5. Inventory Update / Delist

`PATCH /inventory/{inventoryId}` — Update cost basis or request re-pricing.

`DELETE /inventory/{inventoryId}` — Remove item from all channels.

### 6. Orders

`GET /orders`

Retrieve completed sales. We'd poll this on a schedule to automatically record sales in our system.

**Query parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `since` | string | ISO 8601 timestamp (required) |
| `status` | string | Default: `completed` |
| `page` | integer | Page number |
| `per_page` | integer | Results per page |

**Expected response (per order):**

| Field | Type | Description |
|-------|------|-------------|
| `dh_order_id` | string | Unique order identifier |
| `cert_number` | string | PSA cert number of the sold item |
| `sale_price` | number | Sale price (USD) |
| `sale_date` | string | ISO 8601 timestamp |
| `channel` | string | Channel where it sold (e.g. `ebay`, `shopify`) |
| `fees` | object | Breakdown of channel fees and commissions |
| `fees.channel_fee` | number | Marketplace fee (USD) |
| `fees.commission` | number | Any additional commission (USD) |
| `net_amount` | number | Sale price minus all fees (USD) |

## What We Don't Need

- **Webhooks** — Scheduled polling for orders and inventory status is sufficient for our use case.
- **Pricing control** — We trust DoubleHolo's auto-pricing. We just need to send our cost basis.
- **Listing customization** — No need to send titles, descriptions, or photos. DoubleHolo's card database covers this.
- **Real-time notifications** — Periodic polling handles our needs.

## Volume Estimates

- Typical PSA sheet: 50-300 certs
- Inventory pushes: same volume, a few times per month
- Inventory status polls: a few times per day
- Order polls: several times per day
- Total active inventory: low thousands at peak

## Migrating Existing Integration API Calls

We currently use the following endpoints on the **integration API** (`/api/v1/integrations/`):

| Endpoint | What we use it for |
|---|---|
| `GET /api/v1/integrations/catalog/search` | Card search for discovery and matching |
| `POST /api/v1/integrations/match` | AI-powered title/SKU matching to resolve card identity |
| `GET /api/v1/integrations/market_data/{id}?tier=tier3` | Market intelligence (sentiment, forecast, population, ROI, recent sales) |
| `GET /api/v1/integrations/suggestions` | Daily AI-generated buy/sell suggestions |

We'd like to consolidate everything onto the enterprise API. The enterprise v1 spec already covers equivalents for search (`GET /search`) and market data (`GET /cards/{cardId}/insights`), so we can migrate those.

**Questions:**
- Will the **suggestions** endpoint be available on the enterprise API? We poll this every 6 hours to surface buy/sell recommendations.
- Will the **match** endpoint (title/SKU → card identity) be available on enterprise? With cert resolution this becomes less critical, but it's still useful as a fallback for scenarios where we don't have a cert number (e.g. manual card lookup).

## Questions for DoubleHolo

1. Does the cert resolution approach align with your existing PSA cert lookup capabilities?
2. For auto-pricing — do you need anything beyond our cost basis to make pricing decisions? (e.g. target margin percentage, minimum price floor)
3. Are there any constraints on your Shopify/eBay sync that would affect the inventory creation flow? (e.g. required fields, sync delays)
4. For the orders endpoint — would you include fees inline, or is there a separate endpoint for fee/commission details?
5. What authentication scheme are you planning for v2? (We currently use `X-Integration-API-Key` header with v1)
6. Will the **suggestions** and **match** endpoints be available on the enterprise API, or will they remain on the integration API only?
