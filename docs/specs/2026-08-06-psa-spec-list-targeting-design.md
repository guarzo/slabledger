# PSA Spec-List Targeting — Design

**Date:** 2026-08-06
**Branch:** `psa-spec-list-targeting` (worktree `.worktrees/psa-spec-list-targeting`)
**Baseline:** `main` @ `1de9c915`

---

## Background

PSA replaced category selection in the Buyer Campaign Manager. Campaigns used to
be `campaignType=CATEGORY` with `category=POKEMON`; they are now
`campaignType=SPEC_LIST` with `prepackagedSpecListIds` naming a curated list —
"Japanese Pokemon" or "English Pokemon". The operator has already converted 6 of
9 live campaigns; 3 remain on the old shape and are paused.

The change is not a serialization detail. Two hard business constraints recorded
in `docs/private/CAMPAIGN_STRATEGY.md:54` have been lifted:

- Language was previously unfilterable — English, Japanese, Chinese and Korean
  cards all flowed through a single Pokemon campaign. The curated list is now a
  language axis.
- Exclusion lists were character-level only and mutually exclusive with
  inclusion lists. The portal now exposes character subjects with an explicit
  polarity *and* a separate card-level deny list (`deniedSpecs`).

So the portal gained targeting power that SlabLedger has no way to express.

### The larger problem this exposes

SlabLedger's campaign targeting model is already materially wrong, and the
portal change is what surfaced it. Both translators explicitly punt subject
translation — `mapper.go` says *"Subject and publisher lists are deferred
(v1) — created empty, filled in the portal UI before activation"* — because no
name→ID resolver was ever reverse-engineered. Every subject list has therefore
been maintained by hand in the portal, and internal state has drifted:

| Campaign | Internal | Portal |
|---|---|---|
| Vintage Core | 41-character inclusion list | `Exclude` / 0 subjects (open net) |
| Vintage PSA 10 | 41-character inclusion list | `Exclude` / 0 subjects (open net) |
| Crystal | 10 characters | 9 |
| Gold Stars | 27 characters | 28 |
| (five others) | inclusion list present | no portal link at all |

The intent is that **SlabLedger is authoritative and pushes accurate campaigns
to the portal, with no direct data entry in the portal.** This design gets there.

---

## Scope

**In scope**

1. Read the new targeting axes off the portal without loss.
2. A one-time baseline pull that copies live portal targeting into SlabLedger,
   performing zero portal writes.
3. Replace `InclusionList string` + `ExclusionMode bool` with three independent
   targeting axes on `inventory.Campaign`, with persistence and a migration.
4. Make `TranslateToCreate` / `TranslateToDiff` emit all three axes, so pushes
   carry targeting.
5. A `getSubjects` resolver for subjects the operator *adds*.
6. Frontend: edit the three axes instead of a comma-separated string.

**Out of scope**

- Retiring `dhlisting.InferDHLanguage` (`psa_import_language.go:13`). It infers
  language from set/card name for DH listing submission, not campaign
  targeting. R-005 is unaffected.
- A UI for *discovering* new cards to deny. Storage, decode and push of
  `deniedSpecs` are in scope; the search/browse experience is not — see
  "Deferred: spec discovery" below.
- Publisher filters. Already decoded and round-tripped; no change needed.
- Automatic conversion of the 3 remaining CATEGORY campaigns. See
  "Conversion path".

---

## Confirmed current behaviour

| Behaviour | Evidence |
|---|---|
| Creates are hardcoded to the dead model | `psacampaign/mapper.go:69-72` — `createCampaignType = "CATEGORY"`, `createCategory = "POKEMON"` |
| Diffs cover scalars only | `mapper.go:14-53` emits `bidPercentage`, `dailyBudget`, grade/year/price bounds, `cardLadderConfidenceMinimum` — nothing else |
| Updates survive the portal change by accident | `psaportal/push.go:36-68` read-modify-writes the raw formData map, mutating only named fields; untouched keys round-trip verbatim |
| Push rejects unknown fields | `push.go:56` — `unknown campaign field %q` |
| `prepackagedSpecListIds` is decoded nowhere | `campaigns_decode.go:58-81` decodes every other formData key including `deniedSpecs` and `selectedPublishers`, but not this one |
| `deniedSpecs` is decoded then discarded | `campaigns_decode.go:79` reads it; `campaigns.go:151-160 applyFormData` maps only subject and publisher filters onto `PortalCampaign` |
| `PortalCampaign` has no field for either axis | `psacampaign/types.go:6-20` |
| Snapshot widening needs no migration | `psa_campaign_snapshot_store.go` — singleton JSONB blob, id=1 (migration 000017) |
| Targeting columns are plain scalars | `migrations/000001_initial_schema.up.sql:145-146` |
| Matching is substring-on-card-or-set-name | `inventory/matching.go:86-100`, `inclusionListMatches:125-139` |
| Coverage duplicates that logic | `postgres/campaign_coverage.go:175-196` — documented as mirroring `matching.go` |
| Domain → `internal/platform` is legal | `scripts/check-imports.sh:11` bans only `internal/adapters`; `inventory/service_cert_entry.go:13` already imports `platform/cardutil` |
| `psacampaign` imports `inventory` | `mapper.go:8` — so `inventory` must not import `psacampaign` (cycle) |

### Consumers of the fields being replaced

11 non-test files reference `InclusionList` or `ExclusionMode`:

```
8  internal/adapters/storage/postgres/campaign_store.go
6  internal/domain/portfolio/analysis.go
6  internal/domain/inventory/suggestion_rules_optimization.go
6  internal/domain/inventory/suggestion_rules.go
6  internal/domain/demand/campaign_signals.go
5  internal/domain/inventory/matching.go
3  internal/domain/inventory/portfolio.go
3  internal/adapters/storage/postgres/campaign_coverage.go
2  internal/domain/inventory/types_core.go
2  internal/domain/demand/repository.go
1  internal/domain/inventory/suggestion_types.go
```

Frontend: `web/src/types/campaigns/core.ts`, `.../portfolio.ts`,
`react/ui/CampaignFormFields.tsx`, `react/pages/CampaignsPage.tsx`,
`react/utils/campaignConstants.ts`, plus two test files.

---

## Design

### 1. Data model — three independent axes

`inventory.Campaign` (`types_core.go:162-182`) gains:

```go
// TargetLanguage selects the PSA curated spec list the campaign buys from.
// "" means unset (a legacy CATEGORY campaign, or a campaign not yet linked).
TargetLanguage string `json:"targetLanguage"` // "" | "english" | "japanese"

// SubjectFilterMode is the polarity of Subjects: "Target" buys only the
// listed characters, "Exclude" buys everything except them. An empty
// Subjects list with mode "Exclude" is an open net.
SubjectFilterMode string `json:"subjectFilterMode"` // "Target" | "Exclude"

// Subjects are the characters this campaign targets or excludes. ID is the
// PSA subject id and is authoritative — it is never re-derived from Name.
Subjects []TargetSubject `json:"subjects"`

// DeniedSpecs are individual cards excluded regardless of Subjects.
DeniedSpecs []TargetSubject `json:"deniedSpecs"`
```

and a new type in the same package:

```go
// TargetSubject is a PSA-namespace identifier with its display name. The ID is
// opaque to SlabLedger: PSA has issued at least three generations of subject
// ids (4xxx, 8xxx, 22xxx) and they coexist on live campaigns, so an id pulled
// from the portal is preserved verbatim rather than re-resolved by name.
type TargetSubject struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}
```

`TargetSubject` lives in `inventory`, not `psacampaign`, because `psacampaign`
imports `inventory` (`mapper.go:8`) and the reverse would cycle. `psacampaign`
maps `inventory.TargetSubject` ↔ `psacampaign.SubjectRef` in its translators.

`InclusionList` and `ExclusionMode` are **retained** on the struct for this
cycle — see the migration section.

#### Why "english"/"japanese" and not the raw UUID

`prepackagedSpecListIds` is a `[]string` of PSA UUIDs. SlabLedger stores a
stable internal token and maps it to the UUID at push time, because:

- the UUID is a PSA implementation detail that can be re-issued;
- the edit page already ships the full `prepackagedSpecLists` catalog, so
  name→UUID resolution is a local lookup on data we already fetch, not an extra
  network call;
- the token is what the operator and every analytics consumer want to reason
  about.

The catalog shape is confirmed from `docs/psa-campaign-edit-raw.json` (node 204):
27 entries of `{id: uuid, name, status}`, e.g.
`6a5484fc-366a-4b5a-90c5-87d72cba3b71 | Riftbound | ENABLED`. That capture
predates this portal change, so it contains **no Pokemon lists** — "Japanese
Pokemon" and "English Pokemon" are new entries that will appear on the live page.
The baseline pull is what records their UUIDs; nothing in this design hardcodes
them.

The token→UUID map is built from the catalog on the edit-page fetch and cached
per run. Resolution matches on name and requires `status == "ENABLED"`. If a
token has no matching enabled list, the push fails loudly rather than pushing an
empty list, which would silently widen the campaign to every card PSA sells. The
edit page carries a `removedUnavailableSpecList` field, so the portal has its own
notion of a list going away — the status check is not hypothetical.

#### Persistence

Migration `000023_campaign_targeting_axes`:

```sql
ALTER TABLE campaigns
  ADD COLUMN target_language      TEXT  NOT NULL DEFAULT '',
  ADD COLUMN subject_filter_mode  TEXT  NOT NULL DEFAULT 'Target',
  ADD COLUMN subjects             JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN denied_specs         JSONB NOT NULL DEFAULT '[]'::jsonb;
```

`inclusion_list` and `exclusion_mode` are left in place and keep being written
(see migration strategy). No index is added: the campaigns table holds ~10 rows
and every consumer scans it whole (`campaign_coverage.go:45-51`, `:131-137`).

### 2. Read path — stop losing fields

Three edits, all mechanical:

- `campaigns_decode.go:58` — add
  `PrepackagedSpecListIDs: asStrings(fd["prepackagedSpecListIds"])`, with a new
  `asStrings` helper alongside the existing `asSubjectRefs`.
- `psacampaign/types.go` — `PortalCampaign` gains `SpecListIDs []string`,
  `SpecListNames []string` and `DeniedSpecs []SubjectRef`.
- `campaigns.go:151 applyFormData` — carry `PrepackagedSpecListIDs` and
  `DeniedSpecs` onto the `PortalCampaign`.

`SpecListNames` is resolved from the edit-page catalog at decode time so the
snapshot is human-readable and survives a PSA re-key of the UUIDs.

The snapshot store needs no migration — it stores `PortalCampaign` as an opaque
JSONB blob. Its existing refusal to save an empty snapshot stays: it is the
guard that would catch a decode regression blanking targeting wholesale.

### 3. One-time baseline pull

A `-baseline-pull` flag on `cmd/psa-harvest`. In that mode the job:

1. logs in once (existing session machinery);
2. fetches all campaigns *including* the edit-form fetch per campaign, so the
   three axes are populated;
3. writes a report to stdout and to `psa_campaign_snapshot`;
4. for each campaign with a `PSACampaignRequestID` link, writes the portal's
   targeting into the campaign row — `target_language`,
   `subject_filter_mode`, `subjects`, `denied_specs`;
5. **skips `DrainPushQueue` entirely.** Zero portal writes.

Point 5 is the whole safety property, and it is enforced structurally: the flag
short-circuits before the drain call in `cmd/psa-harvest/main.go`, rather than
relying on the queue happening to be empty.

Subject IDs are copied **verbatim**. This is the single highest-risk decision in
the design. Gold Star currently holds `4807 Charizard`; Masaki mixes `8105
Crystal Golem` (8xxx) with `22210 Machamp` (22xxx); `getSubjects` returns only
22xxx ids. Resolving by name during the baseline would rewrite the subject ids
on six ACTIVE, money-spending campaigns on the first subsequent push. So:
**ids that came from the portal are never re-resolved.** `getSubjects` is used
only to resolve subjects the operator newly adds.

The five campaigns with an inclusion list but no portal link are **not**
touched: they have nothing to pull from. Their `inclusion_list` is left as-is
and the report flags them for the operator to link or discard.

The report names, per campaign: the axes pulled, and every case where the
pulled value disagrees with the existing `inclusion_list`. Vintage Core and
Vintage PSA 10 will report "41 internal characters replaced by Exclude/0
(open net)" — that is the correct outcome (portal is truth at baseline time),
but it is a large enough change to deserve an explicit line in the report.

### 4. Write path

`TranslateToCreate` (`mapper.go:80`) changes:

```go
CampaignType:           "SPEC_LIST",
Category:               "",
PrepackagedSpecListIDs: specListIDsFor(internal.TargetLanguage),
SubjectFilterType:      internal.SubjectFilterMode,
SelectedSubjects:       toSubjectRefs(internal.Subjects),
DeniedSpecs:            toSubjectRefs(internal.DeniedSpecs),
```

with `createCampaignType`/`createCategory` deleted. A campaign with an empty
`TargetLanguage` fails translation with a clear error rather than creating an
untargeted campaign.

`TranslateToDiff` gains three comparisons. Unlike the existing scalar changes,
these are list-valued, so `FieldChange.Old`/`.New` carry a canonical rendering:
ids sorted ascending, joined `"id:name"` comma-separated. Sorting matters — the
portal does not guarantee list order, and an unsorted comparison would emit a
spurious diff on every run and re-push identical targeting hourly.

`PushCampaign` needs no change to its mutation loop, but `numericFormDataFields`
does not cover list values. The loop's `else` branch assigns `ch.New` as a
string, which would send `"4807:Charizard,..."` where the portal expects an
array. So `FieldChange` gains an optional structured payload:

```go
// Value carries the new value for list-valued fields, where the string
// rendering in New is for display and audit only.
Value any `json:"value,omitempty"`
```

`push.go` uses `ch.Value` when non-nil and falls back to `ch.New` otherwise, so
every existing scalar change is unaffected.

The unknown-field guard at `push.go:56` interacts with conversion — see below.

### 5. Subject resolution

`getSubjects` is a SvelteKit remote function taking a category id (`[16]` =
POKEMON) and returning 492 `{id, name}` Pokemon subjects. It uses the same
envelope as `createCampaign`/`updateCampaign`, so the existing `fetchRemoteHash`
+ `EncodeRefPacked`/`DecodeRefPacked` machinery applies directly.

New `psaportal.FetchSubjects(ctx, categoryID int) ([]psacampaign.SubjectRef, error)`,
memoized per run alongside `remoteHashCache`. It is called only when a campaign
carries a subject with `ID == 0` (operator added it by name in the UI). Unknown
names fail the push with the offending name in the error, rather than being
silently dropped.

### 6. Matching — one implementation

`inventory.PurchaseMatchesCampaign` (`matching.go:42`) replaces the
inclusion-list block with the three axes:

- **Language.** If `TargetLanguage != ""`, the purchase's set name must match.
  Japanese sets carry a "Japanese " prefix that `platform/cardutil` already
  detects (`normalize_sets.go:129`); `TargetLanguage == "english"` means the
  prefix must be absent. Domain → platform is a legal import.
- **Subjects.** Case-insensitive comparison against the card's character, with
  `SubjectFilterMode` supplying polarity. An empty list means no constraint,
  matching current behaviour for an empty inclusion list.
- **DeniedSpecs.** If the purchase matches a denied spec, reject.

The set-name half of the old substring match is deliberately dropped for
subjects: a subject is a character, and matching a character name against a set
name is how "japanese" as a list entry accidentally became a language filter.
Language now has its own axis. Auditing the existing 41-character lists for
entries that are really set or language tokens is part of the baseline report.

`campaign_coverage.go:175-196 characterMatchesInclusion` is deleted and the
coverage lookup calls a single exported predicate from `inventory`. That removes
the duplicate implementation the existing comment already warns about.

`demand.ActiveCampaign` (`repository.go`) carries the new axes in place of the
two old fields, and `campaign_signals.go`, `portfolio/analysis.go`,
`suggestion_rules.go`, `suggestion_rules_optimization.go` and
`inventory/portfolio.go` are updated to read them.

### 7. Migration strategy for the old fields

`inclusion_list` and `exclusion_mode` are **not dropped in this cycle**. They
become a derived, write-only mirror: on every campaign write the store
serializes `Subjects` back into the comma-separated string and
`SubjectFilterMode == "Exclude"` into the bool. Nothing reads them after this
change.

This costs a few lines in `campaign_store.go` and buys a rollback: if the new
model misbehaves in production, reverting the binary leaves a database whose
legacy columns are still correct. A follow-up migration drops both columns once
the new model has run a full cycle. This is called out explicitly so it is not
forgotten — the mirror is transitional, not permanent.

Backfill in `000023`: `subject_filter_mode` is set from `exclusion_mode`, and
`subjects` from `inclusion_list` split on commas with `id = 0` and the token as
`name`. Those zero ids are placeholders. The baseline pull overwrites them with
real portal ids on every campaign that carries a `PSACampaignRequestID`; the
five unlinked campaigns keep placeholders and cannot be pushed until the
operator resolves them — which the `ID == 0` resolution path in §5 handles.

### 8. Conversion path for the 3 remaining CATEGORY campaigns

They are paused, so there is no live money at stake. They are converted by an
ordinary push once the operator sets `TargetLanguage` on each, with one caveat:
`push.go:56` rejects a field absent from the fetched formData. If a CATEGORY-era
edit form does not carry `prepackagedSpecListIds`, the push fails terminally
rather than converting.

The baseline pull answers this — it fetches the edit form for all 9 campaigns
and the report records, per campaign, whether the key is present. If it is
absent on CATEGORY campaigns, the fallback is to flip them in the portal by hand
(three campaigns, one-time) and re-run the baseline. That is a deliberate
acceptance: building a create-and-relink path to work around a one-time,
three-row problem is not worth it.

### 9. Frontend

`CampaignFormFields.tsx` replaces the inclusion-list textarea with:

- a language select (Unset / English / Japanese);
- a polarity toggle (Target / Exclude);
- a subject list editor — add by name with typeahead against a
  `GET /api/psa/subjects` endpoint backed by the cached `getSubjects` result,
  chips showing name with the id in the title attribute;
- a read-only denied-specs list, since discovery is deferred.

`web/src/types/campaigns/core.ts` and `portfolio.ts` mirror the new Go JSON
tags. `campaignConstants.ts` and `CampaignsPage.tsx` drop their inclusion-list
handling.

### Deferred: spec discovery

`deniedSpecs` is stored, decoded, diffed and pushed — a card denied in the
portal survives the baseline and every subsequent push, and one removed in
SlabLedger is removed at the portal. What is *not* built is a UI to search PSA's
spec catalog and add a new denial.

The reason is a genuine data gap: `SpecDenyModal.CN8DXVGy.css` exists in the
captured HAR (`tmp/www.psacard.com.har`, 2026-07-15, 176 entries) but the modal
was never opened during capture, so no spec-search call was recorded.
`docs/psa-campaign-edit-raw.json` ships catalogs for `categories`, `publishers`
and `prepackagedSpecLists` but no spec catalog. The
resolution contract is unknown, and guessing it would mean pushing unvalidated
identifiers to a live buying system.

One HAR capture with the modal opened closes this. Until then, adding a new
denial is done in the portal and picked up by the next pull — the one documented
exception to "no direct data entry in the portal", and a narrow one.

---

## Error handling

| Condition | Behaviour |
|---|---|
| `TargetLanguage` empty on create | Translation fails with a named error. Never create an untargeted campaign. |
| Language token has no matching curated list | Push fails. Never push an empty `prepackagedSpecListIds` — it would widen the campaign to PSA's whole catalog. |
| Subject name unresolvable via `getSubjects` | Push fails, naming the subject. Never silently drop. |
| `prepackagedSpecListIds` absent from a CATEGORY edit form | Existing `push.go:56` guard fires; report flags the campaign for manual conversion. |
| Transient portal failure during baseline | Existing `drain.go` classification is unchanged; the baseline is idempotent and simply re-runs. |
| Decode regression blanking targeting | Snapshot store's empty-save refusal fires. |

---

## Testing

- **Translators** (`psacampaign/mapper_test.go`): table-driven over the three
  axes — create with each language, diff with reordered subject lists asserting
  no spurious change, diff with an actual subject change, empty-language error.
- **Decode** (`psaportal/campaigns_decode_test.go`): a SPEC_LIST fixture and a
  CATEGORY fixture, asserting `PrepackagedSpecListIDs` and `DeniedSpecs`
  survive to `PortalCampaign`.
- **Matching** (`inventory/matching_test.go`): language axis against
  Japanese-prefixed and plain set names; Target vs Exclude polarity; empty
  subject list as no-constraint; denied spec overriding a subject match.
- **Coverage parity**: `campaign_coverage` and `matching` agree, now trivially,
  because they share one predicate — a test asserting so guards the regression
  the deleted duplicate caused.
- **Migration**: up/down against a fixture with both an inclusion-mode and an
  exclusion-mode campaign, asserting the derived mirror round-trips.
- **Mocks**: extend `internal/testutil/mocks` per
  `internal/testutil/mocks/README.md`; no inline mocks.
- `go test -race ./...` and `make check` before any completion claim.

---

## Verification

1. `make check` — lint, `scripts/check-imports.sh` (domain must not import
   adapters; `inventory` must not import `psacampaign`), file-size budget.
   `mapper.go` and `matching.go` both grow; if either passes 500 lines it splits
   (subject translation into `mapper_subjects.go`, axis predicates into
   `matching_targeting.go`).
2. `go test -race -timeout 10m ./...`
3. `-baseline-pull` against production, reviewing the report before any push is
   approved.
4. Confirm the push queue is empty and no `updateCampaign` call was made during
   the baseline — portal-side campaign `updatedAt` unchanged for all 9.
5. `cd web && npm run build && npm test`.

---

## Risks

- **Subject re-keying.** Mitigated by never re-resolving portal-sourced ids.
  This is the failure mode that would quietly change what six active campaigns
  buy, and it is worth re-checking at review time.
- **List-diff churn.** An unsorted comparison re-pushes identical targeting
  every hour. Mitigated by canonical sorted rendering, and covered by the
  reordered-list test.
- **The baseline overwrites intent.** Vintage Core's 41-character list is
  discarded in favour of the portal's open net. That is correct — the portal is
  what actually spent money — but the list represents real thinking, so the
  report preserves it in full for the operator to re-apply deliberately.
- **Deferred spec discovery** leaves one narrow portal-data-entry path open,
  contrary to the stated goal. Closing it needs one HAR capture, not a design
  change.
