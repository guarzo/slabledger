# Cert Import UX Redesign — Streaming Scanner

**Date:** 2026-04-05
**Status:** Draft
**Branch:** guarzo/certimport

## Problem

The current cert import screen uses a textarea + batch submit pattern. Users paste cert numbers, click "Import", wait for the entire batch to resolve, then see a summary. This is friction-heavy for the primary use case: standing at a desk scanning/typing cert numbers from a stack of physical cards as they arrive. Users need instant per-cert feedback and the ability to act on sold items inline.

## Design

### Approach: Streaming Scanner

Replace the textarea with a single text input. The user types or scans a cert number, hits Enter, and sees an instant result row appear below. The input clears and refocuses immediately for the next scan.

**Three-tier response speed:**
- **Existing certs (most common):** Instant DB check. Auto-flagged for eBay export. No user action needed.
- **Sold certs:** Instant DB check. Shown with inline "Return to Inventory" button.
- **New certs (rare):** Instant "new" status from DB check, then background PSA API lookup (~500ms) populates card details. Staged for user review before final import.

### User Flow

1. User types/scans cert number into input, hits Enter.
2. Input clears and refocuses immediately (ready for next scan).
3. Cert appears as a row below with instant status from DB check.
4. For new certs, a background call resolves card details and updates the row in place.
5. After scanning is done, user reviews sold items (inline return) and new certs.
6. User clicks "Import New Certs" to commit staged new certs.

### Row States

| State | Color | Description | User Action |
|---|---|---|---|
| Existing | Green | Already in DB, eBay export flag set | None |
| Sold | Amber | Exists but has a sale record | "Return to Inventory" button |
| Returned | Green | Sale deleted after user clicked Return | None |
| Resolving | Blue + spinner | New cert, PSA lookup in progress | None (keep scanning) |
| Resolved | Blue + star | New cert, card info loaded, staged | Committed via "Import New Certs" |
| Failed | Red | PSA lookup failed or cert not found | "Dismiss" button |
| Duplicate | Gray | Already scanned this session | Scroll to existing row and briefly highlight it. No new row persisted. |

### Layout (top to bottom)

1. **Input** — single text field, always focused, monospace, large enough for barcode scanner input.
2. **Stats bar** — running counts: N in inventory, N sold, N new, N failed, total scanned.
3. **Cert rows** — newest first. Color-coded by state. Inline actions where applicable.
4. **Staging area** — dashed border section at bottom. Shows "Import N New Certs" button. Only visible when resolved new certs exist.

## API Changes

### New: `POST /api/purchases/scan-cert`

DB check only. Fast path for the scanning loop.

**Request:**
```json
{"certNumber": "12345678"}
```

**Response:**
```json
{
  "status": "existing|sold|new",
  "cardName": "2023 Pikachu VMAX PSA 10",
  "purchaseId": "uuid",
  "campaignId": "uuid"
}
```

- `cardName`, `purchaseId`, `campaignId` only present for existing/sold status.
- For existing certs: auto-sets eBay export flag as a side effect.

### New: `POST /api/purchases/resolve-cert`

PSA API lookup for new certs. Returns card info for preview. No side effects (does not create a purchase).

**Request:**
```json
{"certNumber": "91234567"}
```

**Response:**
```json
{
  "certNumber": "91234567",
  "cardName": "2022 Umbreon VMAX Alt Art",
  "grade": 10,
  "year": "2022",
  "category": "SWORD & SHIELD",
  "subject": "2022 Pokemon Sword & Shield Umbreon VMAX Alt Art"
}
```

Returns 404 if cert not found at PSA. Returns 500 on API errors.

### Existing: `POST /api/purchases/import-certs`

Unchanged. Called with just the new cert numbers when user clicks "Import New Certs". Performs PSA lookup + creates purchase records + triggers background jobs (DH listing, cert enrichment, card ID resolution).

### Existing: `DELETE /campaigns/{id}/purchases/{id}/sale`

Unchanged. Used by inline "Return to Inventory" button.

## Frontend Implementation

### Component: `CertEntryTab.tsx` (rewrite)

**State:**
```typescript
scannedCerts: Map<string, CertRow>
```

Where `CertRow` is:
```typescript
interface CertRow {
  certNumber: string;
  status: 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed';
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  error?: string;
}
```

**Flow per cert:**
1. Enter pressed → check `scannedCerts` map for duplicate (client-side, no API call).
2. If new to session → add row, fire `scan-cert` API call.
3. Response: existing → update row, done. Sold → update row with card info + IDs. New → set status to `resolving`, fire background `resolve-cert`.
4. Resolve response → update row with card info, set status to `resolved`.
5. "Import New Certs" → collect resolved cert numbers, call `import-certs`, transition successfully imported rows from blue (resolved) to green (existing). Show error inline for any that failed.

**Duplicate handling:** If cert already in `scannedCerts`, briefly highlight the existing row and scroll it into view. No API call.

### Types: `web/src/types/campaigns/core.ts`

Add `ScanCertResponse` and `ResolveCertResponse` interfaces.

### API client: `web/src/js/api.ts`

Add `scanCert(certNumber)` and `resolveCert(certNumber)` methods.

## Backend Implementation

### Handler: `campaigns_imports.go`

Add `HandleScanCert` and `HandleResolveCert` handlers. Both are thin — delegate to service layer.

### Service: `service_cert_entry.go`

Add `ScanCert(ctx, certNumber)` method:
- Single-cert lookup via existing `repo.GetPurchasesByGraderAndCertNumbers` (batch of 1).
- Check for sale via `repo.GetSalesByPurchaseIDs`.
- If existing and not sold: set eBay export flag.
- Return status + card info.

Add `ResolveCert(ctx, certNumber)` method:
- Call `certLookup.LookupCert`.
- Return cert info. No persistence.

### Routes: `routes.go`

```go
mux.Handle("POST /api/purchases/scan-cert", authRoute(rt.campaignsHandler.HandleScanCert))
mux.Handle("POST /api/purchases/resolve-cert", authRoute(rt.campaignsHandler.HandleResolveCert))
```

### Domain types: `ebay_types.go`

```go
type ScanCertRequest struct {
    CertNumber string `json:"certNumber"`
}

type ScanCertResult struct {
    Status     string `json:"status"`     // "existing", "sold", "new"
    CardName   string `json:"cardName,omitempty"`
    PurchaseID string `json:"purchaseId,omitempty"`
    CampaignID string `json:"campaignId,omitempty"`
}

type ResolveCertRequest struct {
    CertNumber string `json:"certNumber"`
}

type ResolveCertResult struct {
    CertNumber string  `json:"certNumber"`
    CardName   string  `json:"cardName"`
    Grade      float64 `json:"grade"`
    Year       string  `json:"year"`
    Category   string  `json:"category"`
    Subject    string  `json:"subject"`
}
```

## Testing

- **Service tests:** Table-driven tests for `ScanCert` covering existing, sold, new, and error cases. Mock repo + certLookup.
- **Service tests:** Table-driven tests for `ResolveCert` covering found, not found, and API error cases.
- **Handler tests:** Verify HTTP status codes and response shapes for both new endpoints.
- **Frontend:** Manual testing of scan flow, duplicate handling, return-to-inventory, and import-new-certs.

## What Stays Unchanged

- `import-certs` endpoint and service logic.
- Return-to-inventory flow (`deleteSale`).
- All background jobs (DH listing, cert enrichment, card ID resolution) — still triggered by `import-certs`.
- Database schema — no migrations needed.
