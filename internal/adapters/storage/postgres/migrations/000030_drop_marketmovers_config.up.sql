-- SLA-20 / audit DBSCHEMA-004: drop the dead marketmovers_config table.
--
-- Market Movers was retired in 000021, which dropped its siblings mm_sales_comps
-- and mm_card_mappings by name but missed this one. Nothing in Go has referenced
-- it since: marketmovers_store.go, the only code that ever read or wrote it, went
-- with the rest of the integration. 000028 still listed the table among its
-- RLS-tightening targets, which is how schema nobody owns kept being maintained
-- as if it were live.
--
-- The table held one production row: a username and an encrypted_refresh_token
-- for the retired vendor. That row was reviewed and explicitly discarded before
-- this migration was authored. No code can decrypt or use it, and an orphaned
-- credential in a table with no owner is a larger risk than losing it. The
-- .down.sql restores the structure only -- the row is not recoverable.
--
-- Drop the policy explicitly first so the table drop needs no CASCADE, matching
-- 000013_drop_advisor_cache.

DROP POLICY IF EXISTS "service role bypass" ON public.marketmovers_config;
DROP TABLE IF EXISTS marketmovers_config;
