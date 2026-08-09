-- Rollback: recreate the four views.
--
-- The definitions are 000003's, not 000001's. 000003's Fix 1 dropped and
-- recreated every view WITH (security_invoker = true), because Supabase flags a
-- view without it as SECURITY DEFINER -- enforcing the creator's permissions
-- rather than the querier's. 000003 runs before this migration, so restoring
-- 000001's plain CREATE VIEW would roll the database back past a security fix
-- that is not being reverted.
--
-- The REVOKEs are the other half. A view holds no rows of its own but is still
-- a separate grantable object, and Supabase's default privileges grant on it,
-- so a view restored without the REVOKE is readable by anon while the tables
-- underneath stay locked. 000028 listed all four among its targets for exactly
-- this reason, and a rollback has to repeat it or it reopens what 000028 closed.
--
-- Role-dependent statements are guarded on pg_roles so this also applies to a
-- local Postgres, where Supabase's roles do not exist.

CREATE VIEW public.active_sessions WITH (security_invoker = true) AS
SELECT
    s.id,
    s.user_id,
    u.username,
    u.google_id,
    s.expires_at,
    s.created_at,
    s.last_accessed_at,
    s.user_agent,
    s.ip_address,
    ROUND(CAST(EXTRACT(EPOCH FROM (s.expires_at - NOW())) / 3600 AS NUMERIC), 1) AS hours_until_expiry
FROM user_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.expires_at > NOW()
ORDER BY s.last_accessed_at DESC;

CREATE VIEW public.expired_sessions WITH (security_invoker = true) AS
SELECT id
FROM user_sessions
WHERE expires_at <= NOW();

CREATE VIEW public.api_hourly_distribution WITH (security_invoker = true) AS
SELECT
    provider,
    TO_CHAR(timestamp, 'YYYY-MM-DD HH24:00') AS hour,
    COUNT(*) AS call_count,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limit_hits
FROM api_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY provider, TO_CHAR(timestamp, 'YYYY-MM-DD HH24:00')
ORDER BY hour DESC;

CREATE VIEW public.api_daily_summary WITH (security_invoker = true) AS
SELECT
    provider,
    CAST(timestamp AS DATE) AS date,
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status_code < 400 THEN 1 END) AS successful_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limit_hits,
    ROUND(100.0 * COUNT(CASE WHEN status_code < 400 THEN 1 END) / COUNT(*), 1) AS success_rate_pct,
    ROUND(CAST(AVG(latency_ms) AS NUMERIC)) AS avg_latency_ms,
    MIN(timestamp) AS first_call,
    MAX(timestamp) AS last_call
FROM api_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY provider, CAST(timestamp AS DATE)
ORDER BY date DESC, provider;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON public.active_sessions FROM anon;
        REVOKE ALL ON public.expired_sessions FROM anon;
        REVOKE ALL ON public.api_hourly_distribution FROM anon;
        REVOKE ALL ON public.api_daily_summary FROM anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON public.active_sessions FROM authenticated;
        REVOKE ALL ON public.expired_sessions FROM authenticated;
        REVOKE ALL ON public.api_hourly_distribution FROM authenticated;
        REVOKE ALL ON public.api_daily_summary FROM authenticated;
    END IF;
END
$$;
