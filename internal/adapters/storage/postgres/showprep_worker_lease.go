package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

type ShowPrepWorkerStore struct{ db *sql.DB }

var _ sp.WorkerStore = (*ShowPrepWorkerStore)(nil)

func NewShowPrepWorkerStore(db *sql.DB) *ShowPrepWorkerStore { return &ShowPrepWorkerStore{db: db} }

func (s *ShowPrepWorkerStore) Acquire(ctx context.Context, owner string) (sp.WorkerLease, bool, error) {
	lease := sp.WorkerLease{Owner: owner}
	if owner == "" {
		return lease, false, sp.ErrInvalid
	}
	err := s.db.QueryRowContext(ctx, `UPDATE showprep_worker SET owner=$1,epoch=epoch+1,
 lease_until=clock_timestamp()+interval '30 seconds',state='running',error='',
 active_retry=retry_requested,requested=false,retry_requested=false,auth_hold=false
 WHERE singleton AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND NOT auth_hold
 RETURNING epoch,active_retry`, owner).Scan(&lease.Epoch, &lease.RetryFailed)
	if errors.Is(err, sql.ErrNoRows) {
		return lease, false, nil
	}
	return lease, err == nil, err
}

// Lock order: worker row -> business advisory lock 731946 -> evidence row.
// Legacy/list writers never take the worker row. No network work belongs here.
// Revalidate after lock waits AND after the body: expiry during a short write
// rolls back the entire transaction, including auth/health and publication.
func (s *ShowPrepWorkerStore) fenced(ctx context.Context, lease sp.WorkerLease, business bool, fn func(*showPrepSession) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var epoch int64
	var until sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT epoch,lease_until FROM showprep_worker WHERE singleton FOR UPDATE`).Scan(&epoch, &until); err != nil {
		return err
	}
	check := func() error {
		var valid bool
		err := tx.QueryRowContext(ctx, `SELECT owner=$1 AND epoch=$2 AND lease_until>clock_timestamp() FROM showprep_worker WHERE singleton`, lease.Owner, lease.Epoch).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return sp.ErrWorkerLeaseLost
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	if business {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731946)`); err != nil {
			return err
		}
	}
	if err := check(); err != nil {
		return err
	}
	if err := fn(&showPrepSession{q: tx}); err != nil {
		return err
	}
	// The row stays locked, so no other owner/control revision can intervene.
	// Check the ORIGINAL expiry even for renewal/release: a slow UPDATE cannot
	// revive an expired lease or publish health after relinquishing ownership.
	var unexpired bool
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE($1::timestamptz>clock_timestamp(),false)`, until).Scan(&unexpired); err != nil {
		return err
	}
	if !unexpired {
		return sp.ErrWorkerLeaseLost
	}
	return tx.Commit()
}

func (s *ShowPrepWorkerStore) Renew(ctx context.Context, lease sp.WorkerLease) error {
	return s.fenced(ctx, lease, false, func(tx *showPrepSession) error {
		_, err := tx.q.ExecContext(ctx, `UPDATE showprep_worker SET lease_until=clock_timestamp()+interval '30 seconds' WHERE singleton`)
		return err
	})
}
func (s *ShowPrepWorkerStore) End(ctx context.Context, lease sp.WorkerLease, result sp.WorkerRunResult, now time.Time) error {
	return s.fenced(ctx, lease, false, func(tx *showPrepSession) error {
		_, err := tx.q.ExecContext(ctx, `UPDATE showprep_worker SET owner='',lease_until=NULL,state=$1,error=$2,last_sweep_at=$3,active_retry=false,auth_hold=auth_hold OR $1='auth_hold' WHERE singleton`, result.State, result.Error, now)
		return err
	})
}
func (s *ShowPrepWorkerStore) ReadState(ctx context.Context) (sp.WorkerState, error) {
	var state sp.WorkerState
	var last sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT CASE WHEN state='running' AND (lease_until IS NULL OR lease_until<=clock_timestamp()) THEN 'failed' ELSE state END,
 CASE WHEN state='running' AND (lease_until IS NULL OR lease_until<=clock_timestamp()) THEN 'Evidence sweep interrupted' ELSE error END,auth_hold,last_sweep_at FROM showprep_worker WHERE singleton`).Scan(&state.State, &state.Error, &state.AuthHold, &last)
	state.LastSweepAt = last.Time
	return state, err
}
func (s *ShowPrepWorkerStore) RequestRun(ctx context.Context, retry bool) error {
	// An explicit repair fences any old-credential attempt before clearing its
	// hold. Normal wakeups only coalesce intent and never disturb ownership.
	_, err := s.db.ExecContext(ctx, `UPDATE showprep_worker SET requested=true,retry_requested=retry_requested OR $1,
 auth_hold=CASE WHEN $1 THEN false ELSE auth_hold END,
 epoch=epoch+CASE WHEN $1 THEN 1 ELSE 0 END,
 owner=CASE WHEN $1 THEN '' ELSE owner END,lease_until=CASE WHEN $1 THEN NULL ELSE lease_until END,
 state=CASE WHEN $1 THEN 'idle' ELSE state END,error=CASE WHEN $1 THEN '' ELSE error END WHERE singleton`, retry)
	return err
}
func (s *ShowPrepWorkerStore) CredentialsChanged(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE showprep_worker SET auth_hold=false,requested=true,
 epoch=epoch+1,owner='',lease_until=NULL,state='idle',error='' WHERE singleton`)
	return err
}
