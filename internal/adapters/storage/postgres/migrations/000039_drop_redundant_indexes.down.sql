-- Rollback: recreate the three indexes exactly as 000001 declared them.
--
-- Definitions are copied verbatim from 000001_initial_schema.up.sql (lines 234,
-- 367 and 591). Unlike the views in 000038, no later migration ever altered
-- these, so 000001 is the correct source rather than a stale one.
--
-- Index sort direction is reproduced exactly even though it is what made
-- idx_access_log_covering redundant in the first place: a rollback restores the
-- prior state, it does not apply a better judgement about it.
--
-- No CONCURRENTLY. golang-migrate wraps each migration in a transaction and
-- CREATE INDEX CONCURRENTLY cannot run inside one. These take a write lock on
-- their tables for the duration of the build, which for a rollback of three
-- indexes is the accepted cost -- the same as every other index in this set.

CREATE INDEX IF NOT EXISTS idx_access_log_covering
    ON card_access_log(card_name, set_name, card_number, accessed_at);

CREATE INDEX IF NOT EXISTS idx_dh_suggestions_date
    ON dh_suggestions(suggestion_date);

CREATE INDEX IF NOT EXISTS idx_purchases_campaign
    ON campaign_purchases(campaign_id);
