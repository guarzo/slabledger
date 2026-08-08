package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
)

// DHEventStore writes dh_state_events rows, reads them back per purchase or
// cert, and prunes them on retention.
// Satisfies dhevents.Recorder, dhevents.CountsStore, dhevents.HistoryStore,
// and dhevents.Pruner.
type DHEventStore struct {
	db *sql.DB
}

var (
	_ dhevents.Recorder     = (*DHEventStore)(nil)
	_ dhevents.CountsStore  = (*DHEventStore)(nil)
	_ dhevents.HistoryStore = (*DHEventStore)(nil)
	_ dhevents.Pruner       = (*DHEventStore)(nil)
)

// NewDHEventStore constructs a DHEventStore wrapping the given database handle.
func NewDHEventStore(db *sql.DB) *DHEventStore {
	return &DHEventStore{db: db}
}

// Record inserts one event row. Zero-value string/int fields become SQL NULL.
//
// event_at is explicitly written as RFC3339 (rather than relying on the
// DEFAULT CURRENT_TIMESTAMP) so that subsequent comparisons in
// CountByTypeSince are consistent. The DEFAULT remains a safety net for any
// direct SQL inserts.
func (s *DHEventStore) Record(ctx context.Context, e dhevents.Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dh_state_events (
			purchase_id, cert_number, event_at, event_type,
			prev_push_status, new_push_status,
			prev_dh_status, new_dh_status,
			dh_inventory_id, dh_card_id,
			dh_order_id, sale_price_cents,
			source, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		nullIfEmpty(e.PurchaseID),
		nullIfEmpty(e.CertNumber),
		time.Now().UTC(),
		string(e.Type),
		nullIfEmpty(e.PrevPushStatus),
		nullIfEmpty(e.NewPushStatus),
		nullIfEmpty(e.PrevDHStatus),
		nullIfEmpty(e.NewDHStatus),
		zeroAsNull(e.DHInventoryID),
		zeroAsNull(e.DHCardID),
		nullIfEmpty(e.DHOrderID),
		zeroAsNull(e.SalePriceCents),
		string(e.Source),
		nullIfEmpty(e.Notes),
	)
	if err != nil {
		return fmt.Errorf("record dh event: %w", err)
	}
	return nil
}

// historySelect is the column list shared by the per-subject reads. The order
// must match scanEvents.
const historySelect = `
	SELECT id, purchase_id, cert_number, event_at, event_type,
	       prev_push_status, new_push_status,
	       prev_dh_status, new_dh_status,
	       dh_inventory_id, dh_card_id,
	       dh_order_id, sale_price_cents,
	       source, notes
	FROM dh_state_events`

// ByPurchase returns the event history for one purchase, newest first.
// Served by idx_dh_state_events_purchase.
func (s *DHEventStore) ByPurchase(ctx context.Context, purchaseID string, limit int) ([]dhevents.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		historySelect+` WHERE purchase_id = $1 ORDER BY event_at DESC, id DESC LIMIT $2`,
		purchaseID, dhevents.ClampHistoryLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("dh events by purchase: %w", err)
	}
	return scanEvents(rows)
}

// ByCert returns the event history for one cert number, newest first. Certs
// outlive the purchases that reference them — purchase_id is deliberately not
// an FK — so this can surface events from a purchase row that no longer exists.
// Served by idx_dh_state_events_cert.
func (s *DHEventStore) ByCert(ctx context.Context, certNumber string, limit int) ([]dhevents.StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		historySelect+` WHERE cert_number = $1 ORDER BY event_at DESC, id DESC LIMIT $2`,
		certNumber, dhevents.ClampHistoryLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("dh events by cert: %w", err)
	}
	return scanEvents(rows)
}

// scanEvents materializes rows selected with historySelect, translating the
// nullable columns back to the zero values Record wrote them from.
func scanEvents(rows *sql.Rows) ([]dhevents.StoredEvent, error) {
	defer func() { _ = rows.Close() }()

	var out []dhevents.StoredEvent
	for rows.Next() {
		var (
			e                                dhevents.StoredEvent
			purchaseID, certNumber           sql.NullString
			prevPush, newPush, prevDH, newDH sql.NullString
			orderID, notes                   sql.NullString
			eventType, source                string
			inventoryID, cardID, salePrice   sql.NullInt64
		)
		if err := rows.Scan(
			&e.ID, &purchaseID, &certNumber, &e.EventAt, &eventType,
			&prevPush, &newPush, &prevDH, &newDH,
			&inventoryID, &cardID, &orderID, &salePrice,
			&source, &notes,
		); err != nil {
			return nil, fmt.Errorf("scan dh event: %w", err)
		}
		e.Type = dhevents.Type(eventType)
		e.Source = dhevents.Source(source)
		e.PurchaseID = purchaseID.String
		e.CertNumber = certNumber.String
		e.PrevPushStatus = prevPush.String
		e.NewPushStatus = newPush.String
		e.PrevDHStatus = prevDH.String
		e.NewDHStatus = newDH.String
		e.DHOrderID = orderID.String
		e.Notes = notes.String
		e.DHInventoryID = int(inventoryID.Int64)
		e.DHCardID = int(cardID.Int64)
		e.SalePriceCents = int(salePrice.Int64)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dh events: %w", err)
	}
	return out, nil
}

// DeleteOlderThan removes events stamped strictly before cutoff. Nothing else
// in the system deletes from this table, so this is the only bound on its size.
func (s *DHEventStore) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM dh_state_events WHERE event_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("prune dh events: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune dh events rows affected: %w", err)
	}
	return n, nil
}

// CountByTypeSince returns the number of events of the given type recorded
// at or after the given time.
func (s *DHEventStore) CountByTypeSince(ctx context.Context, t dhevents.Type, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dh_state_events WHERE event_type = $1 AND event_at >= $2`,
		string(t), since.UTC(),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count dh events by type: %w", err)
	}
	return n, nil
}
