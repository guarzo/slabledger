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
