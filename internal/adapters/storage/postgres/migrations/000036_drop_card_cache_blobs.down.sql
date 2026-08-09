-- Rollback: re-add the five dh_card_cache blob columns (structure only -- the
-- dropped values are not restored).
--
-- Column definitions are the originals from 000001_initial_schema: all five are
-- plain nullable TEXT with no default, no constraint and no index, so restoring
-- them is a straight ADD COLUMN.
--
-- The restored columns land at the end of the table rather than in their
-- 000001 positions, between demand_data_quality and analytics_computed_at.
-- That is deliberate and safe: every statement against dh_card_cache names its
-- columns explicitly (there is no SELECT * and no positional INSERT anywhere in
-- the adapter), so ordinal position is not part of the contract. Rebuilding the
-- table to recover the original order would cost a full copy to fix something
-- nothing reads.
--
-- This restores the schema a pre-SLA-41 binary expects, so that binary can read
-- and write the columns again -- but it comes back to a table where every row
-- has NULL in all five. That is the same state SLA-41 would have left them in
-- anyway after any refresh cycle, since the post-SLA-41 upsert rewrites rows
-- without touching these columns. Nothing reads them for correctness; they were
-- an opaque passthrough cache.

ALTER TABLE dh_card_cache
    ADD COLUMN IF NOT EXISTS demand_json             TEXT,
    ADD COLUMN IF NOT EXISTS velocity_json           TEXT,
    ADD COLUMN IF NOT EXISTS trend_json              TEXT,
    ADD COLUMN IF NOT EXISTS saturation_json         TEXT,
    ADD COLUMN IF NOT EXISTS price_distribution_json TEXT;
