package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDHEventsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, RunMigrations(db, "migrations"))
	return db.DB
}

func TestDHEventStore_Record_FullEvent(t *testing.T) {
	db := newDHEventsTestDB(t)
	store := NewDHEventStore(db)

	err := store.Record(context.Background(), dhevents.Event{
		PurchaseID:     "pur-001",
		CertNumber:     "12345678",
		Type:           dhevents.TypeSold,
		PrevDHStatus:   "listed",
		NewDHStatus:    "sold",
		DHInventoryID:  42,
		DHCardID:       100,
		DHOrderID:      "dh-order-abc",
		SalePriceCents: 5000,
		Source:         dhevents.SourceDHOrdersPoll,
		Notes:          "matched via cert",
	})
	require.NoError(t, err)

	var count int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM dh_state_events WHERE purchase_id = ? AND event_type = ?`,
		"pur-001", string(dhevents.TypeSold),
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestDHEventStore_Record_OrphanNullPurchaseID(t *testing.T) {
	db := newDHEventsTestDB(t)
	store := NewDHEventStore(db)

	err := store.Record(context.Background(), dhevents.Event{
		CertNumber:     "99998888",
		Type:           dhevents.TypeOrphanSale,
		DHOrderID:      "dh-order-xyz",
		SalePriceCents: 7500,
		Source:         dhevents.SourceDHOrdersPoll,
	})
	require.NoError(t, err)

	var purchaseID *string
	err = db.QueryRow(
		`SELECT purchase_id FROM dh_state_events WHERE cert_number = ?`,
		"99998888",
	).Scan(&purchaseID)
	require.NoError(t, err)
	assert.Nil(t, purchaseID, "orphan events should have NULL purchase_id")
}

func TestDHEventStore_CountByTypeSince(t *testing.T) {
	db := newDHEventsTestDB(t)
	store := NewDHEventStore(db)
	ctx := context.Background()

	// 3 recent sold events (now)
	for i := 0; i < 3; i++ {
		require.NoError(t, store.Record(ctx, dhevents.Event{
			PurchaseID: "pur-recent", CertNumber: "c1",
			Type: dhevents.TypeSold, Source: dhevents.SourceDHOrdersPoll,
		}))
	}
	// 1 old sold event (force event_at into 2020)
	_, err := db.Exec(
		`INSERT INTO dh_state_events (event_type, source, event_at) VALUES (?, ?, ?)`,
		string(dhevents.TypeSold), string(dhevents.SourceDHOrdersPoll), "2020-01-01T00:00:00Z",
	)
	require.NoError(t, err)
	// 2 recent orphan_sale events
	for i := 0; i < 2; i++ {
		require.NoError(t, store.Record(ctx, dhevents.Event{
			CertNumber: "c2",
			Type:       dhevents.TypeOrphanSale,
			Source:     dhevents.SourceDHOrdersPoll,
		}))
	}

	n, err := store.CountByTypeSince(ctx, dhevents.TypeSold, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, n, "should count only recent sold events, not the 2020 one")

	n, err = store.CountByTypeSince(ctx, dhevents.TypeOrphanSale, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}
