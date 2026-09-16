package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

// Resolve rows before decoding payloads: canonical ROW authority includes NULL,
// failed and malformed payloads. Independent alias generations cannot be merged.
type showEvidenceRow struct {
	key             string
	identity        sp.Identity
	payload         []byte
	attempt         int64
	state, message  string
	started         time.Time
	retryWindow     string
	retryAttempts   int
	retryNotBefore  sql.NullTime
	retryResetEpoch int64
}
type showEvidenceResolution struct {
	row       *showEvidenceRow
	ambiguous bool
}

const showEvidenceColumns = `identity_key,profile_id,grader,grade,payload,attempt,attempt_state,attempt_error,attempt_started_at,
 retry_window,retry_attempts,retry_not_before,retry_reset_epoch`

func canonicalShowIdentity(id sp.Identity) sp.Identity {
	return (sp.Purchase{ProfileID: id.ProfileID, Grader: id.Grader, Grade: id.Grade}).Identity()
}

// Exact callers retain their raw namespace. Only canonical misses use one
// batched fallback, independent of which spellings remain in today's inventory.
func resolveShowEvidence(ctx context.Context, q showPrepQuery, ids []sp.Identity) (map[sp.Identity]showEvidenceResolution, error) {
	out := make(map[sp.Identity]showEvidenceResolution, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	requested := make(map[string]sp.Identity, len(ids))
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		key := id.Key()
		requested[key] = id
		keys = append(keys, key)
	}
	exact, err := readShowEvidenceRows(ctx, q, `SELECT `+showEvidenceColumns+` FROM showprep_evidence WHERE identity_key=ANY($1::text[])`, keys)
	if err != nil {
		return nil, err
	}
	for _, row := range exact {
		out[requested[row.key]] = showEvidenceResolution{row: row}
	}
	missing := map[sp.Identity]bool{}
	keys = keys[:0]
	var profiles []string
	var grades []float64
	for _, id := range requested {
		if out[id].row == nil && id.Valid() && id == canonicalShowIdentity(id) {
			missing[id] = true
			keys = append(keys, id.Key())
			profiles = append(profiles, id.ProfileID)
			grades = append(grades, id.Grade)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	// Containment/grade are only coarse bounds. Go owns full Unicode trimming and
	// grader normalization. Include exact keys again for concurrent canonical
	// inserts between the two reads; their payload wins even over malformed aliases.
	aliases, err := readShowEvidenceRows(ctx, q, `SELECT `+showEvidenceColumns+` FROM showprep_evidence
 WHERE identity_key=ANY($1::text[]) OR (grade=ANY($2::float8[])
 AND EXISTS(SELECT 1 FROM unnest($3::text[]) p WHERE strpos(profile_id,p)>0))`, keys, grades, profiles)
	if err != nil {
		return nil, err
	}
	for _, row := range aliases {
		id := canonicalShowIdentity(row.identity)
		if !missing[id] {
			continue
		}
		current := out[id]
		if row.key == id.Key() {
			out[id] = showEvidenceResolution{row: row}
			continue
		}
		if current.row != nil && current.row.key == id.Key() {
			continue
		}
		if current.row != nil || current.ambiguous {
			out[id] = showEvidenceResolution{ambiguous: true}
		} else {
			out[id] = showEvidenceResolution{row: row}
		}
	}
	return out, nil
}

func readShowEvidenceRows(ctx context.Context, q showPrepQuery, query string, args ...any) ([]*showEvidenceRow, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*showEvidenceRow
	for rows.Next() {
		row := &showEvidenceRow{}
		if err := rows.Scan(&row.key, &row.identity.ProfileID, &row.identity.Grader, &row.identity.Grade, &row.payload, &row.attempt, &row.state, &row.message, &row.started, &row.retryWindow, &row.retryAttempts, &row.retryNotBefore, &row.retryResetEpoch); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r showEvidenceResolution) snapshot(id sp.Identity) (*sp.Snapshot, error) {
	if r.ambiguous {
		return &sp.Snapshot{Identity: id, Sales: []sp.Sale{}, AttemptState: "failed", AttemptError: "Ambiguous legacy evidence identity"}, nil
	}
	if r.row == nil {
		return nil, nil
	}
	row := r.row
	snapshot := &sp.Snapshot{Sales: []sp.Sale{}}
	if len(row.payload) > 0 {
		if err := json.Unmarshal(row.payload, snapshot); err != nil {
			return nil, err
		}
	}
	snapshot.Identity = id
	snapshot.Attempt = row.attempt
	snapshot.AttemptState = row.state
	snapshot.AttemptError = row.message
	snapshot.AttemptStartedAt = row.started
	return snapshot, nil
}

// Called only inside the existing business/advisory transaction (worker callers
// also hold the worker row/epoch fence). Copy ONE lineage without re-encoding its
// JSONB; old rows and exact-key Finish remain untouched. A rejected/expired
// transaction rolls adoption back together with attempt and retry charging.
func (s *showPrepSession) adoptShowEvidence(ctx context.Context, id sp.Identity) error {
	resolved, err := resolveShowEvidence(ctx, s.q, []sp.Identity{id})
	if err != nil {
		return err
	}
	r := resolved[id]
	if r.ambiguous {
		return sp.ErrConflict
	}
	if r.row == nil || r.row.key == id.Key() {
		return nil
	}
	_, err = s.q.ExecContext(ctx, `INSERT INTO showprep_evidence (`+showEvidenceColumns+`)
 SELECT $1,$2,$3,$4,payload,attempt,attempt_state,attempt_error,attempt_started_at,
 retry_window,retry_attempts,retry_not_before,retry_reset_epoch
 FROM showprep_evidence WHERE identity_key=$5 ON CONFLICT(identity_key) DO NOTHING`, id.Key(), id.ProfileID, id.Grader, id.Grade, r.row.key)
	return err
}
