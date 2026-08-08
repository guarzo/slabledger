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
// id, so the language set (TargetLanguages) and card-level denials
// (DeniedSpecs) have no defined value here and are not evaluated. This is a
// documented reduction versus inventory.PurchaseMatchesCampaign, not an
// oversight: the niche-opportunity leaderboard asks a character-level
// question, and widening CampaignsCovering to carry a language input is
// deferred until something actually asks a language-scoped coverage question.
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
