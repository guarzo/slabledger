package showprep

import (
	"context"
	"errors"
	"time"
)

var (
	ErrWorkerLeaseLost = errors.New("show preparation worker ownership lost")
	ErrWorkerFailed    = errors.New("CardLadder evidence collection failed")
	ErrWorkerAuthHold  = errors.New("CardLadder authentication requires attention")
)

type SourceProvider func(context.Context) (Source, error)

// Identity counts partition resolved eligible identities. Card counts include
// unresolved inventory; neither denominator is interchangeable with the other.
type WorkerStatus struct {
	State              string `json:"state"`
	EligibleIdentities int    `json:"eligibleIdentities"`
	CurrentIdentities  int    `json:"currentIdentities"`
	MissingIdentities  int    `json:"missingIdentities"`
	StaleIdentities    int    `json:"staleIdentities"`
	FailedIdentities   int    `json:"failedIdentities"`
	EligibleCards      int    `json:"eligibleCards"`
	CurrentCards       int    `json:"currentCards"`
	UnresolvedCards    int    `json:"unresolvedCards"`
	LastSweepAt        string `json:"lastSweepAt"`
	RetryAt            string `json:"retryAt"`
	Error              string `json:"error"`
}

type WorkerLease struct {
	Owner       string
	Epoch       int64
	RetryFailed bool
}

type WorkerRunResult struct{ State, Error string }
type WorkerState struct {
	WorkerRunResult
	AuthHold    bool
	LastSweepAt time.Time
}

type WorkerCandidate struct {
	Identity    Identity
	Cards       int
	Snapshot    *Snapshot
	RetryWindow string
	Attempts    int
	NotBefore   time.Time
	ResetEpoch  int64
}

// WorkerStore is evidence-only. Begin rechecks scope/due state and charges the
// attempt atomically. Finish checks BOTH the lease and latest attempt before
// publishing payload, retry state or auth hold. No method writes financial data.
// Acquire consumes coalesced intent. Explicit repair invalidates old ownership.
type WorkerStore interface {
	Acquire(context.Context, string) (WorkerLease, bool, error)
	Renew(context.Context, WorkerLease) error
	End(context.Context, WorkerLease, WorkerRunResult, time.Time) error
	Candidates(context.Context) ([]WorkerCandidate, error)
	Begin(context.Context, WorkerLease, Identity, time.Time) (int64, error)
	Finish(context.Context, WorkerLease, Identity, int64, Snapshot, time.Time, bool) error
	ReadState(context.Context) (WorkerState, error)
	RequestRun(context.Context, bool) error
	CredentialsChanged(context.Context) error
}
