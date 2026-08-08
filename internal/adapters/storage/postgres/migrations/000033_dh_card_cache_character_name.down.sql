DROP INDEX IF EXISTS idx_card_cache_character_name;

ALTER TABLE dh_card_cache DROP COLUMN IF EXISTS character_name;
