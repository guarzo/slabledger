# PSA Multi-Language Targeting Axis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change a campaign's curated-spec-list targeting from one language to a set of languages, so the one-time baseline pull can run and pushes stop narrowing what live campaigns buy.

**Architecture:** `inventory.Campaign.TargetLanguage string` becomes `TargetLanguages []string`, an unordered set of stable tokens. The change propagates through validation, Postgres persistence, card matching, push translation, the harvester's baseline pull, and the React form. Migration `000023_campaign_targeting_axes` is rewritten in place rather than superseded, because it has never been applied outside dev.

**Tech Stack:** Go 1.26 (hexagonal), Postgres via pgx stdlib + golang-migrate, React/TypeScript + Vite + vitest 4.

## Why this plan exists

The completed `psa-spec-list-targeting` branch models the curated-spec-list axis as ONE language token. The live portal contradicts that: **all six active campaigns carry BOTH "English Pokemon" and "Japanese Pokemon."** Two verified consequences, each independently branch-breaking:

1. `baselineLanguage` returns `errAmbiguousSpecListName` when two recognized names are present, so the one-time baseline pull skips **100%** of live campaigns and exits non-zero. The migration cannot run at all.
2. `TranslateToDiff` REPLACES the whole `prepackagedSpecListIds` field from the single token, so the first push would **drop one curated list from every live campaign** — changing what six money-spending campaigns buy.

This is real money. Six live campaigns automatically buy graded cards.

## Global Constraints

Every task's requirements implicitly include this section.

### The two language token sets are different. Do not merge them.

This distinction has already caused one false-positive finding during planning. Get it right:

- **Card-classification tokens — FOUR:** `"english"`, `"japanese"`, `"chinese"`, `"korean"` (`internal/platform/cardutil/normalize_sets.go:360-363`). `cardutil.ClassifyLanguage` sorts *cards* into these by set name. Chinese and Korean cards exist and are matched today by open-net campaigns. **This set is not changing and is not drift. Do not "fix" it.**
- **Targeting tokens — TWO plus empty:** `""` (open net), `"english"`, `"japanese"` (`internal/domain/inventory/validation.go:39-50`). These are the curated spec lists the PSA portal actually offers. There is **no `chinese` targeting token** and none is added by this plan (locked decision 5, YAGNI) — the portal has no such list and its exact name is unknown. Its eventual arrival is a loud, self-explanatory failure by design.

### Binding rules

- **Hexagonal:** `internal/domain/**` must never import `internal/adapters/**`. Inventory sub-packages are flat siblings with no cross-imports. Enforced by `scripts/check-imports.sh`.
- **Real import cycle:** `psacampaign/mapper.go` imports `inventory`, so `inventory` can never import `psacampaign`. The *targeting* closed set is therefore deliberately DUPLICATED across four files — `internal/domain/inventory/validation.go`, `internal/domain/psacampaign/resolver.go`, `cmd/psa-harvest/baseline.go`, `web/src/react/utils/campaignConstants.ts`. All four must stay in sync; any task touching one says so explicitly. Note `baseline.go` switches on portal *display names* (`"Japanese Pokemon"`), not tokens, so a plain grep for `japanese` will not find all four.
- **Matching semantics:** empty set = open net (buys any language); non-empty = the card's classified language must be a member.
- **Fail loud in BOTH directions** (locked decision 4). Baseline: a portal campaign carrying a curated list SlabLedger does not model is refused with an error naming the list — never silently dropped. Push: if the resolver cannot map every token, error rather than pushing a narrowed list. A partial list is worse than no push.
- **Process boundary:** translation runs in the main HTTP server, which has no portal session. All portal I/O lives in `cmd/psa-harvest`.
- **Portal-issued ids are copied verbatim, never re-derived from names.** Live ids span 4xxx/8xxx/22xxx generations that `getSubjects` cannot reproduce; re-resolving would silently rewrite ids on active campaigns.
- **Two distinct subject id markers.** `id 0` = operator typed this name, resolve it by name at push time — an intended, working feature (`SubjectListEditor.tsx:73` creates them, `mapper.go` resolves them). `inventory.LegacyUnreconciledSubjectID` (`-1`) = migration 000023's legacy backfill, never reconciled with the portal. Translation REFUSES a campaign carrying `-1` and tells the operator to run the baseline pull; the `id 0` path keeps working untouched.
- **Money:** cents internally, whole USD on the portal wire.
- **Testing:** table-driven with `[]struct`; mocks only from `internal/testutil/mocks/` (Fn-field pattern, never inline); sentinel errors asserted with `errors.Is`. Run `go test -race` before every commit.
- **Type sync:** TS types in `web/src/types/` are hand-maintained to mirror Go struct JSON tags. The Go tags are authoritative.
- **File size** (`scripts/check-file-size.sh`): warn at 500 lines, fail at 600. Excludes `_test.go` and mocks.
- **Commits** go through `.githooks/pre-commit` (`go vet ./...`). Never `--no-verify`.

### Verification gates

Go: `go build ./...`, `go test -race -timeout 10m ./...`, `make check`.

Postgres: **`make test-postgres`** is the real gate. Plain `go test ./internal/adapters/storage/postgres/...` SKIPS every DB test — `requireTestDB` skips unless `POSTGRES_TEST_URL` is set — and returns in ~0.01s. A green plain `go test` for that package proves only that it compiles.

Frontend: **`cd web && npm run typecheck && npm run build && npm test`.** The `typecheck` step is mandatory and must not be dropped. `npm run build` is `vite build`, which uses esbuild: it strips types without checking them and passes on a tree full of type errors. Type checking exists only in the separate `tsc --noEmit` script.

---

# Multi-Language Targeting Axis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-token `inventory.Campaign.TargetLanguage string` with an unordered set `TargetLanguages []string`, so the six live PSA campaigns — every one of which carries BOTH "English Pokemon" and "Japanese Pokemon" curated spec lists — can be baselined and pushed without the one-time baseline pull refusing them as ambiguous, and without the first push silently dropping one curated list from a money-spending campaign.

**Architecture:** The language axis stays a set of stable internal tokens (`"english"`, `"japanese"`), never raw PSA UUIDs, persisted as a JSONB array on `campaigns`. `inventory` owns the closed set and normalization; `psacampaign` resolves the token set to a *union* of portal spec-list ids at translation time; `cmd/psa-harvest` collects the token set from the portal's curated-list names during the baseline pull. Unrecognized curated lists fail loudly in both directions rather than being dropped.

**Tech Stack:** Go 1.26, hexagonal architecture, Postgres via `jackc/pgx/v5/stdlib`, `golang-migrate/migrate/v4` with embedded FS, Playwright/Chromium (harvester only), React + TypeScript + Vite.

## Global Constraints

- **This is real money.** Six live campaigns automatically buy graded cards from the portal. Any code path that narrows a campaign's curated spec lists — including a temporary "take the first token" shim — is the exact production defect this plan exists to prevent. Union, or a loud error. Never a pick.
- **Matching semantics:** an empty `TargetLanguages` set is an open net (the campaign buys any language); a non-empty set matches when the card's classified language is a member.
- **Two different closed sets — never conflate them.** `cardutil` CLASSIFIES cards into four languages (`normalize_sets.go:360-363`, including `LangChinese` and `LangKorean`); the TARGETING axis ACCEPTS only `"english"` and `"japanese"`, because those are the only curated spec lists the portal offers. A Chinese-classified card already exists in inventory and is matched by an open-net campaign. What does not exist is a Chinese curated spec list. `docs/plans/2026-08-06-psa-spec-list-targeting.md:26` conflates the two; it is a completed-branch artifact and is not authoritative here.
- **Canonical targeting tokens:** `"english"` and `"japanese"` only. **No `chinese` token now** — the portal has no such curated list and its exact name is unknown. Its eventual arrival must surface as a loud, self-explanatory failure, not a silent skip.
- **Unrecognized curated spec lists fail loudly in BOTH directions.** Baseline: a portal campaign carrying a curated-list name SlabLedger does not model is refused — skipped with an error naming the list. Push: if the resolver cannot map every token, error rather than push a narrowed list.
- **The language closed-set is deliberately duplicated in four places** and all four must stay in sync: `internal/domain/inventory/validation.go` (`validTargetLanguages`), `internal/domain/psacampaign/resolver.go:51-54` (`languageListNames`), `cmd/psa-harvest/baseline.go` (the `switch` in `baselineLanguage`), and `web/src/react/utils/campaignConstants.ts:73` (`targetLanguageOptions`). The duplication is forced by a real import cycle: `psacampaign/mapper.go` imports `inventory`, so `inventory` cannot import `psacampaign`. Every task touching one of the four says so.
- **Migration 000023 is rewritten in place**, not stacked. This branch is undeployed and 000023 has never been applied outside dev. Dev databases need a reset (`make test-postgres` uses a throwaway database and is unaffected).
- **Legacy subjects carry an unreconciled marker.** Migration 000023's backfill marks subjects derived from the legacy `inclusion_list` string with `inventory.LegacyUnreconciledSubjectID` (`-1`), never `0`. `id 0` already means "operator typed this name, resolve it by name" (`web/src/react/ui/SubjectListEditor.tsx:73` creates them; `internal/domain/psacampaign/mapper.go:125-139` resolves them deliberately). Translation must refuse a campaign containing unreconciled entries; the genuine `id 0` path keeps working.
- **Process boundary is real.** Translation runs in the main HTTP server (no portal session); all portal I/O lives in `cmd/psa-harvest`. A translator never calls the portal.
- **Portal-issued ids are copied verbatim, never re-derived from names.**
- **Hexagonal invariants** (`scripts/check-imports.sh`): `internal/domain/**` must not import `internal/adapters/**`; the flat-sibling rule covers arbitrage, portfolio, tuning, finance, export, dhlisting only. `internal/domain/**` may import `internal/platform/**`.
- **File size** (`scripts/check-file-size.sh`): warn at 500 lines, fail at 600. Excludes `_test.go` and mocks.
- **Money:** cents internally, USD at the API boundary. Structured logging via `observability`. Mocks only from `internal/testutil/mocks/` (Fn-field pattern), never inline.
- **Type sync:** TS types in `web/src/types/` are hand-maintained to mirror Go struct JSON tags. The Go tags are authoritative.
- **Every commit goes through `.githooks/pre-commit`, which runs `go vet ./...` over the WHOLE tree.** `--no-verify` is forbidden. This is a hard scheduling constraint on a cross-cutting field rename: see "Commit coupling" below.

### Commit coupling (Tasks 1 and 2)

Renaming `TargetLanguage string` → `TargetLanguages []string` breaks compilation in four packages at once. Because the pre-commit hook vets the entire tree, **no task may commit while the tree does not build.**

Task 1 therefore includes a *mechanical sweep* of every non-Postgres call site (with the exact edits spelled out, behavior-preserving, never narrowing), and **Task 1 does not commit**: it ends with `git add -A` and hands the staged tree to Task 2. Task 2 completes the Postgres adapter — the last package that does not build — and makes the single commit covering both tasks. Task 2 must run on top of Task 1's tree, not in a fresh checkout.

---

### Task 1: Data model + migration + validation

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (`Campaign.TargetLanguage` at `:202-208`; new `LegacyUnreconciledSubjectID` const beside `TargetSubject` at `:161-168`)
- Modify: `internal/domain/inventory/validation.go` (`ErrInvalidTargetLanguage` at `:35`, `validTargetLanguages` at `:39-50`, normalization at `:114-121`)
- Modify: `internal/domain/inventory/matching.go` (`LanguageAxisMatches` at `:113-120`, call site at `:98`)
- Modify: `internal/domain/psacampaign/resolver.go` (`Resolver.SpecListIDs` at `:43`, `catalogResolver.SpecListIDs` at `:73-96`) — mechanical widening, hardened by Task 4
- Modify: `internal/testutil/mocks/psa_resolver.go` (`SpecListIDsFn` at `:12`, method at `:18-23`) — mechanical widening
- Modify: `internal/domain/psacampaign/mapper.go` (`TranslateToDiff` spec-list axis at `:79-90`, `TranslateToCreate` at `:163-174`) — mechanical, hardened by Task 4
- Modify: `cmd/psa-harvest/baseline.go` (`:112`, `:189`) — mechanical, replaced by Task 5
- Modify: `internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.up.sql` (rewritten in place)
- Modify: `internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.down.sql` (rewritten in place)
- Test: `internal/domain/inventory/validation_test.go` (replace the `TargetLanguage` cases at `:110-140` and `TestValidateAndNormalizeCampaign_LowercasesTargetLanguage` at `:179-192`)
- Test (mechanical literal sweep only): `internal/domain/inventory/matching_test.go:114,122`; `internal/domain/psacampaign/mapper_test.go:20-26,87,231,237,277,395,441,456,484`; `internal/domain/psacampaign/resolver_test.go:50,56-59,66,95`; `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go:48,118,335,464`; `cmd/psa-harvest/baseline_test.go:154,172,199`

**Interfaces:**
- Consumes: `cardutil.SetLanguage(setName string) string` and `cardutil.LangEnglish`/`LangJapanese` (`internal/platform/cardutil/normalize_sets.go`, already exist).
- Produces:
  - `inventory.Campaign.TargetLanguages []string` with JSON tag `targetLanguages` (replaces `TargetLanguage string` / `targetLanguage`).
  - `const inventory.LegacyUnreconciledSubjectID = -1`.
  - `var inventory.ErrInvalidTargetLanguages error` (replaces `ErrInvalidTargetLanguage`; nothing outside `validation.go`/`validation_test.go` referenced the old name — verified by grep).
  - `func inventory.ValidateAndNormalizeCampaign(c *Campaign) error` — unchanged signature; now also validates, dedupes, sorts and non-nils `TargetLanguages`.
  - `func inventory.LanguageAxisMatches(setName string, targetLanguages []string) bool` — signature changed from `(setName, targetLanguage string)`. **This is the final signature; Task 3 must not redefine it**, only build its consumers and further tests on it.
  - `psacampaign.Resolver.SpecListIDs(languageTokens []string) ([]string, error)` — the interface method itself is widened from `(languageToken string)`. Union of every token's enabled curated-list ids; an unresolvable token returns `ErrUnknownSpecList` and no partial list. Widened here only so the tree compiles; **Task 4 owns hardening it** (dedup ordering, error wording that names every unresolvable token at once, unreconciled-subject refusal, and its own tests). Both existing implementations — `catalogResolver` (`resolver.go`) and `mocks.ResolverMock` — plus the inline `stubResolver` in `mapper_test.go:20` are widened with it.
- Does NOT produce: any change to the Postgres adapter (Task 2), to `demand.ActiveCampaign` (Task 2), to matching's downstream consumers (Task 3), to the frontend (Task 6).

**Marker value justification.** `-1` is the marker because PSA-issued subject ids are always positive and `0` is already load-bearing as "operator-typed, resolve by name". A negative id therefore cannot collide with either. Both storage layers tolerate it: JSON (and therefore `jsonb`) numbers are signed, and Go's `int` is signed — `TargetSubject.ID` is `int` (`types_core.go:166`), so `-1` round-trips through `json.Marshal`/`Unmarshal` and through the `subjects JSONB` column without special handling.

**Validation does NOT reject unreconciled subjects.** If it did, the baseline-pull write path and every ordinary campaign update would fail on exactly the campaigns that need baselining. The refusal belongs at translation time (Task 4), where the operator's action — "run the baseline pull first" — is the meaningful remedy.

**Migration audit (verified).** `000024_psa_portal_catalog` creates the `psa_portal_catalog` table and does not reference `target_language`; it is the highest-numbered migration. No migration other than 000023 mentions the column, so nothing downstream needs changing. The Go references that DO exist (`campaign_store.go:104,113,127,137,163,197,265,273`; `campaign_coverage.go:143,160,177`) belong to Task 2, and the doc references (`docs/SCHEMA.md:319`, `docs/psa-harvester.md:302,329,335,347,363`) belong to Task 7.

**File-size decision.** `internal/domain/inventory/types_core.go` is 538 lines and this task adds roughly 10, landing near 548 — past the 500-line warning but well under the 600-line hard fail. The recommended pure-move split along type families is deliberately NOT folded in here: it would rewrite the diff of a money-sensitive change into a mostly-cosmetic one. Recorded as a follow-up for Task 7's docs pass.

- [ ] **Step 1: Write the failing test**

Replace the `TargetLanguage` cases at `internal/domain/inventory/validation_test.go:110-140` with these cases (leave the `SubjectFilterMode` cases at `:141-166` untouched):

```go
		// TargetLanguages validation
		{
			name:    "nil target languages is valid (open net)",
			c:       Campaign{Name: "Test", TargetLanguages: nil},
			wantErr: nil,
		},
		{
			name:    "empty target languages is valid (open net)",
			c:       Campaign{Name: "Test", TargetLanguages: []string{}},
			wantErr: nil,
		},
		{
			name:    "single english token is valid",
			c:       Campaign{Name: "Test", TargetLanguages: []string{"english"}},
			wantErr: nil,
		},
		{
			name:    "both live tokens are valid — every live campaign carries both",
			c:       Campaign{Name: "Test", TargetLanguages: []string{"japanese", "english"}},
			wantErr: nil,
		},
		{
			name:    "wrong case is normalized, not rejected",
			c:       Campaign{Name: "Test", TargetLanguages: []string{"Japanese", " ENGLISH "}},
			wantErr: nil, // normalized before the closed-set check — see the assertion below
		},
		{
			name:    "chinese is rejected — no curated portal spec list exists for it",
			c:       Campaign{Name: "Test", TargetLanguages: []string{"chinese"}},
			wantErr: ErrInvalidTargetLanguages,
		},
		{
			name:    "one bad token poisons the whole set — no partial acceptance",
			c:       Campaign{Name: "Test", TargetLanguages: []string{"english", "klingon"}},
			wantErr: ErrInvalidTargetLanguages,
		},
```

Then replace `TestValidateAndNormalizeCampaign_LowercasesTargetLanguage` (`:179-192`) with:

```go
// TestValidateAndNormalizeCampaign_NormalizesTargetLanguages pins the stored
// value, not just whether validation errors: LanguageAxisMatches (matching.go)
// compares members with `==` against cardutil's lowercase tokens and performs
// no casing normalization of its own, so a campaign stored with "Japanese"
// would silently match zero cards forever. The sort is not cosmetic either —
// the set is unordered, and a stable order keeps Postgres round-trips,
// portal diffs and test assertions deterministic.
func TestValidateAndNormalizeCampaign_NormalizesTargetLanguages(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "lowercases and trims",
			in:   []string{" Japanese ", "ENGLISH"},
			want: []string{"english", "japanese"},
		},
		{
			name: "sorts into a stable order",
			in:   []string{"japanese", "english"},
			want: []string{"english", "japanese"},
		},
		{
			name: "dedupes after normalization",
			in:   []string{"english", "English", " english"},
			want: []string{"english"},
		},
		{
			name: "drops empty and whitespace-only entries",
			in:   []string{"english", "", "   "},
			want: []string{"english"},
		},
		{
			name: "nil becomes a non-nil empty slice",
			in:   nil,
			want: []string{},
		},
		{
			name: "an all-empty set collapses to the open net",
			in:   []string{"", "  "},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Campaign{Name: "Test", TargetLanguages: tt.in}
			if err := ValidateAndNormalizeCampaign(&c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.TargetLanguages == nil {
				t.Fatal("TargetLanguages is nil; must be a non-nil empty slice so it marshals to [] not null")
			}
			if !slices.Equal(c.TargetLanguages, tt.want) {
				t.Errorf("TargetLanguages = %#v, want %#v", c.TargetLanguages, tt.want)
			}
		})
	}
}
```

Add `"slices"` to `validation_test.go`'s import block.

Then replace the language cases in `internal/domain/inventory/matching_test.go` — at `:114` and `:122` the field literal `TargetLanguage: "japanese",` becomes `TargetLanguages: []string{"japanese"},` — and append this new test to that file:

```go
// TestLanguageAxisMatches_Set pins the set semantics that make the live fleet
// workable: every active campaign carries BOTH curated lists, so a two-token
// set must match cards of either language, and an empty set stays an open net.
func TestLanguageAxisMatches_Set(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		langs   []string
		want    bool
	}{
		{name: "empty set is an open net", setName: "JAPANESE SV4a-SHINY TREASURE ex", langs: nil, want: true},
		{name: "empty non-nil set is an open net", setName: "CELEBRATIONS CLASSIC COLLECTION", langs: []string{}, want: true},
		{name: "single token matches", setName: "JAPANESE M1S-MEGA SYMPHONIA", langs: []string{"japanese"}, want: true},
		{name: "single token rejects other language", setName: "SWSH BLACK STAR PROMO", langs: []string{"japanese"}, want: false},
		{name: "both tokens match japanese", setName: "JAPANESE M1S-MEGA SYMPHONIA", langs: []string{"english", "japanese"}, want: true},
		{name: "both tokens match english", setName: "SWSH BLACK STAR PROMO", langs: []string{"english", "japanese"}, want: true},
		{name: "both tokens still reject chinese", setName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", langs: []string{"english", "japanese"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LanguageAxisMatches(tt.setName, tt.langs); got != tt.want {
				t.Errorf("LanguageAxisMatches(%q, %#v) = %v, want %v", tt.setName, tt.langs, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test (expect failure)**

Run: `go test ./internal/domain/inventory/... -run 'TestValidateAndNormalizeCampaign|TestLanguageAxisMatches_Set'`

Expected: FAIL to compile — `unknown field TargetLanguages in struct literal of type Campaign`, `undefined: ErrInvalidTargetLanguages`, and `too many arguments in call to LanguageAxisMatches`.

- [ ] **Step 3: Implement the model, the marker, and validation**

In `internal/domain/inventory/types_core.go`, add the marker constant directly after the `TargetSubject` type (`:161-168`):

```go
// LegacyUnreconciledSubjectID marks a subject that migration 000023 backfilled
// from the legacy inclusion_list string: a name with no portal id behind it
// and no reconciliation against live portal state yet.
//
// It exists because id 0 is already taken. An operator who types a new
// subject name in the UI creates it with id 0 (SubjectListEditor.tsx), and
// TranslateToCreate/TranslateToDiff deliberately resolve those by name. If
// backfilled legacy subjects also carried 0, a push issued between deploy and
// the baseline pull would re-resolve them by name and swap the live 4xxx/8xxx
// portal ids on six money-spending campaigns for current-generation 22xxx
// ids. -1 cannot collide with either case: portal-issued ids are positive.
const LegacyUnreconciledSubjectID = -1
```

Replace the `TargetLanguage` field (`:202-208`) with:

```go
	// TargetLanguages is the set of PSA curated spec lists the campaign buys
	// from, held as stable internal tokens rather than portal UUIDs (which PSA
	// can re-issue). It is an unordered set; ValidateAndNormalizeCampaign
	// (validation.go) sorts it so persistence and diffs stay deterministic.
	//
	// Empty means an open net: the campaign buys any language. Every live
	// campaign carries BOTH "english" and "japanese" — the single-token model
	// this replaced could not represent them.
	//
	// The closed set is "english" | "japanese" only. cardutil.SetLanguage
	// classifies chinese and korean sets too, but the portal offers no curated
	// spec list for either, so those tokens are rejected rather than stored
	// unmatchable.
	TargetLanguages []string `json:"targetLanguages"`
```

In `internal/domain/inventory/validation.go`, replace `ErrInvalidTargetLanguage` (`:35`):

```go
	ErrInvalidTargetLanguages   = errors.NewAppError(ErrCodeCampaignValidation, "targetLanguages entries must be 'english' or 'japanese'")
```

Replace `validTargetLanguages` (`:39-50`):

```go
// validTargetLanguages is the closed set ValidateAndNormalizeCampaign accepts
// as members of TargetLanguages. Unlike the single-token model it replaces,
// "" is NOT a member — an open net is the empty SET, not a set containing an
// empty string, so normalizeTargetLanguages drops empty entries before this
// check rather than accepting them.
//
// This intentionally duplicates (rather than imports) psacampaign's
// languageListNames map (internal/domain/psacampaign/resolver.go:51-54) —
// psacampaign already imports inventory to build ProposedDiffs, so the reverse
// import would be a cycle. The same set is also duplicated in
// cmd/psa-harvest/baseline.go and web/src/react/utils/campaignConstants.ts.
// All four must stay in sync.
var validTargetLanguages = map[string]bool{
	"english":  true,
	"japanese": true,
}
```

Replace the normalization block (`:114-121`) with a call to a new helper:

```go
	langs, err := normalizeTargetLanguages(c.TargetLanguages)
	if err != nil {
		return err
	}
	c.TargetLanguages = langs
```

And add the helper next to `validateRange`:

```go
// normalizeTargetLanguages lowercases and trims every entry, drops empties,
// rejects anything outside the closed set, dedupes, and sorts. It always
// returns a non-nil slice: a nil TargetLanguages marshals to JSON null, and
// the TS Campaign type declares the field non-nullable.
//
// Lowercasing is not cosmetic. LanguageAxisMatches compares members with `==`
// against cardutil's lowercase tokens and normalizes nothing itself, so a
// campaign stored with "Japanese" would silently match zero cards forever.
// Rejection is all-or-nothing: a set containing one unknown token is a
// mistake to surface, not a set to silently narrow.
func normalizeTargetLanguages(in []string) ([]string, error) {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		if !validTargetLanguages[token] {
			return nil, ErrInvalidTargetLanguages
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	sort.Strings(out)
	return out, nil
}
```

Add `"sort"` to `validation.go`'s import block.

In `internal/domain/inventory/matching.go`, replace `LanguageAxisMatches` (`:113-120`):

```go
// LanguageAxisMatches reports whether a set name satisfies the language axis.
// An empty targetLanguages set is an open net and always matches; otherwise
// the set name's classified language must be a member. Members are assumed
// already normalized by ValidateAndNormalizeCampaign — this function does no
// casing normalization of its own.
func LanguageAxisMatches(setName string, targetLanguages []string) bool {
	if len(targetLanguages) == 0 {
		return true
	}
	lang := cardutil.SetLanguage(setName)
	for _, want := range targetLanguages {
		if lang == want {
			return true
		}
	}
	return false
}
```

And update the call site at `:98`:

```go
	if !LanguageAxisMatches(in.SetName, c.TargetLanguages) {
```

- [ ] **Step 4: Run the test (expect pass)**

Run: `go test ./internal/domain/inventory/... -run 'TestValidateAndNormalizeCampaign|TestLanguageAxisMatches'`

Expected: `ok  	github.com/guarzo/slabledger/internal/domain/inventory` — all `TestValidateAndNormalizeCampaign`, `TestValidateAndNormalizeCampaign_NormalizesTargetLanguages`, `TestLanguageAxisMatches` and `TestLanguageAxisMatches_Set` subtests pass.

- [ ] **Step 5: Rewrite migration 000023 in place**

Replace `internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.up.sql` in full. Everything except the language column and the backfill id is preserved verbatim from the current file:

```sql
ALTER TABLE campaigns
  ADD COLUMN target_languages    JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN subject_filter_mode TEXT  NOT NULL DEFAULT 'Target',
  ADD COLUMN subjects            JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN denied_specs        JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Backfill: subject_filter_mode mirrors the existing polarity bool.
UPDATE campaigns
SET subject_filter_mode = CASE WHEN exclusion_mode THEN 'Exclude' ELSE 'Target' END;

-- Backfill: subjects from inclusion_list, split on comma-or-whitespace runs
-- with empty entries dropped. Ids are the legacy-unreconciled marker (-1,
-- matching inventory.LegacyUnreconciledSubjectID), NOT 0: id 0 already means
-- "operator typed this name, resolve it by name" and psacampaign's
-- translators act on that. Marking these -1 keeps them distinguishable, so
-- translation can refuse the campaign until the operator runs the baseline
-- pull instead of silently swapping live portal ids. See design doc §7.
UPDATE campaigns c
SET subjects = COALESCE(
    (
        SELECT jsonb_agg(jsonb_build_object('id', -1, 'name', tok) ORDER BY ord)
        FROM unnest(regexp_split_to_array(trim(c.inclusion_list), '[,\s]+')) WITH ORDINALITY AS t(tok, ord)
        WHERE tok <> ''
    ),
    '[]'::jsonb
)
WHERE c.inclusion_list IS NOT NULL AND trim(c.inclusion_list) <> '';
```

`target_languages` is intentionally NOT backfilled: no pre-existing column carries language information, and the baseline pull (Task 5) is what seeds it from live portal state. `'[]'` means open net until then, which is the same permissive behavior these rows had before the column existed.

Replace `.../000023_campaign_targeting_axes.down.sql` in full:

```sql
ALTER TABLE campaigns
  DROP COLUMN denied_specs,
  DROP COLUMN subjects,
  DROP COLUMN subject_filter_mode,
  DROP COLUMN target_languages;
```

- [ ] **Step 6: Mechanical sweep — make the rest of the tree compile**

None of the edits below change behavior for a single-token campaign; they exist because the pre-commit hook runs `go vet ./...` over the whole tree. Each names the task that owns its real semantics.

**Union, never a pick.** The widened `SpecListIDs` must resolve *every* token. Resolving only the first would push a narrowed `prepackagedSpecListIds` and change what six money-spending campaigns buy — the precise defect this plan exists to prevent. An unresolvable token errors the whole call for the same reason: a partial list is worse than no push.

In `internal/domain/psacampaign/resolver.go`, widen the interface method (`:43`):

```go
	SpecListIDs(languageTokens []string) ([]string, error)
```

and replace `catalogResolver.SpecListIDs` (`:73-96`) with the loop form. The per-token "no enabled list matched" check stays per-token — hoisting it to the aggregate would let one resolvable token mask another that resolved to nothing:

```go
// SpecListIDs maps a set of language tokens to the union of the portal
// UUID(s) whose Name equals each token's curated list name (case-insensitive)
// and whose Status is "ENABLED". Lists with any other status are skipped even
// when the name matches, since the portal can retire a list without removing
// it from the catalog payload.
//
// Every token must resolve to at least one enabled list; one that does not
// returns ErrUnknownSpecList and no ids at all. A campaign carrying both
// curated lists must push both or push nothing.
//
// Task 4 hardens this: dedup ordering, and an error that names every
// unresolvable token at once rather than stopping at the first.
func (r *catalogResolver) SpecListIDs(languageTokens []string) ([]string, error) {
	out := make([]string, 0, len(languageTokens))
	for _, token := range languageTokens {
		wantName, ok := languageListNames[token]
		if !ok {
			return nil, ErrUnknownSpecList
		}
		matched := 0
		for _, l := range r.specLists {
			if !strings.EqualFold(l.Name, wantName) || l.Status != "ENABLED" {
				continue
			}
			matched++
			if !slices.Contains(out, l.ID) {
				out = append(out, l.ID)
			}
		}
		if matched == 0 {
			return nil, ErrUnknownSpecList
		}
	}
	return out, nil
}
```

Add `"slices"` to `resolver.go`'s import block (it currently imports `strings` and `time`).

In `internal/testutil/mocks/psa_resolver.go`, widen the Fn field (`:12`) and the method (`:18-23`), leaving the zero-value `ErrUnknownSpecList` default and its doc comment intact:

```go
	SpecListIDsFn func(languageTokens []string) ([]string, error)
```

```go
func (m *ResolverMock) SpecListIDs(languageTokens []string) ([]string, error) {
	if m.SpecListIDsFn != nil {
		return m.SpecListIDsFn(languageTokens)
	}
	return nil, psacampaign.ErrUnknownSpecList
}
```

In `internal/domain/psacampaign/mapper.go`, replace the `TranslateToDiff` spec-list block (`:79-90`):

```go
	// An empty TargetLanguages set means this campaign has no spec-list axis
	// to propose yet (legacy/unlinked campaign) — that must not block every
	// other scalar fix in this diff, so the axis is skipped rather than
	// erroring the whole call.
	if len(internal.TargetLanguages) > 0 {
		specListIDs, err := r.SpecListIDs(internal.TargetLanguages)
		if err != nil {
			return d, fmt.Errorf("psacampaign: resolve spec lists for languages %v: %w", internal.TargetLanguages, err)
		}
		addList("prepackagedSpecListIds",
			renderStringList(portal.SpecListIDs), renderStringList(specListIDs), specListIDs)
	}
```

Replace the guard and resolve in `TranslateToCreate` (`:167-174`):

```go
	if len(internal.TargetLanguages) == 0 {
		return fd, fmt.Errorf("psacampaign: campaign has no target languages set")
	}
	specListIDs, err := r.SpecListIDs(internal.TargetLanguages)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: resolve spec lists for languages %v: %w", internal.TargetLanguages, err)
	}
```

In `cmd/psa-harvest/baseline.go`, replace `:112` (Task 5 replaces `baselineLanguage` itself with a multi-name collector):

```go
	updated.TargetLanguages = []string{lang}
```

and `:189`:

```go
			observability.String("targetLanguages", strings.Join(updated.TargetLanguages, ",")))
```

Add `"strings"` to `baseline.go`'s import block (it currently imports `context`, `errors`, `fmt`, `sort`, `strconv` plus the four project packages).

Test-literal sweep — replace the field name and wrap the value in a one-element slice, changing nothing else:

| File | Lines | `TargetLanguage: "x"` → |
| --- | --- | --- |
| `internal/domain/psacampaign/mapper_test.go` | 87, 277, 395, 441 | `TargetLanguages: []string{"english"}` (`:87,395,441`), `[]string{"english"}` (`:277`) |
| `internal/domain/psacampaign/mapper_test.go` | 231, 484 | `c.TargetLanguages = nil` / `TargetLanguages: nil` (the empty-language cases) |
| `internal/domain/psacampaign/mapper_test.go` | 237, 456 | `= []string{"korean"}` (still expects `ErrUnknownSpecList`) |
| `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go` | 48, 335, 464 | `TargetLanguages: []string{"english"}` |
| `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go` | 118 | `c.TargetLanguages = []string{"korean"}` |
| `cmd/psa-harvest/baseline_test.go` | 154, 172 | `TargetLanguages: []string{"japanese"}` / `[]string{"english"}` |
| `cmd/psa-harvest/baseline_test.go` | 199 | `!slices.Equal(got.TargetLanguages, tt.want.TargetLanguages)` (add `"slices"` to the imports) |

Also rename `TestTranslateToDiff_EmptyTargetLanguageSkipsSpecListAxis` (`mapper_test.go:479`) to `TestTranslateToDiff_EmptyTargetLanguagesSkipsSpecListAxis` and update the two message strings inside it to say "TargetLanguages".

Two more sites implement or call `Resolver.SpecListIDs` and must be widened with it. Grep confirms these are the ONLY ones — `grep -rn "SpecListIDs" --include="*.go" .` returns nothing else that is a Resolver implementation or call; every other hit is the unrelated `PortalCampaign.SpecListIDs` / `CampaignFormData.PrepackagedSpecListIDs` struct field.

`internal/domain/psacampaign/mapper_test.go:19-26` — the inline `stubResolver` is a second `Resolver` implementation (it predates this plan; leave its inline-mock status alone, this is not the change to fix that):

```go
func (s stubResolver) SpecListIDs(languageTokens []string) ([]string, error) {
	out := make([]string, 0, len(languageTokens))
	for _, token := range languageTokens {
		ids, ok := s.specLists[token]
		if !ok {
			return nil, ErrUnknownSpecList
		}
		out = append(out, ids...)
	}
	return out, nil
}
```

`internal/domain/psacampaign/resolver_test.go` — in `TestCatalogResolver_SpecListIDs`, rename the table field `languageToken string` (`:50`) to `languageTokens []string`, wrap the four case values (`:56-59`) so `"japanese"` → `[]string{"japanese"}`, `"english"` → `[]string{"english"}`, `"korean"` → `[]string{"korean"}` and `""` → `[]string{""}`, change the call at `:66` to `r.SpecListIDs(tt.languageTokens)`, and change `:95` to `r.SpecListIDs([]string{"japanese"})`. All four expectations stay as they are — the empty-string token is still unknown, and the disabled-only fixture still yields `ErrUnknownSpecList`.

- [ ] **Step 7: Verify the non-Postgres tree**

Run: `go build ./internal/domain/... ./cmd/... ./internal/platform/...`

Expected: no output (success).

Run: `go test -race ./internal/domain/... ./cmd/psa-harvest/...`

Expected: `ok` for every package, including `internal/domain/inventory` and `internal/domain/psacampaign`.

Run: `go build ./...`

Expected: FAIL, and ONLY in the Postgres package — `internal/adapters/storage/postgres/campaign_store.go:113:3: c.TargetLanguage undefined (type inventory.Campaign has no field or method TargetLanguage)` plus the sibling references at `:137`, `:197`, `:273` and `campaign_coverage.go:177`. Any failure outside `internal/adapters/storage/postgres` means the sweep in Step 6 is incomplete — fix it before handing off.

Run: `scripts/check-imports.sh`

Expected: passes (no new imports were added to `internal/domain/**`).

Run: `scripts/check-file-size.sh`

Expected: passes; `internal/domain/inventory/types_core.go` reports as a warning near 548 lines, not a failure.

- [ ] **Step 8: Stage, do not commit**

Run: `git add -A && git status --short`

Expected: staged modifications to the inventory, psacampaign, harvester and migration files listed above, and nothing else.

Do **not** run `git commit`. The tree does not build, so `.githooks/pre-commit`'s `go vet ./...` would fail, and `--no-verify` is forbidden. Task 2 completes the Postgres package and makes the commit covering both tasks.

---

### Task 2: Postgres persistence + read-path guard test

**Files:**
- Modify: `internal/adapters/storage/postgres/campaign_store.go` (`:58-70` `normalizeTargetSubjects`, `:93-119` `CreateCampaign`, `:121-155` `GetCampaign`, `:157-213` `ListCampaigns`, `:253-286` `UpdateCampaign`)
- Modify: `internal/adapters/storage/postgres/campaign_coverage.go` (`:141-186` `ActiveCampaigns`)
- Modify: `internal/domain/demand/repository.go` (delete the dead `ActiveCampaign.TargetLanguage` field at `:32`)
- Test: `internal/adapters/storage/postgres/campaign_store_test.go` (`TestCampaignStore_TargetingAxesRoundTrip` at `:78-176`; new `TestCampaignStore_JSONNullNormalizesToEmpty`)
- Test: `internal/adapters/storage/postgres/campaign_coverage_test.go` (`insertCoverageCampaign` at `:12-22`, `TestCampaignCoverageLookup_ActiveCampaigns` at `:24-40`)

**Prerequisite:** runs on top of Task 1's staged tree. `go build ./...` fails at the start of this task; making it pass is this task's job.

**Interfaces:**
- Consumes (all from Task 1): `inventory.Campaign.TargetLanguages []string` (JSON tag `targetLanguages`); `inventory.TargetSubject{ID int; Name string}`; `inventory.LegacyUnreconciledSubjectID = -1`; the `campaigns.target_languages JSONB NOT NULL DEFAULT '[]'::jsonb` column from the rewritten migration 000023.
- Produces:
  - `demand.ActiveCampaign` with the dead `TargetLanguage` field REMOVED (not renamed). Its only producer was `campaign_coverage.go:177`; it had zero readers.
  - `func postgres.marshalTargetLanguages(langs []string) string` and `func postgres.normalizeTargetLanguages(langs []string) []string` (unexported, `campaign_store.go`).
  - Unchanged public surface otherwise: `inventory.CampaignRepository` and `demand.ActiveCampaignSource` signatures are untouched.
- Cross-package note: `demand.ActiveCampaign.TargetLanguage` has exactly one producer (`campaign_coverage.go:177`) and zero readers — `campaign_signals.go` reads only `ID`, `Name`, `GradeRange` and `Subjects` (verified by grep). It is therefore deleted here, not carried forward. This work lands in THIS task rather than Task 3 because Task 2's gate is `make test-postgres`: Task 1 renames the column to `target_languages`, so `ActiveCampaigns`' hardcoded `SELECT ... target_language` fails with `column "target_language" does not exist` and this task cannot go green until it is fixed.

**Guard decision — keep it, and pin it with a test.** The `null`→`[]` normalization on the read path (`normalizeTargetSubjects`, called at `campaign_store.go:151-152` and `:207-208`) survives this task and gains a test. It is not dead defensive padding: `NOT NULL` on a `jsonb` column forbids SQL `NULL`, not the JSON `null` *value*, so `UPDATE campaigns SET subjects = 'null'::jsonb` is perfectly legal, and this package already contains writers that bypass the store's own marshaller (`campaign_coverage_test.go:16-21` inserts `campaigns` rows by raw SQL). The failure mode it prevents is user-visible and silent — a `null` reaching the API contradicts the non-nullable TS `Campaign` type and crashes `SubjectListEditor`'s `value.map`. The reason it currently reads as dead is simply that no test exercised the raw-SQL path; the fix is the test, not the deletion. Task 1's new `target_languages` column has the identical shape and gets the identical guard, so the test covers all three columns at once.

**The real gate is `make test-postgres`.** Plain `go test ./internal/adapters/storage/postgres/...` SKIPS every DB test — `requireTestDB` (`testhelper_test.go:16-21`) skips unless `POSTGRES_TEST_URL` is set, deliberately, because the package truncates schemas and the devcontainer's `DATABASE_URL` points at the developer's real database. `make test-postgres` (`Makefile:96-108`) provisions a throwaway database and sets the variable. A green plain `go test` for this package proves only that it compiles.

Two things measured in the repo that shape this task:

1. `demand.ActiveCampaign.TargetLanguage` (`repository.go:32`) is genuinely **write-only**. `campaign_coverage.go:177` fills it; `grep -rn "TargetLanguage" internal/domain/demand/` returns only the field declaration itself. `campaign_coverage.go:26-32` already documents *why* — coverage evaluates the subject axis only, because `CampaignsCovering`/`UnsoldCountFor` receive a bare `(character, era, grade)` triple with no set name to classify. So this is the moment to delete the field, not to wire it: keeping it would force a `[]string` scan of a JSONB column purely to feed a value nothing reads.
2. Deleting it is not optional cleanup — it is required for correctness. `ActiveCampaigns` hardcodes `target_language` in its SELECT (`campaign_coverage.go:143`), and Task 1 rewrites migration `000023` so that column is named `target_languages` and typed JSONB. Left alone, this query fails at runtime with `column "target_language" does not exist` on every niche-opportunity leaderboard request, and no unit test would catch it (the Postgres suite skips unless `POSTGRES_TEST_URL` is set — `testhelper_test.go:16-21`).

- [ ] **Step 1: Write the failing test**

In `internal/adapters/storage/postgres/campaign_store_test.go`, update `TestCampaignStore_TargetingAxesRoundTrip`: at `:91` replace `TargetLanguage: "japanese",` with `TargetLanguages: []string{"english", "japanese"},` (both tokens — the live fleet shape), and replace the three read assertions at `:108`, `:156` and `:173`:

```go
	assert.Equal(t, []string{"english", "japanese"}, got.TargetLanguages)
```

```go
	assert.Equal(t, []string{}, got2.TargetLanguages)
```

```go
	assert.Equal(t, []string{}, listed.TargetLanguages)
```

The `got2`/`listed` assertions use the exact empty-slice value rather than `assert.Empty` for the same reason the existing `Subjects` assertions do (see the comment at `:158-160`): a nil slice marshals to JSON `null` over an API whose TS type declares the field non-nullable.

Then append the new guard test:

```go
// TestCampaignStore_JSONNullNormalizesToEmpty pins the read-path guard that
// turns a stored JSON null into an empty slice. NOT NULL on a jsonb column
// forbids SQL NULL, not the JSON null *value*, so the UPDATE below is legal —
// and this package already contains raw-SQL writers that bypass the store's
// marshaller (campaign_coverage_test.go:16-21). Without the guard these
// columns reach the API as null, contradicting the non-nullable TS Campaign
// type and crashing SubjectListEditor's value.map.
//
// Requires a real database: run `make test-postgres`. Plain `go test` skips.
func TestCampaignStore_JSONNullNormalizesToEmpty(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	repo := NewCampaignStore(db.DB, logger)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{
		ID:              "camp-json-null",
		Name:            "JSON Null Row",
		Phase:           inventory.PhaseActive,
		TargetLanguages: []string{"english"},
		Subjects:        []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}},
		DeniedSpecs:     []inventory.TargetSubject{{ID: 4807, Name: "Charizard"}},
		CreatedAt:       now,
		UpdatedAt:       now,
	}))

	_, err := db.ExecContext(ctx,
		`UPDATE campaigns
		 SET subjects = 'null'::jsonb, denied_specs = 'null'::jsonb, target_languages = 'null'::jsonb
		 WHERE id = $1`, "camp-json-null")
	require.NoError(t, err)

	got, err := repo.GetCampaign(ctx, "camp-json-null")
	require.NoError(t, err)
	assert.Equal(t, []inventory.TargetSubject{}, got.Subjects)
	assert.Equal(t, []inventory.TargetSubject{}, got.DeniedSpecs)
	assert.Equal(t, []string{}, got.TargetLanguages)

	list, err := repo.ListCampaigns(ctx, false)
	require.NoError(t, err)
	var listed *inventory.Campaign
	for i := range list {
		if list[i].ID == "camp-json-null" {
			listed = &list[i]
		}
	}
	require.NotNil(t, listed, "camp-json-null must appear in ListCampaigns")
	assert.Equal(t, []inventory.TargetSubject{}, listed.Subjects)
	assert.Equal(t, []inventory.TargetSubject{}, listed.DeniedSpecs)
	assert.Equal(t, []string{}, listed.TargetLanguages)
}

// TestMarshalTargetLanguages_NilEmitsEmptyArray is the DB-free half of the
// same contract, mirroring TestMarshalTargetSubjects_NilEmitsEmptyArray: a nil
// slice must marshal to "[]", never "null". json.Marshal(nil slice) alone
// produces "null", so this asserts the function's own normalization and fails
// immediately without needing POSTGRES_TEST_URL.
func TestMarshalTargetLanguages_NilEmitsEmptyArray(t *testing.T) {
	tests := []struct {
		name  string
		langs []string
		want  string
	}{
		{name: "nil slice", langs: nil, want: "[]"},
		{name: "empty slice", langs: []string{}, want: "[]"},
		{name: "both live tokens", langs: []string{"english", "japanese"}, want: `["english","japanese"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, marshalTargetLanguages(tt.langs))
		})
	}
}
```

Do not touch `campaign_coverage_test.go` here — Step 7 rewrites it, and as a *deletion* rather than a rename. See the Cross-package note above.

- [ ] **Step 2: Run the test (expect failure)**

Run: `go test ./internal/adapters/storage/postgres/... -run 'TestCampaignStore|TestMarshalTarget|TestCampaignCoverageLookup'`

Expected: FAIL to compile — `c.TargetLanguage undefined (type inventory.Campaign has no field or method TargetLanguage)` at `campaign_store.go:113`, plus `undefined: marshalTargetLanguages` and `unknown field TargetLanguages in struct literal of type inventory.Campaign` in the test file.

- [ ] **Step 3: Implement the persistence change**

In `internal/adapters/storage/postgres/campaign_store.go`, add the language helpers directly after `marshalTargetSubjects` (`:82-91`):

```go
// normalizeTargetLanguages converts a nil slice to an empty one, mirroring
// normalizeTargetSubjects. Defensive on read for the same reason: the
// NOT NULL DEFAULT '[]'::jsonb column (migration 000023) rejects SQL NULL but
// not the JSON null value, and any writer bypassing this store — a raw SQL
// fixup, another tool — can leave one behind. A nil slice would reach the API
// as JSON null, which the non-nullable TS Campaign.targetLanguages type and
// the CampaignFormFields language multi-select do not accept.
func normalizeTargetLanguages(langs []string) []string {
	if langs == nil {
		return []string{}
	}
	return langs
}

// marshalTargetLanguages marshals the language set to its JSONB wire form.
// A nil slice is normalized to empty first — json.Marshal(nil) produces the
// literal "null", which the jsonb column stores happily as a JSON null and
// later round-trips back into a nil Go slice.
func marshalTargetLanguages(langs []string) string {
	if langs == nil {
		langs = []string{}
	}
	b, err := json.Marshal(langs)
	if err != nil {
		return "[]"
	}
	return string(b)
}
```

In `CreateCampaign`: add `languagesJSON := marshalTargetLanguages(c.TargetLanguages)` beside the existing `deniedJSON` line (`:97`), change the column list at `:104` from `target_language` to `target_languages`, and replace the `c.TargetLanguage` argument at `:113` with `languagesJSON`.

In `UpdateCampaign`: the same `languagesJSON` line beside `:257`, `target_languages = $17` at `:265`, and `languagesJSON` in place of `c.TargetLanguage` at `:273`.

In `GetCampaign`: change the selected column at `:127` to `target_languages`, declare `languagesJSON` alongside `subjectsJSON, deniedJSON` at `:131`, scan into `&languagesJSON` instead of `&c.TargetLanguage` at `:137`, and add the unmarshal + normalize beside the existing pair:

```go
	if err := json.Unmarshal([]byte(languagesJSON), &c.TargetLanguages); err != nil {
		return nil, fmt.Errorf("unmarshal target languages: %w", err)
	}
```

```go
	c.TargetLanguages = normalizeTargetLanguages(c.TargetLanguages)
```

In `ListCampaigns`: the identical four edits at `:163`, `:191`, `:197` and `:201-208`, with the unmarshal error wrapped as `fmt.Errorf("unmarshal target languages: %w", err)` and returned the same way the sibling unmarshals are.

In `internal/domain/demand/repository.go`, replace the field at `:32`:

```go
	TargetLanguages   []string // Empty means an open net: the campaign buys any language.
```

In `internal/adapters/storage/postgres/campaign_coverage.go`, change the query at `:143` to select `target_languages`, replace the `targetLanguage string` scan variable at `:160` with `targetLanguagesJSON []byte`, scan into `&targetLanguagesJSON`, unmarshal it beside the existing subjects unmarshal, and populate the renamed field:

```go
		var targetLanguages []string
		if err := json.Unmarshal(targetLanguagesJSON, &targetLanguages); err != nil {
			return nil, fmt.Errorf("unmarshal target languages for campaign %s: %w", id, err)
		}
		if targetLanguages == nil {
			targetLanguages = []string{}
		}
```

```go
			TargetLanguages:   targetLanguages,
```

The explicit nil check is the same `null`→`[]` guard as the store's, spelled inline because this file does not share the store's helpers; `ActiveCampaigns` already builds `out := []demand.ActiveCampaign{}` in the same non-nil spirit at `:154`.

- [ ] **Step 4: Run the test (expect pass)**

Run: `go build ./...`

Expected: no output. This is the point where the whole tree compiles again for the first time since Task 1.

Run: `go test ./internal/adapters/storage/postgres/... -run 'TestMarshalTarget' -v`

Expected: `PASS` for `TestMarshalTargetSubjects_NilEmitsEmptyArray` and `TestMarshalTargetLanguages_NilEmitsEmptyArray`. The DB-backed tests report `SKIP: POSTGRES_TEST_URL not set` — that is expected and is exactly why the next command exists.

Run: `make test-postgres`

Expected: `ok  	github.com/guarzo/slabledger/internal/adapters/storage/postgres` with `TestCampaignStore_TargetingAxesRoundTrip`, `TestCampaignStore_JSONNullNormalizesToEmpty` and `TestCampaignCoverageLookup_ActiveCampaigns` all passing against a real database. This run also proves the rewritten migration 000023 applies cleanly, since `setupTestDB` (`testhelper_test.go:34-40`) executes the embedded migrations.

Sanity-check that the guard test is real rather than vacuous: temporarily delete the `normalizeTargetLanguages`/`normalizeTargetSubjects` call pairs in `GetCampaign` and `ListCampaigns`, re-run `make test-postgres`, and confirm `TestCampaignStore_JSONNullNormalizesToEmpty` now FAILS with `expected: []inventory.TargetSubject{} actual: []inventory.TargetSubject(nil)`. Restore the calls. (Before this task, that same deletion left the suite green — that is the gap being closed.)

- [ ] **Step 5: `internal/domain/demand/repository.go:25-35`** — drop the dead field and record why:

```go
// ActiveCampaign describes a single active campaign's targeting rules, used
// by the campaign-signals service to correlate per-campaign market data.
// Kept minimal — only the fields needed to filter characters and grades.
//
// The language axis is deliberately absent: signals are keyed by character and
// grade with no set name to classify, so a language set has no defined meaning
// here. See postgres.CampaignCoverageLookup's type doc for the same reduction
// on the coverage side.
type ActiveCampaign struct {
	ID                string // Campaign primary key (UUID for standard campaigns, "external" for the imported bucket).
	Name              string
	GradeRange        string // e.g. "9-10"; empty means no grade constraint.
	SubjectFilterMode string
	Subjects          []inventory.TargetSubject
}
```

- [ ] **Step 6: `internal/adapters/storage/postgres/campaign_coverage.go:141-181`** — stop selecting the renamed column:

```go
func (l *CampaignCoverageLookup) ActiveCampaigns(ctx context.Context) ([]demand.ActiveCampaign, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT id, name, grade_range, subject_filter_mode, subjects
		 FROM campaigns
		 WHERE phase = $1 AND id <> $2`,
		string(inventory.PhaseActive),
		inventory.ExternalCampaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("query active campaigns: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := []demand.ActiveCampaign{}
	for rows.Next() {
		var (
			id                string
			name              string
			gradeRange        string
			subjectFilterMode string
			subjectsJSON      []byte
		)
		if err := rows.Scan(&id, &name, &gradeRange, &subjectFilterMode, &subjectsJSON); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}

		var subjects []inventory.TargetSubject
		if err := json.Unmarshal(subjectsJSON, &subjects); err != nil {
			return nil, fmt.Errorf("unmarshal subjects for campaign %s: %w", id, err)
		}

		out = append(out, demand.ActiveCampaign{
			ID:                id,
			Name:              name,
			GradeRange:        gradeRange,
			SubjectFilterMode: subjectFilterMode,
			Subjects:          subjects,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaigns: %w", err)
	}
	return out, nil
}
```

Also update the type doc at `campaign_coverage.go:26-32`, which names the old field:

```go
// Coverage evaluates the SUBJECT AXIS ONLY. CampaignsCovering/UnsoldCountFor
// receive a bare (character, era, grade) triple with no set name and no spec
// id, so the language set (TargetLanguages) and card-level denials
// (DeniedSpecs) have no defined value here and are not evaluated. This is a
// documented reduction versus inventory.PurchaseMatchesCampaign, not an
// oversight: the niche-opportunity leaderboard asks a character-level
// question, and widening CampaignsCovering to carry a language input is
// deferred until something actually asks a language-scoped coverage question.
```

- [ ] **Step 7: `internal/adapters/storage/postgres/campaign_coverage_test.go:14-40`** — the helper writes a column that no longer exists. Drop the parameter (it was only ever passed to satisfy the INSERT; no test reads it back except the one assertion):

```go
// insertCoverageCampaign inserts a minimal campaigns row exercising only the
// columns CampaignCoverageLookup reads; every other column keeps its DB default.
func insertCoverageCampaign(t *testing.T, db *DB, id, phase, gradeRange, subjectFilterMode, subjectsJSON string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO campaigns (id, name, phase, grade_range, subject_filter_mode, subjects)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, "Test "+id, phase, gradeRange, subjectFilterMode, subjectsJSON,
	)
	require.NoError(t, err)
}

func TestCampaignCoverageLookup_ActiveCampaigns(t *testing.T) {
	db := setupTestDB(t)
	lookup := NewCampaignCoverageLookup(db.DB)
	ctx := context.Background()

	insertCoverageCampaign(t, db, "active-1", string(inventory.PhaseActive), "9-10", "Target", `[{"id":1,"name":"Pikachu"}]`)
	insertCoverageCampaign(t, db, "pending-1", string(inventory.PhasePending), "9-10", "Target", `[]`)

	got, err := lookup.ActiveCampaigns(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "active-1", got[0].ID)
	assert.Equal(t, "9-10", got[0].GradeRange)
	assert.Equal(t, "Target", got[0].SubjectFilterMode)
	assert.Equal(t, []inventory.TargetSubject{{ID: 1, Name: "Pikachu"}}, got[0].Subjects)
}
```

The remaining five `insertCoverageCampaign` call sites in `TestCampaignCoverageLookup_CampaignsCovering` (`:48`, `:50`, `:54`, `:55`ff) each drop their `""` language argument; none of them asserted on it.

- [ ] **Step 8: Full verification and the joint commit**

Run: `go test -race -timeout 10m ./...`

Expected: `ok` or `no test files` for every package; no `FAIL`.

Run: `make check`

Expected: lint, `scripts/check-imports.sh` and `scripts/check-file-size.sh` all pass. `campaign_store.go` grows to roughly 315 lines and stays well under the warning threshold.

Run: `git add -A && git commit -m "campaigns: model target language as a set, not a single token"`

Expected: `.githooks/pre-commit` runs `go vet ./...` and `golangci-lint run --new-from-rev=HEAD` and both pass; the commit succeeds without `--no-verify`. This single commit carries Task 1 and Task 2 together, per the commit-coupling note in Global Constraints.

**Dev-database note for the operator:** migration 000023 was rewritten in place, so any dev database that already applied the old version has a `target_language TEXT` column and a `schema_migrations` row claiming 000023 is done. Those databases must be reset (drop and re-migrate) — `golang-migrate` will not re-run a version it considers applied. `make test-postgres` is unaffected: it provisions a throwaway database. No deployed environment has applied 000023.

---

---

### Task 3: Matching + downstream consumers

**Files:**
- Modify: `internal/domain/inventory/matching_test.go` (`:110-125` two language cases, `:215-234` `TestLanguageAxisMatches`)
- Modify: `internal/platform/cardutil/normalize_sets.go:357-358` (comment only — it names `inventory.Campaign.TargetLanguage`)

**Interfaces:**
- Consumes from Task 1: `inventory.Campaign.TargetLanguages []string` with JSON tag `targetLanguages`, replacing `TargetLanguage string` at `types_core.go:202-208`. Tokens are lowercase and drawn from the closed set `{"english", "japanese"}`; an empty/nil slice is legal and means "open net".
- Consumes (already exists, unchanged): `cardutil.SetLanguage(setName string) string` at `internal/platform/cardutil/normalize_sets.go:384`, returning exactly one of `cardutil.LangEnglish` / `LangJapanese` / `LangChinese` / `LangKorean` (`normalize_sets.go:359-364`).
- Consumes from Task 1: `inventory.LanguageAxisMatches(setName string, targetLanguages []string) bool`. Task 1 already wrote the final body and updated its only non-test caller (`PurchaseMatchesCampaign` at `matching.go:98`, same file) — it had to, because changing `Campaign.TargetLanguages` breaks that call site and the tree will not compile otherwise. This task does not edit `matching.go`; it builds the test coverage and the downstream consumers on top of the finished signature.

The language closed set is deliberately duplicated in four places (`internal/domain/inventory/validation.go:46-50`, `internal/domain/psacampaign/resolver.go:51-54`, `cmd/psa-harvest/baseline.go`, `web/src/react/utils/campaignConstants.ts`) because `psacampaign` imports `inventory`, making the reverse import a cycle. **This task changes none of them** — it only changes the shape of the value carried, not the vocabulary. Tasks 1, 4, 5, and 6 own those four sites respectively.

**The seven Go test files that reference `TargetLanguage`**, and where each is updated (so no two tasks edit the same file):

| File | Change | Owning task |
|---|---|---|
| `internal/domain/inventory/matching_test.go` | `:110-125` two campaign literals take `TargetLanguages: []string{"japanese"}`; three new both-languages cases; `:215-234` `TestLanguageAxisMatches` table field becomes `targetLanguages []string` with a reversed-order case | **Task 3 (this one)** |
| `internal/adapters/storage/postgres/campaign_coverage_test.go` | drop the `targetLanguage` helper parameter, the `target_language` INSERT column, all six call sites' language argument, and the `:37` assertion | **Task 3 (this one)** |
| `internal/domain/inventory/validation_test.go` | `:110-139` closed-set cases and `:179-191` lowercasing test move to `TargetLanguages []string`, plus a duplicate-token / both-tokens case | Task 1 |
| `internal/adapters/storage/postgres/campaign_store_test.go` | `:91`, `:108`, `:140-156`, `:173` — round-trip a two-element set and the empty-set open net through JSONB | Task 2 |
| `internal/domain/psacampaign/mapper_test.go` | `stubResolver`, `englishResolver`, every `TargetLanguage:` literal, plus new multi-language and sentinel tests | Task 4 |
| `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go` | `:48` `diffCampaign()`, `:118` korean case, plus a new sentinel-refusal 400 case | Task 4 |
| `cmd/psa-harvest/baseline_test.go` | `:154`, `:172`, `:199` — `baselineLanguage` returns a set, and the two-recognized-names case stops being an error | Task 5 |

- [ ] **Step 1: Write the failing test**

Replace the two language cases at `internal/domain/inventory/matching_test.go:110-125` with these five (the last three are new — they pin the case the single-token model could not express):

```go
		{
			name: "language axis rejects mismatched set",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "SWSH BLACK STAR PROMO"},
			campaign: Campaign{
				TargetLanguages: []string{"japanese"},
			},
			want: false,
		},
		{
			name: "language axis accepts matching set",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "JAPANESE M1S-MEGA SYMPHONIA"},
			campaign: Campaign{
				TargetLanguages: []string{"japanese"},
			},
			want: true,
		},
		{
			// The shape of all six live campaigns: BOTH curated spec lists are
			// selected on the portal. The single-token model could not express
			// this, so it rejected half of what these campaigns actually buy.
			name: "both languages selected accepts an english printing",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "SWSH BLACK STAR PROMO"},
			campaign: Campaign{
				TargetLanguages: []string{"english", "japanese"},
			},
			want: true,
		},
		{
			name: "both languages selected accepts a japanese printing",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "JAPANESE M1S-MEGA SYMPHONIA"},
			campaign: Campaign{
				TargetLanguages: []string{"english", "japanese"},
			},
			want: true,
		},
		{
			// A set is still a closed net, not a wildcard: chinese is in neither
			// token, so it must not match even with both tokens selected.
			name: "both languages selected still rejects a chinese printing",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Pikachu", SetName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1"},
			campaign: Campaign{
				TargetLanguages: []string{"english", "japanese"},
			},
			want: false,
		},
```

Replace `TestLanguageAxisMatches` at `matching_test.go:215-234` entirely:

```go
func TestLanguageAxisMatches(t *testing.T) {
	tests := []struct {
		name            string
		setName         string
		targetLanguages []string
		want            bool
	}{
		{"nil set is open net", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", nil, true},
		{"empty set is open net", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", []string{}, true},
		{"japanese set matches japanese-only set", "JAPANESE M1S-MEGA SYMPHONIA", []string{"japanese"}, true},
		{"chinese set does not match japanese-only set", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", []string{"japanese"}, false},
		{"english set matches english-only set", "SWSH BLACK STAR PROMO", []string{"english"}, true},
		{"english set matches a both-languages set", "SWSH BLACK STAR PROMO", []string{"english", "japanese"}, true},
		{"japanese set matches a both-languages set", "JAPANESE M1S-MEGA SYMPHONIA", []string{"english", "japanese"}, true},
		// The set is unordered: reversing the tokens must not change the answer.
		{"membership is order-insensitive", "JAPANESE M1S-MEGA SYMPHONIA", []string{"japanese", "english"}, true},
		{"korean set matches neither token", "KOREAN S1-SWORD SHIELD", []string{"english", "japanese"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LanguageAxisMatches(tt.setName, tt.targetLanguages); got != tt.want {
				t.Errorf("LanguageAxisMatches(%q, %v) = %v, want %v", tt.setName, tt.targetLanguages, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test — it must fail**

```bash
go test ./internal/domain/inventory/ -run 'TestPurchaseMatchesCampaign|TestLanguageAxisMatches' -race
```

Expected output (compile failure — Task 1 has already added the `TargetLanguages` field, so the break is the predicate's signature):

```
# github.com/guarzo/slabledger/internal/domain/inventory [github.com/guarzo/slabledger/internal/domain/inventory.test]
internal/domain/inventory/matching_test.go:229:29: cannot use tt.targetLanguages (variable of type []string) as string value in argument to LanguageAxisMatches
FAIL	github.com/guarzo/slabledger/internal/domain/inventory [build failed]
```

- [ ] **Step 3: Implement `LanguageAxisMatches` as set membership**

Add `"slices"` to the import block at `internal/domain/inventory/matching.go:3-8`:

```go
import (
	"slices"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/platform/cardutil"
)
```

Update the call site at `matching.go:98`:

```go
	if !LanguageAxisMatches(in.SetName, c.TargetLanguages) {
		return false
	}
```

Replace `LanguageAxisMatches` at `matching.go:113-120`:

```go
// LanguageAxisMatches reports whether a set name satisfies the language axis.
// targetLanguages is an unordered SET of canonical tokens. An empty (or nil)
// set is an open net and always matches; a non-empty set matches only when the
// set name's classified language is a member.
//
// The set — rather than the single token this replaces — exists because every
// live portal campaign carries BOTH the "English Pokemon" and "Japanese
// Pokemon" curated spec lists. A single token could only ever describe half of
// what those campaigns buy, so the other half's purchases fell through to
// "unmatched" and were attributed to no campaign.
//
// Membership is a plain == comparison per element: cardutil.SetLanguage always
// returns one of the canonical Lang* tokens, and ValidateAndNormalizeCampaign
// lowercases every stored token, so this function performs no casing
// normalization of its own — exactly as the single-token version did not.
func LanguageAxisMatches(setName string, targetLanguages []string) bool {
	if len(targetLanguages) == 0 {
		return true
	}
	return slices.Contains(targetLanguages, cardutil.SetLanguage(setName))
}
```

Note what is preserved: the short-circuit on an empty axis (unchanged in spirit — `""` became `len(...) == 0`), and the ordering of `PurchaseMatchesCampaign`'s checks (year → grade → price → language → subject → deny). `SpecDenied`'s documented fail-open direction (`matching.go:180-187`) is untouched by this task; it never consulted the language axis.

- [ ] **Step 4: Run the test — it must pass**

```bash
go test ./internal/domain/inventory/ -run 'TestPurchaseMatchesCampaign|TestLanguageAxisMatches' -race
```

Expected output:

```
ok  	github.com/guarzo/slabledger/internal/domain/inventory	0.0XXs
```

- [ ] **Step 5: `internal/platform/cardutil/normalize_sets.go:357-358`** — comment only, no behavior change:

```go
// Language tokens. These are the canonical values stored in the
// inventory.Campaign.TargetLanguages set and matched against set names.
```

- [ ] **Step 6: Run the affected packages**

```bash
go build ./... && go test ./internal/domain/inventory/ ./internal/domain/demand/ ./internal/adapters/storage/postgres/ ./internal/platform/cardutil/ -race
```

Expected output (the Postgres package skips without `POSTGRES_TEST_URL`; compiling it is the point here — that is what proves the SELECT and the struct literal agree):

```
ok  	github.com/guarzo/slabledger/internal/domain/inventory	0.0XXs
ok  	github.com/guarzo/slabledger/internal/domain/demand	0.0XXs
ok  	github.com/guarzo/slabledger/internal/adapters/storage/postgres	0.0XXs [no tests to run]
ok  	github.com/guarzo/slabledger/internal/platform/cardutil	0.0XXs
```

Then run the Postgres suite against a throwaway database, which is the only way `campaign_coverage_test.go` actually executes:

```bash
make test-postgres
```

Expected: `TestCampaignCoverageLookup_ActiveCampaigns` and `TestCampaignCoverageLookup_CampaignsCovering` both pass against the rewritten `000023`.

- [ ] **Step 7: Architecture checks and commit**

```bash
scripts/check-imports.sh && scripts/check-file-size.sh && go vet ./...
```

Expected: both scripts exit 0 (`matching.go` is 249 lines today and this change is roughly net-neutral; nothing new is imported into `internal/domain/**` from `internal/adapters/**` — `slices` is stdlib).

```bash
git add internal/domain/inventory/matching.go internal/domain/inventory/matching_test.go \
        internal/domain/demand/repository.go \
        internal/adapters/storage/postgres/campaign_coverage.go \
        internal/adapters/storage/postgres/campaign_coverage_test.go \
        internal/platform/cardutil/normalize_sets.go
git commit -m "Match the language axis as a set, not a single token"
```

---

### Task 4: Resolver + mapper (push translation) + sentinel refusal

**Files:**
- Modify: `internal/domain/psacampaign/resolver.go` (`:41-45` `Resolver` interface, `:73-97` `SpecListIDs`, import block `:3-8`, error block `:17-21`)
- Modify: `internal/domain/psacampaign/mapper.go` (`:79-90` spec-list axis, `:117-139` `toSubjectRefs`, `:159-174` `TranslateToCreate` doc + guard)
- Modify: `internal/domain/psacampaign/mapper_test.go` (`:15-41` stub resolver, every `TargetLanguage:` literal, `:479-504` renamed test; three new tests)
- Modify: `internal/testutil/mocks/psa_resolver.go:11-23` (`SpecListIDsFn` signature)
- Modify: `internal/adapters/httpserver/handlers/campaigns_psa.go:196-209` (4xx classification for the new sentinel)
- Modify: `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go` (`:38-50` `diffCampaign`, `:112-123` korean case, one new case)

**Interfaces:**
- **Consumes from Task 1 — assumed name and type, flag at assembly if it differs:** `inventory.LegacyUnreconciledSubjectID`, an untyped integer constant with value `-1`, declared in `internal/domain/inventory/types_core.go` next to `TargetSubject` (`:165-168`). It is the marker migration `000023` backfills into `subjects[].id` for legacy inclusion-list tokens, replacing the literal `0` the migration writes today (`000023_campaign_targeting_axes.up.sql`, the `jsonb_build_object('id', 0, 'name', tok)` backfill). This task compares `inventory.TargetSubject.ID` (declared `int`) against it, so any negative integer constant with any name works — only the identifier must match. If Task 1 names it differently, this task's `mapper.go` and its tests are the only places to update.
- Consumes from Task 1: `inventory.Campaign.TargetLanguages []string`.
- Produces: the HARDENED `psacampaign.Resolver.SpecListIDs(languageTokens []string) ([]string, error)`. **Task 1 already widened this signature** (mechanically — a plain loop — because changing `Campaign.TargetLanguages` breaks `mapper.go` compilation immediately, so the slice-valued resolver has to exist by the end of Task 1). This task replaces that minimal body with the all-or-nothing version below: `ErrUnknownSpecList` naming the offending token, the ENABLED-status filter, and its own tests. The interface line and `mocks.ResolverMock` will already read `languageTokens []string` when you open the files — that is expected, not a merge artifact. Do not treat the overlap as work already done: the minimal loop returns a partial list on an unresolvable token, which is the exact defect this task exists to close.
- Produces: `psacampaign.ErrLegacySubjectsUnreconciled`, a stdlib `errors.New` sentinel asserted with `errors.Is`. Consumed by `internal/adapters/httpserver/handlers/campaigns_psa.go` for 4xx classification and by Task 6's publish modal via the response body.

`psacampaign.languageListNames` (`resolver.go:51-54`) keeps exactly its current two entries and its current shape — one token → one curated list name. It is one of the four deliberately duplicated copies of the language closed set (the others: `internal/domain/inventory/validation.go:46-50` in Task 1, `cmd/psa-harvest/baseline.go` in Task 5, `web/src/react/utils/campaignConstants.ts` in Task 6). Per locked decision 5 no `chinese` token is added anywhere; this task's all-or-nothing resolution is what makes a future Chinese list surface as a loud, self-explanatory failure instead of a silent narrowing.

**On the "unset language skips the axis" comment at `mapper.go:79-82`:** the logic still holds, and gains a second reason. Originally it said only "a legacy/unlinked campaign must not block every other scalar fix in this diff". With a set, an empty `TargetLanguages` also means "open net" (locked decision 3), and the portal cannot represent an open net — a `SPEC_LIST` campaign must carry at least one curated list. So proposing anything for an empty set would mean proposing `prepackagedSpecListIds: []`, which *clears every curated list off a live campaign*. Skipping is now the only safe direction, not merely the convenient one. The replacement comment below says so.

- [ ] **Step 1: Write the failing tests**

First update `mapper_test.go`'s local stub to the new interface (`:15-41`), adding `"fmt"` to that file's import block:

```go
type stubResolver struct {
	specLists map[string][]string
	subjects  map[string]int
}

// SpecListIDs mirrors catalogResolver's all-or-nothing contract: one
// unresolvable token fails the whole call, never a partial list.
func (s stubResolver) SpecListIDs(languageTokens []string) ([]string, error) {
	var out []string
	for _, token := range languageTokens {
		ids, ok := s.specLists[token]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownSpecList, token)
		}
		out = append(out, ids...)
	}
	return out, nil
}

func (s stubResolver) SubjectID(name string) (int, error) {
	id, ok := s.subjects[name]
	if !ok {
		return 0, ErrUnknownSubject
	}
	return id, nil
}

func englishResolver() stubResolver {
	return stubResolver{
		specLists: map[string][]string{"english": {"list-en-1"}},
		subjects:  map[string]int{"Pikachu": 90001},
	}
}

// bothLanguagesResolver is the live portal's shape: every active campaign
// carries both curated lists.
func bothLanguagesResolver() stubResolver {
	return stubResolver{
		specLists: map[string][]string{"english": {"list-en-1"}, "japanese": {"list-ja-1"}},
		subjects:  map[string]int{"Pikachu": 90001},
	}
}
```

Then mechanically swap every `TargetLanguage: "english"` literal in that file for `TargetLanguages: []string{"english"}` (`:87`, `:277`, `:395`, `:441`), `internal.TargetLanguage = "korean"` for `internal.TargetLanguages = []string{"korean"}` (`:456`), `c.TargetLanguage = ""` for `c.TargetLanguages = nil` (`:231`), and `c.TargetLanguage = "korean"` for `c.TargetLanguages = []string{"korean"}` (`:237`). Rename `TestTranslateToDiff_EmptyTargetLanguageSkipsSpecListAxis` (`:479`) to `TestTranslateToDiff_EmptyTargetLanguagesSkipsSpecListAxis`, setting `TargetLanguages: nil` at `:484` and keeping every assertion as-is.

Now the three new tests. Append them to `mapper_test.go`:

```go
// TestTranslateToDiff_MultiLanguageSpecLists is the regression this whole
// change exists for. Before it, a campaign carrying both curated lists
// translated to a single-element prepackagedSpecListIds, so the first push
// would have DROPPED one curated list from every live campaign — silently
// changing what six money-spending campaigns buy.
func TestTranslateToDiff_MultiLanguageSpecLists(t *testing.T) {
	r := bothLanguagesResolver()
	base := func() (inventory.Campaign, PortalCampaign) {
		internal := inventory.Campaign{
			BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
			GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
			TargetLanguages: []string{"english", "japanese"}, SubjectFilterMode: "Target",
		}
		portal := PortalCampaign{
			BuyPercentClv: 75, DailyBudgetCents: 400000,
			BuyBox: CampaignBuyBox{
				GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
				PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
			},
			SubjectFilter: CampaignFilter{Type: "Target"},
		}
		return internal, portal
	}

	specListChange := func(t *testing.T, d ProposedDiff) *FieldChange {
		t.Helper()
		for i := range d.Changes {
			if d.Changes[i].Field == "prepackagedSpecListIds" {
				return &d.Changes[i]
			}
		}
		return nil
	}

	t.Run("portal already carries both lists, reversed, produces no diff", func(t *testing.T) {
		internal, portal := base()
		// Reversed relative to the resolver's token order: renderStringList's
		// canonical (sorted) rendering must absorb that for a two-element list.
		portal.SpecListIDs = []string{"list-ja-1", "list-en-1"}
		d, err := TranslateToDiff(internal, portal, r)
		if err != nil {
			t.Fatalf("TranslateToDiff: %v", err)
		}
		if c := specListChange(t, d); c != nil {
			t.Fatalf("unexpected prepackagedSpecListIds change for the same two lists in a different order: %+v", c)
		}
	})

	t.Run("portal missing one list proposes both, not one", func(t *testing.T) {
		internal, portal := base()
		portal.SpecListIDs = []string{"list-en-1"}
		d, err := TranslateToDiff(internal, portal, r)
		if err != nil {
			t.Fatalf("TranslateToDiff: %v", err)
		}
		c := specListChange(t, d)
		if c == nil {
			t.Fatal("expected a prepackagedSpecListIds change when the portal is missing the japanese list")
		}
		ids, ok := c.Value.([]string)
		if !ok {
			t.Fatalf("Value = %#v, want []string", c.Value)
		}
		got := append([]string(nil), ids...)
		sort.Strings(got)
		if len(got) != 2 || got[0] != "list-en-1" || got[1] != "list-ja-1" {
			t.Fatalf("prepackagedSpecListIds Value = %v, want both list-en-1 and list-ja-1", got)
		}
	})

	t.Run("one unresolvable token fails the whole call, never a partial list", func(t *testing.T) {
		internal, portal := base()
		internal.TargetLanguages = []string{"english", "korean"}
		_, err := TranslateToDiff(internal, portal, r)
		if err == nil {
			t.Fatal("expected an error: a partial spec list would narrow a live campaign")
		}
		if !errors.Is(err, ErrUnknownSpecList) {
			t.Fatalf("errors.Is(err, ErrUnknownSpecList) = false, err = %v", err)
		}
		if !strings.Contains(err.Error(), "korean") {
			t.Fatalf("err = %v, want it to name the offending token %q", err, "korean")
		}
	})
}

// TestToSubjectRefs_LegacySentinelRefused pins locked decision 6: migration
// 000023's legacy backfill is marked with a distinct sentinel id, and
// translation refuses it outright. The genuine operator-typed id-0 path — a
// name the operator entered that has never been reconciled with the portal —
// must keep resolving, since those two cases were previously
// indistinguishable and re-resolving a legacy subject would swap live
// 4xxx/8xxx portal ids for current-generation 22xxx ids.
func TestToSubjectRefs_LegacySentinelRefused(t *testing.T) {
	tests := []struct {
		name      string
		subjects  []inventory.TargetSubject
		wantIDs   []int
		wantErrIs error
		wantErrIn string
	}{
		{
			name:     "portal-sourced id passes through verbatim",
			subjects: []inventory.TargetSubject{{ID: 4807, Name: "Charizard"}},
			wantIDs:  []int{4807},
		},
		{
			name:     "operator-typed id 0 still resolves by name",
			subjects: []inventory.TargetSubject{{Name: "Pikachu"}},
			wantIDs:  []int{90001},
		},
		{
			name:      "legacy sentinel is refused, naming the subject",
			subjects:  []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard"}},
			wantErrIs: ErrLegacySubjectsUnreconciled,
			wantErrIn: "Charizard",
		},
		{
			name: "one sentinel poisons an otherwise resolvable list",
			subjects: []inventory.TargetSubject{
				{ID: 4807, Name: "Charizard"},
				{ID: inventory.LegacyUnreconciledSubjectID, Name: "Machamp"},
			},
			wantErrIs: ErrLegacySubjectsUnreconciled,
			wantErrIn: "Machamp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := toSubjectRefs(tt.subjects, englishResolver())
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("errors.Is(err, %v) = false, err = %v", tt.wantErrIs, err)
				}
				if !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("err = %v, want it to name %q", err, tt.wantErrIn)
				}
				if !strings.Contains(err.Error(), "baseline") {
					t.Fatalf("err = %v, want it to tell the operator to run the baseline pull", err)
				}
				if refs != nil {
					t.Fatalf("refs = %+v, want nil on error", refs)
				}
				return
			}
			if err != nil {
				t.Fatalf("toSubjectRefs: %v", err)
			}
			if len(refs) != len(tt.wantIDs) {
				t.Fatalf("refs = %+v, want %d entries", refs, len(tt.wantIDs))
			}
			for i, want := range tt.wantIDs {
				if refs[i].ID != want {
					t.Fatalf("refs[%d].ID = %d, want %d", i, refs[i].ID, want)
				}
			}
		})
	}
}

// TestTranslateToCreate_LegacySentinelRefused covers the create side of the
// same refusal — TranslateToCreate calls toSubjectRefs for both Subjects and
// DeniedSpecs, and a create pushed with re-resolved legacy ids is just as
// wrong as an update.
func TestTranslateToCreate_LegacySentinelRefused(t *testing.T) {
	c := baseCreateCampaign()
	c.DeniedSpecs = []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard EX"}}
	_, err := TranslateToCreate(c, englishResolver())
	if !errors.Is(err, ErrLegacySubjectsUnreconciled) {
		t.Fatalf("errors.Is(err, ErrLegacySubjectsUnreconciled) = false, err = %v", err)
	}
}
```

`mapper_test.go`'s import block becomes:

```go
import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)
```

- [ ] **Step 2: Run the tests — they must fail**

```bash
go test ./internal/domain/psacampaign/ -race
```

Expected output:

```
# github.com/guarzo/slabledger/internal/domain/psacampaign [github.com/guarzo/slabledger/internal/domain/psacampaign.test]
internal/domain/psacampaign/mapper_test.go:20:22: cannot use stubResolver{} (value of struct type stubResolver) as Resolver value in argument to TranslateToDiff: stubResolver does not implement Resolver (wrong type for method SpecListIDs)
internal/domain/psacampaign/mapper_test.go:XXX:12: undefined: ErrLegacySubjectsUnreconciled
FAIL	github.com/guarzo/slabledger/internal/domain/psacampaign [build failed]
```

- [ ] **Step 3: Widen the resolver to a token set**

`internal/domain/psacampaign/resolver.go` — add `"fmt"` to the import block:

```go
import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)
```

Change the interface at `:41-45`:

```go
// Resolver maps SlabLedger's internal targeting vocabulary onto portal ids.
type Resolver interface {
	SpecListIDs(languageTokens []string) ([]string, error)
	SubjectID(name string) (int, error)
}
```

Replace `SpecListIDs` at `:73-97` with the set-valued version plus the single-token helper it now wraps:

```go
// SpecListIDs maps a SET of language tokens to the portal UUIDs of every
// curated list they name. It is all-or-nothing: if any token has no ENABLED
// matching list, it returns ErrUnknownSpecList naming that token and NO ids
// at all.
//
// The all-or-nothing contract is the point, not caution. The caller writes the
// returned slice over a live campaign's entire prepackagedSpecListIds field,
// so returning the resolvable subset would silently narrow what that campaign
// buys — a real-money change, invisible in a diff that shows only the ids that
// survived. A future curated list SlabLedger does not model yet (Chinese, say)
// therefore surfaces as a loud, self-explanatory failure.
func (r *catalogResolver) SpecListIDs(languageTokens []string) ([]string, error) {
	var ids []string
	for _, token := range languageTokens {
		tokenIDs, err := r.specListIDsForToken(token)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", err, token)
		}
		ids = append(ids, tokenIDs...)
	}
	return ids, nil
}

// specListIDsForToken resolves one language token to the portal UUID(s) of the
// matching list(s) whose Name equals the token's curated list name
// (case-insensitive) and whose Status is "ENABLED". Lists with any other
// status are skipped even when the name matches, since the portal can retire a
// list without removing it from the catalog payload.
func (r *catalogResolver) specListIDsForToken(languageToken string) ([]string, error) {
	wantName, ok := languageListNames[languageToken]
	if !ok {
		return nil, ErrUnknownSpecList
	}
	var ids []string
	for _, l := range r.specLists {
		if !strings.EqualFold(l.Name, wantName) {
			continue
		}
		if l.Status != "ENABLED" {
			continue
		}
		ids = append(ids, l.ID)
	}
	if len(ids) == 0 {
		return nil, ErrUnknownSpecList
	}
	return ids, nil
}
```

Add the sentinel to the error block at `resolver.go:17-21`:

```go
var (
	ErrCatalogStale    = errors.New("psacampaign: portal catalog is stale")
	ErrUnknownSpecList = errors.New("psacampaign: no spec list for language")
	ErrUnknownSubject  = errors.New("psacampaign: no subject id for name")
	// ErrLegacySubjectsUnreconciled means a campaign still carries subjects
	// marked by migration 000023's backfill as legacy and never reconciled
	// against the portal. Pushing one would re-resolve a live portal id by
	// name; the operator must run the baseline pull first.
	ErrLegacySubjectsUnreconciled = errors.New(
		"psacampaign: campaign carries legacy unreconciled subjects; run the psa-harvest baseline pull to reconcile them against the portal before pushing")
)
```

- [ ] **Step 4: Build the spec list from the full token set, and refuse the sentinel**

`internal/domain/psacampaign/mapper.go:79-90` — this is the code that currently drops a list:

```go
	// An empty TargetLanguages set means either that this campaign has no
	// spec-list axis to propose yet (legacy/unlinked), or that it is a
	// deliberate open net. The portal can express neither: a SPEC_LIST
	// campaign must carry at least one curated list, so proposing anything
	// here would mean proposing an EMPTY prepackagedSpecListIds — clearing
	// every curated list off a live campaign. The axis is therefore skipped,
	// which also keeps an unlinked campaign from blocking every other scalar
	// fix in this diff.
	if len(internal.TargetLanguages) > 0 {
		specListIDs, err := r.SpecListIDs(internal.TargetLanguages)
		if err != nil {
			return d, fmt.Errorf("psacampaign: resolve spec lists for languages %v: %w", internal.TargetLanguages, err)
		}
		addList("prepackagedSpecListIds",
			renderStringList(portal.SpecListIDs), renderStringList(specListIDs), specListIDs)
	}
```

`renderStringList` (`mapper.go:109-115`) is unchanged and needs no change: it already sorts a copy before joining, so a two-element portal response in either order renders identically and produces no spurious diff. The typed `Value` handed to `push.go` stays the resolver's own unsorted slice — `push.go:66-81` JSON-round-trips it and the portal treats `prepackagedSpecListIds` as a set.

`internal/domain/psacampaign/mapper.go:117-139` — `toSubjectRefs`. The `id == 0` branch is deliberate and survives verbatim; only a new, earlier branch is added:

```go
// toSubjectRefs converts internal.TargetSubject entries to the portal's
// SubjectRef wire shape. An entry with a positive ID is portal-sourced and
// passes through verbatim — it is never re-resolved by name, because live
// portal ids span multiple id generations (4xxx/8xxx/22xxx) that getSubjects
// cannot reproduce. Only ID == 0 entries (operator-entered names never yet
// reconciled with the portal) are resolved via r; a resolution failure returns
// an error naming the subject rather than silently dropping it from what the
// campaign buys.
//
// inventory.LegacyUnreconciledSubjectID is refused outright. Migration 000023
// backfills legacy inclusion-list tokens with that sentinel precisely so they
// are distinguishable from the operator-typed ID == 0 case above: the two were
// previously identical, so a propose issued between deploy and the baseline
// pull would have re-resolved legacy subjects by name and swapped live
// 4xxx/8xxx portal ids for current-generation 22xxx ids. Refusing is the only
// safe answer, since translation has no portal session with which to reconcile
// them itself.
func toSubjectRefs(subjects []inventory.TargetSubject, r Resolver) ([]SubjectRef, error) {
	out := make([]SubjectRef, 0, len(subjects))
	for _, s := range subjects {
		id := s.ID
		switch id {
		case inventory.LegacyUnreconciledSubjectID:
			return nil, fmt.Errorf("%w (subject %q)", ErrLegacySubjectsUnreconciled, s.Name)
		case 0:
			resolved, err := r.SubjectID(s.Name)
			if err != nil {
				return nil, fmt.Errorf("psacampaign: resolve subject %q: %w", s.Name, err)
			}
			id = resolved
		}
		out = append(out, SubjectRef{ID: id, Name: s.Name})
	}
	return out, nil
}
```

`internal/domain/psacampaign/mapper.go:159-174` — `TranslateToCreate`'s doc and guard:

```go
// TranslateToCreate builds the full createCampaign formData for an internal
// campaign. The portal campaign is always created paused (IsActive false);
// money fields are whole USD on the wire (internal cents / 100). Campaigns
// are created as SPEC_LIST (the CATEGORY/POKEMON shape PSA has retired) with
// every curated spec list resolved from the campaign's TargetLanguages set,
// and the subject filter carries the campaign's Subjects/DeniedSpecs.
func TranslateToCreate(internal inventory.Campaign, r Resolver) (CampaignFormData, error) {
	var fd CampaignFormData

	// Unlike the diff path, create cannot skip the axis: a SPEC_LIST campaign
	// with no curated list is not a campaign the portal will accept.
	if len(internal.TargetLanguages) == 0 {
		return fd, fmt.Errorf("psacampaign: campaign has no target languages set")
	}
	specListIDs, err := r.SpecListIDs(internal.TargetLanguages)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: resolve spec lists for languages %v: %w", internal.TargetLanguages, err)
	}
```

The existing `mapper_test.go` case `"empty target language fails"` asserts `wantErr: "target language"`, which still matches the substring of `"no target languages set"` — no change needed to that assertion beyond the `c.TargetLanguages = nil` mutation from Step 1. The `"unmapped language token fails"` case asserts `"resolve spec list"`, still a substring of `"resolve spec lists for languages"`.

- [ ] **Step 5: Update the shared mock**

`internal/testutil/mocks/psa_resolver.go:11-23`:

```go
type ResolverMock struct {
	SpecListIDsFn func(languageTokens []string) ([]string, error)
	SubjectIDFn   func(name string) (int, error)
}

var _ psacampaign.Resolver = (*ResolverMock)(nil)

func (m *ResolverMock) SpecListIDs(languageTokens []string) ([]string, error) {
	if m.SpecListIDsFn != nil {
		return m.SpecListIDsFn(languageTokens)
	}
	return nil, psacampaign.ErrUnknownSpecList
}
```

The type's existing doc comment (`:5-10`) still applies verbatim — zero-value defaults return the "unknown" sentinel rather than a silent empty list — and gains force here: an empty list from a defaulted mock would now mean "clear the campaign's curated lists".

- [ ] **Step 6: Run the psacampaign tests — they must pass**

```bash
go test ./internal/domain/psacampaign/ ./internal/testutil/mocks/ -race
```

Expected output:

```
ok  	github.com/guarzo/slabledger/internal/domain/psacampaign	0.0XXs
ok  	github.com/guarzo/slabledger/internal/testutil/mocks	0.0XXs [no tests to run]
```

- [ ] **Step 7: Classify the sentinel as a 400 at the HTTP boundary**

`internal/adapters/httpserver/handlers/campaigns_psa.go:196-209` — without this, the refusal reaches the operator as a bare 500 with the actionable message swallowed by the generic `"Internal server error"` body:

```go
	diff, err := psacampaign.TranslateToDiff(*c, *portal, resolver)
	if err != nil {
		// ErrUnknownSubject/ErrUnknownSpecList mean an operator-entered
		// name (or campaign language) hasn't been reconciled with the portal
		// catalog yet. ErrLegacySubjectsUnreconciled means migration 000023's
		// legacy backfill hasn't been reconciled by a baseline pull yet. All
		// three are expected, actionable 400s naming the offender and the fix,
		// not server faults. Anything else is unanticipated and stays a 500.
		if errors.Is(err, psacampaign.ErrUnknownSubject) ||
			errors.Is(err, psacampaign.ErrUnknownSpecList) ||
			errors.Is(err, psacampaign.ErrLegacySubjectsUnreconciled) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error(r.Context(), "failed to translate campaign diff", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
```

The create path at `campaigns_psa.go:346-350` already maps every `TranslateToCreate` error to a 400 with `err.Error()` as the body, so it needs no change.

In `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go`, update `diffCampaign()` at `:48`:

```go
		TargetLanguages:      []string{"english"},
```

update the korean case at `:118` to `c.TargetLanguages = []string{"korean"}`, and add one case to the `TestHandlePSAPropose` table:

```go
		{
			// Migration 000023's legacy backfill is unreconciled until the
			// baseline pull runs. Proposing in that window must refuse with an
			// actionable 400 rather than silently re-resolving legacy subjects
			// to current-generation portal ids.
			name: "legacy unreconciled subject maps to 400 not 500",
			campaign: func() *inventory.Campaign {
				c := diffCampaign()
				c.Subjects = []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard"}}
				return &c
			}(),
			portalRows: []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus: http.StatusBadRequest,
		},
```

- [ ] **Step 8: Run the handler tests — they must pass**

```bash
go test ./internal/adapters/httpserver/... -race
```

Expected output:

```
ok  	github.com/guarzo/slabledger/internal/adapters/httpserver/handlers	0.XXXs
ok  	github.com/guarzo/slabledger/internal/adapters/httpserver/middleware	0.XXXs
...
```

- [ ] **Step 9: Full build, architecture checks, and commit**

```bash
go build ./... && go vet ./... && scripts/check-imports.sh && scripts/check-file-size.sh
```

Expected: all four exit 0. `mapper.go` is 252 lines and `resolver.go` 111 before this task; both stay well under the 500-line warning. `check-imports.sh` must stay green — `psacampaign` importing `inventory` is the pre-existing, allowed direction (it is the reverse import that would cycle).

```bash
go test ./... -race -timeout 10m
```

Expected: `ok` for every package, with `internal/adapters/storage/postgres` skipping without `POSTGRES_TEST_URL`.

```bash
git add internal/domain/psacampaign/resolver.go internal/domain/psacampaign/mapper.go \
        internal/domain/psacampaign/mapper_test.go \
        internal/testutil/mocks/psa_resolver.go \
        internal/adapters/httpserver/handlers/campaigns_psa.go \
        internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go
git commit -m "Push every curated spec list, and refuse unreconciled legacy subjects"
```

---

### Task 5: Harvester baseline pull — multi-language, fail-loud, validated

**Files:**
- Modify: `cmd/psa-harvest/baseline.go` (`errAmbiguousSpecListName` deleted, `baselineLanguage` → `baselineLanguages`, `buildBaselineCampaign`, the log call in `runBaselinePull`)
- Modify: `cmd/psa-harvest/baseline_test.go` (`TestBaselineLanguage` → `TestBaselineLanguages`; `TestBuildBaselineCampaign` and `TestRunBaselinePull` fixtures)

`baseline_test.go` is the only test file in `cmd/psa-harvest/`; it already holds
`TestParseBaselineFlag`, `TestBaselineLanguage`, `TestBuildBaselineCampaign`,
and `TestRunBaselinePull`. Do not create a new test file — extend this one.
`TestParseBaselineFlag` is untouched by this task.

**Interfaces:**

- Consumes (from Task 1, already landed — read the real file before editing, do
  not re-declare any of these):
  - `inventory.Campaign.TargetLanguages []string`, JSON tag `targetLanguages`,
    replacing the old `TargetLanguage string`. Lowercase tokens drawn from the
    closed set `{"english","japanese"}`; empty/nil means open net.
  - `func inventory.ValidateAndNormalizeCampaign(c *inventory.Campaign) error` —
    already exists at `internal/domain/inventory/validation.go:82`. Task 1
    extends it to lowercase/trim each entry of `TargetLanguages` and reject any
    token outside the closed set. It already rejects any `SubjectFilterMode`
    outside `{"", "Target", "Exclude"}` (`validation.go:122-125`) and requires a
    non-empty `Name` under `MaxCampaignNameLength` (`validation.go:83-89`).
  - `cardutil.LangEnglish = "english"`, `cardutil.LangJapanese = "japanese"`
    (`internal/platform/cardutil/normalize_sets.go:360-361`).
- Consumes (pre-existing, unchanged by this plan):
  - `psacampaign.PortalCampaign` fields `CampaignRequestID string`,
    `SpecListIDs []string`, `SpecListNames []string`, `SubjectFilter
    psacampaign.CampaignFilter`, `DeniedSpecs []psacampaign.SubjectRef`,
    `TargetingComplete bool` (`internal/domain/psacampaign/types.go:6-34`).
  - `psacampaign.CampaignFilter{Type string; Subjects []SubjectRef}`
    (`types.go:50-53`), `psacampaign.SubjectRef{ID int; Name string}`
    (`types.go:56-59`).
  - `inventory.CampaignRepository` (`ListCampaigns`, `UpdateCampaign`), mocked
    by `mocks.CampaignRepositoryMock`
    (`internal/testutil/mocks/inventory_campaign_repo.go:10`) via
    `ListCampaignsFn func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)`
    and `UpdateCampaignFn func(ctx context.Context, c *inventory.Campaign) error`.
  - `observability.NewNoopLogger()`, `observability.String`, `observability.Err`.
- Produces (package-private to `cmd/psa-harvest`):
  - `func baselineLanguages(specListNames []string) ([]string, error)` — sorted,
    deduplicated token set.
  - `var errNoSpecListName error` (kept, comment preserved),
    `var errUnrecognizedSpecListName error` (new),
    `var errUnexplainedSpecListID error` (new).
  - `errAmbiguousSpecListName` is **deleted**. Nothing outside this package
    referenced it (`grep -rn errAmbiguousSpecListName --include='*.go' .` returns
    only `baseline.go:70,88` and no test reference).
- Nothing outside `cmd/psa-harvest` consumes anything this task produces.

**Task 1 stopgap warning.** Task 1 renames `Campaign.TargetLanguage` to
`TargetLanguages`, which would break `baseline.go:112` and `:189`. To keep the
per-task "tree compiles" invariant, Task 1 leaves some mechanical patch there
(e.g. `updated.TargetLanguages = []string{lang}`). This task replaces that
stopgap wholesale. **Read the current `cmd/psa-harvest/baseline.go` before
editing** — the line numbers cited below are from the pre-Task-1 file and the
stopgap may have shifted them by a line or two.

**Why the language axis is a set here.** All six live portal campaigns carry
both "English Pokemon" and "Japanese Pokemon", so the current
`errAmbiguousSpecListName` branch (`baseline.go:87-89`) rejects 100% of them and
the one-time migration cannot run at all. The closed set of targeting tokens is
duplicated across `internal/domain/inventory/validation.go`,
`internal/domain/psacampaign/resolver.go` (`languageListNames`), this file, and
`web/src/react/utils/campaignConstants.ts` — the import cycle
(`psacampaign` imports `inventory`) makes a shared constant impossible. Any
future token must be added in all four places. This task adds none.

**Three fail-loud guards, not one.** Locked decision 4 requires that a curated
list SlabLedger does not model can never be silently dropped. There are two
distinct drop sites, and the brief for this task named only the second:

1. `specListNames` (`internal/adapters/clients/psaportal/campaigns.go:189-201`)
   builds `SpecListNames` by resolving each id in `SpecListIDs` against the
   harvested catalog and **skipping ids the catalog does not explain**. An
   unmodelled list can therefore vanish before `baselineLanguages` ever sees it.
   `campaigns_test.go:314` pins exactly this skip. The only evidence left behind
   is a length mismatch between `SpecListIDs` and `SpecListNames` —
   `errUnexplainedSpecListID` checks it.
2. A name the catalog *does* explain but SlabLedger has no token for (the
   catalog really does contain e.g. "English Base Set") reaches
   `baselineLanguage`'s `default: continue` (`baseline.go:84-85`) and is dropped
   there — `errUnrecognizedSpecListName` replaces that branch.
3. `errNoSpecListName` (`baseline.go:60-66`) survives unchanged for the
   genuinely empty case. Its comment encodes a real decision: CATEGORY-era
   campaigns predate the curated-list model and name no list at all; they are
   converted by hand in the portal and the baseline is re-run. Do not fold this
   case into either new error — an empty list is expected and self-explanatory,
   an unmodelled list is a code gap.

Guard 1 is deliberately strict: if a catalog fetch returns partial data, every
campaign refuses and the run exits non-zero. For a one-time migration of six
money-spending campaigns, refusing on a partial view of what a campaign buys is
the correct failure.

**Validation on the write path.** `buildBaselineCampaign` currently assigns
`pc.SubjectFilter.Type` raw into `SubjectFilterMode` (`baseline.go:113`), and
`runBaselinePull` writes through `campaigns.UpdateCampaign` (`baseline.go:184`),
which applies no validation. Any portal string other than `"Exclude"` therefore
lands in the database and is read as Target semantics by
`inventory.SubjectAxisMatches` (`matching.go:150-153`) — an unvalidated remote
string reaching a live buy decision. The fix is to route the built campaign
through `inventory.ValidateAndNormalizeCampaign` rather than hand-checking the
string, which closes the mode hole and the language-token hole together. A
validation failure is a **skip**, not an abort: it flows into the existing
`buildBaselineCampaign` error path (`baseline.go:174-182`), so it is logged,
counted, and makes the run exit non-zero, exactly like every other skip reason.

The existing skip/unobserved-links accounting (`baseline.go:158-218`) is
preserved byte-for-byte apart from the one log call. Its comments encode
hard-won decisions — the unobserved-links check exists because the loop is
driven by portal campaigns and structurally cannot see an internal campaign the
portal omitted. Do not touch it.

**Test fixture consequence.** Routing through validation means every
`inventory.Campaign` fixture that reaches `buildBaselineCampaign` now needs a
non-empty `Name`. `TestRunBaselinePull`'s fleet fixtures
(`baseline_test.go:249-254`) have none today, so without this change every write
case would fail validation and the whole test would go red for the wrong reason.

---

- [ ] **Step 1: Write the failing test**

Replace `TestBaselineLanguage` (`baseline_test.go:91-130`) with
`TestBaselineLanguages`, and replace `TestBuildBaselineCampaign`
(`:132-220`) and `TestRunBaselinePull` (`:222-333`) with the versions below.
Leave `TestParseBaselineFlag` and the import block alone except for adding
`"reflect"` — it is already imported at `baseline_test.go:6`.

```go
func TestBaselineLanguages(t *testing.T) {
	tests := []struct {
		name          string
		specListNames []string
		want          []string
		wantErr       error
	}{
		{name: "japanese only", specListNames: []string{"Japanese Pokemon"}, want: []string{"japanese"}},
		{name: "english only", specListNames: []string{"English Pokemon"}, want: []string{"english"}},
		{
			// The live shape: all six portal campaigns carry both lists. The
			// old code rejected this as ambiguous, which is why the baseline
			// pull could not run at all.
			name:          "both lists is the live shape, not an error",
			specListNames: []string{"Japanese Pokemon", "English Pokemon"},
			want:          []string{"english", "japanese"},
		},
		{
			name:          "order is normalized, not preserved",
			specListNames: []string{"English Pokemon", "Japanese Pokemon"},
			want:          []string{"english", "japanese"},
		},
		{
			name:          "duplicates collapse to one token",
			specListNames: []string{"Japanese Pokemon", "Japanese Pokemon"},
			want:          []string{"japanese"},
		},
		{
			// CATEGORY-era campaign: names no curated list at all. Expected,
			// and distinct from an unmodelled list.
			name:          "no names at all is the CATEGORY-era case",
			specListNames: []string{},
			wantErr:       errNoSpecListName,
		},
		{
			name:          "nil names is the CATEGORY-era case",
			specListNames: nil,
			wantErr:       errNoSpecListName,
		},
		{
			// Locked decision 4: a curated list SlabLedger does not model is
			// refused, never silently dropped.
			name:          "unmodelled list alone is refused",
			specListNames: []string{"English Base Set"},
			wantErr:       errUnrecognizedSpecListName,
		},
		{
			// The dangerous case: baselining this campaign from the two names
			// we do understand would record a narrower buy scope than the
			// portal actually has.
			name:          "unmodelled list alongside modelled ones is still refused",
			specListNames: []string{"Japanese Pokemon", "English Base Set", "English Pokemon"},
			wantErr:       errUnrecognizedSpecListName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := baselineLanguages(tt.specListNames)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("baselineLanguages(%v) error = %v, want errors.Is(_, %v)", tt.specListNames, err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("baselineLanguages(%v) = %v on error, want nil", tt.specListNames, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("baselineLanguages(%v): unexpected error: %v", tt.specListNames, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("baselineLanguages(%v) = %v, want %v", tt.specListNames, got, tt.want)
			}
		})
	}
}

func TestBaselineLanguagesErrorNamesTheList(t *testing.T) {
	// The operator's only lead on a refused campaign is this message; it must
	// carry the list name, not just the fact that something was unmodelled.
	_, err := baselineLanguages([]string{"English Base Set", "Japanese Pokemon"})
	if err == nil {
		t.Fatal("baselineLanguages(): got nil error, want error")
	}
	if !strings.Contains(err.Error(), "English Base Set") {
		t.Errorf("error %q does not name the unrecognized list", err)
	}
}

func TestBuildBaselineCampaign(t *testing.T) {
	// Name is required: buildBaselineCampaign now runs the result through
	// inventory.ValidateAndNormalizeCampaign, which rejects an empty Name.
	existing := inventory.Campaign{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"}

	tests := []struct {
		name    string
		pc      psacampaign.PortalCampaign
		want    inventory.Campaign
		wantErr error
	}{
		{
			name: "both curated lists, subjects and denied specs copied verbatim",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-jp", "uuid-en"},
				SpecListNames:     []string{"Japanese Pokemon", "English Pokemon"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Target",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}, {ID: 8105, Name: "Crystal Golem"}},
				},
				DeniedSpecs: []psacampaign.SubjectRef{{ID: 4807, Name: "Gold Star Charizard"}},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguages:   []string{"english", "japanese"},
				SubjectFilterMode: "Target",
				Subjects: []inventory.TargetSubject{
					{ID: 22210, Name: "Machamp"},
					{ID: 8105, Name: "Crystal Golem"},
				},
				DeniedSpecs: []inventory.TargetSubject{{ID: 4807, Name: "Gold Star Charizard"}},
			},
		},
		{
			name: "exclude with zero subjects is an open net, not an error",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-en"},
				SpecListNames:     []string{"English Pokemon"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Exclude"},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguages:   []string{"english"},
				SubjectFilterMode: "Exclude",
				Subjects:          []inventory.TargetSubject{},
				DeniedSpecs:       []inventory.TargetSubject{},
			},
		},
		{
			name: "no curated list is the CATEGORY-era skip",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{},
				SpecListNames:     []string{},
			},
			wantErr: errNoSpecListName,
		},
		{
			// The catalog explained neither id, so SpecListNames came back
			// empty and baselineLanguages would have reported the harmless
			// CATEGORY-era case for a campaign that in fact buys two lists.
			name: "spec-list ids the catalog could not explain are refused",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-unknown-a", "uuid-unknown-b"},
				SpecListNames:     []string{},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Target"},
			},
			wantErr: errUnexplainedSpecListID,
		},
		{
			name: "one unexplained id alongside two explained ones is refused",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-jp", "uuid-en", "uuid-unknown"},
				SpecListNames:     []string{"Japanese Pokemon", "English Pokemon"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Target"},
			},
			wantErr: errUnexplainedSpecListID,
		},
		{
			name: "unmodelled curated list is refused",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-en-base"},
				SpecListNames:     []string{"English Base Set"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Target"},
			},
			wantErr: errUnrecognizedSpecListName,
		},
		{
			// The unvalidated-remote-string hole: anything but "Exclude" was
			// read as Target semantics by SubjectAxisMatches, so this string
			// used to reach a live buy decision unchecked.
			name: "unknown subject filter type is rejected by validation, not silently treated as Target",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-en"},
				SpecListNames:     []string{"English Pokemon"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Include",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}},
				},
			},
			wantErr: inventory.ErrInvalidSubjectFilterMode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildBaselineCampaign(existing, tt.pc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("buildBaselineCampaign() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
				}
				if !reflect.DeepEqual(got, inventory.Campaign{}) {
					t.Errorf("buildBaselineCampaign() = %+v on error, want zero Campaign", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildBaselineCampaign(): unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got.TargetLanguages, tt.want.TargetLanguages) {
				t.Errorf("TargetLanguages = %v, want %v", got.TargetLanguages, tt.want.TargetLanguages)
			}
			if got.SubjectFilterMode != tt.want.SubjectFilterMode {
				t.Errorf("SubjectFilterMode = %q, want %q", got.SubjectFilterMode, tt.want.SubjectFilterMode)
			}
			if !reflect.DeepEqual(got.Subjects, tt.want.Subjects) {
				t.Errorf("Subjects = %+v, want %+v", got.Subjects, tt.want.Subjects)
			}
			if !reflect.DeepEqual(got.DeniedSpecs, tt.want.DeniedSpecs) {
				t.Errorf("DeniedSpecs = %+v, want %+v", got.DeniedSpecs, tt.want.DeniedSpecs)
			}
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.PSACampaignRequestID != tt.want.PSACampaignRequestID {
				t.Errorf("existing campaign fields altered: got %+v", got)
			}
		})
	}
}

func TestRunBaselinePull(t *testing.T) {
	linkedComplete := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListIDs:   []string{"uuid-jp", "uuid-en"},
		SpecListNames: []string{"Japanese Pokemon", "English Pokemon"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedIncomplete := psacampaign.PortalCampaign{CampaignRequestID: "req-2", TargetingComplete: false}
	// Same shape as linkedComplete (a cleanly resolvable language set and
	// subject filter) except for TargetingComplete, so a case built from it
	// isolates the TargetingComplete guard: if that guard were ever bypassed,
	// this fixture would resolve and write instead of skip, unlike
	// linkedIncomplete (which also fails on empty SpecListNames and would mask
	// the bypass).
	linkedIncompleteOtherwiseValid := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: false,
		SpecListIDs:   []string{"uuid-jp", "uuid-en"},
		SpecListNames: []string{"Japanese Pokemon", "English Pokemon"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedNoSpecList := psacampaign.PortalCampaign{
		CampaignRequestID: "req-3", TargetingComplete: true,
		SpecListIDs:   []string{},
		SpecListNames: []string{}, // no curated list -> unconverted CATEGORY campaign, §8
	}
	// The mode hole, end to end: a raw portal string that is neither Target nor
	// Exclude must never reach the database.
	linkedBadFilterType := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListIDs:   []string{"uuid-en"},
		SpecListNames: []string{"English Pokemon"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Include"},
	}
	// The decode-time drop, end to end: the catalog explained neither id.
	linkedUnexplainedIDs := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListIDs:   []string{"uuid-unknown"},
		SpecListNames: []string{},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	notLinked := psacampaign.PortalCampaign{CampaignRequestID: "req-unlinked", TargetingComplete: true}

	// The internal fleet is per-case, not a shared fixture: the unobserved-links
	// check makes the result depend on which internal campaigns exist, so a case
	// that passes only one portal campaign must also narrow the fleet or it will
	// (correctly) report the other two as missing from the portal.
	//
	// Name is populated on every row because buildBaselineCampaign now runs the
	// result through inventory.ValidateAndNormalizeCampaign, which requires it.
	allThree := []inventory.Campaign{
		{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"},
		{ID: "camp-2", Name: "Modern Slabs", PSACampaignRequestID: "req-2"},
		{ID: "camp-3", Name: "Legacy Category", PSACampaignRequestID: "req-3"},
	}
	onlyOne := []inventory.Campaign{{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"}}

	tests := []struct {
		name       string
		internal   []inventory.Campaign
		portal     []psacampaign.PortalCampaign
		updateErr  error
		wantErr    bool
		wantWrites int
		wantLangs  []string // asserted on the first write, when wantWrites > 0
	}{
		{
			name:       "writes the linked complete campaign, skips the rest, exits non-zero",
			internal:   allThree,
			portal:     []psacampaign.PortalCampaign{linkedComplete, linkedIncomplete, linkedNoSpecList, notLinked},
			wantErr:    true,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
		{
			// The whole point of the change: a campaign carrying both curated
			// lists is the live shape and must write cleanly, exit 0.
			name:       "both curated lists writes cleanly and exits zero",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    false,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
		{
			name:       "an update failure aborts immediately",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			updateErr:  errors.New("db down"),
			wantErr:    true,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
		{
			// Isolates the TargetingComplete guard from every other skip
			// reason: SpecListNames/SubjectFilter here resolve cleanly, so the
			// only thing that can cause a skip (writes == 0) is the guard
			// itself. A bypassed guard would write and this case would fail.
			name:       "incomplete edit-form fetch skips even when the rest of the record is resolvable",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedIncompleteOtherwiseValid},
			wantErr:    true,
			wantWrites: 0,
		},
		{
			name:       "an unvalidatable subject filter type skips rather than writing",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedBadFilterType},
			wantErr:    true,
			wantWrites: 0,
		},
		{
			name:       "spec-list ids the catalog could not explain skip rather than writing",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedUnexplainedIDs},
			wantErr:    true,
			wantWrites: 0,
		},
		{
			// The blind spot the loop cannot see: camp-2 and camp-3 are linked
			// but the portal never returned them, so they keep stale targeting.
			// Without the unobserved check this case returns nil and exits 0.
			name:       "linked campaigns absent from the portal fetch are an error",
			internal:   allThree,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    true,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			var firstWritten inventory.Campaign
			repo := &mocks.CampaignRepositoryMock{
				ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
					return tt.internal, nil
				},
				UpdateCampaignFn: func(ctx context.Context, c *inventory.Campaign) error {
					if writes == 0 {
						firstWritten = *c
					}
					writes++
					return tt.updateErr
				},
			}
			logger := observability.NewNoopLogger()
			err := runBaselinePull(context.Background(), tt.portal, repo, logger)
			if tt.wantErr && err == nil {
				t.Fatalf("runBaselinePull(): got nil error, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("runBaselinePull(): unexpected error: %v", err)
			}
			if writes != tt.wantWrites {
				t.Errorf("UpdateCampaign called %d times, want %d", writes, tt.wantWrites)
			}
			if tt.wantLangs != nil && !reflect.DeepEqual(firstWritten.TargetLanguages, tt.wantLangs) {
				t.Errorf("first written TargetLanguages = %v, want %v", firstWritten.TargetLanguages, tt.wantLangs)
			}
		})
	}
}
```

`TestBaselineLanguagesErrorNamesTheList` uses `strings`, which
`baseline_test.go` does not import yet. Add it to the test file's import block:

```go
import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./cmd/psa-harvest/...
```

Expected: a compile failure, because the new symbols do not exist yet:

```
# github.com/guarzo/slabledger/cmd/psa-harvest [github.com/guarzo/slabledger/cmd/psa-harvest.test]
cmd/psa-harvest/baseline_test.go:115:16: undefined: baselineLanguages
cmd/psa-harvest/baseline_test.go:129:24: undefined: errUnrecognizedSpecListName
cmd/psa-harvest/baseline_test.go:...: undefined: errUnexplainedSpecListID
FAIL	github.com/guarzo/slabledger/cmd/psa-harvest [build failed]
```

(Exact line numbers depend on where the blocks land; the `undefined:` symbols are
the assertion that matters.)

- [ ] **Step 3: Write the implementation**

In `cmd/psa-harvest/baseline.go`, add `"strings"` to the import block. The final
block is:

```go
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)
```

Replace the sentinel block and `baselineLanguage` (`baseline.go:60-96`) with:

```go
// errNoSpecListName means the campaign named no curated spec lists at all.
// This is the expected shape of the remaining CATEGORY-era campaigns (design
// doc §8): their edit form predates the curated-list model, so it names no
// "Japanese Pokemon" / "English Pokemon" list. They are not writable here; the
// operator converts them by hand in the portal and re-runs the baseline.
//
// Distinct from errUnrecognizedSpecListName: an empty list is an expected,
// self-explanatory state, while an unmodelled list is a gap in this code.
var errNoSpecListName = errors.New("no curated spec-list name (see design §8: conversion path for CATEGORY campaigns)")

// errUnrecognizedSpecListName means the portal campaign carries a curated list
// this code has no language token for. Baselining from only the lists we do
// understand would record a narrower buy scope than the campaign actually has,
// and the next push would then shrink the live campaign to match. Refuse, and
// name the list so the operator knows what to add.
//
// The token set is duplicated in internal/domain/inventory/validation.go,
// internal/domain/psacampaign/resolver.go (languageListNames), and
// web/src/react/utils/campaignConstants.ts — psacampaign imports inventory, so
// a single shared constant would be an import cycle. Adding a token here means
// adding it in all four places.
var errUnrecognizedSpecListName = errors.New("unrecognized curated spec-list name")

// errUnexplainedSpecListID means the portal campaign referenced curated
// spec-list ids that the harvested catalog could not name. specListNames
// (internal/adapters/clients/psaportal/campaigns.go:189-201) drops those ids
// silently rather than failing the decode, so SpecListNames is a partial view
// and the count mismatch is the only surviving evidence. Refusing here is
// deliberately strict — a partial catalog fetch fails every campaign — because
// the alternative is baselining a live buying campaign from an incomplete
// picture of what it buys.
var errUnexplainedSpecListID = errors.New("curated spec-list ids the harvested catalog could not name")

// baselineLanguages maps a portal campaign's curated spec-list names to the
// set of language tokens stored in inventory.Campaign.TargetLanguages. The
// result is deduplicated and sorted, so the stored value, the log line, and
// the test assertions are all stable regardless of portal ordering.
//
// Carrying more than one list is the normal live shape — every active campaign
// targets both English and Japanese Pokemon — not an ambiguity to resolve.
func baselineLanguages(specListNames []string) ([]string, error) {
	seen := make(map[string]bool, len(specListNames))
	var unrecognized []string
	for _, name := range specListNames {
		switch name {
		case "Japanese Pokemon":
			seen[cardutil.LangJapanese] = true
		case "English Pokemon":
			seen[cardutil.LangEnglish] = true
		default:
			unrecognized = append(unrecognized, name)
		}
	}
	// Report every unmodelled list at once: the operator's remedy is a code
	// change, and learning about the second list only after shipping the first
	// fix costs another deploy.
	if len(unrecognized) > 0 {
		sort.Strings(unrecognized)
		return nil, fmt.Errorf("%w: %q", errUnrecognizedSpecListName, unrecognized)
	}
	if len(seen) == 0 {
		return nil, errNoSpecListName
	}
	tokens := make([]string, 0, len(seen))
	for token := range seen {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens, nil
}
```

Replace `buildBaselineCampaign` (`baseline.go:98-117`) with:

```go
// buildBaselineCampaign copies one portal campaign's targeting onto a copy of
// the internal campaign already linked to it. Subject and denied-spec ids are
// copied verbatim, never re-resolved by name: live portal ids span 4xxx/8xxx/
// 22xxx generations while getSubjects (used only for operator-added subjects,
// see (*psaportal.Client).FetchSubjects) returns only 22xxx ids, so
// name-based resolution here would silently rewrite ids on active,
// money-spending campaigns on the very next push.
//
// Every failure returns a zero Campaign so a caller cannot mistake a partial
// build for a writable one.
func buildBaselineCampaign(existing inventory.Campaign, pc psacampaign.PortalCampaign) (inventory.Campaign, error) {
	// Check coverage before reading names: SpecListNames omits every id the
	// catalog could not explain, so an unmodelled list can look like the
	// harmless CATEGORY-era empty case.
	if len(pc.SpecListIDs) != len(pc.SpecListNames) {
		return inventory.Campaign{}, fmt.Errorf("%w: %d id(s), %d named",
			errUnexplainedSpecListID, len(pc.SpecListIDs), len(pc.SpecListNames))
	}

	langs, err := baselineLanguages(pc.SpecListNames)
	if err != nil {
		return inventory.Campaign{}, err
	}

	updated := existing
	updated.TargetLanguages = langs
	updated.SubjectFilterMode = pc.SubjectFilter.Type
	updated.Subjects = toTargetSubjects(pc.SubjectFilter.Subjects)
	updated.DeniedSpecs = toTargetSubjects(pc.DeniedSpecs)

	// SubjectFilter.Type is a raw remote string. inventory.SubjectAxisMatches
	// (matching.go:150-153) treats anything other than SubjectFilterExclude as
	// Target semantics, so an unexpected portal value would reach a live buy
	// decision unchecked. Validation is the gate rather than a hand-written
	// string comparison here: it also enforces the language-token closed set,
	// and runBaselinePull writes through CampaignRepository.UpdateCampaign,
	// which applies no validation of its own.
	if err := inventory.ValidateAndNormalizeCampaign(&updated); err != nil {
		return inventory.Campaign{}, fmt.Errorf("validate baselined targeting: %w", err)
	}
	return updated, nil
}
```

In `runBaselinePull`, change only the success log call (`baseline.go:187-189`)
so it reports the set rather than a single token:

```go
		logger.Info(ctx, "psa-harvest: baseline wrote campaign targeting",
			observability.String("campaignId", existing.ID),
			observability.String("targetLanguages", strings.Join(updated.TargetLanguages, ",")))
```

`observability` has no slice field constructor (`logger.go:25-41` offers
`String`, `Int`, `Int64`, `Err` only), so a comma-joined string is the
repo-consistent representation. The slice is already sorted by
`baselineLanguages`, so the log text is stable.

Also update `runBaselinePull`'s doc comment, whose last skip clause still
describes the one-language rule (`baseline.go:135-136`). Replace the sentence

```
// spec-list names don't map to exactly one language.
```

with

```
// spec-list names don't map cleanly onto the internal targeting model (an
// unmodelled curated list, an id the catalog could not name, or a subject
// filter mode validation rejects).
```

Leave the rest of `runBaselinePull` — the skip accounting, the unobserved-links
check, the sorted error messages (`baseline.go:158-218`) — exactly as it is.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race ./cmd/psa-harvest/... -run 'TestBaselineLanguages|TestBaselineLanguagesErrorNamesTheList|TestBuildBaselineCampaign|TestRunBaselinePull|TestParseBaselineFlag' -v
```

Expected: every subtest reports `--- PASS`, ending with

```
ok  	github.com/guarzo/slabledger/cmd/psa-harvest
```

Then confirm nothing else in the tree referenced the removed symbol or the old
function name:

```bash
grep -rn 'errAmbiguousSpecListName\|baselineLanguage(' --include='*.go' .
```

Expected: no output.

Then the full suite and the quality gates:

```bash
go test -race -timeout 10m ./...
go vet ./...
scripts/check-file-size.sh
```

Expected: `ok`/`no test files` for every package, no `vet` output, and no
file-size warning for `cmd/psa-harvest/baseline.go` (it grows from 219 to
roughly 265 lines, well under the 500-line warn threshold).

- [ ] **Step 5: Commit**

```bash
git add cmd/psa-harvest/baseline.go cmd/psa-harvest/baseline_test.go
git commit -m "fix: baseline pull records the full curated spec-list set and validates portal targeting"
```

`.githooks/pre-commit` runs `go vet ./...`. Never bypass it with `--no-verify`.

---

### Task 6: Frontend — multi-language targeting + readable spec-list diff

**Files:**
- Modify: `web/src/types/campaigns/core.ts:20` (`Campaign.targetLanguage`), `:150` (`CreateCampaignInput.targetLanguage`)
- Modify: `web/src/react/utils/campaignConstants.ts:73-77` (`targetLanguageOptions`), `:82-97` (`defaultCampaignInput`)
- Modify: `web/src/react/ui/CampaignFormFields.tsx:1-3` (imports), `:19` (`CampaignFormValues.targetLanguage`), `:29-37` (props), `:137-141` (component head), `:188-190` (the language `Select`)
- Modify: `web/src/react/ui/SubjectListEditor.tsx:5` (imports), `:124-142` (chip list)
- Modify: `web/src/react/pages/campaign-detail/PSAPublishModal.tsx:8` (imports), `:320-335` (change list)
- Modify: `web/src/react/pages/CampaignsPage.tsx:45` (comment naming the targeting fields)
- Create: `web/src/react/pages/campaign-detail/SpecListChangeRow.tsx`
- Test: `web/src/react/ui/CampaignFormFields.test.tsx:10-27` (`baseValues`), `:39-44` (language test)
- Test: `web/src/react/ui/SubjectListEditor.test.tsx` (append one case)
- Test: `web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx:24-46` (`makeCampaign`), append two cases
- Test: `web/src/react/pages/campaigns/CampaignsTab.test.tsx:17-39` (`makeCampaign`)

**Interfaces:**
- Consumes (from Task 1, frozen): `inventory.Campaign.TargetLanguages []string` with JSON tag `targetLanguages`, replacing `TargetLanguage string`. Values are lowercase tokens from the closed set `{"english","japanese"}`; an empty or absent array is the open net (the campaign buys any language). Go tags are authoritative; `web/src/types/` mirrors them by hand.
- Consumes (from Task 1, frozen): `inventory.LegacyUnreconciledSubjectID = -1`, the id migration 000023 backfills onto pre-axis subjects. It is a *different* marker from `id 0`, which means "operator typed this name, resolve it by name at push time" and stays fully functional.
- Consumes (already shipped, unchanged by this task): `GET /api/psa/subjects` → `{"subjects":[{"id":number,"name":string}],"fetchedAt":string}` via `api.listPSASubjects()` (`web/src/js/api/psaCampaigns.ts:16`); `psacampaign.FieldChange` → `{field, old, new, value?}` (`internal/domain/psacampaign/types.go:107-115`), mirrored at `web/src/types/campaigns/psaCampaign.ts:47-52`. The curated-spec-list axis arrives as the change whose `field` is `prepackagedSpecListIds` (`internal/domain/psacampaign/mapper.go:88`), with `old`/`new` rendered by `renderStringList` (`mapper.go:111-115`): sorted, comma-joined portal UUIDs, no spaces.
- Produces: `Campaign.targetLanguages: string[]` and `CreateCampaignInput.targetLanguages: string[]`; `CampaignFormValues.targetLanguages: string[]`; `CampaignFormFields`'s `onChange` widened to `(field: string, value: string | number | string[] | SubjectRef[]) => void`; `targetLanguageOptions` losing its `{ value: '', label: 'Unset' }` entry; `LEGACY_UNRECONCILED_SUBJECT_ID = -1` exported from `campaignConstants.ts`; `SpecListChangeRow` + `SPEC_LIST_FIELD` from the new `SpecListChangeRow.tsx`.

**Duplicated closed set — say it out loud.** `targetLanguageOptions` in `web/src/react/utils/campaignConstants.ts` is one of FOUR copies of the language closed set. The other three are Go-side: `internal/domain/inventory/validation.go`, `internal/domain/psacampaign/resolver.go`, `cmd/psa-harvest/baseline.go`. They cannot be collapsed into one — `psacampaign` imports `inventory`, so `inventory` can never import `psacampaign`, and the frontend copy is across a process boundary regardless. This task edits the frontend copy only; the Go copies are edited by Tasks 1, 4 and 5. Any future token (e.g. `chinese`) must land in all four.

**Where the form actually lives.** `CampaignFormFields` (`web/src/react/ui/CampaignFormFields.tsx`) is mounted in exactly one place: the create form inside `CampaignsTab` (`web/src/react/pages/campaigns/CampaignsTab.tsx:96`), which feeds it `useForm<CreateCampaignInput>` values seeded from `defaultCampaignInput`. There is no post-create targeting edit surface — `web/src/react/pages/CampaignsPage.tsx:45-56` documents that deliberately, and its comment names `targetLanguage`, so it must be updated in the same change. The control being replaced is a single-value `<Select id={targetLanguageId} label="Language" …>` at `:188-190`.

**Why the spec-list names are NOT rendered, and what would have to change.** The names are not available client-side. Verified: the only id→name source for curated spec lists is `psacampaign.SpecListRef` (`internal/domain/psacampaign/resolver.go:23-27`), persisted by `PSAPortalCatalogStore.SpecLists` (`internal/adapters/storage/postgres/psa_portal_catalog_store.go:73`) and read by the server only inside `buildResolver` (`internal/adapters/httpserver/handlers/campaigns_psa.go:42`). No HTTP route exposes it — `internal/adapters/httpserver/routes.go:117` registers `GET /api/psa/subjects` and nothing equivalent for spec lists. `PortalCampaign.specListNames` exists on the `/api/psa-campaigns` snapshot, but its own doc comment (`internal/domain/psacampaign/types.go:19-23`) forbids zipping it against `specListIDs` positionally, so it cannot be turned into a map; that query is also gated `enabled: open && !isLinked` (`PSAPublishModal.tsx:106`) while the diff only renders when the campaign *is* linked. Making real names available needs a backend change, and this task does not invent one. The two candidates, for whoever picks it up:
1. **Preferred — a new read-only route** `GET /api/psa/spec-lists` returning `{"specLists":[{"id":string,"name":string}],"fetchedAt":string}`, served from `PSAPortalCatalogStore.SpecLists` exactly the way `HandleGetPSASubjects` (`campaigns_psa.go:63-77`) serves subjects. The frontend then maps ids→names and falls back to the raw id for anything the catalog cannot explain.
2. **Rejected — rendering names into `FieldChange.Old`/`New` in the mapper.** `mapper.go` compares those two renderings for equality to decide whether a change exists at all (`addList`, `mapper.go:26-30`), so folding portal-mutable display names into them would make a pure portal rename look like a targeting change and enqueue a spurious push. Do not do this.

What this task delivers instead, which solves the actual reported problem (a dropped or swapped curated list being invisible to the approver): the `prepackagedSpecListIds` row stops being two comma-joined blobs and becomes a **set diff** — explicit "removed" and "added" chips, an unchanged count, and a loud removal warning — captioned with the campaign's own configured target languages. A dropped list renders as a red "Removed" chip and a "1 curated list will be removed" warning instead of hiding inside a wall of UUIDs.

**Legacy subjects (`id -1`) in `SubjectListEditor` — decision and justification.** Decision: surface them distinctly. Justification: Task 4 makes push translation *refuse* any campaign carrying a `-1` subject, so the operator meets a rejection whose cause must be visible where the subjects are; the alternative (leaving them as an ordinary chip with `title="id: -1"`) is a puzzle. The cost is one styling branch plus one warning line. Honest caveat, stated because a reviewer will notice: the editor is currently reachable only from the create form, whose subjects always start empty, so a `-1` entry cannot reach it *today* — the code is correct-in-advance for the first task that adds a targeting edit surface, which the file itself already anticipates. The `id === 0` paths are untouched: `:47` (`selectedIds` skipping id 0), `:55-56` (name-based dedupe for id 0), and `:73` (`{ id: 0, name: trimmed }` on Enter) keep their exact current behavior. `-1` entering `selectedIds` at `:47` is harmless — portal subject ids are positive, so no catalog row is ever filtered out by it. `-1` chips stay removable: removing one is a deliberate operator edit, and inventing a "you may not delete this" rule would be policy this task has no mandate to set.

**Verification note.** `npm run build` is `vite build` (`web/package.json:11`) against `web/vite.config.js` — there is no type-checker plugin, so esbuild strips types and the build does **not** fail on TypeScript errors. Type drift is caught by `npm run typecheck` (`tsc --noEmit`), which is therefore run at every gate below alongside the build. `web/vite.config.ts` does not exist; the file is `vite.config.js`.

**Vitest note.** The suite is vitest 4 (`web/package.json:64`). In vitest 4 `expect.objectContaining` does not match a missing key against an expected `undefined`, so none of the assertions below use `objectContaining` on the changed shapes — they assert on whole arrays (`toHaveBeenCalledWith('targetLanguages', ['english'])`) and on rendered text, both of which are exact.

- [ ] **Step 1: Write the failing multi-select tests**

Replace `web/src/react/ui/CampaignFormFields.test.tsx` in full:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CampaignFormFields, { type CampaignFormValues } from './CampaignFormFields';

vi.mock('../../js/api', () => ({
  api: { listPSASubjects: vi.fn().mockResolvedValue({ subjects: [], fetchedAt: '' }) },
}));

function baseValues(): CampaignFormValues {
  return {
    name: 'Test',
    sport: 'Pokemon',
    yearRange: '',
    gradeRange: '',
    priceRange: '',
    clConfidence: '',
    buyTermsCLPct: 0.7,
    dailySpendCapCents: 50000,
    targetLanguages: [],
    subjectFilterMode: 'Target',
    subjects: [],
    deniedSpecs: [],
    psaSourcingFeeCents: 300,
    ebayFeePct: 0.1235,
  };
}

function renderFields(values: CampaignFormValues, onChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <CampaignFormFields values={values} onChange={onChange} />
    </QueryClientProvider>,
  );
  return onChange;
}

describe('CampaignFormFields targeting section', () => {
  it('checking a language adds its token to the set', () => {
    const onChange = renderFields(baseValues());
    fireEvent.click(screen.getByRole('checkbox', { name: 'Japanese' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['japanese']);
  });

  it('checking a second language keeps the first — the live campaigns carry both', () => {
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['english'] });
    fireEvent.click(screen.getByRole('checkbox', { name: 'Japanese' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['english', 'japanese']);
  });

  it('unchecking a language removes only that token', () => {
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['english', 'japanese'] });
    fireEvent.click(screen.getByRole('checkbox', { name: 'English' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['japanese']);
  });

  it('reflects the current selection in the checkbox states', () => {
    renderFields({ ...baseValues(), targetLanguages: ['japanese'] });
    expect(screen.getByRole('checkbox', { name: 'Japanese' })).toBeChecked();
    expect(screen.getByRole('checkbox', { name: 'English' })).not.toBeChecked();
  });

  it('reads an empty selection as an open net, not as an unfilled field', () => {
    renderFields(baseValues());
    expect(screen.getByText(/open net/i)).toBeInTheDocument();
    expect(screen.getByText(/buys any language/i)).toBeInTheDocument();
  });

  it('describes a non-empty selection in plain words', () => {
    renderFields({ ...baseValues(), targetLanguages: ['english', 'japanese'] });
    expect(screen.getByText(/Buys English and Japanese cards only\./)).toBeInTheDocument();
  });

  it('surfaces a token outside the known set instead of silently keeping it', () => {
    // Defensive: the backend closed set is {english, japanese} today. If a
    // future token arrives before this copy of the set is updated, it must be
    // visible — and toggling a known box must not drop it (see toggleLanguage).
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['chinese'] });
    expect(screen.getByText(/Unrecognized language token: chinese/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox', { name: 'English' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['chinese', 'english']);
  });

  it('toggling the subject mode segmented control calls onChange with Exclude', () => {
    const onChange = renderFields(baseValues());
    fireEvent.click(screen.getByRole('radio', { name: 'Exclude' }));
    expect(onChange).toHaveBeenCalledWith('subjectFilterMode', 'Exclude');
  });

  it('shows portal-managed denied specs read-only when present', () => {
    renderFields({ ...baseValues(), deniedSpecs: [{ id: 999, name: 'Bad Card' }] });
    expect(screen.getByText('Bad Card')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /remove bad card/i })).not.toBeInTheDocument();
  });
});
```

Run it:

```bash
cd web && npx vitest run src/react/ui/CampaignFormFields.test.tsx
```

Expected: the file fails to compile/collect, because `CampaignFormValues` has no `targetLanguages` property and the component renders a `<select>`, not checkboxes. Vitest reports a failed test file, e.g.:

```
 FAIL  src/react/ui/CampaignFormFields.test.tsx [ src/react/ui/CampaignFormFields.test.tsx ]
TestingLibraryElementError: Unable to find an accessible element with the role "checkbox"

 Test Files  1 failed (1)
      Tests  7 failed | 2 passed (9)
```

- [ ] **Step 2: Make it pass — types, constants, multi-select control**

`web/src/types/campaigns/core.ts` — replace line 20 (inside `Campaign`) and line 150 (inside `CreateCampaignInput`), both currently `  targetLanguage: string;`:

```typescript
  /** Curated-spec-list language tokens; empty = open net (buys any language).
      Mirrors Go's inventory.Campaign.TargetLanguages (json: targetLanguages). */
  targetLanguages: string[];
```

`web/src/react/utils/campaignConstants.ts` — replace `targetLanguageOptions` (`:73-77`):

```typescript
/**
 * Closed set of curated-spec-list language tokens. There is no "unset" entry:
 * unset is the empty array, expressed by checking nothing.
 *
 * DUPLICATED, deliberately. The same set lives in
 * internal/domain/inventory/validation.go, internal/domain/psacampaign/resolver.go
 * and cmd/psa-harvest/baseline.go. psacampaign imports inventory, so inventory
 * can never import psacampaign, and this copy is across a process boundary
 * anyway. A new token must be added to all four.
 */
export const targetLanguageOptions = [
  { value: 'english', label: 'English' },
  { value: 'japanese', label: 'Japanese' },
] as const;

/**
 * Mirrors Go's inventory.LegacyUnreconciledSubjectID. Migration 000023 backfills
 * pre-axis subjects with this id to mean "legacy, not yet reconciled with the
 * portal". Distinct from id 0, which means "operator typed this name, resolve it
 * by name at push time" — push translation resolves 0 and refuses -1.
 */
export const LEGACY_UNRECONCILED_SUBJECT_ID = -1;
```

and in `defaultCampaignInput` (`:82-97`) replace `  targetLanguage: '',` with:

```typescript
  targetLanguages: [],
```

`web/src/react/ui/CampaignFormFields.tsx` — line 3 imports `targetLanguageOptions` already; no import change is needed there, but `Select` stays imported for the Phase field. Replace line 19 (`  targetLanguage: string;`) inside `CampaignFormValues` with:

```typescript
  targetLanguages: string[];
```

Widen the `onChange` prop in `CampaignFormValues`' consumers — line 31 and the identical line 69 inside `EconomicsSection`:

```typescript
  onChange: (field: string, value: string | number | string[] | SubjectRef[]) => void;
```

Add the control component above `export default function CampaignFormFields` (i.e. before `:137`):

```tsx
function LanguageMultiSelect({
  value, onChange,
}: {
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const groupId = useId();
  const selected = new Set(value);
  const known = new Set<string>(targetLanguageOptions.map(o => o.value));
  // Tokens the backend sent that this copy of the closed set doesn't know.
  // They are preserved through every toggle rather than silently dropped —
  // dropping one would narrow what a live, money-spending campaign buys.
  const unknown = value.filter(t => !known.has(t));

  function toggleLanguage(token: string) {
    onChange(selected.has(token) ? value.filter(t => t !== token) : [...value, token]);
  }

  const labels = value
    .filter(t => known.has(t))
    .map(t => targetLanguageOptions.find(o => o.value === t)?.label ?? t);
  const summary = labels.length === 0
    ? 'None selected — open net: this campaign buys any language.'
    : `Buys ${labels.join(' and ')} cards only.`;

  return (
    <fieldset className="space-y-1.5">
      <legend className="block text-xs text-[var(--text-muted)] mb-1">Languages</legend>
      <div className="flex flex-wrap items-center gap-4">
        {targetLanguageOptions.map(o => (
          <label
            key={o.value}
            htmlFor={`${groupId}-${o.value}`}
            className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer"
          >
            <input
              id={`${groupId}-${o.value}`}
              type="checkbox"
              checked={selected.has(o.value)}
              onChange={() => toggleLanguage(o.value)}
              className="rounded border-[var(--surface-2)]"
            />
            {o.label}
          </label>
        ))}
      </div>
      <p className="text-xs text-[var(--text-subtle)]">{summary}</p>
      {unknown.map(t => (
        <p key={t} className="text-xs text-[var(--warning)]">
          Unrecognized language token: {t} — kept as-is and still pushed.
        </p>
      ))}
    </fieldset>
  );
}
```

In `CampaignFormFields` itself, delete `const targetLanguageId = useId();` (`:140`) and replace the `Select` at `:188-190` with:

```tsx
          <LanguageMultiSelect
            value={values.targetLanguages}
            onChange={(next) => onChange('targetLanguages', next)}
          />
```

Update the two other-file fixtures so the tree type-checks. In `web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx:35` and `web/src/react/pages/campaigns/CampaignsTab.test.tsx:28`, replace `    targetLanguage: '',` with:

```typescript
    targetLanguages: [],
```

Update the stale field name in the `CampaignsPage` comment (`web/src/react/pages/CampaignsPage.tsx:45`):

```typescript
// Targeting (targetLanguages, subjectFilterMode, subjects, deniedSpecs) is
```

Run:

```bash
cd web && npx vitest run src/react/ui/CampaignFormFields.test.tsx && npm run typecheck && npm run build
```

Expected:

```
 Test Files  1 passed (1)
      Tests  9 passed (9)
```

then `tsc --noEmit` prints nothing, then `vite build` ends with `✓ built in …`.

- [ ] **Step 3: Commit the multi-select**

```bash
cd /workspace/.worktrees/psa-spec-list-targeting && git add web/src/types/campaigns/core.ts web/src/react/utils/campaignConstants.ts web/src/react/ui/CampaignFormFields.tsx web/src/react/ui/CampaignFormFields.test.tsx web/src/react/pages/CampaignsPage.tsx web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx web/src/react/pages/campaigns/CampaignsTab.test.tsx && git commit -m "web: campaign language targeting becomes a set"
```

Expected: `.githooks/pre-commit` runs `go vet ./...` (silent, no Go files touched) and the commit succeeds.

- [ ] **Step 4: Write the failing spec-list-diff tests**

Append to `describe('PSAPublishModal', …)` in `web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx`:

```tsx
  // Two real-shaped portal UUIDs. mapper.go renders list changes sorted and
  // comma-joined with no spaces (renderStringList), which is what the modal parses.
  const ENGLISH_LIST = '1c0f4e6a-1111-4111-8111-111111111111';
  const JAPANESE_LIST = '2d1a5f7b-2222-4222-8222-222222222222';

  it('renders a dropped curated spec list as an explicit removal, not a wall of UUIDs', async () => {
    vi.mocked(api.psaPropose).mockResolvedValue({
      pushId: 'push-2',
      diff: {
        changes: [{
          field: 'prepackagedSpecListIds',
          old: `${ENGLISH_LIST},${JAPANESE_LIST}`,
          new: ENGLISH_LIST,
          value: [ENGLISH_LIST],
        }],
      },
    });

    renderModal(makeCampaign({ targetLanguages: ['english'] }));
    fireEvent.click(screen.getByRole('button', { name: /check for changes/i }));

    await waitFor(() => {
      expect(screen.getByText('Curated spec lists')).toBeInTheDocument();
    });
    expect(screen.getByText('1 curated list will be REMOVED from this campaign.')).toBeInTheDocument();
    expect(screen.getByText('Target languages: English')).toBeInTheDocument();
    expect(screen.getByText(`Removed ${JAPANESE_LIST}`)).toBeInTheDocument();
    expect(screen.getByText('1 unchanged')).toBeInTheDocument();
    // The raw field name and blob rendering are gone for this field.
    expect(screen.queryByText('prepackagedSpecListIds')).not.toBeInTheDocument();
    expect(screen.queryByText(`${ENGLISH_LIST},${JAPANESE_LIST}`)).not.toBeInTheDocument();
  });

  it('renders an added curated spec list without a removal warning', async () => {
    vi.mocked(api.psaPropose).mockResolvedValue({
      pushId: 'push-3',
      diff: {
        changes: [{
          field: 'prepackagedSpecListIds',
          old: ENGLISH_LIST,
          new: `${ENGLISH_LIST},${JAPANESE_LIST}`,
          value: [ENGLISH_LIST, JAPANESE_LIST],
        }],
      },
    });

    renderModal(makeCampaign({ targetLanguages: ['english', 'japanese'] }));
    fireEvent.click(screen.getByRole('button', { name: /check for changes/i }));

    await waitFor(() => {
      expect(screen.getByText(`Added ${JAPANESE_LIST}`)).toBeInTheDocument();
    });
    expect(screen.getByText('Target languages: English, Japanese')).toBeInTheDocument();
    expect(screen.queryByText(/will be REMOVED/)).not.toBeInTheDocument();
  });

  it('describes an open-net campaign as such in the spec-list caption', async () => {
    vi.mocked(api.psaPropose).mockResolvedValue({
      pushId: 'push-4',
      diff: {
        changes: [{
          field: 'prepackagedSpecListIds',
          old: '',
          new: ENGLISH_LIST,
          value: [ENGLISH_LIST],
        }],
      },
    });

    renderModal(makeCampaign({ targetLanguages: [] }));
    fireEvent.click(screen.getByRole('button', { name: /check for changes/i }));

    await waitFor(() => {
      expect(screen.getByText('Target languages: any language (open net)')).toBeInTheDocument();
    });
    expect(screen.getByText(`Added ${ENGLISH_LIST}`)).toBeInTheDocument();
    expect(screen.queryByText(/unchanged/)).not.toBeInTheDocument();
  });
```

Run:

```bash
cd web && npx vitest run src/react/pages/campaign-detail/PSAPublishModal.test.tsx
```

Expected: the three new cases fail (the existing cases still pass), because the modal renders `prepackagedSpecListIds` through the generic scalar row:

```
 FAIL  src/react/pages/campaign-detail/PSAPublishModal.test.tsx > PSAPublishModal > renders a dropped curated spec list as an explicit removal, not a wall of UUIDs
TestingLibraryElementError: Unable to find an element with the text: Curated spec lists

 Test Files  1 failed (1)
      Tests  3 failed | 8 passed (11)
```

(The "8 passed" count is whatever the file already has; only the three new cases must fail.)

- [ ] **Step 5: Make it pass — the spec-list change row**

Create `web/src/react/pages/campaign-detail/SpecListChangeRow.tsx`:

```tsx
import type { FieldChange } from '../../../types/campaigns';
import { targetLanguageOptions } from '../../utils/campaignConstants';

/**
 * The diff field name the mapper emits for the curated-spec-list axis
 * (internal/domain/psacampaign/mapper.go, addList("prepackagedSpecListIds", …)).
 */
export const SPEC_LIST_FIELD = 'prepackagedSpecListIds';

/**
 * The mapper renders list-valued changes with renderStringList: sorted, comma
 * joined, no spaces. Splitting is therefore lossless.
 */
function parseIDList(rendered: string): string[] {
  return rendered.split(',').map(s => s.trim()).filter(Boolean);
}

/**
 * The campaign's own language axis, in words. This is NOT a per-id name: no
 * endpoint exposes the curated spec-list catalog to the browser (the catalog is
 * server-side only, read inside buildResolver), so the ids below are shown raw.
 * Adding GET /api/psa/spec-lists would let this component name each id.
 */
function languageSummary(tokens: string[]): string {
  if (tokens.length === 0) return 'any language (open net)';
  return tokens
    .map(t => targetLanguageOptions.find(o => o.value === t)?.label ?? t)
    .join(', ');
}

function IDChip({ prefix, id, tone }: { prefix: string; id: string; tone: 'danger' | 'success' }) {
  const toneVar = `var(--${tone})`;
  return (
    <span
      className="inline-flex items-baseline gap-1.5 rounded-[var(--radius-sm)] px-2 py-1 font-mono text-2xs break-all"
      style={{
        backgroundColor: `color-mix(in oklab, ${toneVar} 12%, transparent)`,
        color: 'var(--text)',
      }}
    >
      <span className="font-sans font-medium" style={{ color: toneVar }}>{prefix}</span>
      {id}
    </span>
  );
}

/**
 * Renders the curated-spec-list axis as a set diff instead of two comma-joined
 * UUID blobs. A dropped or swapped list is what this exists to make visible:
 * these lists decide what a live campaign spends money on.
 */
export default function SpecListChangeRow({
  change, targetLanguages,
}: {
  change: FieldChange;
  targetLanguages: string[];
}) {
  const before = parseIDList(change.old);
  const after = parseIDList(change.new);
  const beforeSet = new Set(before);
  const afterSet = new Set(after);
  const removed = before.filter(id => !afterSet.has(id));
  const added = after.filter(id => !beforeSet.has(id));
  const keptCount = after.length - added.length;

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[var(--text-subtle)] whitespace-nowrap">Curated spec lists</span>
        <span className="text-[var(--text-muted)] text-right">
          Target languages: {languageSummary(targetLanguages)}
        </span>
      </div>
      {removed.length > 0 && (
        <p className="text-[var(--danger)] font-medium">
          {removed.length === 1
            ? '1 curated list will be REMOVED from this campaign.'
            : `${removed.length} curated lists will be REMOVED from this campaign.`}
        </p>
      )}
      <div className="flex flex-col gap-1">
        {removed.map(id => <IDChip key={id} prefix="Removed" id={id} tone="danger" />)}
        {added.map(id => <IDChip key={id} prefix="Added" id={id} tone="success" />)}
      </div>
      {keptCount > 0 && (
        <span className="text-[var(--text-subtle)]">
          {keptCount === 1 ? '1 unchanged' : `${keptCount} unchanged`}
        </span>
      )}
    </div>
  );
}
```

Wire it into `web/src/react/pages/campaign-detail/PSAPublishModal.tsx`. Add after the existing import block (`:10`):

```tsx
import SpecListChangeRow, { SPEC_LIST_FIELD } from './SpecListChangeRow';
```

and replace the change list body at `:323-332` (the `effectiveDiff.changes.map(...)` call) with:

```tsx
                    {effectiveDiff.changes.map((change) => (
                      change.field === SPEC_LIST_FIELD ? (
                        <SpecListChangeRow
                          key={change.field}
                          change={change}
                          targetLanguages={campaign.targetLanguages}
                        />
                      ) : (
                        <div key={change.field} className="flex items-baseline justify-between gap-3">
                          <span className="text-[var(--text-subtle)] whitespace-nowrap">{change.field}</span>
                          <span className="tabular-nums text-right">
                            <span className="text-[var(--text-muted)]">{change.old}</span>
                            <span className="text-[var(--text-subtle)] mx-1.5">&rarr;</span>
                            <span className="text-[var(--text)] font-medium">{change.new}</span>
                          </span>
                        </div>
                      )
                    ))}
```

Run:

```bash
cd web && npx vitest run src/react/pages/campaign-detail/PSAPublishModal.test.tsx && npm run typecheck
```

Expected:

```
 Test Files  1 passed (1)
      Tests  11 passed (11)
```

then `tsc --noEmit` prints nothing.

- [ ] **Step 6: Commit the readable spec-list diff**

```bash
cd /workspace/.worktrees/psa-spec-list-targeting && git add web/src/react/pages/campaign-detail/SpecListChangeRow.tsx web/src/react/pages/campaign-detail/PSAPublishModal.tsx web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx && git commit -m "web: show curated spec-list changes as an explicit set diff"
```

Expected: pre-commit `go vet ./...` silent, commit succeeds.

- [ ] **Step 7: Write the failing legacy-subject test**

Append to `describe('SubjectListEditor', …)` in `web/src/react/ui/SubjectListEditor.test.tsx`:

```tsx
  it('flags legacy unreconciled subjects (id -1) without disturbing operator-typed ones (id 0)', async () => {
    // -1 is inventory.LegacyUnreconciledSubjectID, backfilled by migration
    // 000023 onto pre-axis subjects. Push translation refuses a campaign that
    // still carries one, so the reason has to be visible here. id 0 is the
    // unrelated, fully supported "resolve this name at push time" marker.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([
      { id: -1, name: 'Blastoise' },
      { id: 0, name: 'Mewtwo' },
      { id: 4807, name: 'Charizard' },
    ]);

    await waitFor(() => {
      expect(screen.getByText(/baseline pull/i)).toBeInTheDocument();
    });
    expect(screen.getByTitle('legacy subject — no portal id yet; run the harvester baseline pull')).toHaveTextContent('Blastoise');
    expect(screen.getByTitle('id: 0')).toHaveTextContent('Mewtwo');
    expect(screen.getByTitle('id: 4807')).toHaveTextContent('Charizard');

    // Legacy chips stay removable — removing one is a deliberate operator edit.
    fireEvent.click(screen.getByRole('button', { name: /remove blastoise/i }));
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }, { id: 4807, name: 'Charizard' }]);
  });
```

Run:

```bash
cd web && npx vitest run src/react/ui/SubjectListEditor.test.tsx
```

Expected: the new case fails, the eight existing ones pass:

```
 FAIL  src/react/ui/SubjectListEditor.test.tsx > SubjectListEditor > flags legacy unreconciled subjects (id -1) without disturbing operator-typed ones (id 0)
TestingLibraryElementError: Unable to find an element with the text: /baseline pull/i

 Test Files  1 failed (1)
      Tests  1 failed | 8 passed (9)
```

- [ ] **Step 8: Make it pass — distinct legacy chips**

In `web/src/react/ui/SubjectListEditor.tsx`, extend the import at `:5`:

```tsx
import type { SubjectRef } from '../../types/campaigns';
import { LEGACY_UNRECONCILED_SUBJECT_ID } from '../utils/campaignConstants';
```

Replace the chip list (`:120-146`, the `<div className="flex flex-wrap gap-1.5">` block through its closing `</div>`) with:

```tsx
      {value.some(s => s.id === LEGACY_UNRECONCILED_SUBJECT_ID) && (
        <p className="text-xs text-[var(--warning)]">
          Some subjects were carried over from before portal targeting and have no
          portal id yet. Publishing is refused until the harvester baseline pull
          reconciles them.
        </p>
      )}
      <div className="flex flex-wrap gap-1.5">
        {value.map((s, i) => {
          // -1 (legacy, unreconciled) and 0 (operator-typed, resolved by name at
          // push time) are different markers with different fates — only the
          // first one blocks a push, so only it is called out here.
          const isLegacy = s.id === LEGACY_UNRECONCILED_SUBJECT_ID;
          return (
            <span
              key={`${s.id}-${s.name}-${i}`}
              title={isLegacy ? 'legacy subject — no portal id yet; run the harvester baseline pull' : `id: ${s.id}`}
              className={
                isLegacy
                  ? 'inline-flex items-center gap-1 rounded-full bg-[var(--warning)]/15 text-[var(--warning)] text-xs px-2.5 py-1'
                  : 'inline-flex items-center gap-1 rounded-full bg-[var(--brand-500)]/15 text-[var(--brand-400)] text-xs px-2.5 py-1'
              }
            >
              {s.name}
              <button
                type="button"
                onClick={() => removeSubject(i)}
                aria-label={`Remove ${s.name}`}
                className="hover:text-[var(--danger)]"
              >
                ×
              </button>
            </span>
          );
        })}
      </div>
```

Nothing else in the file changes: `matches` still skips `s.id !== 0` at `:47`, `addSubject` still dedupes by name for id 0 at `:55-56`, and `handleKeyDown` still creates `{ id: 0, name: trimmed }` at `:73`.

Run:

```bash
cd web && npx vitest run src/react/ui/SubjectListEditor.test.tsx
```

Expected:

```
 Test Files  1 passed (1)
      Tests  9 passed (9)
```

- [ ] **Step 9: Full verification and commit**

```bash
cd web && npm run typecheck && npm run build && npm test
```

Expected: `tsc --noEmit` prints nothing; `vite build` ends `✓ built in …`; `vitest run` ends with every test file passing and `Test Files  N passed (N)` — no failed files, and in particular `CampaignFormFields.test.tsx` (9), `SubjectListEditor.test.tsx` (9), `PSAPublishModal.test.tsx` (11) and `CampaignsTab.test.tsx` all green.

Then:

```bash
cd /workspace/.worktrees/psa-spec-list-targeting && git add web/src/react/ui/SubjectListEditor.tsx web/src/react/ui/SubjectListEditor.test.tsx && git commit -m "web: mark legacy unreconciled subjects distinctly from operator-typed names"
```

Expected: pre-commit `go vet ./...` silent, commit succeeds. Do not pass `--no-verify`.

---

### Task 7: Docs + final verification gate

**Files:**
- Modify: `docs/SCHEMA.md` (the `campaigns` table rows at `:319-322`)
- Modify: `CLAUDE.md` (the migration paragraph under `## Database`)
- Modify: `docs/psa-harvester.md` (`## Baseline pull (one-time targeting migration)`, `:299-368`)

**Interfaces:**
- Consumes: the rewritten migration `000023_campaign_targeting_axes` (Task 1) — column
  `target_languages JSONB NOT NULL DEFAULT '[]'::jsonb` replacing `target_language TEXT`,
  and the negative-id sentinel used by its legacy-subject backfill; the `-baseline-pull`
  flag on `cmd/psa-harvest` and its skip causes (Task 5); the four in-sync copies of the
  language closed set (`internal/domain/inventory/validation.go`,
  `internal/domain/psacampaign/resolver.go`, `cmd/psa-harvest/baseline.go`,
  `web/src/react/utils/campaignConstants.ts`).
- Produces: nothing consumed by later tasks — this is the terminal documentation and
  verification task.

This task has no unit tests of its own: it edits prose and then runs every other task's
tests as one gate. The "failing test" for a docs task is the doc being wrong; each step
below states the exact text to replace and the exact check that proves it landed.

- [ ] **Step 1: Confirm the docs are in their pre-task state**

Run:

```bash
grep -n "target_language" docs/SCHEMA.md CLAUDE.md docs/psa-harvester.md
```

Expected: hits on `docs/SCHEMA.md:319`, and inside the `docs/psa-harvester.md` baseline
section (roughly `:302`, `:329`, `:335`, `:347`, `:363`). `CLAUDE.md` has no literal
`target_language` — it describes the migration in prose only. If `docs/SCHEMA.md` already
says `target_languages`, an earlier task overreached into this one; stop and reconcile
rather than editing twice.

- [ ] **Step 2: Update the `campaigns` rows in `docs/SCHEMA.md`**

Replace the `target_language` row (`docs/SCHEMA.md:319`) with:

```markdown
| `target_languages` | JSONB | NOT NULL DEFAULT '[]' | `[]string` — curated PSA spec-list language tokens this campaign buys: any of `'english'`, `'japanese'`. An **empty array is an open net** (buys any language); a non-empty array requires the card's classified language to be a member. Unordered set — order is not meaningful and must not be compared. Added migration 000023 |
```

Leave the `subject_filter_mode`, `subjects`, and `denied_specs` rows (`:320-322`) as they
are — they remain accurate. Extend only the `subjects` row's Notes to name the sentinel,
because migration 000023 now writes it and a reader of the schema has no other way to
learn that a negative id is not a real portal id:

```markdown
| `subjects` | JSONB | NOT NULL DEFAULT '[]' | `[]TargetSubject` (`{id, name}`) — character subjects this campaign targets or excludes, ids copied verbatim from the portal. Migration 000023's legacy backfill writes id `-1` as a sentinel meaning "legacy name, never reconciled against the portal"; push translation refuses a campaign containing sentinel entries until a baseline pull replaces them. Id `0` is distinct and means "operator-typed name awaiting name-based resolution". Added migration 000023 |
```

Verify:

```bash
grep -n "target_languages\|sentinel" docs/SCHEMA.md
```

Expected: the two rewritten rows, and no surviving `| \`target_language\` |` row.

- [ ] **Step 3: Correct the migration paragraph in `CLAUDE.md`**

The current text under `## Database` describes 000023 as adding "four new `campaigns`
columns replacing the inclusion/exclusion model with language, subject-mode, subject-list,
and denied-spec axes". The language axis is now multi-valued and 000023 is rewritten in
place rather than superseded, so a reader must not be led to expect a follow-up migration.
Replace the paragraph with:

```markdown
Migration files: `internal/adapters/storage/postgres/migrations/`. `000001_initial_schema`
represents the final-state schema after cutover from SQLite; subsequent migrations are
incremental (Supabase index/RLS fixes, hot-query indexes, `resolved_at` indexes, DH push
plumbing, MM grade-mismatch repair, and the dead-code cleanups dropping
`advisor_cache` (000013) and `psa_exchange_policy` (000014)); most recently
`campaign_targeting_axes` (000023 — four new `campaigns` columns replacing the
inclusion/exclusion model with a multi-valued language axis (`target_languages` JSONB,
empty = open net), subject-mode, subject-list, and denied-spec axes; its legacy subject
backfill marks unreconciled rows with a negative sentinel id)
and `psa_portal_catalog` (000024 — persisted PSA spec-list/subject reference data so the
main server can resolve portal identifiers without a portal session).
```

Verify:

```bash
grep -n "target_languages" CLAUDE.md
```

Expected: one hit, inside the migration paragraph.

- [ ] **Step 4: Rewrite the baseline-pull section of `docs/psa-harvester.md`**

This is the highest-stakes doc in the change: it is what the operator follows against six
live, money-spending campaigns. Everything not listed below — the zero-portal-writes
claim, the portal-UI spot check, the `psa_campaign_push_queue` check, and the deferred
spec-discovery note — was verified correct against shipped code. Preserve it verbatim and
change only what this work actually invalidates.

Replace the intro paragraph (`docs/psa-harvester.md:301-305`) and the first three
checklist bullets (`:318-364`). Leave the `docker run` block, the fourth bullet (`:365`),
and `### Deferred: spec discovery` untouched.

New intro paragraph:

```markdown
`cmd/psa-harvest -baseline-pull` performs the one-time copy of live portal targeting
(languages, subject list, denied specs) into `campaigns.target_languages` /
`subject_filter_mode` / `subjects` / `denied_specs`. A campaign may carry more than one
curated language list — all six live campaigns carry both "English Pokemon" and "Japanese
Pokemon" — and every recognized list is copied, not collapsed to one. It makes **zero
portal writes** — the flag returns before `DrainPushQueue` runs. Run it once, review the
report, and only then resume the normal (non-baseline) scheduled harvest.
```

New first three bullets:

```markdown
- [ ] Run the baseline pull once and confirm it **exits zero**. A non-zero exit most often
      means `runBaselinePull` (`cmd/psa-harvest/baseline.go`) skipped at least one linked
      campaign: its edit-form fetch was incomplete (`TargetingComplete == false`); it
      carries a curated spec-list name SlabLedger does not model, which is **refused, never
      silently dropped**, and is named in the log line (add the token in
      `internal/domain/inventory/validation.go`, `internal/domain/psacampaign/resolver.go`,
      `cmd/psa-harvest/baseline.go`, and `web/src/react/utils/campaignConstants.ts`
      together — the closed set is duplicated across all four); it names no recognized
      curated list at all (the CATEGORY-era shape, converted by hand in the portal); its
      portal targeting failed validation (e.g. a `subjectFilterType` that is neither
      `Target` nor `Exclude`); or it never appeared in the portal fetch. Any of these
      leaves that campaign's row unwritten (its pre-baseline targeting is left in place,
      never blanked out) — re-run until clean before trusting the copy. A non-zero exit can
      also mean the whole run aborted on an ordinary database failure (`ListCampaigns` or
      the campaign write returning an error) rather than a per-campaign skip; check the log
      line immediately before the exit to tell which case you're in.
- [ ] For at least one named, currently-linked campaign, open its edit page in the PSA
      portal UI directly and confirm the pulled `target_languages` / `subject_filter_mode` /
      `subjects` / `denied_specs` match what the portal UI shows — in particular that a
      campaign showing **both** "English Pokemon" and "Japanese Pokemon" landed with both
      tokens, not one. This is the check for a silently wrong translation, not just a
      successful fetch.
- [ ] Re-run the baseline a second time against campaigns whose portal targeting you know
      is unchanged, and confirm the copy is idempotent by diffing a direct snapshot of the
      affected columns from before and after. `runBaselinePull` has no diff or dedup logic
      of its own — it unconditionally rewrites `target_languages` / `subject_filter_mode` /
      `subjects` / `denied_specs` on every linked, complete campaign — so this is the only
      way to catch a real regression before trusting it for six active, money-spending
      campaigns:
      ```sql
      -- Before the second run: target_languages/subjects/denied_specs are all unordered
      -- sets, so each is sorted before comparison and the diff is order-insensitive,
      -- mirroring psacampaign/mapper.go's renderSubjectRefs (which exists precisely because
      -- "an unordered portal response never produces a spurious diff" — the edit-form fetch
      -- this baseline reads is not guaranteed order-stable across calls either).
      SELECT
        id,
        COALESCE((SELECT jsonb_agg(elem ORDER BY elem #>> '{}')
                  FROM jsonb_array_elements(target_languages) elem), '[]'::jsonb)
          AS target_languages_sorted,
        subject_filter_mode,
        COALESCE((SELECT jsonb_agg(elem ORDER BY (elem->>'id')::int)
                  FROM jsonb_array_elements(subjects) elem), '[]'::jsonb) AS subjects_sorted,
        COALESCE((SELECT jsonb_agg(elem ORDER BY (elem->>'id')::int)
                  FROM jsonb_array_elements(denied_specs) elem), '[]'::jsonb) AS denied_specs_sorted
      FROM campaigns
      WHERE psa_campaign_request_id IS NOT NULL AND psa_campaign_request_id <> ''
      ORDER BY id;
      -- (save this output, e.g. psql ... > before.txt)
      ```
      `elem #>> '{}'` extracts each language element as text, since `target_languages`
      holds bare JSON strings rather than objects with an `id`. Run `-baseline-pull` again,
      then run the identical query into `after.txt` and `diff before.txt after.txt`.
      Because all three arrays are sorted in both snapshots, a plain portal-side reorder of
      the same set collapses to identical output and won't show up as a diff — the *set* is
      the real signal, not raw array order. Any remaining difference on a campaign whose
      portal targeting genuinely did not change (added/removed/changed id, a gained or lost
      language token, or a different `subject_filter_mode`) is a real bug and must be fixed
      before trusting this baseline.
```

Verify:

```bash
grep -c "target_language\b" docs/psa-harvester.md
```

Expected: `0` — every occurrence is now the plural column. Then:

```bash
grep -n "zero portal writes\|Deferred: spec discovery\|psa_campaign_push_queue" docs/psa-harvester.md
```

Expected: all three still present — proof the preserved content survived the rewrite.

- [ ] **Step 5: Reset the dev database so the rewritten 000023 re-applies**

Migration 000023 is rewritten in place, not superseded. Any dev or devcontainer database
that already applied the old version holds `target_language TEXT` and a `subjects` backfill
using literal id `0` — the wrong schema and the wrong data, with `schema_migrations.version`
already past 23, so startup will never re-run it. Reset it by hand.

**Do not run `migrate down` against the rewritten files.** The rewritten
`000023_campaign_targeting_axes.down.sql` drops `target_languages`, a column the
old-schema database does not have; the rollback errors out and leaves
`schema_migrations.dirty = true`, which blocks every subsequent startup. Use the literal
DDL below, which matches the schema actually on disk.

```bash
export DATABASE_URL="postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable"

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
ALTER TABLE campaigns
  DROP COLUMN IF EXISTS denied_specs,
  DROP COLUMN IF EXISTS subjects,
  DROP COLUMN IF EXISTS subject_filter_mode,
  DROP COLUMN IF EXISTS target_language,
  DROP COLUMN IF EXISTS target_languages;
DROP TABLE IF EXISTS psa_portal_catalog;
UPDATE schema_migrations SET version = 22, dirty = false;
SQL
```

Expected: `ALTER TABLE`, `DROP TABLE`, `UPDATE 1`.

Then let the app re-apply 000023 and 000024 on startup:

```bash
go build -o slabledger ./cmd/slabledger && ./slabledger
```

Expected: startup logs the migration run and the server binds `:8081`. Stop it, then
confirm the new schema:

```bash
psql "$DATABASE_URL" -c "\d campaigns" | grep -E "target_language|subjects|denied_specs"
psql "$DATABASE_URL" -c "SELECT version, dirty FROM schema_migrations;"
```

Expected: `target_languages | jsonb | not null default '[]'::jsonb` (and **no**
`target_language`), plus the three other axis columns; `version = 24`, `dirty = f`.

**Production has never run this migration.** The branch is undeployed — prod's
`schema_migrations` sits at 22 and has no `campaigns` targeting columns at all. There is no
production migration concern, no data to repair, and no forward-fix migration needed. The
reset above is a dev-machine-only chore.

The throwaway test database (`slabledger_test`, used by `make test-postgres`) needs no
manual reset: that suite drops schemas and migrates from scratch on every run.

- [ ] **Step 6: Verify the four copies of the language closed set are in sync**

The real import cycle (`psacampaign/mapper.go` imports `inventory`, so `inventory` cannot
import `psacampaign`) forces the language closed set to be duplicated four ways. Nothing in
the build catches a drifted copy — only this check does.

```bash
grep -n -A6 "validTargetLanguages = map" internal/domain/inventory/validation.go
grep -n -A5 "languageListNames = map" internal/domain/psacampaign/resolver.go
grep -n -B2 -A2 '"Japanese Pokemon"' cmd/psa-harvest/baseline.go
grep -n -A6 "targetLanguageOptions" web/src/react/utils/campaignConstants.ts
```

Expected: all four print exactly the tokens `english` and `japanese` and nothing else —
`validTargetLanguages` has two entries (the empty-string "unset" entry is gone; an empty
*set* is now the open net, not an empty token), `languageListNames` maps two tokens to
`"English Pokemon"` / `"Japanese Pokemon"`, `baseline.go` switches on those same two portal
names, and `targetLanguageOptions` lists two selectable options.

Then confirm no fifth token crept into any copy:

```bash
grep -rniE "chinese|korean" \
  internal/domain/inventory/validation.go \
  internal/domain/psacampaign/resolver.go \
  cmd/psa-harvest/baseline.go \
  web/src/react/utils/campaignConstants.ts
```

Expected: only comment lines (e.g. the note in `resolver.go` that the portal offers no
curated Chinese/Korean list). Any hit on a map key, a `case` label, or an option object is
a drifted copy and must be fixed — per the locked decision, there is no `chinese` token
yet, and its eventual arrival is meant to surface as a loud baseline refusal naming the
unrecognized list, not as a half-wired token.

Note that `cardutil.SetLanguage` classifies more languages than this set contains — that is
correct and not drift. `cardutil` answers "what language is this card?"; the closed set
answers "what curated portal lists do we model?". A Chinese card classified by `cardutil`
simply matches no campaign whose `target_languages` lacks a Chinese token.

- [ ] **Step 7: Run the full verification gate**

Run: `go build ./...`
Expected: PASS, no output.

Run: `go test -race -timeout 10m ./...`
Expected: PASS, all packages `ok` or `no test files`.

Run: `make check`
Expected: PASS — `golangci-lint run`, then `scripts/check-imports.sh` (domain must not
import adapters; `inventory` must not import `psacampaign`; flat-sibling rule holds across
arbitrage/portfolio/tuning/finance/export/dhlisting), then `scripts/check-file-size.sh`
(warn at 500, hard fail at 600). Watch
`internal/domain/inventory/types_core.go` specifically: it is at 538 lines before this work
and this change touches it. If it crosses 600 the gate FAILS; the remedy is a **pure-move**
split along the campaign / purchase / sale / finance type families — move type
declarations into sibling files in the same package, change no behavior, and re-run
`go test -race ./internal/domain/inventory/...` to prove the move was inert.

Run: `make test-postgres`
Expected: PASS against the real `slabledger_test` database, taking seconds, not
milliseconds.

**This step cannot be replaced by `go test ./internal/adapters/storage/postgres/...`.**
Without `POSTGRES_TEST_URL` set, that package **skips** and returns in about `0.01s` with a
green `ok` line. A skip looks exactly like a pass and proves nothing — it would leave the
`target_languages` round-trip and the `null`→`[]` read-path guard completely unverified.
Use the Makefile target, which provisions the dedicated throwaway database and sets
`POSTGRES_TEST_URL` for you, and sanity-check the elapsed time in the output before
believing it.

Run: `cd web && npm run typecheck && npm run build && npm test`
Expected: PASS — `tsc --noEmit` reports no type errors, `vite build` completes, full
Vitest suite green.

`npm run typecheck` is not optional here and must not be dropped from this gate.
`npm run build` is `vite build` (`web/package.json:11`), which uses esbuild: it strips
types without checking them and therefore passes on a tree full of type errors. Type
checking lives only in the separate `typecheck` script (`web/package.json:14`). A gate
of `build && test` alone reports green on TypeScript drift — which is exactly what the
preceding branch's gate did.

If any command fails, fix the failure in the task that owns the affected file and re-run
this step — do not weaken a test or delete a check to make the gate pass.

- [ ] **Step 8: Commit the docs**

```bash
git add docs/SCHEMA.md CLAUDE.md docs/psa-harvester.md
git commit -m "docs: multi-valued campaign language axis, sentinel-marked legacy subjects, baseline-pull checklist"
```

Expected: `.githooks/pre-commit` runs `go vet ./...` and the commit succeeds. Never
`--no-verify`.

---
