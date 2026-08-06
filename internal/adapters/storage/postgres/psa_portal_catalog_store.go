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
