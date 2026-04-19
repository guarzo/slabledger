package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeReconciler struct {
	mu     sync.Mutex
	calls  int
	result dhlisting.ReconcileResult
	err    error
}

func (f *fakeReconciler) Reconcile(_ context.Context) (dhlisting.ReconcileResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.result, f.err
}

func TestDHReconcileScheduler_DisabledReturnsImmediately(t *testing.T) {
	rec := &fakeReconciler{}
	s := NewDHReconcileScheduler(rec, mocks.NewMockLogger(),
		config.DHReconcileConfig{Enabled: false, Interval: 24 * time.Hour})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("disabled scheduler should return immediately from Start")
	}
	assert.Equal(t, 0, rec.calls, "Reconcile should not be called when disabled")
}

func TestDHReconcileScheduler_RunOnceCallsReconcile(t *testing.T) {
	rec := &fakeReconciler{
		result: dhlisting.ReconcileResult{Scanned: 100, MissingOnDH: 5, Reset: 5},
	}
	s := NewDHReconcileScheduler(rec, mocks.NewMockLogger(),
		config.DHReconcileConfig{Enabled: true, Interval: 1 * time.Hour})

	err := s.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
}

func TestDHReconcileScheduler_RunOnceReturnsReconcileError(t *testing.T) {
	wantErr := errors.New("snapshot failed")
	rec := &fakeReconciler{err: wantErr}
	s := NewDHReconcileScheduler(rec, mocks.NewMockLogger(),
		config.DHReconcileConfig{Enabled: true, Interval: 1 * time.Hour})

	err := s.RunOnce(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestDHReconcileScheduler_DefaultIntervalApplied(t *testing.T) {
	rec := &fakeReconciler{}
	s := NewDHReconcileScheduler(rec, mocks.NewMockLogger(),
		config.DHReconcileConfig{Enabled: true, Interval: 0})
	assert.Equal(t, 1*time.Hour, s.config.Interval, "zero interval should default to 1h")
}
