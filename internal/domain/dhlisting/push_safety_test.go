package dhlisting

import (
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestResolveListingPriceCents_DelegatesToHub pins dhlisting's wrapper to the
// hub implementation. The precedence table itself lives in
// inventory/listing_price_test.go; this asserts the wrapper stays a pass-through
// so a future re-inlining that diverges fails here (SLA-89).
func TestResolveListingPriceCents_DelegatesToHub(t *testing.T) {
	for _, p := range sharedResolverPurchases() {
		got := ResolveListingPriceCents(p)
		want := inventory.ResolveListingPriceCents(p)
		if got != want {
			t.Errorf("reviewed=%d@%q override=%d@%q: dhlisting got %d, hub got %d",
				p.ReviewedPriceCents, p.ReviewedAt,
				p.OverridePriceCents, p.OverrideSetAt, got, want)
		}
	}
}

// sharedResolverPurchases returns the resolution scenarios that distinguish
// the reviewed↔override rule's branches, including the cross-offset case
// where parsed and lexicographic comparison disagree.
func sharedResolverPurchases() []*inventory.Purchase {
	const oldTS = "2026-01-01T00:00:00Z"
	const newTS = "2026-04-21T12:00:00Z"

	return []*inventory.Purchase{
		{},
		{ReviewedPriceCents: 5000, ReviewedAt: newTS},
		{OverridePriceCents: 4500, OverrideSetAt: newTS},
		{ReviewedPriceCents: 5000, ReviewedAt: oldTS, OverridePriceCents: 7000, OverrideSetAt: newTS},
		{ReviewedPriceCents: 5000, ReviewedAt: newTS, OverridePriceCents: 7000, OverrideSetAt: oldTS},
		{ReviewedPriceCents: 5000, OverridePriceCents: 7000},
		{ReviewedPriceCents: 5000, OverridePriceCents: 7000, OverrideSetAt: newTS},
		{ReviewedPriceCents: 5000, ReviewedAt: newTS, OverridePriceCents: 7000},
		// Cross-offset: override is the later instant but the earlier string.
		{
			ReviewedPriceCents: 5000, ReviewedAt: "2026-04-21T12:00:00Z",
			OverridePriceCents: 7000, OverrideSetAt: "2026-04-21T08:00:00-05:00",
		},
	}
}

// TestEvaluateHoldTriggers_InitialPush covers checkInitialPushValueMismatch
// (the branch for DHInventoryID == 0).
func TestEvaluateHoldTriggers_InitialPush(t *testing.T) {
	cfg := inventory.DefaultDHPushConfig()

	tests := []struct {
		name               string
		buyCostCents       int
		reviewedPriceCents int
		clValueCents       int
		wantHeld           bool
		wantContains       string
	}{
		{
			name:         "no buy cost: no hold",
			buyCostCents: 0,
			clValueCents: 10000,
			wantHeld:     false,
		},
		{
			name:         "no market value: no hold",
			buyCostCents: 10000,
			clValueCents: 0,
			wantHeld:     false,
		},
		{
			name:               "market value well above floor: no hold",
			buyCostCents:       10000,
			reviewedPriceCents: 12000,
			wantHeld:           false,
		},
		{
			name:               "listing price far below floor: hold",
			buyCostCents:       10000,
			reviewedPriceCents: 1000, // 10% of cost — well below any reasonable floor
			wantHeld:           true,
			wantContains:       "initial_value_mismatch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &inventory.Purchase{
				DHInventoryID:      0,
				BuyCostCents:       tc.buyCostCents,
				ReviewedPriceCents: tc.reviewedPriceCents,
				CLValueCents:       tc.clValueCents,
			}
			reason := EvaluateHoldTriggers(p, cfg)
			held := reason != ""
			if held != tc.wantHeld {
				t.Errorf("held=%v, want %v (reason=%q)", held, tc.wantHeld, reason)
			}
			if tc.wantContains != "" && !strings.Contains(reason, tc.wantContains) {
				t.Errorf("reason %q does not contain %q", reason, tc.wantContains)
			}
		})
	}
}

// TestEvaluateHoldTriggers_RePush covers the three re-push hold triggers
// (DHInventoryID != 0 path).
func TestEvaluateHoldTriggers_RePush(t *testing.T) {
	// highPct / highCents are sentinel values large enough to prevent a trigger from firing
	// unintentionally when another trigger is under test.
	const highPct = 9999
	const highCents = 999999

	tests := []struct {
		name                     string
		dhInventoryID            int
		dhListingPriceCents      int
		reviewedPriceCents       int
		clValueCents             int
		lastSoldCents            int
		swingPctThreshold        int
		swingMinCents            int
		disagreementPctThreshold int
		unreviewedChangePct      int
		unreviewedChangeMin      int
		wantHeld                 bool
		wantContains             string
	}{
		{
			name:                     "no last pushed price: no hold",
			dhInventoryID:            99,
			dhListingPriceCents:      0,
			clValueCents:             15000,
			swingPctThreshold:        30,
			swingMinCents:            100,
			disagreementPctThreshold: 30,
			unreviewedChangePct:      30,
			unreviewedChangeMin:      100,
			wantHeld:                 false,
		},
		{
			name:                     "price swing above threshold: hold",
			dhInventoryID:            99,
			dhListingPriceCents:      10000,
			reviewedPriceCents:       18000, // +80% swing
			swingPctThreshold:        30,
			swingMinCents:            100,
			disagreementPctThreshold: highPct, // disable disagreement
			unreviewedChangePct:      highPct, // disable unreviewed CL
			unreviewedChangeMin:      100,
			wantHeld:                 true,
			wantContains:             "price_swing",
		},
		{
			name:                     "price swing above pct but below min cents: no hold",
			dhInventoryID:            99,
			dhListingPriceCents:      10000,
			reviewedPriceCents:       18000, // +80%
			swingPctThreshold:        30,
			swingMinCents:            highCents, // min cents far above actual delta
			disagreementPctThreshold: highPct,
			unreviewedChangePct:      highPct,
			unreviewedChangeMin:      highCents,
			wantHeld:                 false,
		},
		{
			name:                     "source disagreement above threshold: hold",
			dhInventoryID:            99,
			dhListingPriceCents:      12000,
			reviewedPriceCents:       12000, // newValue > 0 so triggers run
			clValueCents:             10000,
			lastSoldCents:            20000,   // cl=10000 vs lastSold=20000 = 50% diff
			swingPctThreshold:        highPct, // disable swing
			swingMinCents:            highCents,
			disagreementPctThreshold: 30,
			unreviewedChangePct:      highPct, // disable unreviewed CL
			unreviewedChangeMin:      highCents,
			wantHeld:                 true,
			wantContains:             "source_disagreement",
		},
		{
			name:                     "only one price source: no source disagreement",
			dhInventoryID:            99,
			dhListingPriceCents:      10000,
			reviewedPriceCents:       12000,   // only one source present (no CL/lastSold)
			swingPctThreshold:        highPct, // disable swing
			swingMinCents:            highCents,
			disagreementPctThreshold: 20,
			unreviewedChangePct:      highPct,
			unreviewedChangeMin:      highCents,
			wantHeld:                 false,
		},
		{
			// This trigger is largely vestigial now: with reviewed-only resolver,
			// EvaluateHoldTriggers short-circuits when there's no reviewed price,
			// so checkUnreviewedCLChange only fires if the operator manually
			// clears reviewedPriceCents back to 0 between pushes (rare).
			name:                     "unreviewed CL change requires zero reviewed price → no hold (newValue==0)",
			dhInventoryID:            99,
			dhListingPriceCents:      10000,
			reviewedPriceCents:       0,
			clValueCents:             18000,   // +80% change — but ignored, newValue==0
			swingPctThreshold:        highPct, // disable swing
			swingMinCents:            highCents,
			disagreementPctThreshold: highPct, // disable disagreement
			unreviewedChangePct:      30,
			unreviewedChangeMin:      100,
			wantHeld:                 false,
		},
		{
			name:                     "reviewed price present: unreviewed CL check skipped",
			dhInventoryID:            99,
			dhListingPriceCents:      10000,
			reviewedPriceCents:       10500, // market value = 10500 (+5% swing, below swing threshold)
			clValueCents:             25000, // large CL change — but ignored because reviewed
			swingPctThreshold:        highPct,
			swingMinCents:            highCents,
			disagreementPctThreshold: highPct,
			unreviewedChangePct:      5,
			unreviewedChangeMin:      100,
			wantHeld:                 false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := inventory.DHPushConfig{
				SwingPctThreshold:            tc.swingPctThreshold,
				SwingMinCents:                tc.swingMinCents,
				DisagreementPctThreshold:     tc.disagreementPctThreshold,
				UnreviewedChangePctThreshold: tc.unreviewedChangePct,
				UnreviewedChangeMinCents:     tc.unreviewedChangeMin,
			}
			if cfg.InitialPushValueFloorPct == 0 {
				cfg.InitialPushValueFloorPct = 50
			}

			p := &inventory.Purchase{
				DHInventoryID:       tc.dhInventoryID,
				DHListingPriceCents: tc.dhListingPriceCents,
				ReviewedPriceCents:  tc.reviewedPriceCents,
				CLValueCents:        tc.clValueCents,
			}
			// MarketSnapshotData is a deprecated embedded struct; accessing it directly
			// is the only way to set LastSoldCents in tests until the field is promoted.
			p.MarketSnapshotData.LastSoldCents = tc.lastSoldCents //nolint:staticcheck
			reason := EvaluateHoldTriggers(p, cfg)
			held := reason != ""
			if held != tc.wantHeld {
				t.Errorf("held=%v, want %v (reason=%q)", held, tc.wantHeld, reason)
			}
			if tc.wantContains != "" && !strings.Contains(reason, tc.wantContains) {
				t.Errorf("reason %q does not contain %q", reason, tc.wantContains)
			}
		})
	}
}
