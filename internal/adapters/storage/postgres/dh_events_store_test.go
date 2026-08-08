package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
)

// insertEventAt writes one dh_state_events row with an explicit event_at.
// Record() stamps now(), so retention and ordering can only be exercised by
// writing the column directly.
func insertEventAt(ctx context.Context, t *testing.T, db *DB, e dhevents.Event, at time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO dh_state_events (
			purchase_id, cert_number, event_at, event_type,
			prev_push_status, new_push_status, prev_dh_status, new_dh_status,
			dh_inventory_id, dh_card_id, dh_order_id, sale_price_cents, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id`,
		nullIfEmpty(e.PurchaseID), nullIfEmpty(e.CertNumber), at, string(e.Type),
		nullIfEmpty(e.PrevPushStatus), nullIfEmpty(e.NewPushStatus),
		nullIfEmpty(e.PrevDHStatus), nullIfEmpty(e.NewDHStatus),
		zeroAsNull(e.DHInventoryID), zeroAsNull(e.DHCardID),
		nullIfEmpty(e.DHOrderID), zeroAsNull(e.SalePriceCents),
		string(e.Source), nullIfEmpty(e.Notes),
	).Scan(&id), "insert dh_state_events row")
	return id
}

func truncateEvents(ctx context.Context, t *testing.T, db *DB) {
	t.Helper()
	_, err := db.ExecContext(ctx, `TRUNCATE TABLE dh_state_events RESTART IDENTITY`)
	require.NoError(t, err, "truncate dh_state_events")
}

func TestDHEventStore_ByPurchase(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	truncateEvents(ctx, t, db)
	store := NewDHEventStore(db.DB)

	now := time.Now().UTC().Truncate(time.Second)
	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-1", CertNumber: "111", Type: dhevents.TypeEnrolled, Source: dhevents.SourceCertIntake,
	}, now.Add(-2*time.Hour))
	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-1", CertNumber: "111", Type: dhevents.TypePushed, Source: dhevents.SourceDHPush,
		NewPushStatus: "pushed", DHInventoryID: 42, DHCardID: 7,
	}, now.Add(-time.Hour))
	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-2", CertNumber: "222", Type: dhevents.TypeEnrolled, Source: dhevents.SourceCertIntake,
	}, now)

	got, err := store.ByPurchase(ctx, "p-1", 0)
	require.NoError(t, err)
	require.Len(t, got, 2, "only p-1's events should be returned")

	// Newest first.
	assert.Equal(t, dhevents.TypePushed, got[0].Type)
	assert.Equal(t, dhevents.TypeEnrolled, got[1].Type)

	assert.NotZero(t, got[0].ID, "row id should be populated")
	assert.WithinDuration(t, now.Add(-time.Hour), got[0].EventAt, time.Second)
	assert.Equal(t, "p-1", got[0].PurchaseID)
	assert.Equal(t, "111", got[0].CertNumber)
	assert.Equal(t, dhevents.SourceDHPush, got[0].Source)
	assert.Equal(t, "pushed", got[0].NewPushStatus)
	assert.Equal(t, 42, got[0].DHInventoryID)
	assert.Equal(t, 7, got[0].DHCardID)

	// NULL columns must scan back as zero values, not error.
	assert.Empty(t, got[1].NewPushStatus)
	assert.Zero(t, got[1].DHInventoryID)
	assert.Empty(t, got[1].Notes)
}

func TestDHEventStore_ByPurchase_LimitIsClamped(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	truncateEvents(ctx, t, db)
	store := NewDHEventStore(db.DB)

	now := time.Now().UTC()
	for i := range 5 {
		insertEventAt(ctx, t, db, dhevents.Event{
			PurchaseID: "p-limit", Type: dhevents.TypePushed, Source: dhevents.SourceDHPush,
		}, now.Add(-time.Duration(i)*time.Minute))
	}

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "explicit limit is applied", limit: 2, want: 2},
		{name: "zero falls back to the default", limit: 0, want: 5},
		{name: "negative falls back to the default", limit: -1, want: 5},
		{name: "over-max does not error", limit: dhevents.MaxHistoryLimit + 1000, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.ByPurchase(ctx, "p-limit", tt.limit)
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

// TestDHEventStore_ByCert covers the case purchase_id cannot: a cert whose
// purchase row is gone. purchase_id is deliberately not an FK, so those rows
// survive and the cert lookup is the only way to reach them.
func TestDHEventStore_ByCert(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	truncateEvents(ctx, t, db)
	store := NewDHEventStore(db.DB)

	now := time.Now().UTC()
	insertEventAt(ctx, t, db, dhevents.Event{
		CertNumber: "999", Type: dhevents.TypeOrphanSale, Source: dhevents.SourceDHOrdersPoll,
		DHOrderID: "ord-1", SalePriceCents: 4500,
	}, now)
	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-9", CertNumber: "888", Type: dhevents.TypeSold, Source: dhevents.SourceDHOrdersPoll,
	}, now)

	got, err := store.ByCert(ctx, "999", 0)
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Empty(t, got[0].PurchaseID, "orphan events carry no purchase id")
	assert.Equal(t, dhevents.TypeOrphanSale, got[0].Type)
	assert.Equal(t, "ord-1", got[0].DHOrderID)
	assert.Equal(t, 4500, got[0].SalePriceCents)

	none, err := store.ByCert(ctx, "no-such-cert", 0)
	require.NoError(t, err)
	assert.Empty(t, none, "an unknown cert should return no rows, not an error")
}

func TestDHEventStore_DeleteOlderThan(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	truncateEvents(ctx, t, db)
	store := NewDHEventStore(db.DB)

	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-old", Type: dhevents.TypePushed, Source: dhevents.SourceDHPush,
	}, cutoff.Add(-time.Hour))
	// Exactly at the cutoff must survive: the delete is strictly-before.
	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-edge", Type: dhevents.TypePushed, Source: dhevents.SourceDHPush,
	}, cutoff)
	insertEventAt(ctx, t, db, dhevents.Event{
		PurchaseID: "p-new", Type: dhevents.TypePushed, Source: dhevents.SourceDHPush,
	}, now)

	deleted, err := store.DeleteOlderThan(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "only the row strictly before the cutoff should go")

	var remaining int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM dh_state_events`).Scan(&remaining))
	assert.Equal(t, 2, remaining)

	// Idempotent: a second pass finds nothing left to remove.
	deleted, err = store.DeleteOlderThan(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}
