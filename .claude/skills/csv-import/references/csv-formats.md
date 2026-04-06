# CSV Format Reference

Detailed column specifications for each SlabLedger import format. All column name matching is case-insensitive and whitespace-trimmed.

---

## CL Refresh (`POST /api/purchases/refresh-cl`)

Updates `clValueCents` and optionally `population` on existing purchases matched by `slab serial #`. Does not create new purchases.

**Parser:** `ParseCLRefreshRows` in `internal/domain/campaigns/parse_cl.go`

### Required Columns

| Column | Type | Notes |
|---|---|---|
| `slab serial #` | string | PSA cert number; must match an existing purchase |
| `current value` | float | CL market value in dollars (e.g. `125.50`); no `$` prefix accepted |

### Optional Columns

| Column | Type | Notes |
|---|---|---|
| `card` | string | Informational only; not stored |
| `set` | string | Informational only; not stored |
| `number` | string | Informational only; not stored |
| `population` | integer | PSA pop count; stored if present; parse errors silently zero out the value |

### Behavior

- Rows with empty `slab serial #` are silently skipped
- Rows with missing or unparseable `current value` produce a `ParseError` and are excluded from the result
- Fatal error if `slab serial #` column is absent from the header

### Example CSV

```csv
Slab Serial #,Card,Set,Number,Current Value,Population
12345678,Charizard,Base Set,4,1250.00,120
87654321,Pikachu,Jungle,60,45.00,5000
```

---

## CL Import (`POST /api/purchases/import-cl`)

Creates new purchases (auto-allocated to matching campaigns) and refreshes CL values for existing ones.

**Parser:** `ParseCLImportRows` in `internal/domain/campaigns/parse_cl.go`

### Required Columns

| Column | Type | Notes |
|---|---|---|
| `slab serial #` | string | PSA cert number |
| `investment` | float | Buy cost in dollars (e.g. `800.00`); no `$` prefix accepted |
| `current value` | float | CL market value in dollars; no `$` prefix accepted |

### Optional Columns

| Column | Type | Notes |
|---|---|---|
| `date purchased` | string | Format: `M/D/YYYY` (e.g. `3/15/2026`); converted to `YYYY-MM-DD`; rows with invalid format are skipped with an error |
| `card` | string | Verbose card description (e.g. `2025 Pokemon Prismatic Charizard PSA 10`) |
| `player` | string | Clean card name (e.g. `Charizard`); preferred over `card` when both present |
| `set` | string | Set name |
| `number` | string | Card number within set |
| `condition` | string | Grade string (e.g. `PSA 10`) |
| `population` | integer | PSA pop count; best-effort parse; zero on failure |

### Behavior

- Card name preference: `player` field is used when non-empty; falls back to `card` field (see `CLCardName` in `import_types.go`)
- Rows with empty `slab serial #` produce a `ParseError`
- Rows with missing or unparseable `investment` or `current value` produce a `ParseError` and are excluded
- Fatal error if any required column is absent from the header

### Example CSV

```csv
Date Purchased,Card,Player,Set,Number,Condition,Investment,Current Value,Slab Serial #,Population
3/15/2026,2026 Pokemon Charizard Base Set PSA 10,Charizard,Base Set,4,PSA 10,800.00,1250.00,12345678,120
3/15/2026,2026 Pokemon Pikachu Jungle PSA 9,Pikachu,Jungle,60,PSA 9,30.00,45.00,87654321,5000
```

---

## PSA Communication Spreadsheet (`POST /api/purchases/import-psa`)

Imports purchases from PSA's communication spreadsheet. The header row does not need to be the first row.

**Parser:** `ParsePSAExportRows` in `internal/domain/campaigns/parse_psa.go`

### Header Detection

`FindPSAHeaderRow` (in `parse_helpers.go`) scans the **first 6 rows** (indices 0–5). A row is accepted as the header if it contains at least **3** of these 4 known columns:

- `cert number`
- `listing title`
- `grade`
- `price paid`

If no qualifying row is found within 6 rows, the import fails with: `could not find PSA header row`.

### Required Columns (for header detection)

At least 3 of the 4 detection columns above must be present.

### All Recognized Columns

| Column | Type | Notes |
|---|---|---|
| `cert number` | string | PSA cert number; rows with empty cert are silently skipped |
| `listing title` | string | Raw PSA listing title; used for card name extraction |
| `grade` | float | Numeric grade (e.g. `10`, `9.5`, `8`); parse error skips the row |
| `price paid` | string | Currency string; accepts `$` prefix and commas (e.g. `$1,234.56`); parse error skips the row |
| `date` | string | Purchase date; accepts `M/D/YYYY` or `YYYY-MM-DD`; parse error skips the row |
| `invoice date` | string | Invoice date; same format rules as `date` |
| `was refunded?` | string | `yes`, `true`, or `1` (case-insensitive) = refunded; anything else = not refunded |
| `category` | string | Sport or set category (stored as-is) |
| `purchase source` | string | Where the card was purchased |
| `vault status` | string | PSA vault status string |
| `front image url` | string | URL to front card image |
| `back image url` | string | URL to back card image |

### Behavior

- Import is **tolerant**: rows with parse errors are collected but processing continues; only fails if zero valid rows remain
- `was refunded?` parsing: `strings.ToLower` applied before checking `yes`/`true`/`1`

### Example CSV

```csv
Invoice #,Date Generated,,
12345,1/15/2026,,
Cert Number,Listing Title,Grade,Price Paid,Date,Category,Purchase Source,Vault Status,Invoice Date,Was Refunded?,Front Image URL,Back Image URL
12345678,2023 Pokemon Scarlet & Violet Charizard ex #6 PSA 10,10,$125.00,1/15/2026,Pokemon,PSA Partner Offers,In Vault,1/16/2026,No,https://example.com/front.jpg,https://example.com/back.jpg
87654321,2000 Pokemon Jungle Pikachu #60 PSA 9,9,$35.00,1/15/2026,Pokemon,PSA Partner Offers,,,,
```

---

## Shopify Product Export (`POST /api/purchases/import-external`)

Imports from Shopify's standard product CSV export. Only PSA-graded cards with a resolvable cert number are imported.

**Parser:** `ParseShopifyExportRows` in `internal/domain/campaigns/parse_shopify.go`

### Required Columns

| Column | Type | Notes |
|---|---|---|
| `handle` | string | Shopify product handle; groups rows belonging to the same product |
| `title` | string | Product title; rows with empty title are treated as variant/image-only rows |

### Optional Columns

| Column | Type | Notes |
|---|---|---|
| `cert number` | string | PSA cert; plain digits or `PSA-XXXXX` format; checked first |
| `cert` | string | Alternate cert column; same format; checked second |
| `sku` | string | Shopify SKU; must match `PSA-XXXXX` pattern; checked third |
| `tags` | string | Comma-separated positional tags: `cardName, cardNumber, setName, sport`; see tag parsing below |
| `variant price` | string | Sell price in dollars; checked before `price`; accepts `$` and commas |
| `price` | string | Fallback sell price if `variant price` is absent |
| `cost per item` | string | Buy cost in dollars; accepts `$` and commas |
| `image src` | string | Image URL; first data row with title = front image; variant-only rows = back image |

### Cert Number Resolution

Resolution order (first non-empty normalized result wins):

1. `cert number` column — `NormalizePSACert()` applied
2. `cert` column — `NormalizePSACert()` applied
3. `sku` column — must match `PSA-XXXXX` exactly; digits extracted

`NormalizePSACert` accepts plain digit strings or `PSA-XXXXX` format and returns the digits only. Any other format returns empty string and the row is skipped.

### Tag Parsing

`ParseShopifyTags` splits on `,` and treats parts positionally:
- Part 0: `cardName` (required; empty = parse error)
- Part 1: `cardNumber` (optional; must match `^[A-Za-z0-9]+([/\-][A-Za-z0-9]+)?$` if present)
- Part 2: `setName` (optional)
- Part 3: `sport` (optional)
- More than 4 parts = parse error

If tags parsing fails or `cardName` is empty, `ExtractCardNameFromTitle` is used to derive the card name from the title by stripping grader/grade and condition patterns.

### Grader and Grade Extraction

`ExtractGraderAndGrade` scans the title for patterns like `PSA 10`, `CGC 9.5`, `BGS 8`. If no match, grader defaults to `PSA` (cert number implies PSA), grade defaults to `0`.

### Multi-Row Product Consolidation

- Multiple rows sharing the same `handle` AND `cert number` are deduplicated; the first row wins
- Multiple rows sharing the same `handle` but with **different** cert numbers produce separate purchases
- Variant-only rows (empty title) do not produce purchases; their `image src` is stored as the back image for the product with the same handle

### Behavior

- Rows with empty `handle` are silently skipped
- Rows with no resolvable PSA cert number are silently skipped
- Price parse errors skip the row and add to `parseErrors` (returned in response `errors` array)
- Fatal error if `handle` or `title` columns are absent from the header

### Example CSV

```csv
Handle,Title,Tags,Variant Price,Cost Per Item,Image Src,SKU
charizard-psa10,2023 Pokemon Charizard ex PSA 10,"Charizard ex,6,Scarlet & Violet",125.00,80.00,https://example.com/charizard-front.jpg,PSA-12345678
charizard-psa10,,,,,,https://example.com/charizard-back.jpg
pikachu-psa9,2000 Pokemon Jungle Pikachu PSA 9,"Pikachu,60,Jungle",45.00,22.00,https://example.com/pikachu-front.jpg,PSA-87654321
```

---

## Cert Number Import (`POST /api/purchases/import-certs`)

Adds PSA certs to the External campaign by cert number only. Uses PSA's cert lookup to resolve card details. This is a JSON endpoint, not a file upload.

**Handler:** `HandleImportCerts` in `internal/adapters/httpserver/handlers/campaigns_imports.go`

### Request Format

```json
{
  "certNumbers": ["12345678", "87654321", "11111111"]
}
```

| Field | Type | Notes |
|---|---|---|
| `certNumbers` | string array | One or more PSA cert numbers as strings; duplicates and blank entries are deduplicated and removed |

### Constraints

- Maximum request body: **1 MB**
- Cert numbers are whitespace-trimmed and deduplicated before processing
- Empty `certNumbers` array returns HTTP 400

### Response (`CertImportResult`)

```json
{
  "imported": 2,
  "alreadyExisted": 1,
  "failed": 0,
  "errors": []
}
```

| Field | Type | Notes |
|---|---|---|
| `imported` | integer | Certs successfully added to External campaign |
| `alreadyExisted` | integer | Certs that were already in the system; eBay export flag is set on existing purchases |
| `failed` | integer | Certs that could not be imported |
| `errors` | array | Per-cert errors with `certNumber` and `error` message |
