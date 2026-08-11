-- Contract half of the cl_confidence_at_purchase rename (see migration 000042
-- for the expand half and the rolling-deploy hazard it exists to avoid). By the
-- time this migration runs, TWO deploys have shipped since the expand: deploy N
-- (dual-write, still reading the old column) and deploy N+1 (code-only, last
-- reference removed). Nothing in any live binary reads or writes
-- cl_confidence_at_purchase anymore, so it can go.
--
-- Reconcile first. Migration 000042's backfill was a single point-in-time copy.
-- Instances running the pre-expand binary kept writing the old column, and only
-- the old column, for the remainder of the deploy-N rollout -- those rows have
-- a value here and NULL in the new column, and would lose it at the DROP below.
-- Nothing can race this UPDATE: the last writer of the old column cycled out a
-- full deploy ago, which is exactly what the blocking precondition on this task
-- guarantees.
UPDATE campaign_purchases
    SET cl_policy_confidence_min_at_purchase = cl_confidence_at_purchase
    WHERE cl_policy_confidence_min_at_purchase IS NULL
      AND cl_confidence_at_purchase IS NOT NULL;

ALTER TABLE campaign_purchases
    DROP COLUMN cl_confidence_at_purchase;
