-- Drop the Market Movers (MM) integration.
-- MM pricing was retired in favor of Card Ladder, which covers ~98% of inventory
-- and provides per-card values without MM's variant-contaminated collectible buckets.
-- See the 2026-07 MM removal. This drops the two MM-only tables and the seven
-- MM columns (plus their index) on campaign_purchases.

DROP INDEX IF EXISTS idx_mm_sales_comps_lookup;
DROP TABLE IF EXISTS mm_sales_comps;
DROP TABLE IF EXISTS mm_card_mappings;

DROP INDEX IF EXISTS idx_purchases_mm_last_error;

ALTER TABLE campaign_purchases
    DROP COLUMN IF EXISTS mm_value_cents,
    DROP COLUMN IF EXISTS mm_trend_pct,
    DROP COLUMN IF EXISTS mm_sales_30d,
    DROP COLUMN IF EXISTS mm_active_low_cents,
    DROP COLUMN IF EXISTS mm_value_updated_at,
    DROP COLUMN IF EXISTS mm_last_error,
    DROP COLUMN IF EXISTS mm_last_error_at;
