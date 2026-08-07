# PSA Spec-List Targeting — Design

**Date:** 2026-08-06
**Branch:** `psa-spec-list-targeting` (worktree `.worktrees/psa-spec-list-targeting`)
**Baseline:** `main` @ `1de9c915`

> **Amended 2026-08-07.** This spec originally modeled `TargetLanguage` as a
> single token (`"" | "english" | "japanese"`). Shipped code
> (`docs/plans/2026-08-07-psa-multi-language-axis.md`) replaced it with
> `TargetLanguages []string`, because every live campaign carries **both**
> curated language lists at once and a single-token model cannot represent
> that. This document has been updated throughout to describe the
> multi-valued axis as shipped, and the curated portal list names have been
> corrected to their real values: **"Pokemon - English Language Only"** and
> **"Pokemon - Japanese Language Only"** (the code briefly used the wrong
> names "English Pokemon"/"Japanese Pokemon" in both pull and push
> directions; fixed in commit `8b5e2f1e`). This is the authoritative design
> record for the shipped multi-language axis; the 2026-08-06 and
> 2026-08-07 plan documents are left as historical execution records and are
> not mass-corrected — see the corrections note at the top of each.

---

## Background

PSA replaced category selection in the Buyer Campaign Manager. Campaigns used to
be `campaignType=CATEGORY` with `category=POKEMON`; they are now
`campaignType=SPEC_LIST` with `prepackagedSpecListIds` naming one or more curated
lists — "Pokemon - Japanese Language Only" and/or "Pokemon - English Language
Only". The operator has already converted 6 of 9 live campaigns; 3 remain on the
old shape and are paused. All 6 converted campaigns carry **both** curated
lists at once, which is why the language axis must be multi-valued rather than
a single token.

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
// TargetLanguages is the set of PSA curated spec lists the campaign buys
// from, held as stable internal tokens ("english" | "japanese") rather than
// portal UUIDs (which PSA can re-issue). It is an unordered set.
//
// Empty means an open net: the campaign buys any language. Every live
// campaign carries BOTH "english" and "japanese" — a single-token model
// cannot represent them.
TargetLanguages []string `json:"targetLanguages"`

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

#### Persistence

Migration `000024_campaign_targeting_axes`:

```sql
ALTER TABLE campaigns
  ADD COLUMN target_languages    JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN subject_filter_mode TEXT  NOT NULL DEFAULT 'Target',
  ADD COLUMN subjects            JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN denied_specs        JSONB NOT NULL DEFAULT '[]'::jsonb;
```

`inclusion_list` and `exclusion_mode` are left in place and keep being written
(see migration strategy). No index is added: the campaigns table holds ~10 rows
and every consumer scans it whole (`campaign_coverage.go:45-51`, `:131-137`).

#### Why "english"/"japanese" tokens, held as a set, and not the raw UUID

`prepackagedSpecListIds` is a `[]string` of PSA UUIDs. SlabLedger stores a
stable internal token and maps it to the UUID at push time, because:

- the UUID is a PSA implementation detail that can be re-issued;
- the edit page already ships the full `prepackagedSpecLists` catalog on a page
  the harvester already loads, so name→UUID resolution needs no extra portal
  call — only that we stop discarding the catalog (§2a);
- the token is what the operator and every analytics consumer want to reason
  about.

The catalog shape is confirmed from `docs/psa-campaign-edit-raw.json` (node 204):
27 entries of `{id: uuid, name, status}`, e.g.
`6a5484fc-366a-4b5a-90c5-87d72cba3b71 | Riftbound | ENABLED`. That capture
predates this portal change, so it contains **no Pokemon lists** — "Pokemon -
Japanese Language Only" and "Pokemon - English Language Only" are new entries
that appear on the live page. The baseline pull is what records their UUIDs;
nothing in this design hardcodes them.

Resolution matches on name and requires `status == "ENABLED"`. If a token has no
matching enabled list, translation fails loudly rather than emitting an empty
list, which would silently widen the campaign to every card PSA sells. The edit
page carries a `removedUnavailableSpecList` field, so the portal has its own
notion of a list going away — the status check is not hypothetical.

**Where the catalog lives is a cross-process problem** — see §2a.

### 2a. Catalog ownership across processes

Translation does not happen in the process that can talk to the portal.
`TranslateToDiff` and `TranslateToCreate` are called from the main HTTP server
(`httpserver/handlers/campaigns_psa.go:137`, `:264`), which has no browser
session and reads only the DB snapshot. The browser-capable code is a separate
binary, `cmd/psa-harvest`. So a per-run in-memory cache inside `psaportal.Client`
— where `remoteHashCache` lives — is unreachable from translation, and cannot
back a `/api/psa/subjects` endpoint either.

Worse, the catalog is currently thrown away before it leaves the adapter:
`fetchCampaignFormData` (`campaigns.go:50-68`) extracts `formData` from the
edit-page root and discards everything else, including `prepackagedSpecLists`.

The fix is a persisted catalog with a domain port, mirroring the existing
`SnapshotStore` / `PushQueueStore` / `CampaignLinker` shape in
`psacampaign/repository.go:33-62`:

```go
// SpecListRef is a PSA curated spec list: portal UUID, display name, status.
type SpecListRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // "ENABLED" | …
}

// CatalogStore persists PSA-side reference data harvested by the browser job so
// the main server — which has no portal session — can resolve names to portal
// identifiers at translation time.
type CatalogStore interface {
	SaveSpecLists(ctx context.Context, lists []SpecListRef) error
	SaveSubjects(ctx context.Context, categoryID int, subjects []SubjectRef) error
	SpecLists(ctx context.Context) ([]SpecListRef, time.Time, error)
	Subjects(ctx context.Context, categoryID int) ([]SubjectRef, time.Time, error)
}
```

Migration `000025_psa_portal_catalog`: `(kind TEXT, key TEXT, payload JSONB,
fetched_at TIMESTAMPTZ)` with `PRIMARY KEY (kind, key)` — `kind` is
`'spec_lists'` or `'subjects'`, `key` is `''` or the category id. The harvester
writes it on every run; the server reads it. It wires into `cmd/slabledger` as
another optional port beside `PSASnapshotStore` and `PSAPushQueue`
(`cmd/slabledger/server.go:65-66`).

`fetchCampaignFormData` is widened to return the spec-list catalog alongside the
formData so the harvester can persist it without a second fetch.

The translators stay pure — they take a resolver rather than reaching for
storage:

```go
// Resolver maps SlabLedger's stable tokens to PSA portal identifiers. Built
// from CatalogStore by the caller; the translators never touch storage.
type Resolver interface {
	SpecListIDs(languageTokens []string) ([]string, error)
	SubjectID(name string) (int, error)
}

func TranslateToCreate(internal inventory.Campaign, r Resolver) (CampaignFormData, error)
func TranslateToDiff(internal inventory.Campaign, portal PortalCampaign, r Resolver) (ProposedDiff, error)
```

**Staleness fails closed.** If the catalog is missing, or its `fetched_at` is
older than 7 days, the resolver returns an error naming the staleness and
translation fails with "PSA catalog is stale — run the harvester". Translating
against a catalog that predates a PSA re-key is exactly how a campaign gets
pushed pointing at a list that no longer means what it meant.

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
JSONB blob. Its existing refusal to save an empty snapshot stays, but note what
it does and does not cover: it rejects a *total* fetch failure
(`psa_campaign_snapshot_store.go:29-30`, `len(campaigns) == 0`) and nothing
finer. Per-campaign blanking is caught by `TargetingComplete` instead — see §3.

### 3. One-time baseline pull

A `-baseline-pull` flag on `cmd/psa-harvest`. In that mode the job:

1. logs in once (existing session machinery);
2. fetches all campaigns *including* the edit-form fetch per campaign, so the
   three axes are populated;
3. persists the spec-list catalog and the `getSubjects` result (§2a);
4. writes a report to stdout and to `psa_campaign_snapshot`;
5. for each campaign that is **both** linked (`PSACampaignRequestID` set) **and**
   complete (see below), writes the portal's targeting into the campaign row —
   `target_languages`, `subject_filter_mode`, `subjects`, `denied_specs`, copying
   **every** recognized curated language list, not collapsing to one;
6. **skips `DrainPushQueue` entirely.** Zero portal writes.

Point 6 is the whole safety property, and it is enforced structurally: the flag
returns before the `DrainPushQueue` call at `cmd/psa-harvest/main.go:121`, rather
than relying on the queue happening to be empty.

#### Per-campaign completeness — the baseline must fail closed

`FetchCampaigns` currently swallows a failed edit-form fetch and appends the
campaign anyway:

```go
fd, err := c.fetchCampaignFormData(ctx, pc.CampaignRequestID)
if err != nil {
	c.logger.Warn(ctx, "psaportal: edit fetch failed", …)   // campaigns.go:32-38
} else {
	applyFormData(&pc, fd)
}
out = append(out, pc)
```

So one transient 403 on one edit form yields a `PortalCampaign` with zero-valued
targeting that is indistinguishable from a campaign genuinely targeting nothing.
Writing that into the campaign row would erase real targeting during the one
operation whose entire purpose is a faithful copy — the worst possible time.

An earlier draft of this design claimed the snapshot store's empty-save refusal
guarded against this. **It does not**:
`psa_campaign_snapshot_store.go:29-30` rejects only `len(campaigns) == 0`, a
total failure. It cannot see a single campaign's targeting go blank.

So `PortalCampaign` gains:

```go
// TargetingComplete is false when the edit-form fetch for this campaign failed,
// meaning the targeting fields are zero values rather than portal truth. The
// baseline pull refuses to write a campaign row from an incomplete record.
TargetingComplete bool `json:"targetingComplete"`
```

set true only on the `applyFormData` path. The baseline then:

- **skips** any campaign with `TargetingComplete == false` — no row write;
- names each skipped campaign in the report;
- **exits non-zero** if any campaign was skipped, so a partial baseline is a
  visible failure the operator re-runs rather than a silent partial success.

The baseline is idempotent, so re-running after a transient failure is the whole
remedy. The normal (non-baseline) harvest path keeps today's warn-and-continue
behaviour: a stale snapshot entry is a display problem, while a bad row write is
a data-loss problem, and only the latter needs to fail closed.

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
PrepackagedSpecListIDs: specListIDs,                  // from Resolver, all of TargetLanguages (§2a)
SubjectFilterType:      internal.SubjectFilterMode,
SelectedSubjects:       toSubjectRefs(internal.Subjects),
DeniedSpecs:            toSubjectRefs(internal.DeniedSpecs),
```

with `createCampaignType`/`createCategory` deleted. A campaign with an empty
`TargetLanguages` set, or one whose language tokens the resolver cannot fully map
to enabled lists, fails translation rather than creating an untargeted campaign.

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

New `psaportal.FetchSubjects(ctx, categoryID int) ([]psacampaign.SubjectRef, error)`.
It runs in the **harvester**, on every run, and its result is persisted via
`CatalogStore.SaveSubjects` (§2a). The main server never calls it — it reads the
persisted catalog, which is what makes both `Resolver.SubjectID` and the
typeahead endpoint in §9 possible from a process with no portal session.

Resolution is used only when a campaign carries a subject with `ID == 0` — an
operator-entered name that has never been reconciled with the portal. Portal-
sourced ids are never re-resolved (§3). An unresolvable name fails the push with
the offending name in the error, rather than being silently dropped.

### 6. Matching — one implementation

`inventory.PurchaseMatchesCampaign` (`matching.go:42`) already takes six
positional arguments and needs two more (card number, PSA spec id), so it moves
to a struct parameter:

```go
// MatchInput is the purchase-side half of a campaign match.
type MatchInput struct {
	Grade        float64
	BuyCostCents int
	CardName     string
	SetName      string
	CardNumber   string // within-set number, from PSA
	PSASpecID    int    // Card Ladder spec id; 0 when unmapped
	CardYear     int
}

func PurchaseMatchesCampaign(in MatchInput, c *Campaign) bool
func FindMatchingCampaign(in MatchInput, allCampaigns []Campaign) MatchResult
```

There is exactly one caller outside the package
(`inventory/service_import_psa.go:104`), so the blast radius is small. Both new
fields are already on `Purchase` (`types_core.go:211` `CardNumber`, `:287`
`PSASpecID`).

The inclusion-list block is replaced by the three axes:

**Language.** If `TargetLanguages` is non-empty, the purchase's classified set
language must be a member of the set (an empty set is an open net — matches any
language). Classification is **marker-based and positive** — never "English is
whatever isn't Japanese". That negation was in an earlier draft and it is wrong: there are
real Simplified Chinese Pokemon certs in this inventory
(`cardutil/normalization_chain_test.go:134-145`, e.g. `SIMPLIFIED CHINESE CBB1
C-GEM PACK VOL 1`), and calling them English would make SlabLedger's matching
disagree with what PSA's curated English list actually buys.

A new `cardutil.SetLanguage(setName) string` returns `"japanese"`, `"chinese"`,
`"korean"`, `"english"`, or `""` (unrecognized), built from markers the package
already knows:

- Chinese — `cardutil.IsChineseSet` (`normalize_sets.go:352`), which matches a
  `cn ` prefix or `chinese` anywhere in the name, covering both the
  `SIMPLIFIED CHINESE …` and `TRADITIONAL CHINESE …` forms;
- Japanese — the existing `"japanese "` prefix test (`normalize_sets.go:129`),
  promoted to an exported `IsJapaneseSet`;
- Korean — a new `"korean "` prefix test, same shape;
- otherwise `"english"`.

**The English default is an assumption, and it is the one thing here that must be
checked against data before this ships.** It holds only if every non-English set
in inventory carries a marker. Implementation includes a query over distinct
`set_name` in `campaign_purchases` to confirm no unmarked non-English sets exist;
if any turn up, `SetLanguage` returns `""` for them and a language-constrained
campaign does not match, failing closed with a log line rather than silently
buying the wrong language.

**Subjects.** Case-insensitive comparison of `CardPlayer`/card name against
`Subjects`, with `SubjectFilterMode` supplying polarity. An empty list means no
constraint, matching current behaviour for an empty inclusion list.

**DeniedSpecs.** Identity is **ID-first**: if both `in.PSASpecID` and the denied
entry's `ID` are non-zero, deny on equality. Otherwise fall back to normalized
set name plus `CardNumber`. If neither identity is available, **do not deny**.

That fail-open direction is deliberate and worth stating, because the instinct
runs the other way. This predicate does not gate buying — the portal enforces
denials at buy time, which is why `deniedSpecs` is pushed there. Internally it
only attributes an already-purchased card to a campaign. A false deny would
silently misattribute a legitimate purchase and distort every downstream
analytic; a missed deny merely attributes a card that shouldn't have been bought,
which is visible and correctable. `PSASpecID` is `omitempty` and CL-sourced, so
it is frequently 0 — the fallback is the common path, not the edge case.

The set-name half of the old substring match is deliberately dropped for
subjects: a subject is a character, and matching a character name against a set
name is how "japanese" as a list entry accidentally became a language filter.
Language now has its own axis. Auditing the existing 41-character lists for
entries that are really set or language tokens is part of the baseline report.

#### Coverage evaluates the subject axis only

`campaign_coverage.go:175-196 characterMatchesInclusion` is deleted, and the
coverage lookup calls a single exported **subject-axis** predicate from
`inventory` — the same one `PurchaseMatchesCampaign` uses for its subject check.

It cannot do more than that, and the spec should not pretend otherwise.
`CampaignsCovering(ctx, character, era string, grade int)`
(`demand/repository.go:52-55`) receives no set name and no spec id, and its query
selects only `id, grade_range, inclusion_list, exclusion_mode`
(`campaign_coverage.go:46-49`). So coverage answers "does any campaign target
this character at this grade", deliberately ignoring language and denied specs.

That is a documented reduction, not parity. It is the right one: the niche
leaderboard asks a character-level question about characters, where language and
individual card denials are not part of the input and would have no defined
value. The existing method comments already document `era` as an
accepted-and-ignored parameter for the same reason (`campaign_coverage.go:35`,
`:88`) — this extends that convention rather than inventing one. Widening
`CampaignsCovering` to carry language is deferred until something actually asks a
language-scoped coverage question.

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
legacy columns are still populated rather than empty. The mirror is best-effort,
not an exact round trip — a multi-word subject name such as "Crystal Golem"
joins into the comma-separated string and the old binary's
comma-or-whitespace splitter breaks it back into two independent tokens, each
matching more cards than the original name did. A rollback therefore degrades
to slightly wider matching, not to no matching; that is the intended tradeoff,
and `deriveLegacyMirror` documents it at the call site. A follow-up migration
drops both columns once the new model has run a full cycle. This is called out
explicitly so it is not forgotten — the mirror is transitional, not permanent.

Backfill in `000024`: `subject_filter_mode` is set from `exclusion_mode`, and
`subjects` from `inclusion_list` split on comma-or-whitespace runs with
`id = inventory.LegacyUnreconciledSubjectID` (`-1`) and the token as `name`.
That sentinel is deliberately **not** `0` — `id == 0` already means "operator
typed this name, resolve it by name" (§5), and the two must stay
distinguishable: a propose issued between deploy and the baseline pull would
otherwise re-resolve legacy subjects by name and silently swap live 4xxx/8xxx
portal ids for current-generation 22xxx ids. Push translation refuses outright
any campaign still carrying a `-1` sentinel (`toSubjectRefs`,
`ErrLegacySubjectsUnreconciled`) rather than resolving it. The baseline pull
overwrites sentinel entries with real portal ids on every campaign that carries
a `PSACampaignRequestID`; the five unlinked campaigns keep sentinels and cannot
be pushed until the operator resolves them.

### 8. Conversion path for the 3 remaining CATEGORY campaigns

They are paused, so there is no live money at stake. They are converted by an
ordinary push once the operator sets `TargetLanguages` on each, with one caveat:
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

- a `LanguageMultiSelect` — multiple languages may be checked at once (empty
  selection is the open net), since every live campaign carries both English
  and Japanese;
- a polarity toggle (Target / Exclude);
- a subject list editor — add by name with typeahead against a
  `GET /api/psa/subjects` endpoint, chips showing name with the id in the title
  attribute;
- a read-only denied-specs list, since discovery is deferred.

`GET /api/psa/subjects` is served from `CatalogStore` (§2a), not from a live
portal call. The main server has no portal session, so this endpoint is only
possible because the harvester persists the catalog. It returns the subject list
plus `fetchedAt`; the form shows a staleness warning when that is older than 7
days, and an explicit empty state — "subject catalog not yet harvested, run the
harvester" — rather than an empty typeahead that reads as "no such character".

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
| `TargetLanguages` empty on create | Translation fails with a named error. Never create an untargeted campaign. |
| Language token has no matching curated list | Push fails. Never push an empty `prepackagedSpecListIds` — it would widen the campaign to PSA's whole catalog. |
| Catalog missing or older than 7 days | Translation fails closed (§2a), naming the stale catalog kind. Never translate against guessed identifiers. |
| Subject name absent from the persisted catalog | Translation fails, naming the subject. Never silently drop. |
| Edit-form fetch fails for one campaign during baseline | `TargetingComplete` is false; that campaign is skipped, listed in the report, and the run exits non-zero (§3). Never write zero targeting over real targeting. |
| `prepackagedSpecListIds` absent from a CATEGORY edit form | Existing `push.go:56` guard fires; report flags the campaign for manual conversion. |
| Transient portal failure during baseline | Existing `drain.go` classification is unchanged; the baseline is idempotent and simply re-runs. |
| Set name has no recognized language marker | `cardutil.SetLanguage` returns `""`; a language-constrained campaign does not match, and the miss is logged (§6). |
| Denied spec with neither a spec id nor a card number | Not denied. Fail-open is deliberate here — see §6. |

---

## Testing

- **Translators** (`psacampaign/mapper_test.go`): table-driven over the three
  axes — create with a single language, create with the multi-language set
  every live campaign carries, diff with reordered subject lists asserting
  no spurious change, diff with an actual subject change, empty-language-set
  error. Driven by a stub `Resolver`, since the translators no longer touch storage:
  one case per resolver failure (unknown language token, unknown subject) assert
  the error names the offending token.
- **Catalog store** (`postgres/psa_portal_catalog_test.go`): round-trip save and
  read, upsert on `(kind, key)` replacing rather than duplicating, and
  `fetchedAt` older than 7 days surfacing to the caller so translation can fail
  closed.
- **Decode** (`psaportal/campaigns_decode_test.go`): a SPEC_LIST fixture and a
  CATEGORY fixture, asserting `PrepackagedSpecListIDs` and `DeniedSpecs`
  survive to `PortalCampaign`, plus the `prepackagedSpecLists` catalog being
  extracted from the edit-page root rather than discarded.
- **Baseline completeness** (`psaportal/campaigns_test.go` +
  `cmd/psa-harvest`): a fixture where one campaign's edit fetch fails asserts
  `TargetingComplete == false` on that campaign only, that the baseline writes
  no campaign row for it, and that the run exits non-zero. This is the
  regression test for the bug that motivated the guard — a passing baseline that
  silently erased targeting.
- **Language classification** (`cardutil/normalize_sets_test.go`): the real
  certs already in `normalization_chain_test.go:134-145` — including
  `2025 POKEMON SIMPLIFIED CHINESE CBB1 …` — must classify as `chinese`, not
  `english`. Plus Japanese-prefixed, Korean-prefixed, plain English, and an
  unmarked-oddity case returning `""`.
- **Matching** (`inventory/matching_test.go`): language axis using
  `SetLanguage`; Target vs Exclude polarity; empty subject list as
  no-constraint; denied spec by `PSASpecID` overriding a subject match; denied
  spec by set+number fallback when `PSASpecID == 0`; and the fail-open case
  where neither identity is present.
- **Coverage semantics**: `campaign_coverage` and `matching` agree *on the
  subject axis*, because they share one predicate — a test asserting so guards
  the regression the deleted duplicate caused. A second test pins the documented
  reduction: a language-constrained campaign is still returned by
  `CampaignsCovering`, because coverage does not evaluate language (§6). That
  test exists so the reduction is a decision on record, not a latent surprise.
- **Migration**: up/down against a fixture with both an inclusion-mode and an
  exclusion-mode campaign, asserting the derived mirror round-trips.
- **Mocks**: extend `internal/testutil/mocks` per
  `internal/testutil/mocks/README.md` — including a `CatalogStore` mock; no
  inline mocks.
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
   approved. The run must exit zero — a non-zero exit means at least one
   campaign's targeting could not be read, and the baseline is not a faithful
   copy until it is re-run clean.
4. Confirm the push queue is empty and no `updateCampaign` call was made during
   the baseline — portal-side campaign `updatedAt` unchanged for all 9.
5. Confirm the catalog tables are populated after the harvest and that
   `GET /api/psa/subjects` returns from the main server with no portal session.
6. Run the distinct-`set_name` query from §6 against production and confirm no
   unmarked non-English sets exist, before relying on the English default.
7. `cd web && npm run build && npm test`.

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
- **A stale catalog silently degrades to a hard stop.** Translation fails closed
  after 7 days, which is safe but noisy: if the harvester stops running, every
  push starts failing with a staleness error rather than a "harvester is down"
  error. The staleness message names the catalog kind and its `fetchedAt` so the
  real cause is one line away, but this is the most likely source of a confusing
  outage.
- **The English default is an unverified assumption** until the distinct-set
  query in §6 is run. It is the one item in this design that could be wrong in a
  way that changes which cards match which campaign, and it is cheap to check —
  so it is a verification step, not a build-time discovery.
