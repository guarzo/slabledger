package postgres

import (
	"context"
	"encoding/json"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

func (s *ShowPrepStore) ReadSnapshots(ctx context.Context, ids []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
	return (&showPrepSession{q: s.db}).ReadSnapshots(ctx, ids)
}
func (s *showPrepSession) ReadSnapshots(ctx context.Context, ids []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
	out := map[sp.Identity]*sp.Snapshot{}
	resolved, err := resolveShowEvidence(ctx, s.q, ids)
	if err != nil {
		return nil, err
	}
	for id, r := range resolved {
		snapshot, err := r.snapshot(id)
		if err != nil {
			return nil, err
		}
		out[id] = snapshot
	}
	return out, nil
}
func (s *ShowPrepStore) BeginAttempt(ctx context.Context, id sp.Identity, now time.Time) (int64, error) {
	var attempt int64
	err := s.transaction(ctx, func(tx *showPrepSession) error {
		var err error
		attempt, err = tx.beginEvidence(ctx, id, now)
		return err
	})
	return attempt, err
}
func (s *showPrepSession) beginEvidence(ctx context.Context, id sp.Identity, now time.Time) (int64, error) {
	if err := s.adoptShowEvidence(ctx, id); err != nil {
		return 0, err
	}
	var attempt int64
	err := s.q.QueryRowContext(ctx, `INSERT INTO showprep_evidence(identity_key,profile_id,grader,grade,attempt,attempt_state,attempt_started_at)
  VALUES($1,$2,$3,$4,1,'running',$5) ON CONFLICT(identity_key) DO UPDATE SET
  attempt=showprep_evidence.attempt+1,attempt_state='running',attempt_error='',attempt_started_at=EXCLUDED.attempt_started_at RETURNING attempt`, id.Key(), id.ProfileID, id.Grader, id.Grade, now).Scan(&attempt)
	return attempt, err
}
func (s *ShowPrepStore) FinishAttempt(ctx context.Context, id sp.Identity, attempt int64, snapshot sp.Snapshot) error {
	return s.transaction(ctx, func(tx *showPrepSession) error {
		_, err := tx.finishEvidence(ctx, id, attempt, snapshot)
		return err
	})
}
func (s *showPrepSession) finishEvidence(ctx context.Context, id sp.Identity, attempt int64, snapshot sp.Snapshot) (bool, error) {
	snapshot.Identity = id
	snapshot.Generation = attempt
	if snapshot.Sales == nil {
		snapshot.Sales = []sp.Sale{}
	}
	state := "failed"
	if snapshot.Complete {
		state = "complete"
	} else if len(snapshot.Sales) > 0 {
		state = "partial"
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return false, err
	}
	// Older/out-of-order completions cannot replace the newest attempt, including
	// when that newer attempt is still running or has failed. Worker callers use
	// the same transaction for lease validation, publication and retry/auth state.
	result, err := s.q.ExecContext(ctx, `UPDATE showprep_evidence SET payload=CASE WHEN $3 THEN $4::jsonb ELSE COALESCE(payload,$4::jsonb) END,
  attempt_state=$5,attempt_error=$6 WHERE identity_key=$1 AND attempt=$2`, id.Key(), attempt, snapshot.Complete, string(payload), state, snapshot.AttemptError)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
