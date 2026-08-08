-- Drop the AI advisor's persistence: ai_calls, its two usage views, and
-- scoring_data_gaps.
--
-- The Azure AI advisor feature is removed in this change set. ai_tracker.go and
-- gap_store.go — the only code that ever read or wrote these objects — go with
-- it, along with the gap-cleanup scheduler that existed solely to keep
-- scoring_data_gaps from growing unbounded. What is left behind is schema
-- nobody owns, which 000028 was still dutifully maintaining RLS policies for.
--
-- ai_calls holds production history: one row per advisor invocation, with
-- latency, token counts and cost estimates. That history is NOT recoverable
-- after this runs, and this repository auto-deploys from main, so it is
-- destroyed on merge rather than on a deliberate operator action. Export it
-- before merging if it is wanted; the .down.sql restores structure only.
--
-- Order matters. ai_usage_summary and ai_usage_by_operation are views over
-- ai_calls (000003, security_invoker = true), and Postgres raises
--   ERROR: cannot drop table ai_calls because other objects depend on it
-- rather than cascading through them. They are dropped first, by name. DROP
-- TABLE ... CASCADE would also work today and is deliberately not used: it
-- would silently take anything that attaches to ai_calls between now and the
-- deploy, which is the failure mode 000021 and 000030 were cleaning up after.
--
-- Policies are dropped explicitly before each table so the table drop needs no
-- CASCADE, matching 000013_drop_advisor_cache and 000030_drop_marketmovers_config.

DROP VIEW IF EXISTS public.ai_usage_by_operation;
DROP VIEW IF EXISTS public.ai_usage_summary;

DROP POLICY IF EXISTS "service role bypass" ON public.ai_calls;
DROP TABLE IF EXISTS ai_calls;

DROP POLICY IF EXISTS "service role bypass" ON public.scoring_data_gaps;
DROP TABLE IF EXISTS scoring_data_gaps;
