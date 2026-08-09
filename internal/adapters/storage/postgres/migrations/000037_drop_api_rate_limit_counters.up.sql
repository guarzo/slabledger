-- Drop the three never-populated call-counter columns from api_rate_limits.
--
-- calls_last_minute, calls_last_hour and calls_last_day have been carried since
-- 000001 at their DEFAULT 0. Only two statements in the repository touch this
-- table, and neither names a counter: the UPSERT in UpdateRateLimit writes
-- (provider, blocked_until, last_429_at, updated_at), and IsProviderBlocked
-- selects blocked_until. No dynamically-assembled SQL exists in the postgres
-- adapter, so an explicit column list is the only channel by which a column
-- name reaches the database — there is no third writer.
--
-- The trap worth naming, because the next reader will hit it too: the codebase
-- does read a column called calls_last_hour, at api_tracker.go:52. That one is
-- computed by the api_usage_summary VIEW over api_calls, and is a different
-- object that happens to share a name. GetAPIUsage selects FROM
-- api_usage_summary, not FROM api_rate_limits, so it is untouched by this
-- migration. Per-window call counts were always derived there; these columns
-- are a second, never-populated copy of the same idea, and reading the table
-- would have reported zero traffic forever.
--
-- No data is lost that was not already zero, and nothing depends on the
-- columns: no view selects from api_rate_limits, and the table has no indexes
-- beyond its primary key on provider. RLS is unaffected — 000003 and 000028
-- act on the table, not on individual columns.

ALTER TABLE api_rate_limits
    DROP COLUMN IF EXISTS calls_last_minute,
    DROP COLUMN IF EXISTS calls_last_hour,
    DROP COLUMN IF EXISTS calls_last_day;
