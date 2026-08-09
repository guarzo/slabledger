package dhpricing

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestResolveListingPrice_DelegatesToHub pins dhpricing's resolver to the hub
// implementation. dhpricing used to carry a verbatim copy of the reviewed↔override
// precedence rule; the copy collapsed onto inventory.ResolveListingPriceCents
// (SLA-89) and this asserts the two paths cannot silently diverge again. The
// precedence table itself lives in inventory/listing_price_test.go.
func TestResolveListingPrice_DelegatesToHub(t *testing.T) {
	const oldTS = "2026-01-01T00:00:00Z"
	const newTS = "2026-04-21T12:00:00Z"

	purchases := []*inventory.Purchase{
		{},
		{ReviewedPriceCents: 5000, ReviewedAt: newTS},
		{OverridePriceCents: 4500, OverrideSetAt: newTS},
		{ReviewedPriceCents: 5000, ReviewedAt: oldTS, OverridePriceCents: 7000, OverrideSetAt: newTS},
		{ReviewedPriceCents: 5000, ReviewedAt: newTS, OverridePriceCents: 7000, OverrideSetAt: oldTS},
		// Zero-value guards and the empty-timestamp lexicographic fallback,
		// which dhpricing's own suite did not previously cover.
		{ReviewedPriceCents: 5000, OverridePriceCents: 7000},
		{ReviewedPriceCents: 5000, OverridePriceCents: 7000, OverrideSetAt: newTS},
		{ReviewedPriceCents: 5000, ReviewedAt: newTS, OverridePriceCents: 7000},
		// Cross-offset: override is the later instant but the earlier string.
		{
			ReviewedPriceCents: 5000, ReviewedAt: "2026-04-21T12:00:00Z",
			OverridePriceCents: 7000, OverrideSetAt: "2026-04-21T08:00:00-05:00",
		},
	}

	for _, p := range purchases {
		got := resolveListingPrice(p)
		want := inventory.ResolveListingPriceCents(p)
		if got != want {
			t.Errorf("reviewed=%d@%q override=%d@%q: dhpricing got %d, hub got %d",
				p.ReviewedPriceCents, p.ReviewedAt,
				p.OverridePriceCents, p.OverrideSetAt, got, want)
		}
	}
}
