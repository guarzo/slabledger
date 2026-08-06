package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignStore_CampaignCRUD(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	repo := NewCampaignStore(db.DB, logger)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	c := &inventory.Campaign{
		ID:                   "camp-1",
		Name:                 "Vintage Core PSA 8-9",
		Sport:                "Pokemon",
		YearRange:            "1999-2003",
		GradeRange:           "8-9",
		PriceRange:           "250-1500",
		CLConfidence:         "3-4",
		BuyTermsCLPct:        0.80,
		DailySpendCapCents:   150000,
		InclusionList:        "charizard pikachu blastoise",
		Phase:                inventory.PhasePending,
		PSASourcingFeeCents:  300,
		EbayFeePct:           0.1235,
		PSACampaignRequestID: "psa-req-123",
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	err := repo.CreateCampaign(ctx, c)
	require.NoError(t, err)

	got, err := repo.GetCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, c.Name, got.Name)
	assert.Equal(t, c.Sport, got.Sport)
	assert.Equal(t, c.BuyTermsCLPct, got.BuyTermsCLPct)
	assert.Equal(t, c.Phase, got.Phase)
	// ExclusionMode/InclusionList are now a legacy mirror derived from
	// Subjects/SubjectFilterMode by the store, not from whatever the caller
	// set directly (see TestCampaignStore_TargetingAxesRoundTrip). No
	// Subjects are set here, so the mirror derives to Target/false/"" —
	// the caller-supplied InclusionList above is discarded, not stored.
	assert.Equal(t, false, got.ExclusionMode)
	assert.Equal(t, "", got.InclusionList)
	assert.Equal(t, "psa-req-123", got.PSACampaignRequestID)

	list, err := repo.ListCampaigns(ctx, false)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "psa-req-123", list[0].PSACampaignRequestID)

	c.Phase = inventory.PhaseActive
	c.UpdatedAt = time.Now().UTC()
	c.PSACampaignRequestID = "psa-req-456"
	err = repo.UpdateCampaign(ctx, c)
	require.NoError(t, err)

	got, err = repo.GetCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, inventory.PhaseActive, got.Phase)
	assert.Equal(t, "psa-req-456", got.PSACampaignRequestID)

	_, err = repo.GetCampaign(ctx, "nonexistent")
	assert.ErrorIs(t, err, inventory.ErrCampaignNotFound)
}

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

	// Open-net case: an empty TargetLanguage is a legitimate, unfiltered
	// campaign, not an error — and a nil Subjects/DeniedSpecs slice must
	// round-trip through ListCampaigns as well as GetCampaign.
	c2 := &inventory.Campaign{
		ID:            "camp-axes-open",
		Name:          "Unlinked Legacy Campaign",
		Sport:         "Pokemon",
		BuyTermsCLPct: 0.80,
		Phase:         inventory.PhaseActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, repo.CreateCampaign(ctx, c2))

	got2, err := repo.GetCampaign(ctx, "camp-axes-open")
	require.NoError(t, err)
	assert.Equal(t, "", got2.TargetLanguage)
	assert.Equal(t, inventory.SubjectFilterTarget, got2.SubjectFilterMode)
	// assert.Empty accepts both nil and []; a nil slice here would marshal to
	// JSON null over the API despite the non-nullable TS Campaign type, so pin
	// the exact non-nil empty value rather than just "empty".
	assert.Equal(t, []inventory.TargetSubject{}, got2.Subjects)
	assert.Equal(t, []inventory.TargetSubject{}, got2.DeniedSpecs)

	list, err := repo.ListCampaigns(ctx, false)
	require.NoError(t, err)
	var listed *inventory.Campaign
	for i := range list {
		if list[i].ID == "camp-axes-open" {
			listed = &list[i]
		}
	}
	require.NotNil(t, listed, "camp-axes-open must appear in ListCampaigns")
	assert.Equal(t, "", listed.TargetLanguage)
	assert.Equal(t, []inventory.TargetSubject{}, listed.Subjects)
	assert.Equal(t, []inventory.TargetSubject{}, listed.DeniedSpecs)
}

// TestMarshalTargetSubjects_NilEmitsEmptyArray is a fast, DB-free pin on the
// exact wire form: a nil slice must marshal to the JSON array literal "[]",
// never the JSON null literal "null". json.Marshal(nil slice) alone produces
// "null" — this asserts the function's own normalization, not the stdlib's
// default behavior, so a regression here fails immediately without needing
// POSTGRES_TEST_URL / `make test-postgres`.
func TestMarshalTargetSubjects_NilEmitsEmptyArray(t *testing.T) {
	tests := []struct {
		name     string
		subjects []inventory.TargetSubject
		want     string
	}{
		{name: "nil slice", subjects: nil, want: "[]"},
		{name: "empty slice", subjects: []inventory.TargetSubject{}, want: "[]"},
		{
			name:     "populated slice",
			subjects: []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}},
			want:     `[{"id":22210,"name":"Machamp"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marshalTargetSubjects(tt.subjects)
			assert.Equal(t, tt.want, got)
		})
	}
}
