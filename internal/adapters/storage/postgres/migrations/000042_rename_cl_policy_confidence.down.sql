-- Reverses the expand half only: drops the new column. The old column
-- (cl_confidence_at_purchase) was never touched by the up migration, so it
-- needs no restoration here.
ALTER TABLE campaign_purchases
    DROP COLUMN cl_policy_confidence_min_at_purchase;
