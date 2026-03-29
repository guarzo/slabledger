# Card Ladder API Integration — Design Spec

**Date**: 2026-03-29
**Status**: Approved
**Phases**: Phase 1 (Value Refresh), Phase 2 (Sales Comps)
**Future**: Phase 3 (Fusion Engine Integration)

## Context

Card Ladder is the most accurate pricing source for PSA-graded cards in the portfolio:

| Source | Avg deviation from reviewed prices | Coverage |
|--------|-----------------------------------|----------|
| Card Ladder | **9.3%** | 100% (via CSV import) |
| CardHedger (with mapping) | 31.5% | 59% |

Currently CL values are imported manually via CSV export/upload. CL exposes an internal JSON API (Cloud Run search service) that we can automate against. CL support has confirmed this is allowed within their ToS for collection items, with a future API migration path when they make one available.

## API Discovery

Card Ladder is a Firebase-based SPA. Two Cloud Run search endpoints power the data we need:

### Collection Cards Endpoint
```
GET https://search-zzvl7ri3bq-uc.a.run.app/search
  ?index=collectioncards
  &query=
  &page=0
  &limit=100
  &filters=collectionId:{collectionId}|hasQuantityAvailable:true
  &sort=player
  &direction=asc
Authorization: Bearer {firebase_id_token}
```

Returns collection cards with: `currentValue`, `investment`, `profit`, `condition`, `player`, `set`, `number`, `year`, `variation`, `label`, `image`, `imageBack`, `collectionCardId`, `weeklyPercentChange`, `monthlyPercentChange`.

### Sales Archive Endpoint
```
GET https://search-zzvl7ri3bq-uc.a.run.app/search
  ?index=salesarchive
  &query=
  &limit=100
  &filters=condition:{grade}|gemRateId:{gemRateId}|gradingCompany:psa
  &sort=date
Authorization: Bearer {firebase_id_token}
```

Returns sold listings with: `price`, `date`, `platform` (eBay, Fanatics-Vault, Fanatics-Weekly), `listingType` (Auction, BestOffer, FixedPrice), `seller`, `feedback`, `slabSerial`, `url`, `itemId`, `cardDescription`, `gemRateId`, `condition`.

### Authentication

Firebase Auth email/password flow:
1. POST `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key={apiKey}` with email + password → returns `idToken` (1hr TTL) + `refreshToken`
2. Token refresh: POST `https://securetoken.googleapis.com/v1/token?key={apiKey}` with `grant_type=refresh_token&refresh_token={token}` → new `idToken`
3. `idToken` is passed as `Authorization: Bearer {idToken}` on all search requests

## Architecture

### New Package: `internal/adapters/clients/cardladder/`

Follows the hexagonal pattern established by CardHedger:

- `client.go` — HTTP client wrapping `httpx.Client`. Token management with auto-refresh on expiry. Rate limiter at 1 req/sec.
- `auth.go` — Firebase Auth login + token refresh logic. Stores token expiry time, refreshes proactively before expiry.
- `types.go` — Request/response structs: `CollectionCard`, `SaleComp`, `SearchResponse`, `AuthTokens`.
- `client_test.go` — Tests with `httptest.NewServer`.

### Card-to-Purchase Mapping

CL collection cards don't return an explicit cert number field. Mapping strategy:

1. **Primary**: Match CL `image` field to `campaign_purchases.front_image_url`. Both reference the same PSA cert image CDN (`d1htnxwo4o0jhw.cloudfront.net/cert/...`) — exact URL match.
2. **Fallback**: Extract cert number from CL image URL path pattern `/cert/{number}/`, match to `campaign_purchases.slab_serial`.
3. **Cache**: Store mappings in `cl_card_mappings` table so subsequent syncs are instant lookups. This table also stores `gemRateId` needed for Phase 2 sales comp queries.

First sync builds the mapping table; subsequent syncs are value refreshes only.

## Schema Changes

### Migration: `cardladder_config` (singleton)

```sql
CREATE TABLE IF NOT EXISTS cardladder_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    encrypted_refresh_token TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    firebase_api_key TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Migration: `cl_card_mappings`

```sql
CREATE TABLE IF NOT EXISTS cl_card_mappings (
    slab_serial TEXT PRIMARY KEY,
    cl_collection_card_id TEXT NOT NULL,
    cl_gem_rate_id TEXT NOT NULL DEFAULT '',
    cl_condition TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Migration (Phase 2): `cl_sales_comps`

```sql
CREATE TABLE IF NOT EXISTS cl_sales_comps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    gem_rate_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    sale_date DATE NOT NULL,
    price_cents INTEGER NOT NULL,
    platform TEXT NOT NULL,
    listing_type TEXT NOT NULL DEFAULT '',
    seller TEXT NOT NULL DEFAULT '',
    item_url TEXT NOT NULL DEFAULT '',
    slab_serial TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_sales_comps_item
    ON cl_sales_comps(gem_rate_id, item_id);
CREATE INDEX IF NOT EXISTS idx_cl_sales_comps_gem_rate
    ON cl_sales_comps(gem_rate_id, sale_date DESC);
```

Existing tables unchanged — writes to `campaign_purchases.cl_value_cents` and `cl_value_history` exactly as the CSV import does.

## Scheduler

### `CardLadderRefreshScheduler`

- **Frequency**: Once daily, configurable via `CARDLADDER_REFRESH_HOUR` env var (default: 4 AM, after CardHedger batch).
- **Manual trigger**: `POST /api/admin/cardladder/refresh`.

**Refresh flow**:
1. Load `cardladder_config`; skip if not configured.
2. Refresh Firebase token if expired.
3. Paginate through `collectioncards` index (all collection items).
4. First run or empty mappings: build `cl_card_mappings` by matching image URLs to `campaign_purchases.front_image_url`.
5. For each matched card: update `cl_value_cents` on the purchase, record to `cl_value_history` with `source = 'api_sync'`.
6. **(Phase 2)**: For cards with stale sales data (>24h), fetch `salesarchive` by `gemRateId`, upsert into `cl_sales_comps`.

**Performance**: ~289 active purchases. Collection fetch: ~3-5 paginated requests. Phase 2 sales comps: ~289 requests at 1 req/sec ≈ 5 minutes.

**Rate limiting**: 1 req/sec via `rate.Limiter`. Circuit breaker on repeated failures. 429 tracking via `api_rate_limits` table.

## Admin Endpoints

### `POST /api/admin/cardladder/config`

One-time setup. Accepts `email`, `password`, `collectionId`, `firebaseApiKey`. Authenticates with Firebase, stores refresh token (encrypted with existing AES `ENCRYPTION_KEY`), discards password.

### `POST /api/admin/cardladder/refresh`

Manual sync trigger. Runs the same flow as the scheduler.

### `GET /api/admin/cardladder/status`

Returns: configured (bool), last sync time, cards mapped, last error.

## Phase 2: Sales Comps

### Data Capture

For each card, store the last 90 days of sales from `salesarchive`. Deduplicate by `item_id`. On each refresh, fetch new sales since the most recent stored sale date per `gem_rate_id`.

### API Exposure

`GET /api/purchases/{id}/sales-comps` — returns recent comps for a purchase:
- Recent sold prices with date, platform, listing type
- Supports pricing negotiations with actual market data

### Analytics Derived from Comps

- Median/mean recent sale price (our own calculation alongside CL's value)
- Price trend (30-day direction)
- Platform breakdown (eBay auction vs fixed price vs Fanatics)
- Velocity (sales per week — indicates liquidity)

### Sell Sheet Enrichment

The sell sheet (`service_sell_sheet.go`) currently uses `cl_value_cents` as a pricing floor. With comps available, surface "last 5 sold prices" alongside the CL value for fuller negotiation context.

## Future: Phase 3 — Fusion Engine Integration

CL sales comp data could be wired into the fusion engine as a `SecondaryPriceSource`, contributing to fused price calculations alongside CardHedger. This was deferred because CL values already dominate reviewed prices (74% are set directly to CL value). Feeding CL into fusion risks over-indexing on one source. Revisit once Phase 2 is stable and the value of blending CL comps with CardHedger data is better understood.

## CSV Import Compatibility

CSV import (`POST /api/purchases/refresh-cl`, `POST /api/purchases/import-cl`) remains unchanged:
- Functions as bootstrap for initial data load and override/correction.
- If both CSV and API write values for the same cert on the same day, the last write wins (existing upsert on `cl_value_history`).
- API sync uses `source = 'api_sync'`; CSV uses `source = 'csv_import'` — distinguishable in history.

## Configuration

New environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CARDLADDER_REFRESH_ENABLED` | `false` | Enable the CL refresh scheduler |
| `CARDLADDER_REFRESH_HOUR` | `4` | Hour (UTC) to run daily refresh |

Firebase credentials and collection ID are stored in the `cardladder_config` DB table (not env vars) since they're set up via the admin endpoint.

## Dependencies

- No new Go dependencies required. Firebase Auth is a simple REST API. HTML parsing (`goquery`) is NOT needed — this is a pure JSON API integration.
- Uses existing `httpx.Client` for retry + circuit breaker.
- Uses existing `platform/crypto` for refresh token encryption.
