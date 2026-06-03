-- Audit §1.4 / sweep R3: remove the advisor cache. The /insights page,
-- /insights/overview endpoint, AdvisorRefreshScheduler, and the
-- AdvisorCacheRepository are all gone; nothing reads or writes this table
-- anymore.  CASCADE clears the RLS policy added in 000003 and the
-- updated_at trigger added in 000001.

DROP TABLE IF EXISTS advisor_cache CASCADE;
