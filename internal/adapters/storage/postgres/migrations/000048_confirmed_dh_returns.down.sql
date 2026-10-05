-- Explicit downgrade discards safety history; never downgrade an enabled
-- deployment until mutators are drained and retained uncertainty is reconciled.
DROP TABLE dh_target_watermarks;
DROP TABLE dh_mutation_attempts;
DROP TABLE confirmed_dh_returns;
