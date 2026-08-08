DROP INDEX IF EXISTS idx_campaign_purchases_attribution_source;
ALTER TABLE campaign_purchases DROP CONSTRAINT IF EXISTS campaign_purchases_attribution_source_check;
ALTER TABLE campaign_purchases DROP COLUMN IF EXISTS attribution_source;
ALTER TABLE campaign_purchases DROP COLUMN IF EXISTS psa_campaign_name;
