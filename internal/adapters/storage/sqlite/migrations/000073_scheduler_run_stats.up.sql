-- scheduler_run_stats: last-completed-run snapshot for a named scheduler.
-- Scheduler stats used to live only in memory on the scheduler struct, which
-- meant every server restart wiped the `lastRun` the admin UI depends on.
-- Persisting a single row per scheduler keeps the admin UI hydrated across
-- restarts. History isn't required — only the most recent run drives the UI.
CREATE TABLE scheduler_run_stats (
    name TEXT PRIMARY KEY,            -- e.g. 'card_ladder_refresh', 'market_movers_refresh'
    last_run_at TEXT NOT NULL,        -- RFC3339 UTC
    duration_ms INTEGER NOT NULL,
    stats_json TEXT NOT NULL,         -- opaque JSON blob of the scheduler's run stats struct
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
