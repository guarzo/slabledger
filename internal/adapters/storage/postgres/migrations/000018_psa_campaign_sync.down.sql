DROP TABLE IF EXISTS psa_campaign_push_queue;
DROP TABLE IF EXISTS psa_campaign_snapshot;
ALTER TABLE campaigns DROP COLUMN IF EXISTS psa_campaign_request_id;
