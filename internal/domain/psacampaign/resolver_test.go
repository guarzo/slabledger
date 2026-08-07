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
		name           string
		languageTokens []string
		wantIDs        []string
		wantErr        error
	}{
		{"japanese resolves", []string{"japanese"}, []string{"fixture-uuid-japanese-pokemon"}, nil},
		{"english resolves, disabled duplicate skipped", []string{"english"}, []string{"fixture-uuid-english-pokemon"}, nil},
		{"both tokens resolve to the union of both ids, not just the first", []string{"english", "japanese"}, []string{"fixture-uuid-english-pokemon", "fixture-uuid-japanese-pokemon"}, nil},
		{"unknown token", []string{"korean"}, nil, ErrUnknownSpecList},
		{"empty token", []string{""}, nil, ErrUnknownSpecList},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewCatalogResolver(lists, nil, fetchedAt, now)
			if err != nil {
				t.Fatalf("NewCatalogResolver() unexpected error = %v", err)
			}
			ids, err := r.SpecListIDs(tt.languageTokens)
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
		if _, err := r.SpecListIDs([]string{"japanese"}); !errors.Is(err, ErrUnknownSpecList) {
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
