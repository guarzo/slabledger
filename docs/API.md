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
  "providers": { "cards": true, "prices": true },
  "database": "healthy"
}
```

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

Returns API call statistics for all pricing providers (cardhedger, pricecharting).

**Response:** `200 OK`
```json
{
  "providers": [
    {
      "name": "cardhedger",
      "today": {
        "calls": 50,
        "limit": 1000,
        "remaining": 950,
        "successRate": 98.0,
        "avgLatencyMs": 220.5,
        "rateLimitHits": 0,
        "minuteCalls": 3
      },
      "blocked": false,
      "lastCallAt": "2025-01-01T00:00:00Z"
    }
  ],
  "timestamp": "2025-01-01T00:00:00Z"
}
```

---

### `GET /api/admin/cache-stats`

Auth: RequireAdmin

Returns card data cache statistics from the TCGdex provider.

**Response:** `200 OK` — `cards.CacheStats` object with `enabled` bool and hit/miss counts.

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

## Favorites

### `GET /api/favorites`

Auth: RequireAuth

Lists the current user's saved cards (paginated).

**Query params:** `page` (int, default 0), `page_size` (int, default 20, max 100)

**Response:** `200 OK` — `FavoritesList`
```json
{
  "favorites": [
    {
      "id": 1,
      "user_id": 1,
      "card_name": "Charizard",
      "set_name": "Base Set",
      "card_number": "4",
      "image_url": "https://...",
      "notes": "",
      "created_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 0,
  "page_size": 20,
  "total_pages": 1
}
```

---

### `POST /api/favorites`

Auth: RequireAuth

Adds a card to favorites.

**Body:**
```json
{
  "card_name": "Charizard",
  "set_name": "Base Set",
  "card_number": "4",
  "image_url": "https://...",
  "notes": ""
}
```

**Response:** `201 Created` — `Favorite` object

**Errors:** `400` invalid data; `409` already favorited

---

### `DELETE /api/favorites`

Auth: RequireAuth

Removes a card from favorites.

**Query params:** `card_name` (required), `set_name` (required), `card_number` (optional)

**Response:** `204 No Content`

**Errors:** `400` missing required params; `404` not found

---

### `POST /api/favorites/toggle`

Auth: RequireAuth

Toggles favorite status.

**Body:** `FavoriteInput` (same as POST /api/favorites)

**Response:** `200 OK`
```json
{ "is_favorite": true }
```

---

### `POST /api/favorites/check`

Auth: RequireAuth

Checks favorite status for up to 100 cards in one request.

**Body:** Array of `FavoriteInput` objects (max 100)

**Response:** `200 OK` — Array of `FavoriteCheck`
```json
[
  { "card_name": "Charizard", "set_name": "Base Set", "card_number": "4", "is_favorite": true }
]
```

**Errors:** `400` more than 100 items

---

## Cards & Pricing

### `GET /api/cards/search`

Auth: RequireAuth

Searches cards by name, set, or number. Also accepts `POST` with JSON body.

**Query params (GET):** `q` (string, required)

**Body (POST):**
```json
{ "query": "Charizard Base Set", "limit": 10 }
```
`limit` defaults to 10, max 100.

**Response:** `200 OK`
```json
{
  "cards": [
    {
      "id": "tcgdex-id",
      "name": "Charizard",
      "number": "4",
      "set": "base1",
      "setName": "Base Set",
      "rarity": "Rare Holo",
      "imageUrl": "https://...",
      "marketPrice": 350.00,
      "score": 0.95
    }
  ],
  "total": 1
}
```

---

### `GET /api/cards/pricing`

Auth: RequireAuth

Looks up fusion price data for a specific card.

**Query params:** `name` (required, max 200), `set` (optional, max 200), `number` (optional, max 50)

**Response:** `200 OK`
```json
{
  "card": "Charizard",
  "set": "Base Set",
  "number": "4",
  "rawUSD": 120.00,
  "psa8": 250.00,
  "psa9": 400.00,
  "psa10": 1200.00,
  "confidence": 0.92,
  "matchQuality": "good",
  "conservativePsa10": 1100.00,
  "conservativePsa9": 370.00,
  "conservativeRaw": 110.00,
  "lastSold": {
    "psa10": { "lastSoldPrice": 1250.00, "lastSoldDate": "2025-01-01", "saleCount": 5 }
  },
  "gradeData": {
    "10": {
      "ebay": { "price": 1200.00, "confidence": "high", "salesCount": 12, "trend": "up", "median": 1195.00 },
      "estimate": { "price": 1180.00, "low": 1100.00, "high": 1280.00, "confidence": 0.9 }
    }
  },
  "market": { "activeListings": 8, "lowestListing": 1100.00, "sales30d": 4, "sales90d": 12 },
  "velocity": { "dailyAverage": 0.15, "weeklyAverage": 1.0, "monthlyTotal": 4 },
  "sources": ["cardhedger"]
}
```

**Errors:** `400` missing name; `404` no pricing data found; `503` pricing not available

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
    "provider": "pricecharting",
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
  "provider": "pricecharting",
  "externalId": "12345"
}
```
`provider` must be `"pricecharting"` or `"cardhedger"`.

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

### `GET /api/admin/pricing-diagnostics`

Auth: RequireAdmin

Returns pricing data quality diagnostics (coverage rates, match quality distribution, etc.).

**Response:** `200 OK` — `pricing.PricingDiagnostics` object

---

### `GET /api/admin/card-requests`

Auth: RequireAdmin

Lists all CardHedger card request submissions. Enriches pending rows from current purchases before returning.

**Response:** `200 OK` — Array of `sqlite.CardRequestSubmission`

---

### `POST /api/admin/card-requests/{id}/submit`

Auth: RequireAdmin

Submits a single pending card request to CardHedger.

**Path params:** `id` (int64)

**Response:** `200 OK`
```json
{ "status": "submitted", "requestId": "ch-req-abc123" }
```

**Errors:** `400` invalid ID; `404` not found; `409` already claimed; `503` client not configured; `502` CardHedger API error

---

### `POST /api/admin/card-requests/submit-all`

Auth: RequireAdmin

Submits all pending card requests to CardHedger.

**Response:** `200 OK`
```json
{ "submitted": 5, "errors": 0 }
```

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
    "phase": "active",
    "psaSourcingFeeCents": 300,
    "ebayFeePct": 0.1235,
    "expectedFillRate": 80.0,
    "createdAt": "2025-01-01T00:00:00Z",
    "updatedAt": "2025-01-01T00:00:00Z"
  }
]
```

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

**Body:** `Campaign` object

**Response:** `200 OK` — updated `Campaign`

**Errors:** `400` invalid data; `404` not found

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
    "createdAt": "2025-01-01T00:00:00Z",
    "updatedAt": "2025-01-01T00:00:00Z"
  }
]
```

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
    "wasCracked": false,
    "createdAt": "2025-02-01T00:00:00Z",
    "updatedAt": "2025-02-01T00:00:00Z"
  }
]
```

---

### `POST /api/campaigns/{id}/sales`

Auth: RequireAuth

Records a sale for a purchase within this campaign. Computes `netProfitCents`, `daysToSell`, and `saleFeeCents` automatically.

**Path params:** `id` (campaign UUID)

**Body:** `Sale` object — required: `purchaseId`, `saleChannel`, `salePriceCents`, `saleDate`

Valid `saleChannel` values: `ebay`, `tcgplayer`, `local`, `other`, `gamestop`, `website`, `cardshow`

**Response:** `201 Created` — `Sale` object (may include `warnings` array)

**Errors:** `400` invalid data or purchase belongs to different campaign; `404` purchase or campaign not found; `409` sale already exists for this purchase

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
    { "purchaseId": "uuid", "salePriceCents": 130000 }
  ]
}
```

**Response:** `201 Created` — `BulkSaleResult`
```json
{
  "created": 5,
  "failed": 0,
  "errors": []
}
```

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

**Response:** `200 OK` — Array of `DailySpend`
```json
[
  {
    "date": "2025-01-15",
    "spendCents": 84000,
    "capCents": 1000000,
    "fillRatePct": 0.084,
    "purchaseCount": 1
  }
]
```

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

### `POST /api/campaigns/{id}/sell-sheet`

Auth: RequireAuth

Generates a sell sheet for selected purchases.

**Path params:** `id` (campaign UUID)

**Body:**
```json
{ "purchaseIds": ["uuid1", "uuid2"] }
```
At least one purchaseId required.

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

---

### `GET /api/campaigns/{id}/tuning`

Auth: RequireAuth

Returns parameter tuning recommendations for a campaign (buy terms, fee rates, etc.).

**Response:** `200 OK` — campaign tuning data object

**Errors:** `404` campaign not found

---

### `GET /api/campaigns/{id}/crack-candidates`

Auth: RequireAuth

Returns cards that may be worth cracking from their slabs (raw value exceeds slabbed value net of cracking cost).

**Response:** `200 OK` — Array of `CrackAnalysis`

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

### `POST /api/purchases/refresh-cl`

Auth: RequireAuth

Refreshes CL values across all campaigns from a Card Ladder export CSV upload.

**Body:** `multipart/form-data` — `file` field (CSV, max 10MB)

Required CSV columns: `slab serial #`, `current value`

**Response:** `200 OK` — `GlobalCLRefreshResult`
```json
{
  "updated": 15,
  "notFound": 2,
  "failed": 0,
  "results": [
    { "certNumber": "12345678", "status": "updated", "oldValueCents": 110000, "newValueCents": 120000 }
  ],
  "byCampaign": { "uuid": { "campaignName": "Q1", "updated": 15 } }
}
```

---

### `POST /api/purchases/import-cl`

Auth: RequireAuth

Imports and auto-allocates purchases from a Card Ladder export CSV.

**Body:** `multipart/form-data` — `file` field (CSV, max 10MB)

Required CSV columns: `slab serial #`, `investment`, `current value`

Optional columns: `date purchased` (M/D/YYYY), `card`, `player`, `set`, `number`, `condition`, `population`

**Response:** `200 OK` — `GlobalImportResult`
```json
{
  "allocated": 10,
  "refreshed": 5,
  "unmatched": 2,
  "ambiguous": 1,
  "skipped": 0,
  "failed": 0,
  "results": [
    { "certNumber": "12345678", "status": "allocated", "campaignId": "uuid", "campaignName": "Q1" }
  ]
}
```

---

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

### `GET /api/purchases/export-cl`

Auth: RequireAuth

Exports all unsold inventory as a Card Ladder import-format CSV.

**Query params:** `missing_cl_only=true` (optional, export only items without CL values)

**Response:** `200 OK` with `Content-Type: text/csv`, `Content-Disposition: attachment; filename="card_ladder_import.csv"`

CSV columns: `Date Purchased`, `Cert #`, `Grader`, `Investment`, `Estimated Value`, `Notes`, `Date Sold`, `Sold Price`

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
  "results": [
    { "certNumber": "12345678", "status": "imported", "cardName": "Charizard" }
  ]
}
```

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
  "failed": 0,
  "errors": []
}
```

---

### `GET /api/purchases/export-ebay`

Auth: RequireAuth

Returns unsold inventory items for eBay export review.

**Query params:** `flagged_only=true` (optional)

**Response:** `200 OK` — `EbayExportListResponse`
```json
{
  "items": [
    {
      "purchaseId": "uuid",
      "certNumber": "12345678",
      "cardName": "Charizard",
      "setName": "Base Set",
      "gradeValue": 10,
      "grader": "PSA",
      "clValueCents": 120000,
      "marketMedianCents": 115000,
      "suggestedPriceCents": 120000,
      "hasCLValue": true,
      "hasMarketData": true
    }
  ]
}
```

---

### `POST /api/purchases/export-ebay/generate`

Auth: RequireAuth

Generates an eBay bulk listing CSV file.

**Body:**
```json
{
  "items": [
    { "purchaseId": "uuid", "priceCents": 130000 }
  ]
}
```

**Response:** `200 OK` with `Content-Type: text/csv`, `Content-Disposition: attachment; filename=ebay_import.csv`

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
Valid `source` values: `manual`, `cost_markup`, `ai_accepted`

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

## Credit & Invoices

### `GET /api/credit/summary`

Auth: RequireAuth

Returns current credit position summary.

**Response:** `200 OK` — `CreditSummary`
```json
{
  "creditLimitCents": 2000000,
  "outstandingCents": 840000,
  "utilizationPct": 42.0,
  "refundedCents": 0,
  "paidCents": 500000,
  "unpaidInvoiceCount": 1,
  "alertLevel": "ok",
  "projectedExposureCents": 960000,
  "daysToNextInvoice": 8
}
```

---

### `GET /api/credit/config`

Auth: RequireAuth

Returns the current cashflow configuration.

**Response:** `200 OK` — `CashflowConfig`
```json
{
  "creditLimitCents": 2000000,
  "cashBufferCents": 200000,
  "updatedAt": "2025-01-01T00:00:00Z"
}
```

---

### `PUT /api/credit/config`

Auth: RequireAuth

Updates the cashflow configuration.

**Body:** `CashflowConfig` (same shape as GET response)

**Response:** `200 OK` — updated `CashflowConfig`

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
    "dueDate": "2025-01-30",
    "status": "unpaid",
    "createdAt": "2025-01-15T00:00:00Z",
    "updatedAt": "2025-01-15T00:00:00Z"
  }
]
```

---

### `PUT /api/credit/invoices`

Auth: RequireAuth

Updates an invoice (e.g. mark as paid, record payment).

**Body:** `Invoice` object with the `id` field set.

**Response:** `200 OK` — updated `Invoice`

**Errors:** `404` invoice not found

---

## Portfolio

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
    "createdAt": "2025-01-01T00:00:00Z"
  }
]
```

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
  "creditUtilizationPct": 42.0,
  "topPerformers": [...],
  "bottomPerformers": [...]
}
```

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

### `POST /api/shopify/price-sync`

Auth: RequireAuth

Matches Shopify inventory items against internal purchases and returns price recommendations.

**Body:**
```json
{
  "items": [
    { "certNumber": "12345678", "grader": "PSA", "currentPriceCents": 120000 }
  ]
}
```
Max 5,000 items.

**Response:** `200 OK` — `ShopifyPriceSyncResponse`
```json
{
  "matched": [
    {
      "certNumber": "12345678",
      "cardName": "Charizard",
      "grade": 10,
      "currentPriceCents": 120000,
      "suggestedPriceCents": 130000,
      "minimumPriceCents": 90000,
      "recommendation": "Increase price",
      "priceDeltaPct": 0.083
    }
  ],
  "unmatched": []
}
```

**Errors:** `400` no items / too many items (>5000)

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

### `POST /api/advisor/campaign-analysis`

Auth: RequireAuth

Streams a health and tuning narrative for a specific campaign.

**Body:**
```json
{ "campaignId": "uuid" }
```

**Response:** `200 OK` — SSE stream

**Errors:** `400` missing campaignId

---

### `POST /api/advisor/liquidation-analysis`

Auth: RequireAuth

Streams liquidation candidate recommendations across all campaigns.

**Body:** (empty)

**Response:** `200 OK` — SSE stream

---

### `POST /api/advisor/purchase-assessment`

Auth: RequireAuth

Streams an AI assessment of a potential card purchase.

**Body:**
```json
{
  "campaignId": "uuid",
  "cardName": "Charizard",
  "setName": "Base Set",
  "grade": "10",
  "buyCostCents": 84000,
  "clValueCents": 120000,
  "certNumber": "12345678"
}
```
`grade`, `buyCostCents`, `campaignId`, and `cardName` are required.

**Response:** `200 OK` — SSE stream

**Errors:** `400` missing required fields

---

### `GET /api/advisor/cache/{type}`

Auth: RequireAuth

Returns cached analysis result. `type` must be `digest` or `liquidation`.

**Path params:** `type` (`digest` | `liquidation`)

**Response:** `200 OK`
```json
{
  "status": "ready",
  "content": "## Weekly Digest...",
  "errorMessage": "",
  "updatedAt": "2025-01-01T00:00:00Z"
}
```
When no cache: `{ "status": "empty" }`

**Errors:** `400` invalid type; `503` caching not configured

---

### `POST /api/advisor/refresh/{type}`

Auth: RequireAuth

Triggers a background analysis refresh. Returns immediately.

**Path params:** `type` (`digest` | `liquidation`)

**Response:** `202 Accepted`
```json
{ "status": "running" }
```
If already running: `200 OK` — `{ "status": "running" }`

**Errors:** `400` invalid type; `503` caching not configured

---

## Social Content

### `GET /api/social/posts`

Auth: RequireAdmin

Lists social posts. Optionally filtered by status.

**Query params:** `status` (optional: `draft`, `publishing`, `published`, `failed`)

**Response:** `200 OK` — Array of `SocialPost` (up to 100)

---

### `GET /api/social/posts/{id}`

Auth: RequireAdmin

Returns a single social post with card detail.

**Path params:** `id` (UUID)

**Response:** `200 OK` — `SocialPost` detail object

**Errors:** `404` not found

---

### `POST /api/social/posts/generate`

Auth: RequireAdmin

Triggers async detection of publishable sales and AI caption generation. Returns immediately.

**Body:** (empty)

**Response:** `202 Accepted`
```json
{ "status": "generating" }
```

---

### `PATCH /api/social/posts/{id}/caption`

Auth: RequireAdmin

Updates a post's caption and hashtags.

**Path params:** `id` (UUID)

**Body:**
```json
{ "caption": "Updated caption text", "hashtags": "#pokemon #psa10" }
```

**Response:** `204 No Content`

---

### `DELETE /api/social/posts/{id}`

Auth: RequireAdmin

Deletes a social post and removes associated media files.

**Path params:** `id` (UUID)

**Response:** `204 No Content`

---

### `POST /api/social/backfill-images`

Auth: RequireAdmin

Triggers PSA slab image backfill for purchases missing images.

**Response:** `200 OK`
```json
{ "updated": 15, "errors": 2 }
```

**Errors:** `503` PSA image API not configured

---

### `POST /api/social/posts/{id}/regenerate-caption`

Auth: RequireAdmin

Regenerates the AI caption for a post via SSE streaming. Same event format as advisor endpoints.

**Path params:** `id` (UUID)

**Response:** `200 OK` — SSE stream

---

### `POST /api/social/posts/{id}/upload-slides`

Auth: RequireAdmin

Uploads rendered slide images (JPEG) for a post. Post must be in `draft` or `failed` status.

**Path params:** `id` (UUID)

**Body:** `multipart/form-data` — fields `slide-0` through `slide-9` (JPEG, max 8MB each, max 10 files total)

**Response:** `200 OK`
```json
{ "slides": 3 }
```

**Errors:** `400` invalid post ID, post in wrong status, non-JPEG file, or no slides; `404` post not found

---

### `GET /api/media/social/{postId}/{filename}`

Auth: None (public — required for Instagram API access)

Serves rendered slide image files.

**Path params:** `postId` (UUID), `filename` (`slide-N.jpg` where N is 0–9)

**Response:** `200 OK` with `Content-Type: image/jpeg`, `Cache-Control: public, max-age=86400`

**Errors:** `404` invalid ID format, invalid filename, or file not found

---

## Instagram

### `GET /api/instagram/status`

Auth: RequireAdmin

Returns Instagram connection status.

**Response:** `200 OK`
```json
{
  "connected": true,
  "username": "myaccount",
  "expiresAt": "2025-07-01T00:00:00Z",
  "connectedAt": "2025-01-01T00:00:00Z"
}
```
When not connected: `{ "connected": false }`

---

### `POST /api/instagram/connect`

Auth: RequireAdmin

Initiates Instagram OAuth flow. Returns the authorization URL.

**Response:** `200 OK`
```json
{ "url": "https://api.instagram.com/oauth/authorize?..." }
```

---

### `GET /auth/instagram/callback`

Auth: None (public OAuth redirect target)

Handles Instagram OAuth callback. Validates state, exchanges code for long-lived token, stores connection.

**Query params:** `code`, `state`, `error` (optional)

**Response:** `302 Found` → `/?instagram=connected` on success, `/?instagram=denied|invalid|invalid_state|exchange_failed|save_failed` on error

---

### `POST /api/instagram/disconnect`

Auth: RequireAdmin

Removes the Instagram connection (deletes stored token).

**Response:** `204 No Content`

---

### `POST /api/social/posts/{id}/publish`

Auth: RequireAdmin

Starts async publishing of a post to Instagram. Returns immediately.

**Path params:** `id` (post UUID)

**Response:** `202 Accepted`
```json
{ "status": "publishing" }
```

**Errors:** `400` post not found or not publishable; `503` Instagram not configured; `500` publish failed

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

Auth: RequireAuth (POST not required — uses GET)

Generates a global sell sheet across all active campaigns.

**Response:** `200 OK` — `SellSheet` object (same shape as campaign sell sheet, `campaignName` set per item)
