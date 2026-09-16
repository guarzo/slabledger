DROP TABLE IF EXISTS showprep_worker;
ALTER TABLE showprep_evidence
 DROP COLUMN IF EXISTS retry_window,
 DROP COLUMN IF EXISTS retry_attempts,
 DROP COLUMN IF EXISTS retry_not_before,
 DROP COLUMN IF EXISTS retry_reset_epoch;
-- Original snapshots/generations, lists/items and durable safety holds survive.
