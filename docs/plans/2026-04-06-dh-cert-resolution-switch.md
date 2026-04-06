# Migrate DH Client from Integration API to Enterprise API

**Date:** 2026-04-06
**Status:** Approved

## Problem

The integration API (`/api/v1/integrations/*`) was a temporary measure before the enterprise API existed. The integration Match endpoint returns `success: false` for all queries, causing 100% unmatched items in production. The enterprise API (`/api/v1/enterprise/*`) has equivalents for every endpoint we use.

## Solution

Replace all integration API calls with enterprise equivalents, then remove the integration API key, methods, and types.

## Endpoint Migration

| Current (integration) | New (enterprise) | Auth change |
|---|---|---|
| `POST /integrations/match` | `POST /enterprise/certs/resolve` | `X-Integration-API-Key` → `Bearer` |
| `GET /integrations/market_data/{id}?tier=tier3` | `GET /enterprise/cards/lookup?card_id={id}` | Same |
| `GET /integrations/suggestions` | `GET /enterprise/suggestions` | Same |
| `GET /integrations/catalog/search` | Dead code — remove | — |

Note: For matching, we switch to cert resolution (not enterprise match) because cert resolution takes structured data and returns a definitive dh_card_id. The enterprise match still uses title-based fuzzy matching.

## Changes by File

### 1. `internal/adapters/clients/dh/client.go`

**Remove:** `Match`, `Search`, `MarketData`, `Suggestions` methods and the `get`/`post` helpers (integration-key auth).
**Remove:** `apiKey` field, `Available()` method, `apiKeyHeader` constant.

**Add:** `CardLookup(ctx, cardID int) (*CardLookupResponse, error)` — calls `GET /enterprise/cards/lookup?card_id={id}`, uses enterprise Bearer auth.

**Modify:** `Suggestions` — change from `get()` (integration auth) to `getEnterprise()` (bearer auth), update URL to `/enterprise/suggestions`.

`ResolveCert` already exists and uses enterprise auth. No change needed.

### 2. `internal/adapters/clients/dh/types.go`

**Remove:** `MatchRequest`, `MatchResponse`, `SearchResponse`, `SearchCard`.

**Modify:** `MarketDataResponse` — update fields to match enterprise `cards/lookup` response schema:
- Wrap in `CardLookupResponse` with `card` and `market_data` objects
- `market_data` fields change: `current_price` → `mid_price`, add `best_bid`, `best_ask`, `spread`, `last_sale`, etc.

### 3. Match callers (3 sites)

**`dh_match_handler.go` — `runBulkMatch`:**
- Change interface from `DHMatchClient.Match` to `DHCertResolver.ResolveCert`
- Iterate over purchases directly (not deduplicated card identities) since cert resolution needs a cert_number
- Build `CertResolveRequest` from purchase: cert_number, card_name (cleaned), set_name, card_number, year
- Replace confidence check with `resp.Status != "matched"` → mark unmatched

**`dh_push.go` — `processPurchase`:**
- Same interface swap
- Already iterates per-purchase, direct replacement

**`campaigns_dh_listing.go` — `inlineMatchAndPush`:**
- Same interface swap

### 4. MarketData callers (2 sites)

**`fusionprice/dh_adapter.go`:**
- Change `DHMarketDataClient` interface to use new `CardLookup` method
- Map new response fields to existing fusion price format

**`scheduler/dh_intelligence_refresh.go`:**
- Change client method from `MarketData` to `CardLookup`
- Map new response fields to intelligence store format

### 5. Suggestions caller (1 site)

**`scheduler/dh_suggestions.go`:**
- No interface change needed if response shape is the same
- Just the client method switches from integration to enterprise auth internally

### 6. Card name cleaning helper

Add to `internal/domain/campaigns/`:
- Strip `-HOLO` suffix → pass as `variant: "Holo"` in CertResolveRequest
- Strip `-REVERSE HOLO` → pass as `variant: "Reverse Holo"`

### 7. Wiring (`main.go`, `init.go`)

- Remove `cfg.Adapters.DHKey` / `DH_INTEGRATION_API_KEY` references
- `Available()` → `EnterpriseAvailable()` for all client availability checks
- All guards that check `dhClient.Available()` switch to `dhClient.EnterpriseAvailable()`

### 8. Config cleanup

- Remove `DH_INTEGRATION_API_KEY` from `config/loader.go`, `config/types.go`, `.env.example`
- `NewClient` no longer needs `apiKey` parameter

### 9. Dead code removal

- `Match`, `Search`, `MarketData`, `Suggestions` methods on `dh.Client`
- `get()`, `post()` helpers (integration-key auth)
- `apiKey` field, `Available()`, `apiKeyHeader` constant
- `MatchRequest`, `MatchResponse`, `SearchResponse`, `SearchCard` types
- `DHMatchConfidenceThreshold` constant in campaigns
- `BuildDHMatchTitle` function in campaigns
- `TestClient_Match`, `TestClient_Search`, `TestClient_NotAvailable` tests

### 10. Tests

- Update bulk match handler tests to mock `ResolveCert`
- Update push scheduler tests to mock `ResolveCert`
- Update DH adapter test to mock `CardLookup`
- Update `client_test.go` — remove integration tests, add enterprise equivalents
- Integration test `TestDHEnterprise_ResolveCert` already exists

## Not Changing

- `ResolveCert`, `ResolveCertsBatch`, `GetCertResolutionJob` — already enterprise
- `PushInventory`, `ListInventory`, `UpdateInventory` — already enterprise
- `GetOrders` — already enterprise
- `SyncChannels`, `DelistChannels` — already enterprise
- Manual fix-match handler (`dh_fix_match_handler.go`)
- DH inventory poll / orders poll schedulers
