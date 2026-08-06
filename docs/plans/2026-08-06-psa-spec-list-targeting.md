# PSA Spec-List Targeting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace SlabLedger's single `InclusionList`/`ExclusionMode` field pair with the three independent targeting axes the PSA portal now exposes — curated spec list (language), character subjects with Target/Exclude polarity, and card-level denied specs — then make SlabLedger the authoritative source that pushes complete campaign config to the portal.

**Architecture:** Targeting lives on `inventory.Campaign` as three axes persisted in Postgres. The portal's own vocabulary (curated-list UUIDs, subject IDs) is a *catalog* that the out-of-process `cmd/psa-harvest` binary fetches and writes to a `psa_portal_catalog` table; the main HTTP server reads that table and builds a pure, I/O-free `Resolver` to translate internal targeting into portal form data. A one-time `-baseline-pull` run seeds SlabLedger from live portal state with zero portal writes; after that, pushes flow one way.

**Tech Stack:** Go 1.26, hexagonal architecture, Postgres via `jackc/pgx/v5/stdlib`, `golang-migrate/migrate/v4` with embedded FS, Playwright/Chromium (harvester only), React + TypeScript + Vite.

## Global Constraints

- **Zero portal writes during the baseline pull.** The baseline reads live portal targeting and writes it into SlabLedger only. It must return before `DrainPushQueue` (`cmd/psa-harvest/main.go:121`). This is a binding requirement from the operator, not a preference.
- **The process boundary is real.** Translation runs in the main HTTP server (`internal/adapters/httpserver/handlers/campaigns_psa.go:137`, `:264`), which has no browser session and no portal access. Portal I/O runs only in `cmd/psa-harvest`. A translator must never call the portal.
- **Portal-sourced IDs are copied verbatim, never re-resolved.** Live subject IDs span 4xxx/8xxx/22xxx generations while `getSubjects` returns only 22xxx. Name→ID resolution applies solely to new subjects the operator adds by name (ID 0).
- **Fail closed on incomplete data.** A campaign whose edit-form fetch failed carries `TargetingComplete: false` and must be skipped, not written as zero targeting. A stale catalog (older than `CatalogMaxAge` = 7 days) fails translation with `ErrCatalogStale` rather than guessing.
- **Not a permanently mixed fleet.** Every CATEGORY campaign converts to SPEC_LIST. No long-term dual-mode support is in scope.
- **Hexagonal invariants** (`scripts/check-imports.sh`): `internal/domain/**` must not import `internal/adapters/**`; `internal/adapters/storage/**` must not import `internal/adapters/clients/**`; the flat-sibling rule covers arbitrage, portfolio, tuning, finance, export, dhlisting only. `internal/domain/**` may import `internal/platform/**`.
- **File size** (`scripts/check-file-size.sh`): warn at 500 lines, fail at 600. Excludes `_test.go` and mocks.
- **Money:** cents internally, whole USD on the portal wire — `centsToWholeUSD(cents) = (cents + 50) / 100`.
- **Testing:** table-driven tests with `[]struct`; mocks only from `internal/testutil/mocks/` using the Fn-field pattern, never inline; sentinel errors asserted with `errors.Is`. Run `go test -race` before every commit.
- **Type sync:** TS types in `web/src/types/` are hand-maintained to mirror Go struct JSON tags. The Go tags are authoritative.
- **Canonical language tokens:** `"english"`, `"japanese"`, `"chinese"`, `"korean"`. An empty `TargetLanguage` is an open net. "Not Japanese" never means "English".
- **Subject filter modes:** `"Target"` and `"Exclude"`, matching the portal's own `subjectFilterType` values. Empty normalizes to `"Target"` on read.

---


### Task 1: Language classification in cardutil

**Files:**
- Modify: `internal/platform/cardutil/normalize_sets.go:349-355` (append after `IsChineseSet`)
- Test: `internal/platform/cardutil/set_language_test.go`

**Interfaces:**
- Consumes: none (pure package, no dependencies).
- Produces: `LangEnglish`, `LangJapanese`, `LangChinese`, `LangKorean` constants; `IsJapaneseSet(setName string) bool`; `IsKoreanSet(setName string) bool`; `SetLanguage(setName string) string`. `IsChineseSet` already exists at `normalize_sets.go:352` and is reused, not duplicated. Task 3's `LanguageAxisMatches` calls `cardutil.SetLanguage`.

The bug being fixed: an earlier draft classified language as "not Japanese ⇒ English", which misclassifies real Simplified/Traditional Chinese certs (and would do the same to Korean). `SetLanguage` instead checks positive markers in order — japanese, then chinese, then korean — and only falls back to english once all three are ruled out. Korean has no precedent anywhere in this repo (`IsChineseSet`/the japanese prefix check are the only two markers that exist today); this task defines it as the same shape as the other two markers: a `"korean "` prefix, or `"korean"` appearing anywhere in the set name.

- [ ] **Step 1: Write the failing test**

```go
package cardutil

import "testing"

func TestSetLanguage(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		want    string
	}{
		{
			name:    "simplified chinese gem pack vol 1, cert 130221147",
			setName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1",
			want:    LangChinese,
		},
		{
			name:    "simplified chinese gem pack vol 2, cert 123238115",
			setName: "SIMPLIFIED CHINESE CBB2 C-GEM PACK VOL 2",
			want:    LangChinese,
		},
		{
			name:    "japanese mega symphonia, cert 139414865",
			setName: "JAPANESE M1S-MEGA SYMPHONIA",
			want:    LangJapanese,
		},
		{
			name:    "japanese shiny treasure ex, cert 132537172",
			setName: "JAPANESE SV4a-SHINY TREASURE ex",
			want:    LangJapanese,
		},
		{
			name:    "korean prefix (no repo precedent; convention defined by this task)",
			setName: "KOREAN S1-SWORD SHIELD",
			want:    LangKorean,
		},
		{
			name:    "korean marker mid-string",
			setName: "2024 POKEMON KOREAN PROMO CARD",
			want:    LangKorean,
		},
		{
			name:    "plain english promo, cert 72973327",
			setName: "SWSH BLACK STAR PROMO",
			want:    LangEnglish,
		},
		{
			name:    "plain english set, cert 145396462",
			setName: "CELEBRATIONS CLASSIC COLLECTION",
			want:    LangEnglish,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SetLanguage(tt.setName); got != tt.want {
				t.Errorf("SetLanguage(%q) = %q, want %q", tt.setName, got, tt.want)
			}
		})
	}
}

func TestIsJapaneseSet(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		want    bool
	}{
		{"japanese prefix", "JAPANESE M1S-MEGA SYMPHONIA", true},
		{"english set", "SWSH BLACK STAR PROMO", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsJapaneseSet(tt.setName); got != tt.want {
				t.Errorf("IsJapaneseSet(%q) = %v, want %v", tt.setName, got, tt.want)
			}
		})
	}
}

func TestIsKoreanSet(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		want    bool
	}{
		{"korean prefix", "KOREAN S1-SWORD SHIELD", true},
		{"korean contains", "2024 POKEMON KOREAN PROMO CARD", true},
		{"english set", "SWSH BLACK STAR PROMO", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKoreanSet(tt.setName); got != tt.want {
				t.Errorf("IsKoreanSet(%q) = %v, want %v", tt.setName, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/cardutil/... -run TestSetLanguage -v`
Expected: FAIL to compile — `undefined: LangChinese` (and the other new identifiers).

- [ ] **Step 3: Write the implementation**

Append to `internal/platform/cardutil/normalize_sets.go`, directly after `IsChineseSet` (`:349-355`):

```go
// Language tokens. These are the canonical values stored in
// inventory.Campaign.TargetLanguage and matched against set names.
const (
	LangEnglish  = "english"
	LangJapanese = "japanese"
	LangChinese  = "chinese"
	LangKorean   = "korean"
)

// IsJapaneseSet returns true if the set name carries PSA's "Japanese " marker
// prefix. This mirrors the unexported check already used inside
// normalizeSetNameBase (normalize_sets.go:116), exported here so matching can
// use it as a positive language marker instead of a negation.
func IsJapaneseSet(setName string) bool {
	return strings.HasPrefix(strings.ToLower(setName), "japanese ")
}

// IsKoreanSet returns true if the set name carries a "Korean " marker prefix,
// or contains "korean" anywhere, mirroring the shape of IsChineseSet. There is
// no Korean-marked cert anywhere in this repo today to confirm PSA's exact
// convention, so this follows the same prefix-or-contains pattern PSA already
// uses for Japanese and Chinese sets.
func IsKoreanSet(setName string) bool {
	lower := strings.ToLower(setName)
	return strings.HasPrefix(lower, "korean ") || strings.Contains(lower, "korean")
}

// SetLanguage classifies a set name into one of the Lang* tokens. Order
// matters: japanese, then chinese, then korean are checked first because each
// carries its own positive marker; english is the fallback only once all
// three are ruled out — it is never derived by negating a single marker,
// which is the bug this function fixes (a Simplified Chinese set is not
// merely "not Japanese").
func SetLanguage(setName string) string {
	switch {
	case IsJapaneseSet(setName):
		return LangJapanese
	case IsChineseSet(setName):
		return LangChinese
	case IsKoreanSet(setName):
		return LangKorean
	default:
		return LangEnglish
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/platform/cardutil/... -run 'TestSetLanguage|TestIsJapaneseSet|TestIsKoreanSet' -v`
Expected: PASS

- [ ] **Step 5: Audit real `set_name` data against the English default**

The `default: LangEnglish` branch above is an **assumption**, and the spec calls
it "the one thing here that must be checked against data before this ships"
(`docs/specs/2026-08-06-psa-spec-list-targeting-design.md:485-494`). It holds
only if every non-English set in inventory carries a marker. A non-English set
with no marker would classify as `english` and match an English-targeted
campaign — buying the wrong language silently.

Run the audit. `SUPABASE_DB_URL` is in `.env`; it contains a password, so do not
echo it or paste query output that includes it:

```bash
set -a && . ./.env && set +a
psql "$SUPABASE_DB_URL" -At -c "
  SELECT DISTINCT set_name
  FROM campaign_purchases
  WHERE set_name IS NOT NULL
    AND set_name <> ''
    AND set_name !~* '(japanese|chinese|korean)'
  ORDER BY 1;
"
```

Read the result and decide:

- **Every row is genuinely an English set** → the default holds. Record the date
  and the row count in the commit message and move on. This is the expected
  outcome.
- **Any row is a non-English set with no marker** → the default is unsafe for it.
  Do **not** widen the marker lists to paper over it. Instead add the observed
  marker to the matching `IsXSet` helper if it is a real, recurring marker; if
  the set name carries no usable signal at all, change `SetLanguage`'s `default`
  branch to return `""` for that shape and add a test case asserting `""`. Then
  confirm the fail-closed path: `LanguageAxisMatches` returns `false` when
  `SetLanguage` yields `""` and `targetLanguage` is non-empty, so a
  language-constrained campaign declines the card rather than guessing.

This step is a gate, not a formality — the spec ties the whole English default to
its outcome. If the audit cannot be run (no DB reachable), say so explicitly and
do not claim Task 1 complete.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/cardutil/normalize_sets.go internal/platform/cardutil/set_language_test.go
git commit -m "feat: add positive-marker language classification to cardutil"
```

---

### Task 2: Three targeting axes on inventory.Campaign + persistence

**Files:**
- Modify: `internal/domain/inventory/types_core.go:161-183` (Campaign struct, new `TargetSubject` type, new consts)
- Create: `internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.down.sql`
- Modify: `internal/adapters/storage/postgres/campaign_store.go` (all 4 queries: `CreateCampaign` insert cols/args, `GetCampaign`, `ListCampaigns`, `UpdateCampaign`)
- Test: `internal/adapters/storage/postgres/campaign_store_test.go` (extend), `internal/adapters/storage/postgres/migrations_test.go` if present, else a new migration assertion added to `campaign_store_test.go`

**Interfaces:**
- Consumes: none new from other tasks.
- Produces: `inventory.TargetSubject{ID int; Name string}`; `Campaign.TargetLanguage string`, `Campaign.SubjectFilterMode string`, `Campaign.Subjects []TargetSubject`, `Campaign.DeniedSpecs []TargetSubject`; `inventory.SubjectFilterTarget = "Target"`, `inventory.SubjectFilterExclude = "Exclude"`. Task 3's matching rewrite reads all four fields directly off `*Campaign`.

`InclusionList string` and `ExclusionMode bool` are **kept** on `Campaign` this cycle (legacy mirror, no consumers read them after Task 4 lands — they exist purely so a rollback to the previous binary sees a correct database). `campaign_store.go` becomes the sole writer of both: it derives them from `Subjects`/`SubjectFilterMode` on every write rather than trusting whatever the caller happened to set on those two legacy fields. `SubjectFilterMode` empty string is normalized to `SubjectFilterTarget` both on read (defensive, since a pre-migration row or a caller-constructed `Campaign{}` literal may leave it unset) and on write (so the derived `exclusion_mode` boolean and the stored `subject_filter_mode` column agree, rather than relying on the column's `DEFAULT 'Target'` — which the store bypasses anyway since it supplies the column explicitly in every INSERT).

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/storage/postgres/campaign_store_test.go`:

```go
func TestCampaignStore_TargetingAxesRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	repo := NewCampaignStore(db.DB, logger)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	c := &inventory.Campaign{
		ID:                "camp-axes",
		Name:              "Japanese Pokemon",
		Sport:             "Pokemon",
		BuyTermsCLPct:     0.80,
		Phase:             inventory.PhaseActive,
		TargetLanguage:    "japanese",
		SubjectFilterMode: inventory.SubjectFilterExclude,
		Subjects: []inventory.TargetSubject{
			{ID: 22210, Name: "Machamp"},
			{ID: 8105, Name: "Crystal Golem"},
		},
		DeniedSpecs: []inventory.TargetSubject{
			{ID: 4807, Name: "Charizard"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, repo.CreateCampaign(ctx, c))

	got, err := repo.GetCampaign(ctx, "camp-axes")
	require.NoError(t, err)
	assert.Equal(t, "japanese", got.TargetLanguage)
	assert.Equal(t, inventory.SubjectFilterExclude, got.SubjectFilterMode)
	assert.Equal(t, c.Subjects, got.Subjects)
	assert.Equal(t, c.DeniedSpecs, got.DeniedSpecs)

	// Legacy mirror is derived from the new fields on write, not from whatever
	// the caller happened to leave on InclusionList/ExclusionMode.
	assert.Equal(t, "Machamp,Crystal Golem", got.InclusionList)
	assert.Equal(t, true, got.ExclusionMode)

	// Update flips polarity and subjects; mirror must be re-derived.
	c.SubjectFilterMode = inventory.SubjectFilterTarget
	c.Subjects = []inventory.TargetSubject{{ID: 100, Name: "Pikachu"}}
	c.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.UpdateCampaign(ctx, c))

	got, err = repo.GetCampaign(ctx, "camp-axes")
	require.NoError(t, err)
	assert.Equal(t, inventory.SubjectFilterTarget, got.SubjectFilterMode)
	assert.Equal(t, c.Subjects, got.Subjects)
	assert.Equal(t, "Pikachu", got.InclusionList)
	assert.Equal(t, false, got.ExclusionMode)

	// A row with an empty subject_filter_mode (simulating a pre-migration or
	// otherwise blank row) normalizes to SubjectFilterTarget on read.
	_, err = db.ExecContext(ctx,
		`UPDATE campaigns SET subject_filter_mode = '' WHERE id = $1`, "camp-axes")
	require.NoError(t, err)
	got, err = repo.GetCampaign(ctx, "camp-axes")
	require.NoError(t, err)
	assert.Equal(t, inventory.SubjectFilterTarget, got.SubjectFilterMode)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/storage/postgres/... -run TestCampaignStore_TargetingAxesRoundTrip -v`
Expected: FAIL to compile — `unknown field TargetLanguage in struct literal of type inventory.Campaign` (skips at runtime with "POSTGRES_TEST_URL not set" only once it compiles; run `make test-postgres` for the real round trip, per repo convention in `testhelper_test.go:19`).

- [ ] **Step 3: Write the implementation**

`internal/domain/inventory/types_core.go` — replace the `Campaign` struct (`:161-183`) with:

```go
// TargetSubject is one portal-sourced targeting entity: a character subject or
// a card-level spec. ID is copied verbatim from the portal and is never
// re-resolved from Name — live IDs span multiple generations (4xxx, 8xxx,
// 22xxx) while getSubjects returns only 22xxx.
type TargetSubject struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SubjectFilterMode values. Target buys only the listed subjects; Exclude
// buys everything except them.
const (
	SubjectFilterTarget  = "Target"
	SubjectFilterExclude = "Exclude"
)

// Campaign represents a PSA Direct Buy campaign with buy parameters and fee configuration.
type Campaign struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Sport                string    `json:"sport"`
	YearRange            string    `json:"yearRange"`          // e.g. "1999-2003"
	GradeRange           string    `json:"gradeRange"`         // e.g. "9-10"
	PriceRange           string    `json:"priceRange"`         // e.g. "50-500"
	CLConfidence         string    `json:"clConfidence"`       // CL confidence range, e.g. "2.5-4"
	BuyTermsCLPct        float64   `json:"buyTermsCLPct"`      // Buy at X% of CL value (0-1)
	DailySpendCapCents   int       `json:"dailySpendCapCents"` // Max daily spend in cents

	// InclusionList and ExclusionMode are a legacy mirror kept for one release
	// so a rollback to the previous binary sees a database that still matches
	// its own model. Nothing reads them after this change — campaign_store.go
	// derives both from Subjects/SubjectFilterMode on every write; they are
	// never read back into matching or coverage logic.
	InclusionList string `json:"inclusionList"`
	ExclusionMode bool   `json:"exclusionMode"`

	// TargetLanguage selects the PSA curated spec list the campaign buys from.
	// "" means unset (a legacy CATEGORY campaign, or a campaign not yet linked).
	TargetLanguage string `json:"targetLanguage"` // "" | "english" | "japanese" | "chinese" | "korean"

	// SubjectFilterMode is the polarity of Subjects: Target buys only the
	// listed characters, Exclude buys everything except them. Empty is
	// normalized to SubjectFilterTarget on read.
	SubjectFilterMode string `json:"subjectFilterMode"`

	// Subjects are the characters this campaign targets or excludes. ID is the
	// PSA subject id and is authoritative — it is never re-derived from Name.
	Subjects []TargetSubject `json:"subjects"`

	// DeniedSpecs are individual cards excluded regardless of Subjects.
	DeniedSpecs []TargetSubject `json:"deniedSpecs"`

	Phase                Phase     `json:"phase"`
	PSASourcingFeeCents  int       `json:"psaSourcingFeeCents"`            // Default 300 ($3)
	EbayFeePct           float64   `json:"ebayFeePct"`                     // Default 0.1235 (12.35%)
	ExpectedFillRate     float64   `json:"expectedFillRate"`               // Target fill rate as percentage (0-100)
	PSACampaignRequestID string    `json:"psaCampaignRequestId,omitempty"` // 1:1 link to PSA portal campaign
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Kind                 string    `json:"kind"` // Derived at HTTP layer: "external" or "standard" (not persisted)
}
```

`internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.up.sql`:

```sql
ALTER TABLE campaigns
  ADD COLUMN target_language      TEXT  NOT NULL DEFAULT '',
  ADD COLUMN subject_filter_mode  TEXT  NOT NULL DEFAULT 'Target',
  ADD COLUMN subjects             JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN denied_specs         JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Backfill: subject_filter_mode mirrors the existing polarity bool.
UPDATE campaigns
SET subject_filter_mode = CASE WHEN exclusion_mode THEN 'Exclude' ELSE 'Target' END;

-- Backfill: subjects from inclusion_list, split on the same rule as
-- inventory.SplitInclusionList (comma-or-whitespace runs, empty entries
-- dropped). Ids are placeholders (0) — the operator resolves them via a
-- baseline pull or the getSubjects resolver; see design doc §7.
UPDATE campaigns c
SET subjects = COALESCE(
    (
        SELECT jsonb_agg(jsonb_build_object('id', 0, 'name', tok) ORDER BY ord)
        FROM unnest(regexp_split_to_array(trim(c.inclusion_list), '[,\s]+')) WITH ORDINALITY AS t(tok, ord)
        WHERE tok <> ''
    ),
    '[]'::jsonb
)
WHERE c.inclusion_list IS NOT NULL AND trim(c.inclusion_list) <> '';
```

`internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.down.sql`:

```sql
ALTER TABLE campaigns
  DROP COLUMN denied_specs,
  DROP COLUMN subjects,
  DROP COLUMN subject_filter_mode,
  DROP COLUMN target_language;
```

`internal/adapters/storage/postgres/campaign_store.go` — full file:

```go
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CampaignStore implements campaign CRUD operations.
type CampaignStore struct {
	base
}

// NewCampaignStore creates a new campaign store.
func NewCampaignStore(db *sql.DB, logger observability.Logger) *CampaignStore {
	return &CampaignStore{base{db: db, logger: logger}}
}

var _ inventory.CampaignRepository = (*CampaignStore)(nil)

// deriveLegacyMirror computes the legacy inclusion_list/exclusion_mode
// columns from the authoritative Subjects/SubjectFilterMode fields. This
// store is the sole writer of both legacy columns; the mirror exists only so
// a rollback to the previous binary sees a correct database — nothing in the
// current binary reads it back.
func deriveLegacyMirror(c *inventory.Campaign) (inclusionList string, exclusionMode bool) {
	names := make([]string, 0, len(c.Subjects))
	for _, s := range c.Subjects {
		names = append(names, s.Name)
	}
	return strings.Join(names, ","), normalizeSubjectFilterMode(c.SubjectFilterMode) == inventory.SubjectFilterExclude
}

// normalizeSubjectFilterMode maps an empty mode to SubjectFilterTarget, both
// on write (so the derived legacy mirror agrees with what gets stored) and on
// read (defensive, in case a row predates this migration's DEFAULT).
func normalizeSubjectFilterMode(mode string) string {
	if mode == "" {
		return inventory.SubjectFilterTarget
	}
	return mode
}

// marshalTargetSubjects marshals a subject list to its JSONB wire form,
// falling back to an empty array on marshal failure (TargetSubject has no
// fields that can fail to marshal, so this is defensive only).
func marshalTargetSubjects(subjects []inventory.TargetSubject) string {
	b, err := json.Marshal(subjects)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func (cs *CampaignStore) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	inclusionList, exclusionMode := deriveLegacyMirror(c)
	subjectFilterMode := normalizeSubjectFilterMode(c.SubjectFilterMode)
	subjectsJSON := marshalTargetSubjects(c.Subjects)
	deniedJSON := marshalTargetSubjects(c.DeniedSpecs)

	query := `
		INSERT INTO campaigns (id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate,
			psa_campaign_request_id, created_at, updated_at,
			target_language, subject_filter_mode, subjects, denied_specs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22)
	`
	_, err := cs.db.ExecContext(ctx, query,
		c.ID, c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, inclusionList,
		exclusionMode, string(c.Phase), c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate,
		c.PSACampaignRequestID, c.CreatedAt, c.UpdatedAt,
		c.TargetLanguage, subjectFilterMode, subjectsJSON, deniedJSON,
	)
	if err != nil {
		return fmt.Errorf("create campaign: %w", err)
	}
	return nil
}

func (cs *CampaignStore) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	query := `
		SELECT id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate,
			COALESCE(psa_campaign_request_id, ''), created_at, updated_at,
			target_language, subject_filter_mode, subjects, denied_specs
		FROM campaigns WHERE id = $1
	`
	var c inventory.Campaign
	var subjectsJSON, deniedJSON string
	err := cs.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
		&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
		&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate,
		&c.PSACampaignRequestID, &c.CreatedAt, &c.UpdatedAt,
		&c.TargetLanguage, &c.SubjectFilterMode, &subjectsJSON, &deniedJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrCampaignNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(subjectsJSON), &c.Subjects); err != nil {
		return nil, fmt.Errorf("unmarshal subjects: %w", err)
	}
	if err := json.Unmarshal([]byte(deniedJSON), &c.DeniedSpecs); err != nil {
		return nil, fmt.Errorf("unmarshal denied specs: %w", err)
	}
	c.SubjectFilterMode = normalizeSubjectFilterMode(c.SubjectFilterMode)
	return &c, nil
}

func (cs *CampaignStore) ListCampaigns(ctx context.Context, activeOnly bool) (result []inventory.Campaign, err error) {
	query := `
		SELECT id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate,
			COALESCE(psa_campaign_request_id, ''), created_at, updated_at,
			target_language, subject_filter_mode, subjects, denied_specs
		FROM campaigns
	`
	if activeOnly {
		query += ` WHERE phase = 'active'`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := cs.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query campaigns: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	const campaignsInitialCapacity = 64
	result = make([]inventory.Campaign, 0, campaignsInitialCapacity)
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var c inventory.Campaign
		var subjectsJSON, deniedJSON string
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
			&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
			&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate,
			&c.PSACampaignRequestID, &c.CreatedAt, &c.UpdatedAt,
			&c.TargetLanguage, &c.SubjectFilterMode, &subjectsJSON, &deniedJSON,
		); err != nil {
			return nil, fmt.Errorf("scan campaign row: %w", err)
		}
		if err := json.Unmarshal([]byte(subjectsJSON), &c.Subjects); err != nil {
			return nil, fmt.Errorf("unmarshal subjects: %w", err)
		}
		if err := json.Unmarshal([]byte(deniedJSON), &c.DeniedSpecs); err != nil {
			return nil, fmt.Errorf("unmarshal denied specs: %w", err)
		}
		c.SubjectFilterMode = normalizeSubjectFilterMode(c.SubjectFilterMode)
		result = append(result, c)
	}
	return result, rows.Err()
}

func (cs *CampaignStore) DeleteCampaign(ctx context.Context, id string) (retErr error) {
	tx, err := cs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback() //nolint:errcheck // best-effort; error logged via retErr
		}
	}()

	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_sales WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = $1)`, id,
	); retErr != nil {
		return retErr
	}

	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_purchases WHERE campaign_id = $1`, id,
	); retErr != nil {
		return retErr
	}

	result, retErr := tx.ExecContext(ctx, `DELETE FROM campaigns WHERE id = $1`, id)
	if retErr != nil {
		return retErr
	}
	n, retErr := result.RowsAffected()
	if retErr != nil {
		return retErr
	}
	if n == 0 {
		return inventory.ErrCampaignNotFound
	}

	return tx.Commit()
}

func (cs *CampaignStore) UpdateCampaign(ctx context.Context, c *inventory.Campaign) error {
	inclusionList, exclusionMode := deriveLegacyMirror(c)
	subjectFilterMode := normalizeSubjectFilterMode(c.SubjectFilterMode)
	subjectsJSON := marshalTargetSubjects(c.Subjects)
	deniedJSON := marshalTargetSubjects(c.DeniedSpecs)

	query := `
		UPDATE campaigns SET name = $1, sport = $2, year_range = $3, grade_range = $4,
			price_range = $5, cl_confidence = $6, buy_terms_cl_pct = $7,
			daily_spend_cap_cents = $8, inclusion_list = $9, exclusion_mode = $10, phase = $11,
			psa_sourcing_fee_cents = $12, ebay_fee_pct = $13, expected_fill_rate = $14,
			psa_campaign_request_id = $15, updated_at = $16,
			target_language = $17, subject_filter_mode = $18, subjects = $19, denied_specs = $20
		WHERE id = $21
	`
	result, err := cs.db.ExecContext(ctx, query,
		c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, inclusionList,
		exclusionMode, string(c.Phase), c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate,
		c.PSACampaignRequestID, c.UpdatedAt,
		c.TargetLanguage, subjectFilterMode, subjectsJSON, deniedJSON, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update campaign: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrCampaignNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `POSTGRES_TEST_URL=<local test db dsn> go test -race ./internal/adapters/storage/postgres/... -run 'TestCampaignStore_CampaignCRUD|TestCampaignStore_TargetingAxesRoundTrip' -v` (or `make test-postgres`)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/types_core.go \
  internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.up.sql \
  internal/adapters/storage/postgres/migrations/000023_campaign_targeting_axes.down.sql \
  internal/adapters/storage/postgres/campaign_store.go \
  internal/adapters/storage/postgres/campaign_store_test.go
git commit -m "feat: add three-axis campaign targeting fields with legacy mirror"
```

---

### Task 3: Matching rewrite

**Files:**
- Modify: `internal/domain/inventory/matching.go` (full rewrite of `PurchaseMatchesCampaign`/`FindMatchingCampaign`; delete `inclusionListMatches` at `:125-139`; add `MatchInput`, `LanguageAxisMatches`, `SubjectAxisMatches`, `SpecDenied`)
- Modify: `internal/domain/inventory/service_import_psa.go:104-111` (sole external caller)
- Test: `internal/domain/inventory/matching_test.go` (full rewrite)

matching.go is 168 lines today; the rewrite adds roughly 60 lines net (three new small functions, one new struct) and stays under 230 lines — well inside the 500-line warning threshold, so no split into `matching_axes.go` is needed.

**Interfaces:**
- Consumes: `cardutil.SetLanguage` (Task 1); `Campaign.TargetLanguage`, `Campaign.SubjectFilterMode`, `Campaign.Subjects`, `Campaign.DeniedSpecs`, `SubjectFilterExclude` (Task 2).
- Produces: `MatchInput{Grade float64; BuyCostCents int; CardName string; SetName string; CardNumber string; PSASpecID int; CardYear int}`; `PurchaseMatchesCampaign(in MatchInput, c *Campaign) bool`; `FindMatchingCampaign(in MatchInput, allCampaigns []Campaign) MatchResult`; `SubjectAxisMatches(cardName string, subjects []TargetSubject, mode string) bool`; `LanguageAxisMatches(setName, targetLanguage string) bool`; `SpecDenied(in MatchInput, denied []TargetSubject) bool`. `SubjectAxisMatches` is also the coverage predicate a later task wires into `campaign_coverage.go` in place of the deleted `characterMatchesInclusion`. `ParseRange`, `SplitInclusionList`, and `MatchResult{CampaignID, Candidates, Status}` are unchanged and still exported from this package.

- [ ] **Step 1: Write the failing test**

Replace `internal/domain/inventory/matching_test.go` in full:

```go
package inventory

import "testing"

func TestParseRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
		wantOK  bool
	}{
		{"empty", "", 0, 0, false},
		{"valid grade range", "9-10", 9, 10, true},
		{"valid price range", "50-500", 50, 500, true},
		{"single value", "10-10", 10, 10, true},
		{"inverted", "10-5", 0, 0, false},
		{"no dash", "910", 0, 0, false},
		{"non-numeric", "abc-def", 0, 0, false},
		{"whitespace", " 9 - 10 ", 9, 10, true},
		{"zero based", "0-100", 0, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi, ok := ParseRange(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ParseRange(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && (lo != tt.wantMin || hi != tt.wantMax) {
				t.Errorf("ParseRange(%q) = (%d, %d), want (%d, %d)", tt.input, lo, hi, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestPurchaseMatchesCampaign(t *testing.T) {
	tests := []struct {
		name     string
		in       MatchInput
		campaign Campaign
		want     bool
	}{
		{
			name:     "no filters set - matches anything",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{},
			want:     true,
		},
		{
			name:     "grade in range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "9-10"},
			want:     true,
		},
		{
			name:     "grade out of range",
			in:       MatchInput{Grade: 7, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "9-10"},
			want:     false,
		},
		{
			name:     "half-grade 9.5 in range 9-10",
			in:       MatchInput{Grade: 9.5, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "9-10"},
			want:     true,
		},
		{
			name:     "price in range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{PriceRange: "50-500"},
			want:     true,
		},
		{
			name:     "price below range",
			in:       MatchInput{Grade: 9, BuyCostCents: 2000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{PriceRange: "50-500"},
			want:     false,
		},
		{
			name:     "price range scaled by buy terms - in range",
			in:       MatchInput{Grade: 10, BuyCostCents: 19799, CardName: "Umbreon EX", SetName: "Pokemon"},
			campaign: Campaign{PriceRange: "200-500", BuyTermsCLPct: 0.78},
			want:     true, // effective range: $156-$390
		},
		{
			name:     "price range scaled by buy terms - below range",
			in:       MatchInput{Grade: 10, BuyCostCents: 10000, CardName: "Umbreon EX", SetName: "Pokemon"},
			campaign: Campaign{PriceRange: "200-500", BuyTermsCLPct: 0.78},
			want:     false,
		},
		{
			name:     "malformed grade range rejects match",
			in:       MatchInput{Grade: 7, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			campaign: Campaign{GradeRange: "bad"},
			want:     false,
		},
		{
			name:     "cardYear inside campaign year range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set", CardYear: 2000},
			campaign: Campaign{YearRange: "1999-2003"},
			want:     true,
		},
		{
			name:     "cardYear outside campaign year range",
			in:       MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard", SetName: "Vivid Voltage", CardYear: 2020},
			campaign: Campaign{YearRange: "1999-2003"},
			want:     false,
		},
		{
			name: "language axis rejects mismatched set",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "SWSH BLACK STAR PROMO"},
			campaign: Campaign{
				TargetLanguage: "japanese",
			},
			want: false,
		},
		{
			name: "language axis accepts matching set",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Mega Gardevoir ex", SetName: "JAPANESE M1S-MEGA SYMPHONIA"},
			campaign: Campaign{
				TargetLanguage: "japanese",
			},
			want: true,
		},
		{
			name: "subject axis Target mode - matches",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterTarget,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: true,
		},
		{
			name: "subject axis Target mode - no match",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Blastoise", SetName: "Jungle"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterTarget,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: false,
		},
		{
			name: "subject axis Exclude mode - excluded card rejected",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterExclude,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: false,
		},
		{
			name: "subject axis Exclude mode - other card accepted",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Pikachu VMAX", SetName: "Vivid Voltage"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterExclude,
				Subjects:          []TargetSubject{{ID: 100, Name: "Charizard"}},
			},
			want: true,
		},
		{
			name: "empty subjects is an open net regardless of mode",
			in:   MatchInput{Grade: 9, BuyCostCents: 15000, CardName: "Anything", SetName: "Any Set"},
			campaign: Campaign{
				SubjectFilterMode: SubjectFilterExclude,
			},
			want: true,
		},
		{
			name: "denied spec by PSASpecID overrides a subject match",
			in: MatchInput{
				Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set", CardNumber: "004", PSASpecID: 4807,
			},
			campaign: Campaign{
				Subjects:    []TargetSubject{{ID: 100, Name: "Charizard"}},
				DeniedSpecs: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			},
			want: false,
		},
		{
			name: "denied spec by set+number fallback when PSASpecID is 0",
			in: MatchInput{
				Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set", CardNumber: "004",
			},
			campaign: Campaign{
				Subjects:    []TargetSubject{{ID: 100, Name: "Charizard"}},
				DeniedSpecs: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			},
			want: false,
		},
		{
			name: "no deny when neither identity is available (card number missing)",
			in: MatchInput{
				Grade: 9, BuyCostCents: 15000, CardName: "Charizard VMAX", SetName: "Base Set",
			},
			campaign: Campaign{
				Subjects:    []TargetSubject{{ID: 100, Name: "Charizard"}},
				DeniedSpecs: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			},
			want: true, // fail-open: PSASpecID/ID both 0, and CardNumber is empty so no composite key can be built
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PurchaseMatchesCampaign(tt.in, &tt.campaign)
			if got != tt.want {
				t.Errorf("PurchaseMatchesCampaign() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLanguageAxisMatches(t *testing.T) {
	tests := []struct {
		name           string
		setName        string
		targetLanguage string
		want           bool
	}{
		{"empty target is open net", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", "", true},
		{"japanese set matches japanese target", "JAPANESE M1S-MEGA SYMPHONIA", "japanese", true},
		{"chinese set does not match japanese target", "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1", "japanese", false},
		{"english set matches english target", "SWSH BLACK STAR PROMO", "english", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LanguageAxisMatches(tt.setName, tt.targetLanguage); got != tt.want {
				t.Errorf("LanguageAxisMatches(%q, %q) = %v, want %v", tt.setName, tt.targetLanguage, got, tt.want)
			}
		})
	}
}

func TestSubjectAxisMatches(t *testing.T) {
	tests := []struct {
		name     string
		cardName string
		subjects []TargetSubject
		mode     string
		want     bool
	}{
		{"empty list is open net in Target mode", "Charizard", nil, SubjectFilterTarget, true},
		{"empty list is open net in Exclude mode", "Charizard", nil, SubjectFilterExclude, true},
		{"Target mode matches", "Charizard VMAX", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterTarget, true},
		{"Target mode no match", "Blastoise", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterTarget, false},
		{"Exclude mode rejects listed", "Charizard VMAX", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterExclude, false},
		{"Exclude mode accepts unlisted", "Pikachu", []TargetSubject{{ID: 1, Name: "Charizard"}}, SubjectFilterExclude, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SubjectAxisMatches(tt.cardName, tt.subjects, tt.mode); got != tt.want {
				t.Errorf("SubjectAxisMatches(%q, %v, %q) = %v, want %v", tt.cardName, tt.subjects, tt.mode, got, tt.want)
			}
		})
	}
}

func TestSpecDenied(t *testing.T) {
	tests := []struct {
		name   string
		in     MatchInput
		denied []TargetSubject
		want   bool
	}{
		{
			name:   "denied by matching PSASpecID",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 4807},
			denied: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			want:   true,
		},
		{
			name:   "not denied when PSASpecID differs",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 100},
			denied: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "falls back to set+number composite when PSASpecID is 0",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 4807, Name: "Base Set 004"}},
			want:   true,
		},
		{
			name:   "falls back to set+number composite when denied entry id is 0",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 999},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   true,
		},
		{
			name:   "composite comparison is case-insensitive",
			in:     MatchInput{SetName: "base set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "BASE SET 004"}},
			want:   true,
		},
		{
			name:   "no match when card number differs",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 005"}},
			want:   false,
		},
		{
			name:   "fail-open when card number is missing (no composite key can be built)",
			in:     MatchInput{SetName: "Base Set"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "fail-open when set name is missing (no composite key can be built)",
			in:     MatchInput{CardNumber: "004"},
			denied: []TargetSubject{{ID: 0, Name: "Base Set 004"}},
			want:   false,
		},
		{
			name:   "empty deny list never denies",
			in:     MatchInput{SetName: "Base Set", CardNumber: "004", PSASpecID: 4807},
			denied: nil,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpecDenied(tt.in, tt.denied); got != tt.want {
				t.Errorf("SpecDenied(%+v, %v) = %v, want %v", tt.in, tt.denied, got, tt.want)
			}
		})
	}
}

func TestFindMatchingCampaign(t *testing.T) {
	campaignA := Campaign{
		ID:         "campaign-a",
		Name:       "High Grade",
		GradeRange: "9-10",
		PriceRange: "50-500",
	}
	campaignB := Campaign{
		ID:         "campaign-b",
		Name:       "Low Grade",
		GradeRange: "7-8",
		PriceRange: "10-100",
	}
	campaignC := Campaign{
		ID:                "campaign-c",
		Name:              "Pokemon Only",
		SubjectFilterMode: SubjectFilterTarget,
		Subjects:          []TargetSubject{{ID: 1, Name: "Charizard"}, {ID: 2, Name: "Pikachu"}, {ID: 3, Name: "Mewtwo"}},
	}
	campaignNoFilters := Campaign{
		ID:   "campaign-none",
		Name: "No Filters",
	}

	t.Run("single match by grade and price", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA, campaignB},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-a" {
			t.Errorf("expected campaign-a, got %s", result.CampaignID)
		}
	})

	t.Run("single match to campaign B", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 7.0, BuyCostCents: 5000, CardName: "Blastoise", SetName: "Base Set"},
			[]Campaign{campaignA, campaignB},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-b" {
			t.Errorf("expected campaign-b, got %s", result.CampaignID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 5.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA, campaignB},
		)
		if result.Status != "unmatched" {
			t.Fatalf("expected unmatched, got %s", result.Status)
		}
	})

	t.Run("ambiguous match", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA, campaignC},
		)
		if result.Status != "ambiguous" {
			t.Fatalf("expected ambiguous, got %s", result.Status)
		}
		if len(result.Candidates) != 2 {
			t.Errorf("expected 2 candidates, got %d", len(result.Candidates))
		}
	})

	t.Run("campaign with no filters matches everything", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignNoFilters},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-none" {
			t.Errorf("expected campaign-none, got %s", result.CampaignID)
		}
	})

	t.Run("empty campaign list", func(t *testing.T) {
		result := FindMatchingCampaign(MatchInput{Grade: 9.0, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"}, nil)
		if result.Status != "unmatched" {
			t.Fatalf("expected unmatched, got %s", result.Status)
		}
	})

	t.Run("half-grade 9.5 matches range 9-10", func(t *testing.T) {
		result := FindMatchingCampaign(
			MatchInput{Grade: 9.5, BuyCostCents: 15000, CardName: "Charizard", SetName: "Base Set"},
			[]Campaign{campaignA},
		)
		if result.Status != "matched" {
			t.Fatalf("expected matched, got %s", result.Status)
		}
		if result.CampaignID != "campaign-a" {
			t.Errorf("expected campaign-a, got %s", result.CampaignID)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/... -run 'TestPurchaseMatchesCampaign|TestFindMatchingCampaign|TestLanguageAxisMatches|TestSubjectAxisMatches|TestSpecDenied' -v`
Expected: FAIL to compile — `undefined: MatchInput`, `too many arguments in call to PurchaseMatchesCampaign`.

- [ ] **Step 3: Write the implementation**

Replace `internal/domain/inventory/matching.go` in full:

```go
package inventory

import (
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// ParseRange parses a "min-max" range string into its integer bounds.
// Returns (0, 0, false) if the string is empty or malformed.
func ParseRange(s string) (lo, hi int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	// Normalize common Unicode dashes to ASCII hyphen
	s = strings.ReplaceAll(s, "–", "-") // en dash
	s = strings.ReplaceAll(s, "—", "-") // em dash
	s = strings.ReplaceAll(s, "‒", "-") // figure dash
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	var err error
	lo, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	hi, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

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

// PurchaseMatchesCampaign checks whether a purchase's attributes satisfy all
// of a campaign's defined filter criteria. Unset criteria are treated as
// wildcards (match anything). Evaluation order: year, grade, price (scaled by
// BuyTermsCLPct), language axis, subject axis, then the card-level deny list.
func PurchaseMatchesCampaign(in MatchInput, c *Campaign) bool {
	// Year range check — matches the card's release year against the campaign's year range.
	// This is the primary disambiguator: a 1999 card goes to a vintage campaign, not modern.
	if c.YearRange != "" && in.CardYear > 0 {
		lo, hi, ok := ParseRange(c.YearRange)
		if !ok {
			return false
		}
		if in.CardYear < lo || in.CardYear > hi {
			return false
		}
	}

	// Grade range check (supports half-grades: 9.5 matches range "9-10")
	if c.GradeRange != "" {
		lo, hi, ok := ParseRange(c.GradeRange)
		if !ok {
			return false
		}
		if in.Grade < float64(lo) || in.Grade > float64(hi) {
			return false
		}
	}

	// Price range check — campaign range is the card's market value in dollars,
	// but BuyCostCents is what was actually paid (market value * buyTermsPct).
	// Scale the range by BuyTermsCLPct so we compare apples to apples.
	if c.PriceRange != "" {
		lo, hi, ok := ParseRange(c.PriceRange)
		if !ok {
			return false
		}
		buyPct := c.BuyTermsCLPct
		if buyPct <= 0 || buyPct > 1 {
			buyPct = 1 // no scaling if unset or invalid
		}
		loCents := int(float64(lo*100) * buyPct)
		hiCents := int(float64(hi*100) * buyPct)
		if in.BuyCostCents < loCents || in.BuyCostCents > hiCents {
			return false
		}
	}

	if !LanguageAxisMatches(in.SetName, c.TargetLanguage) {
		return false
	}

	if !SubjectAxisMatches(in.CardName, c.Subjects, c.SubjectFilterMode) {
		return false
	}

	if SpecDenied(in, c.DeniedSpecs) {
		return false
	}

	return true
}

// LanguageAxisMatches reports whether a set name satisfies the language axis.
// An empty targetLanguage is an open net and always matches.
func LanguageAxisMatches(setName, targetLanguage string) bool {
	if targetLanguage == "" {
		return true
	}
	return cardutil.SetLanguage(setName) == targetLanguage
}

// SubjectAxisMatches reports whether a card name satisfies the subject axis.
// An empty subjects list is an open net and always matches, regardless of
// mode. Matching is a case-insensitive substring test of each subject's name
// against the card name.
func SubjectAxisMatches(cardName string, subjects []TargetSubject, mode string) bool {
	if len(subjects) == 0 {
		return true
	}
	lowerCard := strings.ToLower(cardName)
	matched := false
	for _, s := range subjects {
		if s.Name == "" {
			continue
		}
		if strings.Contains(lowerCard, strings.ToLower(s.Name)) {
			matched = true
			break
		}
	}
	if mode == SubjectFilterExclude {
		return !matched
	}
	return matched
}

// SpecDenied reports whether the card is on the campaign's card-level deny
// list. Identity is ID-first: if both in.PSASpecID and the denied entry's ID
// are non-zero, deny on equality. PSASpecID is CL-sourced and omitempty, so it
// is frequently 0 — the ID path is the exception, not the common case.
//
// The common-case fallback compares a composite key built from the
// purchase's normalized set name (cardutil.NormalizeSetNameSimple) and card
// number against the denied entry's Name. TargetSubject carries only
// {ID, Name} — there is no separate set/number field on a denied entry to
// compare against — so this fallback treats a name-only denied entry's Name
// as recorded in that same "<normalized set name> <card number>" form and
// requires an exact, case-insensitive match on the whole composite key. That
// is the most conservative reading available: this plan is defining the
// convention (there is no existing denied-entry data to reverse-engineer it
// from), and an exact composite match can only under-deny relative to any
// substring or fuzzy comparison, never over-deny. If either half of the
// composite key is missing (empty set name or card number), no key is built
// and that entry cannot deny by name.
//
// When neither identity — PSASpecID/ID, or the set+number composite — is
// available, the entry does not deny the purchase. That fail-open direction
// is deliberate: this predicate does not gate buying (the portal enforces
// denials at buy time); it only attributes an already-purchased card to a
// campaign after the fact. A false deny would silently misattribute a
// legitimate purchase and distort every downstream analytic, while a missed
// deny merely attributes a card that shouldn't have been bought — visible
// and correctable. Do not "fix" this into failing closed.
func SpecDenied(in MatchInput, denied []TargetSubject) bool {
	key := specDenyKey(in.SetName, in.CardNumber)
	for _, d := range denied {
		if in.PSASpecID != 0 && d.ID != 0 {
			if in.PSASpecID == d.ID {
				return true
			}
			continue
		}
		if key != "" && strings.EqualFold(strings.TrimSpace(d.Name), key) {
			return true
		}
	}
	return false
}

// specDenyKey builds the composite fallback identity used by SpecDenied when
// PSASpecID is unavailable: the set name normalized via
// cardutil.NormalizeSetNameSimple, plus the card number, space-joined.
// Returns "" if either half is missing, since a partial key is not a
// reliable identity and must not be allowed to match anything.
func specDenyKey(setName, cardNumber string) string {
	normSet := strings.TrimSpace(cardutil.NormalizeSetNameSimple(setName))
	cardNumber = strings.TrimSpace(cardNumber)
	if normSet == "" || cardNumber == "" {
		return ""
	}
	return normSet + " " + cardNumber
}

// SplitInclusionList splits an inclusion/exclusion list string into individual
// entries. It supports both comma-separated ("charizard,pikachu") and
// space-separated ("charizard pikachu") formats, as well as mixed usage.
func SplitInclusionList(s string) []string {
	// First split by commas
	parts := strings.Split(s, ",")
	var entries []string
	for _, part := range parts {
		// Then split each part by whitespace
		for _, word := range strings.Fields(part) {
			if word != "" {
				entries = append(entries, word)
			}
		}
	}
	return entries
}

// MatchResult describes the outcome of matching a purchase against all inventory.
type MatchResult struct {
	CampaignID string   // Set when exactly one campaign matches
	Candidates []string // Set when multiple campaigns match (ambiguous)
	Status     string   // "matched", "unmatched", "ambiguous"
}

// FindMatchingCampaign evaluates a purchase against all provided campaigns and
// returns the matching campaign. If exactly one campaign matches, it is returned.
// If zero match, status is "unmatched". If multiple match, status is "ambiguous"
// with candidate IDs listed.
func FindMatchingCampaign(in MatchInput, allCampaigns []Campaign) MatchResult {
	var matches []string
	for i := range allCampaigns {
		if PurchaseMatchesCampaign(in, &allCampaigns[i]) {
			matches = append(matches, allCampaigns[i].ID)
		}
	}

	switch len(matches) {
	case 0:
		return MatchResult{Status: "unmatched"}
	case 1:
		return MatchResult{CampaignID: matches[0], Status: "matched"}
	default:
		return MatchResult{Candidates: matches, Status: "ambiguous"}
	}
}
```

Update the sole external caller, `internal/domain/inventory/service_import_psa.go:104-111`.

Before:

```go
		match := FindMatchingCampaign(
			gradeValue,
			buyCostCents,
			meta.CardName,
			meta.SetName,
			meta.CardYear,
			matchingCampaigns,
		)
```

After:

```go
		match := FindMatchingCampaign(MatchInput{
			Grade:        gradeValue,
			BuyCostCents: buyCostCents,
			CardName:     meta.CardName,
			SetName:      meta.SetName,
			CardNumber:   meta.CardNumber,
			CardYear:     meta.CardYear,
			// PSASpecID is left at its zero value here: the CSV-title parse
			// path has no CL spec id yet — cert lookups that resolve it run
			// asynchronously after import completes (see the comment two
			// lines above this call site).
		}, matchingCampaigns)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/domain/inventory/... -run 'TestParseRange|TestPurchaseMatchesCampaign|TestFindMatchingCampaign|TestLanguageAxisMatches|TestSubjectAxisMatches|TestSpecDenied' -v`
Expected: PASS

Then run the full package to confirm the call-site update didn't break anything else:

Run: `go test -race ./internal/domain/inventory/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/matching.go internal/domain/inventory/matching_test.go internal/domain/inventory/service_import_psa.go
git commit -m "feat: rewrite campaign matching around three targeting axes"
```

### Task 4: Coverage, demand, portfolio, and suggestion consumers move to the subject axis

**Files:**
- Modify: `internal/domain/demand/repository.go:24-30`
- Modify: `internal/domain/demand/campaign_signals.go:168-229`
- Modify: `internal/adapters/storage/postgres/campaign_coverage.go` (whole file)
- Modify: `internal/domain/portfolio/analysis.go:333-382` (`inScope`)
- Modify: `internal/domain/inventory/suggestion_rules.go:11-77,130-160` (`suggestTopCharacterExpansion`, `suggestCoverageGapCampaigns`)
- Modify: `internal/domain/inventory/suggestion_rules_optimization.go:120-299` (`suggestCharacterAdjustments`, `suggestPhaseTransitions`)
- Modify: `internal/domain/inventory/portfolio.go:26-60,304-355` (`ExtractCharacter`, `DetectCoverageGaps`)
- Modify: `internal/domain/inventory/suggestion_types.go:15-25` (`CampaignSuggestionParams`)
- Test: `internal/domain/demand/campaign_signals_test.go` (rewrite)
- Test: `internal/adapters/storage/postgres/campaign_coverage_test.go` (new)
- Test: `internal/domain/portfolio/analysis_test.go:252-273` (rewrite table cases)
- Test: `internal/domain/inventory/portfolio_test.go` (rewrite `InclusionList` literals; table-ify `TestDetectCoverageGaps`)
- Test: `internal/domain/inventory/suggestions_test.go` (rewrite `InclusionList` literals and `SuggestedParams` assertions)

**Interfaces:**
- Consumes: `inventory.TargetSubject{ID int; Name string}`, `inventory.SubjectFilterTarget`,
  `inventory.SubjectFilterExclude` constants, and
  `func inventory.SubjectAxisMatches(cardName string, subjects []inventory.TargetSubject, mode string) bool`
  (all defined in an earlier task; not redefined here).
- Produces: `demand.ActiveCampaign{ID, Name, GradeRange, TargetLanguage, SubjectFilterMode string, Subjects []inventory.TargetSubject}`
  (no `InclusionList`/`ExclusionMode`) — consumed by `demand.Service.CampaignSignals` and any future
  `CampaignCoverageLookup` caller.
- Produces: `inventory.CampaignSuggestionParams.Subjects []string` (renamed from `InclusionList string`) — plain
  subject names for display only; a suggestion is pre-portal and carries no PSA subject ids to preserve,
  matching the `subjects?: string[]` frontend contract from Task 11.

**Judgment call:** `CampaignsCovering`'s SELECT reads only `grade_range, subject_filter_mode, subjects` —
not `target_language` — because coverage deliberately evaluates the subject axis only (see the code
comment added below); `ActiveCampaigns`' SELECT reads all three new columns because `demand.ActiveCampaign`
carries `TargetLanguage` for future consumers even though `CampaignSignals` doesn't read it today.

**Judgment call:** `portfolio/analysis.go`, `inventory/suggestion_rules.go`,
`inventory/suggestion_rules_optimization.go`, and `inventory/portfolio.go` all read `inventory.Campaign`
directly (they never see `demand.ActiveCampaign`) and none of them belonged to Task 3's Files list
(`matching.go`, `service_import_psa.go`, `matching_test.go` only) or any other task, so they are folded
into this task rather than left as unowned readers of the soon-to-be-mirror-only
`InclusionList`/`ExclusionMode` fields. `types_core.go` and `campaign_store.go` are unchanged — the legacy
fields keep being written by the mirror (Task 2), only their readers move here.

**Judgment call:** `inventory.ExtractCharacter` harvests subject names to extend the character-matching
vocabulary — it is not making a match decision, so it iterates `camp.Subjects` directly rather than routing
through `SubjectAxisMatches`; introducing the predicate there would not add a second implementation, but it
also wouldn't do anything a plain field walk doesn't already do, so the simpler form is kept.

**Judgment call (decision, not a behavior change):** `DetectCoverageGaps` was considered for a switch to
`SubjectAxisMatches`'s open-net convention (empty `Subjects` on an active campaign "covers" every
character) and that reading was deliberately **rejected**. Production data read on 2026-08-06 shows exactly
one active campaign, and it is open-net (`phase=active: total=1, open_net=1`; `phase=pending: total=13,
open_net=6`). Under the open-net-covers-everything reading, `coveredChars` would contain every character on
every run and `DetectCoverageGaps` would permanently return zero character gaps — silently disabling the
coverage-gap tool operators use to find where to start new campaigns, which is the load-bearing behavior
here. `DetectCoverageGaps` therefore keeps today's convention: it walks `c.Subjects` directly (not
`SubjectAxisMatches`) and an empty list contributes zero names to `coveredChars`, i.e. covers nothing. This
is a deliberate asymmetry with `SubjectAxisMatches`'s empty-list-means-open-net rule — do not "fix" it to
match `SubjectAxisMatches` without re-checking the production campaign mix first.

One narrow behavior change does ride along inside `isCovered`: it skips campaigns whose
`SubjectFilterMode` is `Exclude`. Today's code walks `SplitInclusionList(c.InclusionList)` with no polarity
check, so an `Exclude`-mode campaign's denied names count as *covered* — the exact inverse of their meaning,
suppressing a gap for a character no campaign will buy. The skip is deliberate and is the same polarity fix
as the `suggestCharacterAdjustments` judgment call below. It can only ever surface more gaps, never hide
one, so it is safe in the direction that matters here.

**Judgment call (behavior change):** `suggestCharacterAdjustments` and `suggestPhaseTransitions` previously
ignored `ExclusionMode` entirely — they matched a character against `InclusionList` membership regardless
of polarity, so an `ExclusionMode=true` campaign's "remove"/"add"/"activate" suggestions were computed
against the denied-name list as if it were a target list. Routing both through `SubjectAxisMatches` fixes
this for `SubjectFilterMode=Exclude` campaigns (matched now correctly means "not denied"); `Target`-mode
campaigns are unaffected since the two checks agree there.

- [ ] **Step 1: Write the failing tests**

```go
// internal/domain/demand/campaign_signals_test.go
package demand_test

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// velocityJSON builds a velocity_json blob matching the CharacterVelocityFields
// stored format: all numeric fields are JSON numbers (not strings).
func velocityJSON(medianDays, vChangePct float64, sample int) string {
	return `{
		"median_days_to_sell": ` + strconv.FormatFloat(medianDays, 'f', -1, 64) + `,
		"sell_through": {},
		"sample_size": ` + strconv.Itoa(sample) + `,
		"velocity_change_pct": ` + strconv.FormatFloat(vChangePct, 'f', -1, 64) + `
	}`
}

// velocityJSONNoChange omits velocity_change_pct — used to verify the service
// excludes characters with no change metric from contributors.
func velocityJSONNoChange() string {
	return `{"median_days_to_sell": 10, "sell_through": {}, "sample_size": 5}`
}

func charRow(name string, medianDays, vChangePct float64, sample int, computed time.Time) demand.CharacterCache {
	vj := velocityJSON(medianDays, vChangePct, sample)
	return demand.CharacterCache{
		Character:           name,
		Window:              "30d",
		VelocityJSON:        &vj,
		AnalyticsComputedAt: &computed,
	}
}

func charRowNoChange(name string, computed time.Time) demand.CharacterCache {
	vj := velocityJSONNoChange()
	return demand.CharacterCache{
		Character:           name,
		Window:              "30d",
		VelocityJSON:        &vj,
		AnalyticsComputedAt: &computed,
	}
}

// campaignLookupWith builds a CampaignCoverageLookupMock whose ActiveCampaigns
// returns the given list. Leaves the other Fn fields nil so the defaults
// (empty coverage, zero unsold) apply — CampaignSignals only reads
// ActiveCampaigns.
func campaignLookupWith(campaigns []demand.ActiveCampaign) *mocks.CampaignCoverageLookupMock {
	return &mocks.CampaignCoverageLookupMock{
		ActiveCampaignsFn: func(ctx context.Context) ([]demand.ActiveCampaign, error) {
			return campaigns, nil
		},
	}
}

func TestCampaignSignals(t *testing.T) {
	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)

	tests := []struct {
		name      string
		rows      []demand.CharacterCache
		campaigns []demand.ActiveCampaign
		wantSigs  int
		wantTop   string // first signal's top accelerator name (empty = skip check)
		wantQual  string
	}{
		{
			name:      "empty cache",
			rows:      nil,
			campaigns: []demand.ActiveCampaign{{ID: "c1", Name: "Modern"}},
			wantSigs:  0,
			wantQual:  demand.QualityEmpty,
		},
		{
			name: "subject list campaign one accelerator",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Charizard", 8, 2.0, 52, computed), // below accel threshold
				charRow("Umbreon", 21, -8.3, 18, computed), // decelerating
			},
			campaigns: []demand.ActiveCampaign{{
				ID:                "c1",
				Name:              "Vintage Core",
				GradeRange:        "9-10",
				SubjectFilterMode: inventory.SubjectFilterTarget,
				Subjects: []inventory.TargetSubject{
					{ID: 1, Name: "Charizard"},
					{ID: 2, Name: "Pikachu"},
					{ID: 3, Name: "Umbreon"},
				},
			}},
			wantSigs: 1,
			wantTop:  "Pikachu",
			wantQual: demand.QualityFull,
		},
		{
			name: "open net campaign matches all cached characters",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Gengar", 12, 10.0, 20, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: "c4", Name: "Modern"}},
			wantSigs:  1,
			wantTop:   "Pikachu",
			wantQual:  demand.QualityFull,
		},
		{
			name: "subject list with no cache overlap produces no signal",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
			},
			campaigns: []demand.ActiveCampaign{{
				ID:                "c7",
				Name:              "Crystal",
				SubjectFilterMode: inventory.SubjectFilterTarget,
				Subjects: []inventory.TargetSubject{
					{ID: 1, Name: "Kingdra"},
					{ID: 2, Name: "Kabutops"},
				},
			}},
			wantSigs: 0,
			wantQual: demand.QualityEmpty,
		},
		{
			name: "character with null velocity_change_pct is excluded from contributors",
			rows: []demand.CharacterCache{
				charRowNoChange("Pikachu", computed),
				charRow("Charizard", 8, 15.7, 52, computed),
			},
			campaigns: []demand.ActiveCampaign{{
				ID:                "c1",
				Name:              "Vintage Core",
				SubjectFilterMode: inventory.SubjectFilterTarget,
				Subjects: []inventory.TargetSubject{
					{ID: 1, Name: "Pikachu"},
					{ID: 2, Name: "Charizard"},
				},
			}},
			wantSigs: 1,
			wantTop:  "Charizard",
			wantQual: demand.QualityFull,
		},
		{
			name: "top accelerating list capped at 5",
			rows: []demand.CharacterCache{
				charRow("A", 5, 30.0, 20, computed),
				charRow("B", 5, 28.0, 20, computed),
				charRow("C", 5, 26.0, 20, computed),
				charRow("D", 5, 24.0, 20, computed),
				charRow("E", 5, 22.0, 20, computed),
				charRow("F", 5, 20.0, 20, computed),
				charRow("G", 5, 18.0, 20, computed),
			},
			campaigns: []demand.ActiveCampaign{{ID: "c4", Name: "Modern"}},
			wantSigs:  1,
			wantTop:   "A",
			wantQual:  demand.QualityFull,
		},
		{
			name: "exclusion mode excludes listed characters",
			rows: []demand.CharacterCache{
				charRow("Pikachu", 11, 22.1, 34, computed),
				charRow("Charizard", 8, 15.7, 52, computed),
				charRow("Gengar", 12, 10.0, 20, computed),
			},
			campaigns: []demand.ActiveCampaign{{
				ID:                "c5",
				Name:              "No Pikachu",
				SubjectFilterMode: inventory.SubjectFilterExclude,
				Subjects:          []inventory.TargetSubject{{ID: 1, Name: "Pikachu"}},
			}},
			wantSigs: 1,
			wantTop:  "Charizard",
			wantQual: demand.QualityFull,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows := tc.rows
			repo := &mocks.DemandRepositoryMock{
				ListCharacterCacheFn: func(ctx context.Context, window string) ([]demand.CharacterCache, error) {
					return rows, nil
				},
			}
			svc := demand.NewService(repo, campaignLookupWith(tc.campaigns))

			resp, err := svc.CampaignSignals(context.Background())
			if err != nil {
				t.Fatalf("CampaignSignals: %v", err)
			}

			if len(resp.Signals) != tc.wantSigs {
				t.Fatalf("want %d signals, got %d: %+v", tc.wantSigs, len(resp.Signals), resp.Signals)
			}
			if resp.DataQuality != tc.wantQual {
				t.Errorf("want quality=%s, got %s", tc.wantQual, resp.DataQuality)
			}
			if tc.wantTop != "" {
				if len(resp.Signals[0].TopAccelerating) == 0 {
					t.Fatalf("want top accelerating, got none")
				}
				if resp.Signals[0].TopAccelerating[0].Character != tc.wantTop {
					t.Errorf("want top=%s, got %s", tc.wantTop, resp.Signals[0].TopAccelerating[0].Character)
				}
				if len(resp.Signals[0].TopAccelerating) > demand.TopContributorsLimit {
					t.Errorf("top list exceeds cap: %d", len(resp.Signals[0].TopAccelerating))
				}
			}
		})
	}
}

// TestCampaignSignals_MedianVelocity verifies the median calculation via
// the public CampaignSignals surface. Even count: average of two middles.
// Odd count: middle element.
func TestCampaignSignals_MedianVelocity(t *testing.T) {
	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)

	tests := []struct {
		name       string
		vChanges   []float64 // velocity_change_pct for each character
		wantMedian float64
	}{
		{
			// Odd count: sorted [10, 20, 30] → middle = 20
			name:       "odd count uses middle element",
			vChanges:   []float64{30.0, 10.0, 20.0},
			wantMedian: 20.0,
		},
		{
			// Even count: sorted [10, 20] → (10+20)/2 = 15
			name:       "even count averages two middles",
			vChanges:   []float64{20.0, 10.0},
			wantMedian: 15.0,
		},
		{
			// Open net case: 22.1 and 10.0 → (10.0+22.1)/2 = 16.05
			name:       "open net two characters",
			vChanges:   []float64{22.1, 10.0},
			wantMedian: 16.05,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rows []demand.CharacterCache
			for i, v := range tc.vChanges {
				rows = append(rows, charRow("Char"+strconv.Itoa(i), 10, v, 10, computed))
			}
			repo := &mocks.DemandRepositoryMock{
				ListCharacterCacheFn: func(ctx context.Context, window string) ([]demand.CharacterCache, error) {
					return rows, nil
				},
			}
			campaign := demand.ActiveCampaign{ID: "c1", Name: "Test"}
			svc := demand.NewService(repo, campaignLookupWith([]demand.ActiveCampaign{campaign}))

			resp, err := svc.CampaignSignals(context.Background())
			if err != nil {
				t.Fatalf("CampaignSignals: %v", err)
			}
			if len(resp.Signals) != 1 {
				t.Fatalf("want 1 signal, got %d", len(resp.Signals))
			}
			got := resp.Signals[0].MedianVelocityChangePct
			// Tolerance comparison: the (22.1+10.0)/2 path involves IEEE 754
			// rounding that happens to match the 16.05 literal today, but
			// that's fragile for a median calculation. 1e-9 is well under any
			// precision we care about for a percentage-point metric.
			if math.Abs(got-tc.wantMedian) > 1e-9 {
				t.Errorf("want median=%v, got %v", tc.wantMedian, got)
			}
		})
	}
}
```

```go
// internal/adapters/storage/postgres/campaign_coverage_test.go
package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertCoverageCampaign inserts a minimal campaigns row exercising only the
// columns CampaignCoverageLookup reads; every other column keeps its DB default.
func insertCoverageCampaign(t *testing.T, db *DB, id, phase, gradeRange, targetLanguage, subjectFilterMode, subjectsJSON string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO campaigns (id, name, phase, grade_range, target_language, subject_filter_mode, subjects)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, "Test "+id, phase, gradeRange, targetLanguage, subjectFilterMode, subjectsJSON,
	)
	require.NoError(t, err)
}

func TestCampaignCoverageLookup_ActiveCampaigns(t *testing.T) {
	db := setupTestDB(t)
	lookup := NewCampaignCoverageLookup(db.DB)
	ctx := context.Background()

	insertCoverageCampaign(t, db, "active-1", string(inventory.PhaseActive), "9-10", "english", "Target", `[{"id":1,"name":"Pikachu"}]`)
	insertCoverageCampaign(t, db, "pending-1", string(inventory.PhasePending), "9-10", "", "Target", `[]`)

	got, err := lookup.ActiveCampaigns(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "active-1", got[0].ID)
	assert.Equal(t, "9-10", got[0].GradeRange)
	assert.Equal(t, "english", got[0].TargetLanguage)
	assert.Equal(t, "Target", got[0].SubjectFilterMode)
	assert.Equal(t, []inventory.TargetSubject{{ID: 1, Name: "Pikachu"}}, got[0].Subjects)
}

func TestCampaignCoverageLookup_CampaignsCovering(t *testing.T) {
	db := setupTestDB(t)
	lookup := NewCampaignCoverageLookup(db.DB)
	ctx := context.Background()

	// target-1: Target mode, matches only "Pikachu", grade 9-10 only.
	insertCoverageCampaign(t, db, "target-1", string(inventory.PhaseActive), "9-10", "", "Target", `[{"id":1,"name":"Pikachu"}]`)
	// exclude-1: Exclude mode, denies "Pikachu", no grade constraint.
	insertCoverageCampaign(t, db, "exclude-1", string(inventory.PhaseActive), "", "", "Exclude", `[{"id":2,"name":"Pikachu"}]`)

	tests := []struct {
		name      string
		character string
		grade     int
		want      []string
	}{
		{"target campaign matches its subject at valid grade", "Pikachu", 9, []string{"target-1"}},
		{"target campaign's grade filter excludes it; exclude campaign denies the same character", "Pikachu", 5, []string{}},
		{"grade 0 means no grade filter", "Pikachu", 0, []string{"target-1"}},
		{"exclude campaign allows an unlisted character; target campaign requires exact subject", "Charizard", 0, []string{"exclude-1"}},
		{"unlisted character satisfies exclude but not target", "Bulbasaur", 9, []string{"exclude-1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := lookup.CampaignsCovering(ctx, tc.character, "", tc.grade)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/demand/... -run TestCampaignSignals -v`
Expected: FAIL to compile — `undefined: inventory.SubjectFilterTarget` /
`unknown field Subjects in struct literal` (`demand.ActiveCampaign` doesn't have the new fields yet).

Run: `go test ./internal/adapters/storage/postgres/... -run TestCampaignCoverageLookup -v`
Expected: FAIL to compile — `unknown field TargetLanguage in struct literal`, and once
`POSTGRES_TEST_URL` is set, FAIL at the `INSERT` (`column "target_language" of relation "campaigns" does not exist`)
until migration `000023` (an earlier task) has run.

- [ ] **Step 3: Write the implementation**

```go
// internal/domain/demand/repository.go
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
	ListCardCacheByDemandScore(ctx context.Context, window string, limit int) ([]CardCache, error)
	CardDataQualityStats(ctx context.Context, window string) (QualityStats, error)

	// Character cache
	UpsertCharacterCache(ctx context.Context, row CharacterCache) error
	GetCharacterCache(ctx context.Context, character, window string) (*CharacterCache, error)
	ListCharacterCache(ctx context.Context, window string) ([]CharacterCache, error)
}

// ActiveCampaign describes a single active campaign's targeting rules, used
// by the campaign-signals service to correlate per-campaign market data.
// Kept minimal — only the fields needed to filter characters and grades.
type ActiveCampaign struct {
	ID                string // Campaign primary key (UUID for standard campaigns, "external" for the imported bucket).
	Name              string
	GradeRange        string // e.g. "9-10"; empty means no grade constraint.
	TargetLanguage    string
	SubjectFilterMode string
	Subjects          []inventory.TargetSubject
}

// ActiveCampaignSource is the narrow interface used by CampaignSignals to
// enumerate active campaigns. Separating it from CampaignCoverageLookup
// makes the two access patterns explicit: per-niche indexed lookup (leaderboard)
// vs. full table scan (campaign signals).
type ActiveCampaignSource interface {
	// ActiveCampaigns returns all campaigns with Phase="active". Returns an
	// empty slice when there are no active campaigns.
	ActiveCampaigns(ctx context.Context) ([]ActiveCampaign, error)
}

// CampaignCoverageLookup answers coverage questions for a niche bucket
// (character, era, grade). The real implementation is wired in T5/T6 against
// the campaigns store; for now this interface is the seam the Service depends
// on so it can be fully unit-tested.
//
// It embeds ActiveCampaignSource so the same concrete type can satisfy both
// interfaces without two separate injection points on Service.
type CampaignCoverageLookup interface {
	ActiveCampaignSource

	// CampaignsCovering returns active campaign IDs whose subject-axis rules
	// match the given (character, era, grade) triple. An empty slice means no
	// campaign currently targets this niche.
	CampaignsCovering(ctx context.Context, character, era string, grade int) ([]string, error)

	// UnsoldCountFor returns the count of our unsold inventory matching the
	// bucket. Zero means the niche is uncovered by our holdings.
	UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error)
}
```

```go
// internal/domain/demand/campaign_signals.go — replace collectContributors
// (the rest of the file, including buildSignalIndex which still uses strings,
// is unchanged)

// collectContributors returns the signalEntry values from idx that belong to
// the given campaign's subject axis, delegating to inventory.SubjectAxisMatches
// so subject matching has exactly one implementation across the codebase.
//
// GradeRange is intentionally not applied here: the character cache aggregates
// velocity across all grades, so grade-range filtering would require
// per-grade cache rows that do not currently exist. Signals therefore reflect
// the campaign's character universe regardless of targeted grades.
func collectContributors(c ActiveCampaign, idx map[string]signalEntry) []signalEntry {
	var out []signalEntry
	for _, entry := range idx {
		if inventory.SubjectAxisMatches(entry.displayName, c.Subjects, c.SubjectFilterMode) {
			out = append(out, entry)
		}
	}
	return out
}
```

```go
// internal/adapters/storage/postgres/campaign_coverage.go
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Compile-time check.
var _ demand.CampaignCoverageLookup = (*CampaignCoverageLookup)(nil)

// CampaignCoverageLookup answers (character, era, grade) coverage questions
// against the campaigns + campaign_purchases tables. It implements
// demand.CampaignCoverageLookup for the niche-opportunity leaderboard.
//
// Era matching is currently a no-op: the campaign schema has no era field
// (it has year_range which is a coarser proxy), and card_year on purchases
// isn't mapped to DH's era enum. era is accepted for interface parity and
// ignored. This is a documented limitation of T5 — when DH era enums are
// authoritatively mapped to CL year ranges, this implementation can narrow.
//
// Coverage evaluates the SUBJECT AXIS ONLY. CampaignsCovering/UnsoldCountFor
// receive a bare (character, era, grade) triple with no set name and no spec
// id, so language (TargetLanguage) and card-level denials (DeniedSpecs) have
// no defined value here and are not evaluated. This is a documented reduction
// versus inventory.PurchaseMatchesCampaign, not an oversight: the
// niche-opportunity leaderboard asks a character-level question, and widening
// CampaignsCovering to carry a language input is deferred until something
// actually asks a language-scoped coverage question.
type CampaignCoverageLookup struct {
	db *sql.DB
}

// NewCampaignCoverageLookup constructs a CampaignCoverageLookup.
func NewCampaignCoverageLookup(db *sql.DB) *CampaignCoverageLookup {
	return &CampaignCoverageLookup{db: db}
}

// CampaignsCovering returns IDs of active campaigns whose subject-axis rules
// match the given (character, grade) pair. era is ignored — see type docs.
func (l *CampaignCoverageLookup) CampaignsCovering(ctx context.Context, character, _ string, grade int) ([]string, error) {
	if strings.TrimSpace(character) == "" {
		return []string{}, nil
	}

	rows, err := l.db.QueryContext(ctx,
		`SELECT id, grade_range, subject_filter_mode, subjects
		 FROM campaigns
		 WHERE phase = $1 AND id <> $2`,
		string(inventory.PhaseActive),
		inventory.ExternalCampaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("query active campaigns: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []string
	for rows.Next() {
		var (
			id                string
			gradeRange        string
			subjectFilterMode string
			subjectsJSON      []byte
		)
		if err := rows.Scan(&id, &gradeRange, &subjectFilterMode, &subjectsJSON); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}

		if !gradeInRange(grade, gradeRange) {
			continue
		}

		var subjects []inventory.TargetSubject
		if err := json.Unmarshal(subjectsJSON, &subjects); err != nil {
			return nil, fmt.Errorf("unmarshal subjects for campaign %s: %w", id, err)
		}
		if !inventory.SubjectAxisMatches(character, subjects, subjectFilterMode) {
			continue
		}

		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaigns: %w", err)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// UnsoldCountFor returns the count of unsold purchases whose card_player
// matches `character` (case-insensitive). era is ignored — see type docs.
// grade 0 means no grade filter.
func (l *CampaignCoverageLookup) UnsoldCountFor(ctx context.Context, character, _ string, grade int) (int, error) {
	if strings.TrimSpace(character) == "" {
		return 0, nil
	}

	query := `
		SELECT COUNT(*)
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		  AND LOWER(p.card_player) = LOWER($1)
		  AND ($2 = 0 OR p.grade_value = $3)
	`
	var count int
	if err := l.db.QueryRowContext(ctx, query, character, grade, grade).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unsold: %w", err)
	}
	return count, nil
}

// gradeInRange returns true if grade falls within the campaign's grade_range
// (e.g. "9-10"). An empty range means no constraint (match).
func gradeInRange(grade int, rangeStr string) bool {
	if grade == 0 {
		return true
	}
	if strings.TrimSpace(rangeStr) == "" {
		return true
	}
	lo, hi, ok := inventory.ParseRange(rangeStr)
	if !ok {
		return false
	}
	return grade >= lo && grade <= hi
}

// ActiveCampaigns returns all standard campaigns with phase=active. The
// "external" bucket is excluded — it represents pre-campaign imports with no
// targeting rules and would distort signal aggregations. Returns an empty
// slice when there are no qualifying campaigns.
func (l *CampaignCoverageLookup) ActiveCampaigns(ctx context.Context) ([]demand.ActiveCampaign, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT id, name, grade_range, target_language, subject_filter_mode, subjects
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
			targetLanguage    string
			subjectFilterMode string
			subjectsJSON      []byte
		)
		if err := rows.Scan(&id, &name, &gradeRange, &targetLanguage, &subjectFilterMode, &subjectsJSON); err != nil {
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
			TargetLanguage:    targetLanguage,
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

`characterMatchesInclusion` (formerly `campaign_coverage.go:175-196`) is deleted outright — no
replacement function remains in this package; both call sites now call
`inventory.SubjectAxisMatches` directly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/domain/demand/... -run TestCampaignSignals -v`
Expected: PASS

Run: `POSTGRES_TEST_URL=<dsn> go test -race ./internal/adapters/storage/postgres/... -run TestCampaignCoverageLookup -v`
Expected: PASS (skips with a clear message when `POSTGRES_TEST_URL` is unset, matching this
package's existing convention in `testhelper_test.go`)

- [ ] **Step 5: Write the failing tests for the portfolio and suggestion consumers**

```go
// internal/domain/portfolio/analysis_test.go — replace the four InclusionList/ExclusionMode
// cases in TestComputeAnalysisScopeFilter's `cases` table (the grade/price cases above are unchanged)
		{
			name:        "subjects (Target) excludes pikachu",
			campaign:    inventory.Campaign{ID: "c1", Subjects: []inventory.TargetSubject{{Name: "charizard"}, {Name: "blastoise"}}, SubjectFilterMode: inventory.SubjectFilterTarget},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Pikachu", PurchaseDate: "2026-07-06"},
			wantInScope: false,
		},
		{
			name:        "subjects (Target) includes charizard, case-insensitive",
			campaign:    inventory.Campaign{ID: "c1", Subjects: []inventory.TargetSubject{{Name: "charizard"}, {Name: "blastoise"}}, SubjectFilterMode: inventory.SubjectFilterTarget},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Charizard", PurchaseDate: "2026-07-06"},
			wantInScope: true,
		},
		{
			name:        "subjects (Exclude) excludes charizard",
			campaign:    inventory.Campaign{ID: "c1", Subjects: []inventory.TargetSubject{{Name: "charizard"}, {Name: "blastoise"}}, SubjectFilterMode: inventory.SubjectFilterExclude},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Charizard", PurchaseDate: "2026-07-06"},
			wantInScope: false,
		},
		{
			name:        "subjects (Exclude) includes pikachu (not denied)",
			campaign:    inventory.Campaign{ID: "c1", Subjects: []inventory.TargetSubject{{Name: "charizard"}, {Name: "blastoise"}}, SubjectFilterMode: inventory.SubjectFilterExclude},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Pikachu", PurchaseDate: "2026-07-06"},
			wantInScope: true,
		},
```

```go
// internal/domain/inventory/portfolio_test.go — replace the Campaign literal in TestExtractCharacter
	campaigns := []Campaign{
		{Subjects: []TargetSubject{{Name: "Charizard"}, {Name: "Pikachu"}, {Name: "Blastoise"}}},
	}
```

```go
// internal/domain/inventory/portfolio_test.go — replace the Campaign literals in
// Test_ComputePortfolioInsights (InclusionList: "Charizard" / "Pikachu" per campaign)
	campaigns := []Campaign{
		{ID: "c1", Phase: PhaseActive, Subjects: []TargetSubject{{Name: "Charizard"}}, SubjectFilterMode: SubjectFilterTarget},
		{ID: "c2", Phase: PhaseActive, Subjects: []TargetSubject{{Name: "Pikachu"}}, SubjectFilterMode: SubjectFilterTarget},
	}
```

```go
// internal/domain/inventory/portfolio_test.go — replace TestDetectCoverageGaps with a table
// that also pins down the deliberate open-net-covers-nothing decision from the judgment call above
func TestDetectCoverageGaps(t *testing.T) {
	byCharacter := []SegmentPerformance{
		{Label: "Charizard", ROI: 0.20, SoldCount: 5, CampaignCount: 1, Dimension: "character"},
		{Label: "Gengar", ROI: 0.25, SoldCount: 8, CampaignCount: 1, Dimension: "character"},
	}
	byGrade := []SegmentPerformance{
		{Label: "PSA 9", ROI: 0.18, SoldCount: 10, CampaignCount: 1, Dimension: "grade"},
	}

	cases := []struct {
		name          string
		campaigns     []Campaign
		wantGapLabels []string
	}{
		{
			name: "Target-mode campaign leaves Gengar uncovered",
			campaigns: []Campaign{
				{Phase: PhaseActive, Subjects: []TargetSubject{{Name: "Charizard"}, {Name: "Pikachu"}}, SubjectFilterMode: SubjectFilterTarget},
			},
			wantGapLabels: []string{"Gengar"},
		},
		{
			name: "open-net active campaign (no Subjects) covers nothing",
			campaigns: []Campaign{
				{Phase: PhaseActive},
			},
			wantGapLabels: []string{"Charizard", "Gengar"},
		},
		{
			name: "Exclude-mode campaign does not cover the characters it denies",
			campaigns: []Campaign{
				{Phase: PhaseActive, Subjects: []TargetSubject{{Name: "Charizard"}}, SubjectFilterMode: SubjectFilterExclude},
			},
			wantGapLabels: []string{"Charizard", "Gengar"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gaps := DetectCoverageGaps(byCharacter, byGrade, tc.campaigns)

			var gotLabels []string
			for _, g := range gaps {
				if g.Segment.Dimension == "character" {
					gotLabels = append(gotLabels, g.Segment.Label)
				}
			}
			if len(gotLabels) != len(tc.wantGapLabels) {
				t.Fatalf("expected character gaps %v, got %v", tc.wantGapLabels, gotLabels)
			}
			for i, want := range tc.wantGapLabels {
				if gotLabels[i] != want {
					t.Errorf("expected gap %q, got %q", want, gotLabels[i])
				}
			}
		})
	}
}
```

Each touch point below replaces a single line or literal in `internal/domain/inventory/suggestions_test.go`;
every other line in these tests (insight fixtures, assertion messages) is unchanged.

```go
// TestGenerateSuggestions_TopCharacterExpansion — replace the campaigns literal
	campaigns := []Campaign{
		{Name: "Campaign A", Phase: PhaseActive, Subjects: []TargetSubject{{Name: "Pikachu"}, {Name: "Blastoise"}}, SubjectFilterMode: SubjectFilterTarget},
	}
```

```go
// TestGenerateSuggestions_TopCharacterExpansion — replace the found-suggestion check
	for _, s := range resp.NewCampaigns {
		if len(s.SuggestedParams.Subjects) == 1 && s.SuggestedParams.Subjects[0] == "Charizard" {
			found = true
			if s.Confidence != "medium" {
				t.Errorf("expected medium confidence for 10 sales, got %s", s.Confidence)
			}
			if math.Abs(s.ExpectedMetrics.ExpectedROI-0.25) > 1e-6 {
				t.Errorf("expected ROI ~0.25, got %f", s.ExpectedMetrics.ExpectedROI)
			}
		}
	}
```

```go
// TestGenerateSuggestions_CharacterAdjustments — replace the campaigns literal
	campaigns := []Campaign{
		{Name: "Test Campaign", Phase: PhaseActive, Subjects: []TargetSubject{{Name: "Pikachu"}, {Name: "Blastoise"}}, SubjectFilterMode: SubjectFilterTarget},
	}
```

```go
// TestGenerateSuggestions_CoverageGap — replace the found-suggestion check
	for _, s := range resp.NewCampaigns {
		if s.Type == "gap" && len(s.SuggestedParams.Subjects) == 1 && s.SuggestedParams.Subjects[0] == "Gengar" {
			found = true
		}
	}
```

```go
// TestDeduplicateSuggestions — replace the "Add top performers to Campaign A" fixture
		{
			Type:       "adjust",
			Title:      "Add top performers to Campaign A",
			Confidence: "medium",
			DataPoints: 20,
			SuggestedParams: CampaignSuggestionParams{
				Name:     "Campaign A",
				Subjects: []string{"Charizard"},
			},
		},
```

```go
// TestPhaseTransition_ActivatePending — replace the campaigns literal
	campaigns := []Campaign{
		{ID: "c1", Name: "Pending Charizard", Phase: PhasePending, Subjects: []TargetSubject{{Name: "Charizard"}}, SubjectFilterMode: SubjectFilterTarget},
	}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./internal/domain/portfolio/... -run TestComputeAnalysisScopeFilter -v`
Expected: FAIL to compile — `unknown field Subjects in struct literal of type inventory.Campaign`
(`inventory.Campaign` doesn't carry `Subjects`/`SubjectFilterMode` as struct literal fields consumable
here until Task 2 lands, and `inScope` still reads `InclusionList`/`ExclusionMode`).

Run: `go test ./internal/domain/inventory/... -run 'TestExtractCharacter|Test_ComputePortfolioInsights|TestDetectCoverageGaps|TestGenerateSuggestions|TestDeduplicateSuggestions|TestPhaseTransition' -v`
Expected: FAIL to compile — same `unknown field Subjects` / `unknown field SuggestedParams.Subjects` errors
until the implementation step below lands.

- [ ] **Step 7: Write the implementation**

```go
// internal/domain/portfolio/analysis.go — replace inScope and its doc comment
// (parseRange, mondayOf, and the rest of the file are unchanged)

// inScope reports whether a purchase satisfies the campaign's filter criteria.
//
// Rules (each absent/unparsable constraint means no filter on that dimension):
//   - GradeRange: GradeValue ∈ [min, max]
//   - PriceRange: BuyCostCents ∈ [min*100, max*100]  (range stored in dollars)
//   - YearRange:  CardYear (int) ∈ [min, max]; skipped if CardYear is empty or non-numeric
//   - Subjects: CardPlayer must satisfy inventory.SubjectAxisMatches against the
//     campaign's Subjects/SubjectFilterMode — the same subject-axis predicate
//     PurchaseMatchesCampaign uses, so an empty Subjects list is an open net.
func inScope(c inventory.Campaign, p inventory.Purchase) bool {
	if minG, maxG, ok := parseRange(c.GradeRange); ok {
		if p.GradeValue < minG || p.GradeValue > maxG {
			return false
		}
	}

	if minP, maxP, ok := parseRange(c.PriceRange); ok {
		minCents := int(minP * 100)
		maxCents := int(maxP * 100)
		if p.BuyCostCents < minCents || p.BuyCostCents > maxCents {
			return false
		}
	}

	if minY, maxY, ok := parseRange(c.YearRange); ok && p.CardYear != "" {
		if year, err := strconv.Atoi(p.CardYear); err == nil {
			if float64(year) < minY || float64(year) > maxY {
				return false
			}
		}
	}

	if !inventory.SubjectAxisMatches(p.CardPlayer, c.Subjects, c.SubjectFilterMode) {
		return false
	}

	return true
}
```

```go
// internal/domain/inventory/portfolio.go — replace ExtractCharacter and DetectCoverageGaps
// (knownCharacters, ClassifyEra, computeCampaignMetrics, and the rest of the file are unchanged)

// ExtractCharacter returns the Pokemon character name from a card name using
// case-insensitive substring match against known characters. Returns "Other" if
// no match is found.
func ExtractCharacter(cardName string, campaigns []Campaign) string {
	lower := strings.ToLower(cardName)

	// Build combined character list from known characters + every campaign's
	// targeted subjects. Subject names are harvested regardless of
	// SubjectFilterMode — a denied name is still a legitimate character to
	// try extracting from a card name; this is a name harvest, not a match
	// decision, so it does not go through SubjectAxisMatches.
	chars := make([]string, len(knownCharacters))
	copy(chars, knownCharacters)
	seen := make(map[string]bool, len(knownCharacters))
	for _, c := range knownCharacters {
		seen[strings.ToLower(c)] = true
	}
	for _, camp := range campaigns {
		for _, subj := range camp.Subjects {
			name := strings.TrimSpace(subj.Name)
			if name != "" && !seen[strings.ToLower(name)] {
				seen[strings.ToLower(name)] = true
				chars = append(chars, name)
			}
		}
	}

	// Match longest first to avoid "Mew" matching before "Mewtwo"
	sort.Slice(chars, func(i, j int) bool {
		return len(chars[i]) > len(chars[j])
	})

	for _, ch := range chars {
		if strings.Contains(lower, strings.ToLower(ch)) {
			return ch
		}
	}
	return "Other"
}

// DetectCoverageGaps compares profitable segments against active campaign coverage.
func DetectCoverageGaps(byCharacter, byGrade []SegmentPerformance, campaigns []Campaign) []CoverageGap {
	var gaps []CoverageGap

	var activeCampaigns []Campaign
	for _, c := range campaigns {
		if c.Phase == PhaseActive {
			activeCampaigns = append(activeCampaigns, c)
		}
	}

	// isCovered reports whether any active campaign EXPLICITLY names character
	// on its subject axis.
	//
	// This deliberately does NOT delegate to SubjectAxisMatches, and the
	// asymmetry is intentional — see the judgment call above before
	// "simplifying" it. SubjectAxisMatches answers "would this campaign accept
	// this card", where an empty Subjects list is an open net that matches
	// everything. This function answers a different question: "does any
	// campaign still need to be pointed at this character". An open-net
	// campaign names nobody, so it contributes nothing here and gaps keep
	// surfacing. Routing this through SubjectAxisMatches would make a single
	// open-net active campaign suppress every gap forever.
	isCovered := func(character string) bool {
		for _, c := range activeCampaigns {
			if c.SubjectFilterMode == SubjectFilterExclude {
				continue
			}
			for _, s := range c.Subjects {
				if strings.EqualFold(strings.TrimSpace(s.Name), character) {
					return true
				}
			}
		}
		return false
	}

	// Check profitable characters not well-covered
	for _, seg := range byCharacter {
		if seg.ROI <= MinCharacterROI || seg.SoldCount < MinCharacterSales || seg.Label == "Other" {
			continue
		}
		if !isCovered(seg.Label) {
			gaps = append(gaps, CoverageGap{
				Segment:     seg,
				Reason:      fmt.Sprintf("%s has %.0f%% ROI across %d sales but is not targeted by any active campaign", seg.Label, seg.ROI*100, seg.SoldCount),
				Opportunity: fmt.Sprintf("Add %s to an existing campaign or create a dedicated campaign", seg.Label),
			})
		}
	}

	// Check profitable grades
	for _, seg := range byGrade {
		if seg.ROI <= MinGradeROI || seg.SoldCount < MinGradeSales || seg.CampaignCount >= len(activeCampaigns) {
			continue
		}
		gaps = append(gaps, CoverageGap{
			Segment:     seg,
			Reason:      fmt.Sprintf("%s has %.0f%% ROI but only appears in %d of %d active campaigns", seg.Label, seg.ROI*100, seg.CampaignCount, len(activeCampaigns)),
			Opportunity: fmt.Sprintf("Expand %s coverage to more campaigns", seg.Label),
		})
	}

	return gaps
}
```

```go
// internal/domain/inventory/suggestion_rules.go — replace suggestTopCharacterExpansion
// and suggestCoverageGapCampaigns (suggestGradeSweetSpot, suggestChannelInformedBuyTerms,
// and suggestBuyTermsFromLiquidation don't touch Subjects/InclusionList and are unchanged)

func suggestTopCharacterExpansion(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	for _, seg := range insights.ByCharacter {
		if seg.ROI <= suggMinROIExpansion || seg.SoldCount < suggMinSoldForConfidence || seg.Label == "Other" {
			continue
		}
		if seg.CampaignCount >= suggMaxCampaignsPerCharacter {
			continue
		}

		var missingCampaigns []string
		for _, c := range campaigns {
			if c.Phase != PhaseActive {
				continue
			}
			if !SubjectAxisMatches(seg.Label, c.Subjects, c.SubjectFilterMode) {
				missingCampaigns = append(missingCampaigns, c.Name)
			}
		}

		if len(missingCampaigns) == 0 {
			continue
		}

		suggestions = append(suggestions, CampaignSuggestion{
			Type:  "new",
			Title: fmt.Sprintf("Expand %s to more campaigns", seg.Label),
			Rationale: fmt.Sprintf("%s has %.0f%% ROI across %d sales in %d inventory. Adding to: %s",
				seg.Label, seg.ROI*100, seg.SoldCount, seg.CampaignCount, strings.Join(missingCampaigns, ", ")),
			Confidence: mathutil.ConfidenceLabel(seg.SoldCount),
			DataPoints: seg.PurchaseCount,
			SuggestedParams: CampaignSuggestionParams{
				Name:        fmt.Sprintf("%s Focus", seg.Label),
				Subjects:    []string{seg.Label},
				PrimaryExit: string(seg.BestChannel),
			},
			ExpectedMetrics: ExpectedMetrics{
				ExpectedROI:       seg.ROI,
				ExpectedMarginPct: seg.AvgMarginPct,
				AvgDaysToSell:     seg.AvgDaysToSell,
				DataConfidence:    mathutil.ConfidenceLabel(seg.SoldCount),
			},
		})
	}

	return suggestions
}

func suggestCoverageGapCampaigns(_ context.Context, insights *PortfolioInsights) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	for _, gap := range insights.CoverageGaps {
		seg := gap.Segment
		if seg.Dimension != "character" || seg.SoldCount < suggMinSoldCoverageGap {
			continue
		}

		suggestions = append(suggestions, CampaignSuggestion{
			Type:       "gap",
			Title:      fmt.Sprintf("Coverage Gap: %s", seg.Label),
			Rationale:  gap.Reason,
			Confidence: mathutil.ConfidenceLabel(seg.SoldCount),
			DataPoints: seg.PurchaseCount,
			SuggestedParams: CampaignSuggestionParams{
				Name:        fmt.Sprintf("%s Campaign", seg.Label),
				Subjects:    []string{seg.Label},
				PrimaryExit: string(seg.BestChannel),
			},
			ExpectedMetrics: ExpectedMetrics{
				ExpectedROI:       seg.ROI,
				ExpectedMarginPct: seg.AvgMarginPct,
				AvgDaysToSell:     seg.AvgDaysToSell,
				DataConfidence:    mathutil.ConfidenceLabel(seg.SoldCount),
			},
		})
	}

	return suggestions
}
```

```go
// internal/domain/inventory/suggestion_rules_optimization.go — replace suggestCharacterAdjustments
// and suggestPhaseTransitions (suggestSpendCapRebalancing, expectedROIFromMargin, and
// computeBuyTermsReduction don't touch Subjects/InclusionList and are unchanged)

func suggestCharacterAdjustments(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.ByCharacter) < suggMinCharacterSegments {
		return nil
	}

	sorted := make([]SegmentPerformance, len(insights.ByCharacter))
	copy(sorted, insights.ByCharacter)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ROI > sorted[j].ROI
	})

	for _, c := range campaigns {
		if c.Phase != PhaseActive || len(c.Subjects) == 0 {
			continue
		}

		var removes []string
		var adds []string

		for _, seg := range sorted {
			if seg.Label == "Other" || seg.PurchaseCount < suggMinSoldForConfidence {
				continue
			}
			targeted := SubjectAxisMatches(seg.Label, c.Subjects, c.SubjectFilterMode)

			if targeted && seg.ROI < suggUnderperformingROI && seg.SoldCount >= suggMinSoldForRemoval {
				removes = append(removes, seg.Label)
			}

			if !targeted && seg.ROI > suggMinROIExpansion && seg.SoldCount >= suggMinSoldForConfidence && len(adds) < suggMaxCampaignsPerCharacter {
				adds = append(adds, seg.Label)
			}
		}

		if len(removes) > 0 {
			suggestions = append(suggestions, CampaignSuggestion{
				Type:  "adjust",
				Title: fmt.Sprintf("Remove underperformers from %s", c.Name),
				Rationale: fmt.Sprintf("Characters %s have underperforming ROI in campaign %s. Consider removing from targeting.",
					strings.Join(removes, ", "), c.Name),
				Confidence: "medium",
				DataPoints: insights.DataSummary.TotalPurchases,
				SuggestedParams: CampaignSuggestionParams{
					Name:     c.Name,
					Subjects: removes,
				},
				ExpectedMetrics: ExpectedMetrics{
					// Removing a segment — expected improvement is directional only.
					ExpectedROI:    0,
					DataConfidence: "medium",
				},
			})
		}

		if len(adds) > 0 {
			// Find the best matching segment in `sorted` for the top add
			// (adds[0] is the highest-ROI entry because `sorted` is ordered
			// by ROI descending and adds preserves that order).
			var expectedROI, expectedMargin, avgDays float64
			for _, seg := range sorted {
				if strings.EqualFold(strings.TrimSpace(seg.Label), adds[0]) {
					expectedROI = seg.ROI
					expectedMargin = seg.AvgMarginPct
					avgDays = seg.AvgDaysToSell
					break
				}
			}

			suggestions = append(suggestions, CampaignSuggestion{
				Type:  "adjust",
				Title: fmt.Sprintf("Add top performers to %s", c.Name),
				Rationale: fmt.Sprintf("Characters %s are top performers not in campaign %s. Consider adding.",
					strings.Join(adds, ", "), c.Name),
				Confidence: "medium",
				DataPoints: insights.DataSummary.TotalPurchases,
				SuggestedParams: CampaignSuggestionParams{
					Name:     c.Name,
					Subjects: adds,
				},
				ExpectedMetrics: ExpectedMetrics{
					ExpectedROI:       expectedROI,
					ExpectedMarginPct: expectedMargin,
					AvgDaysToSell:     avgDays,
					DataConfidence:    "medium",
				},
			})
		}
	}

	return suggestions
}

func suggestPhaseTransitions(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	// metricsMap is only needed for PhaseActive close suggestions
	metricsMap := make(map[string]CampaignPNLBrief)
	for _, m := range insights.CampaignMetrics {
		metricsMap[m.CampaignID] = m
	}

	for _, c := range campaigns {
		if c.Phase == PhaseActive {
			m, ok := metricsMap[c.ID]
			if !ok {
				continue
			}
			if m.PurchaseCount >= suggArchiveMinPurchases && m.ROI < suggArchiveROIThreshold {
				sellThrough := 0.0
				if m.PurchaseCount > 0 {
					sellThrough = float64(m.SoldCount) / float64(m.PurchaseCount)
				}
				if sellThrough < suggLowSellThroughPct {
					suggestions = append(suggestions, CampaignSuggestion{
						Type:  "adjust",
						Title: fmt.Sprintf("Consider closing %s", c.Name),
						Rationale: fmt.Sprintf("%s has %.0f%% ROI with %.0f%% sell-through across %d purchases. Performance is below viable thresholds.",
							c.Name, m.ROI*100, sellThrough*100, m.PurchaseCount),
						Confidence: mathutil.ConfidenceLabel(m.SoldCount),
						DataPoints: m.PurchaseCount,
						SuggestedParams: CampaignSuggestionParams{
							Name: c.Name,
						},
						ExpectedMetrics: ExpectedMetrics{
							ExpectedROI:    m.ROI,
							DataConfidence: mathutil.ConfidenceLabel(m.SoldCount),
						},
					})
				}
			}
		}

		// PhasePending activation uses insights.ByCharacter directly — no metrics needed
		if c.Phase == PhasePending && len(c.Subjects) > 0 {
			var profitableChars []string
			var bestROI float64
			for _, seg := range insights.ByCharacter {
				if seg.ROI > suggActivateMinROI && seg.SoldCount >= suggMinSoldForConfidence {
					if SubjectAxisMatches(seg.Label, c.Subjects, c.SubjectFilterMode) {
						profitableChars = append(profitableChars, fmt.Sprintf("%s (%.0f%% ROI)", seg.Label, seg.ROI*100))
						if seg.ROI > bestROI {
							bestROI = seg.ROI
						}
					}
				}
			}
			if len(profitableChars) > 0 {
				suggestions = append(suggestions, CampaignSuggestion{
					Type:  "adjust",
					Title: fmt.Sprintf("Activate %s", c.Name),
					Rationale: fmt.Sprintf("%s targets profitable characters: %s. Consider activating.",
						c.Name, strings.Join(profitableChars, ", ")),
					Confidence: "medium",
					DataPoints: insights.DataSummary.TotalSales,
					SuggestedParams: CampaignSuggestionParams{
						Name: c.Name,
					},
					ExpectedMetrics: ExpectedMetrics{
						ExpectedROI:    bestROI,
						DataConfidence: "medium",
					},
				})
			}
		}
	}

	return suggestions
}
```

```go
// internal/domain/inventory/suggestion_types.go — replace CampaignSuggestionParams
// (CampaignSuggestion, ExpectedMetrics, and SuggestionsResponse are unchanged)

// CampaignSuggestionParams holds the suggested campaign configuration.
type CampaignSuggestionParams struct {
	Name                    string   `json:"name"`
	YearRange               string   `json:"yearRange,omitempty"`
	GradeRange              string   `json:"gradeRange,omitempty"`
	PriceRange              string   `json:"priceRange,omitempty"`
	BuyTermsCLPct           float64  `json:"buyTermsCLPct,omitempty"`
	BuyTermsCLPctOptimistic float64  `json:"buyTermsCLPctOptimistic,omitempty"`
	DailySpendCapCents      int      `json:"dailySpendCapCents,omitempty"`
	Subjects                []string `json:"subjects,omitempty"`
	PrimaryExit             string   `json:"primaryExit,omitempty"`
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test -race ./internal/domain/portfolio/... -run TestComputeAnalysisScopeFilter -v`
Expected: PASS

Run: `go test -race ./internal/domain/inventory/... -run 'TestExtractCharacter|Test_ComputePortfolioInsights|TestDetectCoverageGaps|TestGenerateSuggestions|TestDeduplicateSuggestions|TestPhaseTransition' -v`
Expected: PASS

Run: `go test -race ./internal/domain/portfolio/... ./internal/domain/inventory/...`
Expected: PASS (full package run — catches any other InclusionList/ExclusionMode reader this task's
`grep -rn "InclusionList\|ExclusionMode"` sweep missed)

Run: `grep -rln "InclusionList\|ExclusionMode" --include='*.go' internal/ cmd/ | grep -v _test.go`
Expected: exactly two files — `internal/domain/inventory/types_core.go` (the field declarations, kept as
the legacy mirror per Task 2) and `internal/adapters/storage/postgres/campaign_store.go` (the sole writer
that populates the mirror from `Subjects`/`SubjectFilterMode`). Any other file in the output is a miss — go
back and convert it to `Subjects`/`SubjectFilterMode`/`SubjectAxisMatches` before moving on.

The sweep is scoped to `internal/ cmd/`, not to the two domain directories this task edits: the second
expected survivor lives under `internal/adapters/storage/postgres/`, so a narrower scope would silently
drop it from the expected set and make the assertion unverifiable. For reference, this grep returns seven
non-test files before Task 4 lands.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/demand/repository.go internal/domain/demand/campaign_signals.go internal/domain/demand/campaign_signals_test.go internal/adapters/storage/postgres/campaign_coverage.go internal/adapters/storage/postgres/campaign_coverage_test.go internal/domain/portfolio/analysis.go internal/domain/portfolio/analysis_test.go internal/domain/inventory/suggestion_rules.go internal/domain/inventory/suggestion_rules_optimization.go internal/domain/inventory/portfolio.go internal/domain/inventory/portfolio_test.go internal/domain/inventory/suggestion_types.go internal/domain/inventory/suggestions_test.go
git commit -m "feat: move coverage, demand, portfolio, and suggestion consumers to the subject axis"
```

### Task 5: psacampaign catalog port + Postgres store

**Files:**
- Create: `internal/domain/psacampaign/resolver.go`
- Create: `internal/domain/psacampaign/resolver_test.go`
- Create: `internal/adapters/storage/postgres/migrations/000024_psa_portal_catalog.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000024_psa_portal_catalog.down.sql`
- Create: `internal/adapters/storage/postgres/psa_portal_catalog_store.go`
- Create: `internal/adapters/storage/postgres/psa_portal_catalog_store_test.go`
- Modify: `internal/testutil/mocks/psa_campaign_stores.go` (append `CatalogStoreMock`)
- Modify: `cmd/slabledger/server.go:65-66` (add `PSACatalogStore` dependency field), `cmd/slabledger/server.go:182-187` (wire `WithPSACatalogStore` option — that handler option belongs to another task; this task only adds the `ServerDependencies` field and the `if deps.PSACatalogStore != nil` construction block, matching the existing `PSASnapshotStore`/`PSAPushQueue` shape)
- Modify: `cmd/slabledger/handlers.go:333-339` (construct `postgres.NewPSAPortalCatalogStore(in.DB.DB)` inside the existing `if in.DB != nil` block)

**Interfaces:**
- Consumes: `SubjectRef{ID int; Name string}` (`internal/domain/psacampaign/types.go:41-44`, already exists)
- Produces:
  - `SpecListRef{ID, Name, Status string}` (`internal/domain/psacampaign/resolver.go`)
  - `type CatalogStore interface { SaveSpecLists(ctx context.Context, lists []SpecListRef) error; SaveSubjects(ctx context.Context, categoryID int, subjects []SubjectRef) error; SpecLists(ctx context.Context) ([]SpecListRef, time.Time, error); Subjects(ctx context.Context, categoryID int) ([]SubjectRef, time.Time, error) }`
  - `type Resolver interface { SpecListIDs(languageToken string) ([]string, error); SubjectID(name string) (int, error) }`
  - `func NewCatalogResolver(specLists []SpecListRef, subjects []SubjectRef, fetchedAt, now time.Time) (Resolver, error)`
  - `const PokemonCategoryID = 16`, `const CatalogMaxAge = 7 * 24 * time.Hour`
  - `var ErrCatalogStale, ErrUnknownSpecList, ErrUnknownSubject error`
  - `func NewPSAPortalCatalogStore(db *sql.DB) *PSAPortalCatalogStore` implementing `CatalogStore`
  - `mocks.CatalogStoreMock` with `SaveSpecListsFn func(ctx context.Context, lists psacampaign.SpecListRef) error` — **exact four method signatures a consuming task needs**:
    - `SaveSpecListsFn func(ctx context.Context, lists []psacampaign.SpecListRef) error`
    - `SaveSubjectsFn func(ctx context.Context, categoryID int, subjects []psacampaign.SubjectRef) error`
    - `SpecListsFn func(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error)`
    - `SubjectsFn func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error)`

---

- [ ] **Step 1: Write the failing test for the resolver**

```go
// internal/domain/psacampaign/resolver_test.go
package psacampaign

import (
	"errors"
	"testing"
	"time"
)

func TestNewCatalogResolver_Staleness(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		fetchedAt time.Time
		wantErr   error
	}{
		{"fresh catalog", now.Add(-time.Hour), nil},
		{"exactly at max age is still fresh", now.Add(-CatalogMaxAge), nil},
		{"stale catalog", now.Add(-CatalogMaxAge - time.Second), ErrCatalogStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCatalogResolver(nil, nil, tt.fetchedAt, now)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("NewCatalogResolver() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewCatalogResolver() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCatalogResolver_SpecListIDs(t *testing.T) {
	// Synthetic fixture UUIDs — the real portal UUIDs for "Japanese Pokemon" and
	// "English Pokemon" are unknown until the baseline pull runs (they are new
	// portal entries). Do not treat these as real values.
	lists := []SpecListRef{
		{ID: "fixture-uuid-japanese-pokemon", Name: "Japanese Pokemon", Status: "ENABLED"},
		{ID: "fixture-uuid-english-pokemon", Name: "English Pokemon", Status: "ENABLED"},
		{ID: "fixture-uuid-riftbound", Name: "Riftbound", Status: "ENABLED"},
		{ID: "fixture-uuid-disabled-english", Name: "english pokemon", Status: "DISABLED"},
	}
	now := time.Now()
	fetchedAt := now.Add(-time.Hour)

	tests := []struct {
		name          string
		languageToken string
		wantIDs       []string
		wantErr       error
	}{
		{"japanese resolves", "japanese", []string{"fixture-uuid-japanese-pokemon"}, nil},
		{"english resolves, disabled duplicate skipped", "english", []string{"fixture-uuid-english-pokemon"}, nil},
		{"unknown token", "korean", nil, ErrUnknownSpecList},
		{"empty token", "", nil, ErrUnknownSpecList},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewCatalogResolver(lists, nil, fetchedAt, now)
			if err != nil {
				t.Fatalf("NewCatalogResolver() unexpected error = %v", err)
			}
			ids, err := r.SpecListIDs(tt.languageToken)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SpecListIDs() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SpecListIDs() unexpected error = %v", err)
			}
			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("SpecListIDs() = %v, want %v", ids, tt.wantIDs)
			}
			for i := range ids {
				if ids[i] != tt.wantIDs[i] {
					t.Fatalf("SpecListIDs()[%d] = %q, want %q", i, ids[i], tt.wantIDs[i])
				}
			}
		})
	}

	t.Run("only disabled match yields ErrUnknownSpecList", func(t *testing.T) {
		onlyDisabled := []SpecListRef{
			{ID: "fixture-uuid-disabled-japanese", Name: "Japanese Pokemon", Status: "DISABLED"},
		}
		r, err := NewCatalogResolver(onlyDisabled, nil, fetchedAt, now)
		if err != nil {
			t.Fatalf("NewCatalogResolver() unexpected error = %v", err)
		}
		if _, err := r.SpecListIDs("japanese"); !errors.Is(err, ErrUnknownSpecList) {
			t.Fatalf("SpecListIDs() error = %v, want %v", err, ErrUnknownSpecList)
		}
	})
}

func TestCatalogResolver_SubjectID(t *testing.T) {
	// Synthetic fixture subject ids — arbitrary and not drawn from any real
	// portal capture.
	subjects := []SubjectRef{
		{ID: 990001, Name: "Fixture Charizard"},
		{ID: 990002, Name: "Fixture Pikachu"},
	}
	now := time.Now()
	fetchedAt := now.Add(-time.Hour)
	r, err := NewCatalogResolver(nil, subjects, fetchedAt, now)
	if err != nil {
		t.Fatalf("NewCatalogResolver() unexpected error = %v", err)
	}

	tests := []struct {
		name    string
		subject string
		wantID  int
		wantErr error
	}{
		{"exact case match", "Fixture Charizard", 990001, nil},
		{"case-insensitive match", "fixture pikachu", 990002, nil},
		{"unknown subject", "Fixture Mewtwo", 0, ErrUnknownSubject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := r.SubjectID(tt.subject)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SubjectID() error = %v, want %v", err, tt.wantErr)
				}
				if id != 0 {
					t.Fatalf("SubjectID() = %d on error, want 0", id)
				}
				return
			}
			if err != nil {
				t.Fatalf("SubjectID() unexpected error = %v", err)
			}
			if id != tt.wantID {
				t.Fatalf("SubjectID() = %d, want %d", id, tt.wantID)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/psacampaign/... -run TestNewCatalogResolver_Staleness -v`
Expected: FAIL with `undefined: NewCatalogResolver` (and `CatalogMaxAge`, `ErrCatalogStale`, `SpecListRef`, `SubjectID` etc. across the three test functions).

- [ ] **Step 3: Implement the resolver**

```go
// internal/domain/psacampaign/resolver.go
package psacampaign

import (
	"context"
	"errors"
	"strings"
	"time"
)

// PokemonCategoryID is the portal category id passed to getSubjects.
const PokemonCategoryID = 16

// CatalogMaxAge is how stale a persisted portal catalog may be before
// translation refuses to run.
const CatalogMaxAge = 7 * 24 * time.Hour

var (
	ErrCatalogStale    = errors.New("psacampaign: portal catalog is stale")
	ErrUnknownSpecList = errors.New("psacampaign: no spec list for language")
	ErrUnknownSubject  = errors.New("psacampaign: no subject id for name")
)

// SpecListRef is one curated ("prepackaged") spec list offered by the portal.
type SpecListRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // "ENABLED" | …
}

// CatalogStore persists PSA-side reference data harvested by the browser job
// so the main server — which has no portal session — can resolve names to
// portal identifiers at translation time. The harvester writes it on every
// run; the main server only ever reads it.
type CatalogStore interface {
	SaveSpecLists(ctx context.Context, lists []SpecListRef) error
	SaveSubjects(ctx context.Context, categoryID int, subjects []SubjectRef) error
	SpecLists(ctx context.Context) ([]SpecListRef, time.Time, error)
	Subjects(ctx context.Context, categoryID int) ([]SubjectRef, time.Time, error)
}

// Resolver maps SlabLedger's internal targeting vocabulary onto portal ids.
type Resolver interface {
	SpecListIDs(languageToken string) ([]string, error)
	SubjectID(name string) (int, error)
}

// languageListNames maps a canonical language token onto the curated portal
// spec list name it resolves to. Matching is case-insensitive against
// SpecListRef.Name. Only english/japanese are known today — the portal offers
// no curated Chinese/Korean list at design time.
var languageListNames = map[string]string{
	"english":  "English Pokemon",
	"japanese": "Japanese Pokemon",
}

// catalogResolver is the pure, in-memory Resolver built by NewCatalogResolver.
type catalogResolver struct {
	specLists []SpecListRef
	subjects  []SubjectRef
}

// NewCatalogResolver builds a Resolver from a persisted catalog snapshot. It
// takes no context and performs no I/O — this purity is why translation can
// run in the main HTTP server, which has no portal session. It returns
// ErrCatalogStale when fetchedAt is older than CatalogMaxAge relative to now.
func NewCatalogResolver(specLists []SpecListRef, subjects []SubjectRef, fetchedAt, now time.Time) (Resolver, error) {
	if now.Sub(fetchedAt) > CatalogMaxAge {
		return nil, ErrCatalogStale
	}
	return &catalogResolver{specLists: specLists, subjects: subjects}, nil
}

// SpecListIDs maps a language token to the portal UUID(s) of the matching
// list(s) whose Name equals the token's curated list name (case-insensitive)
// and whose Status is "ENABLED". Lists with any other status are skipped even
// when the name matches, since the portal can retire a list without removing
// it from the catalog payload.
func (r *catalogResolver) SpecListIDs(languageToken string) ([]string, error) {
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

// SubjectID resolves a subject name to its portal id, case-insensitively.
// This is for NEW subjects an operator adds by name — ids that came from the
// portal are copied verbatim and never re-resolved here. Never silently
// returns 0 for an unresolved name; callers must check err.
func (r *catalogResolver) SubjectID(name string) (int, error) {
	for _, s := range r.subjects {
		if strings.EqualFold(s.Name, name) {
			return s.ID, nil
		}
	}
	return 0, ErrUnknownSubject
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/domain/psacampaign/... -run 'TestNewCatalogResolver_Staleness|TestCatalogResolver_SpecListIDs|TestCatalogResolver_SubjectID' -v`
Expected: PASS

- [ ] **Step 5: Commit the resolver**

```bash
git add internal/domain/psacampaign/resolver.go internal/domain/psacampaign/resolver_test.go
git commit -m "feat(psacampaign): add pure CatalogStore/Resolver port for portal catalog"
```

- [ ] **Step 6: Add migration 000024**

```sql
-- internal/adapters/storage/postgres/migrations/000024_psa_portal_catalog.up.sql
-- Persists PSA portal reference data (curated spec lists, subject lists)
-- harvested by cmd/psa-harvest so the main server — which has no portal
-- session — can resolve names to portal ids at translation time.
-- kind is 'spec_lists' or 'subjects'; key is '' for the singleton spec-list
-- catalog and the category id (as text) for a subjects catalog.
CREATE TABLE psa_portal_catalog (
    kind       TEXT        NOT NULL,
    key        TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (kind, key)
);
```

```sql
-- internal/adapters/storage/postgres/migrations/000024_psa_portal_catalog.down.sql
DROP TABLE IF EXISTS psa_portal_catalog;
```

- [ ] **Step 7: Write the failing test for the Postgres store**

```go
// internal/adapters/storage/postgres/psa_portal_catalog_store_test.go
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/stretchr/testify/require"
)

func TestPSAPortalCatalogStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	store := NewPSAPortalCatalogStore(db.DB)
	_, err := db.ExecContext(ctx, `DELETE FROM psa_portal_catalog`)
	require.NoError(t, err)

	t.Run("empty store returns zero values, no error", func(t *testing.T) {
		lists, fetchedAt, err := store.SpecLists(ctx)
		require.NoError(t, err)
		require.Empty(t, lists)
		require.True(t, fetchedAt.IsZero())

		subjects, fetchedAt, err := store.Subjects(ctx, psacampaign.PokemonCategoryID)
		require.NoError(t, err)
		require.Empty(t, subjects)
		require.True(t, fetchedAt.IsZero())
	})

	t.Run("refuses to save an empty spec-list catalog", func(t *testing.T) {
		err := store.SaveSpecLists(ctx, nil)
		require.Error(t, err)
	})

	t.Run("refuses to save an empty subject catalog", func(t *testing.T) {
		err := store.SaveSubjects(ctx, psacampaign.PokemonCategoryID, []psacampaign.SubjectRef{})
		require.Error(t, err)
	})

	t.Run("save and read back spec lists", func(t *testing.T) {
		// Synthetic fixture values — not real portal UUIDs.
		want := []psacampaign.SpecListRef{
			{ID: "fixture-uuid-japanese-pokemon", Name: "Japanese Pokemon", Status: "ENABLED"},
			{ID: "fixture-uuid-english-pokemon", Name: "English Pokemon", Status: "ENABLED"},
		}
		require.NoError(t, store.SaveSpecLists(ctx, want))

		got, fetchedAt, err := store.SpecLists(ctx)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.WithinDuration(t, time.Now(), fetchedAt, 5*time.Second)
	})

	t.Run("second save overwrites (upsert on kind+key)", func(t *testing.T) {
		newer := []psacampaign.SpecListRef{
			{ID: "fixture-uuid-riftbound", Name: "Riftbound", Status: "ENABLED"},
		}
		require.NoError(t, store.SaveSpecLists(ctx, newer))

		got, _, err := store.SpecLists(ctx)
		require.NoError(t, err)
		require.Equal(t, newer, got)

		var count int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT count(*) FROM psa_portal_catalog WHERE kind = 'spec_lists'`).Scan(&count))
		require.Equal(t, 1, count)
	})

	t.Run("save and read back subjects, keyed by category id", func(t *testing.T) {
		// Synthetic fixture subject ids — arbitrary, not from any real capture.
		want := []psacampaign.SubjectRef{
			{ID: 990001, Name: "Fixture Charizard"},
			{ID: 990002, Name: "Fixture Pikachu"},
		}
		require.NoError(t, store.SaveSubjects(ctx, psacampaign.PokemonCategoryID, want))

		got, fetchedAt, err := store.Subjects(ctx, psacampaign.PokemonCategoryID)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.WithinDuration(t, time.Now(), fetchedAt, 5*time.Second)

		// A different category id has its own row and stays empty.
		otherCategory, _, err := store.Subjects(ctx, psacampaign.PokemonCategoryID+1)
		require.NoError(t, err)
		require.Empty(t, otherCategory)
	})
}
```

- [ ] **Step 8: Run test to verify it fails**

Run: `POSTGRES_TEST_URL=<test-db-dsn> go test ./internal/adapters/storage/postgres/... -run TestPSAPortalCatalogStore -v`
Expected: FAIL with `undefined: NewPSAPortalCatalogStore` (compile error; if `POSTGRES_TEST_URL` is unset the test skips instead — set it to exercise this step, per `make test-postgres`).

- [ ] **Step 9: Implement the Postgres store**

```go
// internal/adapters/storage/postgres/psa_portal_catalog_store.go
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

const (
	catalogKindSpecLists = "spec_lists"
	catalogKindSubjects  = "subjects"
	catalogSpecListsKey  = ""
)

// PSAPortalCatalogStore persists PSA portal reference data (curated spec
// lists, subject lists) as JSONB rows keyed by (kind, key), one row per
// distinct catalog (migration 000024).
type PSAPortalCatalogStore struct {
	db *sql.DB
}

var _ psacampaign.CatalogStore = (*PSAPortalCatalogStore)(nil)

func NewPSAPortalCatalogStore(db *sql.DB) *PSAPortalCatalogStore {
	return &PSAPortalCatalogStore{db: db}
}

// SaveSpecLists upserts the singleton spec-list catalog. An empty catalog is
// refused: silently storing one would make every subsequent translation see
// no enabled list for any language and fail closed for every campaign, which
// is a worse failure mode than a stale-but-populated catalog.
func (s *PSAPortalCatalogStore) SaveSpecLists(ctx context.Context, lists []psacampaign.SpecListRef) error {
	if len(lists) == 0 {
		return fmt.Errorf("psa_portal_catalog: refusing to save empty spec-list catalog")
	}
	return s.save(ctx, catalogKindSpecLists, catalogSpecListsKey, lists)
}

// SaveSubjects upserts the subject catalog for one category id. Refused when
// empty for the same reason as SaveSpecLists.
func (s *PSAPortalCatalogStore) SaveSubjects(ctx context.Context, categoryID int, subjects []psacampaign.SubjectRef) error {
	if len(subjects) == 0 {
		return fmt.Errorf("psa_portal_catalog: refusing to save empty subject catalog for category %d", categoryID)
	}
	return s.save(ctx, catalogKindSubjects, strconv.Itoa(categoryID), subjects)
}

func (s *PSAPortalCatalogStore) save(ctx context.Context, kind, key string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("psa_portal_catalog: marshal %s: %w", kind, err)
	}
	const q = `
		INSERT INTO psa_portal_catalog (kind, key, payload, fetched_at)
		VALUES ($1, $2, $3::jsonb, now())
		ON CONFLICT (kind, key) DO UPDATE
		   SET payload    = EXCLUDED.payload,
		       fetched_at = now()`
	if _, err := s.db.ExecContext(ctx, q, kind, key, string(raw)); err != nil {
		return fmt.Errorf("psa_portal_catalog: upsert %s: %w", kind, err)
	}
	return nil
}

// SpecLists returns the persisted spec-list catalog and when it was fetched.
// No row yet → (empty slice, zero time, nil).
func (s *PSAPortalCatalogStore) SpecLists(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error) {
	var lists []psacampaign.SpecListRef
	fetchedAt, err := s.load(ctx, catalogKindSpecLists, catalogSpecListsKey, &lists)
	if err != nil {
		return nil, time.Time{}, err
	}
	return lists, fetchedAt, nil
}

// Subjects returns the persisted subject catalog for one category id and
// when it was fetched. No row yet → (empty slice, zero time, nil).
func (s *PSAPortalCatalogStore) Subjects(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
	var subjects []psacampaign.SubjectRef
	fetchedAt, err := s.load(ctx, catalogKindSubjects, strconv.Itoa(categoryID), &subjects)
	if err != nil {
		return nil, time.Time{}, err
	}
	return subjects, fetchedAt, nil
}

func (s *PSAPortalCatalogStore) load(ctx context.Context, kind, key string, out any) (time.Time, error) {
	const q = `SELECT payload, fetched_at FROM psa_portal_catalog WHERE kind = $1 AND key = $2`
	var raw []byte
	var fetchedAt time.Time
	err := s.db.QueryRowContext(ctx, q, kind, key).Scan(&raw, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("psa_portal_catalog: query %s: %w", kind, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return time.Time{}, fmt.Errorf("psa_portal_catalog: unmarshal %s: %w", kind, err)
	}
	return fetchedAt, nil
}
```

- [ ] **Step 10: Run test to verify it passes**

Run: `POSTGRES_TEST_URL=<test-db-dsn> go test -race ./internal/adapters/storage/postgres/... -run TestPSAPortalCatalogStore -v`
Expected: PASS

- [ ] **Step 11: Commit the migration and store**

```bash
git add internal/adapters/storage/postgres/migrations/000024_psa_portal_catalog.up.sql \
        internal/adapters/storage/postgres/migrations/000024_psa_portal_catalog.down.sql \
        internal/adapters/storage/postgres/psa_portal_catalog_store.go \
        internal/adapters/storage/postgres/psa_portal_catalog_store_test.go
git commit -m "feat(postgres): add psa_portal_catalog table and CatalogStore implementation"
```

- [ ] **Step 12: Wire the store into cmd/slabledger**

Modify `cmd/slabledger/server.go` — add the field beside the two existing PSA store fields:

```go
	PSASnapshotStore          psacampaign.SnapshotStore        // optional: PSA campaign snapshot reader
	PSAPushQueue              psacampaign.PushQueueStore       // optional: PSA campaign push-queue reader/writer
	PSACatalogStore           psacampaign.CatalogStore         // optional: PSA portal catalog reader (spec lists + subjects)
```

Modify `cmd/slabledger/handlers.go:333-339` — construct it alongside the existing DB-backed stores:

```go
	// Wire PSA campaign snapshot + push-queue stores so the read/approve
	// endpoints work even when the harvester (which populates them) isn't
	// running. DB-only, cheap to construct.
	if in.DB != nil {
		deps.PSASnapshotStore = postgres.NewPSACampaignSnapshotStore(in.DB.DB)
		deps.PSAPushQueue = postgres.NewPSACampaignPushQueueStore(in.DB.DB)
		deps.PSACatalogStore = postgres.NewPSAPortalCatalogStore(in.DB.DB)
	}
```

No handler wiring here — a `WithPSACatalogStore` `CampaignsHandlerOption` and the `/api/psa/subjects` endpoint that reads `deps.PSACatalogStore` belong to the frontend/handler task, not this one. This task only makes the store constructible and available on `ServerDependencies`.

- [ ] **Step 13: Add the CatalogStoreMock**

Append to `internal/testutil/mocks/psa_campaign_stores.go`:

```go
// CatalogStoreMock implements psacampaign.CatalogStore with the Fn-field pattern.
type CatalogStoreMock struct {
	SaveSpecListsFn func(ctx context.Context, lists []psacampaign.SpecListRef) error
	SaveSubjectsFn  func(ctx context.Context, categoryID int, subjects []psacampaign.SubjectRef) error
	SpecListsFn     func(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error)
	SubjectsFn      func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error)
}

var _ psacampaign.CatalogStore = (*CatalogStoreMock)(nil)

func (m *CatalogStoreMock) SaveSpecLists(ctx context.Context, lists []psacampaign.SpecListRef) error {
	if m.SaveSpecListsFn != nil {
		return m.SaveSpecListsFn(ctx, lists)
	}
	return nil
}

func (m *CatalogStoreMock) SaveSubjects(ctx context.Context, categoryID int, subjects []psacampaign.SubjectRef) error {
	if m.SaveSubjectsFn != nil {
		return m.SaveSubjectsFn(ctx, categoryID, subjects)
	}
	return nil
}

func (m *CatalogStoreMock) SpecLists(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error) {
	if m.SpecListsFn != nil {
		return m.SpecListsFn(ctx)
	}
	return []psacampaign.SpecListRef{}, time.Time{}, nil
}

func (m *CatalogStoreMock) Subjects(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
	if m.SubjectsFn != nil {
		return m.SubjectsFn(ctx, categoryID)
	}
	return []psacampaign.SubjectRef{}, time.Time{}, nil
}
```

Run: `go build ./...`
Expected: builds clean (this step adds no new test — `CatalogStoreMock` is exercised by the consuming task's tests; a compile-clean build with `var _ psacampaign.CatalogStore = (*CatalogStoreMock)(nil)` is this step's verification).

- [ ] **Step 14: Commit the wiring and mock**

```bash
git add cmd/slabledger/server.go cmd/slabledger/handlers.go internal/testutil/mocks/psa_campaign_stores.go
git commit -m "feat(psacampaign): wire PSACatalogStore into cmd/slabledger and add its mock"
```

### Task 6: Read path — stop losing fields

**Files:**
- Modify: `internal/domain/psacampaign/types.go:6-20` (`PortalCampaign` new fields)
- Modify: `internal/adapters/clients/psaportal/campaigns_decode.go` (new `asStrings`, `decodeSpecLists` helpers; `decodeFormData` gains one field)
- Modify: `internal/adapters/clients/psaportal/campaigns.go:14-38` (`FetchCampaigns`), `:49-69` (`fetchCampaignFormData`), `:150-160` (`applyFormData`)
- Test: `internal/adapters/clients/psaportal/campaigns_test.go` (three existing `FetchCampaigns` call sites widened to the new return arity; two new tests appended)

**Interfaces:**
- Consumes: `psacampaign.SpecListRef{ID, Name, Status string}` (frozen contract), `psacampaign.SubjectRef{ID int; Name string}` (existing), `psacampaign.CampaignFormData.PrepackagedSpecListIDs []string` (existing field, `types.go:52`), `psacampaign.CampaignFormData.DeniedSpecs []SubjectRef` (existing field, `types.go:69`)
- Produces:
  ```go
  func (c *Client) FetchCampaigns(ctx context.Context) ([]psacampaign.PortalCampaign, []psacampaign.SpecListRef, error)
  func (c *Client) fetchCampaignFormData(ctx context.Context, campaignID string) (psacampaign.CampaignFormData, []psacampaign.SpecListRef, error)
  func asStrings(v any) []string
  ```
  Signature choice for `FetchCampaigns`: a second return value (`[]psacampaign.SpecListRef`), not a wrapper struct. Every edit-form response embeds the full curated-list catalog as a sibling of `formData` (it is portal-wide reference data, not per-campaign), so the loop already has it in hand each time it fetches a campaign's edit form — returning it costs nothing extra and avoids a second portal round trip. A later task (owned by the harvester) reads this return value and calls `psacampaign.CatalogStore.SaveSpecLists` with it; `psaportal` itself never touches storage, matching the process boundary (portal I/O is harvester-only). `FetchCampaigns` keeps the catalog from the *last* edit-form fetch that succeeded — the catalog does not vary per campaign, so any successful fetch is as good as any other, and `nil` is returned when every edit-form fetch failed, letting the harvester decide whether an empty catalog blocks its baseline (that decision belongs to a later task, not this one).

- [ ] **Step 1: Write the failing tests**

Append to `internal/adapters/clients/psaportal/campaigns_test.go` (add `"fmt"` and `"reflect"` to the existing import block, which already has `"context"`, `"encoding/json"`, `"os"`, `"strings"`, `"testing"`):

```go
func TestFetchCampaignFormData_DecodesSpecListCatalog(t *testing.T) {
	editRoot := map[string]any{
		"formData": map[string]any{
			"prepackagedSpecListIds": []any{"list-en-base", "list-en-holo"},
		},
		"prepackagedSpecLists": []any{
			map[string]any{"id": "list-en-base", "name": "English Base Set", "status": "ENABLED"},
			map[string]any{"id": "list-en-holo", "name": "English Holo Rares", "status": "ENABLED"},
			map[string]any{"id": "list-jp-base", "name": "Japanese Base Set", "status": "DISABLED"},
		},
	}
	packed, err := EncodeRefPacked(editRoot)
	if err != nil {
		t.Fatalf("EncodeRefPacked: %v", err)
	}
	env := map[string]any{
		"type": "data",
		"nodes": []any{
			map[string]any{"type": "data", "data": packed},
		},
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	ff := &fakeFetcher{routes: map[string]string{
		fmt.Sprintf(campaignEditPathF, "camp-1"): string(body),
	}}

	c := New(ff, Config{})
	fd, specLists, err := c.fetchCampaignFormData(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("fetchCampaignFormData: %v", err)
	}
	if want := []string{"list-en-base", "list-en-holo"}; !reflect.DeepEqual(fd.PrepackagedSpecListIDs, want) {
		t.Errorf("PrepackagedSpecListIDs = %v, want %v", fd.PrepackagedSpecListIDs, want)
	}
	if len(specLists) != 3 {
		t.Fatalf("expected 3 catalog entries, got %d: %+v", len(specLists), specLists)
	}
	if specLists[2].ID != "list-jp-base" || specLists[2].Status != "DISABLED" {
		t.Errorf("specLists[2] = %+v, want {list-jp-base ... DISABLED}", specLists[2])
	}
}

func TestFetchCampaigns_EditFetchFailure_TargetingIncomplete(t *testing.T) {
	page := buildListEnvelope(t, []any{campaignItem("id-1", "Flaky")}, 1, 1)
	editURL := fmt.Sprintf(campaignEditPathF, "id-1")
	ff := &fakeFetcher{
		routes: map[string]string{
			campaignsListPath: string(page),
			editURL:           "",
		},
		statusFor: map[string]int{
			editURL: 403,
		},
	}

	c := New(ff, Config{})
	got, catalog, err := c.FetchCampaigns(context.Background())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 campaign despite edit-form failure, got %d", len(got))
	}
	if got[0].TargetingComplete {
		t.Error("TargetingComplete = true, want false after edit-form fetch failure")
	}
	if len(got[0].SpecListIDs) != 0 {
		t.Errorf("SpecListIDs = %v, want empty when targeting is incomplete", got[0].SpecListIDs)
	}
	if catalog != nil {
		t.Errorf("catalog = %+v, want nil when no edit-form fetch succeeded", catalog)
	}
}
```

Also widen the three existing `FetchCampaigns` call sites in the same file to the new return arity (no other change to those tests):
- `campaigns_test.go:26`: `got, err := c.FetchCampaigns(context.Background())` → `got, _, err := c.FetchCampaigns(context.Background())`
- `campaigns_test.go:137`: `got, err := c.FetchCampaigns(context.Background())` → `got, _, err := c.FetchCampaigns(context.Background())`
- `campaigns_test.go:179`: `_, err := c.FetchCampaigns(context.Background())` → `_, _, err := c.FetchCampaigns(context.Background())`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/clients/psaportal/... -run 'TestFetchCampaignFormData_DecodesSpecListCatalog|TestFetchCampaigns_EditFetchFailure_TargetingIncomplete' -v`
Expected: FAIL to compile — `c.fetchCampaignFormData(...)` returns 2 values not 3, `c.FetchCampaigns(...)` returns 2 values not 3, `psacampaign.SpecListRef` and `PortalCampaign.TargetingComplete`/`SpecListIDs` undefined.

- [ ] **Step 3: Write the implementation**

`internal/domain/psacampaign/types.go` — widen `PortalCampaign`:

```go
// PortalCampaign is the parsed offer-program config for one PSA campaign.
type PortalCampaign struct {
	CampaignRequestID string         `json:"campaignRequestId"`
	Name              string         `json:"name"`
	Type              string         `json:"type"`     // e.g. "CATEGORY"
	Status            string         `json:"status"`   // e.g. "PAUSED"
	Category          string         `json:"category"` // e.g. "POKEMON"
	BuyPercentClv     int            `json:"buyPercentClv"`
	BuyBox            CampaignBuyBox `json:"buyBox"`
	DailyBudgetCents  int            `json:"dailyBudgetCents"`
	DailySpecLimit    int            `json:"dailySpecLimit"`
	SubjectFilter     CampaignFilter `json:"subjectFilter"`
	PublisherFilter   CampaignFilter `json:"publisherFilter"`
	SpecListIDs       []string       `json:"specListIds"`
	SpecListNames     []string       `json:"specListNames"`
	DeniedSpecs       []SubjectRef   `json:"deniedSpecs"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`

	// TargetingComplete is false when the edit-form fetch for this campaign
	// failed, meaning the targeting fields above are zero values rather than
	// portal truth. The baseline pull refuses to write a campaign row from an
	// incomplete record.
	TargetingComplete bool `json:"targetingComplete"`
}
```

`internal/adapters/clients/psaportal/campaigns_decode.go` — add `asStrings`, add `decodeSpecLists`, and add the one missing field to `decodeFormData`:

```go
// asStrings converts a decoded []any of string values into []string,
// skipping any element that is not a string.
func asStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// decodeSpecLists converts the edit root's prepackagedSpecLists sibling of
// formData into the curated spec-list catalog. This is the field
// fetchCampaignFormData used to discard entirely.
func decodeSpecLists(v any) []psacampaign.SpecListRef {
	arr, _ := v.([]any)
	out := make([]psacampaign.SpecListRef, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, psacampaign.SpecListRef{
			ID:     asString(m["id"]),
			Name:   asString(m["name"]),
			Status: asString(m["status"]),
		})
	}
	return out
}

// decodeFormData maps a decoded formData object into CampaignFormData.
func decodeFormData(fd map[string]any) (psacampaign.CampaignFormData, error) {
	return psacampaign.CampaignFormData{
		CampaignName:                asString(fd["campaignName"]),
		CampaignType:                asString(fd["campaignType"]),
		Category:                    asString(fd["category"]),
		PrepackagedSpecListIDs:      asStrings(fd["prepackagedSpecListIds"]),
		IsActive:                    boolVal(fd["isActive"]),
		BidPercentage:               asInt(fd["bidPercentage"]),
		FlatFee:                     asInt(fd["flatFee"]),
		DailyBudget:                 asInt(fd["dailyBudget"]),
		DailySpecLimit:              asInt(fd["dailySpecLimit"]),
		GradeMinimum:                asString(fd["gradeMinimum"]),
		GradeMaximum:                asString(fd["gradeMaximum"]),
		YearMinimum:                 asInt(fd["yearMinimum"]),
		YearMaximum:                 asInt(fd["yearMaximum"]),
		PriceMinimum:                asInt(fd["priceMinimum"]),
		PriceMaximum:                asInt(fd["priceMaximum"]),
		CardLadderConfidenceMinimum: asInt(fd["cardLadderConfidenceMinimum"]),
		PublisherFilterType:         asString(fd["publisherFilterType"]),
		SelectedPublishers:          asSubjectRefs(fd["selectedPublishers"]),
		SubjectFilterType:           asString(fd["subjectFilterType"]),
		SelectedSubjects:            asSubjectRefs(fd["selectedSubjects"]),
		DeniedSpecs:                 asSubjectRefs(fd["deniedSpecs"]),
	}, nil
}
```

`internal/adapters/clients/psaportal/campaigns.go` — widen `FetchCampaigns`, `fetchCampaignFormData`, and `applyFormData`:

```go
// FetchCampaigns returns all portal campaigns with buy-box + member lists,
// paginating the list endpoint and enriching each item with its edit-form
// subject/publisher/spec-list filters. It also returns the curated spec-list
// catalog embedded in the edit-form responses — the harvester persists it via
// psacampaign.CatalogStore without a second portal round trip.
func (c *Client) FetchCampaigns(ctx context.Context) ([]psacampaign.PortalCampaign, []psacampaign.SpecListRef, error) {
	var out []psacampaign.PortalCampaign
	var catalog []psacampaign.SpecListRef
	page := 1
	for {
		root, err := c.getRefPacked(ctx, fmt.Sprintf("%s%s&page=%d", c.baseURL(), campaignsListPath, page))
		if err != nil {
			return nil, nil, err
		}
		items, pageSize, totalCount, err := campaignItems(root)
		if err != nil {
			return nil, nil, err
		}
		for _, it := range items {
			pc, err := mapListItem(it)
			if err != nil {
				c.logger.Warn(ctx, "psaportal: skipping malformed campaign", observability.Err(err))
				continue
			}
			fd, specLists, err := c.fetchCampaignFormData(ctx, pc.CampaignRequestID)
			if err != nil {
				// TargetingComplete stays false: pc carries zero targeting,
				// and the baseline pull must refuse to write it into the
				// campaign row rather than overwrite real targeting with an
				// open net. The snapshot store's empty-save guard
				// (psa_campaign_snapshot_store.go:29-30) only checks
				// len(campaigns) == 0, so it does NOT catch this — a fleet
				// of zero-targeting campaigns would sail straight through it.
				c.logger.Warn(ctx, "psaportal: edit fetch failed",
					observability.String("campaign_id", pc.CampaignRequestID), observability.Err(err))
			} else {
				applyFormData(&pc, fd, specLists)
				if len(specLists) > 0 {
					catalog = specLists
				}
			}
			out = append(out, pc)
		}
		if len(items) == 0 || len(items) < pageSize || len(out) >= totalCount {
			break
		}
		page++
	}
	return out, catalog, nil
}

// fetchCampaignFormData fetches and decodes the edit-page formData for one
// campaign, along with the curated spec-list catalog embedded alongside it.
func (c *Client) fetchCampaignFormData(ctx context.Context, campaignID string) (psacampaign.CampaignFormData, []psacampaign.SpecListRef, error) {
	url := c.baseURL() + fmt.Sprintf(campaignEditPathF, campaignID)
	root, err := c.getRefPacked(ctx, url)
	if err != nil {
		return psacampaign.CampaignFormData{}, nil, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return psacampaign.CampaignFormData{}, nil, fmt.Errorf("psaportal: edit root not an object")
	}
	fdRaw, ok := m["formData"]
	if !ok {
		return psacampaign.CampaignFormData{}, nil, fmt.Errorf("psaportal: edit response missing formData")
	}
	fd, ok := fdRaw.(map[string]any)
	if !ok {
		return psacampaign.CampaignFormData{}, nil, fmt.Errorf("psaportal: formData not an object")
	}
	formData, err := decodeFormData(fd)
	if err != nil {
		return psacampaign.CampaignFormData{}, nil, err
	}
	return formData, decodeSpecLists(m["prepackagedSpecLists"]), nil
}

// applyFormData fills the subject/publisher/spec-list filters on pc from the
// edit-form data and the sibling spec-list catalog, then marks the record
// complete. SpecListNames resolves SpecListIDs against catalog at decode time
// so the snapshot stays human-readable across a PSA re-key of the UUIDs.
func applyFormData(pc *psacampaign.PortalCampaign, fd psacampaign.CampaignFormData, catalog []psacampaign.SpecListRef) {
	pc.SubjectFilter = psacampaign.CampaignFilter{
		Type:     fd.SubjectFilterType,
		Subjects: fd.SelectedSubjects,
	}
	pc.PublisherFilter = psacampaign.CampaignFilter{
		Type:     fd.PublisherFilterType,
		Subjects: fd.SelectedPublishers,
	}
	pc.SpecListIDs = fd.PrepackagedSpecListIDs
	pc.SpecListNames = specListNames(fd.PrepackagedSpecListIDs, catalog)
	pc.DeniedSpecs = fd.DeniedSpecs
	pc.TargetingComplete = true
}

// specListNames resolves each id against catalog, skipping ids the catalog
// does not (yet) explain rather than failing the whole decode.
func specListNames(ids []string, catalog []psacampaign.SpecListRef) []string {
	byID := make(map[string]string, len(catalog))
	for _, ref := range catalog {
		byID[ref.ID] = ref.Name
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := byID[id]; ok {
			out = append(out, name)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/adapters/clients/psaportal/... -v`
Expected: PASS, including the pre-existing `TestFetchCampaigns_ParsesListAndEdit`, `TestFetchCampaigns_MultiPage`, and `TestFetchCampaigns_InvalidPageSize`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/psacampaign/types.go internal/adapters/clients/psaportal/campaigns.go internal/adapters/clients/psaportal/campaigns_decode.go internal/adapters/clients/psaportal/campaigns_test.go
git commit -m "feat(psaportal): stop discarding spec-list targeting on the read path"
```

### Task 7: psaportal.FetchSubjects

**Files:**
- Create: `internal/adapters/clients/psaportal/subjects.go`
- Test: `internal/adapters/clients/psaportal/subjects_test.go`

**Interfaces:**
- Consumes: `psacampaign.SubjectRef{ID int; Name string}` (existing, `types.go:41-44`); `(c *Client) fetchRemoteHash(ctx, fn string) (string, error)` (`buildhash.go:37`); `EncodeRefPacked(v any) ([]json.RawMessage, error)` and `DecodeRefPacked(data []json.RawMessage) (any, error)` (`svelteref.go:10,78`); `asSubjectRefs(v any) []psacampaign.SubjectRef` (`campaigns_decode.go:41`); `truncateBody(s string) string` (`client.go:120`); `c.fetch.Do(ctx, FetchRequest{...}) (FetchResponse, error)`.
- Produces: `func (c *Client) FetchSubjects(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, error)` — consumed by Task 10 (the harvester's `-baseline-pull` / regular-run subject persistence via `CatalogStore.SaveSubjects`, Task 5's interface).

- [ ] **Step 1: Write the failing test**

```go
package psaportal

import (
	"context"
	"testing"
)

func TestFetchSubjects_PostsCategoryIDAndDecodesResult(t *testing.T) {
	routes := bundleRoutes()
	routes["immutable/chunks/REMOTE.js"] = `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign"),z=_t("abc123/getSubjects")`
	routes["/buyercampaignmanager/_app/remote/abc123/getSubjects"] = `{"type":"result","result":"[[2,3],{\"id\":22210,\"name\":\"Machamp\"},{\"id\":22301,\"name\":\"Charizard\"}]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	got, err := c.FetchSubjects(context.Background(), 16)
	if err != nil {
		t.Fatalf("FetchSubjects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != 22210 || got[0].Name != "Machamp" {
		t.Errorf("got[0] = %+v, want {22210 Machamp}", got[0])
	}
	if got[1].ID != 22301 || got[1].Name != "Charizard" {
		t.Errorf("got[1] = %+v, want {22301 Charizard}", got[1])
	}

	payloadStr := extractPayload(t, ff.captured["/buyercampaignmanager/_app/remote/abc123/getSubjects"])
	decoded, err := base64Decode(t, payloadStr)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	if decoded != "[16]" {
		t.Errorf("decoded ref-packed root = %q, want the flat array %q", decoded, "[16]")
	}
}

func TestFetchSubjects_NonResultEnvelope(t *testing.T) {
	routes := bundleRoutes()
	routes["immutable/chunks/REMOTE.js"] = `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign"),z=_t("abc123/getSubjects")`
	routes["/buyercampaignmanager/_app/remote/abc123/getSubjects"] = `{"type":"error","message":"nope"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	if _, err := c.FetchSubjects(context.Background(), 16); err == nil {
		t.Fatal("expected error for non-result envelope")
	}
}
```

```go
// base64Decode decodes a ref-packed payload string and renders slot 0 back to
// a compact JSON string for a direct literal comparison in the test above.
// getSubjects takes a bare one-element array argument (no object wrapper),
// unlike createCampaign's bare-object argument — this pins that shape.
func base64Decode(t *testing.T, payloadStr string) (string, error) {
	t.Helper()
	raw, err := stdBase64Decode(payloadStr)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
```

Note: `stdBase64Decode` is `encoding/base64.StdEncoding.DecodeString`, aliased only so the helper reads as one line; the real test file imports `encoding/base64` directly and calls it inline rather than through a wrapper — see Step 3's finished form in the test file below.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/psaportal/... -run TestFetchSubjects -v`
Expected: FAIL with `./subjects_test.go:1:1: undefined: c.FetchSubjects` (compile error — `FetchSubjects` does not exist yet)

- [ ] **Step 3: Write the implementation**

```go
package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// FetchSubjects calls the portal's getSubjects remote function for categoryID
// (16 = POKEMON) and returns every subject the category offers — 492 for
// Pokemon, confirmed live 2026-08-06. It runs only in the harvester: the
// result is persisted via psacampaign.CatalogStore.SaveSubjects so the main
// HTTP server, which has no portal session, can resolve subject names without
// ever calling the portal itself.
//
// The returned ids are all current-generation (22xxx). Live campaigns hold a
// mix of 4xxx/8xxx/22xxx subject ids from older PSA subject generations, so
// this catalog is for resolving NEW subjects an operator adds by name — it
// must never be used to rewrite an id already pulled from a live campaign.
func (c *Client) FetchSubjects(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, error) {
	remoteHash, err := c.fetchRemoteHash(ctx, "getSubjects")
	if err != nil {
		return nil, err
	}

	// getSubjects takes one positional argument, the category id, packed as a
	// bare one-element array — `getSubjects([16])` — not the object shape
	// createCampaign/updateCampaign use.
	packed, err := EncodeRefPacked([]any{float64(categoryID)})
	if err != nil {
		return nil, fmt.Errorf("psaportal: encode getSubjects payload: %w", err)
	}
	arrJSON, err := json.Marshal(packed)
	if err != nil {
		return nil, fmt.Errorf("psaportal: marshal getSubjects payload: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"payload":   base64.StdEncoding.EncodeToString(arrJSON),
		"refreshes": []any{},
	})
	if err != nil {
		return nil, fmt.Errorf("psaportal: marshal getSubjects request: %w", err)
	}

	url := fmt.Sprintf("%s/buyercampaignmanager/_app/remote/%s/getSubjects", c.baseURL(), remoteHash)
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: url, Method: "POST", Body: string(body)})
	if err != nil {
		return nil, fmt.Errorf("psaportal: get subjects: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("psaportal: get subjects status %d", resp.Status)
	}

	var envelope struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &envelope); err != nil {
		return nil, fmt.Errorf("psaportal: decode getSubjects response: %w", err)
	}
	if envelope.Type != "result" {
		return nil, fmt.Errorf("psaportal: getSubjects response type %q, want \"result\": %s", envelope.Type, truncateBody(resp.Body))
	}

	var resultPacked []json.RawMessage
	if err := json.Unmarshal([]byte(envelope.Result), &resultPacked); err != nil {
		return nil, fmt.Errorf("psaportal: getSubjects result undecodable: %w", err)
	}
	root, err := DecodeRefPacked(resultPacked)
	if err != nil {
		return nil, fmt.Errorf("psaportal: getSubjects result undecodable: %w", err)
	}
	return asSubjectRefs(root), nil
}
```

Rewrite the test file's helper without the artificial `base64Decode`/`stdBase64Decode` split (that indirection existed only to show the intent above); the real `subjects_test.go` imports `encoding/base64` and decodes inline:

```go
package psaportal

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestFetchSubjects_PostsCategoryIDAndDecodesResult(t *testing.T) {
	routes := bundleRoutes()
	routes["immutable/chunks/REMOTE.js"] = `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign"),z=_t("abc123/getSubjects")`
	routes["/buyercampaignmanager/_app/remote/abc123/getSubjects"] = `{"type":"result","result":"[[2,3],{\"id\":22210,\"name\":\"Machamp\"},{\"id\":22301,\"name\":\"Charizard\"}]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	got, err := c.FetchSubjects(context.Background(), 16)
	if err != nil {
		t.Fatalf("FetchSubjects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != 22210 || got[0].Name != "Machamp" {
		t.Errorf("got[0] = %+v, want {22210 Machamp}", got[0])
	}
	if got[1].ID != 22301 || got[1].Name != "Charizard" {
		t.Errorf("got[1] = %+v, want {22301 Charizard}", got[1])
	}

	payloadStr := extractPayload(t, ff.captured["/buyercampaignmanager/_app/remote/abc123/getSubjects"])
	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	if string(decoded) != "[16]" {
		t.Errorf("decoded ref-packed root = %s, want the flat array [16]", decoded)
	}
}

func TestFetchSubjects_NonResultEnvelope(t *testing.T) {
	routes := bundleRoutes()
	routes["immutable/chunks/REMOTE.js"] = `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign"),z=_t("abc123/getSubjects")`
	routes["/buyercampaignmanager/_app/remote/abc123/getSubjects"] = `{"type":"error","message":"nope"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	if _, err := c.FetchSubjects(context.Background(), 16); err == nil {
		t.Fatal("expected error for non-result envelope")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/adapters/clients/psaportal/... -run TestFetchSubjects -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/psaportal/subjects.go internal/adapters/clients/psaportal/subjects_test.go
git commit -m "feat(psaportal): add FetchSubjects for the getSubjects remote function"
```

---

### Task 8: Translators take a Resolver

**Files:**
- Modify: `internal/domain/psacampaign/mapper.go` (whole file — signatures, constants, both translators)
- Modify: `internal/domain/psacampaign/types.go:91-96` (`FieldChange` gains `Value any`)
- Modify: `internal/adapters/clients/psaportal/push.go:55-68`
- Create: `internal/testutil/mocks/psa_resolver.go` (`ResolverMock`)
- Modify: `internal/domain/psacampaign/mapper_test.go` (existing tests updated for the new signatures; new list-axis cases added)
- Modify: `internal/adapters/clients/psaportal/push_test.go` (new list-valued-field case)

**Interfaces:**
- Consumes: `psacampaign.Resolver` (Task 5, `resolver.go`): `SpecListIDs(languageToken string) ([]string, error)`, `SubjectID(name string) (int, error)`; `psacampaign.ErrUnknownSpecList`, `psacampaign.ErrUnknownSubject` (Task 5, `resolver.go`); `inventory.Campaign.TargetLanguage/SubjectFilterMode/Subjects/DeniedSpecs` and `inventory.TargetSubject{ID int; Name string}` (Task 1, `types_core.go`).
- Produces: `func TranslateToCreate(internal inventory.Campaign, r Resolver) (CampaignFormData, error)` and `func TranslateToDiff(internal inventory.Campaign, portal PortalCampaign, r Resolver) (ProposedDiff, error)` — consumed by Task 9 (`campaigns_psa.go:137,264`). `FieldChange.Value any` (`json:"value,omitempty"`) — consumed by Task 9 and by push.go's own mutation loop in this task. `mocks.ResolverMock` — consumed by Task 9's handler tests and any later test needing a stub `Resolver`.

**Judgment call (recorded here since it isn't in the frozen contract):** `TranslateToDiff` skips the `prepackagedSpecListIds` comparison entirely when `internal.TargetLanguage == ""` rather than calling the resolver and failing the whole diff — a legacy/unlinked campaign with no language set yet has nothing to propose on that axis, and erroring out would block every *other* scalar fix (bid, budget, grade bounds) a user might want to push for such a campaign. `TranslateToCreate`, by contrast, hard-fails on empty `TargetLanguage` per the design's explicit error-handling table (§4/"Error handling").

- [ ] **Step 1: Write the failing test**

```go
package psacampaign

import (
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// stubResolver is a minimal local Resolver for the tests in this file that
// need one; internal/testutil/mocks.ResolverMock is used by other packages
// (e.g. the httpserver handlers in Task 9) that cannot import psacampaign's
// unexported test helpers. Both implement the same psacampaign.Resolver.
type stubResolver struct {
	specLists map[string][]string
	subjects  map[string]int
}

func (s stubResolver) SpecListIDs(languageToken string) ([]string, error) {
	ids, ok := s.specLists[languageToken]
	if !ok {
		return nil, ErrUnknownSpecList
	}
	return ids, nil
}

func (s stubResolver) SubjectID(name string) (int, error) {
	id, ok := s.subjects[name]
	if !ok {
		return 0, ErrUnknownSubject
	}
	return id, nil
}

func baseCreateCampaign() inventory.Campaign {
	return inventory.Campaign{
		Name: "Modern 10s", BuyTermsCLPct: 0.72, DailySpendCapCents: 300000,
		GradeRange: "10", YearRange: "2024-2026", PriceRange: "500-3000",
		CLConfidence: "3-4", PSASourcingFeeCents: 300,
		TargetLanguage: "english", SubjectFilterMode: "Target",
	}
}

func englishResolver() stubResolver {
	return stubResolver{
		specLists: map[string][]string{"english": {"list-en-1"}},
		subjects:  map[string]int{"Pikachu": 90001},
	}
}

func TestTranslateToCreate_SpecListAndSubjects(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *inventory.Campaign)
		r       Resolver
		wantErr string
		check   func(t *testing.T, fd CampaignFormData)
	}{
		{
			name:   "spec-list type and resolved subject",
			mutate: func(c *inventory.Campaign) { c.Subjects = []inventory.TargetSubject{{Name: "Pikachu"}} },
			r:      englishResolver(),
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.CampaignType != "SPEC_LIST" || fd.Category != "" {
					t.Fatalf("CampaignType/Category = %q/%q, want SPEC_LIST/\"\"", fd.CampaignType, fd.Category)
				}
				if len(fd.PrepackagedSpecListIDs) != 1 || fd.PrepackagedSpecListIDs[0] != "list-en-1" {
					t.Fatalf("PrepackagedSpecListIDs = %+v, want [list-en-1]", fd.PrepackagedSpecListIDs)
				}
				if len(fd.SelectedSubjects) != 1 || fd.SelectedSubjects[0] != (SubjectRef{ID: 90001, Name: "Pikachu"}) {
					t.Fatalf("SelectedSubjects = %+v, want [{90001 Pikachu}]", fd.SelectedSubjects)
				}
			},
		},
		{
			name:   "portal-sourced subject id passes through verbatim",
			mutate: func(c *inventory.Campaign) { c.Subjects = []inventory.TargetSubject{{ID: 4807, Name: "Charizard"}} },
			r:      englishResolver(),
			check: func(t *testing.T, fd CampaignFormData) {
				if len(fd.SelectedSubjects) != 1 || fd.SelectedSubjects[0] != (SubjectRef{ID: 4807, Name: "Charizard"}) {
					t.Fatalf("SelectedSubjects = %+v, want [{4807 Charizard}] unresolved", fd.SelectedSubjects)
				}
			},
		},
		{
			name:    "empty target language fails",
			mutate:  func(c *inventory.Campaign) { c.TargetLanguage = "" },
			r:       englishResolver(),
			wantErr: "target language",
		},
		{
			name:    "unmapped language token fails",
			mutate:  func(c *inventory.Campaign) { c.TargetLanguage = "korean" },
			r:       englishResolver(),
			wantErr: "resolve spec list",
		},
		{
			name:    "unresolvable subject name fails naming the subject",
			mutate:  func(c *inventory.Campaign) { c.Subjects = []inventory.TargetSubject{{Name: "Mewtwo"}} },
			r:       englishResolver(),
			wantErr: "Mewtwo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := baseCreateCampaign()
			tt.mutate(&c)
			fd, err := TranslateToCreate(c, tt.r)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranslateToCreate: %v", err)
			}
			tt.check(t, fd)
		})
	}
}

func TestTranslateToDiff_ListAxes(t *testing.T) {
	r := englishResolver()
	internal := inventory.Campaign{
		BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
		GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
		TargetLanguage: "english", SubjectFilterMode: "Exclude",
		Subjects:    []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}, {ID: 4807, Name: "Charizard"}},
		DeniedSpecs: []inventory.TargetSubject{{ID: 22301, Name: "Charizard EX"}},
	}
	portal := PortalCampaign{
		BuyPercentClv: 75, DailyBudgetCents: 400000,
		BuyBox: CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
		SubjectFilter: CampaignFilter{
			Type: "Target",
			// Same two subjects, REVERSED order plus one difference: this
			// asserts order-insensitivity would suppress a spurious change
			// while the actual polarity change still fires.
			Subjects: []SubjectRef{{ID: 4807, Name: "Charizard"}, {ID: 22210, Name: "Machamp"}},
		},
		SpecListIDs: []string{"list-en-1"},
		DeniedSpecs: []SubjectRef{{ID: 22301, Name: "Charizard EX"}},
	}

	diff, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff: %v", err)
	}
	got := map[string]FieldChange{}
	for _, c := range diff.Changes {
		got[c.Field] = c
	}

	if _, ok := got["selectedSubjects"]; ok {
		t.Errorf("selectedSubjects should NOT appear: identical subject sets in different order must not diff")
	}
	if c, ok := got["subjectFilterType"]; !ok || c.Old != "Target" || c.New != "Exclude" {
		t.Errorf("subjectFilterType = %+v, want Target -> Exclude", c)
	}
	if _, ok := got["deniedSpecs"]; ok {
		t.Errorf("deniedSpecs should NOT appear: identical denied lists must not diff")
	}
	if _, ok := got["prepackagedSpecListIds"]; ok {
		t.Errorf("prepackagedSpecListIds should NOT appear: identical single-entry lists must not diff")
	}

	// Now change one subject's name and assert the diff DOES fire and carries
	// a typed Value (not just a rendered string), for push.go to consume.
	internal.Subjects = []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}, {ID: 90001, Name: "Pikachu"}}
	diff2, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff (2): %v", err)
	}
	var found *FieldChange
	for i := range diff2.Changes {
		if diff2.Changes[i].Field == "selectedSubjects" {
			found = &diff2.Changes[i]
		}
	}
	if found == nil {
		t.Fatal("expected a selectedSubjects change after swapping Charizard for Pikachu")
	}
	refs, ok := found.Value.([]SubjectRef)
	if !ok || len(refs) != 2 {
		t.Fatalf("Value = %#v, want []SubjectRef of length 2", found.Value)
	}
}

func TestTranslateToDiff_EmptyTargetLanguageSkipsSpecListAxis(t *testing.T) {
	r := englishResolver()
	internal := inventory.Campaign{
		BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
		GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
		TargetLanguage: "", SubjectFilterMode: "Target",
	}
	portal := PortalCampaign{
		BuyPercentClv: 75, DailyBudgetCents: 400000,
		BuyBox: CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
		SubjectFilter: CampaignFilter{Type: "Target"},
		SpecListIDs:   []string{"stale-list"},
	}
	diff, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff: %v (empty TargetLanguage must not error the whole diff)", err)
	}
	for _, c := range diff.Changes {
		if c.Field == "prepackagedSpecListIds" {
			t.Fatalf("did not expect a prepackagedSpecListIds change with an unset TargetLanguage: %+v", c)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/psacampaign/... -run 'TestTranslateToCreate_SpecListAndSubjects|TestTranslateToDiff_ListAxes|TestTranslateToDiff_EmptyTargetLanguageSkipsSpecListAxis' -v`
Expected: FAIL to compile — `too many arguments in call to TranslateToCreate`, `undefined: PortalCampaign.SpecListIDs` (until Task 2 lands; see Step 4 note), `undefined: FieldChange.Value` (until this task's Step 3 also touches `types.go`)

- [ ] **Step 3: Write the implementation**

`types.go:91-96` — `FieldChange` gains `Value`:

```go
// FieldChange is one proposed field mutation (old -> new), for audit + UI diff.
type FieldChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
	// Value carries the new value for list-valued fields, where the string
	// rendering in New is for display and audit only. Scalar fields leave
	// this nil and push.go falls back to New.
	Value any `json:"value,omitempty"`
}
```

`mapper.go` — full rewrite:

```go
package psacampaign

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TranslateToDiff compares an internal campaign against the current portal
// campaign and returns the field changes needed to make the portal match
// internal, across both the scalar buy-box fields and the three targeting
// axes (spec list, subject filter, denied specs).
func TranslateToDiff(internal inventory.Campaign, portal PortalCampaign, r Resolver) (ProposedDiff, error) {
	var d ProposedDiff
	add := func(field, old, newv string) {
		if old != newv {
			d.Changes = append(d.Changes, FieldChange{Field: field, Old: old, New: newv})
		}
	}
	// addList is add's list-valued sibling: field, old, its canonical
	// (order-insensitive, sorted) rendering, new's rendering, and the typed
	// new value push.go sends to the portal instead of a string.
	addList := func(field, oldRendering, newRendering string, value any) {
		if oldRendering != newRendering {
			d.Changes = append(d.Changes, FieldChange{Field: field, Old: oldRendering, New: newRendering, Value: value})
		}
	}

	newBid := strconv.Itoa(int(internal.BuyTermsCLPct*100 + 0.5))
	add("bidPercentage", strconv.Itoa(portal.BuyPercentClv), newBid)

	add("dailyBudget", strconv.Itoa(portal.DailyBudgetCents/100),
		strconv.Itoa(internal.DailySpendCapCents/100))

	gMin, gMax, err := splitRange(internal.GradeRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: grade range: %w", err)
	}
	add("gradeMinimum", portal.BuyBox.GradeMin, gMin)
	add("gradeMaximum", portal.BuyBox.GradeMax, gMax)

	yMin, yMax, err := splitRange(internal.YearRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: year range: %w", err)
	}
	add("yearMinimum", strconv.Itoa(portal.BuyBox.YearMin), yMin)
	add("yearMaximum", strconv.Itoa(portal.BuyBox.YearMax), yMax)

	pMin, pMax, err := splitRange(internal.PriceRange)
	if err != nil {
		return d, fmt.Errorf("psacampaign: price range: %w", err)
	}
	add("priceMinimum", strconv.Itoa(portal.BuyBox.PriceMinCents/100), pMin)
	add("priceMaximum", strconv.Itoa(portal.BuyBox.PriceMaxCents/100), pMax)

	if cMin, _, err := splitRange(internal.CLConfidence); err == nil {
		add("cardLadderConfidenceMinimum", strconv.Itoa(portal.BuyBox.ClvConfidenceMin), cMin)
	}

	add("subjectFilterType", portal.SubjectFilter.Type, internal.SubjectFilterMode)

	selectedSubjects, err := toSubjectRefs(internal.Subjects, r)
	if err != nil {
		return d, err
	}
	addList("selectedSubjects",
		renderSubjectRefs(portal.SubjectFilter.Subjects), renderSubjectRefs(selectedSubjects), selectedSubjects)

	deniedSpecs, err := toSubjectRefs(internal.DeniedSpecs, r)
	if err != nil {
		return d, err
	}
	addList("deniedSpecs",
		renderSubjectRefs(portal.DeniedSpecs), renderSubjectRefs(deniedSpecs), deniedSpecs)

	// An unset TargetLanguage means this campaign has no spec-list axis to
	// propose yet (legacy/unlinked campaign) — that must not block every
	// other scalar fix in this diff, so the axis is skipped rather than
	// erroring the whole call.
	if internal.TargetLanguage != "" {
		specListIDs, err := r.SpecListIDs(internal.TargetLanguage)
		if err != nil {
			return d, fmt.Errorf("psacampaign: resolve spec list for language %q: %w", internal.TargetLanguage, err)
		}
		addList("prepackagedSpecListIds",
			renderStringList(portal.SpecListIDs), renderStringList(specListIDs), specListIDs)
	}

	return d, nil
}

// renderSubjectRefs renders subject refs as a canonical, order-insensitive
// string: sorted by ID ascending, "id:name" pairs comma-joined. Two lists
// holding the same subjects in a different order render identically, so an
// unordered portal response never produces a spurious diff.
func renderSubjectRefs(refs []SubjectRef) string {
	sorted := append([]SubjectRef(nil), refs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	parts := make([]string, len(sorted))
	for i, ref := range sorted {
		parts[i] = fmt.Sprintf("%d:%s", ref.ID, ref.Name)
	}
	return strings.Join(parts, ",")
}

// renderStringList renders a string list canonically (sorted, comma-joined)
// for the same order-insensitivity reason as renderSubjectRefs.
func renderStringList(ss []string) string {
	sorted := append([]string(nil), ss...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// toSubjectRefs converts internal.TargetSubject entries to the portal's
// SubjectRef wire shape. An entry with a non-zero ID is portal-sourced and
// passes through verbatim — it is never re-resolved by name, because live
// portal ids span multiple id generations (4xxx/8xxx/22xxx) that getSubjects
// cannot reproduce. Only ID == 0 entries (operator-entered names never yet
// reconciled with the portal) are resolved via r; a resolution failure
// returns an error naming the subject rather than silently dropping it from
// what the campaign buys.
func toSubjectRefs(subjects []inventory.TargetSubject, r Resolver) ([]SubjectRef, error) {
	out := make([]SubjectRef, 0, len(subjects))
	for _, s := range subjects {
		id := s.ID
		if id == 0 {
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

// splitRange parses "a-b" (or a single "a") into its two ends as trimmed strings.
func splitRange(s string) (lo, hi string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("empty range")
	}
	parts := strings.SplitN(s, "-", 2)
	lo = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return lo, lo, nil
	}
	return lo, strings.TrimSpace(parts[1]), nil
}

// createDailySpecLimit is the portal-side daily spec cap SlabLedger always
// creates with; the internal campaign carries no equivalent field (v1).
const createDailySpecLimit = 2

// TranslateToCreate builds the full createCampaign formData for an internal
// campaign. The portal campaign is always created paused (IsActive false);
// money fields are whole USD on the wire (internal cents / 100). Campaigns
// are created as SPEC_LIST (the CATEGORY/POKEMON shape PSA has retired) with
// the spec list resolved from the campaign's TargetLanguage, and the subject
// filter carries the campaign's Subjects/DeniedSpecs.
func TranslateToCreate(internal inventory.Campaign, r Resolver) (CampaignFormData, error) {
	var fd CampaignFormData

	if internal.TargetLanguage == "" {
		return fd, fmt.Errorf("psacampaign: campaign has no target language set")
	}
	specListIDs, err := r.SpecListIDs(internal.TargetLanguage)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: resolve spec list for language %q: %w", internal.TargetLanguage, err)
	}

	gMin, gMax, err := splitRange(internal.GradeRange)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: grade range: %w", err)
	}
	yMin, yMax, err := splitRangeInts(internal.YearRange)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: year range: %w", err)
	}
	pMin, pMax, err := splitRangeInts(internal.PriceRange)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: price range: %w", err)
	}
	clMinStr, _, err := splitRange(internal.CLConfidence)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: cl confidence: %w", err)
	}
	clF, err := strconv.ParseFloat(clMinStr, 64)
	if err != nil {
		return fd, fmt.Errorf("psacampaign: cl confidence: %w", err)
	}
	clMin := int(clF)

	selectedSubjects, err := toSubjectRefs(internal.Subjects, r)
	if err != nil {
		return fd, err
	}
	deniedSpecs, err := toSubjectRefs(internal.DeniedSpecs, r)
	if err != nil {
		return fd, err
	}

	return CampaignFormData{
		CampaignName:                internal.Name,
		CampaignType:                "SPEC_LIST",
		Category:                    "",
		PrepackagedSpecListIDs:      specListIDs,
		IsActive:                    false,
		BidPercentage:               int(internal.BuyTermsCLPct*100 + 0.5),
		FlatFee:                     centsToWholeUSD(internal.PSASourcingFeeCents),
		DailyBudget:                 centsToWholeUSD(internal.DailySpendCapCents),
		DailySpecLimit:              createDailySpecLimit,
		GradeMinimum:                gMin,
		GradeMaximum:                gMax,
		YearMinimum:                 yMin,
		YearMaximum:                 yMax,
		PriceMinimum:                pMin,
		PriceMaximum:                pMax,
		CardLadderConfidenceMinimum: clMin,
		PublisherFilterType:         "Target",
		SelectedPublishers:          []SubjectRef{},
		SubjectFilterType:           internal.SubjectFilterMode,
		SelectedSubjects:            selectedSubjects,
		DeniedSpecs:                 deniedSpecs,
	}, nil
}

// centsToWholeUSD converts a cent value to whole USD for the portal wire,
// rounding to nearest dollar so sub-dollar remainders aren't silently dropped.
func centsToWholeUSD(cents int) int {
	return (cents + 50) / 100
}

// splitRangeInts parses "a-b" (or "a") into integer ends.
func splitRangeInts(s string) (lo, hi int, err error) {
	loS, hiS, err := splitRange(s)
	if err != nil {
		return 0, 0, err
	}
	if lo, err = strconv.Atoi(loS); err != nil {
		return 0, 0, fmt.Errorf("low bound %q: %w", loS, err)
	}
	if hi, err = strconv.Atoi(hiS); err != nil {
		return 0, 0, fmt.Errorf("high bound %q: %w", hiS, err)
	}
	return lo, hi, nil
}
```

`internal/testutil/mocks/psa_resolver.go` — new mock:

```go
package mocks

import "github.com/guarzo/slabledger/internal/domain/psacampaign"

// ResolverMock implements psacampaign.Resolver with the Fn-field pattern.
type ResolverMock struct {
	SpecListIDsFn func(languageToken string) ([]string, error)
	SubjectIDFn   func(name string) (int, error)
}

var _ psacampaign.Resolver = (*ResolverMock)(nil)

func (m *ResolverMock) SpecListIDs(languageToken string) ([]string, error) {
	if m.SpecListIDsFn != nil {
		return m.SpecListIDsFn(languageToken)
	}
	return nil, nil
}

func (m *ResolverMock) SubjectID(name string) (int, error) {
	if m.SubjectIDFn != nil {
		return m.SubjectIDFn(name)
	}
	return 0, nil
}
```

`push.go:36-68` — the mutation loop falls back to `ch.Value` for non-nil, non-numeric changes:

```go
for _, ch := range changes {
	if _, exists := formData[ch.Field]; !exists {
		return fmt.Errorf("psaportal: unknown campaign field %q", ch.Field)
	}
	switch {
	case numericFormDataFields[ch.Field]:
		n, err := strconv.ParseFloat(ch.New, 64)
		if err != nil {
			return fmt.Errorf("psaportal: field %q value %q is not numeric: %w", ch.Field, ch.New, err)
		}
		formData[ch.Field] = n
	case ch.Value != nil:
		// List-valued changes (selectedSubjects, deniedSpecs,
		// prepackagedSpecListIds) carry a typed Go value, not the display
		// string in ch.New. Round-trip it through JSON so EncodeRefPacked
		// sees the same plain map[string]any/[]any/scalar types the rest of
		// formData already holds (it came from DecodeRefPacked), exactly as
		// CreateCampaign round-trips its formData struct (create.go:24-34).
		valueJSON, err := json.Marshal(ch.Value)
		if err != nil {
			return fmt.Errorf("psaportal: marshal field %q value: %w", ch.Field, err)
		}
		var plain any
		if err := json.Unmarshal(valueJSON, &plain); err != nil {
			return fmt.Errorf("psaportal: remarshal field %q value: %w", ch.Field, err)
		}
		formData[ch.Field] = plain
	default:
		formData[ch.Field] = ch.New
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

The list-axis tests in this task depend on `PortalCampaign.SpecListIDs`/`DeniedSpecs` (Task 2) and `inventory.Campaign.TargetLanguage`/`SubjectFilterMode`/`Subjects`/`DeniedSpecs` plus `inventory.TargetSubject` (Task 1) — merge or stack this task on top of those first if run standalone.

Run: `go test -race ./internal/domain/psacampaign/... ./internal/adapters/clients/psaportal/... -v`
Expected: PASS, including the pre-existing `TestTranslateToDiff_ScalarFields` and `TestTranslateToCreate` (updated in this step to pass a `stubResolver`/`englishResolver()` as the new second/third argument) and `TestPushCampaign_MutatesAndPosts` (unaffected — no `Value` set on its `FieldChange`s, so it still hits the `default` branch).

Add one case to `push_test.go` alongside `TestPushCampaign_MutatesAndPosts`:

```go
func TestPushCampaign_ListValuedFieldUsesValueOverNew(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	routes := bundleRoutes()
	routes["/edit/__data.json?x-sveltekit-invalidated=0001"] = string(edit)
	routes["/buyercampaignmanager/_app/remote/abc123/updateCampaign"] = `{"type":"result","result":"[{}]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	err = c.PushCampaign(context.Background(), "660a980d-bf1c-4988-9958-1eb2d1853c66",
		[]psacampaign.FieldChange{
			{Field: "selectedSubjects", Old: "1:Old", New: "1:Old", Value: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}}},
		})
	if err != nil {
		t.Fatalf("PushCampaign: %v", err)
	}
	payloadStr := extractPayload(t, ff.captured["/buyercampaignmanager/_app/remote/abc123/updateCampaign"])
	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	var packed []json.RawMessage
	if err := json.Unmarshal(decoded, &packed); err != nil {
		t.Fatalf("unmarshal packed: %v", err)
	}
	resolved, err := DecodeRefPacked(packed)
	if err != nil {
		t.Fatalf("DecodeRefPacked: %v", err)
	}
	entry := resolved.(map[string]any)
	formData := entry["formData"].(map[string]any)
	subjects, ok := formData["selectedSubjects"].([]any)
	if !ok {
		t.Fatalf("selectedSubjects = %#v (%T), want a JSON array (from Value, not the New string)", formData["selectedSubjects"], formData["selectedSubjects"])
	}
	if len(subjects) != 1 {
		t.Fatalf("len(subjects) = %d, want 1", len(subjects))
	}
	entry0 := subjects[0].(map[string]any)
	if entry0["id"] != float64(22210) || entry0["name"] != "Machamp" {
		t.Errorf("subjects[0] = %+v, want {id:22210 name:Machamp}", entry0)
	}
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/psacampaign/mapper.go internal/domain/psacampaign/mapper_test.go internal/domain/psacampaign/types.go internal/adapters/clients/psaportal/push.go internal/adapters/clients/psaportal/push_test.go internal/testutil/mocks/psa_resolver.go
git commit -m "feat(psacampaign): translate spec-list/subject/denied-spec axes via Resolver"
```

---

### Task 9: Wire the main server

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns.go:31-114` (new `psaCatalog` field + `WithPSACatalogStore` option)
- Modify: `internal/adapters/httpserver/handlers/campaigns_psa.go:1-20,118-142,240-268` (resolver-building helper, new `GET /api/psa/subjects` handler, staleness/nil guards at the two translate call sites)
- Modify: `internal/adapters/httpserver/routes.go:116` (register the new endpoint)
- Modify: `docs/API.md` (document `GET /api/psa/subjects`)
- Modify: `internal/adapters/httpserver/handlers/campaigns_psa_test.go` (`newTestPSAHandler` gains a catalog param; new subjects-endpoint tests)
- Modify: `internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go` (existing propose/create tests wired with a working `CatalogStoreMock` + `ResolverMock`-backed catalog so they keep passing under the new nil/staleness guards)
- Test: covered by the modified files above (no new standalone test file)

**Interfaces:**
- Consumes: `psacampaign.CatalogStore` (Task 5, `repository.go`): `SpecLists(ctx) ([]SpecListRef, time.Time, error)`, `Subjects(ctx, categoryID int) ([]SubjectRef, time.Time, error)`; `psacampaign.NewCatalogResolver(specLists []SpecListRef, subjects []SubjectRef, fetchedAt, now time.Time) (Resolver, error)` and `psacampaign.ErrCatalogStale`, `psacampaign.PokemonCategoryID` (Task 5, `resolver.go`); `psacampaign.TranslateToDiff`/`TranslateToCreate` (Task 8, this part); `mocks.ResolverMock` (Task 8, this part); `mocks.CatalogStoreMock` (Task 5, `internal/testutil/mocks/`) implementing `psacampaign.CatalogStore` with the Fn-field pattern:
  - `SaveSpecLists(ctx context.Context, lists []psacampaign.SpecListRef) error`
  - `SaveSubjects(ctx context.Context, categoryID int, subjects []psacampaign.SubjectRef) error`
  - `SpecLists(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error)`
  - `Subjects(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error)`
- Produces: `func WithPSACatalogStore(s psacampaign.CatalogStore) CampaignsHandlerOption`; `func (h *CampaignsHandler) HandleGetPSASubjects(w http.ResponseWriter, r *http.Request)`; `GET /api/psa/subjects` route — leaves with no further in-plan consumers, but the main server's construction (`cmd/slabledger/server.go`) must pass a real Postgres-backed `CatalogStore` (Task 5's adapter) into `WithPSACatalogStore` for this to work outside tests — that wiring is Task 5/10's responsibility (the `CatalogStore` adapter and its constructor are out of this task's scope; only the handler-side option and its use are produced here).

**Judgment call:** both "no catalog dependency configured" and "catalog present but stale" return `503 Service Unavailable`, mirroring the existing `h.psaSnapshots == nil` guard (`campaigns_psa.go:19-23`) rather than inventing a new status code for staleness — both cases mean the same thing operationally ("the harvester needs to run"), and the response body names which.

- [ ] **Step 1: Write the failing test**

```go
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestHandleGetPSASubjects_NoStore(t *testing.T) {
	h := NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/psa/subjects", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPSASubjects(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetPSASubjects_Success(t *testing.T) {
	fetchedAt := time.Now().Add(-time.Hour)
	catalog := &mocks.CatalogStoreMock{
		SubjectsFn: func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
			if categoryID != psacampaign.PokemonCategoryID {
				t.Fatalf("categoryID = %d, want PokemonCategoryID", categoryID)
			}
			return []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}}, fetchedAt, nil
		},
	}
	h := NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background(),
		WithPSACatalogStore(catalog))
	req := httptest.NewRequest(http.MethodGet, "/api/psa/subjects", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPSASubjects(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/httpserver/handlers/... -run TestHandleGetPSASubjects -v`
Expected: FAIL to compile — `undefined: WithPSACatalogStore`, `undefined: h.HandleGetPSASubjects` (`mocks.CatalogStoreMock` already exists from Task 5, so it is not part of the expected failure)

- [ ] **Step 3: Write the implementation**

`campaigns.go` — add the field and option, right beside the existing `psaQueue` one:

```go
psaSnapshots psacampaign.SnapshotStore  // optional: PSA portal campaign snapshot reader
psaQueue     psacampaign.PushQueueStore // optional: PSA propose/publish push queue
psaCatalog   psacampaign.CatalogStore   // optional: PSA spec-list/subject catalog reader
```

```go
// WithPSACatalogStore enables the PSA spec-list/subject catalog reader, which
// the translators need (via a Resolver) to push list-valued targeting.
func WithPSACatalogStore(s psacampaign.CatalogStore) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.psaCatalog = s }
}
```

`campaigns_psa.go` — add `"time"` to imports (already imports most of what's needed) and the resolver-building helper plus the new handler:

```go
// buildResolver reads the persisted PSA portal catalog and builds a pure
// Resolver for one translation call. The main server has no portal session —
// see psacampaign.NewCatalogResolver's doc — so it can only translate against
// whatever the harvester most recently wrote.
func (h *CampaignsHandler) buildResolver(ctx context.Context) (psacampaign.Resolver, error) {
	specLists, specFetchedAt, err := h.psaCatalog.SpecLists(ctx)
	if err != nil {
		return nil, fmt.Errorf("read spec-list catalog: %w", err)
	}
	subjects, subjFetchedAt, err := h.psaCatalog.Subjects(ctx, psacampaign.PokemonCategoryID)
	if err != nil {
		return nil, fmt.Errorf("read subject catalog: %w", err)
	}
	// Staleness is judged against whichever half of the catalog is older, so
	// a harvester that stopped updating subjects (say) fails closed even if
	// spec lists happen to still be fresh.
	fetchedAt := specFetchedAt
	if subjFetchedAt.Before(fetchedAt) {
		fetchedAt = subjFetchedAt
	}
	return psacampaign.NewCatalogResolver(specLists, subjects, fetchedAt, time.Now())
}

// HandleGetPSASubjects handles GET /api/psa/subjects, returning the persisted
// subject catalog for the frontend's subject-name typeahead. Served entirely
// from CatalogStore — the main server never calls the portal.
func (h *CampaignsHandler) HandleGetPSASubjects(w http.ResponseWriter, r *http.Request) {
	if h.psaCatalog == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA campaign sync not enabled")
		return
	}
	subjects, fetchedAt, err := h.psaCatalog.Subjects(r.Context(), psacampaign.PokemonCategoryID)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get PSA subject catalog", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subjects":  subjects,
		"fetchedAt": fetchedAt,
	})
}
```

Add `"fmt"` to `campaigns_psa.go`'s import block alongside the existing ones.

Wire the two translate call sites. In `HandlePSAPropose` (around the existing `:137`), replace:

```go
diff, err := psacampaign.TranslateToDiff(*c, *portal)
if err != nil {
	h.logger.Error(r.Context(), "failed to translate campaign diff", observability.Err(err))
	writeError(w, http.StatusInternalServerError, "Internal server error")
	return
}
```

with:

```go
if h.psaCatalog == nil {
	writeError(w, http.StatusServiceUnavailable, "PSA catalog not enabled — run the harvester")
	return
}
resolver, err := h.buildResolver(r.Context())
if err != nil {
	if errors.Is(err, psacampaign.ErrCatalogStale) {
		writeError(w, http.StatusServiceUnavailable, "PSA catalog is stale — run the harvester (cmd/psa-harvest) to refresh spec lists and subjects")
		return
	}
	h.logger.Error(r.Context(), "failed to build PSA catalog resolver", observability.Err(err))
	writeError(w, http.StatusInternalServerError, "Internal server error")
	return
}

diff, err := psacampaign.TranslateToDiff(*c, *portal, resolver)
if err != nil {
	h.logger.Error(r.Context(), "failed to translate campaign diff", observability.Err(err))
	writeError(w, http.StatusInternalServerError, "Internal server error")
	return
}
```

In `HandlePSAProposeCreate` (around the existing `:264`), replace:

```go
fd, err := psacampaign.TranslateToCreate(*c)
if err != nil {
	writeError(w, http.StatusBadRequest, err.Error())
	return
}
```

with:

```go
if h.psaCatalog == nil {
	writeError(w, http.StatusServiceUnavailable, "PSA catalog not enabled — run the harvester")
	return
}
resolver, err := h.buildResolver(r.Context())
if err != nil {
	if errors.Is(err, psacampaign.ErrCatalogStale) {
		writeError(w, http.StatusServiceUnavailable, "PSA catalog is stale — run the harvester (cmd/psa-harvest) to refresh spec lists and subjects")
		return
	}
	h.logger.Error(r.Context(), "failed to build PSA catalog resolver", observability.Err(err))
	writeError(w, http.StatusInternalServerError, "Internal server error")
	return
}

fd, err := psacampaign.TranslateToCreate(*c, resolver)
if err != nil {
	writeError(w, http.StatusBadRequest, err.Error())
	return
}
```

(`errors` is already imported in `campaigns_psa.go` for `errors.Is(err, psacampaign.ErrPushNotPending)`.)

`routes.go:116` — register the new route beside the existing PSA sync group:

```go
mux.Handle("GET /api/psa-campaigns", authRoute(rt.campaignsHandler.HandleListPSACampaigns))
mux.Handle("GET /api/psa/subjects", authRoute(rt.campaignsHandler.HandleGetPSASubjects))
mux.Handle("POST /api/campaigns/{id}/psa-link", authRoute(rt.campaignsHandler.HandlePSALink))
```

`docs/API.md` — add after the `GET /api/psa-campaigns` section (before its trailing `---`):

````markdown
### `GET /api/psa/subjects`

Auth: required (session)

Returns the persisted PSA subject catalog (Pokemon category) harvested by
`cmd/psa-harvest`, for the campaign form's subject-name typeahead. Served
entirely from the database — the main server never contacts psacard.com.

**Response:** `200 OK`
```json
{
  "subjects": [{ "id": 22210, "name": "Machamp" }],
  "fetchedAt": "2026-08-06T10:00:00Z"
}
```

**Errors:** `503` PSA campaign sync not enabled; `500` internal error

---
````

Update `campaigns_psa_test.go`'s helper so existing snapshot/queue tests keep compiling and passing:

```go
func newTestPSAHandler(snap *mocks.SnapshotStoreMock, queue *mocks.PushQueueStoreMock) *CampaignsHandler {
	var opts []CampaignsHandlerOption
	if snap != nil {
		opts = append(opts, WithPSASnapshotStore(snap))
	}
	if queue != nil {
		opts = append(opts, WithPSAPushQueue(queue))
	}
	return NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background(), opts...)
}

// freshCatalog returns a CatalogStoreMock whose fetchedAt is always "now", so
// tests exercising propose/create do not trip the new staleness guard.
func freshCatalog(specLists []psacampaign.SpecListRef, subjects []psacampaign.SubjectRef) *mocks.CatalogStoreMock {
	now := time.Now()
	return &mocks.CatalogStoreMock{
		SpecListsFn: func(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error) {
			return specLists, now, nil
		},
		SubjectsFn: func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
			return subjects, now, nil
		},
	}
}
```

Update `campaigns_psa_propose_test.go`'s existing successful-path cases to pass `WithPSACatalogStore(freshCatalog(nil, nil))` alongside the snapshot/queue mocks wherever they currently expect a 200 from `HandlePSAPropose`/`HandlePSAProposeCreate` — `diffCampaign()`'s `TargetLanguage` stays empty in those fixtures, which (per Task 8's judgment call) means `TranslateToDiff` skips the spec-list axis and needs no spec-list entries in the catalog, and `TranslateToCreate`'s own callers already expect its `TargetLanguage`-empty error path to surface as today's `400`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/adapters/httpserver/... -v`
Expected: PASS, including the pre-existing `TestHandlePSAPropose` and the create-proposal tests once updated with `freshCatalog`.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns.go internal/adapters/httpserver/handlers/campaigns_psa.go internal/adapters/httpserver/handlers/campaigns_psa_test.go internal/adapters/httpserver/handlers/campaigns_psa_propose_test.go internal/adapters/httpserver/routes.go docs/API.md
git commit -m "feat(httpserver): wire PSA catalog resolver into propose/create and add subjects endpoint"
```

### Task 10: Harvester — persist catalogs + the one-time baseline pull

**Files:**
- Create: `cmd/psa-harvest/baseline.go`
- Create: `cmd/psa-harvest/baseline_test.go`
- Modify: `cmd/psa-harvest/main.go:33-127` (`main`/`run`: `-baseline-pull` flag filtering, catalog persistence every run, baseline-mode branch that returns before `DrainPushQueue`, timeout)

**Interfaces:**
- Consumes: `psacampaign.CatalogStore{SaveSpecLists(ctx, []SpecListRef) error; SaveSubjects(ctx, categoryID int, []SubjectRef) error}`; `psacampaign.PokemonCategoryID`; `psacampaign.SpecListRef{ID, Name, Status string}`; `psacampaign.SubjectRef{ID int; Name string}`; `psacampaign.PortalCampaign{CampaignRequestID string; TargetingComplete bool; SpecListIDs, SpecListNames []string; DeniedSpecs []SubjectRef; SubjectFilter CampaignFilter{Type string; Subjects []SubjectRef}}`; `(*psaportal.Client).FetchSubjects(ctx, categoryID int) ([]psacampaign.SubjectRef, error)`; `inventory.TargetSubject{ID int; Name string}`; `inventory.Campaign` fields `TargetLanguage string`, `SubjectFilterMode string`, `Subjects []TargetSubject`, `DeniedSpecs []TargetSubject`; `inventory.CampaignRepository{ListCampaigns(ctx, activeOnly bool) ([]Campaign, error); UpdateCampaign(ctx, *Campaign) error}`; `cardutil.LangJapanese`, `cardutil.LangEnglish`; `mocks.CampaignRepositoryMock` (`internal/testutil/mocks/inventory_campaign_repo.go`, already exists).
  - **Assumption (flagged, not in the frozen contract):** `(*psaportal.Client).FetchCampaigns(ctx context.Context) ([]psacampaign.PortalCampaign, []psacampaign.SpecListRef, error)` — widened to also return the spec-list catalog. The contract only widens the *private* `fetchCampaignFormData`, but §2a of the design says that widening exists "so the harvester can persist it without a second fetch," and the harvester never calls the private method directly — `FetchCampaigns` is its only entry point. This task therefore assumes `FetchCampaigns` carries the second return value through from its internal `fetchCampaignFormData` loop. If the owning task instead threads the catalog out some other way, only the four lines in `main.go` that call `portal.FetchCampaigns` need to change.
  - **Assumption (flagged, not in the frozen contract):** `postgres.NewPSAPortalCatalogStore(db *sql.DB) *postgres.PSAPortalCatalogStore` implementing `psacampaign.CatalogStore`, backed by migration `000024_psa_portal_catalog`. Per `docs/superpowers/plans/parts/part3-translate.md:901` this concrete adapter is Task 5's responsibility, not this task's — this task only calls its assumed constructor from `main.go`, mirroring the existing `postgres.NewPSACampaignSnapshotStore(db.DB)` / `postgres.NewPSACampaignLinker(db.DB)` call sites already in that file.
- Produces:
  - `func parseBaselineFlag(args []string) (baseline bool, rest []string)`
  - `func baselineLanguage(specListNames []string) (string, error)`
  - `func buildBaselineCampaign(existing inventory.Campaign, pc psacampaign.PortalCampaign) (inventory.Campaign, error)`
  - `func runBaselinePull(ctx context.Context, portalCampaigns []psacampaign.PortalCampaign, campaigns inventory.CampaignRepository, logger observability.Logger) error`
  - The `-baseline-pull` operator flag and its zero-portal-write contract on `cmd/psa-harvest`.

- [ ] **Step 1: Write the failing test for flag filtering**

```go
// cmd/psa-harvest/baseline_test.go
package main

import (
	"reflect"
	"testing"
)

func TestParseBaselineFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantBaseline bool
		wantRest     []string
	}{
		{
			name:         "no flags",
			args:         []string{},
			wantBaseline: false,
			wantRest:     []string{},
		},
		{
			name:         "baseline flag alone",
			args:         []string{"-baseline-pull"},
			wantBaseline: true,
			wantRest:     []string{},
		},
		{
			name:         "double-dash form",
			args:         []string{"--baseline-pull"},
			wantBaseline: true,
			wantRest:     []string{},
		},
		{
			name:         "explicit false is filtered but not set",
			args:         []string{"-baseline-pull=false", "-log-level", "debug"},
			wantBaseline: false,
			wantRest:     []string{"-log-level", "debug"},
		},
		{
			name:         "baseline flag mixed with unrelated flags config.Load must still see",
			args:         []string{"-log-level", "debug", "-baseline-pull", "-cache", "/tmp/cache.json"},
			wantBaseline: true,
			wantRest:     []string{"-log-level", "debug", "-cache", "/tmp/cache.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline, rest := parseBaselineFlag(tt.args)
			if baseline != tt.wantBaseline {
				t.Errorf("baseline = %v, want %v", baseline, tt.wantBaseline)
			}
			if !reflect.DeepEqual(rest, tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/psa-harvest/... -run TestParseBaselineFlag -v`
Expected: FAIL with `./baseline_test.go:15:12: undefined: parseBaselineFlag` (compile error)

- [ ] **Step 3: Implement flag filtering**

```go
// cmd/psa-harvest/baseline.go
package main

import "strconv"

// parseBaselineFlag pulls -baseline-pull out of the raw CLI args before they
// reach config.Load(args). config.FromFlags (internal/platform/config/loader.go:236-262)
// builds its own flag.FlagSet defining only 9 flags (web, port, rate-limit,
// trust-proxy, log-level, log-json, cache, database-url, migrations-path) and
// fails with "flag provided but not defined" on anything else, so
// -baseline-pull must never reach it. rest is every arg config.Load is still
// allowed to see.
func parseBaselineFlag(args []string) (baseline bool, rest []string) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "-baseline-pull" || a == "--baseline-pull":
			baseline = true
		case len(a) > len("-baseline-pull=") && a[:len("-baseline-pull=")] == "-baseline-pull=":
			baseline = parseBoolFlag(a[len("-baseline-pull="):])
		case len(a) > len("--baseline-pull=") && a[:len("--baseline-pull=")] == "--baseline-pull=":
			baseline = parseBoolFlag(a[len("--baseline-pull="):])
		default:
			rest = append(rest, a)
		}
	}
	return baseline, rest
}

// parseBoolFlag parses a CLI bool value, defaulting to false on garbage input
// rather than erroring — an operator typo in -baseline-pull=maybe should fall
// back to the safe (non-baseline, zero-write) mode, not crash the harvester.
func parseBoolFlag(v string) bool {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./cmd/psa-harvest/... -run TestParseBaselineFlag -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/psa-harvest/baseline.go cmd/psa-harvest/baseline_test.go
git commit -m "feat(psa-harvest): parse -baseline-pull outside config.Load's FlagSet"
```

- [ ] **Step 6: Write the failing test for language mapping**

```go
// cmd/psa-harvest/baseline_test.go (append)
func TestBaselineLanguage(t *testing.T) {
	tests := []struct {
		name          string
		specListNames []string
		want          string
		wantErr       bool
	}{
		{name: "japanese", specListNames: []string{"Japanese Pokemon"}, want: "japanese"},
		{name: "english", specListNames: []string{"English Pokemon"}, want: "english"},
		{name: "no recognized name", specListNames: []string{}, wantErr: true},
		{name: "unrecognized name only", specListNames: []string{"Something Else"}, wantErr: true},
		{
			name:          "both recognized names is ambiguous",
			specListNames: []string{"Japanese Pokemon", "English Pokemon"},
			wantErr:       true,
		},
		{
			name:          "duplicate of the same name is not ambiguous",
			specListNames: []string{"Japanese Pokemon", "Japanese Pokemon"},
			want:          "japanese",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := baselineLanguage(tt.specListNames)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("baselineLanguage(%v) = %q, nil; want error", tt.specListNames, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("baselineLanguage(%v): unexpected error: %v", tt.specListNames, err)
			}
			if got != tt.want {
				t.Errorf("baselineLanguage(%v) = %q, want %q", tt.specListNames, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./cmd/psa-harvest/... -run TestBaselineLanguage -v`
Expected: FAIL with `./baseline_test.go:XX:XX: undefined: baselineLanguage` (compile error)

- [ ] **Step 8: Implement language mapping**

```go
// cmd/psa-harvest/baseline.go (append)
import (
	"errors"

	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// errNoSpecListName means none of the campaign's curated spec-list names
// mapped to a language token. This is the expected shape of the 3 remaining
// CATEGORY-era campaigns (design doc §8): their edit form predates the
// curated-list model, so it names no "Japanese Pokemon" / "English Pokemon"
// list. They are not writable here; the operator converts them by hand in
// the portal and re-runs the baseline.
var errNoSpecListName = errors.New("no recognized curated spec-list name (see design §8: conversion path for CATEGORY campaigns)")

// errAmbiguousSpecListName means more than one distinct language mapped —
// writing either guess would be a coin flip on a live buying campaign.
var errAmbiguousSpecListName = errors.New("multiple recognized curated spec-list names present")

// baselineLanguage maps a portal campaign's curated spec-list names to the
// language token stored in inventory.Campaign.TargetLanguage. Exactly one
// distinct recognized name must be present.
func baselineLanguage(specListNames []string) (string, error) {
	token := ""
	for _, name := range specListNames {
		var candidate string
		switch name {
		case "Japanese Pokemon":
			candidate = cardutil.LangJapanese
		case "English Pokemon":
			candidate = cardutil.LangEnglish
		default:
			continue
		}
		if token != "" && token != candidate {
			return "", errAmbiguousSpecListName
		}
		token = candidate
	}
	if token == "" {
		return "", errNoSpecListName
	}
	return token, nil
}
```

- [ ] **Step 9: Run test to verify it passes**

Run: `go test -race ./cmd/psa-harvest/... -run TestBaselineLanguage -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add cmd/psa-harvest/baseline.go cmd/psa-harvest/baseline_test.go
git commit -m "feat(psa-harvest): map curated spec-list names to a language token"
```

- [ ] **Step 11: Write the failing test for the per-campaign mapping**

```go
// cmd/psa-harvest/baseline_test.go (append)
import (
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

func TestBuildBaselineCampaign(t *testing.T) {
	existing := inventory.Campaign{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"}

	tests := []struct {
		name    string
		pc      psacampaign.PortalCampaign
		want    inventory.Campaign
		wantErr bool
	}{
		{
			name: "japanese target campaign copies subjects and denied specs verbatim",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListNames:     []string{"Japanese Pokemon"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Target",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}, {ID: 8105, Name: "Crystal Golem"}},
				},
				DeniedSpecs: []psacampaign.SubjectRef{{ID: 4807, Name: "Gold Star Charizard"}},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguage:    "japanese",
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
				SpecListNames:     []string{"English Pokemon"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Exclude"},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguage:    "english",
				SubjectFilterMode: "Exclude",
				Subjects:          []inventory.TargetSubject{},
				DeniedSpecs:       []inventory.TargetSubject{},
			},
		},
		{
			name: "unmappable language is an error, existing campaign untouched",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListNames:     []string{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildBaselineCampaign(existing, tt.pc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildBaselineCampaign(): got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildBaselineCampaign(): unexpected error: %v", err)
			}
			if got.TargetLanguage != tt.want.TargetLanguage ||
				got.SubjectFilterMode != tt.want.SubjectFilterMode ||
				len(got.Subjects) != len(tt.want.Subjects) ||
				len(got.DeniedSpecs) != len(tt.want.DeniedSpecs) {
				t.Fatalf("buildBaselineCampaign() = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want.Subjects {
				if got.Subjects[i] != tt.want.Subjects[i] {
					t.Errorf("Subjects[%d] = %+v, want %+v", i, got.Subjects[i], tt.want.Subjects[i])
				}
			}
			for i := range tt.want.DeniedSpecs {
				if got.DeniedSpecs[i] != tt.want.DeniedSpecs[i] {
					t.Errorf("DeniedSpecs[%d] = %+v, want %+v", i, got.DeniedSpecs[i], tt.want.DeniedSpecs[i])
				}
			}
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.PSACampaignRequestID != tt.want.PSACampaignRequestID {
				t.Errorf("existing campaign fields altered: got %+v", got)
			}
		})
	}
}
```

- [ ] **Step 12: Run test to verify it fails**

Run: `go test ./cmd/psa-harvest/... -run TestBuildBaselineCampaign -v`
Expected: FAIL with `./baseline_test.go:XX:XX: undefined: buildBaselineCampaign` (compile error)

- [ ] **Step 13: Implement the per-campaign mapping**

```go
// cmd/psa-harvest/baseline.go (append)

// buildBaselineCampaign copies one portal campaign's targeting onto a copy of
// the internal campaign already linked to it. Subject and denied-spec ids are
// copied verbatim, never re-resolved by name: live portal ids span 4xxx/8xxx/
// 22xxx generations while getSubjects (used only for operator-added subjects,
// see (*psaportal.Client).FetchSubjects) returns only 22xxx ids, so
// name-based resolution here would silently rewrite ids on active,
// money-spending campaigns on the very next push.
func buildBaselineCampaign(existing inventory.Campaign, pc psacampaign.PortalCampaign) (inventory.Campaign, error) {
	lang, err := baselineLanguage(pc.SpecListNames)
	if err != nil {
		return inventory.Campaign{}, err
	}

	updated := existing
	updated.TargetLanguage = lang
	updated.SubjectFilterMode = pc.SubjectFilter.Type
	updated.Subjects = toTargetSubjects(pc.SubjectFilter.Subjects)
	updated.DeniedSpecs = toTargetSubjects(pc.DeniedSpecs)
	return updated, nil
}

// toTargetSubjects copies portal subject refs into the internal shape,
// preserving order and ids as-is.
func toTargetSubjects(refs []psacampaign.SubjectRef) []inventory.TargetSubject {
	out := make([]inventory.TargetSubject, len(refs))
	for i, r := range refs {
		out[i] = inventory.TargetSubject{ID: r.ID, Name: r.Name}
	}
	return out
}
```

- [ ] **Step 14: Run test to verify it passes**

Run: `go test -race ./cmd/psa-harvest/... -run TestBuildBaselineCampaign -v`
Expected: PASS

- [ ] **Step 15: Commit**

```bash
git add cmd/psa-harvest/baseline.go cmd/psa-harvest/baseline_test.go
git commit -m "feat(psa-harvest): map portal targeting onto an internal campaign copy"
```

- [ ] **Step 16: Write the failing test for the orchestration loop**

```go
// cmd/psa-harvest/baseline_test.go (append)
import (
	"context"
	"errors"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestRunBaselinePull(t *testing.T) {
	linkedComplete := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListNames: []string{"Japanese Pokemon"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedIncomplete := psacampaign.PortalCampaign{CampaignRequestID: "req-2", TargetingComplete: false}
	linkedAmbiguousLanguage := psacampaign.PortalCampaign{
		CampaignRequestID: "req-3", TargetingComplete: true,
		SpecListNames: []string{}, // no recognized name -> unconverted CATEGORY campaign, §8
	}
	notLinked := psacampaign.PortalCampaign{CampaignRequestID: "req-unlinked", TargetingComplete: true}

	internal := []inventory.Campaign{
		{ID: "camp-1", PSACampaignRequestID: "req-1"},
		{ID: "camp-2", PSACampaignRequestID: "req-2"},
		{ID: "camp-3", PSACampaignRequestID: "req-3"},
	}

	tests := []struct {
		name       string
		portal     []psacampaign.PortalCampaign
		updateErr  error
		wantErr    bool
		wantWrites int
	}{
		{
			name:       "writes the linked complete campaign, skips the rest, exits non-zero",
			portal:     []psacampaign.PortalCampaign{linkedComplete, linkedIncomplete, linkedAmbiguousLanguage, notLinked},
			wantErr:    true,
			wantWrites: 1,
		},
		{
			name:       "all campaigns clean is a nil error",
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    false,
			wantWrites: 1,
		},
		{
			name:       "an update failure aborts immediately",
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			updateErr:  errors.New("db down"),
			wantErr:    true,
			wantWrites: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			repo := &mocks.CampaignRepositoryMock{
				ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
					return internal, nil
				},
				UpdateCampaignFn: func(ctx context.Context, c *inventory.Campaign) error {
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
		})
	}
}
```

- [ ] **Step 17: Run test to verify it fails**

Run: `go test ./cmd/psa-harvest/... -run TestRunBaselinePull -v`
Expected: FAIL with `./baseline_test.go:XX:XX: undefined: runBaselinePull` (compile error).

- [ ] **Step 18: Implement the orchestration loop**

```go
// cmd/psa-harvest/baseline.go (append)
import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// runBaselinePull copies live portal targeting into every linked, complete
// SlabLedger campaign row. It performs zero portal writes — the caller in
// main.go must return the result of this function directly, before ever
// reaching DrainPushQueue. Campaigns are skipped (not written) when they are
// not linked to a portal campaign, when the edit-form fetch that produced pc
// failed (TargetingComplete == false), or when the campaign's curated
// spec-list names don't map to exactly one language. Any skip is logged and
// makes the whole run return a non-zero-exit error, so a partial baseline is
// never mistaken for a complete one; the pull is idempotent, so re-running it
// is the remedy.
func runBaselinePull(ctx context.Context, portalCampaigns []psacampaign.PortalCampaign, campaigns inventory.CampaignRepository, logger observability.Logger) error {
	internal, err := campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return fmt.Errorf("baseline: list campaigns: %w", err)
	}
	byRequestID := make(map[string]inventory.Campaign, len(internal))
	for _, c := range internal {
		if c.PSACampaignRequestID != "" {
			byRequestID[c.PSACampaignRequestID] = c
		}
	}

	var skipped []string
	for _, pc := range portalCampaigns {
		existing, linked := byRequestID[pc.CampaignRequestID]
		if !linked {
			continue // no internal campaign to write to; the report step (not this function) flags these for the operator
		}
		if !pc.TargetingComplete {
			logger.Warn(ctx, "psa-harvest: baseline skipping campaign, edit-form fetch was incomplete",
				observability.String("campaignId", existing.ID),
				observability.String("psaCampaignRequestId", pc.CampaignRequestID))
			skipped = append(skipped, existing.ID)
			continue
		}

		updated, err := buildBaselineCampaign(existing, pc)
		if err != nil {
			logger.Warn(ctx, "psa-harvest: baseline skipping campaign",
				observability.String("campaignId", existing.ID),
				observability.String("psaCampaignRequestId", pc.CampaignRequestID),
				observability.Err(err))
			skipped = append(skipped, existing.ID)
			continue
		}

		if err := campaigns.UpdateCampaign(ctx, &updated); err != nil {
			return fmt.Errorf("baseline: update campaign %s: %w", existing.ID, err)
		}
		logger.Info(ctx, "psa-harvest: baseline wrote campaign targeting",
			observability.String("campaignId", existing.ID),
			observability.String("targetLanguage", updated.TargetLanguage))
	}

	if len(skipped) > 0 {
		return fmt.Errorf("baseline: %d campaign(s) skipped, see warnings above: %v", len(skipped), skipped)
	}
	return nil
}
```

- [ ] **Step 19: Run test to verify it passes**

Run: `go test -race ./cmd/psa-harvest/... -run TestRunBaselinePull -v`
Expected: PASS

- [ ] **Step 20: Commit**

```bash
git add cmd/psa-harvest/baseline.go cmd/psa-harvest/baseline_test.go
git commit -m "feat(psa-harvest): orchestrate the fail-closed baseline pull"
```

- [ ] **Step 21: Wire main.go — catalog persistence every run, baseline branch, timeout**

This step has no new unit test: `cmd/psa-harvest/main.go` has never had one (no `main_test.go` exists today), because `run()` wires a real browser session and a real DB connection that only exist in an integration environment. Every decision `run()` makes now delegates to the four unit-tested functions above; `run()` itself is glue. Modify it as follows.

```go
// cmd/psa-harvest/main.go (replace lines 33-127)
func main() {
	baseline, rest := parseBaselineFlag(os.Args[1:])
	if err := run(baseline, rest); err != nil {
		log.Fatalf("psa-harvest: %v", err)
	}
}

func run(baseline bool, args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "json")
	// Bound the whole harvest: the Playwright browser run and the DB writes
	// inherit this deadline, so a hung login or navigation kills the process
	// instead of leaving the scheduled machine blocked (and auto-restarting)
	// forever. The in-script Playwright steps time out well inside this.
	// Baseline mode fetches the edit form for every campaign in the fleet
	// (9 campaigns today) rather than relying on the incremental snapshot, so
	// it gets a longer budget; it is still bounded, and it is a one-time,
	// operator-invoked run rather than the hourly schedule.
	timeout := 5 * time.Minute
	if baseline {
		timeout = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch {
	case cfg.PSAPortal.Email == "" || cfg.PSAPortal.Password == "":
		return errors.New("PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required")
	case cfg.Auth.EncryptionKey == "":
		return errors.New("ENCRYPTION_KEY is required (token is encrypted at rest)")
	case cfg.Database.URL == "":
		return errors.New("DATABASE_URL is required")
	}

	dbCtx, dbCancel := context.WithTimeout(ctx, 90*time.Second)
	db, err := postgres.Open(dbCtx, cfg.Database.URL, logger)
	dbCancel()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	defer func() { _ = db.Close() }()

	enc, err := crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryptor: %w", err)
	}
	store := postgres.NewPSAPortalTokenStore(db.DB, enc)
	snapshots := postgres.NewPSAPortalSnapshotStore(db.DB)

	// One browser login per run, shared by the token/analytics harvest and the
	// campaign sync/drain, so every psacard.com call clears Cloudflare. The
	// writes cannot reach the portal any other way, so a failed session open is
	// fatal for the run.
	storedToken, _, _ := store.CurrentToken(ctx) // best-effort; "" just means full SSO
	session, token, expiresAt, err := psaportal.OpenBrowserSession(ctx, ".", cfg.PSAPortal.Email, cfg.PSAPortal.Password, storedToken, cfg.PSAPortal.ProxyURL, logger)
	if err != nil {
		return fmt.Errorf("open portal session: %w", err)
	}
	defer func() { _ = session.Close() }()

	h := psaportal.NewHarvester(store, snapshots, logger)
	// Best-effort: a failed analytics read must not skip queued writes (the
	// session is already authenticated). A persistence failure (token/snapshot
	// DB write) is retryable, so propagate it for a non-zero exit; a
	// browser/Lightdash failure is not helped by a retry — log and continue to
	// the drain, which rides the same authenticated session.
	if err := h.Run(ctx, session, token, expiresAt); err != nil {
		if errors.Is(err, psaportal.ErrPersistence) {
			return err
		}
		logger.Warn(ctx, "psa-harvest: token/analytics harvest failed, continuing to drain",
			observability.Err(err))
	} else {
		logger.Info(ctx, "psa-harvest: token and rows snapshot refreshed")
	}

	if cfg.PSASync.CampaignSyncEnabled {
		portal := psaportal.New(session, psaportal.Config{}, psaportal.WithLogger(logger))
		snap := postgres.NewPSACampaignSnapshotStore(db.DB)
		queue := postgres.NewPSACampaignPushQueueStore(db.DB)
		linker := postgres.NewPSACampaignLinker(db.DB)
		catalog := postgres.NewPSAPortalCatalogStore(db.DB)
		campaignRepo := postgres.NewCampaignStore(db.DB, logger)

		campaigns, specLists, err := portal.FetchCampaigns(ctx)
		switch {
		case err != nil:
			logger.Error(ctx, "psa-harvest: fetch campaigns failed", observability.Err(err))
		case len(campaigns) == 0:
			logger.Warn(ctx, "psa-harvest: fetch campaigns returned no rows, skipping snapshot save")
		default:
			if err := snap.SaveSnapshot(ctx, campaigns); err != nil {
				logger.Error(ctx, "psa-harvest: save snapshot failed", observability.Err(err))
			}
		}

		// Persist the portal reference catalog on every run, baseline or not:
		// this is what keeps the main server's translation Resolver inside
		// psacampaign.CatalogMaxAge without needing its own portal session.
		if len(specLists) > 0 {
			if err := catalog.SaveSpecLists(ctx, specLists); err != nil {
				logger.Error(ctx, "psa-harvest: save spec-list catalog failed", observability.Err(err))
			}
		}
		if subjects, err := portal.FetchSubjects(ctx, psacampaign.PokemonCategoryID); err != nil {
			logger.Error(ctx, "psa-harvest: fetch subjects failed", observability.Err(err))
		} else if err := catalog.SaveSubjects(ctx, psacampaign.PokemonCategoryID, subjects); err != nil {
			logger.Error(ctx, "psa-harvest: save subject catalog failed", observability.Err(err))
		}

		if baseline {
			// Zero portal writes: return here, before DrainPushQueue is ever
			// reached. This is the whole safety property of -baseline-pull,
			// enforced structurally rather than by hoping the queue is empty.
			if err := runBaselinePull(ctx, campaigns, campaignRepo, logger); err != nil {
				return fmt.Errorf("baseline: %w", err)
			}
			logger.Info(ctx, "psa-harvest: baseline pull complete, all linked campaigns had complete targeting")
			return nil
		}

		pushed, failed := psaportal.DrainPushQueue(ctx, portal, queue, linker, logger)
		logger.Info(ctx, "psa-harvest: push queue drained",
			observability.Int("pushed", pushed), observability.Int("failed", failed))
	}

	return nil
}
```

Add the compile-time guard alongside the existing two at the top of the file (`var _ psaportal.TokenRepository = …`, `var _ psaportal.SnapshotWriter = …`):

```go
var _ psacampaign.CatalogStore = (*postgres.PSAPortalCatalogStore)(nil)
```

and add `"github.com/guarzo/slabledger/internal/domain/psacampaign"` to the import block.

- [ ] **Step 22: Build to verify the wiring compiles**

Run: `go build ./cmd/psa-harvest/...`
Expected: succeeds once Task 5's `psacampaign.CatalogStore`/`postgres.PSAPortalCatalogStore`, the widened `FetchCampaigns`, and the four `inventory.Campaign` targeting fields exist. If `postgres.NewPSAPortalCatalogStore` does not yet exist under that exact name, that is this task's one open dependency — see the flagged assumption above; the fix is a one-line rename here, not a redesign.

- [ ] **Step 23: Commit**

```bash
git add cmd/psa-harvest/main.go
git commit -m "feat(psa-harvest): persist portal catalog every run, add -baseline-pull"
```

**Operator command** (run once, from wherever the harvester image runs, e.g. `docker run --rm <harvest-image> ./psa-harvest -baseline-pull`, with the same `PSA_PORTAL_EMAIL`/`PSA_PORTAL_PASSWORD`/`ENCRYPTION_KEY`/`DATABASE_URL` env already required for the hourly job):

```bash
./psa-harvest -baseline-pull
```

A non-zero exit means at least one linked campaign was skipped (incomplete edit-form fetch, or an unconverted CATEGORY campaign per §8); re-run after fixing the cause. A zero exit with the "baseline pull complete" log line means every linked, complete campaign now carries the portal's live targeting and `DrainPushQueue` was never called.

### Task 11: Frontend — targeting axes editor

**Files:**
- Create: `web/src/react/ui/SubjectListEditor.tsx`
- Create: `web/src/react/ui/SubjectListEditor.test.tsx`
- Create: `web/src/js/api/psaCampaigns.test.ts`
- Create: `web/src/react/ui/CampaignFormFields.test.tsx`
- Modify: `web/src/types/campaigns/core.ts:8-27` (Campaign), `:137-151` (CreateCampaignInput)
- Modify: `web/src/types/campaigns/portfolio.ts:66-76` (CampaignSuggestionParams)
- Modify: `web/src/types/campaigns/psaCampaign.ts` (add `PSASubjectsResponse`)
- Modify: `web/src/react/queries/queryKeys.ts:45-47`
- Modify: `web/src/js/api/psaCampaigns.ts`
- Modify: `web/src/react/ui/CampaignFormFields.tsx` (full file)
- Modify: `web/src/react/utils/campaignConstants.ts` (full file)
- Modify: `web/src/react/pages/CampaignsPage.tsx:43-167`
- Test: `web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx:24-46` (`makeCampaign`)
- Test: `web/src/react/pages/campaigns/CampaignsTab.test.tsx:17-39` (`makeCampaign`)

**Interfaces:**
- Consumes: `internal/domain/inventory.Campaign` JSON tags `targetLanguage string`, `subjectFilterMode string`, `subjects []TargetSubject` (`{id int, name string}`), `deniedSpecs []TargetSubject` — frozen contract. `GET /api/psa/subjects` → `{"subjects":[{"id":number,"name":string}],"fetchedAt":string}` (produced by the harvester/catalog-store author's task, not this one).
- Produces: `SubjectListEditor(props: { label: string; value: SubjectRef[]; onChange: (next: SubjectRef[]) => void; inputSize?: 'sm' })`, `api.listPSASubjects(): Promise<PSASubjectsResponse>`, `CampaignFormValues` gaining `targetLanguage`/`subjectFilterMode`/`subjects`/`deniedSpecs` in place of `inclusionList`/`exclusionMode`.

Existing frontend types already declare `export interface SubjectRef { id: number; name: string; }` in `web/src/types/campaigns/psaCampaign.ts:5-8` — that is reused directly as the TS shape for `inventory.TargetSubject`; no duplicate type is introduced.

The frontend API surface lives under `web/src/js/api/*.ts` (a barrel re-exported from `web/src/js/api/index.ts` as the `api` singleton), not a single `api.ts` file — `psaCampaigns.ts` is the existing file for PSA-portal-adjacent endpoints and is where `listPSASubjects` is added.

- [ ] **Step 1: Write the failing tests for the new subject editor and API method**

```typescript
// web/src/js/api/psaCampaigns.test.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { APIClient } from './client';
import './psaCampaigns';

describe('APIClient.listPSASubjects', () => {
  let client: APIClient;

  beforeEach(() => {
    client = new APIClient('/api');
    client.maxRetries = 1;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('GETs /api/psa/subjects and returns the parsed catalog', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        subjects: [{ id: 22210, name: 'Machamp' }],
        fetchedAt: '2026-08-01T00:00:00Z',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.listPSASubjects();

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/psa/subjects',
      expect.objectContaining({ method: undefined }),
    );
    expect(result).toEqual({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
  });
});
```

```tsx
// web/src/react/ui/SubjectListEditor.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import SubjectListEditor from './SubjectListEditor';

vi.mock('../../js/api', () => ({
  api: { listPSASubjects: vi.fn() },
}));

import { api } from '../../js/api';

function renderEditor(value: { id: number; name: string }[], onChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <SubjectListEditor label="Subjects" value={value} onChange={onChange} />
    </QueryClientProvider>,
  );
  return onChange;
}

describe('SubjectListEditor', () => {
  it('shows an empty-catalog message when no subjects have been harvested', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({ subjects: [], fetchedAt: '' });
    renderEditor([]);
    await waitFor(() => {
      expect(screen.getByText(/not yet harvested/i)).toBeInTheDocument();
    });
  });

  it('filters the catalog by typed text and adds a chip on selection, preserving the id', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'char' } });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charizard' })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('button', { name: 'Charizard' }));
    expect(onChange).toHaveBeenCalledWith([{ id: 4807, name: 'Charizard' }]);
  });

  it('adds an unresolved name with id 0 on Enter when there is no exact catalog match', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'Mewtwo' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }]);
  });

  it('removes a chip', () => {
    const onChange = renderEditor([{ id: 4807, name: 'Charizard' }]);
    fireEvent.click(screen.getByRole('button', { name: /remove charizard/i }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('warns when the catalog is older than 7 days', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 1, name: 'Pikachu' }],
      fetchedAt: '2020-01-01T00:00:00Z',
    });
    renderEditor([]);
    await waitFor(() => {
      expect(screen.getByText(/over 7 days old/i)).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/js/api/psaCampaigns.test.ts src/react/ui/SubjectListEditor.test.tsx`
Expected: FAIL — `client.listPSASubjects is not a function` (method not yet declared on `APIClient`) and `Failed to resolve import "./SubjectListEditor"` (module does not exist).

- [ ] **Step 3: Implement the API method, catalog type, and editor component**

```typescript
// web/src/types/campaigns/psaCampaign.ts — append at end of file
export interface PSASubjectsResponse {
  subjects: SubjectRef[];
  fetchedAt: string;
}
```

```typescript
// web/src/js/api/psaCampaigns.ts (full file)
/**
 * PSA portal campaign sync API methods (Task 8 endpoints) and catalog reads.
 */

import type { Campaign, ListPSACampaignsResponse, PSAProposeResponse, PSAProposeCreateResponse, PSAPublishResponse, ListPSAPushesResponse, PSASubjectsResponse } from '../../types/campaigns';
import type { APIClient } from './client';

declare module './client' {
  interface APIClient {
    listPSACampaigns(): Promise<ListPSACampaignsResponse>;
    psaLink(id: string, psaCampaignRequestId: string): Promise<Campaign>;
    psaPropose(id: string): Promise<PSAProposeResponse>;
    psaProposeCreate(id: string): Promise<PSAProposeCreateResponse>;
    psaPublish(id: string, pushId: string): Promise<PSAPublishResponse>;
    listPSAPushes(): Promise<ListPSAPushesResponse>;
    listPSASubjects(): Promise<PSASubjectsResponse>;
  }
}

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

proto.listPSACampaigns = async function (this: APIClient): Promise<ListPSACampaignsResponse> {
  return this.get<ListPSACampaignsResponse>('/psa-campaigns');
};

proto.psaLink = async function (this: APIClient, id: string, psaCampaignRequestId: string): Promise<Campaign> {
  return this.post<Campaign>(`/campaigns/${id}/psa-link`, { psaCampaignRequestId });
};

proto.psaPropose = async function (this: APIClient, id: string): Promise<PSAProposeResponse> {
  return this.post<PSAProposeResponse>(`/campaigns/${id}/psa-propose`, {});
};

proto.psaProposeCreate = async function (this: APIClient, id: string): Promise<PSAProposeCreateResponse> {
  return this.post<PSAProposeCreateResponse>(`/campaigns/${id}/psa-propose-create`, {});
};

proto.psaPublish = async function (this: APIClient, id: string, pushId: string): Promise<PSAPublishResponse> {
  return this.post<PSAPublishResponse>(`/campaigns/${id}/psa-publish`, { pushId });
};

proto.listPSAPushes = async function (this: APIClient): Promise<ListPSAPushesResponse> {
  return this.get<ListPSAPushesResponse>('/psa-pushes');
};

// Served from the persisted PSA portal catalog (CatalogStore), not a live portal
// call — the main server has no portal session. See docs/psa-harvester.md.
proto.listPSASubjects = async function (this: APIClient): Promise<PSASubjectsResponse> {
  return this.get<PSASubjectsResponse>('/psa/subjects');
};
```

```typescript
// web/src/react/queries/queryKeys.ts — add one entry alongside the existing psa* keys
  psaSubjects: { list: ['psa-subjects', 'list'] as const },
```

```tsx
// web/src/react/ui/SubjectListEditor.tsx
import { useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from '../queries/queryKeys';
import type { SubjectRef } from '../../types/campaigns';

const CATALOG_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;

interface SubjectListEditorProps {
  label: string;
  value: SubjectRef[];
  onChange: (next: SubjectRef[]) => void;
  inputSize?: 'sm';
}

export default function SubjectListEditor({ label, value, onChange, inputSize }: SubjectListEditorProps) {
  const { data } = useQuery({
    queryKey: queryKeys.psaSubjects.list,
    queryFn: () => api.listPSASubjects(),
    staleTime: 5 * 60 * 1000,
  });

  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const catalog = data?.subjects ?? [];
  const fetchedAt = data?.fetchedAt;
  const isStale = !!fetchedAt && Date.now() - new Date(fetchedAt).getTime() > CATALOG_MAX_AGE_MS;

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    const selectedIds = new Set(value.filter(s => s.id !== 0).map(s => s.id));
    return catalog
      .filter(s => s.name.toLowerCase().includes(q) && !selectedIds.has(s.id))
      .slice(0, 20);
  }, [query, catalog, value]);

  function addSubject(subject: SubjectRef) {
    const alreadyPresent = value.some(s =>
      (subject.id !== 0 && s.id === subject.id) ||
      (subject.id === 0 && s.name.toLowerCase() === subject.name.toLowerCase()),
    );
    if (!alreadyPresent) onChange([...value, subject]);
    setQuery('');
    setOpen(false);
  }

  function removeSubject(index: number) {
    onChange(value.filter((_, i) => i !== index));
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const trimmed = query.trim();
    if (!trimmed) return;
    const exact = matches.find(s => s.name.toLowerCase() === trimmed.toLowerCase());
    addSubject(exact ?? { id: 0, name: trimmed });
  }

  const inputPad = inputSize === 'sm' ? 'py-1.5 text-xs' : 'py-2 text-sm';

  return (
    <div className="space-y-2" ref={containerRef}>
      <label className="block text-xs text-[var(--text-muted)] mb-1">{label}</label>
      {catalog.length === 0 && (
        <p className="text-xs text-[var(--text-muted)]">
          Subject catalog not yet harvested — run the harvester.
        </p>
      )}
      {catalog.length > 0 && isStale && (
        <p className="text-xs text-[var(--warning)]">
          Subject catalog is over 7 days old (last fetched {new Date(fetchedAt as string).toLocaleDateString()}).
        </p>
      )}
      <div className="relative">
        <input
          type="text"
          value={query}
          onChange={e => { setQuery(e.target.value); setOpen(true); }}
          onFocus={() => setOpen(true)}
          onKeyDown={handleKeyDown}
          placeholder="Add a subject by name…"
          className={`w-full px-4 ${inputPad} text-[var(--text)] bg-[var(--surface-2)] border border-[var(--surface-2)] rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-[var(--brand-500)]/20 focus:border-[var(--brand-500)]`}
        />
        {open && matches.length > 0 && (
          <div className="absolute z-20 mt-1 w-full max-h-48 overflow-y-auto rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] shadow-lg">
            {matches.map(s => (
              <button
                key={s.id}
                type="button"
                onClick={() => addSubject(s)}
                className="block w-full text-left px-3 py-1.5 text-xs text-[var(--text)] hover:bg-[var(--surface-2)]/60"
              >
                {s.name}
              </button>
            ))}
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-1.5">
        {value.map((s, i) => (
          <span
            key={`${s.id}-${s.name}-${i}`}
            title={`id: ${s.id}`}
            className="inline-flex items-center gap-1 rounded-full bg-[var(--brand-500)]/15 text-[var(--brand-400)] text-xs px-2.5 py-1"
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
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/js/api/psaCampaigns.test.ts src/react/ui/SubjectListEditor.test.tsx`
Expected: PASS (10 tests)

- [ ] **Step 5: Commit**

```bash
git add web/src/types/campaigns/psaCampaign.ts web/src/js/api/psaCampaigns.ts web/src/js/api/psaCampaigns.test.ts web/src/react/queries/queryKeys.ts web/src/react/ui/SubjectListEditor.tsx web/src/react/ui/SubjectListEditor.test.tsx
git commit -m "feat(web): add PSA subject catalog endpoint and subject list editor"
```

- [ ] **Step 6: Write the failing tests for the campaign form and update the existing PSA test fixtures**

```tsx
// web/src/react/ui/CampaignFormFields.test.tsx
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
    targetLanguage: '',
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
  it('changing the language select calls onChange with the token', () => {
    const onChange = renderFields(baseValues());
    fireEvent.change(screen.getByLabelText(/language/i), { target: { value: 'japanese' } });
    expect(onChange).toHaveBeenCalledWith('targetLanguage', 'japanese');
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

```typescript
// web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx:24-46 — replace makeCampaign
function makeCampaign(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: 'c1',
    name: 'Test Campaign',
    sport: 'Pokemon',
    yearRange: '',
    gradeRange: '',
    priceRange: '',
    clConfidence: '',
    buyTermsCLPct: 0.7,
    dailySpendCapCents: 100000,
    targetLanguage: '',
    subjectFilterMode: 'Target',
    subjects: [],
    deniedSpecs: [],
    phase: 'active',
    psaSourcingFeeCents: 0,
    ebayFeePct: 0,
    expectedFillRate: 0,
    psaCampaignRequestId: 'PSA-123',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  } as Campaign;
}
```

```typescript
// web/src/react/pages/campaigns/CampaignsTab.test.tsx:17-39 — replace makeCampaign
function makeCampaign(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: 'c1',
    name: 'Test Campaign',
    sport: 'Pokemon',
    yearRange: '',
    gradeRange: '',
    priceRange: '',
    clConfidence: '',
    buyTermsCLPct: 0.7,
    dailySpendCapCents: 100000,
    targetLanguage: '',
    subjectFilterMode: 'Target',
    subjects: [],
    deniedSpecs: [],
    phase: 'active',
    psaSourcingFeeCents: 0,
    ebayFeePct: 0,
    expectedFillRate: 0,
    psaCampaignRequestId: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  } as Campaign;
}
```

- [ ] **Step 7: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/react/ui/CampaignFormFields.test.tsx src/react/pages/campaign-detail/PSAPublishModal.test.tsx src/react/pages/campaigns/CampaignsTab.test.tsx`
Expected: FAIL — a TypeScript error because `Campaign`/`CreateCampaignInput` still declare `inclusionList`/`exclusionMode` as required fields and the test fixtures no longer supply them (`Property 'inclusionList' is missing`/excess-property errors depending on the `as Campaign` cast resolution), and `CampaignFormFields.test.tsx` fails to resolve `targetLanguage`/`subjectFilterMode` on the exported `CampaignFormValues` type.

- [ ] **Step 8: Implement the type changes, form fields, constants, and paste-import scope reduction**

```typescript
// web/src/types/campaigns/core.ts — top of file and the two interfaces (lines 1-27, 137-151)
/**
 * Core campaign domain types
 */

import type { SubjectRef } from './psaCampaign';

export type Phase = 'pending' | 'active' | 'closed';
export type SaleChannel = 'ebay' | 'website' | 'inperson' | 'tcgplayer' | 'local' | 'other' | 'gamestop' | 'cardshow' | 'doubleholo';

export interface Campaign {
  id: string;
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  targetLanguage: string;
  subjectFilterMode: string;
  subjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
  phase: Phase;
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  expectedFillRate: number;
  psaCampaignRequestId?: string;
  createdAt: string;
  updatedAt: string;
}
```

```typescript
// web/src/types/campaigns/core.ts — CreateCampaignInput (was lines 137-151)
export interface CreateCampaignInput {
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  targetLanguage: string;
  subjectFilterMode: string;
  subjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  phase?: Phase;
}
```

```typescript
// web/src/types/campaigns/portfolio.ts — CampaignSuggestionParams (was line 74 `inclusionList?: string`)
export interface CampaignSuggestionParams {
  name: string;
  yearRange?: string;
  gradeRange?: string;
  priceRange?: string;
  buyTermsCLPct?: number;
  buyTermsCLPctOptimistic?: number;
  dailySpendCapCents?: number;
  // Plain subject names for display only — a suggestion is pre-portal and
  // carries no PSA subject ids to preserve.
  subjects?: string[];
  primaryExit?: string;
}
```

```typescript
// web/src/react/utils/campaignConstants.ts (full file)
import type { Phase, SaleChannel, CreateCampaignInput } from '../../types/campaigns';

export const DEFAULT_SALE_CHANNEL: SaleChannel = 'ebay';

/** Channels available for recording new sales. */
export const activeSaleChannels: SaleChannel[] = ['ebay', 'website', 'inperson'];

/** Maps any channel (including legacy) to its display label. */
export const saleChannelLabels: Record<SaleChannel, string> = {
  ebay: 'eBay',
  website: 'Website',
  inperson: 'In Person',
  // Legacy channels — displayed for historical data
  tcgplayer: 'eBay',
  local: 'In Person',
  other: 'In Person',
  gamestop: 'In Person',
  cardshow: 'In Person',
  doubleholo: 'In Person',
};

/** Normalizes a legacy channel to one of the 3 active channels. */
export function normalizeChannel(ch: SaleChannel): SaleChannel {
  switch (ch) {
    case 'ebay':
    case 'tcgplayer':
      return 'ebay';
    case 'website':
      return 'website';
    default:
      return 'inperson';
  }
}

export const saleChannelColors: Record<SaleChannel, string> = {
  ebay: 'bg-blue-500',
  website: 'bg-indigo-500',
  inperson: 'bg-green-500',
  // Legacy channels map to their normalized color
  tcgplayer: 'bg-blue-500',
  local: 'bg-green-500',
  other: 'bg-green-500',
  gamestop: 'bg-green-500',
  cardshow: 'bg-green-500',
  doubleholo: 'bg-green-500',
};

export const phaseHexColors: Record<Phase, string> = {
  active: '#059669',
  pending: '#f59e0b',
  closed: '#4b5563',
};

export const campaignTabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'transactions', label: 'Transactions' },
  { id: 'tuning', label: 'Tuning' },
  { id: 'settings', label: 'Settings' },
] as const;

export type CampaignTabId = typeof campaignTabs[number]['id'];

export const phaseOptions = [
  { value: 'pending', label: 'Pending' },
  { value: 'active', label: 'Active' },
  { value: 'closed', label: 'Closed' },
] as const;

/** Token stored in `Campaign.targetLanguage` / `CreateCampaignInput.targetLanguage`. */
export const SUBJECT_FILTER_TARGET = 'Target';
export const SUBJECT_FILTER_EXCLUDE = 'Exclude';

export const targetLanguageOptions = [
  { value: '', label: 'Unset' },
  { value: 'english', label: 'English' },
  { value: 'japanese', label: 'Japanese' },
] as const;

export const subjectFilterModeOptions: { value: 'Target' | 'Exclude'; label: string }[] = [
  { value: SUBJECT_FILTER_TARGET, label: 'Target' },
  { value: SUBJECT_FILTER_EXCLUDE, label: 'Exclude' },
];

export const defaultCampaignInput: CreateCampaignInput = {
  name: '',
  sport: 'Pokemon',
  yearRange: '',
  gradeRange: '',
  priceRange: '',
  clConfidence: '',
  buyTermsCLPct: 0.78,
  dailySpendCapCents: 50000,
  targetLanguage: '',
  subjectFilterMode: SUBJECT_FILTER_TARGET,
  subjects: [],
  deniedSpecs: [],
  psaSourcingFeeCents: 300,
  ebayFeePct: 0.1235,
};
```

```tsx
// web/src/react/ui/CampaignFormFields.tsx (full file)
import type { Phase, SubjectRef } from '../../types/campaigns';
import { useId, type ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { phaseOptions, targetLanguageOptions, subjectFilterModeOptions, SUBJECT_FILTER_EXCLUDE } from '../utils/campaignConstants';
import { Input, Select } from '../ui';
import { Segmented } from './Segmented';
import ConfidenceRating from './ConfidenceRating';
import GradeRangeSlider from './GradeRangeSlider';
import SubjectListEditor from './SubjectListEditor';

export interface CampaignFormValues {
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  targetLanguage: string;
  subjectFilterMode: string;
  subjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  expectedFillRate?: number;
  phase?: Phase;
}

interface CampaignFormFieldsProps {
  values: CampaignFormValues;
  onChange: (field: string, value: string | number | boolean | SubjectRef[]) => void;
  inputSize?: 'sm';
  showPhase?: boolean;
  showFees?: boolean;
  nameError?: string;
  onNameBlur?: () => void;
}

function FormSection({
  icon,
  title,
  accent,
  children,
}: {
  icon: ReactNode;
  title: string;
  accent: string;
  children: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-[var(--surface-2)]/60 bg-[var(--surface-0)]/40 p-4 md:p-5 space-y-4">
      <div className="flex items-center gap-2.5 pb-3 border-b border-[var(--surface-2)]/40">
        <div className={`flex items-center justify-center w-7 h-7 rounded-lg ${accent}`}>
          {icon}
        </div>
        <h3 className="text-sm font-semibold text-[var(--text)] tracking-wide">
          {title}
        </h3>
      </div>
      {children}
    </div>
  );
}

function EconomicsSection({
  values, onChange, inputSize, showFees,
}: {
  values: CampaignFormValues;
  onChange: (field: string, value: string | number | boolean) => void;
  inputSize?: 'sm';
  showFees?: boolean;
}) {
  const [buyTermsInput, setBuyTermsInput] = useState(() =>
    values.buyTermsCLPct == null ? '' : String(Math.round(values.buyTermsCLPct * 1000) / 10),
  );
  const [dailySpendCapInput, setDailySpendCapInput] = useState(() =>
    values.dailySpendCapCents == null ? '' : String(values.dailySpendCapCents / 100),
  );
  const [ebayFeeInput, setEbayFeeInput] = useState(() =>
    values.ebayFeePct == null ? '' : String(Math.round(values.ebayFeePct * 10000) / 100),
  );
  const [psaSourcingFeeInput, setPsaSourcingFeeInput] = useState(() =>
    values.psaSourcingFeeCents == null ? '' : String(values.psaSourcingFeeCents / 100),
  );

  // Sync local inputs when parent form values change (e.g. after setForm(campaign))
  useEffect(() => {
    setBuyTermsInput(values.buyTermsCLPct == null ? '' : String(Math.round(values.buyTermsCLPct * 1000) / 10));
    setDailySpendCapInput(values.dailySpendCapCents == null ? '' : String(values.dailySpendCapCents / 100));
    setEbayFeeInput(values.ebayFeePct == null ? '' : String(Math.round(values.ebayFeePct * 10000) / 100));
    setPsaSourcingFeeInput(values.psaSourcingFeeCents == null ? '' : String(values.psaSourcingFeeCents / 100));
  }, [values.buyTermsCLPct, values.dailySpendCapCents, values.ebayFeePct, values.psaSourcingFeeCents]);

  return (
    <FormSection
      title="Economics"
      accent="bg-[var(--warning)]/15 text-[var(--warning)]"
      icon={
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
          <line x1="12" y1="1" x2="12" y2="23" />
          <path d="M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6" />
        </svg>
      }
    >
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Input label="Buy Terms (%)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 78"
          value={buyTermsInput}
          onChange={e => setBuyTermsInput(e.target.value)}
          onBlur={() => { const v = parseFloat(buyTermsInput); onChange('buyTermsCLPct', Number.isNaN(v) ? 0 : v / 100); }} />
        <Input label="Daily Spend Cap ($)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 500"
          value={dailySpendCapInput}
          onChange={e => setDailySpendCapInput(e.target.value)}
          onBlur={() => { const v = parseFloat(dailySpendCapInput); onChange('dailySpendCapCents', Number.isNaN(v) ? 0 : Math.round(v * 100)); }} />
        {showFees && (
          <>
            <Input label="Expected Fill Rate (%)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 80" value={values.expectedFillRate != null ? String(values.expectedFillRate) : ''}
              onChange={e => { const v = parseFloat(e.target.value); onChange('expectedFillRate', Number.isNaN(v) ? 0 : v); }} />
            <Input label="eBay Fee %" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 12.35"
              value={ebayFeeInput}
              onChange={e => setEbayFeeInput(e.target.value)}
              onBlur={() => { const v = parseFloat(ebayFeeInput); onChange('ebayFeePct', Number.isNaN(v) ? 0 : v / 100); }} />
            <Input label="PSA Sourcing Fee ($)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 3.00"
              value={psaSourcingFeeInput}
              onChange={e => setPsaSourcingFeeInput(e.target.value)}
              onBlur={() => { const v = parseFloat(psaSourcingFeeInput); onChange('psaSourcingFeeCents', Number.isNaN(v) ? 0 : Math.round(v * 100)); }} />
          </>
        )}
        <div className="md:col-span-2">
          <ConfidenceRating label="CL Confidence" value={values.clConfidence ? parseFloat(values.clConfidence) : 1}
            onChange={(val) => onChange('clConfidence', String(val))} />
        </div>
      </div>
    </FormSection>
  );
}

export default function CampaignFormFields({
  values, onChange, inputSize, showPhase, showFees, nameError, onNameBlur,
}: CampaignFormFieldsProps) {
  const targetLanguageId = useId();
  return (
    <div className="space-y-4">
      {/* Identity */}
      <FormSection
        title="Identity"
        accent="bg-[var(--brand-500)]/15 text-[var(--brand-400)]"
        icon={
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
            <path d="M20.59 13.41l-7.17 7.17a2 2 0 01-2.83 0L2 12V2h10l8.59 8.59a2 2 0 010 2.82z" />
            <line x1="7" y1="7" x2="7.01" y2="7" />
          </svg>
        }
      >
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input label="Name" required type="text" inputSize={inputSize} value={values.name}
            onChange={e => onChange('name', e.target.value)}
            onBlur={onNameBlur}
            error={nameError} />
          {showPhase && values.phase !== undefined && (
            <Select label="Phase" selectSize={inputSize} value={values.phase}
              onChange={e => onChange('phase', e.target.value)}
              options={[...phaseOptions]} />
          )}
          <Input label="Year Range" type="text" inputSize={inputSize} placeholder="e.g. 1999-2003" value={values.yearRange}
            onChange={e => onChange('yearRange', e.target.value)} />
        </div>
      </FormSection>

      {/* Targeting */}
      <FormSection
        title="Targeting"
        accent="bg-[var(--success)]/15 text-[var(--success)]"
        icon={
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
            <circle cx="12" cy="12" r="10" />
            <circle cx="12" cy="12" r="6" />
            <circle cx="12" cy="12" r="2" />
          </svg>
        }
      >
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="md:col-span-2">
            <GradeRangeSlider label="Grade Range" value={values.gradeRange}
              onChange={(val) => onChange('gradeRange', val)} />
          </div>
          <Input label="Price Range" type="text" inputSize={inputSize} placeholder="e.g. 250-1500" value={values.priceRange}
            onChange={e => onChange('priceRange', e.target.value)} />
          <Select id={targetLanguageId} label="Language" selectSize={inputSize} value={values.targetLanguage}
            onChange={e => onChange('targetLanguage', e.target.value)}
            options={[...targetLanguageOptions]} />
          <div className="space-y-1.5">
            <span className="block text-xs text-[var(--text-muted)] mb-1">Subject Mode</span>
            <Segmented
              ariaLabel="Subject filter mode"
              options={subjectFilterModeOptions}
              value={(values.subjectFilterMode || 'Target') as 'Target' | 'Exclude'}
              onChange={(v) => onChange('subjectFilterMode', v)}
            />
          </div>
          <div className="md:col-span-2">
            <SubjectListEditor
              label={values.subjectFilterMode === SUBJECT_FILTER_EXCLUDE ? 'Excluded Subjects' : 'Targeted Subjects'}
              value={values.subjects}
              onChange={(next) => onChange('subjects', next)}
              inputSize={inputSize}
            />
          </div>
          {values.deniedSpecs.length > 0 && (
            <div className="md:col-span-2 space-y-1.5">
              <span className="block text-xs text-[var(--text-muted)] mb-1">
                Denied Specs (portal-managed — add or remove in the PSA portal)
              </span>
              <div className="flex flex-wrap gap-1.5">
                {values.deniedSpecs.map((spec, i) => (
                  <span key={`${spec.id}-${spec.name}-${i}`} title={`id: ${spec.id}`}
                    className="inline-flex items-center rounded-full bg-[var(--surface-2)] text-[var(--text-muted)] text-xs px-2.5 py-1">
                    {spec.name}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      </FormSection>

      {/* Economics */}
      <EconomicsSection values={values} onChange={onChange} inputSize={inputSize} showFees={showFees} />
    </div>
  );
}
```

```typescript
// web/src/react/pages/CampaignsPage.tsx:43-167 — parser/exporter scope reduction
// Parsed campaign: only fields explicitly present in the text are set.
//
// Targeting (targetLanguage, subjectFilterMode, subjects, deniedSpecs) is
// deliberately NOT part of this bulk paste format. Subjects and denied specs
// carry portal-issued ids that must never be re-derived from a name (see the
// design doc's "ids are copied verbatim, never re-resolved" rule) — a text
// round-trip through paste would reset those ids to 0 and corrupt targeting on
// the next push for every campaign already linked to the portal. Operators
// edit targeting through the form's subject editor instead; this paste format
// stays scoped to scalar economics/range fields, which round-trip safely.
type ParsedCampaign = Partial<CreateCampaignInput> & { name: string };

function parseExportText(text: string): ParsedCampaign[] {
  // Split at campaign boundaries (before "Campaign N — ..." lines) instead of
  // blank lines, so the parser handles both compact and spaced-out clipboard formats.
  // Allow optional leading whitespace (^\s*) so indented clipboard text still splits.
  const blocks = text.trim().split(/(?=^\s*Campaign\s+\d+\s*[-–—])/m);
  const campaigns: ParsedCampaign[] = [];

  for (const block of blocks) {
    const lines = block.split('\n').map(l => l.trim()).filter(Boolean);
    if (lines.length === 0) continue;

    // First line must match "Campaign N — Name" (allow leading whitespace)
    const headerMatch = lines[0].match(/^\s*Campaign\s+\d+\s*[-–—]\s*(.+)$/);
    if (!headerMatch) continue;

    // Only set fields that actually appear in the text. When updating an
    // existing campaign, omitted fields (e.g. PSA Sourcing Fee, eBay Fee)
    // keep their current values instead of being reset to defaults.
    // Conditionally-emitted string filters (year, grade, price, clConfidence)
    // default to '' so an absent line clears the filter — buildExportText
    // only emits these when non-empty.
    const input: ParsedCampaign = {
      name: headerMatch[1].trim(),
      yearRange: '',
      gradeRange: '',
      priceRange: '',
      clConfidence: '',
    };

    for (let i = 1; i < lines.length; i++) {
      const line = lines[i];
      const colonIdx = line.indexOf(':');
      if (colonIdx === -1) continue;

      const key = line.slice(0, colonIdx).trim().toUpperCase();
      const val = line.slice(colonIdx + 1).trim();

      switch (key) {
        case 'SPORT':
          input.sport = val;
          break;
        case 'YEAR':
          input.yearRange = val;
          break;
        case 'PSA GRADE':
          // Normalize single value "7" → "7-7" for backend range validation
          input.gradeRange = /^\d+$/.test(val.trim()) ? `${val.trim()}-${val.trim()}` : val;
          break;
        case 'PRICE': {
          // Reverse formatPriceRange: "$10 to $50" → "10-50", strip commas
          const raw = val.replace(/[$,]/g, '').replace(/\s+to\s+/gi, '-');
          input.priceRange = raw;
          break;
        }
        case 'CL CONFIDENCE':
          input.clConfidence = val;
          break;
        case 'BUY TERMS': {
          // "78.0%" → 0.78
          const pct = parseFloat(val.replace('%', ''));
          if (!isNaN(pct)) input.buyTermsCLPct = pct / 100;
          break;
        }
        case 'DAILY SPEND': {
          // "$500.00" → 50000 cents
          const dollars = parseFloat(val.replace(/[$,]/g, ''));
          if (!isNaN(dollars)) input.dailySpendCapCents = Math.round(dollars * 100);
          break;
        }
        case 'PSA SOURCING FEE': {
          const dollars = parseFloat(val.replace(/[$,]/g, ''));
          if (!isNaN(dollars)) input.psaSourcingFeeCents = Math.round(dollars * 100);
          break;
        }
        case 'EBAY FEE': {
          const pct = parseFloat(val.replace('%', ''));
          if (!isNaN(pct)) input.ebayFeePct = pct / 100;
          break;
        }
        // 'INCLUSION'/'EXCLUSION' lines from the pre-spec-list format are no
        // longer recognized — see the comment on ParsedCampaign above.
      }
    }

    if (input.name) campaigns.push(input);
  }

  return campaigns;
}

function buildExportText(campaigns: Campaign[]): string {
  const active = campaigns.filter(c => c.phase === 'active');
  if (active.length === 0) return '';

  return active.map((c, i) => {
    const lines: string[] = [];
    lines.push(`Campaign ${i + 1} — ${c.name}`);
    lines.push(`SPORT: ${c.sport}`);
    if (c.yearRange) lines.push(`YEAR: ${c.yearRange}`);
    if (c.gradeRange) lines.push(`PSA GRADE: ${c.gradeRange}`);
    if (c.priceRange) lines.push(`Price: ${formatPriceRange(c.priceRange)}`);
    if (c.clConfidence) lines.push(`CL CONFIDENCE: ${c.clConfidence}`);
    lines.push(`BUY TERMS: ${formatPct(c.buyTermsCLPct)}`);
    lines.push(`Daily Spend: ${formatCents(c.dailySpendCapCents)}`);
    lines.push(`PSA Sourcing Fee: ${formatCents(c.psaSourcingFeeCents)}`);
    lines.push(`eBay Fee: ${formatPct(c.ebayFeePct)}`);
    return lines.join('\n');
  }).join('\n\n');
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/react/ui/CampaignFormFields.test.tsx src/react/pages/campaign-detail/PSAPublishModal.test.tsx src/react/pages/campaigns/CampaignsTab.test.tsx src/react/pages/CampaignsPage.psaLink.test.tsx`
Expected: PASS

- [ ] **Step 10: Full frontend gate and commit**

Run: `cd web && npm run build && npm test`
Expected: `npm run build` completes with no TypeScript errors; `npm test` passes for the whole suite.

```bash
git add web/src/types/campaigns/core.ts web/src/types/campaigns/portfolio.ts web/src/react/utils/campaignConstants.ts web/src/react/ui/CampaignFormFields.tsx web/src/react/ui/CampaignFormFields.test.tsx web/src/react/pages/CampaignsPage.tsx web/src/react/pages/campaign-detail/PSAPublishModal.test.tsx web/src/react/pages/campaigns/CampaignsTab.test.tsx
git commit -m "feat(web): replace inclusion/exclusion campaign fields with language, subject-mode, and subject-list axes"
```

---

### Task 12: Verification and docs

**Files:**
- Modify: `docs/SCHEMA.md` (`campaigns` table section, new `psa_portal_catalog` table section)
- Modify: `CLAUDE.md` (migration paragraph under "## Database")
- Modify: `docs/psa-harvester.md` (new "Baseline pull" section)

**Interfaces:**
- Consumes: migrations `000023_campaign_targeting_axes` and `000024_psa_portal_catalog` (frozen contract, authored by an earlier task in this plan); `-baseline-pull` flag on `cmd/psa-harvest` (authored by an earlier task).
- Produces: nothing consumed by later tasks — this is the terminal verification/documentation task.

- [ ] **Step 1: Update `docs/SCHEMA.md` — `campaigns` table**

```markdown
| `inclusion_list` | TEXT | NOT NULL DEFAULT '' | Legacy substring filter. Kept as a derived, write-only mirror of `subjects`/`subject_filter_mode` for one release (nothing reads it) — see migration 000023 |
| `exclusion_mode` | INTEGER | NOT NULL DEFAULT 0 | Legacy polarity flag mirroring `subject_filter_mode == 'Exclude'`. Same write-only status as `inclusion_list` |
| `phase` | TEXT | NOT NULL DEFAULT 'pending' | e.g. 'pending','active','paused','closed' |
| `psa_sourcing_fee_cents` | INTEGER | NOT NULL DEFAULT 300 | Per-card fee ($3.00) |
| `ebay_fee_pct` | REAL | NOT NULL DEFAULT 0.1235 | eBay/TCGPlayer fee percentage |
| `expected_fill_rate` | REAL | NOT NULL DEFAULT 0.0 | Expected % of offers accepted |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `psa_campaign_request_id` | TEXT | | Linked PSA portal campaign request ID; added migration 000017 |
| `target_language` | TEXT | NOT NULL DEFAULT '' | PSA curated spec-list language token: `''` (unset), `'english'`, `'japanese'`; added migration 000023 |
| `subject_filter_mode` | TEXT | NOT NULL DEFAULT 'Target' | `'Target'` (buy only `subjects`) or `'Exclude'` (buy everything except `subjects`); added migration 000023 |
| `subjects` | JSONB | NOT NULL DEFAULT '[]' | `[]TargetSubject` (`{id, name}`) — character subjects this campaign targets or excludes, ids copied verbatim from the portal; added migration 000023 |
| `denied_specs` | JSONB | NOT NULL DEFAULT '[]' | `[]TargetSubject` — individual cards excluded regardless of `subjects`; added migration 000023 |
```

This replaces the existing `inclusion_list`/`exclusion_mode` rows (updating their Notes column to describe the write-only mirror) and inserts four new rows after the existing `psa_campaign_request_id` row, matching the physical column-append order from migration 000023.

- [ ] **Step 2: Add the `psa_portal_catalog` table section to `docs/SCHEMA.md`**

Insert immediately after the existing `### \`psa_campaign_push_queue\`` section (after its closing `---`):

```markdown
### `psa_portal_catalog`
Persisted PSA portal reference data (curated spec lists and character subjects) harvested
by `cmd/psa-harvest`. The main app has no portal session, so it reads this table to build a
pure `psacampaign.Resolver` at translation time instead of calling the portal — see
[docs/psa-harvester.md](../docs/psa-harvester.md#baseline-pull-one-time-targeting-migration).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `kind` | TEXT | PK (composite) | `'spec_lists'` or `'subjects'` |
| `key` | TEXT | PK (composite) | `''` for spec lists; the category id as text (e.g. `'16'` for Pokemon) for subjects |
| `payload` | JSONB | NOT NULL | Serialized `[]SpecListRef` or `[]SubjectRef` |
| `fetched_at` | TIMESTAMPTZ | NOT NULL | When the harvester last wrote this row; `psacampaign.NewCatalogResolver` refuses to build a resolver from a row older than `psacampaign.CatalogMaxAge` (7 days) |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

**Added:** migration 000024

---
```

- [ ] **Step 3: Update the migration paragraph in `CLAUDE.md`**

```markdown
Migration files: `internal/adapters/storage/postgres/migrations/`. `000001_initial_schema`
represents the final-state schema after cutover from SQLite; subsequent migrations are
incremental (Supabase index/RLS fixes, hot-query indexes, `resolved_at` indexes, DH push
plumbing, MM grade-mismatch repair, and the dead-code cleanups dropping
`advisor_cache` (000013) and `psa_exchange_policy` (000014)); most recently
`campaign_targeting_axes` (000023 — four new `campaigns` columns replacing the
inclusion/exclusion model with language, subject-mode, subject-list, and denied-spec axes)
and `psa_portal_catalog` (000024 — persisted PSA spec-list/subject reference data so the
main server can resolve portal identifiers without a portal session).
```

- [ ] **Step 4: Add the baseline-pull operator checklist to `docs/psa-harvester.md`**

Append a new section after the existing "## Campaign sync" section (before "### The three PSA portal endpoints used", or as a new top-level `##` section at the end of the file — append at the end of the file):

```markdown
## Baseline pull (one-time targeting migration)

`cmd/psa-harvest -baseline-pull` performs the one-time copy of live portal targeting
(language, subject list, denied specs) into `campaigns.target_language` /
`subject_filter_mode` / `subjects` / `denied_specs`. It makes **zero portal writes** —
the flag returns before `DrainPushQueue` runs. Run it once, review the report, and only
then resume the normal (non-baseline) scheduled harvest.

```bash
docker run --rm \
  -e PSA_PORTAL_EMAIL="user@example.com" \
  -e PSA_PORTAL_PASSWORD="********" \
  -e ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  -e DATABASE_URL="$DATABASE_URL" \
  slabledger-psa-harvest -baseline-pull
```

### Manual operator checklist

- [ ] Run the baseline pull once and confirm it **exits zero**. A non-zero exit means at
      least one campaign's edit-form fetch failed (`TargetingComplete == false`) and that
      campaign's row was skipped rather than written with blanked-out targeting — re-run
      until clean before trusting the copy.
- [ ] For at least one named, currently-linked campaign, open its edit page in the PSA
      portal UI directly and confirm the pulled `target_language` / `subject_filter_mode` /
      `subjects` / `denied_specs` match what the portal UI shows. This is the check for a
      silently wrong translation, not just a successful fetch.
- [ ] Re-run the baseline a second time against an **unchanged** campaign and confirm the
      diff it reports for that campaign is **empty**. The list comparison is order-insensitive
      by design (ids sorted ascending) specifically so re-running the baseline is idempotent;
      a non-empty diff on an unchanged campaign means list-ordering churn slipped into the
      comparison and needs to be fixed before this is trusted for six active, money-spending
      campaigns.
- [ ] Confirm no portal writes occurred during the baseline: check that every campaign's
      `updatedAt` in the PSA portal UI is unchanged from before the run, and that
      `psa_campaign_push_queue` gained no new rows.

### Deferred: spec discovery

`deniedSpecs` round-trips (pulled, decoded, diffed, and pushed) but there is no UI in
SlabLedger to *discover* a new card to deny — the modal that searches PSA's spec catalog
was never opened during HAR capture, so its request/response shape is unknown. Until a
capture with that modal open is taken, adding a new denial is done by hand in the PSA
portal and picked up on the next pull; this is the one intentional exception to "no direct
data entry in the portal."
```

- [ ] **Step 5: Run the full verification gate**

Run: `go build ./...`
Expected: PASS, no errors.

Run: `go test -race -timeout 10m ./...`
Expected: PASS, all packages green.

Run: `make check`
Expected: PASS — lint, `scripts/check-imports.sh` (domain must not import adapters; `inventory` must not import `psacampaign`; flat-sibling rule holds), `scripts/check-file-size.sh` (no file over 600 lines; `mapper.go`/`matching.go` under the 500-line warn threshold or already split per the design doc's file-size note).

Run: `cd web && npm run build && npm test`
Expected: PASS — no TypeScript errors, full Vitest suite green.

If any command fails, fix the failure in the task that owns the affected file and re-run
this step — do not weaken a test or delete a check to make the gate pass.

- [ ] **Step 6: Commit the docs**

```bash
git add docs/SCHEMA.md CLAUDE.md docs/psa-harvester.md
git commit -m "docs: document campaign targeting axes, psa_portal_catalog, and the baseline-pull operator checklist"
```

