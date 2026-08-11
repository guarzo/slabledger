-- Restores the column's SHAPE only, from the surviving
-- cl_policy_confidence_min_at_purchase column. It does not restore any Go code
-- path that reads or writes it -- every reference to CLConfidenceAtPurchase was
-- deleted a deploy earlier (Task 11). A rollback of this migration undoes the
-- schema change so a rolled-back OLD binary (pre-Task-11, still expecting the
-- column to exist) can start up, but the column will not receive new writes
-- from a rolled-back instance beyond this repopulation -- that is the honest
-- limit of a contract rollback, not a bug in this migration.
ALTER TABLE campaign_purchases
    ADD COLUMN cl_confidence_at_purchase SMALLINT;

UPDATE campaign_purchases
    SET cl_confidence_at_purchase = cl_policy_confidence_min_at_purchase
    WHERE cl_policy_confidence_min_at_purchase IS NOT NULL;
