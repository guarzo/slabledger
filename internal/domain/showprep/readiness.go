package showprep

import "time"

type ReadinessState string

type RefreshEligibility string

const (
	ReadinessNotChecked  ReadinessState = "not_checked"
	ReadinessCurrent     ReadinessState = "current"
	ReadinessStale       ReadinessState = "stale"
	ReadinessRunning     ReadinessState = "running"
	ReadinessInterrupted ReadinessState = "interrupted"
	ReadinessFailed      ReadinessState = "failed"
	ReadinessInvalid     ReadinessState = "invalid"
	ReadinessUnavailable ReadinessState = "unavailable"

	RefreshNeeded      RefreshEligibility = "needed"
	RefreshNotNeeded   RefreshEligibility = "not_needed"
	RefreshWait        RefreshEligibility = "wait"
	RefreshRetryOnly   RefreshEligibility = "retry_only"
	RefreshUnavailable RefreshEligibility = "unavailable"
)

// Readiness is a derived scheduling hint, not price support or acquisition on read.
// It is deliberately excluded from business/evidence version fingerprints.
// Empty timestamps mean inapplicable; nonempty timestamps are UTC RFC3339Nano.
// RetryAt is an observation boundary, never permission for automatic retry.
type Readiness struct {
	State              ReadinessState     `json:"state"`
	RefreshEligibility RefreshEligibility `json:"refreshEligibility"`
	IdentityKey        string             `json:"identityKey"`
	ExpiresAt          string             `json:"expiresAt"`
	RetryAt            string             `json:"retryAt"`
}

func unavailableReadiness(identity Identity) *Readiness {
	r := &Readiness{State: ReadinessUnavailable, RefreshEligibility: RefreshUnavailable}
	if identity.Valid() {
		r.IdentityKey = identity.Key()
	}
	return r
}

func deriveReadiness(identity Identity, s *Snapshot, now time.Time, invalidRecords bool) *Readiness {
	if !identity.Valid() {
		return unavailableReadiness(identity)
	}
	r := &Readiness{State: ReadinessInvalid, RefreshEligibility: RefreshRetryOnly, IdentityKey: identity.Key()}
	if s == nil {
		r.State, r.RefreshEligibility = ReadinessNotChecked, RefreshNeeded
		return r
	}
	if s.Identity != identity {
		return r
	}
	// The latest attempt takes precedence over retained payload health. A first
	// running/failed attempt need not have any verified payload or provenance yet.
	switch s.AttemptState {
	case "running":
		if s.AttemptStartedAt.IsZero() || s.AttemptStartedAt.After(now) {
			return r
		}
		bound := s.AttemptStartedAt.Add(120 * time.Second)
		r.RetryAt = Timestamp(bound)
		r.State = ReadinessInterrupted
		if now.Before(bound) {
			r.State, r.RefreshEligibility = ReadinessRunning, RefreshWait
		}
		return r
	case "failed", "partial":
		r.State = ReadinessFailed
		return r
	case "complete":
	default:
		return r
	}
	if s.Source != "cardladder" || invalidRecords {
		return r
	}
	if !s.Complete {
		r.State = ReadinessFailed
		return r
	}
	start, startErr := time.Parse(time.DateOnly, s.WindowStart)
	end, endErr := time.Parse(time.DateOnly, s.WindowEnd)
	_, currentEnd := Window(now)
	if startErr != nil || endErr != nil || !start.AddDate(0, 0, 29).Equal(end) || s.WindowEnd > currentEnd || s.RefreshedAt.IsZero() || s.RefreshedAt.After(now) {
		return r
	}
	// Anchor expiry to the verified window, not the read's date: stale payloads
	// must not gain a new future expiry each time they are read.
	expires := s.RefreshedAt.Add(24 * time.Hour)
	if boundary := end.AddDate(0, 0, 1); boundary.Before(expires) {
		expires = boundary
	}
	r.ExpiresAt = Timestamp(expires)
	r.State, r.RefreshEligibility = ReadinessCurrent, RefreshNotNeeded
	// Preserve the evaluator's inclusive 24-hour age rule. UTC date rollover is
	// exclusive: at midnight the previous window is already stale.
	if s.WindowEnd != currentEnd || now.Sub(s.RefreshedAt) > 24*time.Hour {
		r.State, r.RefreshEligibility = ReadinessStale, RefreshNeeded
	}
	return r
}
