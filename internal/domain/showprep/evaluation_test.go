package showprep

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name   string
		prices []int
		listed int
		change func(*Purchase, *Snapshot)
		want   Status
	}{
		{"exact threshold", []int{27000, 27000}, 30000, nil, Supported},
		{"half cent below", []int{26999, 27000}, 30000, nil, BelowTarget},
		{"above listing", []int{40000}, 30000, nil, ThinEvidence},
		{"one low", []int{20000}, 30000, nil, BelowTarget},
		{"odd median", []int{1, 27000, 40000}, 30000, nil, Supported},
		{"empty complete", nil, 30000, nil, NoRecentComps},
		{"no listing", nil, 0, nil, NoListedPrice},
		{"partial", []int{40000, 40000}, 30000, func(_ *Purchase, s *Snapshot) { s.Complete = false }, NeedsReview},
		{"stale", nil, 30000, func(_ *Purchase, s *Snapshot) { s.RefreshedAt = now.Add(-25 * time.Hour) }, NeedsReview},
		{"rollover", nil, 30000, func(_ *Purchase, s *Snapshot) { s.WindowEnd = "2026-09-13" }, NeedsReview},
		{"failed recheck", []int{40000, 40000}, 30000, func(_ *Purchase, s *Snapshot) { s.AttemptState = "failed"; s.AttemptError = "source timeout" }, NeedsReview},
		{"cross grader collision", []int{40000, 40000}, 30000, func(p *Purchase, _ *Snapshot) { p.PriceAssociationUnclear = true }, NeedsReview},
		{"unknown source", nil, 30000, func(_ *Purchase, s *Snapshot) { s.Source = "legacy" }, NeedsReview},
		{"wrong identity", nil, 30000, func(_ *Purchase, s *Snapshot) { s.Identity.Grader = "BGS" }, NeedsReview},
		{"future date", []int{40000}, 30000, func(_ *Purchase, s *Snapshot) { s.Sales[0].Date = "2026-09-15" }, NeedsReview},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := Purchase{ID: "p1", CardName: "Card", CertNumber: "123", Grader: "PSA", Grade: 10, ProfileID: "psa-1", Known: true, Exists: true, CampaignExists: true, Phase: "pending", Received: true, ListedPriceCents: tt.listed}
			s := Snapshot{Identity: p.Identity(), Source: "cardladder", Generation: 1, Complete: true, WindowStart: "2026-08-16", WindowEnd: "2026-09-14", RefreshedAt: now, AttemptState: "complete"}
			for i, price := range tt.prices {
				s.Sales = append(s.Sales, Sale{ID: string(rune('a' + i)), Date: "2026-09-14", PriceCents: price})
			}
			if tt.change != nil {
				tt.change(&p, &s)
			}
			e := Evaluate(p, &s, now)
			require.Equal(t, tt.want, e.Status)
			require.True(t, e.CanPack)
			require.Equal(t, e.Version, Evaluate(p, &s, now.Add(time.Minute)).Version, "reading time is not a version")
		})
	}
	p := Purchase{ID: "missing", ListedPriceCents: 30000}
	require.Equal(t, NeedsReview, Evaluate(p, nil, now).Status)
}

func TestEvidenceHealthIndependentOfListedPrice(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name   string
		change func(*Purchase, **Snapshot)
		reason string
	}{
		{"healthy", func(*Purchase, **Snapshot) {}, ""},
		{"healthy empty", func(_ *Purchase, s **Snapshot) { (*s).Sales = nil }, ""},
		{"healthy with ambiguous price", func(p *Purchase, _ **Snapshot) { p.PriceAssociationUnclear = true }, ""},
		{"unresolved identity", func(p *Purchase, _ **Snapshot) { p.ProfileID = "" }, "Comparable identity unresolved"},
		{"missing", func(_ *Purchase, s **Snapshot) { *s = nil }, "No verified CardLadder evidence"},
		{"unverified source", func(_ *Purchase, s **Snapshot) { (*s).Source = "legacy" }, "No verified CardLadder source provenance"},
		{"identity mismatch", func(_ *Purchase, s **Snapshot) { (*s).Identity.Grader = "BGS" }, "Comparable identity mismatch"},
		{"failed retained sales", func(_ *Purchase, s **Snapshot) {
			(*s).AttemptState = "failed"
			(*s).AttemptError = "CardLadder source timeout"
		}, "CardLadder source timeout"},
		{"ambiguous price and failed evidence", func(p *Purchase, s **Snapshot) {
			p.PriceAssociationUnclear = true
			(*s).AttemptState = "failed"
			(*s).AttemptError = "CardLadder refresh failed"
		}, "CardLadder refresh failed"},
		{"running", func(_ *Purchase, s **Snapshot) { (*s).AttemptState = "running" }, "Refresh incomplete"},
		{"partial", func(_ *Purchase, s **Snapshot) { (*s).Complete = false }, "Incomplete source window"},
		{"invalid record", func(_ *Purchase, s **Snapshot) { (*s).Sales[0].PriceCents = 0 }, "Invalid comparable records"},
		{"window rollover", func(_ *Purchase, s **Snapshot) { (*s).WindowEnd = "2026-09-13" }, "Evidence does not cover current 30-day window"},
		{"stale", func(_ *Purchase, s **Snapshot) { (*s).RefreshedAt = now.Add(-25 * time.Hour) }, "Evidence is stale"},
	} {
		for _, listed := range []int{0, 30000} {
			t.Run(tt.name+"/listed="+strconv.Itoa(listed), func(t *testing.T) {
				p := Purchase{ID: "p", Grader: "PSA", Grade: 10, ProfileID: "psa-1", ListedPriceCents: listed}
				s := &Snapshot{Identity: p.Identity(), Source: "cardladder", Complete: true, WindowStart: "2026-08-16", WindowEnd: "2026-09-14", RefreshedAt: now, AttemptState: "complete", Sales: []Sale{{ID: "a", Date: "2026-09-14", PriceCents: 30000}, {ID: "b", Date: "2026-09-14", PriceCents: 30000}}}
				tt.change(&p, &s)
				e := Evaluate(p, s, now)
				// Assert the required wire fields, including false/empty healthy values.
				encoded, err := json.Marshal(e)
				require.NoError(t, err)
				var wire map[string]any
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Equal(t, tt.reason != "", wire["evidenceNeedsReview"])
				require.Equal(t, tt.reason, wire["evidenceReason"])
				if listed == 0 {
					require.Equal(t, NoListedPrice, e.Status)
					require.Equal(t, "No positive DH listed price", e.Reason)
				} else if p.PriceAssociationUnclear {
					require.Equal(t, NeedsReview, e.Status)
					require.Equal(t, "DH price association unclear", e.Reason)
				} else if tt.reason != "" {
					require.Equal(t, NeedsReview, e.Status)
					require.Equal(t, tt.reason, e.Reason)
				}
			})
		}
	}
}

func TestShowPrepVersionsTrackObservedInputs(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	p := Purchase{ID: "p", Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "active", Grader: "PSA", Grade: 10, ProfileID: "psa-1", ListedPriceCents: 30000}
	snapshot := Snapshot{Identity: p.Identity(), Source: "cardladder", Generation: 1, Complete: true, WindowStart: "2026-08-16", WindowEnd: "2026-09-14", RefreshedAt: now, AttemptState: "complete", Sales: []Sale{{ID: "a", Date: "2026-09-14", PriceCents: 30000}, {ID: "b", Date: "2026-09-14", PriceCents: 30000}}}
	baseline := Evaluate(p, &snapshot, now)
	for _, tt := range []struct {
		name   string
		change func(*Purchase, *Snapshot)
	}{
		{"listed price", func(p *Purchase, _ *Snapshot) { p.ListedPriceCents++ }},
		{"local price", func(p *Purchase, _ *Snapshot) { p.LocalPriceCents = 40000 }},
		{"availability", func(p *Purchase, _ *Snapshot) { p.Refunded = true }},
		{"immutable identity", func(p *Purchase, _ *Snapshot) { p.CertNumber = "changed" }},
		{"durable hold", func(p *Purchase, _ *Snapshot) { p.PriceAssociationUnclear = true }},
		{"evidence generation", func(_ *Purchase, s *Snapshot) { s.Generation++ }},
		{"coverage", func(_ *Purchase, s *Snapshot) { s.WindowEnd = "2026-09-13" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			purchase, changed := p, snapshot
			tt.change(&purchase, &changed)
			require.NotEqual(t, baseline.Version, Evaluate(purchase, &changed, now).Version)
		})
	}
	require.Equal(t, baseline.Version, Evaluate(p, &snapshot, now.Add(time.Minute)).Version)
}

func TestShowPrepListingTimestampsAreUTCOrUnknown(t *testing.T) {
	for _, tt := range []struct{ input, want string }{{"2026-09-14T08:00:00-04:00", "2026-09-14T12:00:00Z"}, {"", ""}, {"invalid", ""}} {
		t.Run(tt.input, func(t *testing.T) {
			e := Evaluate(Purchase{ListingSyncedAt: tt.input}, nil, time.Now())
			require.Equal(t, tt.want, e.ListingSyncedAt)
		})
	}
}

func TestAvailability(t *testing.T) {
	for _, tt := range []struct {
		name      string
		change    func(*Purchase)
		want      Availability
		add, pack bool
	}{
		{"ready pending", func(*Purchase) {}, Ready, true, true},
		{"unknown", func(p *Purchase) { p.Known = false }, Unknown, false, false},
		{"removed", func(p *Purchase) { p.Exists = false; p.Sold = true }, Removed, false, false},
		{"campaign removed", func(p *Purchase) { p.CampaignExists = false }, Removed, false, false},
		{"sold wins", func(p *Purchase) { p.Sold = true; p.Refunded = true }, Sold, false, false},
		{"refunded wins", func(p *Purchase) { p.Refunded = true; p.Phase = "closed" }, Refunded, false, false},
		{"closed wins", func(p *Purchase) { p.Phase = "closed"; p.Received = false }, CampaignClosed, false, false},
		{"not received", func(p *Purchase) { p.Received = false }, NotReceived, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := Purchase{Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "pending"}
			tt.change(&p)
			a := AvailabilityOf(p)
			require.Equal(t, tt.want, a)
			require.Equal(t, tt.add, a.CanAdd())
			require.Equal(t, tt.pack, a.CanPack())
		})
	}
}
