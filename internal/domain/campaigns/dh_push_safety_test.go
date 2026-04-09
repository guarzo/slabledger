package campaigns

import (
	"strings"
	"testing"
)

func TestResolveMarketValueCents(t *testing.T) {
	tests := []struct {
		name     string
		purchase Purchase
		want     int
	}{
		{
			name:     "reviewed price takes priority",
			purchase: Purchase{ReviewedPriceCents: 5000, CLValueCents: 3000},
			want:     5000,
		},
		{
			name:     "falls back to CL value",
			purchase: Purchase{CLValueCents: 3000},
			want:     3000,
		},
		{
			name:     "returns zero when nothing set",
			purchase: Purchase{},
			want:     0,
		},
		{
			name:     "reviewed price zero falls to CL",
			purchase: Purchase{ReviewedPriceCents: 0, CLValueCents: 4000},
			want:     4000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveMarketValueCents(&tt.purchase)
			if got != tt.want {
				t.Errorf("ResolveMarketValueCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEvaluateHoldTriggers(t *testing.T) {
	defaultCfg := DHPushConfig{
		SwingPctThreshold:            20,
		SwingMinCents:                5000,
		DisagreementPctThreshold:     25,
		UnreviewedChangePctThreshold: 15,
		UnreviewedChangeMinCents:     3000,
		InitialPushValueFloorPct:     50,
	}

	tests := []struct {
		name         string
		p            Purchase
		cfg          DHPushConfig
		wantHeld     bool
		wantContains string
	}{
		{
			name:     "initial push - no buy cost set - not held",
			p:        Purchase{CLValueCents: 10000, DHInventoryID: 0},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "initial push - market value far below buy cost - held",
			p: Purchase{
				CLValueCents:  3000,  // $30 market value
				BuyCostCents:  10000, // $100 buy cost (70% below)
				DHInventoryID: 0,
			},
			cfg:          defaultCfg,
			wantHeld:     true,
			wantContains: "initial_value_mismatch",
		},
		{
			name: "initial push - market value near buy cost - not held",
			p: Purchase{
				CLValueCents:  8000,  // $80 market value
				BuyCostCents:  10000, // $100 buy cost (20% below, within threshold)
				DHInventoryID: 0,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "initial push - no buy cost - not held",
			p: Purchase{
				CLValueCents:  5000,
				BuyCostCents:  0,
				DHInventoryID: 0,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "initial push - market value above buy cost - not held",
			p: Purchase{
				CLValueCents:  15000,
				BuyCostCents:  10000,
				DHInventoryID: 0,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "small price change - not held",
			p: Purchase{
				CLValueCents:        10500,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "large swing triggers hold",
			p: Purchase{
				CLValueCents:        18000,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:          defaultCfg,
			wantHeld:     true,
			wantContains: "price_swing",
		},
		{
			name: "swing pct met but absolute below threshold - not held",
			p: Purchase{
				CLValueCents:        1500,
				DHInventoryID:       1,
				DHListingPriceCents: 1000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "source disagreement - CL vs reviewed",
			p: Purchase{
				CLValueCents:        45000,
				ReviewedPriceCents:  30000,
				DHInventoryID:       1,
				DHListingPriceCents: 30000,
			},
			cfg:          defaultCfg,
			wantHeld:     true,
			wantContains: "source_disagreement",
		},
		{
			name: "unreviewed CL change triggers hold",
			p: Purchase{
				CLValueCents:        14000,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:          defaultCfg,
			wantHeld:     true,
			wantContains: "unreviewed_cl_change",
		},
		{
			name: "unreviewed small CL change - not held",
			p: Purchase{
				CLValueCents:        10200,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "reviewed price set and stable - not held even with CL change",
			p: Purchase{
				CLValueCents:        12000,
				ReviewedPriceCents:  10000,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := EvaluateHoldTriggers(&tt.p, tt.cfg)
			held := reason != ""
			if held != tt.wantHeld {
				t.Errorf("held = %v, want %v (reason=%q)", held, tt.wantHeld, reason)
			}
			if tt.wantContains != "" && held && !strings.Contains(reason, tt.wantContains) {
				t.Errorf("reason = %q, want it to contain %q", reason, tt.wantContains)
			}
		})
	}
}
