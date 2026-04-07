-- Add gemRateID and psaSpecID to campaign_purchases
ALTER TABLE campaign_purchases ADD COLUMN gem_rate_id TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN psa_spec_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_purchases_gem_rate_id ON campaign_purchases(gem_rate_id);

-- Add condition to cl_sales_comps for grade-specific comp storage
ALTER TABLE cl_sales_comps ADD COLUMN condition TEXT NOT NULL DEFAULT '';

-- Recreate the unique index to include condition
DROP INDEX IF EXISTS idx_cl_sales_comps_item;
CREATE UNIQUE INDEX idx_cl_sales_comps_item ON cl_sales_comps(gem_rate_id, condition, item_id);
