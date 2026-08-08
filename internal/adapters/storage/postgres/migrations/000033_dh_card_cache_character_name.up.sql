-- Attribute each cached card to its DH character (SLA-63).
--
-- Character-level data_quality has no source in the DH API: data_quality is
-- reported per card (demand_signals) and never per character (top_characters
-- and character_demand both omit it). Confirmed against the live API on
-- 2026-08-08 -- the `top_cards` field the scheduler's NOTE claimed would supply
-- the mapping does not exist on any variant of the top_characters response.
--
-- The mapping is instead obtained by seeding character_demand with a single
-- card id, which attributes that one card to its character. That costs one HTTP
-- call per card, so the result is cached here rather than re-fetched each run:
-- a card's character does not change.
ALTER TABLE dh_card_cache ADD COLUMN character_name TEXT;

-- The aggregation query filters on character_name IS NOT NULL and groups by it.
-- Partial index because unattributed rows are never selected, and every row is
-- unattributed until the scheduler backfills it.
CREATE INDEX idx_card_cache_character_name
    ON dh_card_cache(character_name)
    WHERE character_name IS NOT NULL;
