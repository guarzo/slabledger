-- WARNING: dropping these columns loses the record of which sales DH already
-- confirmed. If this down migration is later followed by the up migration
-- again (e.g. a rollback-then-reapply), every sale re-appears with an empty
-- dh_idempotency_key and dh_sale_id, so the §5b recovery pass will treat them
-- all as never-recorded and attempt to record them again -- under freshly
-- minted idempotency keys, so DH will NOT recognize them as replays of the
-- original request and will NOT dedupe them. Do not re-enable the recovery
-- pass after a down-then-up cycle without first reconciling against DH's own
-- sale ledger to identify which sales it already has.
ALTER TABLE campaign_purchases
    DROP COLUMN dh_sale_conflict,
    DROP COLUMN dh_sale_conflict_at;

DROP INDEX IF EXISTS idx_campaign_sales_needing_dh_record;

ALTER TABLE campaign_sales
    DROP COLUMN dh_idempotency_key,
    DROP COLUMN dh_sale_id,
    DROP COLUMN dh_sale_recorded_at;
