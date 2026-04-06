# Remaining Pricing Remnants Cleanup

Identified 2026-04-06 during codebase review after PriceCharting/CardHedger/JustTCG/fusion engine removal. These items are out of scope for the source enrichment work and should be addressed in a separate cleanup PR.

## Documentation (High Priority)

### 1. README.md — completely outdated
- **Lines 18, 33, 50, 59, 66, 69** — references PriceCharting token, "Multi-Source Price Fusion", CardHedger API key
- **Action**: Rewrite Quick Start, Features, Requirements, and Environment Variables sections for DH + Card Ladder only

### 2. internal/README.md — architecture diagrams show deleted packages
- **Lines 23-31, 150-158** — lists `cardhedger/`, `fusionprice/`, `pricecharting/` directories
- **Lines 118-127** — code example uses `pricecharting.NewClient(...)`
- **Lines 497-500, 537-542** — anti-pattern example references pricecharting
- **Line 557-558** — file size exceptions table lists deleted files
- **Action**: Update diagrams, replace code examples with DH equivalents

### 3. docs/DEVELOPMENT.md — full sections on removed services
- **Lines 110-111** — rate limit table for PriceCharting, CardHedger
- **Lines 142-183** — setup instructions and code paths for PriceCharting, CardHedger, FusionPrice
- **Lines 224-242** — troubleshooting entries for removed services
- **Action**: Remove PriceCharting/CardHedger/FusionPrice sections, update rate limits and troubleshooting

### 4. docs/API.md — references removed providers
- **Lines 118, 125** — API usage example shows `cardhedger`
- **Lines 371, 394, 414, 418** — price hints reference `pricecharting` as valid provider
- **Lines 535-554** — CardHedger card request submission endpoints
- **Action**: Update examples to `doubleholo` only, remove or update CardHedger endpoint docs

### 5. docs/SCHEDULERS.md — deleted CardHedger schedulers
- **Lines 91-122** — full documentation for CardHedger Delta Refresh and Batch schedulers
- **Action**: Remove CardHedger scheduler sections

### 6. docs/SCHEMA.md — historical provider CHECK constraints
- **Lines 56, 76** — documents old provider CHECK constraints
- **Action**: Update to reflect current schema (constraints removed in migration 000037)

## CI & Dev Config (Medium Priority)

### 7. .github/workflows/test.yml — stale env vars
- **Lines 88-89** — passes `PRICECHARTING_TOKEN` and `CARD_HEDGER_API_KEY` secrets to integration tests
- **Action**: Remove these env vars, verify no integration tests reference them

### 8. .devcontainer/post-start.sh — checks for removed service
- **Lines 53-58** — checks for `PRICECHARTING_TOKEN` in .env
- **Action**: Update to check for `DH_ENTERPRISE_API_KEY` instead

### 9. .claude/skills/new-api-client/references/example-client.md — stale example
- **Line 84** — uses `CARD_HEDGER_API_KEY` as example
- **Action**: Replace with DH example

## Code Quality (Low Priority)

### 10. errors_test.go — stale assertion message
- **Line 181** — test correctly checks for `"doubleholo"` but error message says `"want PriceCharting"`
- **Action**: Change error message to `"want doubleholo"`

## Keep As-Is (No Action)

- **`cardhedger_request_id` DB column + Go/TS fields** — legacy column name, migration required to rename
- **`pricecharting_id` in `dh/types.go`** — DH API's response field, not our naming
- **Migration files** — audit trail, must remain
- **`docs/PRICING_DATA.md`** — already marked as historical (removed 2026-04-06)
- **`docs/2026-03-26-maintainability-*.md`** — historical planning documents
