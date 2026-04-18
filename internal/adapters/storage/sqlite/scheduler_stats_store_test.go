package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerStatsStore_UpsertAndRead(t *testing.T) {
	// Each case runs against a fresh in-memory DB and store. Writes from one
	// case don't leak into the next, so the table reads top-down as four
	// independent scenarios of the Save/Get contract.
	type operation struct {
		op           string // "save" or "get"
		row          SchedulerRunStats
		name         string // for "get"
		wantNil      bool
		wantDuration int64
		wantJSON     string
	}

	cases := []struct {
		name string
		ops  []operation
	}{
		{
			name: "empty store returns nil",
			ops: []operation{
				{op: "get", name: "card_ladder_refresh", wantNil: true},
			},
		},
		{
			name: "save then read roundtrips fields",
			ops: []operation{
				{op: "save", row: SchedulerRunStats{
					Name:       "card_ladder_refresh",
					LastRunAt:  time.Date(2026, 4, 18, 4, 1, 0, 0, time.UTC),
					DurationMs: 12345,
					StatsJSON:  `{"updated":196,"resolved":4}`,
				}},
				{op: "get", name: "card_ladder_refresh", wantDuration: 12345, wantJSON: `{"updated":196,"resolved":4}`},
			},
		},
		{
			name: "upsert overwrites same-name row",
			ops: []operation{
				{op: "save", row: SchedulerRunStats{
					Name:       "card_ladder_refresh",
					LastRunAt:  time.Date(2026, 4, 18, 4, 1, 0, 0, time.UTC),
					DurationMs: 12345,
					StatsJSON:  `{"updated":196}`,
				}},
				{op: "save", row: SchedulerRunStats{
					Name:       "card_ladder_refresh",
					LastRunAt:  time.Date(2026, 4, 18, 19, 27, 0, 0, time.UTC),
					DurationMs: 99999,
					StatsJSON:  `{"updated":84,"resolved":23}`,
				}},
				{op: "get", name: "card_ladder_refresh", wantDuration: 99999, wantJSON: `{"updated":84,"resolved":23}`},
			},
		},
		{
			name: "per-name isolation — different scheduler doesn't collide",
			ops: []operation{
				{op: "save", row: SchedulerRunStats{
					Name:       "card_ladder_refresh",
					LastRunAt:  time.Date(2026, 4, 18, 4, 1, 0, 0, time.UTC),
					DurationMs: 12345,
					StatsJSON:  `{}`,
				}},
				{op: "save", row: SchedulerRunStats{
					Name:       "market_movers_refresh",
					LastRunAt:  time.Date(2026, 4, 18, 5, 0, 0, 0, time.UTC),
					DurationMs: 1,
					StatsJSON:  `{"provider":"mm"}`,
				}},
				{op: "get", name: "card_ladder_refresh", wantDuration: 12345, wantJSON: `{}`},
				{op: "get", name: "market_movers_refresh", wantDuration: 1, wantJSON: `{"provider":"mm"}`},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close() //nolint:errcheck
			store := NewSchedulerStatsStore(db.DB)
			ctx := context.Background()

			for i, op := range tc.ops {
				switch op.op {
				case "save":
					require.NoError(t, store.Save(ctx, op.row), "op %d: save", i)
				case "get":
					got, err := store.Get(ctx, op.name)
					require.NoError(t, err, "op %d: get", i)
					if op.wantNil {
						require.Nil(t, got, "op %d: expected nil", i)
						continue
					}
					require.NotNil(t, got, "op %d: expected non-nil", i)
					require.Equal(t, op.name, got.Name, "op %d: name", i)
					require.Equal(t, op.wantDuration, got.DurationMs, "op %d: duration", i)
					require.Equal(t, op.wantJSON, got.StatsJSON, "op %d: stats json", i)
				default:
					t.Fatalf("unknown op %q", op.op)
				}
			}
		})
	}
}
