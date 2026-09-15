package mocks

import (
	"context"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

type ShowPrepWorkerStoreMock struct {
	AcquireFn            func(context.Context, string) (sp.WorkerLease, bool, error)
	RenewFn              func(context.Context, sp.WorkerLease) error
	EndFn                func(context.Context, sp.WorkerLease, sp.WorkerRunResult, time.Time) error
	CandidatesFn         func(context.Context) ([]sp.WorkerCandidate, error)
	BeginFn              func(context.Context, sp.WorkerLease, sp.Identity, time.Time) (int64, error)
	FinishFn             func(context.Context, sp.WorkerLease, sp.Identity, int64, sp.Snapshot, time.Time, bool) error
	ReadStateFn          func(context.Context) (sp.WorkerState, error)
	RequestRunFn         func(context.Context, bool) error
	CredentialsChangedFn func(context.Context) error
}

var _ sp.WorkerStore = (*ShowPrepWorkerStoreMock)(nil)

func (m *ShowPrepWorkerStoreMock) Acquire(ctx context.Context, owner string) (sp.WorkerLease, bool, error) {
	if m.AcquireFn != nil {
		return m.AcquireFn(ctx, owner)
	}
	return sp.WorkerLease{Owner: owner, Epoch: 1}, true, nil
}
func (m *ShowPrepWorkerStoreMock) Renew(ctx context.Context, l sp.WorkerLease) error {
	if m.RenewFn != nil {
		return m.RenewFn(ctx, l)
	}
	return nil
}
func (m *ShowPrepWorkerStoreMock) End(ctx context.Context, l sp.WorkerLease, r sp.WorkerRunResult, now time.Time) error {
	if m.EndFn != nil {
		return m.EndFn(ctx, l, r, now)
	}
	return nil
}
func (m *ShowPrepWorkerStoreMock) Candidates(ctx context.Context) ([]sp.WorkerCandidate, error) {
	if m.CandidatesFn != nil {
		return m.CandidatesFn(ctx)
	}
	return nil, nil
}
func (m *ShowPrepWorkerStoreMock) Begin(ctx context.Context, l sp.WorkerLease, id sp.Identity, now time.Time) (int64, error) {
	if m.BeginFn != nil {
		return m.BeginFn(ctx, l, id, now)
	}
	return 1, nil
}
func (m *ShowPrepWorkerStoreMock) Finish(ctx context.Context, l sp.WorkerLease, id sp.Identity, n int64, s sp.Snapshot, now time.Time, auth bool) error {
	if m.FinishFn != nil {
		return m.FinishFn(ctx, l, id, n, s, now, auth)
	}
	return nil
}
func (m *ShowPrepWorkerStoreMock) ReadState(ctx context.Context) (sp.WorkerState, error) {
	if m.ReadStateFn != nil {
		return m.ReadStateFn(ctx)
	}
	return sp.WorkerState{WorkerRunResult: sp.WorkerRunResult{State: "idle"}}, nil
}
func (m *ShowPrepWorkerStoreMock) RequestRun(ctx context.Context, retry bool) error {
	if m.RequestRunFn != nil {
		return m.RequestRunFn(ctx, retry)
	}
	return nil
}
func (m *ShowPrepWorkerStoreMock) CredentialsChanged(ctx context.Context) error {
	if m.CredentialsChangedFn != nil {
		return m.CredentialsChangedFn(ctx)
	}
	return nil
}
