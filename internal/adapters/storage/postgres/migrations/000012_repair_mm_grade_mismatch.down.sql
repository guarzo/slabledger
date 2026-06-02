-- No-op: this migration is a data repair. There is no useful inverse — the
-- previous bad state is by definition wrong. The next MM refresh will re-resolve
-- mappings under the new grade-aware rules.
SELECT 1;
