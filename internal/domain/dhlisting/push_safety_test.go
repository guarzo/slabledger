package dhlisting

import (
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// TestResolveListingPriceCents covers the reviewed↔override resolution rules:
// both fields hold operator-committed prices; when both are set, the newest
// commit wins via RFC3339 string comparison on reviewed_at vs override_set_at.
func TestResolveListingPriceCents(t *testing.T) {
	const oldTS = "2026-01-01T00:00:00Z"
	const newTS = "2026-04-21T12:00:00Z"

	tests := []struct {
		name               string
		reviewedPriceCents int
		reviewedAt         string
		overridePriceCents int
		overrideSetAt      string
		clValueCents       int
		want               int
	}{
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &inventory.Purchase{
				ReviewedPriceCents: tc.reviewedPriceCents,
				ReviewedAt:         tc.reviewedAt,
				OverridePriceCents: tc.overridePriceCents,
				OverrideSetAt:      tc.overrideSetAt,
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
