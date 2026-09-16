package showprep

import (
	"math"
	"time"
)

func (c WorkerCandidate) Classification(now time.Time) string {
	if !c.Identity.Valid() || math.IsInf(c.Identity.Grade, 0) || math.IsNaN(c.Identity.Grade) {
		return "unresolved"
	}
	if c.Snapshot == nil {
		return "missing"
	}
	p := Purchase{ProfileID: c.Identity.ProfileID, Grader: c.Identity.Grader, Grade: c.Identity.Grade}
	switch Evaluate(p, c.Snapshot, now).Readiness.State {
	case ReadinessCurrent:
		return "current"
	case ReadinessStale:
		return "stale"
	default:
		return "failed"
	}
}

// Due supplies an oldest-due ordering key as well as permission. Missing work
// sorts first; charging its start moves it behind never-attempted peers even if
// the process dies. The next UTC window renews the automatic budget.
func (c WorkerCandidate) Due(now time.Time, retryFailed bool) (time.Time, bool) {
	state := c.Classification(now)
	if state == "unresolved" || state == "current" {
		return time.Time{}, false
	}
	if retryFailed && state == "failed" {
		return c.NotBefore, true
	}
	_, window := Window(now)
	if c.RetryWindow == window && c.Attempts > 0 {
		if c.Attempts >= 3 {
			return now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour), false
		}
		return c.NotBefore, !now.Before(c.NotBefore)
	}
	if c.Snapshot != nil {
		return c.Snapshot.AttemptStartedAt, true
	}
	return time.Time{}, true
}

// RetryDelay is also used when reserving a start, so abandoned attempts obey
// the same minimum spacing as completed failures.
func RetryDelay(attempts int) time.Duration {
	if attempts <= 1 {
		return 15 * time.Minute
	}
	return 60 * time.Minute
}

func workerCoverage(candidates []WorkerCandidate, state WorkerState, now time.Time) WorkerStatus {
	s := WorkerStatus{State: state.State, Error: state.Error, LastSweepAt: Timestamp(state.LastSweepAt)}
	if state.AuthHold {
		s.State = "auth_hold"
		s.Error = ErrWorkerAuthHold.Error()
	}
	var retry time.Time
	for _, c := range candidates {
		s.EligibleCards += c.Cards
		classification := c.Classification(now)
		if classification == "unresolved" {
			s.UnresolvedCards += c.Cards
			continue
		}
		s.EligibleIdentities++
		switch classification {
		case "current":
			s.CurrentIdentities++
			s.CurrentCards += c.Cards
		case "missing":
			s.MissingIdentities++
		case "stale":
			s.StaleIdentities++
		case "failed":
			s.FailedIdentities++
		}
		if at, _ := c.Due(now, false); at.After(now) && (retry.IsZero() || at.Before(retry)) {
			retry = at
		}
	}
	s.RetryAt = Timestamp(retry)
	return s
}
