package scheduler

import (
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

// TestIndexDemand covers the overall/per-era merge. Before SLA-41's review
// pass indexDemand overwrote per character, so with ten eras in
// defaultAnalyticsEras at most one era's bucket survived a run, and the
// character-level totals were whichever era happened to be iterated last.
func TestIndexDemand(t *testing.T) {
	overall := func(name string, cards int) stampedDemandEntry {
		return stampedDemandEntry{
			Entry:      dh.CharacterDemandEntry{CharacterName: name, CardCount: cards, TotalViews: 100},
			ComputedAt: "overall",
		}
	}
	scoped := func(name, era string, cards int) stampedDemandEntry {
		return stampedDemandEntry{
			Entry:      dh.CharacterDemandEntry{CharacterName: name, CardCount: cards, TotalViews: 10},
			ComputedAt: "era",
			Era:        era,
		}
	}

	tests := []struct {
		name           string
		entries        []stampedDemandEntry
		wantEmpty      bool
		wantCards      int
		wantComputedAt string
		wantEras       map[string]int // era -> expected bucket CardCount
	}{
		{
			name:           "overall only",
			entries:        []stampedDemandEntry{overall("Pikachu", 9)},
			wantCards:      9,
			wantComputedAt: "overall",
		},
		{
			name: "every era contributes a bucket",
			entries: []stampedDemandEntry{
				overall("Pikachu", 9),
				scoped("Pikachu", "wotc", 3),
				scoped("Pikachu", "ex", 4),
				scoped("Pikachu", "bw", 2),
			},
			wantCards:      9,
			wantComputedAt: "overall",
			wantEras:       map[string]int{"wotc": 3, "ex": 4, "bw": 2},
		},
		{
			name: "overall arriving after eras still supplies the totals",
			entries: []stampedDemandEntry{
				scoped("Pikachu", "wotc", 3),
				scoped("Pikachu", "ex", 4),
				overall("Pikachu", 9),
			},
			wantCards:      9,
			wantComputedAt: "overall",
			wantEras:       map[string]int{"wotc": 3, "ex": 4},
		},
		{
			name:           "era-only character keeps its own totals",
			entries:        []stampedDemandEntry{scoped("Mewtwo", "wotc", 3)},
			wantCards:      3,
			wantComputedAt: "era",
			wantEras:       map[string]int{"wotc": 3},
		},
		{
			name: "an entry's own by_era wins over the queried era",
			entries: []stampedDemandEntry{{
				Entry: dh.CharacterDemandEntry{
					CharacterName: "Pikachu",
					CardCount:     5,
					ByEra:         map[string]dh.ByEraEntry{"xy": {CardCount: 7}},
				},
				ComputedAt: "era",
				Era:        "wotc",
			}},
			wantCards:      5,
			wantComputedAt: "era",
			wantEras:       map[string]int{"xy": 7},
		},
		{
			name:      "unnamed entries are dropped",
			entries:   []stampedDemandEntry{{Entry: dh.CharacterDemandEntry{CardCount: 1}}},
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := indexDemand(tc.entries)
			if tc.wantEmpty {
				if len(got) != 0 {
					t.Fatalf("indexDemand = %+v; want empty", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("indexDemand = %d characters; want 1", len(got))
			}
			var merged stampedDemandEntry
			for _, v := range got {
				merged = v
			}
			if merged.Entry.CardCount != tc.wantCards {
				t.Errorf("CardCount = %d; want %d", merged.Entry.CardCount, tc.wantCards)
			}
			if merged.ComputedAt != tc.wantComputedAt {
				t.Errorf("ComputedAt = %q; want %q", merged.ComputedAt, tc.wantComputedAt)
			}
			if len(merged.Entry.ByEra) != len(tc.wantEras) {
				t.Fatalf("ByEra = %+v; want %d entries", merged.Entry.ByEra, len(tc.wantEras))
			}
			for era, wantCount := range tc.wantEras {
				bucket, ok := merged.Entry.ByEra[era]
				if !ok {
					t.Errorf("ByEra missing era %q", era)
					continue
				}
				if bucket.CardCount != wantCount {
					t.Errorf("ByEra[%q].CardCount = %d; want %d", era, bucket.CardCount, wantCount)
				}
			}
		})
	}
}

// TestIndexDemand_DoesNotMutateInput guards the detach-on-seed step. The
// entries handed to indexDemand come straight off a DH response, so merging
// into their maps in place would corrupt the caller's data.
func TestIndexDemand_DoesNotMutateInput(t *testing.T) {
	src := map[string]dh.ByEraEntry{"wotc": {CardCount: 3}}
	entries := []stampedDemandEntry{
		{Entry: dh.CharacterDemandEntry{CharacterName: "Pikachu", ByEra: src}, Era: "wotc"},
		{Entry: dh.CharacterDemandEntry{CharacterName: "Pikachu", CardCount: 2}, Era: "ex"},
	}
	got := indexDemand(entries)
	if len(src) != 1 {
		t.Errorf("input ByEra mutated: %+v", src)
	}
	if n := len(got["Pikachu"].Entry.ByEra); n != 2 {
		t.Errorf("merged ByEra = %d entries; want 2", n)
	}
}
