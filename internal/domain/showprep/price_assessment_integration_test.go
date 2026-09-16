package showprep

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluateCanonicalRecentPrice(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name      string
		change    func(*Purchase, *Snapshot)
		want      Status
		count     int
		unhealthy bool
	}{
		{"local not DH", func(*Purchase, *Snapshot) {}, Supported, 2, false},
		{"DH ambiguity is only a warning", func(p *Purchase, _ *Snapshot) { p.PriceAssociationUnclear = true }, Supported, 2, false},
		{"cannot borrow DH price", func(p *Purchase, _ *Snapshot) { p.LocalPriceCents = 0 }, NoListedPrice, 2, false},
		{"local without DH", func(p *Purchase, _ *Snapshot) { p.ListedPriceCents = 0 }, Supported, 2, false},
		{"fresh wider median is context only", func(p *Purchase, s *Snapshot) {
			p.ListedPriceCents = 30000
			for i := range s.Sales {
				s.Sales[i].Date = "2026-09-09"
			}
		}, NoRecentComps, 0, false},
		{"deduplicate before selection", func(_ *Purchase, s *Snapshot) { s.Sales[1] = s.Sales[0] }, ThinEvidence, 1, false},
		{"retained facts cannot certify failed source", func(_ *Purchase, s *Snapshot) { s.AttemptState = "failed"; s.AttemptError = "source timeout" }, NeedsReview, 2, true},
		{"no asking still exposes source failure", func(p *Purchase, s *Snapshot) { p.LocalPriceCents = 0; s.AttemptState = "failed" }, NoListedPrice, 2, true},
		{"invalid record excluded", func(_ *Purchase, s *Snapshot) {
			s.Sales = append(s.Sales, Sale{ID: "bad", Date: "2026-09-16", PriceCents: 0})
		}, NeedsReview, 2, true},
		{"future record excluded", func(_ *Purchase, s *Snapshot) {
			s.Sales = append(s.Sales, Sale{ID: "future", Date: "2026-09-17", PriceCents: 90000})
		}, NeedsReview, 2, true},
		{"malformed date excluded", func(_ *Purchase, s *Snapshot) {
			s.Sales = append(s.Sales, Sale{ID: "bad-date", Date: "2026-09-32", PriceCents: 90000})
		}, NeedsReview, 2, true},
		{"missing ID excluded", func(_ *Purchase, s *Snapshot) { s.Sales = append(s.Sales, Sale{Date: "2026-09-16", PriceCents: 90000}) }, NeedsReview, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := Purchase{ID: "p", Grader: "PSA", Grade: 10, ProfileID: "profile", LocalPriceCents: 30000, ListedPriceCents: 90000}
			start, end := Window(now)
			s := &Snapshot{Identity: p.Identity(), Source: "cardladder", Complete: true, AttemptState: "complete", WindowStart: start, WindowEnd: end, RefreshedAt: now,
				Sales: []Sale{{ID: "a", Date: end, PriceCents: 30000}, {ID: "b", Date: end, PriceCents: 30000}}}
			tt.change(&p, s)
			before := Fingerprint(s)
			e := Evaluate(p, s, now)
			require.Equal(t, tt.want, e.Status)
			require.Equal(t, p.ListedPriceCents, e.ListedPriceCents, "DH diagnostics retained")
			require.Equal(t, p.PriceAssociationUnclear, e.PriceAssociationUnclear)
			require.Equal(t, tt.unhealthy, e.EvidenceNeedsReview)
			require.Equal(t, before, Fingerprint(s), "assessment cannot rewrite saved evidence")
			// Check wire names as well as domain values.
			payload, err := json.Marshal(e)
			require.NoError(t, err)
			var wire struct {
				PolicyVersion string              `json:"policyVersion"`
				Recent        RecentPriceEvidence `json:"recent"`
			}
			require.NoError(t, json.Unmarshal(payload, &wire))
			require.Equal(t, PriceAssessmentPolicy, wire.PolicyVersion)
			require.Equal(t, tt.count, wire.Recent.Count)
			require.Equal(t, "2026-09-10", wire.Recent.WindowStart)
			require.Equal(t, "2026-09-16", wire.Recent.WindowEnd)
			if tt.unhealthy || p.LocalPriceCents <= 0 || tt.count == 0 {
				require.Nil(t, wire.Recent.GapPct)
			} else {
				require.NotNil(t, wire.Recent.GapPct)
				require.Zero(t, *wire.Recent.GapPct)
			}
			if tt.count == 2 {
				require.Equal(t, []string{"a", "b"}, wire.Recent.SaleIDs)
			}
			if tt.name == "fresh wider median is context only" {
				require.Equal(t, 2, e.CompCount)
				require.Equal(t, 30000, e.MedianCents)
			}
		})
	}
}

func TestEvaluateRecentContradictionAndWideMedianOverflow(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name   string
		asking int
		prices []int
		want   Status
		median int
	}{
		{"newest day contradiction", 30000, []int{20000, 30000, 30000}, MixedEvidence, 30000},
		{"maximum cents context", int(^uint(0) >> 1), []int{int(^uint(0) >> 1), int(^uint(0) >> 1)}, Supported, int(^uint(0) >> 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := Purchase{ID: "p", Grader: "PSA", Grade: 10, ProfileID: "profile", LocalPriceCents: tt.asking, ListedPriceCents: tt.asking}
			start, end := Window(now)
			s := &Snapshot{Identity: p.Identity(), Source: "cardladder", Complete: true, AttemptState: "complete", WindowStart: start, WindowEnd: end, RefreshedAt: now}
			for i, price := range tt.prices {
				s.Sales = append(s.Sales, Sale{ID: string(rune('a' + i)), Date: end, PriceCents: price})
			}
			e := Evaluate(p, s, now)
			require.Equal(t, tt.want, e.Status)
			require.Equal(t, tt.median, e.MedianCents)
		})
	}
}
