DROP INDEX IF EXISTS idx_purchases_cl_last_error;
DROP INDEX IF EXISTS idx_purchases_mm_last_error;
ALTER TABLE campaign_purchases DROP COLUMN cl_value_updated_at;
ALTER TABLE campaign_purchases DROP COLUMN cl_last_error_at;
ALTER TABLE campaign_purchases DROP COLUMN cl_last_error;
ALTER TABLE campaign_purchases DROP COLUMN mm_last_error_at;
ALTER TABLE campaign_purchases DROP COLUMN mm_last_error;
