-- Reset all rows stuck at dh_push_status='unmatched' back to 'pending' so the
-- new psa_import-primary push path picks them up on the next scheduler cycle.
--
-- These rows were latched to 'unmatched' by the deprecated skip-attempts cap
-- (maxDHPushSkipAttempts=10) after DH's /api/v1/enterprise/certs/resolve
-- endpoint started returning 404 for every request. The new flow routes
-- directly through /api/v1/enterprise/inventory/psa_import (match + inventory
-- create in one call) and has no consecutive-skip cap — transient DH errors
-- stay pending and auto-retry on the next 5-minute cycle.
--
-- Guard: only reset rows that never acquired a dh_inventory_id. Rows with a
-- non-null/non-zero inventory ID shouldn't be at 'unmatched' (the push
-- scheduler's early-exit guard flips them to 'matched'), but the filter is
-- defensive against pre-existing bad state.
UPDATE campaign_purchases
SET dh_push_status = 'pending',
    dh_push_attempts = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE dh_push_status = 'unmatched'
  AND (dh_inventory_id IS NULL OR dh_inventory_id = 0);
