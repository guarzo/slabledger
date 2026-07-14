-- Prevent duplicate in-flight PSA create proposals for the same internal
-- campaign. Only unresolved creates (pending/approved/pushing) are constrained,
-- so a legitimate retry after a failed create is still allowed once the prior
-- row leaves these states.
CREATE UNIQUE INDEX IF NOT EXISTS uq_psa_push_queue_create_unresolved
    ON psa_campaign_push_queue (internal_campaign_id)
    WHERE operation = 'create' AND status IN ('pending', 'approved', 'pushing');
