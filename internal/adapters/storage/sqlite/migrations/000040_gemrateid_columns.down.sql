DROP INDEX IF EXISTS idx_purchases_gem_rate_id;
ALTER TABLE campaign_purchases DROP COLUMN gem_rate_id;
ALTER TABLE campaign_purchases DROP COLUMN psa_spec_id;

-- Revert cl_sales_comps condition column and index
DROP INDEX IF EXISTS idx_cl_sales_comps_item;
ALTER TABLE cl_sales_comps DROP COLUMN condition;
CREATE UNIQUE INDEX idx_cl_sales_comps_item ON cl_sales_comps(gem_rate_id, item_id);
