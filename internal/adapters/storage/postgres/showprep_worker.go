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
	candidates, err := showWorkerInventory(ctx, q, id)
	if err != nil {
		return nil, err
	}
	out := make([]sp.WorkerCandidate, 0, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	// Inventory rows are closed before this second query, including when q is
	// the fenced transaction's single connection. Load only canonical keys.
	rows, err := q.QueryContext(ctx, `SELECT identity_key,payload,attempt,attempt_state,attempt_error,attempt_started_at,
 retry_window,retry_attempts,retry_not_before,retry_reset_epoch FROM showprep_evidence WHERE identity_key=ANY($1::text[])`, keys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, state, msg, window string
		var payload []byte
		var attempt, reset int64
		var attempts int
		var started time.Time
		var notBefore sql.NullTime
		if err := rows.Scan(&key, &payload, &attempt, &state, &msg, &started, &window, &attempts, &notBefore, &reset); err != nil {
			return nil, err
		}
		c := candidates[key]
		c.RetryWindow, c.Attempts, c.NotBefore, c.ResetEpoch = window, attempts, notBefore.Time, reset
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
		c.Snapshot.AttemptStartedAt = started
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range candidates {
		out = append(out, *c)
	}
	return out, nil
}

func showWorkerInventory(ctx context.Context, q showPrepQuery, only *sp.Identity) (map[string]*sp.WorkerCandidate, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.gem_rate_id,p.grader,p.grade_value,count(*)
 FROM campaign_purchases p JOIN campaigns c ON c.id=p.campaign_id
 WHERE NOT p.was_refunded AND c.phase<>'closed'
 AND NOT EXISTS(SELECT 1 FROM campaign_sales sale WHERE sale.purchase_id=p.id)
 GROUP BY p.gem_rate_id,p.grader,p.grade_value`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]*sp.WorkerCandidate{}
	for rows.Next() {
		var purchase sp.Purchase
		var cards int
		if err := rows.Scan(&purchase.ProfileID, &purchase.Grader, &purchase.Grade, &cards); err != nil {
			return nil, err
		}
		// Reuse the cached reader's authoritative Unicode/case normalization.
		// SQL groups raw values only to reduce the finite inventory projection;
		// spelling variants are merged here, including during dispatch rechecks.
		identity := purchase.Identity()
		if only != nil && identity != *only {
			continue
		}
		key := identity.Key()
		if out[key] == nil {
			out[key] = &sp.WorkerCandidate{Identity: identity}
		}
		out[key].Cards += cards
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
