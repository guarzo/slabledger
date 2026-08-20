package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestPurchaseColumnsMatchScanDests pins the one invariant the compiler
// cannot: the canonical column list and the scan-destination slice must have
// the same length, or every Scan silently reads the wrong value into the
// wrong field — the failure class SLA-85 (e2c3f765) fixed for sales.
func TestPurchaseColumnsMatchScanDests(t *testing.T) {
	var p inventory.Purchase
	var psaCampaignName, attributionSource sql.NullString
	cols := strings.Split(purchaseColumns, ",")
	dests := purchaseScanDests(&p, &psaCampaignName, &attributionSource)
	require.Len(t, dests, len(cols), "purchaseScanDests must have one destination per purchaseColumns entry")
}

// TestPurchaseColumnsAliasedMatchesCanonical guards the hand-maintained JOIN
// variant: unlike saleColumnsAliased, purchaseColumnsAliased has no
// aliasColumns() derivation, so a column added to one list and not the other
// compiles cleanly and only fails at query time.
func TestPurchaseColumnsAliasedMatchesCanonical(t *testing.T) {
	canonical := strings.Split(purchaseColumns, ",")
	aliased := strings.Split(purchaseColumnsAliased, ",")
	require.Len(t, aliased, len(canonical), "aliased list must cover every canonical column")
	for i := range canonical {
		want := "p." + strings.TrimSpace(canonical[i])
		require.Equal(t, want, strings.TrimSpace(aliased[i]))
	}
}
