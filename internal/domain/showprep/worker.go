package showprep

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	workerRunBudget     = 5 * time.Minute
	workerSourceBudget  = 60 * time.Second
	workerPersistBudget = 5 * time.Second
	workerHeartbeat     = 10 * time.Second
)

type EvidenceWorker struct {
	store    WorkerStore
	source   SourceProvider
	now      func() time.Time
	newOwner func() string
}

func NewEvidenceWorker(store WorkerStore, source SourceProvider, now func() time.Time, newOwner func() string) *EvidenceWorker {
	if now == nil {
		now = time.Now
	}
	if newOwner == nil {
		newOwner = rand.Text
	}
	return &EvidenceWorker{store: store, source: source, now: now, newOwner: newOwner}
}
func (w *EvidenceWorker) Status(ctx context.Context) (WorkerStatus, error) {
	candidates, err := w.store.Candidates(ctx)
	if err != nil {
		return WorkerStatus{}, err
	}
	state, err := w.store.ReadState(ctx)
	if err != nil {
		return WorkerStatus{}, err
	}
	return workerCoverage(candidates, state, w.now()), nil
}
func (w *EvidenceWorker) RequestRun(ctx context.Context, retry bool) error {
	return w.store.RequestRun(ctx, retry)
}
func (w *EvidenceWorker) CredentialsChanged(ctx context.Context) error {
	return w.store.CredentialsChanged(ctx)
}

func (w *EvidenceWorker) RunOnce(parent context.Context) error {
	budget, cancelBudget := context.WithTimeout(parent, workerRunBudget)
	defer cancelBudget()
	run, cancelRun := context.WithCancelCause(budget)
	defer cancelRun(nil)
	persist, cancel := context.WithTimeout(run, workerPersistBudget)
	lease, ok, err := w.store.Acquire(persist, w.newOwner())
	cancel()
	if err != nil {
		return workerStorageError("acquire", err)
	}
	if !ok {
		return nil
	}
	heartbeat, stop := context.WithCancel(run)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		ticker := time.NewTicker(workerHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeat.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(heartbeat, workerPersistBudget)
				err := w.store.Renew(ctx, lease)
				cancel()
				if err != nil {
					if heartbeat.Err() == nil {
						cancelRun(errors.Join(ErrWorkerLeaseLost, workerStorageError("renew", err)))
					}
					return
				}
			}
		}
	}()
	result, runErr := w.runOwned(run, lease)
	// Join the owned heartbeat before release. Neither shutdown nor lease loss
	// starts detached cleanup: a cancelled run recovers through database expiry.
	stop()
	<-joined
	if run.Err() != nil {
		return context.Cause(run)
	}
	persist, cancel = context.WithTimeout(run, workerPersistBudget)
	defer cancel()
	if err := w.store.End(persist, lease, result, w.now()); err != nil {
		return workerStorageError("end", err)
	}
	return runErr
}

func (w *EvidenceWorker) runOwned(run context.Context, lease WorkerLease) (WorkerRunResult, error) {
	failed := WorkerRunResult{State: "failed", Error: ErrWorkerFailed.Error()}
	deadline, _ := run.Deadline()
	work, stop := context.WithDeadline(run, deadline.Add(-workerPersistBudget))
	defer stop()
	sourceCtx, cancel := context.WithTimeout(work, workerSourceBudget)
	var source Source
	var err error
	if w.source != nil {
		source, err = w.source(sourceCtx)
	}
	cancel()
	if run.Err() != nil {
		return failed, context.Cause(run)
	}
	if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderAuth) {
		return WorkerRunResult{State: "auth_hold", Error: ErrWorkerAuthHold.Error()}, workerSourceError(ErrWorkerAuthHold, err)
	}
	if apperrors.HasErrorCode(err, apperrors.ErrCodeConfigMissing) || (err == nil && source == nil) {
		return WorkerRunResult{State: "unconfigured"}, nil
	}
	if err != nil {
		return failed, workerSourceError(ErrWorkerFailed, err)
	}
	candidates, err := w.store.Candidates(work)
	if err != nil {
		return failed, workerStorageError("candidates", err)
	}
	now := w.now()
	// Evaluate ordering once per sweep. Recheck due/scope atomically at Begin.
	slices.SortFunc(candidates, func(a, b WorkerCandidate) int {
		at, _ := a.Due(now, lease.RetryFailed)
		bt, _ := b.Due(now, lease.RetryFailed)
		if order := at.Compare(bt); order != 0 {
			return order
		}
		return strings.Compare(a.Identity.Key(), b.Identity.Key())
	})
	var sweepErr error
	for _, c := range candidates {
		if work.Err() != nil {
			break
		}
		if _, due := c.Due(w.now(), lease.RetryFailed); !due {
			continue
		}
		err = w.collect(run, work, lease, source, c.Identity)
		switch {
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict):
			continue
		case errors.Is(err, ErrWorkerAuthHold):
			return WorkerRunResult{State: "auth_hold", Error: ErrWorkerAuthHold.Error()}, err
		case apperrors.HasErrorCode(err, apperrors.ErrCodeConfigMissing):
			return WorkerRunResult{State: "unconfigured"}, nil
		case errors.Is(err, ErrWorkerFailed):
			sweepErr = err
		case err != nil:
			return failed, err
		}
	}
	if sweepErr != nil {
		return failed, sweepErr
	}
	// A no-due tick must not erase durable failure visibility during backoff.
	candidates, err = w.store.Candidates(run)
	if err != nil {
		return failed, workerStorageError("candidates", err)
	}
	for _, c := range candidates {
		if c.Classification(w.now()) == "failed" {
			return failed, nil
		}
	}
	return WorkerRunResult{State: "idle"}, nil
}

// Keep stage and the original cause for internal diagnostics/Is/As. Runtime
// logging must allowlist these fields, never print this error or its Context.
func workerStorageError(operation string, cause error) error {
	return apperrors.StorageError("showprep."+operation, cause).WithContext("operation", "showprep."+operation)
}

func workerSourceError(outcome, cause error) error {
	if errors.Is(outcome, ErrWorkerAuthHold) {
		return apperrors.ProviderAuthFailed("CardLadder", errors.Join(outcome, cause)).WithContext("operation", "showprep.source")
	}
	return apperrors.ProviderUnavailable("CardLadder", errors.Join(outcome, cause)).WithContext("operation", "showprep.source")
}

func (w *EvidenceWorker) collect(run, work context.Context, lease WorkerLease, source Source, id Identity) error {
	now := w.now()
	persist, cancel := context.WithTimeout(work, workerPersistBudget)
	attempt, err := w.store.Begin(persist, lease, id, now)
	cancel()
	if err != nil {
		return workerStorageError("begin", err)
	}
	sourceCtx, cancel := context.WithTimeout(work, workerSourceBudget)
	snap, sourceErr := source.Fetch(sourceCtx, id, now)
	timedOut := errors.Is(sourceCtx.Err(), context.DeadlineExceeded)
	cancel()
	if run.Err() != nil {
		return context.Cause(run)
	}
	auth := apperrors.HasErrorCode(sourceErr, apperrors.ErrCodeProviderAuth)
	unconfigured := apperrors.HasErrorCode(sourceErr, apperrors.ErrCodeConfigMissing)
	// Trust no external error text, even an adapter-provided AttemptError. Reuse
	// the evaluator's provenance/record checks rather than invent weaker success.
	snap.AttemptState = "complete"
	candidate := WorkerCandidate{Identity: id, Snapshot: &snap}
	finished := w.now()
	classification := candidate.Classification(finished)
	start, end := Window(now)
	// A lookup may finish across UTC midnight, but its acquisition must still
	// satisfy the age bound independently of the now-stale captured window.
	if sourceErr != nil || timedOut || snap.WindowStart != start || snap.WindowEnd != end || finished.Sub(snap.RefreshedAt) > 24*time.Hour || (classification != "current" && classification != "stale") {
		snap.Complete = false
	}
	snap.AttemptError = ""
	if !snap.Complete {
		snap.AttemptError = "Incomplete CardLadder window"
		if sourceErr != nil {
			snap.AttemptError = ErrWorkerFailed.Error()
		}
		if timedOut {
			snap.AttemptError = "CardLadder source timeout"
		}
		if auth {
			snap.AttemptError = ErrWorkerAuthHold.Error()
		}
		if unconfigured {
			snap.AttemptError = "CardLadder credentials unavailable"
		}
	}
	persist, cancel = context.WithTimeout(run, workerPersistBudget)
	defer cancel()
	if err := w.store.Finish(persist, lease, id, attempt, snap, w.now(), auth); err != nil {
		return workerStorageError("finish", err)
	}
	if auth {
		return workerSourceError(ErrWorkerAuthHold, sourceErr)
	}
	if unconfigured {
		return apperrors.ConfigMissing("CardLadder", "")
	}
	if !snap.Complete {
		if timedOut {
			sourceErr = errors.Join(sourceErr, context.DeadlineExceeded)
		}
		return workerSourceError(ErrWorkerFailed, sourceErr)
	}
	return nil
}
