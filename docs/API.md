# SlabLedger API Reference

Base URL: `http://localhost:8081`

## Authentication

All auth middleware accepts either a session cookie (`session_id`) set by the OAuth flow, or an `Authorization: Bearer <LOCAL_API_TOKEN>` header when `LOCAL_API_TOKEN` is configured.

**Auth levels used in this document:**

| Level | Requirement |
|---|---|
| None | No authentication required |
| RequireAuth | Valid session cookie or Bearer token |
| RequireAdmin | RequireAuth + `is_admin` flag on user |
| RequireAPIKey | `Authorization: Bearer <PRICING_API_KEY>` header |

Rate limits: auth endpoints are limited to 10 req/s. The pricing API v1 is limited to 60 req/min.

---

## Authentication

### `GET /auth/google/login`

Auth: None

Initiates Google OAuth flow. Generates a CSRF state token, sets `oauth_state` cookie, and redirects to Google.

**Response:** `302 Found` → Google authorization URL

---

### `GET /auth/google/callback`

Auth: None (public OAuth redirect target)

Handles Google OAuth callback. Validates state, exchanges code for tokens, creates/retrieves user, sets `session_id` cookie.

**Query params:** `code` (string), `state` (string)

**Response:** `302 Found` → `/` on success, `/login?error=not_authorized` if not on allowlist

**Errors:** `400` invalid state / missing code; `302` → `/?error=oauth_failed` on provider errors

---

### `POST /api/auth/logout`

Auth: None (clears session)

Deletes the server-side session and clears the `session_id` cookie.

**Response:** `200 OK`
```json
{ "message": "Logged out successfully" }
```

---

### `GET /api/auth/user`

Auth: RequireAuth

Returns the currently authenticated user.

**Response:** `200 OK`
```json
{
  "id": 1,
  "username": "Jane Doe",
  "email": "jane@example.com",
  "avatar_url": "https://...",
  "is_admin": false,
  "last_login_at": "2025-01-01T00:00:00Z"
}
```

---

## Health & Status

### `GET /api/health`

Auth: None

Returns overall system health.

**Response:** `200 OK` (healthy) or `503 Service Unavailable` (degraded)
```json
{
  "status": "healthy",
  "timestamp": "2025-01-01T00:00:00Z",
  "providers": { "prices": true },
  "database": "healthy"
}
```

`database` is `"healthy"`, `"unhealthy: <reason>"`, or `"not configured"` when no
health checker is wired. Anything other than `"healthy"` — except `"not
configured"` — makes `status` `"degraded"` and the code `503`.

**Errors:** `405` method other than GET or HEAD

---

### `GET /api/v1/health`

Auth: None (Pricing API)

Returns pricing API status.

**Response:** `200 OK`
```json
{ "status": "ok", "version": "1.0.0" }
```

---

### `GET /api/admin/api-usage`

Auth: RequireAdmin

Returns API call statistics for all pricing providers.

**Response:** `200 OK`
```json
{
  "providers": [
    {
      "name": "doubleholo",
      "today": {
        "calls": 50,
        "limit": 1000,
        "remaining": 950,
        "successRate": 98.0,
        "avgLatencyMs": 220.5,
        "rateLimitHits": 0
      },
      "blocked": false,
      "lastCallAt": "2025-01-01T00:00:00Z"
    }
  ],
  "timestamp": "2025-01-01T00:00:00Z"
}
```

---

### `GET /api/admin/ai-usage`

Auth: RequireAdmin

Returns AI call usage statistics broken down by operation.

**Response:** `200 OK`
```json
{
  "configured": true,
  "summary": {
    "totalCalls": 42,
    "successRate": 97.6,
    "totalInputTokens": 100000,
    "totalOutputTokens": 20000,
    "totalTokens": 120000,
    "avgLatencyMs": 4500.0,
    "rateLimitHits": 1,
    "callsLast24h": 10,
    "lastCallAt": "2025-01-01T00:00:00Z",
    "totalCostCents": 240
  },
  "operations": [
    {
      "operation": "digest",
      "calls": 5,
      "errors": 0,
      "successRate": 100.0,
      "avgLatencyMs": 5200.0,
      "totalTokens": 30000,
      "totalCostCents": 90
    }
  ],
  "timestamp": "2025-01-01T00:00:00Z"
}
```

---

## Card Catalog

### `GET /api/cards/catalog`

Auth: RequireAuth

Searches the Card Ladder card catalog by query string.

**Query params:**
- `q` (string, required) — search query (card name, set, player, etc.)
- `limit` (int, optional, default 20, max 100) — results per page
- `page` (int, optional, default 0) — zero-based page index
- `condition` (string, optional) — filter by condition
- `set` (string, optional) — filter by set name
- `gradingCompany` (string, optional) — filter by grading company (e.g. `PSA`)
- `category` (string, optional) — filter by card category

**Response:** `200 OK`
```json
{
  "hits": [
    {
      "id": "cl-id-123",
      "gemRateId": "gem-123",
      "psaSpecId": 45678,
      "label": "Charizard",
      "player": "",
      "playerIndexId": "",
      "set": "Base Set",
      "year": "1999",
      "number": "4",
      "variation": "",
      "category": "Pokemon",
      "condition": "10",
      "gradingCompany": "PSA",
      "currentValue": 1200.00,
      "marketValue": 1195.00,
      "pop": 1420,
      "numSales": 12,
      "marketCap": 1704000.00,
      "score": 0.95,
      "weeklyPercentChange": 0.4,
      "monthlyPercentChange": -1.2,
      "quarterlyPercentChange": 3.8,
      "annualPercentChange": 11.5,
      "priceMovement": 5.0,
      "lastSoldDate": "2025-04-10",
      "slug": "1999-base-set-4-charizard",
      "keyCard": true,
      "image": "https://...",
      "ebayQuery": "1999 Pokemon Base Set Charizard PSA 10"
    }
  ],
  "totalHits": 1
}
```

**Errors:** `400` missing `q`; `500` search failed

`pop` and `marketCap` are nullable. `currentValue`, `marketValue`, and
`marketCap` are Card Ladder values in **dollars**, not cents — this endpoint
proxies the CL catalog rather than SlabLedger's own inventory.

---

## Price Hints

### `GET /api/price-hints`

Auth: RequireAdmin

Lists all saved price hints (manual provider ID overrides).

**Response:** `200 OK` — Array of `priceHintResponse`
```json
[
  {
    "cardName": "Charizard",
    "setName": "Base Set",
    "cardNumber": "4",
    "provider": "doubleholo",
    "externalId": "12345"
  }
]
```

---

### `POST /api/price-hints`

Auth: RequireAdmin

Saves a price hint (upserts).

**Body:**
```json
{
  "cardName": "Charizard",
  "setName": "Base Set",
  "cardNumber": "4",
  "provider": "doubleholo",
  "externalId": "12345"
}
```
`provider` must be `"doubleholo"`.

**Response:** `200 OK` — `{ "status": "ok" }`

**Errors:** `400` missing fields or invalid provider

---

### `DELETE /api/price-hints`

Auth: RequireAdmin

Deletes a price hint.

**Body:** Same as POST (minus `externalId`). All of `cardName`, `setName`, `cardNumber`, `provider` required.

**Response:** `200 OK` — `{ "status": "ok" }`

---

## Admin

### `GET /api/admin/allowlist`

Auth: RequireAdmin

Lists all email addresses in the access allowlist.

**Response:** `200 OK` — Array of `auth.AllowedEmail`

---

### `POST /api/admin/allowlist`

Auth: RequireAdmin

Adds an email address to the access allowlist.

**Body:**
```json
{ "email": "user@example.com", "notes": "Authorized by admin" }
```

**Response:** `201 Created` — `{ "status": "ok" }`

**Errors:** `400` invalid email format

---

### `DELETE /api/admin/allowlist/{email}`

Auth: RequireAdmin

Removes an email address from the allowlist.

**Path params:** `email` (URL-encoded email address)

**Response:** `200 OK` — `{ "status": "ok" }`

---

### `GET /api/admin/users`

Auth: RequireAdmin

Lists all registered users.

**Response:** `200 OK` — Array of user objects
```json
[
  {
    "id": 1,
    "username": "Jane Doe",
    "email": "jane@example.com",
    "avatar_url": "https://...",
    "is_admin": true,
    "last_login_at": "2025-01-01T00:00:00Z"
  }
]
```

---

### `GET /api/admin/backup`

Auth: RequireAdmin

Streams a consistent SQLite backup as a downloadable file (`VACUUM INTO`).

**Response:** `200 OK` with `Content-Type: application/octet-stream`, `Content-Disposition: attachment; filename="slabledger-backup-YYYY-MM-DD.db"`

---

### `GET /api/admin/metrics`

Auth: RequireAdmin

Returns timing metrics for tracked endpoints (sell-sheet, insights, capital-timeline, etc.).

**Response:** `200 OK` — timing store data

---

### `GET /api/admin/dh-tombstones/count`

Auth: RequireAdmin

Returns how many DH card tombstones are recorded. A tombstone marks a cert DH could not resolve to a card, suppressing repeat lookups.

**Response:** `200 OK`
```json
{ "count": 128 }
```

**Errors:** `503` DH tombstones not configured; `500` count failed

---

### `POST /api/admin/dh-tombstones/clear`

Auth: RequireAdmin

Clears every DH card tombstone so suppressed certs are retried on the next lookup.

**Body:** (empty)

**Response:** `200 OK`
```json
{ "cleared": 128 }
```

**Errors:** `503` DH tombstones not configured; `500` clear failed

---

### `GET /api/admin/pricing-diagnostics`

Auth: RequireAdmin

Returns pricing data quality diagnostics (coverage rates, match quality distribution, etc.).

**Response:** `200 OK` — `pricing.PricingDiagnostics` object

---

### `GET /api/admin/price-override-stats`

Auth: RequireAdmin

Returns aggregate statistics about price overrides and pending AI suggestions.

**Response:** `200 OK` — `PriceOverrideStats`
```json
{
  "totalUnsold": 120,
  "overrideCount": 15,
  "manualCount": 8,
  "costMarkupCount": 4,
  "aiAcceptedCount": 3,
  "pendingSuggestions": 7,
  "overrideTotalUsd": 4500.00,
  "suggestionTotalUsd": 2100.00
}
```

---

## Campaign CRUD

### `GET /api/campaigns`

Auth: RequireAuth

Lists campaigns.

**Query params:** `activeOnly=true` (optional, filters to pending/active only)

**Response:** `200 OK` — Array of `Campaign`
```json
[
  {
    "id": "uuid",
    "name": "Pokemon Base 2025 Q1",
    "sport": "pokemon",
    "yearRange": "1999-2000",
    "gradeRange": "9-10",
    "priceRange": "50-500",
    "clConfidence": "2.5-4",
    "buyTermsCLPct": 0.7,
    "dailySpendCapCents": 1000000,
    "inclusionList": "",
    "exclusionMode": false,
    "targetLanguages": ["english"],
    "subjectFilterMode": "Target",
    "subjects": [{ "id": 22210, "name": "Machamp" }],
    "deniedSpecs": [],
    "phase": "active",
    "psaSourcingFeeCents": 300,
    "ebayFeePct": 0.1235,
    "expectedFillRate": 80.0,
    "psaCampaignRequestId": "psa-req-1",
    "createdAt": "2025-01-01T00:00:00Z",
    "updatedAt": "2025-01-01T00:00:00Z",
    "kind": "standard"
  }
]
```

`kind` is derived at the HTTP layer (`"standard"` or `"external"`) and is not
persisted. `psaCampaignRequestId` is omitted when the campaign has no linked PSA
portal campaign. `targetLanguages` empty means an open net — the campaign buys
any language; the only accepted tokens are `"english"` and `"japanese"`.
`inclusionList` and `exclusionMode` are a **legacy write-only mirror** derived
from `subjects`/`subjectFilterMode` on every write and read by nothing; do not
build against them (`core_types.go:202-213`).

---

### `POST /api/campaigns`

Auth: RequireAuth

Creates a new campaign.

**Body:** `Campaign` object (id, createdAt, updatedAt are server-assigned)

**Response:** `201 Created` — `Campaign` object

**Errors:** `400` invalid data

---

### `GET /api/campaigns/{id}`

Auth: RequireAuth

Returns a single campaign.

**Path params:** `id` (string UUID)

**Response:** `200 OK` — `Campaign` object

**Errors:** `404` not found

---

### `PUT /api/campaigns/{id}`

Auth: RequireAuth

Updates a campaign. Full replacement.

**Path params:** `id` (string UUID)

**Query params:** `ifUnmodifiedSince` (optional, RFC3339) — the `updatedAt` of the
row this payload was built from. Supplying it makes the write conditional: the
row is written only if its stored `updated_at` still matches, so a
read-modify-write caller cannot overwrite a change it never saw. Omitting it
keeps the unconditional overwrite.

**Body:** `Campaign` object

**Response:** `200 OK` — updated `Campaign`

**Errors:** `400` invalid data or unparseable `ifUnmodifiedSince`; `404` not
found; `409` the row changed since `ifUnmodifiedSince` (nothing was written —
re-read and retry)

---

### `DELETE /api/campaigns/{id}`

Auth: RequireAuth

Deletes a campaign.

**Path params:** `id` (string UUID)

**Response:** `204 No Content`

**Errors:** `404` not found

---

## Campaign Purchases

### `GET /api/campaigns/{id}/purchases`

Auth: RequireAuth

Lists purchases for a campaign (paginated).

**Path params:** `id` (campaign UUID)

**Query params:** `limit` (int, default 100), `offset` (int, default 0)

**Response:** `200 OK` — Array of `Purchase`
```json
[
  {
    "id": "uuid",
    "campaignId": "uuid",
    "cardName": "Charizard",
    "certNumber": "12345678",
    "cardNumber": "4",
    "setName": "Base Set",
    "grader": "PSA",
    "gradeValue": 10,
    "clValueCents": 120000,
    "buyCostCents": 84000,
    "psaSourcingFeeCents": 300,
    "population": 150,
    "purchaseDate": "2025-01-01",
    "snapshotStatus": "pending",
    "overridePriceCents": 0,
    "aiSuggestedPriceCents": 0,
    "dhUnlistedDetectedAt": null,
    "createdAt": "2025-01-01T00:00:00Z",
    "updatedAt": "2025-01-01T00:00:00Z"
  }
]
```

`dhUnlistedDetectedAt` is set by the DH reconciler when an item appears deleted on DH (drives the "unlisted on DH" inventory badge); cleared automatically on successful re-list.

---

### `POST /api/campaigns/{id}/purchases`

Auth: RequireAuth

Creates a new purchase manually.

**Path params:** `id` (campaign UUID)

**Body:** `Purchase` object (id, campaignId, createdAt, updatedAt server-assigned)

**Response:** `201 Created` — `Purchase` object

**Errors:** `400` invalid data; `409` cert number already exists

---

### `POST /api/campaigns/{id}/purchases/quick-add`

Auth: RequireAuth

Creates a purchase by cert number lookup (fetches card metadata automatically).

**Path params:** `id` (campaign UUID)

**Body:**
```json
{
  "certNumber": "12345678",
  "buyCostCents": 84000,
  "clValueCents": 120000,
  "purchaseDate": "2025-01-15"
}
```
`clValueCents` and `purchaseDate` are optional.

**Response:** `201 Created` — `Purchase` object

**Errors:** `400` quick-add failed; `404` campaign not found; `409` cert number already exists

---

### `DELETE /api/campaigns/{id}/purchases/{purchaseId}`

Auth: RequireAuth

Deletes a purchase. Verifies the purchase belongs to the campaign.

**Path params:** `id` (campaign UUID), `purchaseId` (purchase UUID)

**Response:** `204 No Content`

**Errors:** `403` purchase belongs to different campaign; `404` not found

---

## Campaign Sales

### `GET /api/campaigns/{id}/sales`

Auth: RequireAuth

Lists sales for a campaign (paginated).

**Path params:** `id` (campaign UUID)

**Query params:** `limit` (int, default 100), `offset` (int, default 0)

**Response:** `200 OK` — Array of `Sale`
```json
[
  {
    "id": "uuid",
    "purchaseId": "uuid",
    "saleChannel": "ebay",
    "salePriceCents": 130000,
    "saleFeeCents": 16055,
    "saleDate": "2025-02-01",
    "daysToSell": 31,
    "netProfitCents": 29645,
    "orderId": "dh-order-1",
    "originalListPriceCents": 145000,
    "priceReductions": 2,
    "daysListed": 28,
    "soldAtAskingPrice": false,
    "wasCracked": false,
    "forcedLiquidation": false,
    "saleReason": "discretionary",
    "clValueAtSaleCents": 128000,
    "channelFeePctAtSale": 0.1235,
    "lastSoldCents": 131000,
    "lowestListCents": 139900,
    "conservativeCents": 120000,
    "medianCents": 129500,
    "midPriceCents": 133000,
    "lastSoldDate": "2025-01-28",
    "activeListings": 14,
    "salesLast30d": 6,
    "trend30d": -0.03,
    "snapshotDate": "2025-02-01",
    "createdAt": "2025-02-01T00:00:00Z",
    "updatedAt": "2025-02-01T00:00:00Z"
  }
]
```

Only `id`, `purchaseId`, `saleChannel`, `salePriceCents`, `saleFeeCents`,
`saleDate`, `daysToSell`, `netProfitCents`, `forcedLiquidation`, `createdAt`,
and `updatedAt` are always present; every other field is `omitempty` and absent
at its zero value. `saleReason` is one of `discretionary`, `invoice_pressure`,
`aging_policy`, `bulk_lot`, `show_clearout` — frozen at sale-creation time.

`lastSoldCents` through `snapshotDate` are the embedded `MarketSnapshotData`
market snapshot at sale time (best-effort, may be entirely absent). Its
`snapshotJSON`, `confidence`, `sourceCountRaw`, and `marketDataObserved` fields
are `json:"-"` and never appear on the wire (`core_types.go:117-136`).

`clValueAtSaleCents` and `channelFeePctAtSale` freeze the values in effect at
sale time and use **different unknown encodings**, mirroring the columns:
`cl_value_at_sale_cents` is non-null, so a recorded `0` is ambiguous for sales
predating migration 000022 (genuinely zero vs never recorded);
`channel_fee_pct_at_sale` is nullable, so its absence is unambiguous
(`core_types.go:496-507`).

---

### `POST /api/campaigns/{id}/sales`

Auth: RequireAuth

Records a sale for a purchase within this campaign. Computes `netProfitCents`, `daysToSell`, and `saleFeeCents` automatically.

**Path params:** `id` (campaign UUID)

**Body:** `Sale` object — required: `purchaseId`, `saleChannel`, `salePriceCents`, `saleDate`

Valid `saleChannel` values: `ebay`, `website`, `inperson`, `tcgplayer`, `local`, `other`, `gamestop`, `cardshow`, `doubleholo`

Optional `saleReason` (one of `discretionary`, `invoice_pressure`, `aging_policy`, `bulk_lot`, `show_clearout`); omit to let the server derive a default (from `forced_liquidation`/channel heuristics). Sending `saleReason: ""` is equivalent to omitting it.

**Response:** `201 Created` — `Sale` object (may include `warnings` array)

**Errors:** `400` invalid data or purchase belongs to different campaign; `404` purchase or campaign not found; `409` sale already exists for this purchase

---

### `PATCH /api/campaigns/{id}/sales/{saleID}`

Auth: RequireAuth

Updates the `saleReason` on an existing sale record (e.g. correcting a
misclassified reason after the fact).

**Path params:** `id` (campaign UUID), `saleID` (sale UUID)

**Body:**
```json
{ "saleReason": "aging_policy" }
```

`saleReason` must be one of `discretionary`, `invoice_pressure`, `aging_policy`, `bulk_lot`, `show_clearout` (or `""`).

**Response:** `204 No Content`

**Errors:** `400` invalid `saleReason` value; `404` sale not found

---

### `DELETE /api/campaigns/{id}/purchases/{purchaseId}/sale`

Auth: RequireAuth

Deletes the sale recorded against a purchase, returning the purchase to unsold. The purchase must belong to the named campaign.

**Path params:** `id` (campaign UUID), `purchaseId` (purchase UUID)

**Response:** `204 No Content`

**Errors:** `400` missing path param; `403` purchase does not belong to this campaign; `404` purchase not found, or no sale recorded for this purchase; `500` internal error

---

### `POST /api/campaigns/{id}/sales/bulk`

Auth: RequireAuth

Creates multiple sales in one request.

**Path params:** `id` (campaign UUID)

**Body:**
```json
{
  "saleChannel": "ebay",
  "saleDate": "2025-02-01",
  "items": [
    { "purchaseId": "uuid", "salePriceCents": 130000, "saleReason": "bulk_lot" }
  ]
}
```

Per-item optional fields (all in `items[]`, mirroring the single-sale endpoint): `originalListPriceCents`, `priceReductions`, `daysListed`, `saleReason`. `saleReason` follows the same omit-when-default rule as the single-sale endpoint.

**Response:** `201 Created` — `BulkSaleResult`
```json
{
  "created": 5,
  "failed": 0,
  "errors": []
}
```

---

### `GET /api/portfolio/analysis`

Auth: RequireAuth

Returns cross-campaign portfolio analysis. **Query params:** `since` (optional, `YYYY-MM-DD`).

**Response:** `200 OK` — `portfolio.AnalysisResponse`. Each `campaigns[]` entry (`CampaignAnalysis`) includes, in addition to the existing `bpclAtBuy`/`weeklyFill`/`inScopeByGrade`:

- `pnlByConfidenceBuy` — array of cohort rows aggregating P&L and provenance
  averages by `(CL confidence at purchase) × (buy cost as % of CL at
  purchase)`, using the frozen `*_at_purchase` snapshot columns (migration
  000022). Sorted by confidence bucket then buy-terms bucket, `"unknown"`
  last.
- `pnl.byReason` — map keyed by `saleReason` (`discretionary`,
  `invoice_pressure`, `aging_policy`, `bulk_lot`, `show_clearout`) to a
  `PNLBlock` (`soldCount`, `revenueCents`, `netProfitCents`, `roiPct`). All
  five keys are always present, even with zero sales. Sales with an empty/
  legacy `saleReason` are excluded from this map but still counted in the
  existing `pnl.discretionary`/`pnl.forced` split.

Imported historical purchases/sales that predate migration 000022 (or were
imported without a market-data source) will have `null`/absent
`*_at_purchase` and `*_at_sale` fields; these values are **record-time
proxies** captured at import/purchase/sale time, not live market data — treat
them as directional signal, not authoritative pricing.

---


## Campaign Analytics

### `GET /api/campaigns/{id}/pnl`

Auth: RequireAuth

Returns overall P&L summary for a campaign.

**Response:** `200 OK` — `CampaignPNL`
```json
{
  "campaignId": "uuid",
  "totalSpendCents": 840000,
  "totalRevenueCents": 1300000,
  "totalFeesCents": 160550,
  "netProfitCents": 296450,
  "roi": 0.352,
  "avgDaysToSell": 28.5,
  "totalPurchases": 10,
  "totalSold": 7,
  "totalUnsold": 3,
  "sellThroughPct": 0.7
}
```

---

### `GET /api/campaigns/{id}/pnl-by-channel`

Auth: RequireAuth

Returns P&L broken down by sale channel.

**Response:** `200 OK` — Array of `ChannelPNL`
```json
[
  {
    "channel": "ebay",
    "saleCount": 5,
    "revenueCents": 900000,
    "feesCents": 111150,
    "netProfitCents": 200000,
    "avgDaysToSell": 22.0
  }
]
```

---

### `GET /api/campaigns/{id}/fill-rate`

Auth: RequireAuth

Returns daily spend vs daily cap over a window.

**Query params:** `days` (int, default 30, max 365)

**Response:** `200 OK` — Array of daily spend rows. Monetary values are USD, not cents.
```json
[
  {
    "date": "2025-01-15",
    "spendUSD": 840.00,
    "capUSD": 10000.00,
    "fillRatePct": 0.084,
    "purchaseCount": 1
  }
]
```

`capUSD` reflects the campaign's current daily cap, applied to every day in the window; no per-day historical cap is stored.

---

### `GET /api/campaigns/{id}/days-to-sell`

Auth: RequireAuth

Returns a histogram of days-to-sell for sold cards.

**Response:** `200 OK` — Array of `DaysToSellBucket`
```json
[
  { "label": "0-7", "min": 0, "max": 7, "count": 2 },
  { "label": "8-14", "min": 8, "max": 14, "count": 3 }
]
```

---

### `GET /api/campaigns/{id}/inventory`

Auth: RequireAuth

Returns unsold inventory with aging data and market signals.

**Response:** `200 OK` — Array of `AgingItem`
```json
[
  {
    "purchase": { ...Purchase... },
    "daysHeld": 45,
    "campaignName": "Pokemon Base 2025 Q1",
    "signal": {
      "cardName": "Charizard",
      "certNumber": "12345678",
      "grade": 10,
      "clValueCents": 120000,
      "lastSoldCents": 135000,
      "deltaPct": 0.125,
      "direction": "rising",
      "recommendation": "List on eBay"
    }
  }
]
```

---

### `GET /api/campaigns/{id}/tuning`

Auth: RequireAuth

Returns parameter tuning recommendations for a campaign (buy terms, fee rates, etc.).

**Response:** `200 OK` — campaign tuning data object

**Errors:** `404` campaign not found

---

### `GET /api/campaigns/{id}/expected-values`

Auth: RequireAuth

Returns expected value calculations for unsold inventory.

**Response:** `200 OK` — expected value portfolio object

---

### `POST /api/campaigns/{id}/evaluate-purchase`

Auth: RequireAuth

Evaluates a hypothetical purchase decision.

**Body:**
```json
{
  "cardName": "Charizard",
  "grade": 10,
  "buyCostCents": 84000
}
```

**Response:** `200 OK` — evaluation result with EV, margin, recommendation

**Errors:** `400` missing/invalid fields; `404` campaign not found

---

### `GET /api/campaigns/{id}/activation-checklist`

Auth: RequireAuth

Returns a pre-activation checklist for a campaign (configuration completeness, fee settings, etc.).

**Response:** `200 OK` — `ActivationChecklist`
```json
{
  "campaignId": "uuid",
  "campaignName": "Pokemon Base 2025 Q1",
  "allPassed": true,
  "checks": [
    { "name": "Buy Terms Set", "passed": true, "message": "buyTermsCLPct = 0.70" }
  ],
  "warnings": []
}
```

**Errors:** `404` campaign not found

---

### `GET /api/campaigns/{id}/projections`

Auth: RequireAuth

Runs a Monte Carlo projection for campaign outcomes.

**Response:** `200 OK` — projection result with percentile scenarios

**Errors:** `404` campaign not found

---

## Global Purchases

### `POST /api/purchases/import-psa`

Auth: RequireAuth

Imports purchases from a PSA communication spreadsheet CSV.

**Body:** `multipart/form-data` — `file` field (CSV, max 10MB)

Required columns (auto-detected in first 6 rows): `cert number`, `listing title`, `grade`

Optional columns: `price paid`, `date` (M/D/YYYY), `invoice date`, `was refunded?`, `front image url`, `back image url`

**Response:** `200 OK` — `PSAImportResult`
```json
{
  "allocated": 8,
  "updated": 2,
  "refunded": 0,
  "unmatched": 1,
  "certEnrichmentPending": 3,
  "results": [...]
}
```

---

### `POST /api/purchases/import-external`

Auth: RequireAuth

Imports purchases from a Shopify product export CSV.

**Body:** `multipart/form-data` — `file` field (CSV, max 10MB)

Required CSV columns: `handle`, `title`. PSA cert extracted from `cert number`, `cert`, or `sku` (PSA-XXXXX pattern).

**Response:** `200 OK` — `ExternalImportResult`
```json
{
  "imported": 12,
  "skipped": 3,
  "updated": 2,
  "failed": 0,
  "errors": [{ "row": 7, "error": "missing title" }],
  "results": [
    { "certNumber": "12345678", "status": "imported", "cardName": "Charizard" }
  ]
}
```

`errors` and `results` are both `omitempty`. `errors` carries one
`{row, error}` entry per rejected CSV row. `row` is 1-based over the **data
rows**, excluding the header line — so `row: 7` is the 7th data row, the 8th
line of the file (`csvimport/service_import_external.go:73`).

---

### `POST /api/purchases/import-certs`

Auth: RequireAuth

Imports purchases by cert number list (fetches card metadata via PSA API).

**Body:**
```json
{ "certNumbers": ["12345678", "87654321"] }
```

**Response:** `200 OK` — `CertImportResult`
```json
{
  "imported": 2,
  "alreadyExisted": 0,
  "soldExisting": 1,
  "failed": 0,
  "errors": [],
  "soldItems": [
    { "certNumber": "12345678", "purchaseId": "abc", "cardName": "Charizard", "campaignId": "xyz" }
  ]
}
```

---

### `PATCH /api/purchases/{purchaseId}/campaign`

Auth: RequireAuth

Reassigns a purchase to a different campaign.

**Path params:** `purchaseId` (purchase UUID)

**Body:**
```json
{ "campaignId": "uuid" }
```

**Response:** `204 No Content`

**Errors:** `400` missing campaignId; `404` purchase or campaign not found

---

## Price Override & AI Suggestions

### `PATCH /api/purchases/{purchaseId}/price-override`

Auth: RequireAuth

Sets a manual price override for a purchase.

**Path params:** `purchaseId` (purchase UUID)

**Body:**
```json
{ "priceCents": 130000, "source": "manual" }
```
Valid `source` values: `manual`, `cost_markup`

> Note: `ai_accepted` is only set internally via the accept-AI-suggestion endpoint and cannot be used directly.

**Response:** `204 No Content`

**Errors:** `400` validation error; `404` not found

---

### `DELETE /api/purchases/{purchaseId}/price-override`

Auth: RequireAuth

Clears the price override for a purchase.

**Path params:** `purchaseId` (purchase UUID)

**Response:** `204 No Content`

**Errors:** `404` not found

---

### `POST /api/purchases/{purchaseId}/accept-ai-suggestion`

Auth: RequireAuth

Accepts the pending AI price suggestion, converting it to a price override (source `ai_accepted`).

**Path params:** `purchaseId` (purchase UUID)

**Response:** `204 No Content`

**Errors:** `404` not found; `409` AI suggestion no longer available; `400` validation error

---

### `DELETE /api/purchases/{purchaseId}/ai-suggestion`

Auth: RequireAuth

Dismisses (clears) the pending AI price suggestion without accepting it.

**Path params:** `purchaseId` (purchase UUID)

**Response:** `204 No Content`

**Errors:** `404` not found

---

## Price Review & Flags

### `PATCH /api/purchases/{purchaseId}/review-price`

Auth: RequireAuth

Sets a reviewed price for a purchase (human-verified price point).

**Path params:** `purchaseId` (purchase UUID)

**Body:**
```json
{ "priceCents": 130000, "source": "manual" }
```

**Response:** `200 OK`
```json
{ "success": true, "reviewedAt": "2025-01-01T00:00:00Z" }
```

**Errors:** `400` validation error; `404` purchase not found

---

### `POST /api/purchases/{purchaseId}/flag`

Auth: RequireAuth

Creates a price flag for data quality review.

**Path params:** `purchaseId` (purchase UUID)

**Body:**
```json
{ "reason": "wrong_match" }
```
Valid `reason` values: `wrong_match`, `stale_data`, `wrong_grade`, `source_disagreement`, `other`

**Response:** `201 Created`
```json
{ "id": 1, "flaggedAt": "2025-01-01T00:00:00Z" }
```

**Errors:** `400` validation error; `404` purchase not found

---

## Credit & Invoices

### `GET /api/credit/summary`

Auth: RequireAuth

Returns current capital exposure summary with recovery velocity.

**Response:** `200 OK` — `CapitalSummary`
```json
{
  "outstandingCents": 840000,
  "recoveryRate30dCents": 320000,
  "recoveryRate30dPriorCents": 280000,
  "weeksToCover": 11.3,
  "recoveryTrend": "improving",
  "alertLevel": "warning",
  "refundedCents": 0,
  "paidCents": 500000,
  "unpaidInvoiceCount": 1,
  "nextInvoiceDate": "2025-02-15",
  "nextInvoiceDueDate": "2025-03-01",
  "nextInvoiceAmountCents": 340000,
  "daysUntilInvoiceDue": 14,
  "nextInvoicePendingReceiptCents": 60000,
  "nextInvoiceSellThrough": {
    "totalPurchaseCount": 40,
    "soldCount": 18,
    "totalCostCents": 340000,
    "saleRevenueCents": 210000
  }
}
```

The `nextInvoice*` fields are invoice-cycle actuals for the earliest unpaid
invoice, populated by the finance service (`finance/invoice_projection.go`).
`nextInvoiceDate` and `nextInvoiceDueDate` are `omitempty` and absent when
there is no unpaid invoice. `daysUntilInvoiceDue` counts from now to the due
date and goes **negative when overdue**. `nextInvoiceAmountCents` is the amount
still owed (`totalCents - paidCents`), not the invoice's original total.
`nextInvoiceSellThrough` covers only returned, non-refunded purchases for that
invoice date. `weeksToCover` is `99` when there is no recovery data
(`WeeksToCoverNoData`).

---

### `GET /api/credit/config`

Auth: RequireAuth

Returns the current cashflow configuration.

**Response:** `200 OK` — `CashflowConfig`
```json
{
  "capitalBudgetCents": 2000000,
  "cashBufferCents": 200000,
  "updatedAt": "2025-01-01T00:00:00Z"
}
```

---

### `PUT /api/credit/config`

Auth: RequireAuth

Replaces the cashflow configuration. Both fields are required — this is a full replace, not a patch.

**Body:**
```json
{
  "capitalBudgetCents": 2000000,
  "cashBufferCents": 200000
}
```

**Response:** `200 OK` — the saved `CashflowConfig`

**Errors:** `400` invalid configuration; `503` finance service not available; `500` internal error

---

### `GET /api/credit/invoices`

Auth: RequireAuth

Lists all PSA invoices.

**Response:** `200 OK` — Array of `Invoice`
```json
[
  {
    "id": "uuid",
    "invoiceDate": "2025-01-15",
    "totalCents": 840000,
    "paidCents": 0,
    "pendingReceiptCents": 60000,
    "dueDate": "2025-01-30",
    "paidDate": "2025-01-28",
    "status": "unpaid",
    "createdAt": "2025-01-15T00:00:00Z",
    "updatedAt": "2025-01-15T00:00:00Z"
  }
]
```

`status` is `"unpaid"`, `"partial"`, or `"paid"`. `dueDate` and `paidDate` are
`omitempty`; `paidDate` is absent until the invoice is settled.
`pendingReceiptCents` is the cost of cards on this invoice still at PSA.

---

### `PUT /api/credit/invoices`

Auth: RequireAuth

Updates an invoice (e.g. mark as paid, record payment).

**Body:** `Invoice` object with the `id` field set.

**Response:** `200 OK` — updated `Invoice`

**Errors:** `404` invoice not found

---

## Portfolio

### `GET /api/portfolio/snapshot`

Auth: RequireAuth

Composite endpoint returning health, insights, weekly-review, weekly-history (8 weeks), channel-velocity, suggestions, credit summary, and invoices in a single response. Loads shared data once server-side, avoiding redundant DB queries.

**Response:**

```json
{
  "health": { /* same as GET /api/portfolio/health */ },
  "insights": { /* same as GET /api/portfolio/insights */ },
  "weeklyReview": { /* same as GET /api/portfolio/weekly-review */ },
  "weeklyHistory": [ /* same as GET /api/portfolio/weekly-history?weeks=8 */ ],
  "channelVelocity": [ /* same as GET /api/portfolio/channel-velocity */ ],
  "suggestions": { /* same as GET /api/portfolio/suggestions */ },
  "creditSummary": { /* same as GET /api/credit/summary */ },
  "invoices": [ /* same as GET /api/credit/invoices */ ]
}
```

Individual endpoints below remain available unchanged.

### `GET /api/portfolio/health`

Auth: RequireAuth

Returns cross-campaign health assessment.

**Response:** `200 OK` — `PortfolioHealth`
```json
{
  "campaigns": [
    {
      "campaignId": "uuid",
      "campaignName": "Q1",
      "phase": "active",
      "roi": 0.35,
      "sellThroughPct": 0.7,
      "avgDaysToSell": 28.5,
      "totalPurchases": 10,
      "totalUnsold": 3,
      "capitalAtRiskCents": 252000,
      "healthStatus": "healthy",
      "healthReason": "ROI above target"
    }
  ],
  "totalDeployedCents": 840000,
  "totalRecoveredCents": 1300000,
  "totalAtRiskCents": 252000,
  "overallROI": 0.35
}
```

---

### `GET /api/portfolio/channel-velocity`

Auth: RequireAuth

Returns sales velocity statistics per channel across all campaigns.

**Response:** `200 OK` — Array of `ChannelVelocity`
```json
[
  {
    "channel": "ebay",
    "saleCount": 25,
    "avgDaysToSell": 22.0,
    "revenueCents": 3250000
  }
]
```

---

### `GET /api/portfolio/insights`

Auth: RequireAuth

Returns portfolio-level insights and recommendations.

**Response:** `200 OK` — insights object with action items and performance flags

---

### `GET /api/portfolio/suggestions`

Auth: RequireAuth

Returns campaign-level parameter suggestions.

**Response:** `200 OK` — suggestions object

---

### `GET /api/portfolio/revocations`

Auth: RequireAuth

Lists all PSA revocation flags.

**Response:** `200 OK` — Array of `RevocationFlag`
```json
[
  {
    "id": "uuid",
    "segmentLabel": "Charizard Base PSA 9",
    "segmentDimension": "card_grade",
    "reason": "Market declined below buy basis",
    "status": "pending",
    "emailText": "",
    "createdAt": "2025-01-01T00:00:00Z",
    "sentAt": "2025-01-02T00:00:00Z"
  }
]
```

`status` is `"pending"` or `"sent"`. `sentAt` is omitted while the flag is
still `"pending"`.

---

### `POST /api/portfolio/revocations`

Auth: RequireAuth

Creates a new PSA revocation flag.

**Body:**
```json
{
  "segmentLabel": "Charizard Base PSA 9",
  "segmentDimension": "card_grade",
  "reason": "Market declined below buy basis"
}
```

**Response:** `201 Created` — `RevocationFlag` object

**Errors:** `409` flagged too recently (cooldown period)

---

### `GET /api/portfolio/revocations/{flagId}/email`

Auth: RequireAuth

Generates the revocation email text for a flag.

**Path params:** `flagId` (UUID)

**Response:** `200 OK`
```json
{ "emailText": "Dear PSA Partner..." }
```

---

### `GET /api/portfolio/capital-timeline`

Auth: RequireAuth

Returns daily capital deployment and recovery timeline with invoice markers.

**Response:** `200 OK` — `CapitalTimeline`
```json
{
  "dataPoints": [
    {
      "date": "2025-01-01",
      "cumulativeSpendCents": 84000,
      "cumulativeRecoveryCents": 0,
      "outstandingCents": 84000
    }
  ],
  "invoiceDates": ["2025-01-15"]
}
```

---

### `GET /api/portfolio/weekly-review`

Auth: RequireAuth

Returns the Monday weekly review summary (WoW spend, revenue, sales comparisons).

**Response:** `200 OK` — `WeeklyReviewSummary`
```json
{
  "weekStart": "2025-01-13",
  "weekEnd": "2025-01-19",
  "purchasesThisWeek": 5,
  "purchasesLastWeek": 3,
  "spendThisWeekCents": 420000,
  "spendLastWeekCents": 252000,
  "salesThisWeek": 7,
  "salesLastWeek": 4,
  "revenueThisWeekCents": 910000,
  "revenueLastWeekCents": 520000,
  "profitThisWeekCents": 175000,
  "profitLastWeekCents": 85000,
  "byChannel": [...],
  "weeksToCover": 3.5,
  "daysIntoWeek": 1,
  "topPerformers": [...],
  "bottomPerformers": [...]
}
```

`daysIntoWeek` is `0`=Sunday … `6`=Saturday, letting a client tell a partial
week from a complete one before comparing the WoW figures.

---

### `GET /api/portfolio/weekly-history`

Auth: RequireAuth

Returns the N most recent weekly review summaries in reverse chronological order.

**Query params:** `weeks` (optional, default 8, minimum 1, maximum 52)

**Response:** `200 OK` — Array of `WeeklyReviewSummary` (same shape as `GET /api/portfolio/weekly-review`)

**Errors:** `400` `weeks` is not a positive integer or exceeds 52; `500` internal error

---

## Utilities

### `GET /api/certs/{certNumber}`

Auth: RequireAuth

Looks up a PSA cert number and returns card info plus current market snapshot.

**Path params:** `certNumber` (digits)

**Response:** `200 OK`
```json
{
  "cert": { ...PSA cert info... },
  "market": { ...MarketSnapshot... }
}
```

**Errors:** `404` cert lookup failed

---

## AI Advisor

All advisor endpoints stream responses via **Server-Sent Events (SSE)**. Set `Accept: text/event-stream` or handle the `text/event-stream` content type. Each event is `data: <JSON>\n\n`. The stream ends with `data: [DONE]\n\n`.

Event shape:
```json
{ "type": "content", "content": "Markdown text chunk..." }
```
Error event:
```json
{ "type": "error", "content": "Error message" }
```

### `POST /api/advisor/digest`

Auth: RequireAuth

Streams a weekly portfolio intelligence digest.

**Body:** (empty)

**Response:** `200 OK` — SSE stream

---

### `POST /api/advisor/liquidation-analysis`

Auth: RequireAuth

Streams liquidation candidate recommendations across all campaigns.

**Body:** (empty)

**Response:** `200 OK` — SSE stream

---

## Pricing API v1

The Pricing API is a separate public API authenticated by a static bearer token (`PRICING_API_KEY`). It provides read-only price lookups based on internal inventory data.

All requests require: `Authorization: Bearer <PRICING_API_KEY>`

Rate limit: 60 req/min per IP.

Error format:
```json
{ "error": "error_code", "message": "Human readable message" }
```

### `GET /api/v1/prices/{certNumber}`

Auth: RequireAPIKey

Returns pricing data for a single PSA cert number.

**Path params:** `certNumber` (string)

**Response:** `200 OK`
```json
{
  "certNumber": "12345678",
  "suggestedPrice": 1300.00,
  "computedPrice": 1200.00,
  "overridePrice": 1300.00,
  "aiSuggestedPrice": 0,
  "priceSource": "override",
  "currency": "USD"
}
```

`priceSource` values: `cl_value` (CL market value), `override` (manual/AI-accepted override)

**Errors:** `400` missing cert; `404` no pricing data; `500` lookup error

---

### `POST /api/v1/prices/batch`

Auth: RequireAPIKey

Returns pricing data for up to 100 cert numbers in one request. Deduplicates input automatically.

**Body:**
```json
{ "certNumbers": ["12345678", "87654321"] }
```
Max 100 items; no empty strings.

**Response:** `200 OK`
```json
{
  "results": [
    {
      "certNumber": "12345678",
      "suggestedPrice": 1300.00,
      "computedPrice": 1200.00,
      "priceSource": "override",
      "currency": "USD"
    }
  ],
  "notFound": ["87654321"],
  "totalRequested": 2,
  "totalFound": 1
}
```

**Errors:** `400` missing/empty array or >100 items; `500` lookup error

---

## Global Inventory & Sell Sheet

### `GET /api/inventory`

Auth: RequireAuth

Returns unsold inventory aging data across all campaigns.

**Response:** `200 OK` — Array of `AgingItem` (same as `/api/campaigns/{id}/inventory` but cross-campaign, includes `campaignName` on each item)

---

### `GET /api/sell-sheet`

Auth: RequireAuth

Generates a global sell sheet across all active campaigns.

**Response:** `200 OK` — `SellSheet`
```json
{
  "generatedAt": "2025-01-01T00:00:00Z",
  "campaignName": "Pokemon Base 2025 Q1",
  "items": [
    {
      "certNumber": "12345678",
      "cardName": "Charizard",
      "grade": 10,
      "buyCostCents": 84000,
      "costBasisCents": 84300,
      "clValueCents": 120000,
      "recommendation": "List on eBay",
      "targetSellPrice": 130000,
      "minimumAcceptPrice": 90000,
      "recommendedChannel": "ebay"
    }
  ],
  "totals": {
    "totalCostBasis": 84300,
    "totalExpectedRevenue": 130000,
    "totalProjectedProfit": 29645,
    "itemCount": 1,
    "skippedItems": 0
  }
}
```
`campaignName` is set per item rather than at the top level when the sheet spans campaigns.

---

## Global Purchase Operations

### `POST /api/purchases/sync-psa-sheets`

Auth: RequireAuth

Pulls the PSA portal's order rows directly (no file upload) and imports them the same way the CSV path does. The portal fetch is bounded by a 2-minute timeout.

**Body:** (empty)

**Response:** `200 OK` — `PSAImportResult` (same shape as `POST /api/purchases/import-psa`)

**Errors:** `400` portal returned no rows; `502` failed to fetch PSA portal data; `503` PSA portal sync not configured; `500` internal error

---

### `POST /api/purchases/import-orders`

Auth: RequireAuth

Imports an orders export CSV, matches PSA cert numbers against unsold inventory, and returns categorized results for review before confirmation.

**Body:** `multipart/form-data` — `file` field (CSV, max 10MB)

**Response:** `200 OK` — `OrdersImportResult`
```json
{
  "matched": [
    {
      "certNumber": "12345678",
      "productTitle": "PSA 10 Charizard",
      "saleChannel": "ebay",
      "saleDate": "2025-02-01",
      "salePriceCents": 130000,
      "saleFeeCents": 16055,
      "purchaseId": "uuid",
      "campaignId": "uuid",
      "cardName": "Charizard",
      "buyCostCents": 84000,
      "netProfitCents": 29645
    }
  ],
  "alreadySold": [
    { "certNumber": "87654321", "productTitle": "...", "reason": "already_sold" }
  ],
  "notFound": [],
  "skipped": []
}
```

Each matched row may also carry `campaignLookupFailed: true`
(`csvimport/import_types.go:199`, `omitempty` so absent in the normal case). It means the
row's campaign could not be loaded, so `saleFeeCents` was computed against a
zero-fee campaign and both it and `netProfitCents` are **optimistic estimates**
rather than the values the confirm step will persist
(`csvimport/service_import_orders.go:67-96`). `reason` on the `alreadySold` / `notFound` /
`skipped` entries is one of `already_sold`, `not_found`, `duplicate`, `not_psa`,
`unknown_channel`.

---

### `POST /api/purchases/import-orders/confirm`

Auth: RequireAuth

Accepts confirmed matches from order import and creates sale records.

**Body:** Array of `OrdersConfirmItem`
```json
[
  {
    "purchaseId": "uuid",
    "saleChannel": "ebay",
    "saleDate": "2025-02-01",
    "salePriceCents": 130000,
    "orderId": "ORD-123"
  }
]
```

**Response:** `200 OK` — `BulkSaleResult`
```json
{
  "created": 5,
  "failed": 0,
  "errors": []
}
```

**Errors:** `400` no items provided or invalid JSON

---

### `POST /api/purchases/scan-cert`

Auth: RequireAuth

Scans a cert number to determine if it exists in inventory, has been sold, or is new.

**Body:**
```json
{ "certNumber": "12345678" }
```

**Response:** `200 OK` — `ScanCertResult`
```json
{
  "status": "existing",
  "cardName": "Charizard",
  "purchaseId": "uuid",
  "campaignId": "uuid",
  "buyCostCents": 100000,
  "market": { "gradePriceCents": 150000, "clValueCents": 140000 },
  "frontImageUrl": "https://…/cert-front.jpg",
  "setName": "Base Set",
  "cardNumber": "4",
  "cardYear": "1999",
  "gradeValue": 10,
  "population": 1234,
  "dhSearchQuery": "1999 base set charizard 4 psa 10",
  "dhCardId": 98765,
  "dhInventoryId": 12345,
  "dhPushStatus": "matched",
  "dhStatus": "in_stock",
  "dhListingPriceCents": 145000,
  "receivedAt": "2025-02-01T00:00:00Z"
}
```
`status` values: `existing` (unsold in inventory), `sold` (has a sale record), `new` (not in system).

Every field except `status` is `omitempty` (`cert_import_types.go:43-76`), so a
`new` cert returns `{"status":"new"}` and nothing else. The card metadata block
(`frontImageUrl` … `population`) and `dhSearchQuery` are populated only for
`existing` and `sold`, from the matched `Purchase` record. `dhSearchQuery` is
built with the same `cardutil` normalization the backend uses for DH card
matching, so the operator's "Search on DH" link lands on the candidates DH's own
matcher would consider. `receivedAt` being non-empty is the in-hand signal;
`dhListingPriceCents` is the card's current DH listing price.

`dhPushStatus` values (existing/sold only): `pending`, `matched`, `unmatched` (push gave up — intake should surface Fix DH Match), `manual`, `held`, `dismissed`. `dhInventoryId > 0` is the canonical "ready to list on DH" signal; the intake screen gates its price-and-list UI on this rather than on snapshot data.

---

### `POST /api/purchases/scan-certs`

Auth: RequireAuth

Batch variant of `scan-cert`. Used by the cert-intake polling loop so N rows
awaiting sync produce one request per tick instead of N.

**Body:**
```json
{ "certNumbers": ["12345678", "99999999"] }
```

**Response:** `200 OK` — `ScanCertsResult`
```json
{
  "results": {
    "12345678": { "status": "existing", "cardName": "Charizard", "purchaseId": "uuid" },
    "99999999": { "status": "new" }
  },
  "errors": []
}
```

Each requested cert appears in either `results` or `errors`. Duplicate and
empty entries in the request are coalesced/dropped.

---

### `POST /api/purchases/resolve-cert`

Auth: RequireAuth

Resolves a PSA cert number to card metadata via external lookup.

**Body:**
```json
{ "certNumber": "12345678" }
```

**Response:** `200 OK` — `ResolveCertResult`
```json
{
  "certNumber": "12345678",
  "cardName": "Charizard",
  "grade": 10,
  "year": "1999",
  "category": "Pokemon",
  "subject": "Charizard"
}
```

**Errors:** `400` missing certNumber; `404` cert not found

---

### `PATCH /api/purchases/{purchaseId}/buy-cost`

Auth: RequireAuth

Updates the buy cost of a purchase.

**Path params:** `purchaseId` (purchase UUID)

**Body:**
```json
{ "buyCostCents": 84000 }
```

**Response:** `204 No Content`

**Errors:** `400` validation error; `404` purchase not found

---

### `POST /api/purchases/{purchaseId}/list-on-dh`

Auth: RequireAuth

Lists a single received purchase on DH. When the item has no DH inventory ID yet but is push-eligible, the listing service performs an inline match + push first, then lists.

Preconditions: the purchase must be received, not already listed, and carry a human-committed price (a "Set Price" override or a price-review value — CardLadder values alone do not qualify). Items with a DH inventory ID must be `in_stock`.

**Path params:** `purchaseId` (purchase UUID)

**Body:** (empty)

**Response:** `200 OK` — `DHListingResult`
```json
{
  "listed": 1,
  "synced": 1,
  "skipped": 0,
  "total": 1,
  "paused": false
}
```
`paused` is always `false` on a `200` — the pause case is reported as a `409`. The
struct's two `error`-typed fields are `json:"-"`: the handler turns them into the
status codes below before writing a success body.

**Errors:** `400` missing `purchaseId`; `404` purchase not found; `409` not received / already listed / not `in_stock` / push held, unmatched, or dismissed / price not reviewed / DH listings globally paused; `502` PSA authentication temporarily unavailable — retry shortly, or listing failed with no recorded upstream error; `503` DH listing service not configured; `500` internal error

---

## Liquidation

### `POST /api/liquidation/preview`

Auth: RequireAuth

Computes suggested liquidation prices for unsold inventory, discounting against recent sale comps where available and falling back to a no-comps discount otherwise. Read-only — nothing is written.

**Body:** optional; an empty body uses the configured defaults.
```json
{
  "discountWithCompsPct": 0.10,
  "discountNoCompsPct": 0.25
}
```

**Response:** `200 OK` — `PreviewResponse`
```json
{
  "items": [
    {
      "purchaseId": "uuid",
      "certNumber": "12345678",
      "cardName": "Charizard",
      "setName": "Base Set",
      "cardNumber": "4",
      "grade": 10,
      "buyCostCents": 84000,
      "clValueCents": 120000,
      "compPriceCents": 118000,
      "compCount": 6,
      "mostRecentCompDate": "2025-01-01",
      "confidenceLevel": "high",
      "gapPct": -0.02,
      "currentReviewedPriceCents": 130000,
      "suggestedPriceCents": 106200,
      "belowCost": false
    }
  ],
  "summary": {
    "totalCards": 42,
    "withComps": 30,
    "withoutComps": 8,
    "noData": 4,
    "totalCurrentValueCents": 5400000,
    "totalSuggestedValueCents": 4860000,
    "belowCostCount": 3
  }
}
```

**Errors:** `400` invalid request body; `500` preview failed

---

### `POST /api/liquidation/apply`

Auth: RequireAuth

Applies the chosen prices from a preview. Partial success is reported rather than rolled back — check `failed` and `errors`.

**Body:**
```json
{
  "items": [
    { "purchaseId": "uuid", "newPriceCents": 106200 }
  ]
}
```
At least one item is required.

**Response:** `200 OK` — `ApplyResult`
```json
{
  "applied": 40,
  "failed": 2,
  "errors": ["purchase uuid: not found"]
}
```

**Errors:** `400` invalid request body or empty `items`; `500` apply failed

---

## DH Integration

### `GET /api/dh/pending`

Auth: RequireAuth

Lists received, unsold purchases with `dhPushStatus = "pending"` — the queue the DH push pipeline drains. Each item carries a recommended price and a data-freshness confidence signal.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "purchaseId": "uuid",
      "cardName": "Charizard",
      "setName": "Base Set",
      "grade": 10,
      "recommendedPriceCents": 130000,
      "daysQueued": 3,
      "dhConfidence": "high"
    }
  ],
  "count": 1
}
```
`dhConfidence` is `"high"` (synced <24h ago), `"medium"` (<7d), or `"low"` (>7d or never synced).

**Errors:** `503` DH pending lister not available; `500` internal error

---

### `POST /api/dh/dismiss`

Auth: RequireAuth

Marks a purchase as `dismissed` so the DH listing pipeline skips it. Valid from any non-terminal push state (`pending`, `unmatched`, `matched`, `manual`, `held`).

**Body:**
```json
{ "purchaseId": "uuid" }
```

**Response:** `200 OK`
```json
{ "status": "dismissed" }
```

**Errors:** `400` missing `purchaseId` or invalid body; `404` purchase not found; `409` purchase cannot be dismissed from its current state (includes already-dismissed); `500` internal error

---

### `POST /api/dh/undismiss`

Auth: RequireAuth

Restores a dismissed purchase so it can be re-attempted. Received items go back to `pending` for the scheduler to pick up; unreceived items have their push status cleared entirely and wait for intake to re-enroll them.

**Body:**
```json
{ "purchaseId": "uuid" }
```

**Response:** `200 OK` — the new push status, empty string for unreceived items
```json
{ "status": "pending" }
```

**Errors:** `400` missing `purchaseId` or invalid body; `404` purchase not found; `409` purchase is not in dismissed state; `500` internal error

---

### `POST /api/dh/reconcile`

Auth: RequireAdmin

Runs DH reconciliation synchronously: scans our listed inventory against DH and resets rows that no longer exist upstream. Guarded by a mutex so concurrent clicks cannot double-reset the same rows.

**Body:** (empty)

**Response:** `200 OK`
```json
{
  "scanned": 240,
  "missingOnDH": 3,
  "reset": 3,
  "errors": [],
  "resetIds": ["uuid1", "uuid2", "uuid3"]
}
```
`errors` and `resetIds` are omitted when empty.

**Errors:** `409` a reconcile is already running — `{ "status": "already_running" }`; `503` DH reconciliation not configured; `502`/`500` upstream or internal failure

---

### `POST /api/dh/match`

Auth: RequireAuth

Kicks off an async bulk match of unmatched inventory cards against the DH catalog. Returns immediately; progress is visible via `GET /api/dh/status`.

**Body:** (empty)

**Response:** `202 Accepted`
```json
{ "status": "started" }
```

**Errors:** `409` bulk match already running

---

### `GET /api/dh/unmatched`

Auth: RequireAuth

Returns inventory cards that do not yet have a DH mapping (push status = unmatched).

**Response:** `200 OK`
```json
{
  "unmatched": [
    {
      "purchase_id": "uuid",
      "card_name": "Charizard",
      "set_name": "Base Set",
      "card_number": "4",
      "cert_number": "12345678",
      "grade": 10,
      "cl_value_cents": 120000,
      "candidates": [
        { "dh_card_id": 123, "card_name": "Charizard", "set_name": "Base Set" }
      ]
    }
  ],
  "count": 1
}
```
`candidates` is present only for ambiguous matches.

---

### `GET /api/dh/intelligence`

Auth: RequireAuth

Returns market intelligence data for a specific card.

**Query params:** `card_name` (required), `set_name` (required), `card_number` (optional)

**Response:** `200 OK` — market intelligence object

**Errors:** `400` missing card_name or set_name; `404` no intelligence data found

---

### `GET /api/dh/suggestions`

Auth: RequireAuth

Returns the latest DH buy/sell suggestions.

**Response:** `200 OK`
```json
{
  "suggestions": [
    {
      "suggestionDate": "2025-01-15",
      "type": "cards",
      "category": "hottest_cards",
      "rank": 1,
      "isManual": false,
      "dhCardId": "12345",
      "cardName": "Charizard",
      "setName": "Base Set",
      "cardNumber": "4",
      "imageUrl": "https://...",
      "currentPriceCents": 120000,
      "confidenceScore": 0.95,
      "reasoning": "Strong demand...",
      "structuredReasoning": "{...}",
      "metrics": "{...}",
      "sentimentScore": 0.4,
      "sentimentTrend": 0.1,
      "sentimentMentions": 12,
      "fetchedAt": "2025-01-15T10:00:00Z"
    }
  ],
  "count": 10
}
```

`structuredReasoning` and `metrics` hold pre-encoded JSON and are strings on the
wire, not objects.

---

### `GET /api/dh/suggestions/inventory-alerts`

Auth: RequireAuth

Cross-references latest DH suggestions against current inventory. Returns only suggestions that match cards in your unsold inventory.

**Response:** `200 OK`
```json
{
  "alerts": [
    { ...Suggestion... }
  ],
  "count": 3
}
```

---

### `GET /api/dh/status`

Auth: RequireAuth

Returns aggregate stats for the DH integration including match counts, API health, and remote inventory/order counts.

**Response:** `200 OK`
```json
{
  "intelligence_count": 500,
  "intelligence_last_fetch": "2025-01-15T10:00:00Z",
  "suggestions_count": 20,
  "suggestions_last_fetch": "2025-01-15T10:00:00Z",
  "unmatched_count": 5,
  "dismissed_count": 2,
  "pending_count": 3,
  "mapped_count": 120,
  "bulk_match_running": false,
  "bulk_match_last_matched": 118,
  "bulk_match_last_failed": 2,
  "api_health": {
    "totalCalls": 100,
    "successRate": 98.5,
    "avgLatencyMs": 220
  },
  "dh_inventory_count": 130,
  "dh_listings_count": 80,
  "dh_orders_count": 45,
  "last_orders_poll_at": "2025-01-15T10:00:00Z",
  "orders_matched_count_24h": 12,
  "orders_orphan_count_24h": 1,
  "orders_already_sold_count_24h": 0,
  "pending_received_count": 2,
  "unenrolled_received_count": 0
}
```

This is the only endpoint in the API that uses `snake_case` keys throughout
(`dh_status_handler.go:168-199`) — it mirrors DH's own naming rather than the
camelCase used everywhere else. Do not "fix" a client that reads these keys.

`bulk_match_error` is `omitempty` and therefore **absent** when the last bulk
match succeeded; it carries the error string otherwise. `api_health`,
`dh_inventory_count`, `dh_listings_count`, `dh_orders_count`,
`last_orders_poll_at` and the three `orders_*_24h` counters are likewise
`omitempty`. The remaining keys are always present.

Two of the counts are deliberately different views of "pending", and the gap
between them is the diagnostic:

- `pending_count` — every row with `dh_push_status='pending'`.
- `pending_received_count` — what `GET /api/dh/pending` actually drains
  (`dh_push_status='pending'` **and** `received_at IS NOT NULL`). It lags
  `pending_count` when a CardLadder refresh has enrolled rows that have not been
  received yet; the difference points at the receipt gap.
- `unenrolled_received_count` — received, unsold rows carrying no push-pipeline
  state at all. Non-zero means Cert Intake is creating rows the DH sync cannot
  see; it should normally be `0`.

---

### `GET /api/dh/events`

Auth: RequireAuth

Returns the recorded DH pipeline state-transition trail for a single purchase or cert,
newest first. `/api/dh/status` answers "is the pipeline healthy"; this answers "why is
this one item stuck".

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `purchaseId` | one of | Purchase UUID. Mutually exclusive with `cert`. |
| `cert` | one of | PSA cert number. Reaches orphan events that carry no purchase id. |
| `limit` | no | Max rows (default 100, capped at 500). |

Exactly one of `purchaseId` or `cert` must be supplied — an unfiltered listing is
deliberately not offered.

**Response:** `200 OK`
```json
{
  "events": [
    {
      "id": 4821,
      "event_at": "2025-01-15T10:00:00Z",
      "purchase_id": "uuid",
      "cert_number": "12345678",
      "type": "pushed",
      "source": "dh_push",
      "prev_push_status": "pending",
      "new_push_status": "pushed",
      "dh_inventory_id": 67890,
      "dh_card_id": 12345,
      "sale_price_cents": 4500,
      "notes": ""
    }
  ],
  "count": 1,
  "limit": 100
}
```

Monetary values keep the explicit `_cents` suffix rather than converting to USD: this is
a diagnostic view of what was written to `dh_state_events`, and a reader comparing it
against the row wants the stored value. Empty fields are omitted.

**Errors:** `400` neither or both subjects supplied, or a non-positive/non-numeric
`limit`; `500` lookup failed; `503` DH event history not configured

Rows are pruned by the DH event cleanup scheduler (`DH_EVENT_RETENTION_DAYS`, default 90
days) — see [SCHEDULERS.md](SCHEDULERS.md).

---

### `POST /api/dh/fix-match`

Auth: RequireAuth

Manually resolves an unmatched card by pasting a DH URL. Parses the DH card ID, saves the mapping, pushes to DH inventory, and marks the purchase as manually matched.

**Body:**
```json
{
  "purchaseId": "uuid",
  "dhUrl": "https://doubleholo.com/card/12345/..."
}
```

**Response:** `200 OK`
```json
{
  "status": "ok",
  "dhCardId": 12345,
  "dhInventoryId": 67890
}
```

**Errors:** `400` missing fields, invalid URL, or no market value; `404` purchase not found; `502` DH API error

---

### `POST /api/dh/select-match`

Auth: RequireAuth

Selects one of the stored ambiguous candidates for a purchase. Validates the choice against stored candidates, pushes to DH inventory, and persists the match.

**Body:**
```json
{
  "purchaseId": "uuid",
  "dhCardId": 12345
}
```

**Response:** `200 OK`
```json
{
  "status": "ok",
  "dhCardId": 12345,
  "dhInventoryId": 67890
}
```

**Errors:** `400` invalid fields, card ID not among candidates, or no market value; `404` purchase not found; `502` DH API error

---

### `POST /api/dh/unmatch`

Auth: RequireAuth

Resets a matched purchase back to unmatched status, clearing the DH card ID, inventory ID, and stored candidates. Use this to manually correct a bad match before retrying.

**Body:**
```json
{ "purchaseId": "uuid" }
```

**Response:** `200 OK`
```json
{ "status": "ok" }
```

**Errors:** `400` missing purchaseId; `404` purchase not found; `500` DB error

---

### `POST /api/dh/retry-match`

Auth: RequireAuth

Re-runs the full DH match pipeline for an unmatched purchase. First attempts standard cert resolution; if not found or ambiguous with no candidates, falls back to PSA import. The purchase must be in `unmatched` status (call `/api/dh/unmatch` first if needed).

**Body:**
```json
{ "purchaseId": "uuid" }
```

**Response:** `200 OK`
```json
{
  "status": "ok",
  "dhCardId": 12345,
  "dhInventoryId": 67890
}
```

**Errors:** `400` missing purchaseId or purchase not in unmatched status; `404` purchase not found; `422` ambiguous match (candidates updated — use Select) or PSA import unavailable; `502` DH API error

---

### `POST /api/dh/approve/{purchaseId}`

Auth: RequireAuth

Approves a held DH push item. Clears the hold and re-queues the purchase for DH push.

**Path params:** `purchaseId` (purchase UUID)

**Response:** `200 OK`
```json
{ "status": "approved" }
```

**Errors:** `400` validation error; `404` purchase not found; `503` DH approve service not configured

---

### `GET /api/admin/dh-push-config`

Auth: RequireAdmin

Returns the current DH push safety configuration (thresholds for price swing, disagreement, etc.).

**Response:** `200 OK` — `DHPushConfig`
```json
{
  "swingPctThreshold": 20,
  "swingMinCents": 5000,
  "disagreementPctThreshold": 25,
  "unreviewedChangePctThreshold": 15,
  "unreviewedChangeMinCents": 3000,
  "initialPushValueFloorPct": 50,
  "listingsPaused": false,
  "updatedAt": "2025-01-01T00:00:00Z"
}
```

`listingsPaused: true` short-circuits `ListPurchases` before the inline
`psa_import` push: nothing is sent to DoubleHolo and the `in_stock` → `listed`
flip is skipped, so items stay unlisted on DH. It exists for card-show
liquidation windows where local sales should not be undercut by live DH
listings (`dh_types.go:13-18`).

**Errors:** `503` DH approve service not configured

---

### `PUT /api/admin/dh-push-config`

Auth: RequireAdmin

Saves the DH push safety configuration.

**Body:** `DHPushConfig` object (same shape as GET response)

**Response:** `200 OK` — updated `DHPushConfig`

**Errors:** `400` validation error; `503` DH approve service not configured

---

### `POST /api/admin/dh-reconcile/trigger`

Auth: RequireAdmin

Runs the DH inventory reconciler synchronously, scanning for purchases that were
deleted on DH and resetting their push state so the push pipeline can re-enrol
them. Mirrors the hourly scheduler's work on demand.

**Body:** (empty)

**Response:** `200 OK` — `ReconcileResult`
```json
{
  "scanned": 42,
  "missingOnDH": 3,
  "reset": 3,
  "errors": [],
  "resetIds": ["p-123", "p-456", "p-789"]
}
```

**Errors:** `502` reconcile run failed; `503` DH reconciler not configured

---

## Admin Price Flags

### `GET /api/admin/price-flags`

Auth: RequireAdmin

Lists price flags for admin review.

**Query params:** `status` (optional: `open`, `resolved`, `all`; defaults to `open`)

**Response:** `200 OK`
```json
{
  "flags": [
    {
      "id": 1,
      "purchaseId": "uuid",
      "flaggedBy": 1,
      "flaggedAt": "2025-01-01T00:00:00Z",
      "reason": "wrong_match",
      "cardName": "Charizard",
      "setName": "Base Set",
      "cardNumber": "4",
      "grade": 10,
      "certNumber": "12345678",
      "flaggedByEmail": "user@example.com",
      "marketPriceCents": 120000,
      "clValueCents": 115000,
      "reviewedPriceCents": 0,
      "sourcePrices": []
    }
  ],
  "total": 1
}
```

Each element is a `PriceFlagWithContext`, which embeds `PriceFlag` — so `id`,
`purchaseId`, `flaggedBy`, `flaggedAt`, `reason`, `resolvedAt` and `resolvedBy`
are promoted from the embedded type (`price_flags.go:43-51`). `resolvedAt` and
`resolvedBy` are `omitempty` pointers: they are **absent** for open flags, and
populated only in the `status=resolved` and `status=all` responses. `setName`,
`cardNumber` and `sourcePrices` are also `omitempty`.

**Errors:** `400` invalid status value

---

### `PATCH /api/admin/price-flags/{flagId}/resolve`

Auth: RequireAdmin

Resolves a price flag.

**Path params:** `flagId` (int64)

**Response:** `204 No Content`

**Errors:** `400` invalid flag ID; `404` flag not found or already resolved

---

## Admin — CardLadder

### `POST /api/admin/cardladder/config`

Auth: RequireAdmin

Authenticates with Card Ladder via Firebase and stores the refresh token.

**Body:**
```json
{
  "email": "user@example.com",
  "password": "secret",
  "collectionId": "collection-uuid",
  "firebaseApiKey": "AIza..."
}
```

**Response:** `200 OK`
```json
{ "status": "connected" }
```

**Errors:** `400` missing required fields; `401` Firebase authentication failed

---

### `GET /api/admin/cardladder/status`

Auth: RequireAdmin

Returns the current Card Ladder connection status including mapped card count and last refresh run stats.

**Response:** `200 OK`
```json
{
  "configured": true,
  "email": "user@example.com",
  "collectionId": "collection-uuid",
  "cardsMapped": 120,
  "priceStats": {
    "unsoldTotal": 200,
    "withCLValue": 180,
    "syncedCount": 175,
    "oldestUpdate": "2025-01-01T00:00:00Z",
    "newestUpdate": "2025-01-15T10:00:00Z",
    "staleCount": 12
  },
  "lastRun": {
    "lastRunAt": "2025-01-15T10:00:00Z",
    "durationMs": 150000,
    "totalPurchases": 200,
    "updated": 15,
    "resolved": 3,
    "noCert": 1,
    "certResolveFailed": 0,
    "estimateFailed": 0,
    "noValue": 2,
    "quotaExhausted": false,
    "skippedQuota": 0,
    "cardsPushed": 4,
    "cardsRemoved": 1
  }
}
```

`email` and `collectionId` are present only when `configured` is `true`.
`cardsMapped`, `priceStats`, and `lastRun` are each omitted if their underlying
lookup fails or, for `lastRun`, if no run has been recorded — the handler logs
and continues rather than failing the whole response.

---

### `POST /api/admin/cardladder/refresh`

Auth: RequireAdmin

Triggers a manual Card Ladder value sync.

**Response:** `200 OK`
```json
{ "status": "refresh complete" }
```

**Errors:** `503` Card Ladder refresh scheduler not available

---

### `POST /api/admin/cardladder/add-card`

Auth: RequireAdmin

Adds a single card to the Card Ladder collection by cert number.

**Body:**
```json
{
  "certNumber": "12345678",
  "grader": "psa",
  "investment": 840.00,
  "datePurchased": "2025-01-15"
}
```
`grader` defaults to `"psa"`. `datePurchased` is optional.

**Response:** `200 OK`
```json
{
  "certNumber": "12345678",
  "player": "Charizard",
  "set": "Base Set",
  "condition": "PSA 10",
  "estimatedValue": 1200.00,
  "status": "synced"
}
```

`status` is `"synced"`, `"skipped"`, or `"error"`; an `error` string field is
present only when `status` is `"error"`. `investment` (request) and
`estimatedValue` (response) are in **dollars**, not cents — this endpoint
speaks Card Ladder's units rather than SlabLedger's.

**Errors:** `400` missing certNumber; `412` Card Ladder not configured; `503` client not configured

---

### `POST /api/admin/cardladder/sync-to-cl`

Auth: RequireAdmin

Pushes all unsold purchases with cert numbers to the Card Ladder collection. Cards already present are skipped.

**Response:** `200 OK` — `CLSyncResult`
```json
{
  "synced": 10,
  "skipped": 5,
  "failed": 0,
  "total": 15,
  "results": [
    {
      "certNumber": "12345678",
      "player": "Charizard",
      "set": "Base Set",
      "condition": "PSA 10",
      "estimatedValue": 1200.00,
      "status": "synced"
    }
  ]
}
```

Each entry in `results` is the same shape as the `add-card` response: `status`
is `"synced"`, `"skipped"`, or `"error"`, with an `error` string present only
in the `"error"` case, and `estimatedValue` in **dollars**.

**Errors:** `412` Card Ladder not configured; `503` client or purchase lister not available

---

### `GET /api/admin/cardladder/failures`

Auth: RequireAdmin

Returns a breakdown of per-purchase Card Ladder mapping/sync failures for the admin UI. Query param `limit` (default 50, max 200) controls the sample list size.

**Response:** `200 OK` — `IntegrationFailuresReport`
```json
{
  "byReason": {
    "card_not_found": 3,
    "token_expired": 1
  },
  "samples": [
    {
      "purchaseId": "uuid",
      "certNumber": "12345678",
      "cardName": "Charizard",
      "reason": "card_not_found",
      "errorAt": "2026-04-13T10:00:00Z"
    }
  ]
}
```

**Errors:** `500` internal error

### `GET /api/admin/cardladder/coverage`

Auth: RequireAdmin

Reports CardLadder value coverage by purchase month and intake cohort.

Unlike `/status`, which summarizes freshness over live unsold inventory, this covers **all** purchases including sold ones and rows in closed campaigns.

Coverage is measured on `cl_value_updated_at`, not `cl_value_cents` — the latter is also written by the Shopify external import without ever calling CardLadder.

Buckets, evaluated in order:

| Bucket | Meaning |
|---|---|
| `resolved` | CardLadder returned a positive value at least once |
| `unresolved` | CardLadder was asked and failed (today: always `no_value`) |
| `pending` | Skipped at the quota wall, or not yet swept and still sweep-eligible |
| `stranded` | Created after CardLadder went live, never priced, and no longer reachable by the sweep (sold, or campaign closed). Non-zero is a data-quality alarm |
| `preCL` | Created before CardLadder existed (before `eraStart`); will never be swept |

`pct` is `resolved / (resolved + unresolved)`. `pending`, `stranded` and `preCL` are excluded from the denominator, so `rows` is a full total and does not equal it. `pct` is `null`, not `0`, when the denominator is empty.

`reassigned` counts rows whose `purchase_source` is set but whose `campaign_id` is `external` — the two possible cohort definitions disagreeing. It is reported so drift is visible.

**Response:** `200 OK` — `CLCoverageReport`
```json
{
  "eraStart": "2026-04-13T04:00:13Z",
  "months": [
    {
      "month": "2026-07",
      "reassigned": 0,
      "campaign": {"rows": 20, "resolved": 20, "unresolved": 0, "pending": 0, "stranded": 0, "preCL": 0, "pct": 100.0},
      "external": {"rows": 71, "resolved": 56, "unresolved": 15, "pending": 0, "stranded": 0, "preCL": 0, "pct": 78.9},
      "unresolvedByReason": {"no_value": 15}
    }
  ]
}
```

**Errors:** `500` internal error

The same query is available offline as `scripts/cl-coverage.sql`.

---

## Admin — PSA Sync

### `GET /api/admin/psa-sync/status`

Auth: RequireAdmin

Returns PSA sync configuration and last-run stats.

**Response:** `200 OK`
```json
{
  "configured": true,
  "interval": "24h",
  "pendingCount": 3,
  "snapshotFetchedAt": "2026-07-12T09:00:00Z",  // when the harvester last stored portal rows (omitted before first harvest)
  "lastRun": {
    "lastRunAt": "2025-01-15T10:00:00Z",
    "durationMs": 4500,
    "lastError": "",
    "allocated": 8,
    "updated": 2,
    "refunded": 0,
    "unmatched": 1,
    "ambiguous": 1,
    "skipped": 0,
    "failed": 0,
    "totalRows": 12
  }
}
```

`lastError` is omitted when the run succeeded. `pendingCount` and `lastRun` are
omitted when their lookup fails or no run has been recorded; `configured` reports
whether the portal sync is wired at all, which is separate from whether the daily
run is enabled via `PSA_SYNC_ENABLED`.

---

### `POST /api/admin/psa-sync/refresh`

Auth: RequireAdmin

Triggers a manual PSA sync cycle (fetches from Google Sheet and runs import pipeline).

**Body:** (empty)

**Response:** `200 OK`
```json
{ "status": "sync complete" }
```

**Errors:** `503` PSA sync scheduler not available; `500` sync failed

---

### `GET /api/admin/psa-sync/pending`

Auth: RequireAdmin

Lists all pending PSA import items awaiting manual resolution (ambiguous or unmatched).

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "uuid",
      "certNumber": "12345678",
      "cardName": "Charizard",
      "setName": "Base Set",
      "cardNumber": "4",
      "grade": 10,
      "buyCostCents": 84000,
      "purchaseDate": "2025-01-15",
      "status": "ambiguous",
      "candidates": ["Campaign A", "Campaign B"],
      "source": "scheduler",
      "createdAt": "2025-01-15T10:00:00Z"
    }
  ]
}
```

`status` is `"ambiguous"` or `"unmatched"`; `source` is `"scheduler"` or
`"manual"`. `PendingItem` also carries `resolvedAt` and `resolvedCampaignId`,
but this endpoint queries `WHERE resolved_at IS NULL`
(`postgres/pending_items.go:94`), so both are always absent here.

---

### `POST /api/admin/psa-sync/pending/{id}/assign`

Auth: RequireAdmin

Assigns a pending item to a campaign by creating a purchase and resolving the pending item.

**Path params:** `id` (pending item UUID)

**Body:**
```json
{ "campaignId": "uuid" }
```

**Response:** `200 OK` — `Purchase` object

**Errors:** `400` missing campaignId; `404` pending item not found; `409` cert number already exists; `503` purchase creation not available

---

### `DELETE /api/admin/psa-sync/pending/{id}`

Auth: RequireAdmin

Dismisses a pending item without creating a purchase.

**Path params:** `id` (pending item UUID)

**Response:** `204 No Content`

**Errors:** `404` pending item not found

---

## PSA Campaign Sync

See [docs/psa-harvester.md](psa-harvester.md#campaign-sync) for the harvester-side flow
(snapshot fetch, push-queue drain, `updateCampaign`). The endpoints below are the app's
read + human-approval surface; the app never contacts psacard.com directly.

### `POST /api/campaigns/{id}/psa-propose-create`

Auth: RequireAuth

Builds the full PSA portal `formData` for an internal campaign that is not yet linked to a portal campaign, and enqueues it as a pending `create` for human approval. The app does not contact psacard.com — the harvester drains the queue.

Rejected if the campaign is already linked, or if a create for it is already queued. A previously pushed-but-unlinked create is also rejected: the portal campaign already exists, so it should be linked manually (`psa-link`) rather than created twice.

**Path params:** `id` (campaign UUID)

**Body:** (empty)

**Response:** `200 OK`
```json
{
  "pushId": "uuid",
  "formData": { }
}
```
`formData` is the translated portal payload (`CampaignFormData`).

**Errors:** `400` campaign already linked to a PSA portal campaign, or the campaign could not be translated; `404` campaign not found; `409` a create is already queued, or one was already pushed but not linked; `503` PSA campaign sync not enabled, PSA catalog not enabled, or the catalog is stale — run `cmd/psa-harvest`; `500` internal error

---

### `GET /api/psa-campaigns`

Auth: RequireAuth

Returns the most recent PSA portal campaign snapshot (written by the harvester).

**Response:** `200 OK`
```json
{
  "campaigns": [
    {
      "campaignRequestId": "12345",
      "name": "Pokemon Vintage",
      "type": "CATEGORY",
      "status": "PAUSED",
      "category": "POKEMON",
      "buyPercentClv": 78,
      "buyBox": {
        "gradeMin": "8",
        "gradeMax": "10",
        "yearMin": 1998,
        "yearMax": 2003,
        "priceMinCents": 5000,
        "priceMaxCents": 50000,
        "clvConfidenceMin": 60,
        "buyerFlatFeeCents": 300
      },
      "dailyBudgetCents": 100000,
      "dailySpecLimit": 20,
      "subjectFilter": { "type": "Target", "subjects": [{ "id": 1, "name": "Charizard" }] },
      "publisherFilter": { "type": "Target", "subjects": [] },
      "createdAt": "2025-01-15T10:00:00Z",
      "updatedAt": "2025-01-15T10:00:00Z"
    }
  ],
  "fetchedAt": "2025-01-15T10:00:00Z"
}
```

**Errors:** `503` PSA campaign sync not enabled; `500` internal error

---

### `GET /api/psa/subjects`

Auth: RequireAuth

Returns the persisted PSA subject catalog (Pokemon category) harvested by
`cmd/psa-harvest`, for the campaign form's subject-name typeahead. Served
entirely from the database — the main server never contacts psacard.com.

**Response:** `200 OK`
```json
{
  "subjects": [{ "id": 22210, "name": "Machamp" }],
  "fetchedAt": "2026-08-06T10:00:00Z"
}
```

**Errors:** `503` PSA campaign sync not enabled; `500` internal error

---

### `POST /api/campaigns/{id}/psa-link`

Auth: RequireAuth

Links an internal campaign to a PSA portal campaign by request ID.

**Path params:** `id` (internal campaign UUID)

**Body:**
```json
{ "psaCampaignRequestId": "12345" }
```

**Response:** `200 OK` — full `Campaign` object (with `psaCampaignRequestId` set)

**Errors:** `404` campaign not found; `400` invalid campaign data; `500` internal error

---

### `POST /api/campaigns/{id}/psa-propose`

Auth: RequireAuth

Computes the diff between an internal campaign and its linked PSA portal campaign (from
the latest snapshot) and enqueues it as a `pending` row in the push queue for human
approval. If there are no changes, no row is enqueued.

**Path params:** `id` (internal campaign UUID)

**Body:** (empty)

**Response:** `200 OK`
```json
{
  "pushId": "uuid",
  "diff": {
    "changes": [
      { "field": "bidPercentage", "old": "75", "new": "78" }
    ]
  }
}
```
`pushId` is omitted when `diff.changes` is empty (nothing was enqueued).

**Errors:** `503` PSA campaign sync not enabled; `400` campaign not linked to a PSA
portal campaign; `404` campaign not found or linked PSA campaign not found in snapshot;
`500` internal error

---

### `POST /api/campaigns/{id}/psa-publish`

Auth: RequireAuth

Approves a pending push-queue row. This does not contact PSA directly — the row is
marked `approved` and the next harvester run drains it via `updateCampaign`.

**Path params:** `id` (internal campaign UUID)

**Body:**
```json
{ "pushId": "uuid", "approvedBy": "username" }
```

**Response:** `200 OK`
```json
{ "pushId": "uuid", "status": "approved" }
```

**Errors:** `503` PSA campaign sync not enabled; `409` push row is not pending; `500`
internal error

---

### `GET /api/psa-pushes`

Auth: RequireAuth

Returns the most recent push-queue row per internal campaign (any status), for the
UI's pending/in-flight/failed indicators and the publish modal. Creates echo the full
proposed `formData`; updates echo the field `diff`.

**Response:** `200 OK`
```json
{
  "pushes": [
    {
      "campaignId": "uuid",
      "pushId": "uuid",
      "operation": "create",
      "status": "pending",
      "formData": { "campaignName": "…", "bidPercentage": 72 },
      "requestedBy": "user@example.com",
      "updatedAt": "2026-07-14T12:00:00Z"
    },
    {
      "campaignId": "uuid",
      "pushId": "uuid",
      "operation": "update",
      "status": "failed",
      "error": "portal 500",
      "diff": { "changes": [ { "field": "bidPercentage", "old": "70", "new": "72" } ] },
      "requestedBy": "user@example.com",
      "approvedBy": "user@example.com",
      "updatedAt": "2026-07-14T13:00:00Z"
    }
  ]
}
```

**Errors:** `503` PSA campaign sync not enabled; `500` internal error

---

## Opportunities

### `GET /api/opportunities/acquisition`

Auth: RequireAuth

Returns raw-to-graded arbitrage opportunities across all campaigns.

**Response:** `200 OK` — Array of `AcquisitionOpportunity`
```json
[
  {
    "cardName": "Charizard",
    "setName": "Base Set",
    "cardNumber": "4",
    "certNumber": "12345678",
    "rawNMCents": 50000,
    "gradedEstimates": { "10": 120000, "9": 80000 },
    "bestGrade": "10",
    "bestGradedCents": 120000,
    "profitCents": 55000,
    "profitROI": 1.1,
    "source": "doubleholo"
  }
]
```

---

## Intelligence

### `GET /api/intelligence/niches`

Auth: RequireAuth

Returns ranked acquisition-niche opportunities: `(character, era, grade)` buckets where consumer demand is strong relative to our current campaign coverage. Backed by cached DoubleHolo analytics data refreshed daily by the DH analytics scheduler.

**Query parameters** (all optional):

| Param | Values | Default | Notes |
|---|---|---|---|
| `window` | `7d`, `30d` | `30d` | Demand/analytics aggregation window |
| `limit` | 1..200 | 50 | Clamped to 200 |
| `sort` | `opportunity_score`, `demand_score`, `velocity_change_pct`, `low_coverage` | `opportunity_score` | |
| `min_data_quality` | `""`, `proxy`, `full` | `""` | When `full`, excludes proxy-only rows — use post-launch-gate |
| `era` | DH era enum (e.g. `sword_shield`) | — | Optional filter |
| `grade` | 7..10 | — | Optional filter; 0 or missing = all grades |

**Response:** `200 OK`
```json
{
  "opportunities": [
    {
      "character": "Umbreon",
      "era": "sword_shield",
      "grade": 10,
      "demand": {
        "score": 0.82,
        "views": 843,
        "wishlist_adds": 47,
        "data_quality": "proxy",
        "computed_at": "2026-04-15T02:00:00Z"
      },
      "market": {
        "median_days_to_sell": 9.8,
        "velocity_change_pct": 15.2,
        "active_listing_count": 42,
        "sample_size": 312,
        "analytics_not_computed": false,
        "computed_at": "2026-04-15T03:15:00Z"
      },
      "coverage": {
        "our_unsold_count": 2,
        "active_campaign_ids": [],
        "covered": false
      },
      "opportunity_score": 0.64
    }
  ],
  "meta": {
    "window": "30d",
    "limit": 50,
    "sort": "opportunity_score",
    "total_count": 387
  }
}
```

`demand` is `null` when no demand data is cached for the bucket. `market` is `null` when analytics returned `404 analytics_not_computed` (normal during DH's initial pipeline warmup). `market.median_days_to_sell` and `market.velocity_change_pct` are `null` when the underlying JSON lacked those fields.

**Errors:** `400` invalid `window`, `sort`, or out-of-range `grade`; `500` internal


---

### `GET /api/intelligence/campaign-signals`

Auth: RequireAuth

Returns per-campaign demand-velocity signals derived from the cached DH analytics rows: for each campaign, how many of its tracked characters are accelerating vs decelerating, and the strongest movers in each direction.

**Response:** `200 OK`
```json
{
  "computed_at": "2025-01-01T00:00:00Z",
  "data_quality": "ok",
  "signals": [
    {
      "campaign_id": "uuid",
      "campaign_name": "Pokemon Base 2025 Q1",
      "tracked_characters": 12,
      "accelerating_count": 5,
      "decelerating_count": 3,
      "median_velocity_change_pct": 0.08,
      "data_quality": "ok",
      "computed_at": "2025-01-01T00:00:00Z",
      "top_accelerating": [
        {
          "character": "Charizard",
          "velocity_change_pct": 0.34,
          "median_days_to_sell": 11.5,
          "sample_size": 42
        }
      ],
      "top_decelerating": []
    }
  ]
}
```
`computed_at` (top level and per signal) is `null` when no cached analytics rows carried a timestamp. `median_days_to_sell` is `null` when the cached row lacked the field. Unparseable cache rows are skipped and logged rather than failing the request.

**Errors:** `500` internal error
