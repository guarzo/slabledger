package showprep

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

func TestWorkerCandidateQualificationAndRetries(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	id := Identity{ProfileID: "profile", Grader: "PSA", Grade: 10}
	complete := Snapshot{Identity: id, Source: "cardladder", Complete: true, AttemptState: "complete", WindowStart: "2026-08-17", WindowEnd: "2026-09-15", RefreshedAt: now}
	for _, tt := range []struct {
		name   string
		change func(*WorkerCandidate)
		state  string
		due    bool
	}{
		{"missing", func(c *WorkerCandidate) { c.Snapshot = nil }, "missing", true},
		{"complete empty", func(*WorkerCandidate) {}, "current", false},
		{"stale", func(c *WorkerCandidate) { c.Snapshot.WindowStart = "2026-08-16"; c.Snapshot.WindowEnd = "2026-09-14" }, "stale", true},
		{"partial", func(c *WorkerCandidate) { c.Snapshot.AttemptState = "partial" }, "failed", true},
		{"invalid records", func(c *WorkerCandidate) { c.Snapshot.Sales = []Sale{{ID: "bad", PriceCents: -1, Date: "2026-09-15"}} }, "failed", true},
		{"invalid source", func(c *WorkerCandidate) { c.Snapshot.Source = "legacy" }, "failed", true},
		{"unresolved", func(c *WorkerCandidate) { c.Identity.ProfileID = "" }, "unresolved", false},
		{"nonfinite grade", func(c *WorkerCandidate) { c.Identity.Grade = math.Inf(1); c.Snapshot = nil }, "unresolved", false},
		{"abandoned charged", func(c *WorkerCandidate) {
			c.Snapshot.AttemptState = "running"
			c.Attempts = 1
			c.RetryWindow = "2026-09-15"
			c.NotBefore = now.Add(time.Minute)
		}, "failed", false},
		{"exhausted", func(c *WorkerCandidate) {
			c.Snapshot.AttemptState = "failed"
			c.Attempts = 3
			c.RetryWindow = "2026-09-15"
		}, "failed", false},
		{"new window budget", func(c *WorkerCandidate) {
			c.Snapshot.AttemptState = "failed"
			c.Attempts = 3
			c.RetryWindow = "2026-09-14"
		}, "failed", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			copy := complete
			c := WorkerCandidate{Identity: id, Cards: 2, Snapshot: &copy}
			tt.change(&c)
			require.Equal(t, tt.state, c.Classification(now))
			_, due := c.Due(now, false)
			require.Equal(t, tt.due, due)
		})
	}
	c := WorkerCandidate{Identity: id, Snapshot: &complete, Attempts: 3, RetryWindow: "2026-09-15"}
	_, due := c.Due(now, true)
	require.False(t, due, "explicit retry never rechecks current zero sales")
	complete.AttemptState = "failed"
	c.NotBefore = now.Add(time.Hour)
	_, due = c.Due(now, true)
	require.True(t, due, "explicit retry can reset failed backoff once")
	complete.AttemptState = "complete"
	_, due = c.Due(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), false)
	require.True(t, due, "UTC midnight expires a less-than-24h snapshot")
}

func TestWorkerCoverageUsesExplicitUnits(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	id := Identity{ProfileID: "one", Grader: "PSA", Grade: 10}
	candidates := []WorkerCandidate{
		{Identity: id, Cards: 3, Snapshot: &Snapshot{Identity: id, Source: "cardladder", Complete: true, AttemptState: "complete", WindowStart: "2026-08-17", WindowEnd: "2026-09-15", RefreshedAt: now}},
		{Identity: Identity{ProfileID: "two", Grader: "PSA", Grade: 10}, Cards: 2},
		{Cards: 4},
	}
	s := workerCoverage(candidates, WorkerState{WorkerRunResult: WorkerRunResult{State: "idle"}}, now)
	require.Equal(t, 2, s.EligibleIdentities)
	require.Equal(t, 1, s.CurrentIdentities)
	require.Equal(t, 1, s.MissingIdentities)
	require.Equal(t, 9, s.EligibleCards)
	require.Equal(t, 3, s.CurrentCards)
	require.Equal(t, 4, s.UnresolvedCards)
}
