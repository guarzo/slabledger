# DH Integration Redesign: Incremental Inventory Push

**Date:** 2026-04-05
**Status:** Draft
**Depends on:** 2026-04-05-cert-import-ux-design.md (cert entry flow)

## Problem

The current DH integration uses a manual "Bulk Match" button that matches and pushes the entire unsold inventory to DH in one shot. This was designed as a one-time sync for initial onboarding, not as the steady-state flow. New cards entering the system via PSA import and CL import have no automatic path to DH inventory — they sit unmatched until someone remembers to run the bulk match again.

The desired steady state: cards flow to DH automatically as they acquire pricing data, and listings are created when the user physically receives the product (cert entry).

## Design

### Approach: Scheduler-Driven Push (Option C)

CL import sets a flag on purchases. A lightweight scheduler picks up flagged purchases every 5 minutes, matches them against DH's catalog, and pushes inventory. Cert entry creates listings on DH channels for cards that have been pushed.

This approach was chosen over inline-async (goroutine after CL import) because:
- **Retry for free** — transient DH API failures are retried on the next scheduler run
- **No goroutine lifecycle management** — avoids the atomic flags and waitgroups of the current bulk match pattern
- **Batching** — scheduler processes cards from multiple imports efficiently
- **Zero coupling** — CL import just flips a flag, no DH dependency

### Card Lifecycle Through DH

```
PSA Import          → Purchase created (no DH fields)
CL Import           → CLValueCents set, dh_push_status = "pending"
Push Scheduler (5m) → DH Match + PushInventory → dh_push_status = "matched" or "unmatched"
Cert Entry          → SyncChannels to all channels (eBay, Shopify, etc.)
Inventory Poll (2h) → Syncs back listing prices, channel status, DH status
Orders Poll (30m)   → Captures completed sales
```

For cards the scheduler hasn't processed yet at cert entry time, cert entry does an inline match + push + channel sync for that single card (~2-3s, acceptable for one card).

## Data Model

### New Field: `dh_push_status`

Added to the purchases table via migration 000031.

| Value | Meaning |
|---|---|
| `""` (empty) | Not eligible — no CL value yet, or legacy pre-redesign data |
| `"pending"` | Has CL value, awaiting DH match + push |
| `"matched"` | DH matched and inventory created successfully |
| `"unmatched"` | DH couldn't match (low confidence or API failure) |
| `"manual"` | User manually provided DH card ID via fix-match UI |

**Migration 000031:**
```sql
ALTER TABLE purchases ADD COLUMN dh_push_status TEXT NOT NULL DEFAULT '';
```

No changes to existing DH fields (`dh_card_id`, `dh_inventory_id`, `dh_listing_price_cents`, `dh_channels_json`, `dh_status`, `dh_cert_status`).

## Component Changes

### 1. CL Import (`service_import_cl.go`)

**Both `ImportCLExportGlobal` and `RefreshCLValuesGlobal`:**

After updating `CLValueCents` on a purchase, set `dh_push_status = 'pending'` if:
- The purchase does not already have a `DHInventoryID` (not yet pushed)
- The purchase does not already have `dh_push_status = 'unmatched'` or `'manual'` (don't re-queue manually handled or already-failed cards)

This means re-importing CL data for a card that was already pushed to DH is a no-op for DH status.

### 2. New Scheduler: DH Push (`dh_push.go`)

**Config:** `DHPushConfig{Enabled bool, Interval time.Duration}`
- Default interval: 5 minutes
- Gated by `cfg.DH.Enabled` + enterprise client available

**Per-run flow:**
1. Query purchases where `dh_push_status = 'pending'`, limit 50
2. Deduplicate by card identity (name + set + number) — if another purchase with the same identity already has a `DHCardID`, reuse it (skip the Match call)
3. For unresolved identities, call DH `Match(title, sku)` with 90% confidence threshold
4. If matched: save `DHCardID`, build `InventoryItem` with `CLValueCents` as cost, call `PushInventory()`, save `DHInventoryID`, set `dh_push_status = 'matched'`
5. If not matched: set `dh_push_status = 'unmatched'`
6. Rate-limited by DH client's existing 1 RPS limiter

**Error handling:** Transient API errors (timeouts, 5xx) leave status as `'pending'` for automatic retry. Only confident "no match" results (successful API call, confidence < 90%) transition to `'unmatched'`.

### 3. Bulk Match Update (`dh_match_handler.go`)

Update the existing bulk match handler to populate `dh_push_status`:
- Successfully matched + pushed purchases: set to `'matched'`
- Low confidence or failed matches: set to `'unmatched'`

This enables the one-time production backfill to populate the status field for all existing inventory, feeding unmatched cards into the manual fix UI.

### 4. Cert Entry Enhancement (`service_cert_entry.go`)

When processing a cert at import time, before calling `triggerDHListing()`:

1. If `dh_push_status = 'pending'` (scheduler hasn't gotten to it yet):
   - Inline: call DH `Match()` for this single card
   - If matched: `PushInventory()`, save `DHCardID` + `DHInventoryID`, set status to `'matched'`
   - If not matched: set status to `'unmatched'` (listing skipped, user fixes later)
2. If `DHInventoryID` exists (scheduler already ran): proceed to `SyncChannels` on all channels as today
3. If `dh_push_status` is empty or `'unmatched'`: skip DH listing (no inventory to list)

Channel sync targets all configured channels (eBay, Shopify, etc.) by default.

### 5. New Endpoint: Fix Unmatched Card

**`POST /api/dh/fix-match`**

**Request:**
```json
{
  "purchaseId": "uuid",
  "dhUrl": "https://doubleholo.com/card/49097/mega-charizard-x-ex-pokemon-phantasmal-flames-125-125"
}
```

**Server-side processing:**
1. Parse DH card ID from URL — extract numeric segment after `/card/` (e.g., `49097`)
2. Validate URL format, reject if no card ID found
3. Save `DHCardID` on the purchase
4. Call `PushInventory()` with the card ID + `CLValueCents` as cost
5. Save returned `DHInventoryID`
6. Set `dh_push_status = 'manual'`

**Response:**
```json
{
  "status": "ok",
  "dhCardId": 49097,
  "dhInventoryId": "inv-abc-123"
}
```

**Errors:** 400 for invalid URL format, 404 for unknown purchase, 502 for DH API failure.

### 6. DH Status Endpoint Update (`GET /api/dh/status`)

Update `unmatched_count` to source from `dh_push_status = 'unmatched'` instead of the current "missing DHCardID" heuristic. Add `pending_count` for cards awaiting scheduler pickup.

```json
{
  "intelligence_count": 150,
  "intelligence_last_fetch": "2026-04-05T10:00:00Z",
  "suggestions_count": 20,
  "suggestions_last_fetch": "2026-04-05T08:00:00Z",
  "unmatched_count": 3,
  "pending_count": 12,
  "matched_count": 485,
  "bulk_match_running": false
}
```

### 7. Frontend: Unmatched Cards Fix UI

**Location:** DH Admin Tab, new section below the existing status summary.

**Table columns:** Cert Number | Card Name | Card Number | Set | Grade | CL Value | Fix (input)

**Fix column:** Text input where user pastes a DH URL. On blur or Enter, submits `POST /api/dh/fix-match`. On success, row disappears from the list.

**Validation:** Client-side regex check that URL matches `doubleholo.com/card/\d+` before submitting. Show inline error for invalid format.

**Endpoint:** `GET /api/dh/unmatched` already exists and returns unmatched cards — update it to filter by `dh_push_status = 'unmatched'` instead of missing `DHCardID`.

## What Stays Unchanged

- DH inventory poll scheduler (syncs prices/status back every 2h)
- DH orders poll scheduler (captures sales every 30min)
- DH suggestions scheduler (fetches buy/sell picks every 6h)
- DH intelligence refresh scheduler (refreshes market data every 1h)
- `triggerDHListing()` logic in cert entry (just gets invoked more reliably now)
- DH client API methods (all existing methods reused as-is)
- `ResolveCert`/`ResolveCertsBatch` remain unused (not needed for this flow)

## Bulk Match Deprecation Path

After the production backfill is complete:
1. Remove the "Bulk Match" button from the DH admin tab
2. Keep the `POST /api/dh/match` endpoint but mark as deprecated
3. Eventually remove the handler and related goroutine management code

## Testing

- **Scheduler tests:** Table-driven tests for DH push scheduler covering matched, unmatched, deduplication, rate limiting, and transient error retry.
- **Service tests:** Table-driven tests for updated cert entry flow covering pending → inline match, already matched → channel sync, unmatched → skip.
- **Handler tests:** Fix-match endpoint covering valid URL parsing, invalid URL rejection, DH API failure.
- **CL import tests:** Verify `dh_push_status` set to `'pending'` after CLValueCents update, not set if DHInventoryID already exists.
- **Bulk match tests:** Verify status field populated for matched and unmatched results.
- **Frontend:** Manual testing of unmatched cards table, URL paste, and row removal.
