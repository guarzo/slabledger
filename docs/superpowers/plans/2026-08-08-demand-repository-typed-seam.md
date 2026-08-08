# Typed `demand.Repository` Seam (SLA-41) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the opaque `*string` JSON blob fields on `demand.Repository`'s row types with shared exported domain structs, moving all JSON marshalling into the postgres adapter and fixing the always-zero `NicheMarket.ActiveListingCount` bug on the way.

**Architecture:** Today three layers each hold a private opinion about the same JSON bytes: the scheduler marshals DH wire types into strings, the postgres adapter passes the strings through untouched, and `demand`'s service unmarshals them into unexported structs. Nothing type-checks across that seam, which is how the scheduler came to write a *nested* saturation payload that the reader parses as *flat* — `ActiveListingCount` has been silently zero in production ever since. This plan collapses the three opinions into one exported set of payload structs in `internal/domain/demand/payloads.go`, makes those structs the repository contract, and leaves the postgres adapter as the single place that encodes and decodes them. The scheduler then maps DH wire types to domain structs directly, with the compiler checking the shape.

**Tech Stack:** Go 1.26 (generics used for the payload marshal/unmarshal helpers), `encoding/json`, `database/sql` + `jackc/pgx/v5/stdlib`, table-driven tests with doubles from `internal/testutil/mocks/`.

## Global Constraints

- **Hexagonal invariant:** `internal/domain/` must never import `internal/adapters/`. Enforced by `scripts/check-imports.sh` in `make check`.
- **Flat-sibling rule:** `internal/domain/demand` is a sibling of `internal/domain/inventory` and may not import any other governed sibling. Enforced target-side by the same script.
- **Mocks and test doubles live in `internal/testutil/mocks/`** — never define one inline in a test file (CLAUDE.md, Testing).
- **No migration in this change.** The five `dh_card_cache` blob columns and the three `dh_character_cache` blob columns stay in the database. Image rollback does not run Down migrations (`docs/OPERATIONS.md:87`) and the app auto-deploys on push to `main`, so the previous binary must still find the columns it writes. Dropping them is a separate, later release, tracked by the follow-up ticket filed in Task 7.
- **Defect 2 is out of scope.** Character `data_quality` has no DH writer; `CharacterDemand.DataQuality` therefore always decodes to `""`. It is documented in a comment on the field and tracked as SLA-61 — do not attempt to source it here.
- **Accepted payload losses:** `avg_days_to_sell` and `sell_through` are dropped from the velocity payload. No domain code reads them.
- **File size:** warn at 500 lines, fail at 600 (`scripts/check-file-size.sh`). `internal/adapters/scheduler/dh_analytics_refresh_steps.go` is 447 lines today and this plan removes more than it adds.
- **Structured logging only:** `logger.Warn(ctx, "msg", observability.String("key", val))`.
- **Context first:** `ctx context.Context` is always the first parameter.
- **Always run `go test -race`** before any commit.
- **Commit style:** `refactor(demand): … (SLA-41)`, `test(demand): … (SLA-41)`, `fix(scheduler): … (SLA-41)`.

## File Structure

**Created**

| File | Responsibility |
|---|---|
| `internal/domain/demand/payloads.go` | The five exported payload structs. The single definition of the DH-derived JSON shapes the domain persists. |
| `internal/domain/demand/payloads_test.go` | Decode tests against real legacy blob shapes, so the structs keep reading rows already in the database. |
| `internal/adapters/storage/postgres/dh_demand_repository_test.go` | DB-free unit tests of both scan helpers: `scanCardCacheRow`'s narrowed seven-column shape (Task 3) and `scanCharacterCacheRow`'s typed decode including malformed-payload capture (Task 5). |
| `internal/testutil/mocks/row_scanner.go` | `RowScanner` double satisfying the package-private `scanner` interface, so scan helpers are testable without a database. |

**Modified**

| File | Change |
|---|---|
| `internal/domain/demand/types.go` | `CardCache` loses five blob fields; `CharacterCache` gains three typed payload pointers and `MalformedPayloads`; package doc corrected. |
| `internal/domain/demand/repository.go` | `ListCardCacheByDemandScore` removed. |
| `internal/domain/demand/service.go` | Four unexported blob structs and `parseCharacterDemand` deleted; readers re-typed; the malformed-velocity warn relocates to `parseCharacterMarket`; `encoding/json` import drops. |
| `internal/domain/demand/campaign_signals.go` | `buildSignalIndex` reads `row.Velocity`; `SkippedRows` now counts `MalformedPayloads`; `encoding/json` import drops. |
| `internal/adapters/storage/postgres/dh_demand_repository.go` | Gains generic `marshalPayload`/`unmarshalPayload`; character upsert/scan encode and decode; dead list method and card blob columns removed. |
| `internal/adapters/scheduler/dh_analytics_refresh_steps.go` | All eight `json.Marshal` sites replaced by direct wire→domain struct mapping; `encoding/json` import drops. |
| `internal/testutil/mocks/demand_repository.go` | Dead `ListCardCacheByDemandScoreFn` field and method removed. |
| `internal/testutil/mocks/logger.go` | Gains `CapturingLogger` + `LogEntry` so log assertions are possible. |
| `internal/domain/demand/service_test.go`, `campaign_signals_test.go`, `internal/adapters/scheduler/dh_analytics_refresh_test.go` | Fixtures migrate from JSON strings to typed payloads; the scheduler test gains the Defect-1 value assertions. |

---
### Task 1: Shared payload types

**Files:**
- Create `internal/domain/demand/payloads.go`
- Create `internal/domain/demand/payloads_test.go`

**Interfaces:**
- Produces: `demand.CharacterDemand`, `demand.ByEraDemand`, `demand.CharacterVelocity`,
  `demand.VelocityTierStat`, `demand.CharacterSaturation` — all exported structs, no
  methods, no consumers yet (Task 4 wires them into `CharacterCache`).
- Consumes: nothing new. `encoding/json` (stdlib) only, in the test file.

This task is purely additive — no existing file changes, tree stays green throughout.

- [ ] Read `internal/domain/demand/service_test.go` (already done for this plan — package
      is `demand_test`, table-driven tests use `tests := []struct{...}{...}` +
      `t.Run(tc.name, ...)`, and JSON string literals are built as fixtures. Match that
      style in `payloads_test.go`.
- [ ] Write `internal/domain/demand/payloads_test.go` first, referencing the five types
      below before they exist (package `demand_test`, imports
      `encoding/json`, `testing`, and `github.com/guarzo/slabledger/internal/domain/demand`):

  ```go
  package demand_test

  import (
  	"encoding/json"
  	"testing"

  	"github.com/guarzo/slabledger/internal/domain/demand"
  )

  // TestCharacterDemand_DecodesLegacyByEraBlob pins two backward-compatibility
  // behaviors the persisted rows depend on: an obsolete by_era.data_quality key
  // (no longer a field, per SLA-61) is ignored rather than rejected, and an
  // omitted total_search_clicks defaults to zero rather than failing to decode.
  func TestCharacterDemand_DecodesLegacyByEraBlob(t *testing.T) {
  	blob := `{
  		"character_name": "Umbreon",
  		"card_count": 10,
  		"avg_demand_score": 0.9,
  		"total_views": 400,
  		"total_wishlist_adds": 20,
  		"data_quality": "full",
  		"by_era": {
  			"sword_shield": {
  				"card_count": 6,
  				"avg_demand_score": 0.95,
  				"total_views": 240,
  				"total_wishlist_adds": 12,
  				"data_quality": "full"
  			}
  		}
  	}`

  	var got demand.CharacterDemand
  	if err := json.Unmarshal([]byte(blob), &got); err != nil {
  		t.Fatalf("unmarshal: %v", err)
  	}

  	if got.TotalSearchClicks != 0 {
  		t.Errorf("TotalSearchClicks = %d, want 0 (omitted field defaults to zero)", got.TotalSearchClicks)
  	}
  	era, ok := got.ByEra["sword_shield"]
  	if !ok {
  		t.Fatalf("by_era[sword_shield] missing")
  	}
  	if era.TotalSearchClicks != 0 {
  		t.Errorf("era TotalSearchClicks = %d, want 0", era.TotalSearchClicks)
  	}
  	if era.CardCount != 6 || era.AvgDemandScore != 0.95 {
  		t.Errorf("era decoded wrong: %+v", era)
  	}
  }

  // TestCharacterVelocity_DecodesLegacyFlatBlob pins that the two fields the
  // scheduler used to marshal from the raw DH struct — sell_through and
  // avg_days_to_sell — are silently dropped, and that an absent by_grade key
  // decodes to a nil map rather than an empty one.
  func TestCharacterVelocity_DecodesLegacyFlatBlob(t *testing.T) {
  	blob := `{
  		"median_days_to_sell": 9.5,
  		"sample_size": 120,
  		"sell_through": {},
  		"avg_days_to_sell": 8.1
  	}`

  	var got demand.CharacterVelocity
  	if err := json.Unmarshal([]byte(blob), &got); err != nil {
  		t.Fatalf("unmarshal: %v", err)
  	}

  	if got.SampleSize != 120 {
  		t.Errorf("SampleSize = %d, want 120", got.SampleSize)
  	}
  	if got.MedianDaysToSell == nil || *got.MedianDaysToSell != 9.5 {
  		t.Errorf("MedianDaysToSell = %v, want 9.5", got.MedianDaysToSell)
  	}
  	if got.ByGrade != nil {
  		t.Errorf("ByGrade = %v, want nil (absent key)", got.ByGrade)
  	}
  }

  // TestCharacterSaturation_FlatVsNestedLegacyShapes documents Defect 1's
  // footprint: a flat blob (today's correct shape, post-fix) decodes fully, but
  // the pre-existing nested shape the scheduler used to write for saturation
  // decodes ActiveListingCount to zero. This is not new lossy behavior — it
  // pins what already-persisted rows contain until the next scheduler refresh
  // overwrites them with the flat shape.
  func TestCharacterSaturation_FlatVsNestedLegacyShapes(t *testing.T) {
  	tests := []struct {
  		name string
  		blob string
  		want int
  	}{
  		{
  			name: "flat shape decodes the count",
  			blob: `{"active_listing_count":42,"computed_at":"2026-04-15T03:00:00Z"}`,
  			want: 42,
  		},
  		{
  			name: "legacy nested shape loses the count (Defect 1, pre-existing rows)",
  			blob: `{"character_name":"Pikachu","saturation":{"active_listing_count":42}}`,
  			want: 0,
  		},
  	}

  	for _, tc := range tests {
  		t.Run(tc.name, func(t *testing.T) {
  			var got demand.CharacterSaturation
  			if err := json.Unmarshal([]byte(tc.blob), &got); err != nil {
  				t.Fatalf("unmarshal: %v", err)
  			}
  			if got.ActiveListingCount != tc.want {
  				t.Errorf("ActiveListingCount = %d, want %d", got.ActiveListingCount, tc.want)
  			}
  		})
  	}
  }

  // TestCharacterVelocity_RoundTrip covers the pointer fields and both tier
  // maps surviving a marshal -> unmarshal cycle unchanged.
  func TestCharacterVelocity_RoundTrip(t *testing.T) {
  	medianDays := 9.5
  	changePct := 14.2
  	avgDaily := 3.2
  	sellThrough := 0.6
  	vol7 := 21
  	vol30 := 90
  	supply := 12

  	want := demand.CharacterVelocity{
  		MedianDaysToSell:   &medianDays,
  		SampleSize:         120,
  		VelocityChangePct:  &changePct,
  		AvgDailySales:      &avgDaily,
  		SellThroughRate30d: &sellThrough,
  		SalesVolume7d:      &vol7,
  		SalesVolume30d:     &vol30,
  		SupplyCount:        &supply,
  		ByGrade: map[string]demand.VelocityTierStat{
  			"9":  {MedianDays: 8.0, SampleSize: 40},
  			"10": {MedianDays: 6.5, SampleSize: 30},
  		},
  		ByPriceTier: map[string]demand.VelocityTierStat{
  			"low":  {MedianDays: 12.0, SampleSize: 20},
  			"high": {MedianDays: 4.0, SampleSize: 15},
  		},
  	}

  	blob, err := json.Marshal(want)
  	if err != nil {
  		t.Fatalf("marshal: %v", err)
  	}

  	var got demand.CharacterVelocity
  	if err := json.Unmarshal(blob, &got); err != nil {
  		t.Fatalf("unmarshal: %v", err)
  	}

  	if *got.MedianDaysToSell != *want.MedianDaysToSell {
  		t.Errorf("MedianDaysToSell = %v, want %v", *got.MedianDaysToSell, *want.MedianDaysToSell)
  	}
  	if *got.VelocityChangePct != *want.VelocityChangePct {
  		t.Errorf("VelocityChangePct = %v, want %v", *got.VelocityChangePct, *want.VelocityChangePct)
  	}
  	if *got.AvgDailySales != *want.AvgDailySales {
  		t.Errorf("AvgDailySales = %v, want %v", *got.AvgDailySales, *want.AvgDailySales)
  	}
  	if *got.SellThroughRate30d != *want.SellThroughRate30d {
  		t.Errorf("SellThroughRate30d = %v, want %v", *got.SellThroughRate30d, *want.SellThroughRate30d)
  	}
  	if *got.SalesVolume7d != *want.SalesVolume7d {
  		t.Errorf("SalesVolume7d = %v, want %v", *got.SalesVolume7d, *want.SalesVolume7d)
  	}
  	if *got.SalesVolume30d != *want.SalesVolume30d {
  		t.Errorf("SalesVolume30d = %v, want %v", *got.SalesVolume30d, *want.SalesVolume30d)
  	}
  	if *got.SupplyCount != *want.SupplyCount {
  		t.Errorf("SupplyCount = %v, want %v", *got.SupplyCount, *want.SupplyCount)
  	}
  	if len(got.ByGrade) != 2 || got.ByGrade["9"] != want.ByGrade["9"] || got.ByGrade["10"] != want.ByGrade["10"] {
  		t.Errorf("ByGrade = %+v, want %+v", got.ByGrade, want.ByGrade)
  	}
  	if len(got.ByPriceTier) != 2 || got.ByPriceTier["low"] != want.ByPriceTier["low"] || got.ByPriceTier["high"] != want.ByPriceTier["high"] {
  		t.Errorf("ByPriceTier = %+v, want %+v", got.ByPriceTier, want.ByPriceTier)
  	}
  }
  ```

- [ ] Run `go test -race ./internal/domain/demand/ -run TestCharacterDemand_DecodesLegacyByEraBlob -v`
      and confirm it fails to compile (`demand.CharacterDemand` etc. do not exist yet).
- [ ] Create `internal/domain/demand/payloads.go` with exactly the five types, copied
      verbatim from the shared context including the SLA-61 comment:

  ```go
  package demand

  // CharacterDemand is the character-aggregated demand payload persisted on
  // CharacterCache.
  type CharacterDemand struct {
  	CharacterName     string  `json:"character_name"`
  	CardCount         int     `json:"card_count"`
  	AvgDemandScore    float64 `json:"avg_demand_score"`
  	TotalViews        int     `json:"total_views"`
  	TotalSearchClicks int     `json:"total_search_clicks"`
  	TotalWishlistAdds int     `json:"total_wishlist_adds"`
  	// DataQuality is read by qualityAllowed but has no writer: DH's
  	// character demand endpoint does not report it. Character niches
  	// therefore always carry "". Tracked as SLA-61.
  	DataQuality string                 `json:"data_quality"`
  	ComputedAt  string                 `json:"computed_at"`
  	ByEra       map[string]ByEraDemand `json:"by_era,omitempty"`
  }

  type ByEraDemand struct {
  	CardCount         int     `json:"card_count"`
  	AvgDemandScore    float64 `json:"avg_demand_score"`
  	TotalViews        int     `json:"total_views"`
  	TotalSearchClicks int     `json:"total_search_clicks"`
  	TotalWishlistAdds int     `json:"total_wishlist_adds"`
  }

  type CharacterVelocity struct {
  	MedianDaysToSell   *float64                    `json:"median_days_to_sell,omitempty"`
  	SampleSize         int                         `json:"sample_size"`
  	VelocityChangePct  *float64                    `json:"velocity_change_pct,omitempty"`
  	AvgDailySales      *float64                    `json:"avg_daily_sales,omitempty"`
  	SellThroughRate30d *float64                    `json:"sell_through_rate_30d,omitempty"`
  	SalesVolume7d      *int                        `json:"sales_volume_7d,omitempty"`
  	SalesVolume30d     *int                        `json:"sales_volume_30d,omitempty"`
  	SupplyCount        *int                        `json:"supply_count,omitempty"`
  	ByGrade            map[string]VelocityTierStat `json:"by_grade,omitempty"`
  	ByPriceTier        map[string]VelocityTierStat `json:"by_price_tier,omitempty"`
  }

  type VelocityTierStat struct {
  	MedianDays float64 `json:"median_days"`
  	SampleSize int     `json:"sample_size"`
  }

  type CharacterSaturation struct {
  	ActiveListingCount int    `json:"active_listing_count"`
  	ComputedAt         string `json:"computed_at"`
  }
  ```

- [ ] Run `go test -race ./internal/domain/demand/ -run 'TestCharacterDemand_DecodesLegacyByEraBlob|TestCharacterVelocity_DecodesLegacyFlatBlob|TestCharacterSaturation_FlatVsNestedLegacyShapes|TestCharacterVelocity_RoundTrip' -v`
      and confirm all four pass (verified locally against these exact type definitions
      in a scratch module during plan authoring — all four pass).
- [ ] Run `go build ./...` to confirm the rest of the tree still compiles (this task adds
      two new, self-contained files; nothing else references the new types yet).
- [ ] Commit: `test(demand): add typed payload structs with legacy-blob decode tests (SLA-41)`

---

### Task 2: Delete the dead `ListCardCacheByDemandScore`

**Files:**
- Modify `internal/domain/demand/repository.go:16` (interface method)
- Modify `internal/adapters/storage/postgres/dh_demand_repository.go:85-118` (implementation)
- Modify `internal/testutil/mocks/demand_repository.go:14` (Fn field), `:37-42` (method)

**Interfaces:**
- Consumes: none — this is a pure deletion.
- Produces: none — `demand.Repository` loses one method; no replacement.

Verified by reading the current files (2026-08-08):
- `internal/domain/demand/repository.go:16` —
  `ListCardCacheByDemandScore(ctx context.Context, window string, limit int) ([]CardCache, error)`
  on the `Repository` interface.
- `internal/adapters/storage/postgres/dh_demand_repository.go:85-118` — the doc comment
  (85-87) plus the full implementation (88-118), ending right before the
  `CardDataQualityStats` doc comment at line 120.
- `internal/testutil/mocks/demand_repository.go:14` — the `ListCardCacheByDemandScoreFn`
  field; `:37-42` — the corresponding mock method (six lines: signature, nil-check,
  delegate call, closing braces).

This is a deletion task, so the TDD cycle is inverted: the "test" proving safety is a
grep for zero callers, not a new test asserting behavior.

- [ ] Run `grep -rn 'ListCardCacheByDemandScore' internal/ cmd/` and confirm the only
      three hits are the interface declaration, the postgres implementation, and the two
      mock lines above (verified during plan authoring — no fourth call site anywhere
      under `internal/` or `cmd/`).
- [ ] Delete the `ListCardCacheByDemandScore` line from the `Repository` interface in
      `internal/domain/demand/repository.go`. Resulting interface:

  ```go
  package demand

  import (
  	"context"

  	"github.com/guarzo/slabledger/internal/domain/inventory"
  )

  // Repository persists and retrieves the cached DH demand and analytics rows
  // that back the niche-opportunity leaderboard. The SQLite adapter
  // (internal/adapters/storage/postgres) implements this interface.
  type Repository interface {
  	// Card cache
  	UpsertCardCache(ctx context.Context, row CardCache) error
  	GetCardCache(ctx context.Context, cardID, window string) (*CardCache, error)
  	CardDataQualityStats(ctx context.Context, window string) (QualityStats, error)

  	// Character cache
  	UpsertCharacterCache(ctx context.Context, row CharacterCache) error
  	GetCharacterCache(ctx context.Context, character, window string) (*CharacterCache, error)
  	ListCharacterCache(ctx context.Context, window string) ([]CharacterCache, error)
  }
  ```

  (Only the `Repository` interface block is shown; the rest of the file —
  `ActiveCampaign`, `ActiveCampaignSource`, `CampaignCoverageLookup` — is unchanged.)

- [ ] Delete the `ListCardCacheByDemandScore` method (with its doc comment) from
      `internal/adapters/storage/postgres/dh_demand_repository.go`. The file's `--- Card
      cache CRUD ---` section now reads `UpsertCardCache` → `GetCardCache` →
      `CardDataQualityStats`, with no method in between.
- [ ] Delete the `ListCardCacheByDemandScoreFn` field and the `ListCardCacheByDemandScore`
      method from `internal/testutil/mocks/demand_repository.go`. Resulting
      `DemandRepositoryMock`:

  ```go
  package mocks

  import (
  	"context"

  	"github.com/guarzo/slabledger/internal/domain/demand"
  )

  // DemandRepositoryMock is a Fn-field mock of demand.Repository.
  // Unset Fn fields return zero values + nil error.
  type DemandRepositoryMock struct {
  	UpsertCardCacheFn      func(ctx context.Context, row demand.CardCache) error
  	GetCardCacheFn         func(ctx context.Context, cardID, window string) (*demand.CardCache, error)
  	CardDataQualityStatsFn func(ctx context.Context, window string) (demand.QualityStats, error)
  	UpsertCharacterCacheFn func(ctx context.Context, row demand.CharacterCache) error
  	GetCharacterCacheFn    func(ctx context.Context, character, window string) (*demand.CharacterCache, error)
  	ListCharacterCacheFn   func(ctx context.Context, window string) ([]demand.CharacterCache, error)
  }

  var _ demand.Repository = (*DemandRepositoryMock)(nil)

  func (m *DemandRepositoryMock) UpsertCardCache(ctx context.Context, row demand.CardCache) error {
  	if m.UpsertCardCacheFn != nil {
  		return m.UpsertCardCacheFn(ctx, row)
  	}
  	return nil
  }

  func (m *DemandRepositoryMock) GetCardCache(ctx context.Context, cardID, window string) (*demand.CardCache, error) {
  	if m.GetCardCacheFn != nil {
  		return m.GetCardCacheFn(ctx, cardID, window)
  	}
  	return nil, nil
  }

  func (m *DemandRepositoryMock) CardDataQualityStats(ctx context.Context, window string) (demand.QualityStats, error) {
  	if m.CardDataQualityStatsFn != nil {
  		return m.CardDataQualityStatsFn(ctx, window)
  	}
  	return demand.QualityStats{}, nil
  }

  func (m *DemandRepositoryMock) UpsertCharacterCache(ctx context.Context, row demand.CharacterCache) error {
  	if m.UpsertCharacterCacheFn != nil {
  		return m.UpsertCharacterCacheFn(ctx, row)
  	}
  	return nil
  }

  func (m *DemandRepositoryMock) GetCharacterCache(ctx context.Context, character, window string) (*demand.CharacterCache, error) {
  	if m.GetCharacterCacheFn != nil {
  		return m.GetCharacterCacheFn(ctx, character, window)
  	}
  	return nil, nil
  }

  func (m *DemandRepositoryMock) ListCharacterCache(ctx context.Context, window string) ([]demand.CharacterCache, error) {
  	if m.ListCharacterCacheFn != nil {
  		return m.ListCharacterCacheFn(ctx, window)
  	}
  	return nil, nil
  }
  ```

  (`CampaignCoverageLookupMock` below it in the same file is unchanged.)

- [ ] Run `go build ./...` and confirm it succeeds — this is the "test" for a deletion:
      if any caller had been missed, the build breaks here.
- [ ] Run `go test -race ./internal/domain/demand/... ./internal/adapters/storage/postgres/... ./internal/testutil/...`
      and confirm all still pass (no test exercised the deleted method, so none should
      need edits).
- [ ] Run `grep -rn 'ListCardCacheByDemandScore' internal/ cmd/` again and confirm it now
      returns nothing.
- [ ] Commit: `refactor(demand): delete the dead ListCardCacheByDemandScore (SLA-41)`

---

### Task 3: Delete the five `CardCache` blob fields

**Files:**
- Modify `internal/domain/demand/types.go:20-24` (struct fields)
- Modify `internal/adapters/storage/postgres/dh_demand_repository.go` — `UpsertCardCache`,
  `GetCardCache`, `scanCardCacheRow` (post-Task-2 line numbers; re-derive by reading the
  file after Task 2's edits land, since deleting one method shifts everything below it)
- Modify `internal/adapters/scheduler/dh_analytics_refresh_steps.go` — card branch of
  `refreshCards` only (velocity ~242-249, trend ~250-257, saturation ~258-265, price
  distribution ~266-273, demand ~314-319 — verified against the file as it reads before
  this task's edits; line numbers are unaffected by Task 2, which touches a different
  file)
- Create: `internal/testutil/mocks/row_scanner.go`
- Create: `internal/adapters/storage/postgres/dh_demand_repository_test.go`
- Read only: `internal/adapters/storage/postgres/purchase_scan_helpers.go:69-71` (the
  package-private `scanner` interface the new mock has to satisfy)

**Interfaces:**
- Consumes: `scanner` (package-private, `internal/adapters/storage/postgres/purchase_scan_helpers.go:69-71`) — `type scanner interface { Scan(dest ...any) error }`. Confirmed this is the exact declaration; no export shim exists or is needed, since `RowScanner` satisfies it structurally.
- Produces: `mocks.RowScanner{Values []any; Err error}` with method `Scan(dest ...any) error`. **Task 5 consumes this and does not re-create it.**
- Produces: `internal/adapters/storage/postgres/dh_demand_repository_test.go` (`package postgres`), which Task 5 appends a second test function to.

**Why this task carries a test, when the surrounding tasks are pure deletion.** This
task rewrites an INSERT's placeholder numbering (`$1..$12` → `$1..$7`), a SELECT's column
list, and a scan-destination list — three edits where a mistake is a *runtime* error, not
a compile error, so `go build` cannot catch it. And there is nothing else watching:
`grep -rn 'CardCache' internal/adapters/storage/postgres/*_test.go` returns **nothing**
today — the card-cache path has zero test coverage. `scanCardCacheRow` is also stable
after this task (Task 4 touches only the *character* scanner), so the test written here
does not need rewriting later.

The five DB columns (`demand_json`, `velocity_json`, `trend_json`, `saturation_json`,
`price_distribution_json` on `dh_card_cache`) are **intentionally left in place**. Per the
spec's "Column removal is a separate release" section: migrations run automatically and
unconditionally on startup, but rolling the app image back does **not** run the Down
migration (`docs/OPERATIONS.md:87`). If this release dropped the columns and then had to
be rolled back, the old binary — which still names all five columns explicitly in its
`UpsertCardCache`/`GetCardCache` SQL — would fail outright against a schema that no longer
has them. Since this repo auto-deploys on push to `main`, that rollback path is the
ordinary recovery path, not a corner case. So this task **must not** create a migration:
dropping the columns is a separate follow-up ticket (Task 7 files it), landing only once
this release is confirmed stable.

- [ ] Remove `DemandJSON`, `VelocityJSON`, `TrendJSON`, `SaturationJSON`, and
      `PriceDistributionJSON` from `CardCache` in `internal/domain/demand/types.go`
      (currently lines 20-24, verified by reading the file). Resulting struct:

  ```go
  // CardCache is the domain view of a dh_card_cache row. Nullable SQL columns
  // map to pointer fields so callers can distinguish NULL from the zero value.
  type CardCache struct {
  	CardID              string
  	Window              string // "7d" or "30d"
  	DemandScore         *float64
  	DemandDataQuality   *string // "proxy" | "full"
  	AnalyticsComputedAt *time.Time
  	DemandComputedAt    *time.Time
  	FetchedAt           time.Time
  }
  ```

  (`CharacterCache`, `QualityStats`, and everything below in `types.go` are unchanged in
  this task — the three `CharacterCache` blob fields are Task 4's concern.)

- [ ] Strip the five blob columns from `UpsertCardCache`, `GetCardCache`, and
      `scanCardCacheRow` in `internal/adapters/storage/postgres/dh_demand_repository.go`.
      Verified against the file as read during plan authoring (pre-Task-2 line numbers:
      `UpsertCardCache` 31-62, `GetCardCache` 65-83, `scanCardCacheRow` 223-256 — re-check
      after Task 2's deletion shifts everything from `ListCardCacheByDemandScore` onward
      down by roughly 34 lines). Resulting code:

  ```go
  // UpsertCardCache inserts or updates a dh_card_cache row keyed by (card_id, window).
  func (r *DHDemandRepository) UpsertCardCache(ctx context.Context, row demand.CardCache) error {
  	_, err := r.db.ExecContext(ctx,
  		`INSERT INTO dh_card_cache (
  			card_id, "window",
  			demand_score, demand_data_quality,
  			analytics_computed_at, demand_computed_at, fetched_at
  		) VALUES ($1, $2, $3, $4, $5, $6, $7)
  		ON CONFLICT(card_id, "window") DO UPDATE SET
  			demand_score          = excluded.demand_score,
  			demand_data_quality   = excluded.demand_data_quality,
  			analytics_computed_at = excluded.analytics_computed_at,
  			demand_computed_at    = excluded.demand_computed_at,
  			fetched_at            = excluded.fetched_at`,
  		row.CardID, row.Window,
  		nullFloat64FromPtr(row.DemandScore), nullStringFromPtr(row.DemandDataQuality),
  		nullTimeFromPtr(row.AnalyticsComputedAt), nullTimeFromPtr(row.DemandComputedAt),
  		row.FetchedAt,
  	)
  	if err != nil {
  		return fmt.Errorf("upsert dh_card_cache: %w", err)
  	}
  	return nil
  }

  // GetCardCache returns the cached row for (cardID, window), or (nil, nil) if not found.
  func (r *DHDemandRepository) GetCardCache(ctx context.Context, cardID, window string) (*demand.CardCache, error) {
  	row := r.db.QueryRowContext(ctx,
  		`SELECT card_id, "window",
  			demand_score, demand_data_quality,
  			analytics_computed_at, demand_computed_at, fetched_at
  		FROM dh_card_cache
  		WHERE card_id = $1 AND "window" = $2`,
  		cardID, window,
  	)
  	result, err := scanCardCacheRow(row)
  	if errors.Is(err, sql.ErrNoRows) {
  		return nil, nil
  	}
  	if err != nil {
  		return nil, fmt.Errorf("get dh_card_cache: %w", err)
  	}
  	return result, nil
  }

  func scanCardCacheRow(s scanner) (*demand.CardCache, error) {
  	var (
  		row                 demand.CardCache
  		demandScore         sql.NullFloat64
  		demandDataQuality   sql.NullString
  		analyticsComputedAt sql.NullTime
  		demandComputedAt    sql.NullTime
  	)

  	if err := s.Scan(
  		&row.CardID, &row.Window,
  		&demandScore, &demandDataQuality,
  		&analyticsComputedAt, &demandComputedAt, &row.FetchedAt,
  	); err != nil {
  		return nil, err
  	}

  	row.DemandScore = nullFloat64ToPtr(demandScore)
  	row.DemandDataQuality = nullStringToPtr(demandDataQuality)
  	row.AnalyticsComputedAt = nullTimeToPtr(analyticsComputedAt)
  	row.DemandComputedAt = nullTimeToPtr(demandComputedAt)
  	return &row, nil
  }
  ```

  Placeholder numbering: the INSERT now has 7 columns / 7 placeholders (`$1`-`$7`), not
  12 / 12 — `analytics_computed_at`, `demand_computed_at`, and `fetched_at` shift from
  `$10`-`$12` to `$5`-`$7`. The SELECT and scan drop from 12 to 7 columns in the same
  positions. `scanCardCacheRow`'s local `sql.Null*` vars drop from 7 to 4
  (`demandScore`, `demandDataQuality`, `analyticsComputedAt`, `demandComputedAt`), and the
  five `row.*JSON = nullStringToPtr(...)` assignments are gone entirely.

  (`ListCardCacheByDemandScore` no longer exists after Task 2, so there is no third call
  site to touch. `UpsertCharacterCache`, `GetCharacterCache`, `ListCharacterCache`, and
  `scanCharacterCacheRow` below in the same file are unchanged — Task 4's concern.)

- [ ] Strip the five blob assignments from the card branch of `refreshCards` in
      `internal/adapters/scheduler/dh_analytics_refresh_steps.go`: the `row.Velocity`
      block (242-249), `row.Trend` block (250-257), `row.Saturation` block (258-265),
      `row.PriceDistribution` block (266-273), and the `json.Marshal(ds)` /
      `cache.DemandJSON` block inside the `DemandSignals` loop (314-319). Resulting
      `refreshCards`:

  ```go
  // refreshCards runs Step 2: BatchAnalytics + DemandSignals, upserts per-card
  // cache rows, returns 404 analytics_not_computed count and total API call count.
  func (s *DHAnalyticsRefreshScheduler) refreshCards(ctx context.Context, cardIDs []int) (notComputed int, apiCalls int) {
  	if len(cardIDs) == 0 {
  		s.logger.Info(ctx, "no card IDs to refresh")
  		return 0, 0
  	}

  	// --- BatchAnalytics ---
  	apiCalls++
  	analyticsResp, err := s.dhClient.BatchAnalytics(ctx, cardIDs, []string{"velocity", "trend", "saturation", "price_distribution"})
  	if err != nil {
  		s.logger.Warn(ctx, "batch_analytics failed", observability.Err(err))
  	}

  	now := time.Now()
  	analyticsByID := make(map[int]dh.CardAnalytics)
  	if analyticsResp != nil {
  		for _, row := range analyticsResp.Results {
  			if row.Error != "" {
  				if row.Error == "analytics_not_computed" {
  					notComputed++
  					continue
  				}
  				s.logger.Debug(ctx, "per-card analytics error",
  					observability.Int("card_id", row.CardID),
  					observability.String("dh_error", row.Error))
  				continue
  			}
  			analyticsByID[row.CardID] = row
  		}
  	}

  	for cardID, row := range analyticsByID {
  		cache := demand.CardCache{
  			CardID:    strconv.Itoa(cardID),
  			Window:    s.config.Window,
  			FetchedAt: now,
  		}
  		if t, tErr := parseDHTimestamp(row.ComputedAt); tErr == nil {
  			cache.AnalyticsComputedAt = &t
  		}
  		if err := s.repo.UpsertCardCache(ctx, cache); err != nil {
  			s.logger.Warn(ctx, "upsert card cache (analytics) failed",
  				observability.Int("card_id", cardID),
  				observability.Err(err))
  		}
  	}

  	// --- DemandSignals ---
  	apiCalls++
  	demandResp, err := s.dhClient.DemandSignals(ctx, cardIDs, s.config.Window)
  	if err != nil {
  		s.logger.Warn(ctx, "demand_signals failed", observability.Err(err))
  		return notComputed, apiCalls
  	}
  	if demandResp == nil {
  		return notComputed, apiCalls
  	}

  	for _, ds := range demandResp.DemandSignals {
  		cardID := ds.CardID
  		cache, getErr := s.repo.GetCardCache(ctx, strconv.Itoa(cardID), s.config.Window)
  		if getErr != nil {
  			s.logger.Debug(ctx, "get card cache failed (pre-merge)",
  				observability.Int("card_id", cardID),
  				observability.Err(getErr))
  		}
  		if cache == nil {
  			cache = &demand.CardCache{
  				CardID:    strconv.Itoa(cardID),
  				Window:    s.config.Window,
  				FetchedAt: now,
  			}
  		}
  		score := ds.DemandScore
  		cache.DemandScore = &score
  		quality := ds.DataQuality
  		cache.DemandDataQuality = &quality
  		cache.DemandComputedAt = &now
  		cache.FetchedAt = now
  		if err := s.repo.UpsertCardCache(ctx, *cache); err != nil {
  			s.logger.Warn(ctx, "upsert card cache (demand) failed",
  				observability.Int("card_id", cardID),
  				observability.Err(err))
  		}
  	}
  	return notComputed, apiCalls
  }
  ```

  (`refreshCharacters` above `refreshCards` in the same file is untouched by this task —
  it still calls `json.Marshal` for the character demand/velocity/saturation blobs at
  lines 133, 141, and 152, so `encoding/json` **stays** an active import of this file at
  this point in the sequence. It only drops out once Task 6/7's typed character mapping
  replaces those three call sites — not part of this task. Verified by reading the file:
  no other `json.` usage exists in the file besides those three plus the one this task
  removes at line 314, so removing line 314's usage alone would not make the import
  unused.)

- [ ] Write `internal/testutil/mocks/row_scanner.go`. This is a new file — confirmed:
      `ls internal/testutil/mocks/` has no `row*` or `scan*` entry (the closest neighbor,
      `psa_row_provider.go`, is a different kind of double). `internal/testutil/mocks`
      does not import `internal/adapters/storage/postgres` anywhere today, and this file
      imports only `fmt` and `reflect`, so it creates no import cycle.

  ```go
  package mocks

  import (
  	"fmt"
  	"reflect"
  )

  // RowScanner is a test double for the postgres package's unexported
  // `scanner` interface (`Scan(dest ...any) error`), which *sql.Row and
  // *sql.Rows both satisfy structurally. It lets row-scanning functions be
  // unit-tested without a database.
  //
  // Values holds the column values in the exact order the function under
  // test scans them. Scan assigns Values[i] into dest[i] via reflection
  // rather than a type switch: postgres row scanners scan into a long tail
  // of concrete types (string, time.Time, sql.NullString, sql.NullTime,
  // sql.NullFloat64, ...), and a type switch would need one case per type
  // per test, duplicated across every scanner test in the package.
  // Reflection handles all of them uniformly; the cost is that a
  // Values[i] whose type doesn't match *dest[i] panics instead of failing
  // gracefully, so fixtures must supply exactly the type the real driver
  // would produce (e.g. sql.NullString{...}, not *string).
  type RowScanner struct {
  	// Values are the column values, in scan order.
  	Values []any
  	// Err, if set, is returned by Scan without assigning anything.
  	Err error
  }

  // Scan assigns each Values[i] into dest[i]. It returns an error if the
  // number of destinations does not match the number of supplied values.
  func (r *RowScanner) Scan(dest ...any) error {
  	if r.Err != nil {
  		return r.Err
  	}
  	if len(dest) != len(r.Values) {
  		return fmt.Errorf("mocks.RowScanner: got %d scan destinations, have %d values", len(dest), len(r.Values))
  	}
  	for i, d := range dest {
  		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(r.Values[i]))
  	}
  	return nil
  }
  ```

  Verified gofmt-clean (written to a temp file, `gofmt -l` produced no output, `gofmt`
  output byte-identical to the source above).

- [ ] Write `internal/adapters/storage/postgres/dh_demand_repository_test.go`. This is a
      new file — confirmed: `ls internal/adapters/storage/postgres/*_test.go` has no such
      entry, and `grep -rn 'CardCache' internal/adapters/storage/postgres/*_test.go`
      returns nothing. It is `package postgres` (not `postgres_test`), like every other
      test file in the package (`retry_test.go:1`), because `scanCardCacheRow` is
      unexported.

      **Read the point of this test before writing it.** The `len(dest) != len(r.Values)`
      guard inside `RowScanner.Scan` is what makes it a real check on this task's edit:
      supplying exactly seven fixture values makes the test fail loudly if
      `scanCardCacheRow` still passes twelve destinations — i.e. if any of the five blob
      fields survived the deletion. The per-field assertions then pin the *order* of the
      remaining seven against the post-edit SELECT list. What this cannot check is the
      SQL text itself, since no database is involved; the `go build` + eyeball step below
      still stands for the `UpsertCardCache` placeholder renumbering.

  ```go
  package postgres

  import (
  	"database/sql"
  	"testing"
  	"time"

  	"github.com/guarzo/slabledger/internal/domain/demand"
  	"github.com/guarzo/slabledger/internal/testutil/mocks"
  )

  func TestScanCardCacheRow(t *testing.T) {
  	fetchedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
  	demandComputedAt := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
  	analyticsComputedAt := time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC)

  	tests := []struct {
  		name   string
  		values []any
  		assert func(t *testing.T, row *demand.CardCache)
  	}{
  		{
  			name: "all columns populated",
  			values: []any{
  				"sv1-25", "30d",
  				sql.NullFloat64{Float64: 0.82, Valid: true},
  				sql.NullString{String: "full", Valid: true},
  				sql.NullTime{Time: analyticsComputedAt, Valid: true},
  				sql.NullTime{Time: demandComputedAt, Valid: true},
  				fetchedAt,
  			},
  			assert: func(t *testing.T, row *demand.CardCache) {
  				if row.CardID != "sv1-25" {
  					t.Errorf("CardID = %q, want %q", row.CardID, "sv1-25")
  				}
  				if row.Window != "30d" {
  					t.Errorf("Window = %q, want %q", row.Window, "30d")
  				}
  				if row.DemandScore == nil || *row.DemandScore != 0.82 {
  					t.Errorf("DemandScore = %v, want 0.82", row.DemandScore)
  				}
  				if row.DemandDataQuality == nil || *row.DemandDataQuality != "full" {
  					t.Errorf("DemandDataQuality = %v, want \"full\"", row.DemandDataQuality)
  				}
  				if row.AnalyticsComputedAt == nil || !row.AnalyticsComputedAt.Equal(analyticsComputedAt) {
  					t.Errorf("AnalyticsComputedAt = %v, want %v", row.AnalyticsComputedAt, analyticsComputedAt)
  				}
  				if row.DemandComputedAt == nil || !row.DemandComputedAt.Equal(demandComputedAt) {
  					t.Errorf("DemandComputedAt = %v, want %v", row.DemandComputedAt, demandComputedAt)
  				}
  				if !row.FetchedAt.Equal(fetchedAt) {
  					t.Errorf("FetchedAt = %v, want %v", row.FetchedAt, fetchedAt)
  				}
  			},
  		},
  		{
  			name: "every nullable column NULL",
  			values: []any{
  				"sv1-26", "7d",
  				sql.NullFloat64{},
  				sql.NullString{},
  				sql.NullTime{},
  				sql.NullTime{},
  				fetchedAt,
  			},
  			assert: func(t *testing.T, row *demand.CardCache) {
  				if row.DemandScore != nil {
  					t.Errorf("DemandScore = %v, want nil", row.DemandScore)
  				}
  				if row.DemandDataQuality != nil {
  					t.Errorf("DemandDataQuality = %v, want nil", row.DemandDataQuality)
  				}
  				if row.AnalyticsComputedAt != nil {
  					t.Errorf("AnalyticsComputedAt = %v, want nil", row.AnalyticsComputedAt)
  				}
  				if row.DemandComputedAt != nil {
  					t.Errorf("DemandComputedAt = %v, want nil", row.DemandComputedAt)
  				}
  			},
  		},
  	}

  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			row, err := scanCardCacheRow(&mocks.RowScanner{Values: tt.values})
  			if err != nil {
  				t.Fatalf("scanCardCacheRow: %v", err)
  			}
  			tt.assert(t, row)
  		})
  	}
  }
  ```

  Verified gofmt-clean (written to a temp file, `gofmt -l` produced no output).

- [ ] Run `go test -race ./internal/adapters/storage/postgres/ -run TestScanCardCacheRow -v`
      and confirm **both subtests pass**. If you see
      `mocks.RowScanner: got 12 scan destinations, have 7 values`, the five blob fields
      are still being scanned — go back and finish the `scanCardCacheRow` edit. If a
      subtest fails on a *field* assertion, the seven remaining columns are scanned in
      the wrong order relative to the SELECT list; fix the code, not the fixture.
- [ ] Run `go build ./...` and confirm it compiles (this is the "test" for the SQL
      placeholder renumbering — a mismatched arg count/placeholder is a runtime error,
      not a compile error, so also eyeball the renumbered SQL against the `Exec`/`Scan`
      argument lists above before moving on).
- [ ] Run `go test -race ./internal/domain/demand/... ./internal/adapters/storage/postgres/... ./internal/adapters/scheduler/...`
      and confirm all pass. Update any test fixture that still constructs a `CardCache`
      literal with one of the five removed fields — grep first:
      `grep -rn 'DemandJSON:\|VelocityJSON:\|TrendJSON:\|SaturationJSON:\|PriceDistributionJSON:' internal/adapters/scheduler internal/adapters/storage/postgres`
      scoped to `CardCache` literals (the `CharacterCache` literals with the same field
      names are untouched — Task 4's concern; use the surrounding `demand.CardCache{`
      literal context to tell them apart).
- [ ] Run `grep -rn 'TrendJSON\|PriceDistributionJSON' internal/ cmd/` and confirm it
      returns nothing — these two field names exist nowhere except the two now-deleted
      `CardCache` fields, so a clean grep proves no remaining reader or writer.
- [ ] Run `ls internal/adapters/storage/postgres/migrations/` and confirm no new
      migration file was added — the DB columns stay in place.
- [ ] Commit (include the two new files): `refactor(demand): stop reading and writing the five CardCache blob columns (SLA-41)`
### Task 4: The typed `CharacterCache` seam

This is the big, atomic task: the tree cannot compile between the moment
`CharacterCache`'s three blob fields become typed and the moment every
consumer (postgres adapter, service, campaign signals, scheduler, tests) is
updated to match. Everything below lands in one commit.

**Files:**
- Modify `internal/adapters/scheduler/dh_analytics_refresh_test.go:133-238` (append regression assertions to `TestDHAnalyticsRefresh_HappyPath`)
- Modify `internal/domain/demand/types.go:1-7` (package doc), `:30-40` (`CharacterCache`)
- Modify `internal/adapters/storage/postgres/dh_demand_repository.go:143-167` (`UpsertCharacterCache`), `:258-282` (`scanCharacterCacheRow`); add `marshalPayload`/`unmarshalPayload` helpers
- Modify `internal/domain/demand/service.go:1-12` (imports), `:126` (Leaderboard call site), `:253-298` (delete four blob structs), `:300-311` (delete `parseCharacterDemand`), `:313-353` (`parseCharacterMarket`), `:359-372` (`erasForRow`), `:376-401` (`eraDemandFor`)
- Modify `internal/domain/demand/campaign_signals.go:1-12` (imports), `:132-166` (`buildSignalIndex`)
- Modify `internal/adapters/scheduler/dh_analytics_refresh_steps.go:1-13` (imports), `:126-167` (character branch), add `mapCharacterDemand`/`mapCharacterVelocity`/`mapCharacterSaturation` helpers
- Modify `internal/domain/demand/service_test.go:6` (drop `strconv` import), `:14` (delete `strPtr`), `:54` (delete `floatStr`), `:58-72` (`demandJSONWithEras` → `demandPayload`), 16 call sites at lines 104,105,106,140,141,168,169,188,189,218,240,267,268,302,303,316, `:287` (`velocityWithChange`)
- Modify `internal/domain/demand/campaign_signals_test.go:17-24` (`velocityJSON` → `velocityPayload`), `:28-30` (`velocityJSONNoChange` → `velocityPayloadNoChange`), `:32-40` (`charRow`), `:42-50` (`charRowNoChange`) — keep `strconv` import (still used at `:257`)
- Read-only reference: `internal/adapters/clients/dh/types_analytics.go:176-184` (`CharacterDemandEntry`), `:187-193` (`ByEraEntry`), `:224-229` (`CharacterVelocityEntry`), `:233-246` (`CharacterVelocityFields`), `:255-260` (`CharacterSaturationEntry`), `:263-270` (`CharacterSaturationFields`), `:42-45` (`VelocityTierStat`)

**Interfaces:**
- Consumes: `demand.CharacterDemand`, `demand.ByEraDemand`, `demand.CharacterVelocity`, `demand.VelocityTierStat`, `demand.CharacterSaturation` from `internal/domain/demand/payloads.go` (Task 1)
- Produces: `demand.CharacterCache` with typed `Demand *CharacterDemand`, `Velocity *CharacterVelocity`, `Saturation *CharacterSaturation`, `MalformedPayloads []demand.MalformedPayload`; `demand.MalformedPayload{Column string, Err error}`
- Unchanged signatures: `demand.Repository.UpsertCharacterCache(ctx, CharacterCache) error`, `.GetCharacterCache(ctx, character, window string) (*CharacterCache, error)`, `.ListCharacterCache(ctx, window string) ([]CharacterCache, error)`

---

- [ ] **Step 1 — write the failing Defect-1 regression assertions.**

  Append to `TestDHAnalyticsRefresh_HappyPath` in
  `internal/adapters/scheduler/dh_analytics_refresh_test.go`, right after the
  existing `client.topCharactersCalls` check (currently the last statement
  before the closing brace at line 238). The fake client already feeds
  Charizard velocity `MedianDaysToSell: ptrF64(14.5), SampleSize: 120` and
  Pikachu `Saturation: dh.CharacterSaturationFields{ActiveListingCount: 42}`
  (lines 151-177); `upsertCharacters` already captures every
  `UpsertCharacterCache` call (line 202, 206-209). The existing assertions
  only check counts — exactly why Defect 1 (nested saturation marshal)
  survived undetected. These new assertions check the persisted *values*:

  ```go
  	var charizard, pikachu *demand.CharacterCache
  	for i := range upsertCharacters {
  		switch upsertCharacters[i].Character {
  		case "Charizard":
  			charizard = &upsertCharacters[i]
  		case "Pikachu":
  			pikachu = &upsertCharacters[i]
  		}
  	}
  	if pikachu == nil || pikachu.Saturation == nil {
  		t.Fatalf("expected Pikachu row with a Saturation payload; got %+v", pikachu)
  	}
  	if pikachu.Saturation.ActiveListingCount != 42 {
  		t.Fatalf("want Pikachu ActiveListingCount=42 (Defect 1 regression); got %d", pikachu.Saturation.ActiveListingCount)
  	}
  	if charizard == nil || charizard.Velocity == nil {
  		t.Fatalf("expected Charizard row with a Velocity payload; got %+v", charizard)
  	}
  	if charizard.Velocity.MedianDaysToSell == nil || *charizard.Velocity.MedianDaysToSell != 14.5 {
  		t.Fatalf("want Charizard MedianDaysToSell=14.5; got %v", charizard.Velocity.MedianDaysToSell)
  	}
  	if charizard.Velocity.SampleSize != 120 {
  		t.Fatalf("want Charizard SampleSize=120; got %d", charizard.Velocity.SampleSize)
  	}
  ```

  `ptrF64` already exists in this test file (line 367) — no new helper needed.

  Run it: `go test -race ./internal/adapters/scheduler/ -run TestDHAnalyticsRefresh_HappyPath -v`. Expected failing state for this cycle: a **compile error**, not a test failure — `demand.CharacterCache` has no `Saturation` or `Velocity` field until Step 3 below. Confirm you see a compile error naming those fields, then proceed.

- [ ] **Step 2 — retype `CharacterCache` in `internal/domain/demand/types.go`.**

  Replace the package doc comment (lines 1-7), which currently claims DH JSON
  payloads "are parsed here into domain-local structs" — after this change
  parsing lives in the postgres adapter, not here:

  ```go
  // Package demand defines the domain types, repository contract, and service
  // that compute niche-opportunity leaderboards from DoubleHolo (DH) market
  // analytics and demand signals. It is a flat sibling under internal/domain/
  // and does not import any adapter or other domain-sibling packages. DH JSON
  // payloads are decoded by the postgres adapter into the exported structs in
  // payloads.go, so the scoring logic here depends only on those domain types,
  // not the wire format.
  package demand
  ```

  Replace `CharacterCache` (lines 30-40):

  ```go
  // CharacterCache is the domain view of a dh_character_cache row.
  type CharacterCache struct {
  	Character           string
  	Window              string
  	Demand              *CharacterDemand
  	Velocity            *CharacterVelocity
  	Saturation          *CharacterSaturation
  	DemandComputedAt    *time.Time
  	AnalyticsComputedAt *time.Time
  	FetchedAt           time.Time
  	// MalformedPayloads records payload columns that were present but failed
  	// to decode, with the decode error preserved. See postgres.scanCharacterCacheRow.
  	MalformedPayloads []MalformedPayload
  }

  // MalformedPayload names a payload column that failed to decode and carries
  // the error, so the domain can log the same diagnostic the adapter cannot.
  type MalformedPayload struct {
  	// Column is "demand", "velocity", or "saturation".
  	Column string
  	Err    error
  }
  ```

  Run: `go build ./internal/domain/demand/`. Expect failures downstream in
  `service.go`, `campaign_signals.go`, and the postgres adapter — that is the
  next three steps.

- [ ] **Step 3 — rewrite the postgres adapter to marshal/unmarshal the typed payloads.**

  In `internal/adapters/storage/postgres/dh_demand_repository.go`, add
  `"encoding/json"` to the import block. Add the generic helpers (place them
  between `scanCharacterCacheRow` and the pointer↔`sql.Null*` helpers, which
  stay unchanged at lines 286-329):

  ```go
  // marshalPayload encodes a payload struct into a nullable column value. A nil
  // pointer maps to SQL NULL.
  func marshalPayload[T any](p *T) (sql.NullString, error) {
  	if p == nil {
  		return sql.NullString{}, nil
  	}
  	blob, err := json.Marshal(p)
  	if err != nil {
  		return sql.NullString{}, err
  	}
  	return sql.NullString{String: string(blob), Valid: true}, nil
  }

  // unmarshalPayload decodes a nullable column value into a payload struct. A
  // NULL column maps to a nil pointer with no error.
  func unmarshalPayload[T any](n sql.NullString) (*T, error) {
  	if !n.Valid {
  		return nil, nil
  	}
  	var v T
  	if err := json.Unmarshal([]byte(n.String), &v); err != nil {
  		return nil, err
  	}
  	return &v, nil
  }
  ```

  Rewrite `UpsertCharacterCache` (lines 143-167) to marshal each payload,
  wrapping encode failures with column-specific context:

  ```go
  // UpsertCharacterCache inserts or updates a dh_character_cache row keyed by (character, window).
  func (r *DHDemandRepository) UpsertCharacterCache(ctx context.Context, row demand.CharacterCache) error {
  	demandBlob, err := marshalPayload(row.Demand)
  	if err != nil {
  		return fmt.Errorf("encode dh_character_cache demand payload: %w", err)
  	}
  	velocityBlob, err := marshalPayload(row.Velocity)
  	if err != nil {
  		return fmt.Errorf("encode dh_character_cache velocity payload: %w", err)
  	}
  	saturationBlob, err := marshalPayload(row.Saturation)
  	if err != nil {
  		return fmt.Errorf("encode dh_character_cache saturation payload: %w", err)
  	}

  	_, err = r.db.ExecContext(ctx,
  		`INSERT INTO dh_character_cache (
  			character, "window",
  			demand_json, velocity_json, saturation_json,
  			demand_computed_at, analytics_computed_at, fetched_at
  		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
  		ON CONFLICT(character, "window") DO UPDATE SET
  			demand_json           = excluded.demand_json,
  			velocity_json         = excluded.velocity_json,
  			saturation_json       = excluded.saturation_json,
  			demand_computed_at    = excluded.demand_computed_at,
  			analytics_computed_at = excluded.analytics_computed_at,
  			fetched_at            = excluded.fetched_at`,
  		row.Character, row.Window,
  		demandBlob, velocityBlob, saturationBlob,
  		nullTimeFromPtr(row.DemandComputedAt), nullTimeFromPtr(row.AnalyticsComputedAt),
  		row.FetchedAt,
  	)
  	if err != nil {
  		return fmt.Errorf("upsert dh_character_cache: %w", err)
  	}
  	return nil
  }
  ```

  Rewrite `scanCharacterCacheRow` (lines 258-282) to decode each column,
  appending a `demand.MalformedPayload` on decode failure while leaving the
  field nil and **not** failing the scan — the repo struct has no logger
  (only `db *sql.DB`), which is why the diagnostic travels on the row instead
  of being logged here:

  ```go
  func scanCharacterCacheRow(s scanner) (*demand.CharacterCache, error) {
  	var (
  		row                 demand.CharacterCache
  		demandJSON          sql.NullString
  		velocityJSON        sql.NullString
  		saturationJSON      sql.NullString
  		demandComputedAt    sql.NullTime
  		analyticsComputedAt sql.NullTime
  	)

  	if err := s.Scan(
  		&row.Character, &row.Window,
  		&demandJSON, &velocityJSON, &saturationJSON,
  		&demandComputedAt, &analyticsComputedAt, &row.FetchedAt,
  	); err != nil {
  		return nil, err
  	}

  	if v, decErr := unmarshalPayload[demand.CharacterDemand](demandJSON); decErr != nil {
  		row.MalformedPayloads = append(row.MalformedPayloads, demand.MalformedPayload{Column: "demand", Err: decErr})
  	} else {
  		row.Demand = v
  	}
  	if v, decErr := unmarshalPayload[demand.CharacterVelocity](velocityJSON); decErr != nil {
  		row.MalformedPayloads = append(row.MalformedPayloads, demand.MalformedPayload{Column: "velocity", Err: decErr})
  	} else {
  		row.Velocity = v
  	}
  	if v, decErr := unmarshalPayload[demand.CharacterSaturation](saturationJSON); decErr != nil {
  		row.MalformedPayloads = append(row.MalformedPayloads, demand.MalformedPayload{Column: "saturation", Err: decErr})
  	} else {
  		row.Saturation = v
  	}

  	row.DemandComputedAt = nullTimeToPtr(demandComputedAt)
  	row.AnalyticsComputedAt = nullTimeToPtr(analyticsComputedAt)
  	return &row, nil
  }
  ```

  Run: `go build ./internal/adapters/storage/postgres/`. **Expect this to still
  fail, and not because of anything in this file.** This package imports
  `internal/domain/demand` (`dh_demand_repository.go:10`), and that package does
  not compile yet — `service.go` and `campaign_signals.go` still reference the
  `DemandJSON`/`VelocityJSON`/`SaturationJSON` fields Step 2 deleted, and they
  are not retyped until Steps 4 and 5. The only thing to confirm here is that
  **every reported error names `internal/domain/demand`**; any error naming
  `internal/adapters/storage/postgres` itself is a real defect in the code you
  just wrote and must be fixed before moving on. The postgres package is
  verified green at Step 7, after the domain compiles.

- [ ] **Step 4 — delete the four hand-mirrored blob structs and `parseCharacterDemand` from `service.go`; retype the readers.**

  In `internal/domain/demand/service.go`, delete `characterDemandJSON`,
  `byEraJSON`, `velocityBlobJSON`, `characterSaturationJSON` (the four
  structs at lines 253-298, under the `--- JSON parsing helpers ---` comment)
  and `parseCharacterDemand` (lines 300-311).

  At the `Leaderboard` call site (line 126), replace the parse call with a
  nil check, keeping the local name `demand` so downstream references in the
  same loop body are untouched:

  ```go
  		if row.Demand == nil {
  			continue
  		}
  		demand := row.Demand
  ```

  Re-type `erasForRow` and `eraDemandFor` (lines 359-372, 376-401) to take
  `*CharacterDemand`. `eraDemandFor`'s empty-string fallback (lines 390-393)
  collapses to `DataQuality: demand.DataQuality` since `ByEraDemand` has no
  `DataQuality` field — per decision 3 in the design doc, this is
  behaviour-preserving because the fallback fired unconditionally before
  (the per-era `DataQuality` was always `""`):

  ```go
  // erasForRow returns the set of eras to emit buckets for. If the caller
  // filtered by opts.Era we honour it; otherwise we emit every era present in
  // the row's ByEra map. If there is no ByEra map, we emit a single "" bucket
  // for the character overall.
  func erasForRow(demand *CharacterDemand, filter string) []string {
  	if filter != "" {
  		return []string{filter}
  	}
  	if len(demand.ByEra) == 0 {
  		return []string{""}
  	}
  	eras := make([]string, 0, len(demand.ByEra))
  	for era := range demand.ByEra {
  		eras = append(eras, era)
  	}
  	sort.Strings(eras)
  	return eras
  }

  // eraDemandFor returns the NicheDemand for a given era within a character's
  // demand payload. An empty era means "character overall".
  func eraDemandFor(demand *CharacterDemand, era string) (*NicheDemand, bool) {
  	if era == "" {
  		return &NicheDemand{
  			Score:        demand.AvgDemandScore,
  			Views:        demand.TotalViews,
  			WishlistAdds: demand.TotalWishlistAdds,
  			DataQuality:  demand.DataQuality,
  			ComputedAt:   parseTime(demand.ComputedAt),
  		}, true
  	}
  	entry, ok := demand.ByEra[era]
  	if !ok {
  		return nil, false
  	}
  	return &NicheDemand{
  		Score:        entry.AvgDemandScore,
  		Views:        entry.TotalViews,
  		WishlistAdds: entry.TotalWishlistAdds,
  		DataQuality:  demand.DataQuality,
  		ComputedAt:   parseTime(demand.ComputedAt),
  	}, true
  }
  ```

  Rewrite `parseCharacterMarket` (lines 313-353) to read the typed fields
  directly, and relocate the malformed-velocity warn here — it previously
  logged inline during `json.Unmarshal`; now the failure arrives pre-recorded
  on `row.MalformedPayloads`, so the warn loop filters for the velocity
  column and fires with the same message and fields as today:

  ```go
  // parseCharacterMarket extracts the market-axis view (velocity + saturation)
  // from a character cache row. Returns nil if both surfaces are absent.
  func (s *Service) parseCharacterMarket(ctx context.Context, row CharacterCache) *NicheMarket {
  	m := &NicheMarket{}
  	has := false

  	if row.Velocity != nil {
  		v := row.Velocity
  		m.MedianDaysToSell = v.MedianDaysToSell
  		m.VelocityChangePct = v.VelocityChangePct
  		m.SampleSize = v.SampleSize
  		m.AvgDailySales = v.AvgDailySales
  		m.SellThroughRate30d = v.SellThroughRate30d
  		m.SalesVolume7d = v.SalesVolume7d
  		m.SalesVolume30d = v.SalesVolume30d
  		m.SupplyCount = v.SupplyCount
  		has = true
  	}
  	if row.Saturation != nil {
  		m.ActiveListingCount = row.Saturation.ActiveListingCount
  		has = true
  	}
  	for _, mp := range row.MalformedPayloads {
  		if mp.Column != "velocity" {
  			continue
  		}
  		s.logger.Warn(ctx, "velocity_json unmarshal failed",
  			observability.String("character", row.Character),
  			observability.Err(mp.Err))
  	}
  	if !has {
  		// Row has no analytics — flag for callers.
  		if row.AnalyticsComputedAt == nil {
  			return &NicheMarket{AnalyticsNotComputed: true}
  		}
  		return nil
  	}
  	m.ComputedAt = row.AnalyticsComputedAt
  	return m
  }
  ```

  Delete `"encoding/json"` from this file's imports (lines 1-12) — the only
  uses were the three `json.Unmarshal` calls previously at lines 307, 321,
  339, all now removed.

  Run: `go build ./internal/domain/demand/`. Still expect a failure in
  `campaign_signals.go` — next step.

- [ ] **Step 5 — retype `buildSignalIndex` in `campaign_signals.go`.**

  Rewrite `buildSignalIndex` (lines 132-166) to read `row.Velocity` directly.
  `skipped` previously incremented on `json.Unmarshal` failure; it now
  increments from `row.MalformedPayloads` entries with `Column == "velocity"`,
  so `CampaignSignalsResponse.SkippedRows` keeps its meaning:

  ```go
  // buildSignalIndex constructs a map from lowercased character name to its
  // parsed signalEntry. Rows missing Velocity, AnalyticsComputedAt, or a nil
  // velocity_change_pct are silently skipped. The second return value is the
  // count of rows whose velocity payload was present but failed to decode (see
  // MalformedPayload) — a non-zero count indicates unexpected cache corruption
  // that the caller should surface for observability.
  func buildSignalIndex(rows []CharacterCache) (map[string]signalEntry, int) {
  	idx := make(map[string]signalEntry, len(rows))
  	skipped := 0
  	for _, row := range rows {
  		for _, mp := range row.MalformedPayloads {
  			if mp.Column == "velocity" {
  				skipped++
  			}
  		}
  		// Nil Velocity or AnalyticsComputedAt is expected for newly-ingested
  		// rows before the scheduler has run, or when the velocity payload
  		// failed to decode (counted above); skip silently either way. If all
  		// rows for a campaign are nil-guarded out, that campaign will be
  		// absent from the response entirely (not shown with
  		// TrackedCharacters=0).
  		if row.Velocity == nil || row.AnalyticsComputedAt == nil {
  			continue
  		}
  		if row.Velocity.VelocityChangePct == nil {
  			continue // no change metric — exclude from contributors
  		}
  		idx[strings.ToLower(row.Character)] = signalEntry{
  			displayName: row.Character,
  			vChange:     *row.Velocity.VelocityChangePct,
  			medianDays:  row.Velocity.MedianDaysToSell,
  			sampleSize:  row.Velocity.SampleSize,
  			computedAt:  row.AnalyticsComputedAt,
  		}
  	}
  	return idx, skipped
  }
  ```

  Delete `"encoding/json"` from this file's imports (lines 1-12) — the only
  use was the `json.Unmarshal` call previously at line 150.

  Run: `go build ./internal/domain/demand/`. This package now builds green.

- [ ] **Step 6 — fix Defect 1: map DH wire types field-by-field in the scheduler.**

  In `internal/adapters/scheduler/dh_analytics_refresh_steps.go`, replace the
  three `json.Marshal` blocks inside the character branch (the loop over
  `orderedCharacters`, lines 126-167 — the real marshal call sites are 133
  demand, 141 velocity, and **152 saturation, the nested one that is
  Defect 1**) with direct struct mapping:

  ```go
  	for _, name := range orderedCharacters {
  		row := demand.CharacterCache{
  			Character: name,
  			Window:    s.config.Window,
  			FetchedAt: now,
  		}
  		if entry, ok := demandByChar[name]; ok {
  			row.Demand = mapCharacterDemand(entry)
  		}
  		if entry, ok := velocityByChar[name]; ok {
  			row.Velocity = mapCharacterVelocity(entry.Velocity)
  			if t, tErr := parseDHTimestamp(entry.ComputedAt); tErr == nil {
  				row.AnalyticsComputedAt = &t
  			}
  		}
  		if entry, ok := saturationByChar[name]; ok {
  			row.Saturation = mapCharacterSaturation(entry)
  			if t, tErr := parseDHTimestamp(entry.ComputedAt); tErr == nil {
  				row.AnalyticsComputedAt = &t
  			}
  		}
  		if err := s.repo.UpsertCharacterCache(ctx, row); err != nil {
  			s.logger.Warn(ctx, "upsert character cache failed",
  				observability.String("character", name),
  				observability.Err(err))
  		}
  	}
  ```

  Add the three mapping helpers (place them in the `--- helpers ---` section
  alongside `indexDemand`/`indexVelocity`/`indexSaturation`):

  ```go
  // mapCharacterDemand converts a DH character-demand entry into the domain
  // payload persisted on CharacterCache.Demand. dh.CharacterDemandEntry has no
  // computed_at or data_quality field, so both are left "" on the mapped
  // struct — identical to what today's json.Marshal(entry) blob persists.
  func mapCharacterDemand(entry dh.CharacterDemandEntry) *demand.CharacterDemand {
  	out := &demand.CharacterDemand{
  		CharacterName:     entry.CharacterName,
  		CardCount:         entry.CardCount,
  		AvgDemandScore:    entry.AvgDemandScore,
  		TotalViews:        entry.TotalViews,
  		TotalSearchClicks: entry.TotalSearchClicks,
  		TotalWishlistAdds: entry.TotalWishlistAdds,
  	}
  	if len(entry.ByEra) > 0 {
  		out.ByEra = make(map[string]demand.ByEraDemand, len(entry.ByEra))
  		for era, e := range entry.ByEra {
  			out.ByEra[era] = demand.ByEraDemand{
  				CardCount:         e.CardCount,
  				AvgDemandScore:    e.AvgDemandScore,
  				TotalViews:        e.TotalViews,
  				TotalSearchClicks: e.TotalSearchClicks,
  				TotalWishlistAdds: e.TotalWishlistAdds,
  			}
  		}
  	}
  	return out
  }

  // mapCharacterVelocity converts DH's flat velocity block into the domain
  // payload. AvgDaysToSell and SellThrough are the two accepted losses (see
  // the SLA-41 design doc, "Accepted losses") — nothing in the domain reads
  // them, so they are not carried into demand.CharacterVelocity.
  func mapCharacterVelocity(v dh.CharacterVelocityFields) *demand.CharacterVelocity {
  	out := &demand.CharacterVelocity{
  		MedianDaysToSell:   v.MedianDaysToSell,
  		SampleSize:         v.SampleSize,
  		VelocityChangePct:  v.VelocityChangePct,
  		AvgDailySales:      v.AvgDailySales,
  		SellThroughRate30d: v.SellThroughRate30d,
  		SalesVolume7d:      v.SalesVolume7d,
  		SalesVolume30d:     v.SalesVolume30d,
  		SupplyCount:        v.SupplyCount,
  	}
  	if len(v.ByGrade) > 0 {
  		out.ByGrade = make(map[string]demand.VelocityTierStat, len(v.ByGrade))
  		for tier, stat := range v.ByGrade {
  			out.ByGrade[tier] = demand.VelocityTierStat{MedianDays: stat.MedianDays, SampleSize: stat.SampleSize}
  		}
  	}
  	if len(v.ByPriceTier) > 0 {
  		out.ByPriceTier = make(map[string]demand.VelocityTierStat, len(v.ByPriceTier))
  		for tier, stat := range v.ByPriceTier {
  			out.ByPriceTier[tier] = demand.VelocityTierStat{MedianDays: stat.MedianDays, SampleSize: stat.SampleSize}
  		}
  	}
  	return out
  }

  // mapCharacterSaturation converts DH's nested saturation entry into the flat
  // domain payload. This is where Defect 1 dies: reading
  // entry.Saturation.ActiveListingCount directly is an explicit assignment the
  // compiler checks, and there is no marshal depth left to get wrong.
  func mapCharacterSaturation(entry dh.CharacterSaturationEntry) *demand.CharacterSaturation {
  	return &demand.CharacterSaturation{
  		ActiveListingCount: entry.Saturation.ActiveListingCount,
  		ComputedAt:         entry.ComputedAt,
  	}
  }
  ```

  Two behaviour notes:

  - `dh.CharacterDemandEntry` has no `computed_at` and no `data_quality`, so
    the mapped `CharacterDemand` leaves both `""` — identical to today's
    persisted blob, so this is behaviour-preserving.
  - `AvgDaysToSell` and `SellThrough` are the two accepted losses named in
    the design doc's "Accepted losses" section: nothing reads them, and the
    cache self-heals on the next scheduler run.

  Check whether `"encoding/json"` is still used anywhere else in this file —
  Task 3 already stripped the card branch's five `json.Marshal` blocks, and
  this step removes the character branch's three, so the import is very
  likely dead now. If `grep -n 'json\.' internal/adapters/scheduler/dh_analytics_refresh_steps.go`
  returns nothing, delete the `"encoding/json"` import.

  Run: `go build ./internal/adapters/scheduler/`, then
  `go test -race ./internal/adapters/scheduler/ -run TestDHAnalyticsRefresh_HappyPath -v`.
  Confirm the Step 1 assertions now pass — this is the point where Defect 1
  is actually fixed, not merely typed around.

- [ ] **Step 7 — migrate `service_test.go` fixtures from JSON strings to struct literals.**

  In `internal/domain/demand/service_test.go`, replace `demandJSONWithEras`
  (lines 58-72) with a typed constructor:

  ```go
  // demandPayload constructs a CharacterDemand payload for a character with two
  // eras. baseScore is the character-level avg; per-era scores are baseScore±0.05.
  func demandPayload(character string, baseScore float64, quality string) *demand.CharacterDemand {
  	return &demand.CharacterDemand{
  		CharacterName:     character,
  		CardCount:         10,
  		AvgDemandScore:    baseScore,
  		TotalViews:        400,
  		TotalSearchClicks: 80,
  		TotalWishlistAdds: 20,
  		DataQuality:       quality,
  		ByEra: map[string]demand.ByEraDemand{
  			"sword_shield":   {CardCount: 6, AvgDemandScore: baseScore + 0.05, TotalViews: 240, TotalWishlistAdds: 12},
  			"scarlet_violet": {CardCount: 4, AvgDemandScore: baseScore - 0.05, TotalViews: 160, TotalWishlistAdds: 8},
  		},
  	}
  }
  ```

  Update all 16 `DemandJSON: strPtr(...)` / `VelocityJSON: strPtr(...)` call
  sites (lines 104, 105, 106, 140, 141, 168, 169, 188, 189, 218, 240, 267,
  268, 302, 303, 316) to `Demand: demandPayload(...)`. `velocityWithChange`
  (line 287, currently a JSON string literal) becomes a typed literal:

  ```go
  velocityWithChange := &demand.CharacterVelocity{MedianDaysToSell: f64Ptr(9.5), SampleSize: 120, VelocityChangePct: f64Ptr(14.2)}
  ```

  and its one use site (line 303, `VelocityJSON: strPtr(velocityWithChange)`)
  becomes `Velocity: velocityWithChange`.

  Add `func f64Ptr(f float64) *float64 { return &f }`.

  Delete `strPtr` (line 14), `floatStr` (line 54), and the `"strconv"` import
  (line 6) — verified: `strconv` in this file is used only by `floatStr`.

- [ ] **Step 8 — migrate `campaign_signals_test.go` fixtures the same way.**

  In `internal/domain/demand/campaign_signals_test.go`, replace `velocityJSON`
  (lines 17-24) and `velocityJSONNoChange` (lines 28-30) with typed
  constructors, and update `charRow` (lines 32-40) and `charRowNoChange`
  (lines 42-50) to use them:

  ```go
  // velocityPayload builds a CharacterVelocity payload matching the
  // CharacterVelocityFields stored format.
  func velocityPayload(medianDays, vChangePct float64, sample int) *demand.CharacterVelocity {
  	return &demand.CharacterVelocity{
  		MedianDaysToSell:  f64Ptr(medianDays),
  		SampleSize:        sample,
  		VelocityChangePct: f64Ptr(vChangePct),
  	}
  }

  // velocityPayloadNoChange omits VelocityChangePct — used to verify the
  // service excludes characters with no change metric from contributors.
  func velocityPayloadNoChange() *demand.CharacterVelocity {
  	return &demand.CharacterVelocity{MedianDaysToSell: f64Ptr(10), SampleSize: 5}
  }

  func charRow(name string, medianDays, vChangePct float64, sample int, computed time.Time) demand.CharacterCache {
  	return demand.CharacterCache{
  		Character:           name,
  		Window:              "30d",
  		Velocity:            velocityPayload(medianDays, vChangePct, sample),
  		AnalyticsComputedAt: &computed,
  	}
  }

  func charRowNoChange(name string, computed time.Time) demand.CharacterCache {
  	return demand.CharacterCache{
  		Character:           name,
  		Window:              "30d",
  		Velocity:            velocityPayloadNoChange(),
  		AnalyticsComputedAt: &computed,
  	}
  }
  ```

  `f64Ptr` is defined in `service_test.go` (Step 7) and both files are
  package `demand_test`, so no new helper is needed here.

  **Keep** the `"strconv"` import — verified still used at line 257
  (`"Char"+strconv.Itoa(i)`).

- [ ] **Step 9 — full verification and commit.**

  ```bash
  go test -race ./internal/domain/demand/ ./internal/adapters/scheduler/ ./internal/adapters/storage/postgres/
  go build ./...
  make check
  grep -rnE --include='*.go' 'JSON +\*string' internal/domain/demand/
  ```

  The `grep` must return nothing — all eight blob fields (five on `CardCache`
  from Task 3, three on `CharacterCache` from this task) are gone.

  Commit:

  ```
  refactor(demand): type the CharacterCache seam and fix Defect 1 (SLA-41)
  ```
### Task 5: DB-free unit test for `scanCharacterCacheRow`

**Files:**
- Modify: `internal/adapters/storage/postgres/dh_demand_repository_test.go` (created in Task 3; append one test function)
- Read only: `internal/testutil/mocks/row_scanner.go` (created in Task 3), `internal/adapters/storage/postgres/purchase_scan_helpers.go:69-71` (the `scanner` interface), `internal/adapters/storage/postgres/dh_demand_repository.go:258-282` (`scanCharacterCacheRow`, post-Task-4 shape)

**Interfaces:**
- Consumes: `mocks.RowScanner{Values []any; Err error}` with method `Scan(dest ...any) error` — **created in Task 3, not here.** If it is missing, Task 3 did not land; stop and say so rather than re-creating it.
- Consumes: `scanCharacterCacheRow(s scanner) (*demand.CharacterCache, error)` (unexported, same package as the test — `package postgres`).
- Produces: nothing new.

This task appends a second test function to the file Task 3 created. Both the
`package postgres` clause and all four imports it needs (`database/sql`, `testing`,
`time`, `.../domain/demand`, `.../testutil/mocks`) are already in that file from
`TestScanCardCacheRow`, so paste only the function below — do not re-add the header.

- [ ] Append `TestScanCharacterCacheRow` to `internal/adapters/storage/postgres/dh_demand_repository_test.go`. Column order and `sql.Null*` types below were read directly from `scanCharacterCacheRow` (`dh_demand_repository.go:258-274` pre-Task-4; Task 4 keeps the same eight-column scan list and only changes what happens to the three payload columns after `Scan` returns): `character, window, demand_json, velocity_json, saturation_json, demand_computed_at, analytics_computed_at, fetched_at` scanned into `string, string, sql.NullString, sql.NullString, sql.NullString, sql.NullTime, sql.NullTime, time.Time`.

```go
func TestScanCharacterCacheRow(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	demandComputedAt := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	analyticsComputedAt := time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC)

	validDemand := `{"character_name":"Pikachu","card_count":5,"avg_demand_score":0.8,` +
		`"total_views":100,"total_search_clicks":10,"total_wishlist_adds":3,` +
		`"data_quality":"full","computed_at":"2026-08-01T06:00:00Z"}`
	validVelocity := `{"sample_size":12}`
	validSaturation := `{"active_listing_count":42,"computed_at":"2026-08-01T07:00:00Z"}`

	tests := []struct {
		name              string
		values            []any
		wantDemandNil     bool
		wantVelocityNil   bool
		wantSaturationNil bool
		wantMalformed     []demand.MalformedPayload
	}{
		{
			name: "all three payload columns valid",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{String: validDemand, Valid: true},
				sql.NullString{String: validVelocity, Valid: true},
				sql.NullString{String: validSaturation, Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				fetchedAt,
			},
		},
		{
			name: "all three NULL",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{},
				sql.NullString{},
				sql.NullString{},
				sql.NullTime{},
				sql.NullTime{},
				fetchedAt,
			},
			wantDemandNil:     true,
			wantVelocityNil:   true,
			wantSaturationNil: true,
		},
		{
			name: "velocity column present but garbage",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{String: validDemand, Valid: true},
				sql.NullString{String: "{", Valid: true},
				sql.NullString{String: validSaturation, Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				fetchedAt,
			},
			wantVelocityNil: true,
			wantMalformed: []demand.MalformedPayload{
				{Column: "velocity"},
			},
		},
		{
			name: "demand and saturation both garbage",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{String: "{", Valid: true},
				sql.NullString{String: validVelocity, Valid: true},
				sql.NullString{String: "{", Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				fetchedAt,
			},
			wantDemandNil:     true,
			wantSaturationNil: true,
			wantMalformed: []demand.MalformedPayload{
				{Column: "demand"},
				{Column: "saturation"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row, err := scanCharacterCacheRow(&mocks.RowScanner{Values: tc.values})
			if err != nil {
				t.Fatalf("scanCharacterCacheRow: unexpected error: %v", err)
			}

			if (row.Demand == nil) != tc.wantDemandNil {
				t.Errorf("Demand nil = %v, want %v", row.Demand == nil, tc.wantDemandNil)
			}
			if (row.Velocity == nil) != tc.wantVelocityNil {
				t.Errorf("Velocity nil = %v, want %v", row.Velocity == nil, tc.wantVelocityNil)
			}
			if (row.Saturation == nil) != tc.wantSaturationNil {
				t.Errorf("Saturation nil = %v, want %v", row.Saturation == nil, tc.wantSaturationNil)
			}

			if len(row.MalformedPayloads) != len(tc.wantMalformed) {
				t.Fatalf("MalformedPayloads = %d entries, want %d: %+v", len(row.MalformedPayloads), len(tc.wantMalformed), row.MalformedPayloads)
			}
			for i, want := range tc.wantMalformed {
				got := row.MalformedPayloads[i]
				if got.Column != want.Column {
					t.Errorf("MalformedPayloads[%d].Column = %q, want %q", i, got.Column, want.Column)
				}
				if got.Err == nil {
					t.Errorf("MalformedPayloads[%d].Err = nil, want non-nil", i)
				}
			}
		})
	}
}
```

Verified gofmt-clean: the function above was checked in a temp file carrying the same `package postgres` clause and imports the target file already has, and `gofmt -l` produced no output (byte-identical after `gofmt`).

- [ ] Run `go test -race ./internal/adapters/storage/postgres/ -run 'TestScanCardCacheRow|TestScanCharacterCacheRow' -v` and confirm **both** pass — Task 3's card test must still be green after this append, since both functions now live in the same file. On the character test specifically: there is no red step, because Task 4 already implements the behavior under test. If it fails, that means Task 4's implementation does not match the spec's column order or malformed-payload contract, and this task stops to flag that mismatch rather than silently adjusting the test to match.
- [ ] Commit: `test(postgres): cover scanCharacterCacheRow's typed payload decode and malformed capture (SLA-41)`

---

### Task 6: `CapturingLogger` + the malformed-velocity warn test

**Files:**
- Modify: `internal/testutil/mocks/logger.go` (currently 28 lines, read in full)
- Modify: `internal/domain/demand/service_test.go` (append one test function)
- Modify: `internal/domain/demand/campaign_signals_test.go` (add the `"errors"` import, append one test function)
- Modify: `internal/domain/demand/campaign_signals.go:104-106,132-137` (stale doc comments naming `VelocityJSON`)
- Read only: `internal/domain/observability/logger.go:1-56` (the real `Logger` interface and `Field` type)

**Interfaces:**
- Consumes: `observability.Logger` (`internal/domain/observability/logger.go:9-15`) — `Debug/Info/Warn/Error(ctx, msg, ...Field)`, `With(ctx, ...Field) Logger`. `Field` (`logger.go:18-21`) is a plain `struct{ Key string; Value any }` with no accessor methods — confirmed by reading the file; there is no `Field.String()`/`Field.Get()` to reuse, so `LogEntry.FindField` implements the key lookup directly against `Field.Key`/`Field.Value`.
- Produces: `mocks.LogEntry{Level, Message string; Fields []observability.Field}` with `FindField(key string) (any, bool)`; `mocks.CapturingLogger` satisfying `observability.Logger`, `NewCapturingLogger() *CapturingLogger`, `(*CapturingLogger) Entries() []LogEntry`.
- Consumes: `demand.NewService(repo, campaigns).WithLogger(l)` (`internal/domain/demand/service.go:49,55`) and `demand.CharacterCache{..., MalformedPayloads []demand.MalformedPayload}` (Task 4's shape).

`internal/testutil/mocks/logger.go` today (read in full, 28 lines) defines only `MockLogger`, which discards every call and therefore cannot back a "did we log this" assertion. `CapturingLogger` is added to the same file rather than a new one, since both are small `Logger` doubles and the file stays well under the 500-line warning threshold.

- [ ] Rewrite `internal/testutil/mocks/logger.go` in full:

```go
package mocks

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// MockLogger is a test double for observability.Logger.
// It silently discards all log messages, like the production NoopLogger,
// but lives in the test mocks package so tests don't depend on production types.
type MockLogger struct{}

var _ observability.Logger = (*MockLogger)(nil)

// NewMockLogger creates a new test logger that discards all output.
func NewMockLogger() observability.Logger {
	return &MockLogger{}
}

func (m *MockLogger) Debug(_ context.Context, _ string, _ ...observability.Field) {}
func (m *MockLogger) Info(_ context.Context, _ string, _ ...observability.Field)  {}
func (m *MockLogger) Warn(_ context.Context, _ string, _ ...observability.Field)  {}
func (m *MockLogger) Error(_ context.Context, _ string, _ ...observability.Field) {}
func (m *MockLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return m
}

// LogEntry is one captured log call.
type LogEntry struct {
	Level   string
	Message string
	Fields  []observability.Field
}

// FindField returns the value of the first field in the entry whose Key
// matches, and whether one was found.
func (e LogEntry) FindField(key string) (any, bool) {
	for _, f := range e.Fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

// CapturingLogger is a test double for observability.Logger that records
// every call instead of discarding it, so tests can assert on log output.
// It is safe for concurrent use.
type CapturingLogger struct {
	mu      sync.Mutex
	entries []LogEntry
}

var _ observability.Logger = (*CapturingLogger)(nil)

// NewCapturingLogger creates a CapturingLogger with no recorded entries.
func NewCapturingLogger() *CapturingLogger {
	return &CapturingLogger{}
}

func (c *CapturingLogger) record(level, msg string, fields []observability.Field) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, LogEntry{Level: level, Message: msg, Fields: fields})
}

func (c *CapturingLogger) Debug(_ context.Context, msg string, fields ...observability.Field) {
	c.record("debug", msg, fields)
}

func (c *CapturingLogger) Info(_ context.Context, msg string, fields ...observability.Field) {
	c.record("info", msg, fields)
}

func (c *CapturingLogger) Warn(_ context.Context, msg string, fields ...observability.Field) {
	c.record("warn", msg, fields)
}

func (c *CapturingLogger) Error(_ context.Context, msg string, fields ...observability.Field) {
	c.record("error", msg, fields)
}

// With returns the same logger so that fields captured via chained loggers
// still land in the one Entries() slice callers inspect. Production Loggers
// return a child logger that prepends fields to every subsequent call; a
// test double has no such need since assertions read fields directly off
// each LogEntry.
func (c *CapturingLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return c
}

// Entries returns a copy of every log call recorded so far.
func (c *CapturingLogger) Entries() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}
```

Verified gofmt-clean (written to `/tmp/logger.go`, `gofmt -l` produced no output, byte-identical after `gofmt`).

- [ ] Run `go build ./internal/testutil/mocks/` and confirm it compiles (this is additive to an existing file; there is no red step since `MockLogger` is untouched and `CapturingLogger` has no prior callers to break).

- [ ] Append to `internal/domain/demand/service_test.go` (same package `demand_test`, reusing the existing `newRepoWithRows` and `uncoveredLookup` helpers already defined earlier in that file):

```go
// TestService_Leaderboard_MalformedVelocity_LogsWarn asserts that a
// character row carrying a MalformedPayloads entry for "velocity" produces
// exactly one Warn log naming the character, and that a malformed "demand"
// entry produces none — the warn is velocity-specific per decision in the
// SLA-41 spec's "Error handling" section.
func TestService_Leaderboard_MalformedVelocity_LogsWarn(t *testing.T) {
	rows := []demand.CharacterCache{
		{
			Character: "Pikachu",
			Window:    "7d",
			Demand: &demand.CharacterDemand{
				CharacterName:  "Pikachu",
				CardCount:      5,
				AvgDemandScore: 0.8,
			},
			MalformedPayloads: []demand.MalformedPayload{
				{Column: "velocity", Err: errors.New("unexpected end of JSON input")},
			},
		},
		{
			Character: "Charizard",
			Window:    "7d",
			Demand: &demand.CharacterDemand{
				CharacterName:  "Charizard",
				CardCount:      3,
				AvgDemandScore: 0.6,
			},
			MalformedPayloads: []demand.MalformedPayload{
				{Column: "demand", Err: errors.New("some demand decode error")},
			},
		},
	}

	logger := mocks.NewCapturingLogger()
	svc := demand.NewService(newRepoWithRows(rows), uncoveredLookup()).WithLogger(logger)

	if _, err := svc.Leaderboard(context.Background(), demand.LeaderboardOpts{Window: "7d"}); err != nil {
		t.Fatalf("Leaderboard: unexpected error: %v", err)
	}

	var warns []mocks.LogEntry
	for _, e := range logger.Entries() {
		if e.Level == "warn" {
			warns = append(warns, e)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("got %d warn entries, want 1: %+v", len(warns), warns)
	}

	entry := warns[0]
	if entry.Message != "velocity_json unmarshal failed" {
		t.Errorf("Message = %q, want %q", entry.Message, "velocity_json unmarshal failed")
	}
	character, ok := entry.FindField("character")
	if !ok {
		t.Fatal("warn entry has no \"character\" field")
	}
	if character != "Pikachu" {
		t.Errorf("character field = %v, want %q", character, "Pikachu")
	}
}
```

Note: the "Charizard" row's `MalformedPayloads: [{Column: "demand"}]` alongside a non-nil `Demand` is a deliberately synthetic combination — a real adapter row with a failed demand decode would have `Demand == nil` and would never reach `parseCharacterMarket` (the demand-nil row is skipped before the market/warn logic runs at all, per `service.go:126-129`). Constructing it this way isolates exactly what this test needs to prove: the warn loop filters on `Column == "velocity"` and does not fire for other columns, independent of whether the row is otherwise well-formed. A row that is skipped for having no `Demand` would prove nothing, since `parseCharacterMarket` would never be called.

Verified gofmt-clean (written to `/tmp/service_malformed_velocity_test.go` as a full file with matching imports, `gofmt -l` produced no output, byte-identical after `gofmt`).

- [ ] Run `go test -race ./internal/domain/demand/ -run TestService_Leaderboard_MalformedVelocity_LogsWarn -v` and confirm it fails before Task 4's warn-relocation is wired to `MalformedPayloads` (or passes immediately if Task 4 already wired it — this fragment assumes Tasks 1-4 are committed, so expect green; if red, Task 4's `parseCharacterMarket` does not match the spec's `for _, mp := range row.MalformedPayloads` loop and that mismatch must be resolved before continuing).
- [ ] **Step: assert the malformed velocity row still reaches `SkippedRows`.**

  The spec's malformed-payload requirement has two halves — the warn log (above)
  and a non-zero `SkippedRows` from `CampaignSignals`. Task 4 rewired
  `buildSignalIndex` to count `MalformedPayloads` entries with
  `Column == "velocity"`; this asserts that count actually surfaces. There is no
  existing `SkippedRows` test — `grep -n 'Skipped' internal/domain/demand/campaign_signals_test.go`
  returns nothing today.

  Add `"errors"` to the import block of
  `internal/domain/demand/campaign_signals_test.go` (it currently imports
  `context`, `math`, `strconv`, `testing`, `time` plus the three project
  packages), then append:

  ```go
  func TestCampaignSignals_MalformedVelocityCountsAsSkipped(t *testing.T) {
  	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)

  	rows := []demand.CharacterCache{
  		charRow("Pikachu", 11, 22.1, 34, computed),
  		{
  			Character:           "Charizard",
  			Window:              "30d",
  			AnalyticsComputedAt: &computed,
  			MalformedPayloads: []demand.MalformedPayload{
  				{Column: "velocity", Err: errors.New("unexpected end of JSON input")},
  			},
  		},
  		{
  			Character:           "Umbreon",
  			Window:              "30d",
  			AnalyticsComputedAt: &computed,
  			MalformedPayloads: []demand.MalformedPayload{
  				{Column: "demand", Err: errors.New("unexpected end of JSON input")},
  			},
  		},
  	}

  	campaigns := []demand.ActiveCampaign{{
  		ID:                "c1",
  		Name:              "Vintage Core",
  		SubjectFilterMode: inventory.SubjectFilterTarget,
  		Subjects:          []inventory.TargetSubject{{ID: 1, Name: "Pikachu"}},
  	}}

  	svc := demand.NewService(newRepoWithRows(rows), campaignLookupWith(campaigns))

  	resp, err := svc.CampaignSignals(context.Background())
  	if err != nil {
  		t.Fatalf("CampaignSignals: %v", err)
  	}
  	if resp.SkippedRows != 1 {
  		t.Errorf("SkippedRows = %d, want 1 (only the velocity-column failure counts)", resp.SkippedRows)
  	}
  	if len(resp.Signals) != 1 {
  		t.Fatalf("len(Signals) = %d, want 1", len(resp.Signals))
  	}
  }
  ```

  The Umbreon row is the negative control: a `demand`-column failure must not
  inflate `SkippedRows`, which is documented as a velocity-parse counter.
  `newRepoWithRows` (`service_test.go:18-24`) and `campaignLookupWith`
  (`campaign_signals_test.go:56-62`) are existing helpers in the same
  `demand_test` package.

  Verified gofmt-clean (written to `/tmp/sla41_skipped_test.go` with matching
  imports; `gofmt -l` produced no output).

- [ ] Run `go test -race ./internal/domain/demand/ -run TestCampaignSignals_MalformedVelocityCountsAsSkipped -v` and confirm PASS.

- [ ] **Step: correct the stale `buildSignalIndex` doc comment.**

  `internal/domain/demand/campaign_signals.go:132-137` still describes the old
  contract — "Rows missing VelocityJSON, AnalyticsComputedAt, invalid JSON…" and
  "the count of rows that had a non-nil VelocityJSON but failed to parse". After
  Task 4 there is no `VelocityJSON` field. Rewrite it to name `Velocity` and
  `MalformedPayloads`:

  ```go
  // buildSignalIndex constructs a map from lowercased character name to its
  // parsed signalEntry. Rows with a nil Velocity, a nil AnalyticsComputedAt, or
  // a nil velocity_change_pct are silently skipped. The second return value is
  // the count of rows whose velocity payload was present in the database but
  // failed to decode (reported by the adapter via MalformedPayloads) — a
  // non-zero count indicates cache corruption the caller should surface for
  // observability.
  ```

  Also check the comment at `campaign_signals.go:104-106` ("Only rows with a
  valid VelocityJSON…") and update the same way.

- [ ] Run `go test -race ./internal/domain/demand/... ./internal/adapters/storage/postgres/... ./internal/testutil/mocks/...` and confirm all green (no regressions from the `logger.go` rewrite).
- [ ] Commit: `test(demand): assert velocity-only malformed-payload warn logging and skipped-row count (SLA-41)`

---

### Task 7: Follow-up ticket + full verification gate

**Files:** none created or modified — this task is process (ticket filing) and verification only.

**Interfaces:** none — no code changes.

- [ ] File a Linear ticket in team **SLA** titled something like "Drop the five `dh_card_cache` blob columns (`demand_json`, `velocity_json`, `trend_json`, `saturation_json`, `price_distribution_json`)". Body must state the precondition explicitly: rolling back an app image does **not** run Down migrations (`docs/OPERATIONS.md:87`: "Rolling back the app image does NOT undo a migration... the rolled-back app may crash because its code doesn't match the schema"), and this repository auto-deploys on every push to `main`, so a `deploy → migrate → roll back` sequence is the ordinary recovery path, not a corner case. The drop migration (`000032_drop_card_cache_blobs`, per the spec's "Column removal is a separate release" section) must not ship in the same release as SLA-41's code change, and must wait until the SLA-41 release is confirmed live and the team is no longer willing to roll back past it. **This ticket must exist before SLA-41 merges** — the spec's "Open follow-ups" section states the columns become permanent dead weight otherwise. Record the ticket ID in the PR description.

- [ ] Run the full verification gate, one command per step:

  - [ ] `go test -race ./...` — expect all packages pass, no data races reported.
  - [ ] `make check` — expect lint, the hexagonal import check, the flat-sibling check, the file-size check, and the Playwright version check all pass with no violations.
  - [ ] `go build ./...` — expect a clean build with no errors.
  - [ ] `grep -rnE --include='*.go' 'JSON +\*string' internal/domain/demand/` — expect no output. This is the acceptance grep from the spec's "Verification" section; a non-empty result means a blob field survived the Task 1-4 rewrite.
  - [ ] `grep -rn 'ListCardCacheByDemandScore' internal/ cmd/` — expect no output. Confirms Task 2 fully removed the dead method from the interface, the postgres implementation, and the mock.
  - [ ] `ls internal/adapters/storage/postgres/migrations/` — expect no file numbered `000032`. This change must ship no migration; the drop is the follow-up ticket filed above, not this PR.

- [ ] Run `my:polish-core --fix` and inspect every edit it makes. Then **re-run the full gate above, not just the tests** — `make check` covers `go vet`, `golangci-lint`, the hexagonal import check, the flat-sibling check, and the file-size check (`Makefile:139,146`), and a polish edit can break any of them without touching a single test. Specifically re-run, in this order:
  - [ ] `go test -race ./...` — expect all packages pass.
  - [ ] `go build ./...` — expect a clean build.
  - [ ] `make check` — expect all five checks pass. This is the one that catches a polish edit pushing a file past the 500-line warn / 600-line fail threshold, or introducing an import the architecture check rejects.

  If polish makes no edits at all, say so and skip the re-run rather than reporting a gate you did not need to run.

- [ ] `docs/` sweep: `grep -rn 'demand_json\|velocity_json\|saturation_json\|trend_json\|price_distribution_json' docs/` and read every hit in `docs/SCHEMA.md` and `docs/ARCHITECTURE.md` for language that describes these columns as opaque blobs parsed ad hoc, versus describing them as backed by named domain structs. Update any line whose wording no longer matches the typed seam (e.g. "JSON blob" phrasing where the reader is now `CharacterDemand`/`CharacterVelocity`/`CharacterSaturation`). If the grep or the read finds nothing that describes parsing behavior at that level of detail (e.g. the docs only name the column and its purpose, not how it's decoded), say so explicitly in the PR description rather than editing speculatively — do not invent a doc change to have something to report.

- [ ] Commit the ticket reference and any doc updates, then open the PR. PR description must include: the Linear ticket ID for the column-drop follow-up, the exact verification commands run and their results, and a link to the design doc (`docs/superpowers/specs/2026-08-08-demand-repository-typed-seam-design.md`).
