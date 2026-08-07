ALTER TABLE campaign_purchases ADD COLUMN IF NOT EXISTS psa_campaign_name TEXT;
ALTER TABLE campaign_purchases ADD COLUMN IF NOT EXISTS attribution_source TEXT;

-- Every pre-existing row's campaign came from FindMatchingCampaign or a hand
-- assignment we can no longer distinguish. 'inferred' is the weaker, safer claim.
UPDATE campaign_purchases SET attribution_source = 'inferred' WHERE attribution_source IS NULL;

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_attribution_source
	ON campaign_purchases (attribution_source);
