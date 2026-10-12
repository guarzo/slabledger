package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHPriceDriftUsesEffectiveCommittedPrice(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		reviewed, override, cached int
		reviewedAt, overrideAt     string
		drift                      bool
	}{
		{"reviewed winner ignores stale override", 25000, 18000, 25000, "2026-10-11T02:00:00Z", "2026-10-11T01:00:00Z", false},
		{"override winner ignores stale reviewed", 18000, 25000, 25000, "2026-10-11T01:00:00Z", "2026-10-11T02:00:00Z", false},
		{"reviewed winner drifts", 25000, 18000, 18000, "2026-10-11T02:00:00Z", "2026-10-11T01:00:00Z", true},
		{"override winner drifts", 18000, 25000, 18000, "2026-10-11T01:00:00Z", "2026-10-11T02:00:00Z", true},
		{"override only", 0, 25000, 18000, "", "", true},
		{"reviewed only", 25000, 0, 18000, "", "", true},
		{"equal instants reviewed wins", 25000, 18000, 25000, "2026-10-11T12:00:00Z", "2026-10-11T07:00:00-05:00", false},
		{"offset override is chronologically newer", 18000, 25000, 25000, "2026-10-11T12:00:00Z", "2026-10-11T08:00:00-05:00", false},
		{"fractional timestamps", 18000, 25000, 25000, "2026-10-11T12:00:00.100Z", "2026-10-11T12:00:00.200Z", false},
		{"empty legacy timestamps reviewed wins", 25000, 18000, 25000, "", "", false},
		{"legacy populated override wins", 18000, 25000, 25000, "", "2026-10-11T12:00:00Z", false},
		{"malformed timestamps retain lexical fallback", 18000, 25000, 25000, "legacy-a", "legacy-b", false},
		{"no committed price", 0, 0, 18000, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _, _, _, id := setupReturnCore(t, "effective-price-cert", 42, "")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status='listed',reviewed_price_cents=$1,override_price_cents=$2,dh_listing_price_cents=$3,reviewed_at=$4,override_set_at=$5 WHERE id=$6`, tc.reviewed, tc.override, tc.cached, tc.reviewedAt, tc.overrideAt, id)
			require.NoError(t, err)
			fields := NewPurchaseStore(db.DB, mocks.NewMockLogger())
			for range 2 {
				purchases, err := fields.ListDHPriceDrift(ctx)
				require.NoError(t, err)
				if tc.drift {
					require.Len(t, purchases, 1)
					require.Equal(t, id, purchases[0].ID)
					require.NotEqual(t, tc.cached, inventory.ResolveListingPriceCents(&purchases[0]))
				} else {
					require.Empty(t, purchases, "the losing price must not repeatedly acquire mutation ownership")
				}
			}
		})
	}
}
