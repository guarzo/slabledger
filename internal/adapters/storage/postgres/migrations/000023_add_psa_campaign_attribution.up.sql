ALTER TABLE campaign_purchases ADD COLUMN IF NOT EXISTS psa_campaign_name TEXT;
ALTER TABLE campaign_purchases ADD COLUMN IF NOT EXISTS attribution_source TEXT;

-- Every pre-existing row's campaign came from FindMatchingCampaign or a hand
-- assignment we can no longer distinguish. 'inferred' is the weaker, safer claim.
UPDATE campaign_purchases SET attribution_source = 'inferred' WHERE attribution_source IS NULL;

-- Postgres has no ADD CONSTRAINT IF NOT EXISTS; guard manually so this
-- migration is re-runnable. Validated after the backfill above so it only
-- ever sees clean data. NULL still passes (no NOT NULL/DEFAULT here by
-- design — NULL is the canary for a missed write path; see Task 9).
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'campaign_purchases_attribution_source_check'
	) THEN
		ALTER TABLE campaign_purchases
			ADD CONSTRAINT campaign_purchases_attribution_source_check
			CHECK (attribution_source IS NULL
			       OR attribution_source IN ('psa', 'inferred', 'manual'));
	END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_attribution_source
	ON campaign_purchases (attribution_source);
