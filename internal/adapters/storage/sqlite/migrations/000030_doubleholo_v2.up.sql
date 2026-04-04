-- Add DH v2 tracking fields to purchases
ALTER TABLE campaign_purchases ADD COLUMN dh_card_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN dh_inventory_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN dh_cert_status TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN dh_listing_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN dh_channels_json TEXT NOT NULL DEFAULT '';

-- Index for querying by DH cert resolution status (reconciliation, push-to-DH)
CREATE INDEX idx_purchases_dh_cert_status ON campaign_purchases(dh_cert_status)
    WHERE dh_cert_status != '';

-- Add order_id to sales for DH order poll idempotency
ALTER TABLE campaign_sales ADD COLUMN order_id TEXT NOT NULL DEFAULT '';

-- Unique constraint on order_id (only for non-empty values, allows multiple empty)
CREATE UNIQUE INDEX idx_sales_order_id ON campaign_sales(order_id)
    WHERE order_id != '';
