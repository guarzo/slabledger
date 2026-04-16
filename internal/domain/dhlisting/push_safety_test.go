package dhlisting

import (
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestResolveListingPriceCents covers all three priority paths.
func TestResolveListingPriceCents(t *testing.T) {
	tests := []struct {
		name               string
		reviewedPriceCents int
		clValueCents       int
		want               int
	}{
		{
			name:               "reviewed price returned",
			reviewedPriceCents: 5000,
			clValueCents:       3000,
			want:               5000,
		},
		{
			name:               "no reviewed price returns 0 (CL is no longer a fallback)",
			reviewedPriceCents: 0,
			clValueCents:       3000,
			want:               0,
		},
		{
			name:               "zero when both are zero",
			reviewedPriceCents: 0,
			clValueCents:       0,
			want:               0,
		},
		{
			name:               "reviewed price = 1 is non-zero, used",
			reviewedPriceCents: 1,
			clValueCents:       9999,
			want:               1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &inventory.Purchase{
				ReviewedPriceCents: tc.reviewedPriceCents,
				CLValueCents:       tc.clValueCents,
			}
			got := ResolveListingPriceCents(p)
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
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
			reviewedPriceCents:       12000,   // newValue > 0 so triggers run
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
