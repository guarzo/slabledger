package showprep_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestWorkerReservesPersistenceAfterSourceDeadline(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	id := sp.Identity{ProfileID: "one", Grader: "PSA", Grade: 10}
	var persisted sp.Snapshot
	var remaining time.Duration
	store := &mocks.ShowPrepWorkerStoreMock{
		CandidatesFn: func(context.Context) ([]sp.WorkerCandidate, error) {
			return []sp.WorkerCandidate{{Identity: id, Cards: 1}}, nil
		},
		FinishFn: func(ctx context.Context, _ sp.WorkerLease, _ sp.Identity, _ int64, s sp.Snapshot, _ time.Time, _ bool) error {
			require.NoError(t, ctx.Err())
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining = time.Until(deadline)
			persisted = s
			return nil
		},
	}
	source := &mocks.ShowPrepSourceMock{FetchFn: func(ctx context.Context, _ sp.Identity, _ time.Time) (sp.Snapshot, error) {
		<-ctx.Done()
		return sp.Snapshot{}, ctx.Err()
	}}
	w := sp.NewEvidenceWorker(store, func(context.Context) (sp.Source, error) { return source, nil }, func() time.Time { return now }, nil)
	// The source exhausts the short work child; persistence retains its own live
	// five-second child, without a detached Background cleanup.
	ctx, cancel := context.WithTimeout(context.Background(), 5100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, w.RunOnce(ctx), sp.ErrWorkerFailed)
	require.False(t, persisted.Complete)
	require.Contains(t, persisted.AttemptError, "timeout")
	require.Greater(t, remaining, 4*time.Second)
	require.LessOrEqual(t, remaining, 5*time.Second)
}

func TestWorkerSourceAndRunBoundsAndUniqueOwners(t *testing.T) {
	var owners []string
	var sourceBudget, runBudget time.Duration
	store := &mocks.ShowPrepWorkerStoreMock{
		AcquireFn: func(_ context.Context, owner string) (sp.WorkerLease, bool, error) {
			owners = append(owners, owner)
			return sp.WorkerLease{Owner: owner, Epoch: 1}, true, nil
		},
		CandidatesFn: func(ctx context.Context) ([]sp.WorkerCandidate, error) {
			deadline, _ := ctx.Deadline()
			runBudget = time.Until(deadline)
			return []sp.WorkerCandidate{{Identity: sp.Identity{ProfileID: "one", Grader: "PSA", Grade: 10}}}, nil
		},
	}
	source := &mocks.ShowPrepSourceMock{FetchFn: func(ctx context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
		deadline, _ := ctx.Deadline()
		sourceBudget = time.Until(deadline)
		start, end := sp.Window(now)
		return sp.Snapshot{Identity: id, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now}, nil
	}}
	w := sp.NewEvidenceWorker(store, func(context.Context) (sp.Source, error) { return source, nil }, nil, nil)
	require.NoError(t, w.RunOnce(context.Background()))
	require.NoError(t, w.RunOnce(context.Background()))
	require.Len(t, owners, 2)
	require.NotEqual(t, owners[0], owners[1])
	require.Greater(t, sourceBudget, 59*time.Second)
	require.LessOrEqual(t, sourceBudget, 60*time.Second)
	require.Greater(t, runBudget, 4*time.Minute)
	require.LessOrEqual(t, runBudget, 5*time.Minute)
}

func TestWorkerShutdownNeverPublishesFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &mocks.ShowPrepWorkerStoreMock{
		CandidatesFn: func(context.Context) ([]sp.WorkerCandidate, error) {
			return []sp.WorkerCandidate{{Identity: sp.Identity{ProfileID: "one", Grader: "PSA", Grade: 10}}}, nil
		},
		FinishFn: func(context.Context, sp.WorkerLease, sp.Identity, int64, sp.Snapshot, time.Time, bool) error {
			t.Error("published after shutdown")
			return nil
		},
		EndFn: func(context.Context, sp.WorkerLease, sp.WorkerRunResult, time.Time) error {
			t.Error("detached release after shutdown")
			return nil
		},
	}
	source := &mocks.ShowPrepSourceMock{FetchFn: func(c context.Context, _ sp.Identity, _ time.Time) (sp.Snapshot, error) {
		cancel()
		<-c.Done()
		return sp.Snapshot{AttemptError: "secret"}, errors.New("secret")
	}}
	w := sp.NewEvidenceWorker(store, func(context.Context) (sp.Source, error) { return source, nil }, nil, nil)
	require.ErrorIs(t, w.RunOnce(ctx), context.Canceled)
}
