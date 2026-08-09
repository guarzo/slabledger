-- Drop the four views with no consumer: active_sessions, expired_sessions,
-- api_hourly_distribution and api_daily_summary.
--
-- All four were created by 000001 and recreated by 000003 WITH
-- (security_invoker = true). Nothing in the repository has ever read them --
-- no Go file, no TypeScript file, no script. The only references outside the
-- migrations directory are their own entries in docs/SCHEMA.md. What is left
-- is schema nobody owns, which 000028 is still dutifully maintaining REVOKEs
-- for; that is the exact condition 000035 cited when it removed the two
-- ai_usage_* views from the same 000028 target list.
--
-- api_usage_summary is the fifth view and is deliberately NOT dropped. It is
-- live: DBTracker.GetAPIUsage selects from it (api_tracker.go:52). The two
-- api_* views dropped here read the same api_calls table and look
-- interchangeable with it at a glance; they are not.
--
-- expired_sessions is the one that looks load-bearing and is not.
-- docs/SCHEMA.md described it as "used by the session-cleanup scheduler",
-- which is wrong: DeleteExpiredSessions issues
--   DELETE FROM user_sessions WHERE expires_at <= $1
-- against the table directly (auth_session_store.go:185) and never names the
-- view. That doc line is corrected in this change set.
--
-- No CASCADE, and no dependency ordering needed. These are leaf views: nothing
-- else in the schema selects from them, and none of them selects from another.
-- DROP VIEW ... CASCADE would work today and is deliberately not used -- it
-- would silently take anything that attaches between now and the deploy, which
-- is the failure mode 000021 and 000030 were cleaning up after.
--
-- Repository-scoped caveat, restated here because the deploy is automatic: an
-- external Supabase dashboard or saved SQL-console query could read these
-- views, and no repository-scoped check can see that. Structure comes back
-- via the .down.sql; a dashboard pointed at a dropped view does not heal
-- itself until then.

DROP VIEW IF EXISTS public.active_sessions;
DROP VIEW IF EXISTS public.expired_sessions;
DROP VIEW IF EXISTS public.api_hourly_distribution;
DROP VIEW IF EXISTS public.api_daily_summary;
