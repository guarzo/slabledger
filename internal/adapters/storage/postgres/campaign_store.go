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
// store is the sole writer of both legacy columns, and nothing left in this
// tree reads them back — the mirror exists solely so a rollback to the
// previous binary, which still reads inclusion_list/exclusion_mode directly,
// sees a database consistent with what it expects.
//
// The mirror is inherently lossy for multi-word subject names: it joins
// names with "," here, and the old binary's own comma-or-whitespace splitter
// would break "Crystal Golem" into the two separate tokens "Crystal" and
// "Golem". This is a pre-existing limitation of the legacy string format
// itself, only reachable during the rollback window, and not something this
// store can fix without changing what the old binary expects.
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

// normalizeTargetSubjects converts a nil slice to an empty one. Defensive on
// read: the NOT NULL DEFAULT '[]'::jsonb column (migration 000024) should
// never hold a literal jsonb null, but marshalTargetSubjects only started
// guarding against that on write after this fix — any row written before it
// landed would unmarshal a stored "null" back into a nil Go slice, which the
// TS Campaign type and its consumers (SubjectListEditor, CampaignFormFields)
// don't expect.
func normalizeTargetSubjects(subjects []inventory.TargetSubject) []inventory.TargetSubject {
	if subjects == nil {
		return []inventory.TargetSubject{}
	}
	return subjects
}

// marshalTargetSubjects marshals a subject list to its JSONB wire form,
// falling back to an empty array on marshal failure (TargetSubject has no
// fields that can fail to marshal, so this is defensive only). A nil slice
// is normalized to an empty one first — json.Marshal(nil) produces the
// literal "null", which the NOT NULL DEFAULT '[]'::jsonb column (migration
// 000023) happily stores as a jsonb null value rather than an empty array,
// and later round-trips back through json.Unmarshal into a nil Go slice.
// Every consumer downstream — Campaign.Subjects/DeniedSpecs' non-nullable
// TS type, CampaignFormFields' deniedSpecs.length check, SubjectListEditor's
// value.map/value.some — expects `[]`, never `null`.
func marshalTargetSubjects(subjects []inventory.TargetSubject) string {
	if subjects == nil {
		subjects = []inventory.TargetSubject{}
	}
	b, err := json.Marshal(subjects)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// normalizeTargetLanguages converts a nil slice to an empty one, mirroring
// normalizeTargetSubjects. Defensive on read for the same reason: the
// NOT NULL DEFAULT '[]'::jsonb column (migration 000024) rejects SQL NULL but
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

func (cs *CampaignStore) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	inclusionList, exclusionMode := deriveLegacyMirror(c)
	subjectFilterMode := normalizeSubjectFilterMode(c.SubjectFilterMode)
	subjectsJSON := marshalTargetSubjects(c.Subjects)
	deniedJSON := marshalTargetSubjects(c.DeniedSpecs)
	languagesJSON := marshalTargetLanguages(c.TargetLanguages)

	query := `
		INSERT INTO campaigns (id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate,
			psa_campaign_request_id, created_at, updated_at,
			target_languages, subject_filter_mode, subjects, denied_specs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22)
	`
	_, err := cs.db.ExecContext(ctx, query,
		c.ID, c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, inclusionList,
		exclusionMode, string(c.Phase), c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate,
		c.PSACampaignRequestID, c.CreatedAt, c.UpdatedAt,
		languagesJSON, subjectFilterMode, subjectsJSON, deniedJSON,
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
			target_languages, subject_filter_mode, subjects, denied_specs
		FROM campaigns WHERE id = $1
	`
	var c inventory.Campaign
	var subjectsJSON, deniedJSON, languagesJSON string
	err := cs.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
		&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
		&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate,
		&c.PSACampaignRequestID, &c.CreatedAt, &c.UpdatedAt,
		&languagesJSON, &c.SubjectFilterMode, &subjectsJSON, &deniedJSON,
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
	if err := json.Unmarshal([]byte(languagesJSON), &c.TargetLanguages); err != nil {
		return nil, fmt.Errorf("unmarshal target languages: %w", err)
	}
	c.Subjects = normalizeTargetSubjects(c.Subjects)
	c.DeniedSpecs = normalizeTargetSubjects(c.DeniedSpecs)
	c.TargetLanguages = normalizeTargetLanguages(c.TargetLanguages)
	c.SubjectFilterMode = normalizeSubjectFilterMode(c.SubjectFilterMode)
	return &c, nil
}

func (cs *CampaignStore) ListCampaigns(ctx context.Context, activeOnly bool) (result []inventory.Campaign, err error) {
	query := `
		SELECT id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate,
			COALESCE(psa_campaign_request_id, ''), created_at, updated_at,
			target_languages, subject_filter_mode, subjects, denied_specs
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
		var subjectsJSON, deniedJSON, languagesJSON string
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
			&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
			&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate,
			&c.PSACampaignRequestID, &c.CreatedAt, &c.UpdatedAt,
			&languagesJSON, &c.SubjectFilterMode, &subjectsJSON, &deniedJSON,
		); err != nil {
			return nil, fmt.Errorf("scan campaign row: %w", err)
		}
		if err := json.Unmarshal([]byte(subjectsJSON), &c.Subjects); err != nil {
			return nil, fmt.Errorf("unmarshal subjects: %w", err)
		}
		if err := json.Unmarshal([]byte(deniedJSON), &c.DeniedSpecs); err != nil {
			return nil, fmt.Errorf("unmarshal denied specs: %w", err)
		}
		if err := json.Unmarshal([]byte(languagesJSON), &c.TargetLanguages); err != nil {
			return nil, fmt.Errorf("unmarshal target languages: %w", err)
		}
		c.Subjects = normalizeTargetSubjects(c.Subjects)
		c.DeniedSpecs = normalizeTargetSubjects(c.DeniedSpecs)
		c.TargetLanguages = normalizeTargetLanguages(c.TargetLanguages)
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
	languagesJSON := marshalTargetLanguages(c.TargetLanguages)

	query := `
		UPDATE campaigns SET name = $1, sport = $2, year_range = $3, grade_range = $4,
			price_range = $5, cl_confidence = $6, buy_terms_cl_pct = $7,
			daily_spend_cap_cents = $8, inclusion_list = $9, exclusion_mode = $10, phase = $11,
			psa_sourcing_fee_cents = $12, ebay_fee_pct = $13, expected_fill_rate = $14,
			psa_campaign_request_id = $15, updated_at = $16,
			target_languages = $17, subject_filter_mode = $18, subjects = $19, denied_specs = $20
		WHERE id = $21
	`
	result, err := cs.db.ExecContext(ctx, query,
		c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, inclusionList,
		exclusionMode, string(c.Phase), c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate,
		c.PSACampaignRequestID, c.UpdatedAt,
		languagesJSON, subjectFilterMode, subjectsJSON, deniedJSON, c.ID,
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
