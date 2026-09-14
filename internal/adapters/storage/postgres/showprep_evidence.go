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
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, id.Key())
	}
	rows, err := s.q.QueryContext(ctx, `SELECT profile_id,grader,grade,payload,attempt,attempt_state,attempt_error,attempt_started_at FROM showprep_evidence WHERE identity_key=ANY($1::text[])`, keys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id sp.Identity
		var payload []byte
		var attempt int64
		var state, msg string
		var started time.Time
		if err := rows.Scan(&id.ProfileID, &id.Grader, &id.Grade, &payload, &attempt, &state, &msg, &started); err != nil {
			return nil, err
		}
		snapshot := &sp.Snapshot{Identity: id, Sales: []sp.Sale{}}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, snapshot); err != nil {
				return nil, err
			}
		}
		snapshot.Identity = id
		snapshot.Attempt = attempt
		snapshot.AttemptState = state
		snapshot.AttemptError = msg
		snapshot.AttemptStartedAt = started
		out[id] = snapshot
	}
	return out, rows.Err()
}
func (s *ShowPrepStore) BeginAttempt(ctx context.Context, id sp.Identity, now time.Time) (int64, error) {
	var attempt int64
	err := s.transaction(ctx, func(tx *showPrepSession) error {
		return tx.q.QueryRowContext(ctx, `INSERT INTO showprep_evidence(identity_key,profile_id,grader,grade,attempt,attempt_state,attempt_started_at)
  VALUES($1,$2,$3,$4,1,'running',$5) ON CONFLICT(identity_key) DO UPDATE SET
  attempt=showprep_evidence.attempt+1,attempt_state='running',attempt_error='',attempt_started_at=EXCLUDED.attempt_started_at RETURNING attempt`, id.Key(), id.ProfileID, id.Grader, id.Grade, now).Scan(&attempt)
	})
	return attempt, err
}
func (s *ShowPrepStore) FinishAttempt(ctx context.Context, id sp.Identity, attempt int64, snapshot sp.Snapshot) error {
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
		return err
	}
	return s.transaction(ctx, func(tx *showPrepSession) error {
		// Older/out-of-order completions cannot replace the newest attempt, including
		// when that newer attempt is still running or has failed.
		_, err := tx.q.ExecContext(ctx, `UPDATE showprep_evidence SET payload=CASE WHEN $3 THEN $4::jsonb ELSE COALESCE(payload,$4::jsonb) END,
  attempt_state=$5,attempt_error=$6 WHERE identity_key=$1 AND attempt=$2`, id.Key(), attempt, snapshot.Complete, string(payload), state, snapshot.AttemptError)
		return err
	})
}
