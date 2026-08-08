-- Signed push approvals (SLA-44).
--
-- The push queue's approval was previously just a status flag plus an approver
-- name, both of which a database writer can set. These columns carry an HMAC
-- minted with a key that lives only in the application environment, so an
-- approval cannot be forged or altered by writing rows.
--
-- All four are nullable: rows approved before this migration have no signature,
-- and the drain refuses them rather than pretending they were authorized.
ALTER TABLE psa_campaign_push_queue
    ADD COLUMN IF NOT EXISTS approved_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS payload_digest      TEXT,
    ADD COLUMN IF NOT EXISTS approval_signature  TEXT,
    ADD COLUMN IF NOT EXISTS signature_key_id    TEXT;

COMMENT ON COLUMN psa_campaign_push_queue.approved_at IS
    'When the approval was signed; part of the signed envelope and the freshness window.';
COMMENT ON COLUMN psa_campaign_push_queue.payload_digest IS
    'Hex SHA-256 of the canonical proposed_diff at approval time. Re-derived and compared before push.';
COMMENT ON COLUMN psa_campaign_push_queue.approval_signature IS
    'Hex HMAC-SHA256 over the approval envelope. Key is PSA_PUSH_SIGNING_KEY, never stored here.';
COMMENT ON COLUMN psa_campaign_push_queue.signature_key_id IS
    'Identifier of the signing key, so rotation does not strand in-flight approvals.';
