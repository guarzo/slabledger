# Typed Demand Repository Seam (SLA-41)

**Date:** 2026-08-08
**Status:** Design approved, pending implementation plan

## Problem

`demand.Repository` moves DH analytics across the domain/adapter boundary as
opaque JSON strings. `internal/domain/demand/types.go` carries eight such fields
— five on `CardCache` (lines 20-24) and three on `CharacterCache` (lines 34-36):

```
DemandJSON, VelocityJSON, TrendJSON, SaturationJSON, PriceDistributionJSON  // CardCache
DemandJSON, VelocityJSON, SaturationJSON                                    // CharacterCache
```

The package doc claims DH payloads "are parsed here into domain-local structs so
the scoring logic stays decoupled from the wire format." That is only half true.
The structs exist, but they are unexported and hand-mirrored inside
`service.go` (`characterDemandJSON`, `byEraJSON`, `velocityBlobJSON`,
`characterSaturationJSON`) and are re-derived by eye from
`internal/adapters/clients/dh/types_analytics.go`. The compiler cannot see the
relationship, so the two sides drift silently.

They have drifted. Two live defects, both of exactly the class an untyped seam
produces:

**Defect 1 — `active_listing_count` has always been zero.** The scheduler is
inconsistent about how deep it marshals. For velocity it writes the inner block:

```go
// dh_analytics_refresh_steps.go:138
if blob, encErr := json.Marshal(entry.Velocity); encErr == nil {   // flat — correct
```

For saturation it writes the whole entry:

```go
// dh_analytics_refresh_steps.go:151
if blob, encErr := json.Marshal(entry); encErr == nil {            // nested — wrong
```

`dh.CharacterSaturationEntry` nests its payload under `"saturation"`
(`types_analytics.go:255-260`), but the domain reads `active_listing_count` at
the top level (`service.go:296`). The key is never found, `json.Unmarshal`
reports no error, and `NicheMarket.ActiveListingCount` silently takes Go's zero
value. Every character niche has reported 0 active listings in production since
the field was introduced. `computed_at` is unaffected — it sits at the entry top
level, so that one key does resolve.

Both sides are unit-tested and both pass. `dh_analytics_refresh_test.go:173`
asserts the scheduler writes `ActiveListingCount: 42`;
`niches_handler_test.go:43` asserts the handler serves 42 from a mocked service.
Nothing tests the seam between them, which is where the value is lost.

**Defect 2 — `min_data_quality=full` returns nothing for characters.** The
domain reads `data_quality` and `computed_at` from the character demand blob
(`service.go`), but `dh.CharacterDemandEntry` (`types_analytics.go:176-184`)
defines neither. Every character row therefore carries `DataQuality: ""`, and
`qualityAllowed` (`service.go:404-413`) requires an exact `"full"` match, so
`GET /api/niches?min_data_quality=full` filters out every character niche.

Defect 2 is **out of scope here** and filed as **SLA-61**. DH exposes no
character-level data quality — only per-card `dh.DemandSignal.DataQuality`
(`types_analytics.go:154`) — so the fix is a design question (derive an
aggregate? drop the filter?), not wiring. Typing the seam does not answer it; it
makes the gap visible, which is the point.

## Scope

In scope:

- Replace the eight opaque blob fields with exported domain structs.
- Delete the four hand-mirrored unexported structs in `service.go`.
- Fix Defect 1.
- Delete `ListCardCacheByDemandScore` and the five `CardCache` blob columns.

Out of scope: Defect 2 (SLA-61), any change to DH client types, any change to
the niches leaderboard's scoring or ranking.

## Decisions

**1. Card blobs are deleted, not typed.** All five `CardCache` blob fields are
write-only: the scheduler populates them
(`dh_analytics_refresh_steps.go:243-269`) and nothing ever reads them back.
`ListCardCacheByDemandScore`, the only read path that would surface them, has
zero production callers — the symbol appears only in the interface
(`repository.go:16`), the Postgres implementation
(`dh_demand_repository.go:88`), and the mock (`demand_repository.go:14`). Typing
five structs nobody reads would be work in the wrong direction. They go, along
with the method and the columns.

`CardCache` keeps `CardID`, `Window`, `DemandScore`, `DemandDataQuality`,
`AnalyticsComputedAt`, `DemandComputedAt`, and `FetchedAt` — the fields
`GetCardCache` and `CardDataQualityStats` actually serve.

**2. Character structs mirror the consumed fields plus the by-grade
breakdowns.** Not a full mirror of the DH wire types: the domain should express
what the domain needs, and a byte-for-byte copy would recreate the coupling the
ticket exists to remove. `ByGrade` and `ByPriceTier` are carried despite having
no reader today because they are the natural next thing the leaderboard wants
and they cost two map fields.

See "Accepted losses" below — this decision has a write-side consequence, now
resolved in favour of dropping the two unread fields.

**3. `DataQuality` and `ComputedAt` are preserved exactly as they behave
today, at the character level only.** `CharacterDemand` keeps both fields with
their current JSON tags even though no DH writer populates `data_quality`.
Changing that behaviour is SLA-61. The field carries a doc comment citing
SLA-61 so the next reader does not rediscover it.

`ByEraDemand` does **not** get a `DataQuality` field. `dh.ByEraEntry`
(`internal/adapters/clients/dh/types_analytics.go:186-193`) has no
`data_quality` key, so the per-era value can never be populated from DH's wire
format — unlike the character-level field, which at least has a plausible
future source. Today's reader already handles this: `eraDemandFor`
(`internal/domain/demand/service.go:386-393`) reads `entry.DataQuality` and
falls back to `demand.DataQuality` when it is empty, and since it is *always*
empty the fallback always fires. Assigning `demand.DataQuality` directly is
therefore behaviour-preserving in every reachable state — both today (both
values `""`) and after SLA-61 gives the character-level field a real value,
which the fallback would have inherited anyway. This keeps "can character
demand carry a data quality at all?" a single question owned by SLA-61 rather
than splitting it across two fields, one of which is structurally dead.

**4. Defect 1 is fixed as part of this change.** The scheduler will map DH types
into the new domain structs field by field, which makes the nesting bug
unrepresentable rather than merely fixed.

## Changes

### 1. New file `internal/domain/demand/payloads.go`

Exported structs, one per persisted payload. JSON tags match today's DH keys
exactly so rows already in the database decode unchanged.

```go
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

// ByEraDemand is the per-era breakdown inside CharacterDemand. It carries no
// DataQuality: DH's by_era entries have no data_quality key, so era buckets
// inherit the character-level value. See decision 3.
type ByEraDemand struct {
	CardCount         int     `json:"card_count"`
	AvgDemandScore    float64 `json:"avg_demand_score"`
	TotalViews        int     `json:"total_views"`
	TotalSearchClicks int     `json:"total_search_clicks"`
	TotalWishlistAdds int     `json:"total_wishlist_adds"`
}

// CharacterVelocity is the velocity payload persisted on CharacterCache.
// Pointer fields preserve DH's null-vs-zero distinction.
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

// VelocityTierStat is the per-grade / per-price-tier velocity slice.
type VelocityTierStat struct {
	MedianDays float64 `json:"median_days"`
	SampleSize int     `json:"sample_size"`
}

// CharacterSaturation is the saturation payload persisted on CharacterCache.
type CharacterSaturation struct {
	ActiveListingCount int    `json:"active_listing_count"`
	ComputedAt         string `json:"computed_at"`
}
```

### 2. `internal/domain/demand/types.go`

`CharacterCache` swaps its three `*string` fields for typed pointers:

```go
type CharacterCache struct {
	Character           string
	Window              string
	Demand              *CharacterDemand
	Velocity            *CharacterVelocity
	Saturation          *CharacterSaturation
	DemandComputedAt    *time.Time
	AnalyticsComputedAt *time.Time
	FetchedAt           time.Time
	// MalformedPayloads names the payload columns that were present but
	// failed to decode ("demand", "velocity", "saturation"). See
	// "Error handling".
	MalformedPayloads []string
}
```

`CardCache` drops all five blob fields. The package doc is corrected: the
domain-local structs are now real and exported, and the decoupling claim becomes
true.

### 3. `internal/domain/demand/repository.go`

Delete `ListCardCacheByDemandScore` from the interface.

### 4. Migration `000032_drop_card_cache_blobs`

Up — drop five columns from `dh_card_cache` (defined at
`000001_initial_schema.up.sql:639`):

```sql
ALTER TABLE dh_card_cache
    DROP COLUMN demand_json,
    DROP COLUMN velocity_json,
    DROP COLUMN trend_json,
    DROP COLUMN saturation_json,
    DROP COLUMN price_distribution_json;

DROP INDEX IF EXISTS idx_card_cache_demand_score;
```

The index goes too. It exists only to serve `ORDER BY demand_score DESC`, which
appears in exactly one query — `ListCardCacheByDemandScore`
(`dh_demand_repository.go:96`) — and that method is being deleted. The
`demand_score` column itself stays; `GetCardCache` and `CardDataQualityStats`
still read it, neither with an ordering.

Down: re-add the five columns as nullable `TEXT` and recreate the index. Column
data is unrecoverable on rollback, which is acceptable — nothing reads it, and
the scheduler repopulates the rest of the row on its next run.

**`dh_character_cache` needs no migration.** Its `demand_json`,
`velocity_json`, and `saturation_json` columns keep their names, types, and
contents. Only the Go-side representation changes, and the JSON tags are chosen
so persisted rows decode as-is.

### 5. `internal/adapters/storage/postgres/dh_demand_repository.go`

`scanCharacterCacheRow` unmarshals each non-null column into its struct rather
than handing the raw string up. `UpsertCharacterCache` marshals in the other
direction. `scanCardCacheRow` and the card upsert lose their blob handling, and
`ListCardCacheByDemandScore` is deleted.

### 6. `internal/adapters/scheduler/dh_analytics_refresh_steps.go`

The character branch maps `dh` types into the new domain structs field by field
instead of calling `json.Marshal` on whichever value was nearest. This is where
Defect 1 dies: `entry.Saturation.ActiveListingCount` is now an explicit
assignment the compiler checks, and there is no marshal depth left to get wrong.
The card branch drops its five blob assignments.

### 7. `internal/domain/demand/service.go` and `campaign_signals.go`

Delete `characterDemandJSON`, `byEraJSON`, `velocityBlobJSON`, and
`characterSaturationJSON`, and the three `json.Unmarshal` calls at `service.go`
lines 307, 321, 339 plus the one at `campaign_signals.go:150`. Readers use the
typed fields directly. `encoding/json` should drop out of both files' imports.

`eraDemandFor` (`service.go:386-393`) loses its empty-string fallback along with
`ByEraDemand.DataQuality`: the three lines computing `quality` collapse to
`DataQuality: demand.DataQuality`. Per decision 3 this is behaviour-preserving,
not a behaviour change.

## Accepted losses

Decision 2 has a consequence that only bites on the **write** side, recorded
here because it is invisible from the read side.

Today the scheduler marshals the entire `dh.CharacterVelocityFields`, so the
persisted column contains all twelve fields — including `avg_days_to_sell` and
`sell_through`, which no domain code reads. Under this design the adapter
marshals the *domain* struct, so those two fields stop being written at all.
That is not merely a read-side omission; it removes them from the column.

**Decision: accept the loss. Do not add the two fields.** Three things make it
low-risk:

- **Nothing reads them.** The only mention of
  `CharacterVelocityFields.AvgDaysToSell` / `.SellThrough` in the repository is
  their declaration at `internal/adapters/clients/dh/types_analytics.go:235-236`.
  The apparent consumers at `internal/adapters/clients/dh/convert.go:111-113`
  are a *different* struct — `CardAnalytics.Velocity.SellThrough`, a
  `map[string]string` at `types_analytics.go:34`. Card velocity, not character
  velocity, and untouched by this change.
- **The blob is a cache, not a system of record.**
  `DH_ANALYTICS_REFRESH_ENABLED` defaults to `true`
  (`internal/platform/config/loader.go:196`) with a configurable hour, so the
  payload is rewritten daily from DH. Restoring a field means adding it and
  waiting one refresh cycle — there is no backfill and no data migration,
  because DH is authoritative and always re-queryable.
- **Repository convention.** CLAUDE.md's "avoid speculative abstractions" and
  "only make changes directly requested" point directly at persisting fields on
  the theory that someone might later want them.

The counter-argument — that the blob should stay a faithful DH snapshot for
debugging — does not hold up: the snapshot is already lossy (`dh_character_cache`
stores three payloads, not the response), and the honest tool for inspecting raw
DH output is a client call, not a partially-faithful cache column.

Reversibility decides it. If this is wrong, the fix is two struct fields and a
24-hour wait. If the opposite is wrong, two unread fields sit in the database
indefinitely and every future reader must work out whether they are trustworthy.

## Error handling

Today a malformed velocity blob is counted and surfaced:
`buildSignalIndex` increments `skipped`, which becomes
`CampaignSignalsResponse.SkippedRows`, which the caller logs. Moving decoding
into the adapter would destroy that signal — the adapter has no logger
(`DHDemandRepository` holds only `db *sql.DB`) and the domain would lose the
ability to tell "column was null" from "column was corrupt", since both arrive
as a nil pointer.

Rather than thread a logger into the repository, the adapter records the failure
on the row: a column that is present but fails to decode yields a nil field
**and** an entry in `MalformedPayloads`. The domain keeps the absent-vs-corrupt
distinction, `buildSignalIndex` keeps counting, `SkippedRows` keeps its meaning,
and the existing warn log keeps firing. One malformed column does not fail the
scan or drop the row — the other payloads on that row still decode.

Old rows written under the nested saturation shape decode to
`ActiveListingCount: 0` — exactly today's behaviour, not a regression — and
self-heal on the next scheduler run.

## Testing

- **Round-trip tests** for each new struct: marshal → unmarshal → equal, plus
  decoding a literal captured from the DH wire format.
- **Seam test** — the one that would have caught Defect 1: feed
  `dh.CharacterSaturationEntry{Saturation: dh.CharacterSaturationFields{ActiveListingCount: 42}}`
  through the scheduler mapping into `CharacterCache`, and assert the domain
  reads 42. This test must fail against the current `json.Marshal(entry)` line.
- **Malformed-payload test**: a row with corrupt velocity JSON yields a nil
  `Velocity`, a `MalformedPayloads` slice containing `"velocity"`, and a
  non-zero `SkippedRows` from `CampaignSignals`.
- **Backward-compatibility test**: a persisted blob in today's on-disk shape
  decodes into the new struct with the same values the old unexported struct
  produced.
- Update `internal/testutil/mocks/demand_repository.go` to drop
  `ListCardCacheByDemandScoreFn`.
- Existing `service_test.go` and `campaign_signals_test.go` fixtures move from
  JSON string literals to struct literals.

## Verification

1. `go test -race ./...`
2. `make check` — lint, import check (the hexagonal invariant is preserved:
   `payloads.go` is pure domain and imports nothing from `adapters`), file-size
   check, Playwright version check.
3. `go build ./...`
4. Confirm the acceptance grep is clean:
   `grep -rnE --include='*.go' 'JSON +\*string' internal/domain/demand/`
   returns nothing (it returns eight lines today).
5. Confirm no stale references:
   `grep -rn 'ListCardCacheByDemandScore' internal/ cmd/` returns nothing.
   (Scope it to code — this spec mentions the symbol by name, so a repo-root
   grep will always match.)

**One documented exception to "no behaviour change":**
`NicheMarket.ActiveListingCount` changes from a constant 0 to real DH values.
That is Defect 1 being fixed, and it is the only intended output difference.
`GET /api/niches` responses are otherwise byte-identical.

## Open follow-ups

- **SLA-61** — character-level `data_quality` has no DH source, so
  `min_data_quality=full` returns no character niches. Scoped to the single
  `CharacterDemand.DataQuality` field; per decision 3, the per-era variant is
  deleted rather than deferred, since DH's `by_era` entries could not populate
  it even in principle.
- `ByGrade` / `ByPriceTier` are persisted but unread. If they are still unread
  in a few months, delete them rather than let them become the next
  write-only blob.

## Out of scope

- Any change to `internal/adapters/clients/dh` wire types.
- Niche scoring, ranking, or leaderboard ordering.
- The card cache's `DemandScore` / `DemandDataQuality` columns, which are read
  and stay as they are.
