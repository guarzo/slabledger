package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const apiRateLimitsTable = "api_rate_limits"

// apiRateLimitCounters are the three columns 000037 removes. They were carried
// from 000001 at BIGINT DEFAULT 0 and never written by anything.
var apiRateLimitCounters = []string{"calls_last_minute", "calls_last_hour", "calls_last_day"}

// TestMigration000037_CountersDropped is the acceptance criterion: after the
// full embedded migration set applies, api_rate_limits holds exactly the four
// columns the code actually names.
//
// Asserting the whole column map rather than three absences is deliberate — an
// absence-only assertion would still pass if the up migration dropped a column
// the UPSERT depends on.
func TestMigration000037_CountersDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	assert.Equal(t,
		map[string]string{
			"provider":      "text",
			"last_429_at":   "timestamp without time zone",
			"blocked_until": "timestamp without time zone",
			"updated_at":    "timestamp without time zone",
		},
		tableColumns(ctx, t, db, apiRateLimitsTable),
		"after 000037, %s should hold only the columns UpdateRateLimit and "+
			"IsProviderBlocked name", apiRateLimitsTable)
}

// TestMigration000037_ViewKeepsItsOwnCallsLastHour guards the name collision
// that makes this migration look dangerous and is not.
//
// api_usage_summary computes a column called calls_last_hour over api_calls.
// It is a different object that happens to share a name with one of the dropped
// table columns, and it is the one GetAPIUsage reads (api_tracker.go:52 selects
// FROM api_usage_summary). A future reader grepping for calls_last_hour will hit
// both; this test pins which survived, end to end through the real query, so a
// migration that took the view's column instead fails loudly here.
func TestMigration000037_ViewKeepsItsOwnCallsLastHour(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	const provider = "sla81-view-probe"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM api_calls WHERE provider = $1`, provider)
	})

	tracker := NewDBTracker(db)
	require.NoError(t, tracker.RecordAPICall(ctx, &pricing.APICallRecord{
		Provider:   provider,
		Endpoint:   "/probe",
		StatusCode: 200,
		LatencyMS:  12,
		Timestamp:  time.Now().UTC(),
	}), "record an api_calls row inside the view's window")

	stats, err := tracker.GetAPIUsage(ctx, provider)
	require.NoError(t, err, "GetAPIUsage must still resolve after 000037")
	assert.Positive(t, stats.CallsLastHour,
		"CallsLastHour comes from the api_usage_summary view over api_calls, "+
			"not from the api_rate_limits column 000037 dropped")
}

// TestMigration000037_DownRestoresCounters pins the shape the rollback restores,
// which the generic up/down/up roundtrip cannot: that test proves the SQL is
// valid, not that it puts the columns back with their original types.
//
// BIGINT is the point of the type assertion. docs/SCHEMA.md described these as
// INTEGER before this change; 000001 declares BIGINT. A down migration written
// from the doc rather than the DDL would restore the wrong width and the
// roundtrip test would not notice.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000037_DownRestoresCounters(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 36), "roll back to version 36")

	columns := tableColumns(ctx, t, db, apiRateLimitsTable)
	for _, counter := range apiRateLimitCounters {
		assert.Equal(t, "bigint", columns[counter],
			"000037's down migration should restore %s as BIGINT, per 000001", counter)
	}

	// The DEFAULT comes back too, so a row inserted against the rolled-back
	// schema by the UPSERT (which names none of these) still lands at 0 rather
	// than NULL — the value they held before the drop.
	for _, counter := range apiRateLimitCounters {
		var def *string
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT column_default
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
			apiRateLimitsTable, counter).Scan(&def),
			"read column_default for %s", counter)
		require.NotNil(t, def, "%s should be restored with a DEFAULT", counter)
		assert.Equal(t, "0", *def, "%s should default to 0, as in 000001", counter)
	}

	// Re-applying 000037 removes all three again, proving the rollback is not
	// one-way — up-then-down-then-up is idempotent.
	require.NoError(t, migrateToVersion(db, 37), "re-apply version 37")
	after := tableColumns(ctx, t, db, apiRateLimitsTable)
	for _, counter := range apiRateLimitCounters {
		assert.NotContains(t, after, counter,
			"%s should be gone again after re-applying 000037", counter)
	}
}
