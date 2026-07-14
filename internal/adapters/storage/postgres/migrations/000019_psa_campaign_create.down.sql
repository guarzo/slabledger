UPDATE psa_campaign_push_queue SET psa_campaign_id = '' WHERE psa_campaign_id IS NULL;
ALTER TABLE psa_campaign_push_queue ALTER COLUMN psa_campaign_id SET NOT NULL;
ALTER TABLE psa_campaign_push_queue DROP COLUMN IF EXISTS operation;
