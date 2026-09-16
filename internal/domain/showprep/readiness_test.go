package showprep

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func readinessFixture() (Purchase, *Snapshot, time.Time) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	p := Purchase{ID: "p", Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "active", Grader: "PSA", Grade: 10, ProfileID: "psa-1", ListedPriceCents: 30000, LocalPriceCents: 30000}
	s := &Snapshot{Identity: p.Identity(), Source: "cardladder", Generation: 1, Complete: true, WindowStart: "2026-08-16", WindowEnd: "2026-09-14", RefreshedAt: now, Attempt: 1, AttemptState: "complete", AttemptStartedAt: now.Add(-time.Minute), Sales: []Sale{{ID: "a", Date: "2026-09-14", PriceCents: 30000}, {ID: "b", Date: "2026-09-14", PriceCents: 30000}}}
	return p, s, now
}

func readinessWire(t *testing.T, e Evaluation) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	r, ok := wire["readiness"].(map[string]any)
	require.True(t, ok, "evaluation must carry readiness")
	return r
}

func TestReadinessLifecycle(t *testing.T) {
	for _, tt := range []struct {
		name, state, eligibility, expires, retry string
		change                                   func(*Purchase, **Snapshot, *time.Time)
	}{
		{"cold", "not_checked", "needed", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { *s = nil }},
		{"current", "current", "not_needed", "2026-09-15T00:00:00Z", "", nil},
		{"complete empty", "current", "not_needed", "2026-09-15T00:00:00Z", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Sales = nil }},
		{"no price", "current", "not_needed", "2026-09-15T00:00:00Z", "", func(p *Purchase, _ **Snapshot, _ *time.Time) { p.LocalPriceCents = 0 }},
		{"price association unclear", "current", "not_needed", "2026-09-15T00:00:00Z", "", func(p *Purchase, _ **Snapshot, _ *time.Time) { p.PriceAssociationUnclear = true }},
		{"age before bound", "current", "not_needed", "2026-09-14T12:00:00.000000001Z", "", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).RefreshedAt = now.Add(-24*time.Hour + time.Nanosecond)
		}},
		{"age at inclusive bound", "current", "not_needed", "2026-09-14T12:00:00Z", "", func(_ *Purchase, s **Snapshot, now *time.Time) { (*s).RefreshedAt = now.Add(-24 * time.Hour) }},
		{"age past bound", "stale", "needed", "2026-09-14T11:59:59.999999999Z", "", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).RefreshedAt = now.Add(-24*time.Hour - time.Nanosecond)
		}},
		{"before midnight", "current", "not_needed", "2026-09-15T00:00:00Z", "", func(_ *Purchase, _ **Snapshot, now *time.Time) { *now = now.Add(12*time.Hour - time.Nanosecond) }},
		{"at midnight", "stale", "needed", "2026-09-15T00:00:00Z", "", func(_ *Purchase, _ **Snapshot, now *time.Time) { *now = now.Add(12 * time.Hour) }},
		{"old window", "stale", "needed", "2026-09-14T00:00:00Z", "", func(_ *Purchase, s **Snapshot, _ *time.Time) {
			(*s).WindowStart = "2026-08-15"
			(*s).WindowEnd = "2026-09-13"
		}},
		{"running without payload", "running", "wait", "", "2026-09-14T12:01:00Z", func(p *Purchase, s **Snapshot, now *time.Time) {
			*s = &Snapshot{Identity: p.Identity(), Attempt: 1, AttemptState: "running", AttemptStartedAt: now.Add(-time.Minute)}
		}},
		{"running retained", "running", "wait", "", "2026-09-14T12:01:00Z", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).AttemptState = "running" }},
		{"running just before bound", "running", "wait", "", "2026-09-14T12:00:00.000000001Z", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).AttemptState = "running"
			(*s).AttemptStartedAt = now.Add(-120*time.Second + time.Nanosecond)
		}},
		{"running at bound", "interrupted", "retry_only", "", "2026-09-14T12:00:00Z", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).AttemptState = "running"
			(*s).AttemptStartedAt = now.Add(-120 * time.Second)
		}},
		{"running past bound", "interrupted", "retry_only", "", "2026-09-14T11:59:00Z", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).AttemptState = "running"
			(*s).AttemptStartedAt = now.Add(-3 * time.Minute)
		}},
		{"missing attempt start", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) {
			(*s).AttemptState = "running"
			(*s).AttemptStartedAt = time.Time{}
		}},
		{"future attempt start", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).AttemptState = "running"
			(*s).AttemptStartedAt = now.Add(time.Nanosecond)
		}},
		{"failed retained", "failed", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).AttemptState = "failed" }},
		{"failed no price", "failed", "retry_only", "", "", func(p *Purchase, s **Snapshot, _ *time.Time) { p.LocalPriceCents = 0; (*s).AttemptState = "failed" }},
		{"failure text is not a read outcome", "failed", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) {
			(*s).AttemptState = "failed"
			(*s).AttemptError = "Evidence storage unavailable"
		}},
		{"partial retained", "failed", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).AttemptState = "partial" }},
		{"failed stale", "failed", "retry_only", "", "", func(_ *Purchase, s **Snapshot, now *time.Time) {
			(*s).AttemptState = "failed"
			(*s).RefreshedAt = now.Add(-25 * time.Hour)
		}},
		{"incomplete window", "failed", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Complete = false }},
		{"unknown attempt", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).AttemptState = "unknown" }},
		{"missing provenance", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Source = "legacy" }},
		{"wrong identity before running", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) {
			(*s).Identity.Grader = "BGS"
			(*s).AttemptState = "running"
		}},
		{"invalid price", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Sales[0].PriceCents = 0 }},
		{"invalid sale date", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Sales[0].Date = "yesterday" }},
		{"future sale", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Sales[0].Date = "2026-09-15" }},
		{"missing sale id", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).Sales[0].ID = "" }},
		{"missing refresh", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).RefreshedAt = time.Time{} }},
		{"future refresh", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, now *time.Time) { (*s).RefreshedAt = now.Add(time.Nanosecond) }},
		{"malformed window", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).WindowEnd = "yesterday" }},
		{"wrong window span", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) { (*s).WindowStart = "2026-09-01" }},
		{"future window", "invalid", "retry_only", "", "", func(_ *Purchase, s **Snapshot, _ *time.Time) {
			(*s).WindowStart = "2026-08-17"
			(*s).WindowEnd = "2026-09-15"
		}},
		{"invalid identity before snapshot", "unavailable", "unavailable", "", "", func(p *Purchase, _ **Snapshot, _ *time.Time) { p.ProfileID = "" }},
		{"invalid identity cold", "unavailable", "unavailable", "", "", func(p *Purchase, s **Snapshot, _ *time.Time) { p.Grader = ""; *s = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, s, now := readinessFixture()
			if tt.change != nil {
				tt.change(&p, &s, &now)
			}
			before := Fingerprint(s)
			e := Evaluate(p, s, now)
			key := p.Identity().Key()
			if !p.Identity().Valid() {
				key = ""
			}
			require.Equal(t, map[string]any{"state": tt.state, "refreshEligibility": tt.eligibility, "identityKey": key, "expiresAt": tt.expires, "retryAt": tt.retry}, readinessWire(t, e))
			require.Equal(t, before, Fingerprint(s), "readiness must never mutate attempts/payload")
			if p.LocalPriceCents == 0 {
				require.Equal(t, NoListedPrice, e.Status)
			}
			if tt.name == "complete empty" {
				require.Equal(t, NoRecentComps, e.Status)
				require.False(t, e.EvidenceNeedsReview)
			}
			if tt.name == "failed retained" {
				require.Equal(t, 2, e.CompCount)
				require.Equal(t, 30000, e.MedianCents)
				require.True(t, e.EvidenceNeedsReview)
			}
		})
	}
}

func TestReadinessUsesNormalizedIdentityAndUTC(t *testing.T) {
	p, s, now := readinessFixture()
	baseline := readinessWire(t, Evaluate(p, s, now))
	p.ID = "another-purchase"
	p.Grader = " psa "
	require.Equal(t, baseline, readinessWire(t, Evaluate(p, s, now.In(time.FixedZone("west", -4*3600)))))
}

func TestReadinessTransitionDoesNotChangeVersions(t *testing.T) {
	p, s, now := readinessFixture()
	s.AttemptState = "running"
	before := Evaluate(p, s, now)
	after := Evaluate(p, s, now.Add(time.Minute))
	require.Equal(t, "running", readinessWire(t, before)["state"])
	require.Equal(t, "interrupted", readinessWire(t, after)["state"])
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, before.EvidenceVersion, after.EvidenceVersion)
}

func TestReadinessWindowRolloverStillChangesBusinessVersion(t *testing.T) {
	p, s, now := readinessFixture()
	before := Evaluate(p, s, now)
	after := Evaluate(p, s, now.Add(12*time.Hour))
	require.Equal(t, Supported, before.Status)
	require.Equal(t, NeedsReview, after.Status)
	require.NotEqual(t, before.Version, after.Version)
	require.Equal(t, before.EvidenceVersion, after.EvidenceVersion)
}

// Assessment policy changes business hashes; evidence hashes stay unchanged.
// These goldens continue to guard the wire projection with readiness excluded.
func TestReadinessAssessmentVersionCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name, version, evidence string
		change                  func(*Purchase, **Snapshot)
	}{
		{"healthy", "c443f3ab122a4596e046e21e26e312dc76c7d04957959930cb63a93cc19888b8", "595f7f7a0467ebf371176c2582fa77082040c2573f4c76572367b266b85f1f37", nil},
		{"cold", "790471a0a7fcc418e176dcc70c265aad83dd458d0cd664294ca80ee651dd3215", "", func(_ *Purchase, s **Snapshot) { *s = nil }},
		{"failed retained", "b6f6afce41a063e2cd728df0051beac812f1ecfbca622cd54a17c4137d12901f", "48b90e985519ab26f04d37026ae23b8e95c4132653e4e4e96e19d1daaf3bdfb7", func(_ *Purchase, s **Snapshot) { (*s).AttemptState = "failed" }},
		{"unavailable snapshot read", "5ad0c4fb129ea6e003f98aeee7f45016169dc318c871cb2906d4c54c0902de58", "fcdc1fb5b4cadc3df2da7670b8391b0930eac8aa99c9d6479eceffaf6e68e9d2", func(p *Purchase, s **Snapshot) {
			*s = &Snapshot{Identity: p.Identity(), AttemptError: "Evidence storage unavailable"}
		}},
		{"unavailable purchase read", "990cc591b1d732166a200d6776f45f1b45350ff6fc0e7921e8321b953d0ed4c9", "", func(p *Purchase, s **Snapshot) { *p = Purchase{ID: "p"}; *s = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, s, now := readinessFixture()
			if tt.change != nil {
				tt.change(&p, &s)
			}
			e := Evaluate(p, s, now)
			require.Equal(t, tt.version, e.Version)
			require.Equal(t, tt.evidence, e.EvidenceVersion)
		})
	}
}
