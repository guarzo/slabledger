package postgres

import (
	"context"
	"database/sql"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

func (s *ShowPrepStore) GetItems(ctx context.Context, list string) ([]sp.Item, error) {
	return (&showPrepSession{q: s.db}).GetItems(ctx, list)
}
func (s *showPrepSession) GetItems(ctx context.Context, list string) ([]sp.Item, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id,purchase_id,card_name,cert_number,grader,grade,added_at,packed_at,version,acknowledged_price_cents,acknowledged_status,last_command FROM showprep_items WHERE list_id=$1 ORDER BY added_at,id`, list)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []sp.Item{}
	for rows.Next() {
		var item sp.Item
		var added time.Time
		var packed sql.NullTime
		if err := rows.Scan(&item.ID, &item.PurchaseID, &item.CardName, &item.CertNumber, &item.Grader, &item.Grade, &added, &packed, &item.Version, &item.AcknowledgedPriceCents, &item.AcknowledgedStatus, &item.LastCommand); err != nil {
			return nil, err
		}
		item.AddedAt = sp.Timestamp(added)
		if packed.Valid {
			item.PackedAt = sp.Timestamp(packed.Time)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *showPrepSession) InsertItem(ctx context.Context, list string, i sp.Item) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,version,acknowledged_price_cents,acknowledged_status)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, i.ID, list, i.PurchaseID, i.CardName, i.CertNumber, i.Grader, i.Grade, i.AddedAt, i.Version, i.AcknowledgedPriceCents, i.AcknowledgedStatus)
	if err != nil {
		return err
	}
	_, err = s.q.ExecContext(ctx, `UPDATE showprep_lists SET updated_at=now() WHERE id=$1`, list)
	return err
}
func (s *showPrepSession) SaveItem(ctx context.Context, list string, i sp.Item) error {
	res, err := s.q.ExecContext(ctx, `UPDATE showprep_items SET packed_at=NULLIF($3,'')::timestamptz,version=$4,acknowledged_price_cents=$5,acknowledged_status=$6,last_command=$7 WHERE list_id=$1 AND id=$2 AND version=$4-1`, list, i.ID, i.PackedAt, i.Version, i.AcknowledgedPriceCents, i.AcknowledgedStatus, i.LastCommand)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sp.ErrConflict
	}
	_, err = s.q.ExecContext(ctx, `UPDATE showprep_lists SET updated_at=now() WHERE id=$1`, list)
	return err
}
