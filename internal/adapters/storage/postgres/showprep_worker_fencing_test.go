package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowWorkerLeaseExpiryDuringPublicationRollsBackEverything(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	id := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	lease, ok, err := store.Acquire(ctx, "slow-publication")
	require.NoError(t, err)
	require.True(t, ok)
	n, err := store.Begin(ctx, lease, id, time.Now())
	require.NoError(t, err)
	// Pause the actual shared UPDATE after its initial lease check. Expiry during
	// publication must roll back payload, auth hold AND retry completion together.
	_, err = db.ExecContext(ctx, `CREATE FUNCTION slow_worker_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN NEW; END $$;
 CREATE TRIGGER slow_worker_update BEFORE UPDATE ON showprep_evidence FOR EACH ROW EXECUTE FUNCTION slow_worker_update();
 UPDATE showprep_worker SET lease_until=clock_timestamp()+interval '100 milliseconds'`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(ctx, `DROP TRIGGER IF EXISTS slow_worker_update ON showprep_evidence; DROP FUNCTION IF EXISTS slow_worker_update()`)
		require.NoError(t, err)
	})
	require.ErrorIs(t, store.Finish(ctx, lease, id, n, sp.Snapshot{Complete: true}, time.Now(), true), sp.ErrWorkerLeaseLost)
	candidates, err := store.Candidates(ctx)
	require.NoError(t, err)
	require.Equal(t, "running", candidates[0].Snapshot.AttemptState)
	require.False(t, candidates[0].Snapshot.Complete)
	state, err := store.ReadState(ctx)
	require.NoError(t, err)
	require.False(t, state.AuthHold)
}

func TestShowWorkerExpiryDuringHealthOrRenewalCannotReviveLease(t *testing.T) {
	for _, operation := range []string{"end", "renew"} {
		t.Run(operation, func(t *testing.T) {
			db, store := setupShowWorkerDB(t)
			ctx := context.Background()
			lease, ok, err := store.Acquire(ctx, "slow-control")
			require.NoError(t, err)
			require.True(t, ok)
			_, err = db.ExecContext(ctx, `UPDATE showprep_worker SET lease_until=clock_timestamp()+interval '100 milliseconds';
 CREATE FUNCTION slow_worker_control() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN NEW; END $$;
 CREATE TRIGGER slow_worker_control BEFORE UPDATE ON showprep_worker FOR EACH ROW EXECUTE FUNCTION slow_worker_control()`)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := db.ExecContext(ctx, `DROP TRIGGER IF EXISTS slow_worker_control ON showprep_worker; DROP FUNCTION IF EXISTS slow_worker_control()`)
				require.NoError(t, err)
			})
			if operation == "end" {
				err = store.End(ctx, lease, sp.WorkerRunResult{State: "auth_hold"}, time.Now())
			} else {
				err = store.Renew(ctx, lease)
			}
			require.ErrorIs(t, err, sp.ErrWorkerLeaseLost)
			state, err := store.ReadState(ctx)
			require.NoError(t, err)
			require.False(t, state.AuthHold)
			require.Equal(t, "failed", state.State)
		})
	}
}

func TestShowWorkerNormalIntentSurvivesHeartbeatAndHealth(t *testing.T) {
	_, store := setupShowWorkerDB(t)
	ctx := context.Background()
	lease, ok, err := store.Acquire(ctx, "running")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, store.RequestRun(ctx, false))
	require.NoError(t, store.Renew(ctx, lease))
	require.NoError(t, store.End(ctx, lease, sp.WorkerRunResult{State: "idle"}, time.Now()))
	var requested bool
	require.NoError(t, store.db.QueryRowContext(ctx, `SELECT requested FROM showprep_worker`).Scan(&requested))
	require.True(t, requested)
	_, ok, err = store.Acquire(ctx, "next")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, store.db.QueryRowContext(ctx, `SELECT requested FROM showprep_worker`).Scan(&requested))
	require.False(t, requested)
}

func TestShowWorkerControlsAndRetryMetadataDoNotChangeBusinessVersionsOrHolds(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	legacy := NewShowPrepStore(db.DB)
	seedShowEvidence(t, legacy)
	now := time.Now().UTC()
	svc := sp.NewService(legacy, nil, func() time.Time { return now })
	before, err := svc.Evaluate(ctx, []string{showPurchase})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO showprep_price_holds(purchase_id,cert_number,grader) VALUES('other','held','PSA');
 UPDATE showprep_evidence SET retry_window='2026-09-15',retry_attempts=3,retry_not_before=now(),retry_reset_epoch=7`)
	require.NoError(t, err)
	require.NoError(t, store.CredentialsChanged(ctx))
	require.NoError(t, store.RequestRun(ctx, true))
	after, err := svc.Evaluate(ctx, []string{showPurchase})
	require.NoError(t, err)
	require.Equal(t, before[0].Version, after[0].Version)
	require.Equal(t, before[0].EvidenceVersion, after[0].EvidenceVersion)
	var holds int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_price_holds`).Scan(&holds))
	require.Equal(t, 1, holds)
}

func TestShowWorkerRechecksScopeButInflightSaleCanFinish(t *testing.T) {
	db, store := setupShowWorkerDB(t)
	ctx := context.Background()
	seedShowPurchase(t, db, showPurchase, "worker", "cert", "PSA")
	candidates, err := store.Candidates(ctx)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	id := candidates[0].Identity
	lease, ok, err := store.Acquire(ctx, "scope")
	require.NoError(t, err)
	require.True(t, ok)
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`, showPurchase)
	require.NoError(t, err)
	_, err = store.Begin(ctx, lease, id, time.Now())
	require.ErrorIs(t, err, sp.ErrNotFound)
	_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET was_refunded=false WHERE id=$1`, showPurchase)
	require.NoError(t, err)
	n, err := store.Begin(ctx, lease, id, time.Now())
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date) VALUES('sale',$1,'local','2026-09-15')`, showPurchase)
	require.NoError(t, err)
	snapshot, err := workerEmpty(ctx, id, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.Finish(ctx, lease, id, n, snapshot, time.Now(), false))
	candidates, err = store.Candidates(ctx)
	require.NoError(t, err)
	require.Empty(t, candidates)
	snapshots, err := NewShowPrepStore(db.DB).ReadSnapshots(ctx, []sp.Identity{id})
	require.NoError(t, err)
	require.True(t, snapshots[id].Complete)
}
