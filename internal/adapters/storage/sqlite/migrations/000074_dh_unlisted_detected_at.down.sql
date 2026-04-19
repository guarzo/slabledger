-- 000074_dh_unlisted_detected_at.down.sql
-- Requires SQLite 3.35.0+ (released 2021-03-12) for DROP COLUMN support.
ALTER TABLE campaign_purchases DROP COLUMN dh_unlisted_detected_at;
