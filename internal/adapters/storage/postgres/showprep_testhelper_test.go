package postgres

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

const showPurchase = "11111111-1111-4111-8111-111111111111"
const showOther = "22222222-2222-4222-8222-222222222222"
const showList = "33333333-3333-4333-8333-333333333333"

func resetShowPrep(t *testing.T, db *DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `TRUNCATE TABLE showprep_items, showprep_lists, showprep_evidence, showprep_price_holds RESTART IDENTITY`)
	require.NoError(t, err)
}
func setupShowPrepTestDB(t *testing.T) *DB {
	t.Helper()
	db := setupTestDB(t)
	resetShowPrep(t, db)
	t.Cleanup(func() { resetShowPrep(t, db) })
	return db
}
func seedShowPurchase(t *testing.T, db *DB, id, campaign, cert, grader string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES($1,'Show test','pending') ON CONFLICT DO NOTHING`, campaign)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date,received_at,dh_listing_price_cents,reviewed_price_cents,gem_rate_id) VALUES($1,$2,'Card',$3,$4,10,'2026-09-01','2026-09-02',30000,30000,'psa-1')`, id, campaign, cert, grader)
	require.NoError(t, err)
}
func seedShowEvidence(t *testing.T, store *ShowPrepStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	identity := sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	start, end := sp.Window(now)
	n, err := store.BeginAttempt(ctx, identity, now)
	require.NoError(t, err)
	require.NoError(t, store.FinishAttempt(ctx, identity, n, sp.Snapshot{Identity: identity, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now, Sales: []sp.Sale{{ID: "one", Date: end, PriceCents: 27000}, {ID: "two", Date: end, PriceCents: 27000}}}))
}
