package postgres

import (
	"context"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestShowPrepExactCertCandidatesIncludeUnavailable(t *testing.T) {
	for _, tt := range []struct{ name, sql string }{
		{"ready", `SELECT 1`},
		{"sold", `INSERT INTO campaign_sales(id,purchase_id,sale_price_cents,sale_date,sale_channel) VALUES('sale',$1,10000,'2026-09-14','local')`},
		{"refunded", `UPDATE campaign_purchases SET was_refunded=true WHERE id=$1`},
		{"closed", `UPDATE campaigns SET phase='closed' WHERE id=(SELECT campaign_id FROM campaign_purchases WHERE id=$1)`},
		{"not received", `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupShowPrepTestDB(t)
			ctx := context.Background()
			store := NewShowPrepStore(db.DB)
			seedShowPurchase(t, db, showOther, "other-campaign", "00123", "BGS")
			seedShowPurchase(t, db, showPurchase, "campaign", "00123", "PSA")
			if tt.name != "ready" {
				_, err := db.ExecContext(ctx, tt.sql, showOther)
				require.NoError(t, err)
			}
			require.NoError(t, store.Within(ctx, func(tx sp.Session) error {
				ids, err := tx.ReadCertPurchaseIDs(ctx, "00123")
				require.NoError(t, err)
				require.Equal(t, []string{showPurchase, showOther}, ids)
				require.NoError(t, tx.LockPurchases(ctx, ids))
				fresh, err := tx.ReadCertPurchaseIDs(ctx, "00123")
				require.NoError(t, err)
				require.Equal(t, ids, fresh)
				for _, cert := range []string{"123", " 00123", "00123 ", "missing"} {
					unmatched, err := tx.ReadCertPurchaseIDs(ctx, cert)
					require.NoError(t, err)
					require.Empty(t, unmatched)
				}
				return nil
			}))
		})
	}
}
