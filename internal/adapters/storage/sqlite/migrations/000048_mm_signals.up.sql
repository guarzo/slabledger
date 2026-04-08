ALTER TABLE campaign_purchases ADD COLUMN mm_trend_pct    REAL    NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN mm_sales_30d   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN mm_active_low_cents INTEGER NOT NULL DEFAULT 0;
