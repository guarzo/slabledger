package inventory

import "testing"

// TestResolveListingPriceCents covers the reviewed↔override resolution rules:
// both fields hold operator-committed prices; when both are set, the newest
// commit wins, compared as parsed RFC3339 instants with a lexicographic
// fallback when either timestamp is empty or malformed.
//
// This table is the canonical coverage for the rule. It moved here from
// dhlisting/push_safety_test.go when the duplicated sibling copies collapsed
// onto this one implementation (SLA-89).
func TestResolveListingPriceCents(t *testing.T) {
	for _, tc := range listingPriceCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveListingPriceCents(tc.purchase())
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// listingPriceCase is one reviewed/override resolution scenario.
type listingPriceCase struct {
	name               string
	reviewedPriceCents int
	reviewedAt         string
	overridePriceCents int
	overrideSetAt      string
	clValueCents       int
	want               int
}

func (tc listingPriceCase) purchase() *Purchase {
	return &Purchase{
		ReviewedPriceCents: tc.reviewedPriceCents,
		ReviewedAt:         tc.reviewedAt,
		OverridePriceCents: tc.overridePriceCents,
		OverrideSetAt:      tc.overrideSetAt,
		CLValueCents:       tc.clValueCents,
	}
}

func listingPriceCases() []listingPriceCase {
	const oldTS = "2026-01-01T00:00:00Z"
	const newTS = "2026-04-21T12:00:00Z"

	return []listingPriceCase{
		{
			name:               "only reviewed set",
			reviewedPriceCents: 5000,
			reviewedAt:         newTS,
			clValueCents:       3000,
			want:               5000,
		},
		{
			name:               "only override set",
			overridePriceCents: 4500,
			overrideSetAt:      newTS,
			clValueCents:       3000,
			want:               4500,
		},
		{
			name:         "zero when reviewed and override both zero (CL is not a fallback)",
			clValueCents: 3000,
			want:         0,
		},
		{
			name: "all zero",
			want: 0,
		},
		{
			// Common user flow: reviewed was set earlier, then the operator
			// opens the Price Override dialog and commits a new value.
			name:               "newer override wins over older reviewed",
			reviewedPriceCents: 5000,
			reviewedAt:         oldTS,
			overridePriceCents: 7000,
			overrideSetAt:      newTS,
			want:               7000,
		},
		{
			// Reverse: override was set earlier, then the operator ran a
			// proper price-review pass. The review is the latest signal.
			name:               "newer reviewed wins over older override",
			reviewedPriceCents: 5000,
			reviewedAt:         newTS,
			overridePriceCents: 7000,
			overrideSetAt:      oldTS,
			want:               5000,
		},
		{
			// Defensive: if we ever read a row with both prices but empty
			// timestamps (legacy / manual edit), preserve the historical
			// "reviewed wins" behavior rather than picking arbitrarily.
			name:               "both set, both timestamps empty → reviewed wins",
			reviewedPriceCents: 5000,
			overridePriceCents: 7000,
			want:               5000,
		},
		{
			// Mixed: the override dialog was used on a row that predates
			// reviewed_at tracking. Empty string sorts before the populated
			// timestamp, so override (the only committed signal) wins.
			name:               "reviewed price with empty timestamp vs populated override → override wins",
			reviewedPriceCents: 5000,
			overridePriceCents: 7000,
			overrideSetAt:      newTS,
			want:               7000,
		},
		{
			// Mixed inverse: override field retained from an earlier manual
			// set but with no timestamp, and a newer formal review followed.
			name:               "populated reviewed vs override price with empty timestamp → reviewed wins",
			reviewedPriceCents: 5000,
			reviewedAt:         newTS,
			overridePriceCents: 7000,
			want:               5000,
		},
		{
			// Cross-offset: the override instant (13:00 UTC) is an hour
			// AFTER the reviewed instant (12:00 UTC), but as strings
			// "2026-04-21T08:00:00-05:00" sorts BEFORE "2026-04-21T12:00:00Z".
			// Parsed comparison must pick override; lexicographic would have
			// picked reviewed and silently pushed the stale price.
			name:               "override in non-UTC offset newer than reviewed in UTC → override wins",
			reviewedPriceCents: 5000,
			reviewedAt:         "2026-04-21T12:00:00Z",
			overridePriceCents: 7000,
			overrideSetAt:      "2026-04-21T08:00:00-05:00",
			want:               7000,
		},
	}
}
