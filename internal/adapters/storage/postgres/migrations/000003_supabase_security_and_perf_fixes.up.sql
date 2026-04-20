-- ================================================================
-- 000003: Supabase security and performance fixes
-- ================================================================
-- Fix 1: Recreate views with security_invoker = true
-- (Supabase flags views without this as SECURITY DEFINER, which
--  enforces the creator's permissions rather than the querier's)
-- ================================================================

DROP VIEW IF EXISTS public.active_sessions;
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

DROP VIEW IF EXISTS public.expired_sessions;
CREATE VIEW public.expired_sessions WITH (security_invoker = true) AS
SELECT id
FROM user_sessions
WHERE expires_at <= NOW();

DROP VIEW IF EXISTS public.ai_usage_summary;
CREATE VIEW public.ai_usage_summary WITH (security_invoker = true) AS
SELECT
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status = 'success' THEN 1 END) AS success_calls,
    COUNT(CASE WHEN status = 'error' THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status = 'rate_limited' THEN 1 END) AS rate_limit_hits,
    COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
    COALESCE(SUM(input_tokens), 0) AS total_input_tokens,
    COALESCE(SUM(output_tokens), 0) AS total_output_tokens,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) AS total_cost_cents,
    TO_CHAR(MAX(timestamp), 'YYYY-MM-DD HH24:MI:SS') AS last_call_at,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '24 hours' THEN 1 END) AS calls_last_24h
FROM ai_calls
WHERE timestamp > NOW() - INTERVAL '7 days';

DROP VIEW IF EXISTS public.ai_usage_by_operation;
CREATE VIEW public.ai_usage_by_operation WITH (security_invoker = true) AS
SELECT
    operation,
    COUNT(*) AS calls,
    COUNT(CASE WHEN status = 'error' OR status = 'rate_limited' THEN 1 END) AS errors,
    COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) AS total_cost_cents
FROM ai_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY operation;

DROP VIEW IF EXISTS public.api_usage_summary;
CREATE VIEW public.api_usage_summary WITH (security_invoker = true) AS
SELECT
    provider,
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limit_hits,
    AVG(latency_ms) AS avg_latency_ms,
    MAX(timestamp) AS last_call_at,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '1 hour' THEN 1 END) AS calls_last_hour,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '5 minutes' THEN 1 END) AS calls_last_5min
FROM api_calls
WHERE timestamp > NOW() - INTERVAL '24 hours'
GROUP BY provider;

DROP VIEW IF EXISTS public.api_hourly_distribution;
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

DROP VIEW IF EXISTS public.api_daily_summary;
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

-- ================================================================
-- Fix 2: Enable RLS on all public tables + service-role bypass policy
-- The Go backend uses the postgres role which has superuser-level
-- access; the USING (true) policy allows all operations for any role.
-- Supabase service_role bypasses RLS by default.
-- ================================================================

ALTER TABLE public.oauth_states ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.oauth_states USING (true) WITH CHECK (true);

ALTER TABLE public.allowed_emails ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.allowed_emails USING (true) WITH CHECK (true);

ALTER TABLE public.campaigns ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.campaigns USING (true) WITH CHECK (true);

ALTER TABLE public.campaign_purchases ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.campaign_purchases USING (true) WITH CHECK (true);

ALTER TABLE public.campaign_sales ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.campaign_sales USING (true) WITH CHECK (true);

ALTER TABLE public.invoices ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.invoices USING (true) WITH CHECK (true);

ALTER TABLE public.cashflow_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.cashflow_config USING (true) WITH CHECK (true);

ALTER TABLE public.revocation_flags ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.revocation_flags USING (true) WITH CHECK (true);

ALTER TABLE public.price_flags ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.price_flags USING (true) WITH CHECK (true);

ALTER TABLE public.card_access_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.card_access_log USING (true) WITH CHECK (true);

ALTER TABLE public.card_id_mappings ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.card_id_mappings USING (true) WITH CHECK (true);

ALTER TABLE public.sync_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.sync_state USING (true) WITH CHECK (true);

ALTER TABLE public.ai_calls ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.ai_calls USING (true) WITH CHECK (true);

ALTER TABLE public.api_calls ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.api_calls USING (true) WITH CHECK (true);

ALTER TABLE public.api_rate_limits ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.api_rate_limits USING (true) WITH CHECK (true);

ALTER TABLE public.cardladder_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.cardladder_config USING (true) WITH CHECK (true);

ALTER TABLE public.cl_card_mappings ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.cl_card_mappings USING (true) WITH CHECK (true);

ALTER TABLE public.cl_sales_comps ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.cl_sales_comps USING (true) WITH CHECK (true);

ALTER TABLE public.marketmovers_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.marketmovers_config USING (true) WITH CHECK (true);

ALTER TABLE public.mm_card_mappings ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.mm_card_mappings USING (true) WITH CHECK (true);

ALTER TABLE public.market_intelligence ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.market_intelligence USING (true) WITH CHECK (true);

ALTER TABLE public.dh_suggestions ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.dh_suggestions USING (true) WITH CHECK (true);

ALTER TABLE public.scoring_data_gaps ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.scoring_data_gaps USING (true) WITH CHECK (true);

ALTER TABLE public.sell_sheet_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.sell_sheet_items USING (true) WITH CHECK (true);

ALTER TABLE public.dh_push_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.dh_push_config USING (true) WITH CHECK (true);

ALTER TABLE public.dh_card_cache ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.dh_card_cache USING (true) WITH CHECK (true);

ALTER TABLE public.dh_character_cache ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.dh_character_cache USING (true) WITH CHECK (true);

ALTER TABLE public.dh_state_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.dh_state_events USING (true) WITH CHECK (true);

ALTER TABLE public.card_price_trajectory ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.card_price_trajectory USING (true) WITH CHECK (true);

ALTER TABLE public.psa_pending_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.psa_pending_items USING (true) WITH CHECK (true);

ALTER TABLE public.scheduler_run_stats ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.scheduler_run_stats USING (true) WITH CHECK (true);

ALTER TABLE public.schema_migrations ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.schema_migrations USING (true) WITH CHECK (true);

ALTER TABLE public.advisor_cache ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.advisor_cache USING (true) WITH CHECK (true);

ALTER TABLE public.users ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.users USING (true) WITH CHECK (true);

ALTER TABLE public.user_sessions ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.user_sessions USING (true) WITH CHECK (true);

ALTER TABLE public.user_tokens ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.user_tokens USING (true) WITH CHECK (true);

-- ================================================================
-- Fix 3: Drop duplicate index on users.google_id
-- users_google_id_key is the UNIQUE constraint index — it's canonical.
-- idx_users_google_id is a redundant standalone CREATE INDEX.
-- ================================================================

DROP INDEX IF EXISTS public.idx_users_google_id;

-- ================================================================
-- Fix 4: Add missing FK indexes
-- ================================================================

CREATE INDEX idx_allowed_emails_added_by ON public.allowed_emails(added_by);
CREATE INDEX idx_price_flags_flagged_by  ON public.price_flags(flagged_by);
CREATE INDEX idx_price_flags_resolved_by ON public.price_flags(resolved_by);

-- ================================================================
-- Fix 5: Drop 26 unused indexes (per Supabase telemetry)
-- ================================================================

-- From 000002_add_supabase_suggested_indexes (never used):
DROP INDEX IF EXISTS public.idx_card_access_log_card_number;
DROP INDEX IF EXISTS public.idx_campaign_purchases_updated_at;
DROP INDEX IF EXISTS public.idx_campaign_purchases_received_at;

-- From 000001_initial_schema:
DROP INDEX IF EXISTS public.idx_user_sessions_user_id;
DROP INDEX IF EXISTS public.idx_user_tokens_user_id;
DROP INDEX IF EXISTS public.idx_user_tokens_session_id;
DROP INDEX IF EXISTS public.idx_user_tokens_session_unique;
DROP INDEX IF EXISTS public.idx_user_tokens_expires_at;
DROP INDEX IF EXISTS public.idx_oauth_states_expires;
DROP INDEX IF EXISTS public.idx_campaign_purchases_ebay_export_flagged_at;
DROP INDEX IF EXISTS public.idx_purchases_dh_cert_status;
DROP INDEX IF EXISTS public.idx_purchases_gem_rate_id;
DROP INDEX IF EXISTS public.idx_sales_channel;
DROP INDEX IF EXISTS public.idx_invoices_status;
DROP INDEX IF EXISTS public.idx_revocation_flags_status;
DROP INDEX IF EXISTS public.idx_revocation_flags_segment;
DROP INDEX IF EXISTS public.idx_ai_calls_timestamp;
DROP INDEX IF EXISTS public.idx_ai_calls_operation;
DROP INDEX IF EXISTS public.idx_api_calls_timestamp;
DROP INDEX IF EXISTS public.idx_api_calls_errors;
DROP INDEX IF EXISTS public.idx_cl_sales_comps_gem_rate;
DROP INDEX IF EXISTS public.idx_dh_suggestions_card;
DROP INDEX IF EXISTS public.idx_scoring_gaps_factor;
DROP INDEX IF EXISTS public.idx_card_cache_demand_score;
DROP INDEX IF EXISTS public.idx_dh_state_events_purchase;
DROP INDEX IF EXISTS public.idx_dh_state_events_cert;
DROP INDEX IF EXISTS public.idx_card_price_trajectory_card;

-- ================================================================
-- Fix 6: Add 7 index advisor indexes (hot query paths)
-- ================================================================

-- card_id_mappings: lookup by card_name (~48% of total DB query time)
CREATE INDEX idx_card_id_mappings_card_name
    ON public.card_id_mappings USING btree (card_name);

-- campaign_purchases: filter by dh_push_status (~27% of total DB query time)
CREATE INDEX idx_campaign_purchases_dh_push_status
    ON public.campaign_purchases USING btree (dh_push_status);

-- dh_suggestions: MAX(fetched_at) query (~13% of total DB query time)
CREATE INDEX idx_dh_suggestions_fetched_at
    ON public.dh_suggestions USING btree (fetched_at);

-- campaign_purchases: filter/lookup by cert_number (two queries, ~7% combined)
CREATE INDEX idx_campaign_purchases_cert_number
    ON public.campaign_purchases USING btree (cert_number);

-- market_intelligence: COUNT(*) with velocity_last_fetch (~2% of total)
CREATE INDEX idx_market_intelligence_velocity_last_fetch
    ON public.market_intelligence USING btree (velocity_last_fetch);

-- sell_sheet_items: ORDER BY added_at (~1.5% of total)
CREATE INDEX idx_sell_sheet_items_added_at
    ON public.sell_sheet_items USING btree (added_at);

-- campaign_purchases: filter by mm_value_cents for sync gap query (~1.4% of total)
CREATE INDEX idx_campaign_purchases_mm_value_cents
    ON public.campaign_purchases USING btree (mm_value_cents);
