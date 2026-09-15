package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func (s *ShowPrepWorkerStore) Candidates(ctx context.Context) ([]sp.WorkerCandidate, error) {
	return showWorkerCandidates(ctx, s.db, nil)
}
func showWorkerCandidates(ctx context.Context, q showPrepQuery, id *sp.Identity) ([]sp.WorkerCandidate, error) {
	query := `WITH candidates AS (
 SELECT btrim(p.gem_rate_id) AS profile,upper(btrim(p.grader)) AS grader,p.grade_value AS grade,count(*) AS cards
 FROM campaign_purchases p JOIN campaigns c ON c.id=p.campaign_id
 WHERE NOT p.was_refunded AND c.phase<>'closed'
 AND NOT EXISTS(SELECT 1 FROM campaign_sales sale WHERE sale.purchase_id=p.id)
 GROUP BY btrim(p.gem_rate_id),upper(btrim(p.grader)),p.grade_value
 ) SELECT c.profile,c.grader,c.grade,c.cards,e.payload,COALESCE(e.attempt,0),COALESCE(e.attempt_state,''),
 COALESCE(e.attempt_error,''),e.attempt_started_at,COALESCE(e.retry_window,''),COALESCE(e.retry_attempts,0),e.retry_not_before,COALESCE(e.retry_reset_epoch,0)
 FROM candidates c LEFT JOIN showprep_evidence e ON e.profile_id=c.profile AND e.grader=c.grader AND e.grade=c.grade`
	var args []any
	if id != nil {
		query += ` WHERE c.profile=$1 AND c.grader=$2 AND c.grade=$3`
		args = []any{id.ProfileID, id.Grader, id.Grade}
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []sp.WorkerCandidate{}
	for rows.Next() {
		var c sp.WorkerCandidate
		var payload []byte
		var attempt int64
		var state, msg string
		var started, notBefore sql.NullTime
		if err := rows.Scan(&c.Identity.ProfileID, &c.Identity.Grader, &c.Identity.Grade, &c.Cards, &payload, &attempt, &state, &msg, &started, &c.RetryWindow, &c.Attempts, &notBefore, &c.ResetEpoch); err != nil {
			return nil, err
		}
		c.NotBefore = notBefore.Time
		if attempt > 0 {
			c.Snapshot = &sp.Snapshot{}
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, c.Snapshot); err != nil {
					return nil, err
				}
			}
			c.Snapshot.Identity = c.Identity
			c.Snapshot.Attempt = attempt
			c.Snapshot.AttemptState = state
			c.Snapshot.AttemptError = msg
			c.Snapshot.AttemptStartedAt = started.Time
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ShowPrepWorkerStore) Begin(ctx context.Context, lease sp.WorkerLease, id sp.Identity, now time.Time) (int64, error) {
	var attempt int64
	err := s.fenced(ctx, lease, true, func(tx *showPrepSession) error {
		// Recheck live scope and latest payload under the publication lock. A sale
		// committed before dispatch excludes this identity; an in-flight sale does
		// not prevent its evidence-only completion.
		candidates, err := showWorkerCandidates(ctx, tx.q, &id)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return sp.ErrNotFound
		}
		c := candidates[0]
		var retry bool
		if err := tx.q.QueryRowContext(ctx, `SELECT active_retry FROM showprep_worker WHERE singleton`).Scan(&retry); err != nil {
			return err
		}
		retry = retry && c.ResetEpoch != lease.Epoch && c.Classification(now) == "failed"
		if _, due := c.Due(now, retry); !due {
			return sp.ErrConflict
		}
		_, window := sp.Window(now)
		count := c.Attempts
		if retry || c.RetryWindow != window {
			count = 0
		}
		attempt, err = tx.beginEvidence(ctx, id, now)
		if err != nil {
			return err
		}
		reset := c.ResetEpoch
		if retry {
			reset = lease.Epoch
		}
		_, err = tx.q.ExecContext(ctx, `UPDATE showprep_evidence SET retry_window=$2,retry_attempts=$3,retry_not_before=$4,retry_reset_epoch=$5 WHERE identity_key=$1`, id.Key(), window, count+1, now.Add(sp.RetryDelay(count+1)), reset)
		return err
	})
	return attempt, err
}
func (s *ShowPrepWorkerStore) Finish(ctx context.Context, lease sp.WorkerLease, id sp.Identity, attempt int64, snapshot sp.Snapshot, now time.Time, authHold bool) error {
	return s.fenced(ctx, lease, true, func(tx *showPrepSession) error {
		published, err := tx.finishEvidence(ctx, id, attempt, snapshot)
		if err != nil {
			return err
		}
		if !published {
			return sp.ErrConflict
		}
		// Counts were charged at Begin, including abandoned starts. Completion moves
		// backoff forward from the failure, never making a retry earlier.
		_, err = tx.q.ExecContext(ctx, `UPDATE showprep_evidence SET retry_not_before=CASE WHEN $3 THEN NULL ELSE $4::timestamptz + CASE WHEN retry_attempts<=1 THEN interval '15 minutes' ELSE interval '60 minutes' END END WHERE identity_key=$1 AND attempt=$2`, id.Key(), attempt, snapshot.Complete, now)
		if err != nil {
			return err
		}
		if authHold {
			_, err = tx.q.ExecContext(ctx, `UPDATE showprep_worker SET auth_hold=true,state='auth_hold',error=$1 WHERE singleton`, sp.ErrWorkerAuthHold.Error())
		}
		return err
	})
}
