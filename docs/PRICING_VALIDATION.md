# Pricing Validation Checklist

Tracking document for validating pricing accuracy across all sources for current inventory.

## Integration Tests

Run all pricing integration tests:
```bash
go test -tags integration ./internal/integration/ -v -timeout 10m
```

Run individual test suites:
```bash
# Parse PSA CSV → card metadata (no API calls, instant)
go test -tags integration ./internal/integration/ -run TestParseCardMetadataFromCSV -v

# PriceCharting direct lookup (3s rate limit per card, ~2 min)
go test -tags integration ./internal/integration/ -run TestPriceChartingLookup -v -timeout 3m

# Full pipeline: parse → PriceCharting + TCGdex validation
go test -tags integration ./internal/integration/ -run TestImportToPricing -v -timeout 5m

# TCGdex card validation
go test -tags integration ./internal/integration/ -run TestCardValidation -v -timeout 3m
```

Test files: `internal/integration/pricing_test.go`, `internal/integration/import_pricing_test.go`, `internal/integration/pipeline_test.go`, `internal/integration/testdata_test.go`

### Pipeline Tests

```bash
# Normalization audit (no API calls, instant)
go test -tags integration ./internal/integration/ -run TestNormalizationAudit -v

# CardHedger lookup (requires CARD_HEDGER_API_KEY)
go test -tags integration ./internal/integration/ -run TestCardHedgerLookup -v -timeout 3m

# Full multi-source fusion (requires PRICECHARTING_TOKEN, optionally PP + CH keys)
go test -tags integration ./internal/integration/ -run TestFullMultiSourceFusion -v -timeout 5m

# Complete import-to-pricing pipeline (requires PRICECHARTING_TOKEN)
go test -tags integration ./internal/integration/ -run TestImportToFullPricing -v -timeout 5m
```

---

## Current Status (2026-03-17)

### Test Data Coverage

34 inventory entries (28 unique cards) in `testdata_test.go`, covering:
- 13 new cards added (Mega Gardevoir, Pikachu SM162 ×3, Captain Pikachu, SP Delivery Charizard, Umbreon CBB2, Rayquaza CRZ, Eevee ex, Pikachu SV4a, Dark Mewtwo Pokken, Mimikyu DRI, Ancient Mew)
- `TestParseCardMetadataFromCSV` expectations: 34/34 cards covered

### PriceCharting — VALIDATED ✅

All 26 priceable cards (excluding 2 Ancient Mew + Dark Mewtwo with no card number) find the correct PriceCharting product with reasonable prices.

| Card | Cert | Product Matched | Grade Price | Status |
|------|------|----------------|-------------|--------|
| BLASTOISE-HOLO SHADOWLESS #2 | 145076863 | Blastoise [Shadowless] #2 | PSA8.5=$444-$2504 interp | ✅ |
| MEW-HOLO SOUTHERN ISLAND #151 | 145076888 | Mew #151 | PSA9=$209 | ✅ |
| CHARIZARD REVERSE FOIL #006 | 143473336 | Charizard [Holo] #6 | PSA9=$1029 | ⚠️ Wrong set (CD Promo not Neo) |
| MEWTWO-REV.FOIL #56 | 135767318 | Mewtwo [Reverse Holo] #56 | PSA9=$200 | ✅ |
| SYLVEON EX #156 | 149955139 | Sylveon ex #156 | PSA8=$217 | ✅ |
| UMBREON EX #176 | 141627783 | Umbreon Ex #176 | PSA10=$215 | ✅ |
| BIRTHDAY PIKACHU #24 | 145396462 | Pikachu Birthday #24 | PSA10=$190 | ✅ |
| CHARIZARD ex #161 | 145084327 | Charizard ex #161 | PSA10=$200 | ✅ |
| PIKACHU #002 | 143518127 | Pikachu #2 | PSA10=$128 | ✅ |
| DARK GYARADOS 1st Ed #8 | 145076879 | Dark Gyarados [1st Edition] #8 | PSA8=$90 | ✅ |
| GENGAR #094 | 144122685 | Gengar #94 | PSA10=$144 | ✅ |
| RAYQUAZA SPIRIT LINK #126 | 133478793 | Rayquaza Spirit Link #126/XY-P | PSA9=$109 | ✅ |
| UMBREON-HOLO 1st Ed #031 | 122699162 | Umbreon [1st Edition] #31 | PSA9=$115 | ✅ |
| SNORLAX #008 | 145455452 | Snorlax #8 | PSA10=$102 | ✅ |
| SNORLAX-HOLO #076 (×2) | 150154262/150154260 | Snorlax #76 | PSA10=$142 | ✅ |
| TOHOKU'S PIKACHU #260 PSA9 | 150154255 | Tohoku's Pikachu #260/SV-P | PSA9=$123 | ✅ |
| TOHOKU'S PIKACHU #260 PSA8 | 150154256 | Tohoku's Pikachu #260/SV-P | PSA8=$0 | ⚠️ No PSA8 data; grade fallback estimates ~$80 |
| DRAGONITE EX 1st Ed #069 | 143210177 | Dragonite EX [1st Edition] #69 | PSA10=$88 | ✅ |
| MEGA GARDEVOIR ex #087 | 139414865 | Mega Gardevoir ex #87 | PSA10=$164 | ✅ |
| PIKACHU-HOLO SM162 (×3) | 135021722/134110532/134110528 | Pikachu #SM162 | PSA9=$185, PSA8=$81 | ✅ |
| CAPTAIN PIKACHU #09 | 130221147 | Captain Pikachu #709 | PSA9=$208 | ✅ |
| SP.DELIVERY CHARIZARD #075 | 72973327 | Special Delivery Charizard #SWSH075 | PSA9=$216 | ✅ |
| UMBREON #15 (CBB2) | 123238115 | Umbreon #615 | PSA9=$186 | ✅ |
| RAYQUAZA-HOLO #029 | 134093774 | Rayquaza [Cosmos Holo] #SWSH029 | PSA10=$120 | ✅ |
| EEVEE ex #167 | 113751496 | Eevee ex #167 | PSA9=$134 | ✅ |
| PIKACHU S #236 | 132537172 | Pikachu #236 | PSA10=$110 | ✅ |
| TEAM ROCKET'S MIMIKYU #087 | 121986129 | Team Rocket's Mimikyu #87 | PSA9=$25 | ✅ |

### CardHedger — VALIDATED ✅

17 of 26 priceable cards found with grade data. 9 not found (expected — Japanese/Chinese/niche cards).

| Category | Count | Cards |
|----------|-------|-------|
| Found with prices | 17 | Charizard Neo, Dark Gyarados, Sylveon, Dragonite, Mew SI, Umbreon BW, Gengar, Snorlax DB, Blastoise, Pikachu McD, Birthday Pikachu, Mewtwo Exp, Gardevoir, Pikachu SM162, Eevee ex, Pikachu SV4a, Mimikyu |
| Not found (JP/niche) | 9 | Snorlax JP SD100, Rayquaza JP XY, Tohoku's Pikachu, Charizard ex SVP, Umbreon ex SVP, Captain Pikachu CN, SP Delivery Charizard, Umbreon CN, Rayquaza CRZ |

### Import-to-Pricing Pipeline — VALIDATED ✅

34/34 entries pass end-to-end (parse → PriceCharting lookup → price verification).

- Ancient Mew (×3): Now correctly parsed and priced (PSA9=$187.50) via no-number title fallback + "GAME MOVIE" → "Promo" set mapping
- Dark Mewtwo Pokken Tournament: Correctly parsed (name="DARK MEWTWO", set="PROMO POKKEN TOURNAMENT") but PriceCharting doesn't index this Japanese promo — marked as SkipPricing

---

## Fixes Applied (2026-03-17)

### Collection suffix stripping (import.go)
Added new suffixes to strip product/collection description from parsed PSA card names:
- `CROWN ZENITH PREMIUM COLLECTION SEA & SKY`, `CROWN ZENITH` — for CRZ promos
- `TEAM UP SINGLE PACK BLISTERS`, `SINGLE PACK BLISTERS` — for SM promo packaging
- `POKEMON CENTER UNITED KINGDOM` — regional location descriptor
- `SPECIAL ART RARE` — Japanese SAR rarity marker

### No-number title parsing (import.go)
Added `parseNoNumberTitle()` fallback for PSA titles without collector numbers (e.g., Ancient Mew, Dark Mewtwo Pokken).
- Matches known card name patterns (`ANCIENT MEW`, `DARK MEWTWO`) in the title tokens
- Extracts card name and set name from surrounding tokens
- Falls through to raw-title fallback only if no pattern matches

### PSA category mapping: "GAME MOVIE" → "Promo" (import.go)
Ancient Mew PSA titles contain `"GAME MOVIE"` as the inferred set name. Added mapping to `psaCategoryToSetName` so it resolves to "Promo", matching PriceCharting's "Pokemon Promo" console.

### SWSH Black Star Promo handling (pc_query_helpers.go, domain_adapter.go)
- **Set name normalization**: SWSH/SM Black Star Promo sets now normalize to just "Promo" for PriceCharting queries. PriceCharting consolidates all promo eras under "Pokemon Promo" console — era-specific tokens like "Sword Shield" diluted search.
- **Card number prefixing**: For SWSH promos, purely numeric card numbers (e.g., "075") are prefixed with "SWSH" (→ "SWSH075") in both the search query and the verification step. PriceCharting indexes these as "SWSH075" not "75".

---

## Known Pricing Gaps

### Cards with single-source pricing (PriceCharting only)
Japanese cards will likely only get PriceCharting pricing since CardHedger primarily indexes English sets. This is acceptable — PriceCharting has good coverage of Japanese graded cards.

### Charizard Japanese Neo #006 — wrong card match
PriceCharting matches "Charizard [Holo] #6" from Japanese CD Promo ($1029 PSA9) instead of Japanese Neo Premium File ($305 PSA9). Both are Japanese Charizard #6 — disambiguation requires the exact set name. The current set name "JAPANESE NEO 2 PROMO" doesn't overlap with "Japanese CD Promo" OR "Japanese Neo Premium File" well enough to disambiguate.

**Potential fix**: Add the specific PriceCharting product ID to `psaCategoryToSetName` or use PriceCharting's ID-based lookup once the correct product is identified.

### Tohoku's Pikachu PSA 8 — no grade data
PriceCharting has PSA 9 ($123) and PSA 10 ($422) but no PSA 8 data. The grade fallback in `GetMarketSnapshot` estimates PSA 8 at 65% of PSA 9 ≈ $80. This is an estimate, not market data.

### Dark Mewtwo Pokken — not indexed in PriceCharting
This Japanese Pokken Tournament promo is correctly parsed (name="DARK MEWTWO", set="PROMO POKKEN TOURNAMENT") but PriceCharting doesn't have it. Requires manual cert-lookup enrichment or a price hint.

---

## API Rate Limits

| Source | Daily Limit | Credits/Call | Effective Calls | Reset |
|--------|------------|-------------|-----------------|-------|
| PriceCharting | ~30/min | 1 | ~1,800/hour | Per-minute rolling |
| CardHedger | 1,000 calls | 1 | 1,000 | Midnight UTC |
| PSA Cert | 100/day per key | 1 | 200 (2 keys) | Midnight UTC |
| TCGdex | No limit | 1 | Unlimited | N/A |
