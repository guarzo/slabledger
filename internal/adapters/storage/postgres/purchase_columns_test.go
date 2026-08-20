package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestPurchaseColumnInvariants pins the invariants the compiler cannot: the
// canonical column list, the scan-destination slice, and the hand-maintained
// aliased JOIN variant must all agree on shape, or a Scan silently reads the
// wrong value into the wrong field -- the failure class SLA-85 (e2c3f765)
// fixed for sales.
func TestPurchaseColumnInvariants(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			// Guards purchaseScanDests: unlike saleColumnsAliased, purchaseColumnsAliased
			// has no aliasColumns() derivation, so a column added to one list and not the
			// other compiles cleanly and only fails at query time.
			name: "scan dests match canonical column count",
			run: func(t *testing.T) {
				var p inventory.Purchase
				var psaCampaignName, attributionSource sql.NullString
				cols := strings.Split(purchaseColumns, ",")
				dests := purchaseScanDests(&p, &psaCampaignName, &attributionSource)
				require.Len(t, dests, len(cols), "purchaseScanDests must have one destination per purchaseColumns entry")
			},
		},
		{
			name: "aliased list covers every canonical column",
			run: func(t *testing.T) {
				canonical := strings.Split(purchaseColumns, ",")
				aliased := strings.Split(purchaseColumnsAliased, ",")
				require.Len(t, aliased, len(canonical), "aliased list must cover every canonical column")
				for i := range canonical {
					want := "p." + strings.TrimSpace(canonical[i])
					require.Equal(t, want, strings.TrimSpace(aliased[i]))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
