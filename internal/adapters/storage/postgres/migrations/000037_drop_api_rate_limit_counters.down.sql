-- Rollback: restore the three call-counter columns.
--
-- Types and defaults are the originals from 000001_initial_schema
-- (BIGINT DEFAULT 0), not the INTEGER that docs/SCHEMA.md claimed. Existing
-- rows come back at 0, which is the value they held before the drop — nothing
-- ever wrote anything else — so up-then-down-then-up is idempotent in content
-- as well as in structure.
--
-- Column order is not restored: these three sat between provider and
-- last_429_at in 000001, and ADD COLUMN appends. Nothing in the codebase reads
-- this table positionally (both statements name their columns), so ordinal
-- position carries no meaning here.

ALTER TABLE api_rate_limits
    ADD COLUMN IF NOT EXISTS calls_last_minute BIGINT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS calls_last_hour   BIGINT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS calls_last_day    BIGINT DEFAULT 0;
