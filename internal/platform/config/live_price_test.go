package config

import "testing"

func TestLivePriceEnablementIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        bool
	}{
		{"default", "", false}, {"enabled", "true", true}, {"disabled", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DH_LIVE_PRICE_ENABLED", tc.value)
			cfg := FromEnv(Default())
			if cfg.DHPriceSync.LivePriceEnabled != tc.want {
				t.Fatalf("live-price enabled=%v want%v", cfg.DHPriceSync.LivePriceEnabled, tc.want)
			}
		})
	}
}
