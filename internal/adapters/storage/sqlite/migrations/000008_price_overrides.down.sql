-- Requires SQLite >= 3.35.0 for ALTER TABLE ... DROP COLUMN.
-- The vendored go-sqlite3 bundles SQLite 3.51.3; system builds must also meet this minimum.
ALTER TABLE campaign_purchases DROP COLUMN override_price_cents;
ALTER TABLE campaign_purchases DROP COLUMN override_source;
ALTER TABLE campaign_purchases DROP COLUMN override_set_at;
ALTER TABLE campaign_purchases DROP COLUMN ai_suggested_price_cents;
ALTER TABLE campaign_purchases DROP COLUMN ai_suggested_at;
