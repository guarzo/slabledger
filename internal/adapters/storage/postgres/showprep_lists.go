package postgres

import (
	"context"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

func scanShowList(row interface{ Scan(...any) error }) (sp.List, error) {
	var list sp.List
	var created, updated time.Time
	err := row.Scan(&list.ID, &list.Name, &created, &updated)
	list.CreatedAt = sp.Timestamp(created)
	list.UpdatedAt = sp.Timestamp(updated)
	return list, showPrepError(err)
}
func (s *ShowPrepStore) Lists(ctx context.Context) ([]sp.List, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,created_at,updated_at FROM showprep_lists ORDER BY created_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	lists := []sp.List{}
	for rows.Next() {
		list, err := scanShowList(rows)
		if err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return lists, rows.Err()
}
func (s *ShowPrepStore) GetList(ctx context.Context, id string) (sp.List, error) {
	return (&showPrepSession{q: s.db}).GetList(ctx, id)
}
func (s *showPrepSession) GetList(ctx context.Context, id string) (sp.List, error) {
	return scanShowList(s.q.QueryRowContext(ctx, `SELECT id,name,created_at,updated_at FROM showprep_lists WHERE id=$1`, id))
}
func (s *ShowPrepStore) CreateList(ctx context.Context, id, name string) (sp.List, error) {
	var list sp.List
	err := s.transaction(ctx, func(tx *showPrepSession) error {
		_, err := tx.q.ExecContext(ctx, `INSERT INTO showprep_lists(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING`, id, name)
		if err != nil {
			return err
		}
		list, err = tx.GetList(ctx, id)
		if err != nil {
			return err
		}
		if list.Name != name {
			return sp.ErrConflict
		}
		return nil
	})
	return list, err
}
func (s *ShowPrepStore) RenameList(ctx context.Context, id, name string) (sp.List, error) {
	var list sp.List
	err := s.transaction(ctx, func(tx *showPrepSession) error {
		var err error
		list, err = scanShowList(tx.q.QueryRowContext(ctx, `UPDATE showprep_lists SET name=$2,updated_at=CASE WHEN name=$2 THEN updated_at ELSE now() END WHERE id=$1 RETURNING id,name,created_at,updated_at`, id, name))
		return err
	})
	return list, err
}
func (s *ShowPrepStore) RemoveItem(ctx context.Context, list, id string) error {
	return s.transaction(ctx, func(tx *showPrepSession) error {
		if _, err := tx.GetList(ctx, list); err != nil {
			return err
		}
		res, err := tx.q.ExecContext(ctx, `DELETE FROM showprep_items WHERE list_id=$1 AND id=$2`, list, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		_, err = tx.q.ExecContext(ctx, `UPDATE showprep_lists SET updated_at=now() WHERE id=$1`, list)
		return err
	})
}
