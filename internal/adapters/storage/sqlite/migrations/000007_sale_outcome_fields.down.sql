-- SQLite does not support DROP COLUMN before 3.35.0; recreate table without the new columns.
CREATE TABLE campaign_sales_backup AS SELECT
    id, purchase_id, sale_channel, sale_price_cents, sale_fee_cents,
    sale_date, days_to_sell, net_profit_cents, created_at, updated_at,
    last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
    active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json
FROM campaign_sales;
DROP TABLE campaign_sales;
ALTER TABLE campaign_sales_backup RENAME TO campaign_sales;
CREATE UNIQUE INDEX IF NOT EXISTS idx_campaign_sales_purchase ON campaign_sales(purchase_id);
