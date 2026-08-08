ALTER TABLE psa_campaign_push_queue
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS payload_digest,
    DROP COLUMN IF EXISTS approval_signature,
    DROP COLUMN IF EXISTS signature_key_id;
