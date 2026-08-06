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
