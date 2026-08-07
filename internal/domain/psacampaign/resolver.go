package psacampaign

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	// ErrLegacySubjectsUnreconciled means a campaign still carries subjects
	// marked by migration 000023's backfill as legacy and never reconciled
	// against the portal. Pushing one would re-resolve a live portal id by
	// name; the operator must run the baseline pull first.
	ErrLegacySubjectsUnreconciled = errors.New(
		"psacampaign: campaign carries legacy unreconciled subjects; run the psa-harvest baseline pull to reconcile them against the portal before pushing")
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
	SpecListIDs(languageTokens []string) ([]string, error)
	SubjectID(name string) (int, error)
}

// languageListNames maps a canonical language token onto the curated portal
// spec list name it resolves to. Matching is case-insensitive against
// SpecListRef.Name. Only english/japanese are known today — the portal offers
// no curated Chinese/Korean list at design time.
var languageListNames = map[string]string{
	"english":  "Pokemon - English Language Only",
	"japanese": "Pokemon - Japanese Language Only",
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
//
// Deduped across tokens: two language tokens should never share a curated
// list, but the portal catalog is harvested data outside our control, and a
// duplicate id here would both double up prepackagedSpecListIds on the wire
// and — since renderStringList sorts but does not dedup — leave the diff
// against the portal's de-duplicated response perpetually non-empty.
func (r *catalogResolver) SpecListIDs(languageTokens []string) ([]string, error) {
	var ids []string
	for _, token := range languageTokens {
		tokenIDs, err := r.specListIDsForToken(token)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", err, token)
		}
		for _, id := range tokenIDs {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
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
