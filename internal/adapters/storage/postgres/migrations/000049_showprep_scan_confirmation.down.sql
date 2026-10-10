-- Destructive rollback: discards command replay and retired-ID safety state.
DROP TABLE showprep_command_receipts;
DROP TABLE showprep_retired_lists;
ALTER TABLE showprep_lists DROP COLUMN withdrawal_generation;
