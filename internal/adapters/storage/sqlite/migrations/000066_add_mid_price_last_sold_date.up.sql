-- 000066_add_mid_price_last_sold_date.up.sql
ALTER TABLE campaign_purchases ADD COLUMN mid_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN last_sold_date TEXT NOT NULL DEFAULT '';
