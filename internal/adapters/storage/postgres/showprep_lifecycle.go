package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func (s *showPrepSession) GetRetiredList(ctx context.Context, id string) (sp.RetiredList, bool, error) {
	var retired sp.RetiredList
	var at time.Time
	err := s.q.QueryRowContext(ctx, `SELECT list_id,delete_command_id,payload_hash,retired_at FROM showprep_retired_lists WHERE list_id=$1`, id).Scan(&retired.ListID, &retired.DeleteCommandID, &retired.PayloadHash, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return sp.RetiredList{}, false, nil
	}
	if err != nil {
		return sp.RetiredList{}, false, fmt.Errorf("read retired show: %w", err)
	}
	retired.RetiredAt = sp.Timestamp(at)
	return retired, true, nil
}

// Only called within the serialized feature transaction: tombstone and deletion
// commit together, and a delayed CreateList cannot pass its retirement check.
func (s *showPrepSession) RetireList(ctx context.Context, r sp.RetiredList) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO showprep_retired_lists(list_id,delete_command_id,payload_hash,retired_at) VALUES($1,$2,$3,COALESCE(NULLIF($4,'')::timestamptz,now()))`, r.ListID, r.DeleteCommandID, r.PayloadHash, r.RetiredAt)
	if err != nil {
		return fmt.Errorf("retire show: %w", err)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM showprep_lists WHERE id=$1`, r.ListID)
	if err != nil {
		return fmt.Errorf("delete retired show: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sp.ErrNotFound
	}
	return nil
}
