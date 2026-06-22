# PSA Portal Spike Findings (run 2026-06-22 from devcontainer)

## Finding 1 — Collectors OAuth ✅ endpoint resolved / ⚠️ refresh client_id OPEN

Stytch Connected-Apps OAuth.
- `token_endpoint` = `https://auth.collectors.com/oauth2/v1/apps/token`
- `grant_types_supported` includes `refresh_token`
- `jwks_uri` = `https://auth.collectors.com/P2xjgYJ0Yv0TEWIKhBaivYmYlqcQ/.well-known/jwks.json`

**OPEN:** `grant_type=refresh_token` returns `400 E074130 "Invalid client id"` for both
`P2xjgYJ0Yv0TEWIKhBaivYmYlqcQ` and the base64 `cid`/`azp` literal. The real registered
Stytch Connected-App `client_id` is NOT derivable from the JWT claims. Need to capture the
actual refresh/login token request the browser makes (DevTools → Network, the POST to
`oauth2/v1/apps/token`) to read its `client_id` (and whether a `client_secret`/PKCE is used).
Interim: the access token works directly for ~24h (see Finding 3).

## Finding 2 — Lightdash embed ✅ RESOLVED

- Header: **`Lightdash-Embed-Token`** (HTTP 200 confirmed).
- Enumerate tiles: `POST /api/v1/embed/{projectUuid}/dashboard` body `{}`.
- Pull rows:   `POST /api/v1/embed/{projectUuid}/chart-and-results`
  body `{"tileUuid":"<uuid>","dashboardFilters":{"dimensions":[],"metrics":[],"tableCalculations":[]},"dashboardSorts":[]}`
  (the `query/dashboard-tile` route additionally *requires* `dashboardSorts`; `chart-and-results` is simpler — use it.)
- projectUuid: `e4995db3-cb94-4a66-9b19-7bb36f156e33` (derive from embedUrl at runtime).
- dashboard: "PSA Partner Offers_Buyer Reporting Embed" (`0d03183c-074b-4d93-a1ab-8f061bd4406b`).

### Tiles (8)
| tile | uuid | role |
|---|---|---|
| Itemized Purchases | `da5a4113-72cf-405f-9cbb-f829a27ff148` | **primary — per-cert rows** |
| Shipments | `b245958c-57ea-4795-b781-cbeaa8bf8706` | ship/tracking detail |
| Refunded Items | `994f1909-c030-4a60-bae1-5dc8bba67c5e` | refunds |
| Items Awaiting Shipment | `bdd84ae0-2a6b-4bbb-b0b0-598f8f25d502` | in-transit |
| Daily Items Purchased / Daily Spend / Weekly Summary / Packing Slip | — | aggregates (ignore) |

### Itemized Purchases → PSAExportRow column mapping
| PSAExportRow | Lightdash field key | kind |
|---|---|---|
| CertNumber | `fct_instantoffers_offers_cert_number` | dim |
| ListingTitle | `marketplace_listings_listing_title` | dim |
| Grade | `fct_instantoffers_offers_grade_value` | dim |
| PricePaid | `marketplace_listings_total_listing_final_price_metric` | metric |
| PurchaseSource | `fct_instantoffers_offers_origination_source` | dim |
| Date | `marketplace_listings_buyer_payment_date_pst_day` | dim |
| ShipDate | `vault_withdrawal_items_shipment_date_day` | dim |
| WasRefunded | `fct_instantoffers_offers_is_offer_refunded` | dim |
| FrontImageURL | `dim_ims_inventory_front_image_url` | dim |
| BackImageURL | `dim_ims_inventory_back_image_url` | dim |
| Category | `dim_ims_inventory_set_sport_detailed` | dim |
| (extra) tracking | `vault_withdrawal_items_tracking_id` | dim |
| (extra) spec desc | `dim_ims_inventory_spec_description` | dim |
| (extra) collectible id | `fct_instantoffers_offers_collectors_collectible_id` | dim |

**GAP:** no `InvoiceDate` dimension. Confirm whether `buyer_payment_date_pst_day` should
populate `InvoiceDate` (it drove invoice creation in the old sheet) once real rows exist.

### Result/row shape
`chart-and-results` → `results.rows` = list of objects keyed by field id; each cell is
`{ "value": { "raw": <typed>, "formatted": <string> } }`. **0 rows today** (no purchases /
paused), so exact value types + date string format are UNCONFIRMED until data flows.
Mapper must read `row[fieldId]["value"]["raw"]`.

## Finding 3 — Cloudflare ✅ PASSES

`GET https://www.psacard.com/buyercampaignmanager/analytics/__data.json` with only
`Cookie: accessToken=<AT>` + browser UA → **HTTP 200, application/json**, from the
devcontainer with **no `cf_clearance`**. Backend cron is not Cloudflare-blocked. The
access-token cookie alone authenticates the analytics hop. Approach A confirmed viable.

## Net status
- [x] Finding 2 (Lightdash) — fully resolved
- [x] Finding 3 (Cloudflare) — PASSES
- [~] Finding 1 — token endpoint known; **real refresh `client_id` still needed** for
  hands-off automation (only remaining blocker). Access token works 24h as interim.
- [ ] Confirm value types / date format + InvoiceDate mapping once real rows exist (A1).
