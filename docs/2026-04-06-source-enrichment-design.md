# Source Enrichment: Per-Platform DH Breakdown + Card Ladder as Source

**Date**: 2026-04-06
**Status**: Approved
**Branch**: guarzo/postparty

## Problem

After removing PriceCharting, CardHedger, JustTCG, and the fusion engine, DH (DoubleHolo) is the sole price provider. The `dhprice` provider groups all recent sales by grade and computes medians, but ignores the `Platform` field on each sale (e.g., `"ebay"`, `"tcgplayer"`). This means:

- `Sources` is always `["doubleholo"]` and `SourceCount` is always 1
- The `EbayGradeDetail` struct in `pricing.GradeDetail` is never populated (dead code)
- The multi-source trust logic in `applyCLCorrection` (`SourceCount >= 2`) is dead
- Card Ladder, despite being an independent price signal, is never counted as a source

## Solution

Enrich the pricing pipeline so `Sources` and `SourceCount` represent **independent data signals** rather than API-level providers. Three changes:

1. **`dhprice` provider** — Break out per-platform sales data from DH's `RecentSale.Platform` field
2. **`applyCLCorrection`** — Add Card Ladder as a source when CL value is available
3. **`MarketSnapshot`** — Add `Sources []string` field for full traceability

## Validated Assumptions

DH's enterprise API (`/api/v1/enterprise/cards/{id}/recent-sales`) returns:

```json
{
  "sales": [
    {
      "sold_at": "2026-04-05",
      "grading_company": "psa",
      "grade": 9.0,
      "price": 26995.0,
      "platform": "ebay"
    }
  ]
}
```

- `platform` is lowercase (e.g., `"ebay"`, not `"eBay"`)
- Currently all observed sales are from `"ebay"`, but the field supports other platforms
- Building platform-aware logic now means we automatically benefit when DH adds TCGPlayer or other platform data

## Design

### Layer 1: `dhprice/provider.go` — Platform-aware `buildPrice`

Currently `buildPrice` groups sales by grade only. Change to group by **(grade, platform)**:

1. For each grade, compute **per-platform medians** from sales grouped by `Platform`
2. Populate `GradeDetail.Ebay` with eBay-specific data (median, min, max, sale count) when `platform == "ebay"`
3. Keep `GradeDetail.Estimate` as the **cross-platform aggregate** median (current behavior) — this is the "DH combined estimate"
4. Set `Price.Sources` to the **distinct platforms** seen in the sales data (e.g., `["ebay"]` or `["ebay", "tcgplayer"]`)

Platform strings are normalized to lowercase to match the API response format.

**Confidence mapping for eBay data:**
- >= 10 sales → `"high"`
- >= 3 sales → `"medium"`
- < 3 sales → `"low"`

No new types needed — `EbayGradeDetail` already has all required fields.

### Layer 2: `pricelookup/adapter.go` — No changes needed

The adapter already handles both paths:
- `detail.Ebay != nil` → creates `SourcePrice{Source: "eBay", ...}` (display name capitalized)
- `detail.Estimate != nil` → creates `SourcePrice{Source: "Estimate", ...}`
- `snap.SourceCount = len(price.Sources)` already flows through

Once `dhprice` populates `Ebay`, these existing code paths activate automatically.

### Layer 3: `MarketSnapshot` — Add `Sources` field

Add `Sources []string` to `MarketSnapshot`:

```go
type MarketSnapshot struct {
    // ... existing fields ...

    // Pricing metadata
    SourceCount int      `json:"sourceCount,omitempty"`
    Sources     []string `json:"sources,omitempty"`      // NEW: which sources contributed
    Confidence  float64  `json:"confidence,omitempty"`
    // ...
}
```

`SourceCount` is kept as a convenience field for backward compatibility with existing serialized snapshot JSON in the database. New code should derive it from `len(Sources)`.

Note: `SnapshotHistoryEntry` does NOT need a `Sources` field — the `market_snapshot_history` table has no `sources` column. The sources list is persisted inside `snapshot_json` (the full `MarketSnapshot` JSON blob) automatically.

### Layer 4: `applyCLCorrection` — CL as source with correct ordering

The operation order is critical to preserve multi-source trust semantics:

1. Set `snapshot.CLValueCents` and compute deviation (unchanged)
2. **Evaluate multi-source trust using `len(snapshot.Sources)`** — this is the market-only source count, before CL is added
3. Apply CL anchoring if needed (single-source market below CL with high deviation)
4. **Append `"cardladder"` to `snapshot.Sources`** and set `snapshot.SourceCount = len(snapshot.Sources)`

This preserves the intent: "trust multi-source market agreement over CL, but correct single-source market data that deviates significantly below CL."

### Layer 5: Stale comment cleanup (in-scope files only)

Update fusion-era comments in files we're already modifying:

| File | Line(s) | Current | Updated |
|------|---------|---------|---------|
| `service_snapshots.go` | 55 | "fusion pipeline produces unreliable results" | "market data produces unreliable results" |
| `service_snapshots.go` | 64-65 | "fusion result is trusted" | "market result is trusted" |
| `service_snapshots.go` | 87 | "Multi-source fusion that diverges" | "Multi-source market data that diverges" |
| `service_snapshots_test.go` | 90 | "high deviation multi-source trusts fusion" | "high deviation multi-source trusts market" |
| `main.go` | 235 | "market intelligence + fusion source" | "market intelligence + pricing source" |
| `inventory_refresh.go` | 109 | "fusion provider layer" | "DH provider layer" |

## Files Changed

| File | Change |
|------|--------|
| `internal/adapters/clients/dhprice/provider.go` | Group sales by platform, populate `Ebay` + `Estimate` in `GradeDetail`, set `Sources` to distinct platforms |
| `internal/domain/campaigns/service.go` | Add `Sources []string` to `MarketSnapshot` |
| `internal/domain/campaigns/service_snapshots.go` | Append `"cardladder"` to Sources after CL evaluation; update comments |
| `internal/domain/campaigns/history.go` | No change needed — Sources persisted via snapshot_json |
| `internal/adapters/clients/pricelookup/adapter.go` | Set `snap.Sources` from `price.Sources` (minor addition) |
| `cmd/slabledger/main.go` | Fix comment (line 235) |
| `internal/adapters/scheduler/inventory_refresh.go` | Fix comment (line 109) |

## Test Plan

### `dhprice/provider_test.go`
- Test with all-eBay sales: verify `Sources == ["ebay"]`, `GradeDetail["psa10"].Ebay` populated with median/min/max/count
- Test with mixed-platform sales (eBay + TCGPlayer): verify `Sources == ["ebay", "tcgplayer"]`, each platform's `Ebay`/detail populated independently
- Test with no sales: verify nil return (unchanged)
- Test `GradeDetail.Estimate` still contains cross-platform aggregate median

### `service_snapshots_test.go`
- Update existing `applyCLCorrection` tests to set and verify `Sources` field
- Test: CL available → `"cardladder"` appended to Sources
- Test: multi-source market (SourceCount >= 2) with CL → no anchoring, CL still appended to Sources
- Test: single-source market below CL with high deviation → anchoring applied, then CL appended
- Test: CL value is 0 → `"cardladder"` NOT appended

### `pricelookup/adapter_test.go`
- Update mock `pricing.Price` fixtures to include `Ebay` in `GradeDetails`
- Verify `snap.Sources` populated from `price.Sources`
- Verify `SourceCount` matches `len(Sources)`

## Out of Scope

Documentation cleanup, CI config, README updates, and other pricing remnants are tracked separately in `docs/cleanup-remaining-pricing-remnants.md`.
