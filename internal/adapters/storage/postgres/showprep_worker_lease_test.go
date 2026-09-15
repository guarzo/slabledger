package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func setupShowWorkerDB(t *testing.T) (*DB, *ShowPrepWorkerStore) {
	t.Helper()
	db := setupShowPrepTestDB(t)
	_, err := db.ExecContext(context.Background(), `DELETE FROM showprep_worker; INSERT INTO showprep_worker(singleton) VALUES(true)`)
	require.NoError(t, err)
	return db, NewShowPrepWorkerStore(db.DB)
}

func TestShowWorkerTwoConnectionsFenceEveryWrite(t *testing.T) {
	db, a := setupShowWorkerDB(t)
	other := requireTestDB(t)
	b := NewShowPrepWorkerStore(other.DB)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	id := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	lease, ok, err := a.Acquire(ctx, "owner-a")
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = b.Acquire(ctx, "owner-b")
	require.NoError(t, err)
	require.False(t, ok)
	n, err := a.Begin(ctx, lease, id, now)
	require.NoError(t, err)
	require.NoError(t, a.Renew(ctx, lease))
	var seconds float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT extract(epoch FROM (lease_until-clock_timestamp())) FROM showprep_worker`).Scan(&seconds))
	require.InDelta(t, 30, seconds, 2)
	_, err = db.ExecContext(ctx, `UPDATE showprep_worker SET lease_until=clock_timestamp()-interval '1 second'`)
	require.NoError(t, err)
	// Expiry alone, without a replacement owner, forbids publication and renewal.
	require.ErrorIs(t, a.Finish(ctx, lease, id, n, sp.Snapshot{Complete: true}, now, false), sp.ErrWorkerLeaseLost)
	require.ErrorIs(t, a.Renew(ctx, lease), sp.ErrWorkerLeaseLost)
	next, ok, err := b.Acquire(ctx, "owner-b")
	require.NoError(t, err)
	require.True(t, ok)
	require.Greater(t, next.Epoch, lease.Epoch)
	_, err = a.Begin(ctx, lease, id, now.Add(time.Hour))
	require.ErrorIs(t, err, sp.ErrWorkerLeaseLost)
	require.ErrorIs(t, a.Finish(ctx, lease, id, n, sp.Snapshot{AttemptError: "failed"}, now, true), sp.ErrWorkerLeaseLost)
	require.ErrorIs(t, a.End(ctx, lease, sp.WorkerRunResult{State: "failed"}, now), sp.ErrWorkerLeaseLost)
	require.ErrorIs(t, a.Renew(ctx, lease), sp.ErrWorkerLeaseLost)
	state, err := b.ReadState(ctx)
	require.NoError(t, err)
	require.False(t, state.AuthHold)
	require.Equal(t, "running", state.State)
	require.NoError(t, b.End(ctx, next, sp.WorkerRunResult{State: "idle"}, now))
	_, ok, err = a.Acquire(ctx, "owner-c")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestShowWorkerPublicationRetainsGenerationFence(t *testing.T) {
	db, s := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	now := time.Now().UTC()
	id := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	legacy := NewShowPrepStore(db.DB)
	seedShowEvidence(t, legacy)
	before, err := legacy.ReadSnapshots(ctx, []sp.Identity{id})
	require.NoError(t, err)
	lease, ok, err := s.Acquire(ctx, "first")
	require.NoError(t, err)
	require.True(t, ok)
	n, err := s.Begin(ctx, lease, id, now.AddDate(0, 0, 1))
	require.NoError(t, err)
	newer, err := legacy.BeginAttempt(ctx, id, now.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Greater(t, newer, n)
	require.ErrorIs(t, s.Finish(ctx, lease, id, n, sp.Snapshot{Complete: true}, now, false), sp.ErrConflict)
	require.ErrorIs(t, s.Finish(ctx, lease, id, n, sp.Snapshot{}, now, true), sp.ErrConflict)
	state, err := s.ReadState(ctx)
	require.NoError(t, err)
	require.False(t, state.AuthHold)
	after, err := legacy.ReadSnapshots(ctx, []sp.Identity{id})
	require.NoError(t, err)
	require.Equal(t, before[id].Generation, after[id].Generation)
	require.Equal(t, before[id].Sales, after[id].Sales)
}
