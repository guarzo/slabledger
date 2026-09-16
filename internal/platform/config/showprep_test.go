package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestShowPrepGateIndependentOfLegacyCL(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        bool
	}{{"default", "", true}, {"disabled", "false", false}, {"enabled", "true", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHOW_PREP_REFRESH_ENABLED", tc.value)
			t.Setenv("CARDLADDER_REFRESH_ENABLED", "false")
			cfg := FromEnv(Default())
			require.Equal(t, tc.want, cfg.ShowPrepRefresh.Enabled)
			require.False(t, cfg.CardLadder.Enabled)
		})
	}
}
