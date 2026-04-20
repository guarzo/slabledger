package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
)

// DHEventStore writes dh_state_events rows and provides aggregate reads.
// Satisfies dhevents.Recorder and dhevents.CountsStore.
type DHEventStore struct {
	db *sql.DB
}

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
