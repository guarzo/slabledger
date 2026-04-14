-- 000066_add_mid_price_last_sold_date.down.sql
-- Requires SQLite 3.35.0+ (released 2021-03-12) for DROP COLUMN support.
ALTER TABLE campaign_purchases DROP COLUMN mid_price_cents;
ALTER TABLE campaign_purchases DROP COLUMN last_sold_date;
