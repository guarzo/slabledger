DROP INDEX IF EXISTS idx_sales_order_id;
ALTER TABLE campaign_sales DROP COLUMN order_id;

DROP INDEX IF EXISTS idx_purchases_dh_cert_status;
ALTER TABLE campaign_purchases DROP COLUMN dh_channels_json;
ALTER TABLE campaign_purchases DROP COLUMN dh_listing_price_cents;
ALTER TABLE campaign_purchases DROP COLUMN dh_cert_status;
ALTER TABLE campaign_purchases DROP COLUMN dh_inventory_id;
ALTER TABLE campaign_purchases DROP COLUMN dh_card_id;
