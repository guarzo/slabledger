---
name: csv-import
description: Import cards into SlabLedger from CSV files
---

# CSV Import Skill

This skill guides you through importing cards into SlabLedger from CSV files. SlabLedger supports five import types, each with a dedicated endpoint.

---

## Step 1: Identify Import Type

| Import Type | Endpoint | Key Columns | Use Case |
|---|---|---|---|
| CL Refresh | `POST /api/purchases/refresh-cl` | `slab serial #`, `current value` | Update CL market values for existing purchases |
| CL Import | `POST /api/purchases/import-cl` | `slab serial #`, `investment`, `current value` | Auto-allocate new cards + refresh existing from Card Ladder export |
| PSA Import | `POST /api/purchases/import-psa` | `cert number`, `listing title`, `grade` | Import purchases from PSA communication spreadsheet |
| Shopify Import | `POST /api/purchases/import-external` | `handle`, `title`, cert from `cert number`/`cert`/`sku` | Import from Shopify product export |
| Cert Entry | `POST /api/purchases/import-certs` | JSON body: `certNumbers` array | Quick-add PSA certs by number only (JSON, not CSV) |

See `references/csv-formats.md` for detailed column requirements and example rows.

---

## Step 2: Validate CSV

**Universal rules (all CSV imports):**
- Maximum file size: **10 MB**
- Must have at least one header row and one data row
- All column name matching is **case-insensitive** and whitespace-trimmed
- Rows with an empty primary key column are silently skipped

**Per-format quirks:**

**CL Refresh**
- Required: `slab serial #`, `current value`
- Rows missing `current value` are parse errors (not silently skipped)
- `current value` must be a bare float (e.g. `125.00`) — no `$` prefix

**CL Import**
- Required: `slab serial #`, `investment`, `current value`
- `date purchased` must be `M/D/YYYY` format if present (e.g. `3/15/2025`)
- Rows with missing or unparseable required fields are skipped with an error

**PSA Import**
- Header row detection scans the **first 6 rows** looking for at least 3 of: `cert number`, `listing title`, `grade`, `price paid`
- Rows with empty `cert number` are silently skipped
- `price paid` accepts `$` prefix and commas (e.g. `$1,234.56`)
- `date` and `invoice date` accept `M/D/YYYY` or `YYYY-MM-DD`
- `was refunded?` accepts `yes`, `true`, or `1` (case-insensitive)
- PSA import is tolerant: continues processing valid rows even when some rows have parse errors

**Shopify Import**
- Required: `handle`, `title`
- Rows without a title are treated as variant/image-only rows — the image URL is captured as the back image for the product sharing the same handle
- Cert number resolution order: `cert number` column → `cert` column → `sku` column (must match `PSA-XXXXX` pattern or be plain digits)
- Rows with no resolvable PSA cert number are skipped (CGC, raw cards, etc.)
- Products sharing the same handle+cert are deduplicated; the first occurrence wins
- Prices (`variant price` or `price`) and `cost per item` accept `$` prefix and commas
- Tags field (comma-separated, positional): `cardName, cardNumber, setName, sport` — falls back to title extraction if absent or malformed

---

## Step 3: Upload via API

All CSV endpoints accept `multipart/form-data` with the file in a field named `file`. Authentication is required (session cookie or `Authorization: Bearer <LOCAL_API_TOKEN>`).

**CL Refresh:**
```bash
curl -X POST http://localhost:8081/api/purchases/refresh-cl \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -F "file=@card_ladder_export.csv"
```

**CL Import:**
```bash
curl -X POST http://localhost:8081/api/purchases/import-cl \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -F "file=@card_ladder_export.csv"
```

**PSA Import:**
```bash
curl -X POST http://localhost:8081/api/purchases/import-psa \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -F "file=@psa_communication.csv"
```

**Shopify Import:**
```bash
curl -X POST http://localhost:8081/api/purchases/import-external \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -F "file=@shopify_products_export.csv"
```

**Cert Entry (JSON — not a CSV upload):**
```bash
curl -X POST http://localhost:8081/api/purchases/import-certs \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"certNumbers": ["12345678", "87654321"]}'
```

---

## Step 4: Review Results

Each endpoint returns a JSON summary. HTTP 200 does not mean all rows succeeded — check the counters and `errors` array.

**CL Refresh response (`GlobalCLRefreshResult`):**
```json
{
  "updated": 42,
  "notFound": 3,
  "failed": 0,
  "results": [
    {"certNumber": "12345678", "oldValueCents": 5000, "newValueCents": 5500, "status": "updated"}
  ],
  "byCampaign": {"Campaign A": {"campaignName": "Campaign A", "updated": 42}}
}
```

**CL Import response (`GlobalImportResult`):**
```json
{
  "allocated": 10,
  "refreshed": 32,
  "unmatched": 2,
  "ambiguous": 1,
  "skipped": 0,
  "failed": 0,
  "results": [
    {"certNumber": "12345678", "status": "allocated", "campaignId": "1", "campaignName": "Campaign A"}
  ],
  "byCampaign": {}
}
```
Status values: `allocated`, `refreshed`, `unmatched`, `ambiguous`, `skipped`, `failed`

**PSA Import response (`PSAImportResult`):**
```json
{
  "allocated": 5,
  "updated": 2,
  "refunded": 0,
  "unmatched": 1,
  "ambiguous": 0,
  "skipped": 0,
  "failed": 0,
  "certEnrichmentPending": 3,
  "results": [
    {"certNumber": "12345678", "status": "allocated", "campaignId": "1"}
  ]
}
```
Status values: `allocated`, `updated`, `refunded`, `unmatched`, `ambiguous`, `skipped`, `failed`

**Shopify Import response (`ExternalImportResult`):**
```json
{
  "imported": 8,
  "skipped": 1,
  "updated": 2,
  "failed": 0,
  "results": [
    {"certNumber": "12345678", "cardName": "Charizard", "status": "imported"}
  ]
}
```
Status values: `imported`, `updated`, `skipped`, `failed`

**Cert Entry response (`CertImportResult`):**
```json
{
  "imported": 2,
  "alreadyExisted": 0,
  "failed": 0,
  "errors": []
}
```

---

## Step 5: Post-Import

**Background enrichment** runs automatically after successful imports:

- **CL Import**: Triggers DH price refresh for all `allocated` and `refreshed` rows (5-minute timeout, background goroutine)
- **PSA Import**: Triggers DH price refresh for `allocated` and `updated` rows; cert enrichment queued for card metadata lookup (see `certEnrichmentPending` count in response)
- **Shopify Import**: Triggers DH price refresh for `imported` and `updated` rows
- **Market snapshots**: Newly imported purchases with `snapshotStatus: "pending"` are enriched by the snapshot scheduler (retries up to `SNAPSHOT_ENRICH_MAX_RETRIES` times at `SNAPSHOT_ENRICH_RETRY_INTERVAL` intervals)

The HTTP response is returned before enrichment completes. Background work does not affect the import result.

---

## Troubleshooting

| Error | Cause | Fix |
|---|---|---|
| `File upload required` | No `file` form field in request | Use `-F "file=@path/to/file.csv"` |
| `File too large (max 10MB)` | CSV exceeds 10 MB | Split file or remove unused columns |
| `CSV must have a header row and at least one data row` | File has 0 or 1 rows | Verify file is not empty; check for BOM or encoding issues |
| `missing required column: "slab serial #"` | CL export missing cert column | Export from Card Ladder with all columns; do not rename headers |
| `missing required column: "investment"` | CL Import missing cost column | Use full CL export (not refresh-only export) for import-cl endpoint |
| `could not find PSA header row` | PSA file has more than 5 preamble rows or missing key columns | Ensure file contains `cert number`, `listing title`, and `grade` within first 6 rows |
| `No valid PSA data rows found in CSV` | All rows had empty cert numbers | Verify the correct PSA communication file is uploaded |
| `CSV is missing required column: handle` | Shopify export format changed | Verify export includes standard Shopify `Handle` column |
| `Invalid JSON body` | Cert import body is malformed | Use `{"certNumbers": ["..."]}` with `Content-Type: application/json` |
| `unmatched` rows in result | Cert not in any active campaign's date/grade/price range | Review campaign parameters or allocate manually |
| `ambiguous` rows in result | Cert matched multiple campaigns equally | Resolve in UI or tighten campaign parameters |

---

## Code Locations

| Component | Path |
|---|---|
| CL parsing | `internal/domain/campaigns/parse_cl.go` |
| PSA parsing | `internal/domain/campaigns/parse_psa.go` |
| Shopify parsing | `internal/domain/campaigns/parse_shopify.go` |
| Shared parse helpers | `internal/domain/campaigns/parse_helpers.go` |
| Import + row types | `internal/domain/campaigns/import_types.go` |
| Cert entry types | `internal/domain/campaigns/ebay_types.go` |
| HTTP handlers | `internal/adapters/httpserver/handlers/campaigns_imports.go` |
| CL import service | `internal/domain/campaigns/service_import_cl.go` |
| PSA import service | `internal/domain/campaigns/service_import_psa.go` |
| External import service | `internal/domain/campaigns/service_import_external.go` |
| Cert entry service | `internal/domain/campaigns/service_cert_entry.go` |
