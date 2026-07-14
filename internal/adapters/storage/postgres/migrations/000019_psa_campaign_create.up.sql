ALTER TABLE psa_campaign_push_queue ADD COLUMN IF NOT EXISTS operation TEXT NOT NULL DEFAULT 'update';
ALTER TABLE psa_campaign_push_queue ALTER COLUMN psa_campaign_id DROP NOT NULL;
