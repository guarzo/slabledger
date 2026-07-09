# CardLadder Pricing Fix: `gemRateId` → `profileId` Contract Migration

**Date:** 2026-06-21
**Status:** Design approved — ready for implementation plan
**Branch:** `fix/cl-pricing-profileid`
**Source:** `CL_PRICING_ROOT_CAUSE_HANDOFF.md` (root cause), plus live API re-verification on 2026-06-21

---

## Problem

CardLadder split one identifier into two. Our code was written when they were a single
value — which was true until this migration — so it reads and sends fields that no longer
exist on CardLadder's responses. Every cert fails resolution and **no inventory gets a CL
price**.

### The identifier split (live-verified 2026-06-21)

| Identifier | Example value | Now used by |
|---|---|---|
| **profileId** | `psa-1813135` | CL `buildcollectioncard` output, `cardestimate` input, `salesarchive` filter key |
| **hash gemRateId** | `fb123c6ac87154de3bab52bcf3f90ecacbc760fc` | DH direct-lookup hint; still echoed on CL comp rows as `gemRateId` + `universalGemRateId` |

`httpbuildcollectioncard` no longer returns `gemRateId`, `gemRateCondition`, or `condition`.
It now returns `profileId` and `grade` (firestore form, e.g. `"g8"`). Our `BuildCardResponse`
struct maps the absent fields → they unmarshal to `""` → the `resolveGemRate` guard
(`resp.GemRateID == "" || resp.Condition == ""`) rejects every card.

### Live evidence (re-verified this session, not just inherited from the handoff)

```
POST httpbuildcollectioncard {"data":{"cert":"158507531","grader":"psa"}}
  result keys: ... grade, profileId, set, ...   (gemRateId / gemRateCondition / condition ABSENT)
  profileId = "psa-1813135"   grade = "g8"

POST httpcardestimate {"data":{"profileId":"psa-1813135",...,"condition":"g8"}}  → estimatedValue = 429   ✅
POST httpcardestimate {"data":{"gemRateId":"psa-1813135",...}}                   → estimatedValue = null  ❌

GET salesarchive  filters=...|gemRateId:psa-1813135|...  → totalHits = 0
GET salesarchive  filters=...|profileId:psa-1813135|...  → totalHits = 4   ✅
   comp row: profileId="psa-1813135"  gemRateId="fb123c6ac8…"  universalGemRateId="fb123c6ac8…"
```

The comp row's `gemRateId` (`fb123c6ac8…`) is a **different value** from its `profileId`
(`psa-1813135`). They are not interchangeable.

---

## Scope (approved)

Fix all three CL code paths broken by the contract change, in one PR:

1. **Card-value pricing** (the outage) — `cardladder_refresh.go` resolve + estimate.
2. **Sales comps** — `FetchSalesComps` filter + comp storage identifier.
3. **CL collection push** — `ResolveAndCreateCard`.

Internal naming: **reuse existing DB columns and Go field names** (`gem_rate_id`,
`cl_gem_rate_id`, `GemRateID`). Only JSON tags and the value that flows through change. No
migration for column renames. The name becomes a mild misnomer (it now holds a profileId);
accepted to keep the diff small and low-risk.

---

## Design by path

### Path A — Card-value pricing (restores the outage)

**`internal/adapters/clients/cardladder/types.go`**
- `BuildCardResponse`: map `profileId` into the existing `GemRateID` field, and `grade`
  into `GemRateCondition`. Remove the now-dead `gemRateId` / `gemRateCondition` /
  `condition` JSON tags.
- `CardEstimateRequest`: change the `GemRateID` field's JSON tag from `gemRateId` to
  `profileId`. (The live `cardestimate` ignores `gemRateId` and requires `profileId`.)

**`internal/adapters/scheduler/cardladder_refresh.go` (`resolveGemRate`, ~478–548)**
- The guard, cache write, mapping persist, purchase persist, and return all keep using the
  `GemRateID` field — which now carries the profileId. No structural change.
- **Condition form is the subtlety.** CL's new `grade` is firestore form (`"g8"`), but the
  `cl_condition` column contract is **display form** (`"PSA 8"`), relied on by
  `firestoreConditionFor` and every downstream catalog/estimate lookup (see
  `cardladder_sync.go:82–84`). So convert before storing: `BuildCardResponse.GemRateCondition`
  (`"g8"`) → `cardutil.ConditionToAPIFormat("g8")` → `"PSA 8"`. This keeps the existing
  `firestoreConditionFor("PSA 8") → "g8"` round-trip intact at the estimate call site.

### Path B — Sales comps (prevents a silent join breakage)

**`internal/adapters/clients/cardladder/client.go:131` (`FetchSalesComps`)**
- Filter string `gemRateId:%s` → `profileId:%s`. (Old key returns 0 hits.)

**`internal/adapters/clients/cardladder/types.go` (`SaleComp`)**
- Add a `ProfileID string \`json:"profileId"\`` field (keep the existing `GemRateID` field
  for completeness/debug, but it is no longer the lookup key).

**`internal/adapters/scheduler/cardladder_gap_fill.go:68`**
- Store `comp.ProfileID` (not `comp.GemRateID`) into `cl_sales_comps.gem_rate_id`.
- **Why this is critical:** purchases now store profileId in `campaign_purchases.gem_rate_id`.
  The comp-refresh join (`comp_refresh.go`: `sc.gem_rate_id = cp.gem_rate_id`) only matches
  if comps are also keyed by profileId. Storing the comp-row hash would fetch comps that
  never join — the pipeline would look healthy while matching nothing.

### Path C — CL collection push

**`internal/adapters/clients/cardladder/firestore.go` (`ResolveAndCreateCard`, ~170–233)**
- Feed `CardEstimateRequest` from `buildResp.GemRateID` (profileId) and a condition derived
  from the new `grade` instead of the absent `gemRateCondition`.
- Verify against a live push that the Firestore card document still writes acceptable values
  (the `gemRateId` / `gemRateCondition` Firestore doc fields at `firestore.go:305–306`).
  In scope to avoid regressing the CL collection UI, but not part of the pricing outage.

### DH path — no change (investigated, not assumed)

DH's `POST /api/v1/enterprise/certs/resolve` takes a `gemrate_id` field used as a
"skip fuzzy matching" hint. Live tests this session:

- Resolve with **no identifier** → `matched`, returns `gemrate_id = "fb123c6ac8…"` (the hash,
  identical to CL's comp-row `gemRateId`). DH lives in the **hash** identifier space.
- Resolve sending a **profileId** as `gemrate_id` (alongside a valid cert) → still `matched`,
  correct `dh_card_id`, still echoes the hash.
- Sending `gemrate_id` **alone** (hash or profileId, no cert) → `"Invalid cert number format"`.
  So `gemrate_id` is only a hint; `cert_number` is the real key.

**Conclusion:** after the fix the shared column holds a profileId, so the DH hint stops being
a valid hash — DH **loses an optimization, it does not break** (it falls back to cert-based
matching and still returns the correct card). Leave the DH contract untouched; add a code
comment noting the hint is now a profileId. A future enhancement could persist DH's returned
hash separately, but that is out of scope here.

---

## Backfill (approved: clear + re-resolve)

After deploy, the 34 existing mapped rows hold OLD hash gemRateIds (or empty). They must be
cleared so the refresh re-resolves them to profileIds:

```sql
UPDATE cl_card_mappings   SET cl_gem_rate_id = '' , cl_condition = '';
UPDATE campaign_purchases SET gem_rate_id    = '' WHERE gem_rate_id <> '';
```

`cl_sales_comps` rows keyed by the old hash become orphaned; the comp refresh re-populates
with profileId-keyed rows on the next run. Open decision for the plan: run this as a one-time
runbook step vs. an idempotent migration. (Reuse-columns means no schema migration is
otherwise required.)

---

## Testing

**Unit (no creds):**
1. `BuildCardResponse` unmarshals the **real captured JSON** (profileId + grade present, old
   fields absent) → non-empty `GemRateID`, `GemRateCondition == "g8"`.
2. `CardEstimateRequest` serializes the identifier under the `profileId` JSON key.
3. Condition round-trip: `g8 → ConditionToAPIFormat → "PSA 8" → firestoreConditionFor → g8`.
4. `SaleComp` unmarshals the captured comp JSON and exposes `ProfileID == "psa-1813135"`;
   gap-fill stores `ProfileID` into the comp record.

**Integration (`-tags integration`, creds in env):**
5. Resolve cert `158507531` → estimate → expect `estimatedValue ≈ 429`.
6. `FetchSalesComps` with the profileId filter → `totalHits > 0`.

**Post-deploy verification:**
7. `/api/admin/cardladder/status` shows `resolved > 0`, `certResolveFailed` collapses,
   `updated > 0`, `cardsMapped` climbs past 34.

---

## Files touched

| File | Change |
|---|---|
| `cardladder/types.go` | `BuildCardResponse` tags → profileId/grade; `CardEstimateRequest` tag → profileId; `SaleComp` add `ProfileID` |
| `scheduler/cardladder_refresh.go` | condition-form conversion in `resolveGemRate` |
| `cardladder/client.go` | `FetchSalesComps` filter `gemRateId:` → `profileId:` |
| `scheduler/cardladder_gap_fill.go` | store `comp.ProfileID` into comp record |
| `cardladder/firestore.go` | `ResolveAndCreateCard` estimate inputs; DH-hint comment |
| `cardladder/*_test.go` | new unit tests with captured JSON |
| backfill | one-time SQL (runbook vs migration — decide in plan) |

## Out of scope
- Renaming DB columns or Go fields (`gem_rate_id` → `profile_id`).
- Changing the DH cert-resolve contract or persisting DH's hash separately.
- Any non-CardLadder pricing source.
