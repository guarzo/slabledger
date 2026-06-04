-- Audit §1.4 / sweep R3: remove the advisor cache. The /insights page,
-- /insights/overview endpoint, AdvisorRefreshScheduler, and the
-- AdvisorCacheRepository are all gone; nothing reads or writes this table
-- anymore.  Explicitly drop the trigger + function + policy first so the
-- table drop does not need CASCADE (safer rollback semantics — no
-- accidental drop of external dependents).

DROP TRIGGER IF EXISTS trg_advisor_cache_updated_at ON advisor_cache;
DROP FUNCTION IF EXISTS trg_advisor_cache_updated_at_fn();
DROP POLICY IF EXISTS "service role bypass" ON public.advisor_cache;
DROP TABLE IF EXISTS advisor_cache;
