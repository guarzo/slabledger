package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 000003 dropped these two indexes in a "never scanned" sweep. That was correct
// on the evidence available then: dh_state_events was write-only, so nothing
// ever looked a row up by purchase or cert and the indexes were pure write cost.
// SLA-58 adds the read path (GET /api/dh/events and the retention prune), which
// is what makes them earn their keep again.
const (
	dhEventsPurchaseIndex = "idx_dh_state_events_purchase"
	dhEventsCertIndex     = "idx_dh_state_events_cert"
)

func TestMigration000032_LookupIndexesExist(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		index   string
		columns []string
	}{
		{name: "purchase lookup", index: dhEventsPurchaseIndex, columns: []string{"purchase_id", "event_at"}},
		{name: "cert lookup", index: dhEventsCertIndex, columns: []string{"cert_number", "event_at"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.columns, indexColumns(ctx, t, db, "dh_state_events", tt.index),
				"%s should cover %v after migration 000032", tt.index, tt.columns)
		})
	}
}

// TestMigration000032_DownDropsIndexes proves the rollback is not one-way.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000032_DownDropsIndexes(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 31), "roll back to version 31")

	assert.Nil(t, indexColumns(ctx, t, db, "dh_state_events", dhEventsPurchaseIndex),
		"down migration should drop %s", dhEventsPurchaseIndex)
	assert.Nil(t, indexColumns(ctx, t, db, "dh_state_events", dhEventsCertIndex),
		"down migration should drop %s", dhEventsCertIndex)

	require.NoError(t, migrateToVersion(db, 32), "re-apply version 32")

	assert.Equal(t, []string{"purchase_id", "event_at"},
		indexColumns(ctx, t, db, "dh_state_events", dhEventsPurchaseIndex),
		"%s should return after re-applying 000032", dhEventsPurchaseIndex)
	assert.Equal(t, []string{"cert_number", "event_at"},
		indexColumns(ctx, t, db, "dh_state_events", dhEventsCertIndex),
		"%s should return after re-applying 000032", dhEventsCertIndex)
}

// indexColumns returns the columns covered by a named index, in ordinal order,
// or nil when no such index exists.
func indexColumns(ctx context.Context, t *testing.T, db *DB, table, index string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT a.attname
		FROM pg_index      i
		JOIN pg_class      c ON c.oid = i.indexrelid
		JOIN pg_class      r ON r.oid = i.indrelid
		JOIN pg_namespace  n ON n.oid = r.relnamespace
		JOIN unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord) ON TRUE
		JOIN pg_attribute  a ON a.attrelid = r.oid AND a.attnum = k.attnum
		WHERE n.nspname = 'public' AND r.relname = $1 AND c.relname = $2
		ORDER BY k.ord`, table, index)
	require.NoError(t, err, "query index %s on %s", index, table)
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		columns = append(columns, column)
	}
	require.NoError(t, rows.Err())
	return columns
}
