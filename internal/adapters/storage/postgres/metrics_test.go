package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNewDBStatsCollector_RegistersAndCollects(t *testing.T) {
	stats := sql.DBStats{
		MaxOpenConnections: 25,
		OpenConnections:    7,
		InUse:              3,
		Idle:               4,
		WaitCount:          11,
		MaxIdleClosed:      2,
		MaxLifetimeClosed:  1,
	}
	collector := NewDBStatsCollector("test", func() sql.DBStats { return stats })

	reg := prometheus.NewRegistry()
	err := reg.Register(collector)
	assert.NoError(t, err)

	// Expected names match our collector's FQ names.
	count, err := testutil.GatherAndCount(reg,
		"test_db_max_open_connections",
		"test_db_open_connections",
		"test_db_in_use_connections",
		"test_db_idle_connections",
		"test_db_wait_count_total",
		"test_db_max_idle_closed_total",
		"test_db_max_lifetime_closed_total",
	)
	assert.NoError(t, err)
	assert.Equal(t, 7, count)

	// Spot-check one gauge value.
	mf, err := reg.Gather()
	assert.NoError(t, err)
	var found bool
	for _, f := range mf {
		if strings.HasSuffix(f.GetName(), "_in_use_connections") {
			assert.InDelta(t, 3.0, f.Metric[0].Gauge.GetValue(), 0.0001)
			found = true
		}
	}
	assert.True(t, found)
}
