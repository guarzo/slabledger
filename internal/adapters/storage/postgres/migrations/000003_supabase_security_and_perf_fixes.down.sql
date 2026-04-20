-- ================================================================
-- 000003 DOWN: Undo Supabase security and performance fixes
-- Reverse order of up migration.
-- ================================================================

-- ================================================================
-- Undo Fix 6: Drop the 7 index advisor indexes
-- ================================================================

DROP INDEX IF EXISTS public.idx_card_id_mappings_card_name;
DROP INDEX IF EXISTS public.idx_campaign_purchases_dh_push_status;
DROP INDEX IF EXISTS public.idx_dh_suggestions_fetched_at;
DROP INDEX IF EXISTS public.idx_campaign_purchases_cert_number;
DROP INDEX IF EXISTS public.idx_market_intelligence_velocity_last_fetch;
DROP INDEX IF EXISTS public.idx_sell_sheet_items_added_at;
DROP INDEX IF EXISTS public.idx_campaign_purchases_mm_value_cents;

-- ================================================================
-- Undo Fix 5: Recreate the 26 unused indexes
-- ================================================================

-- Originally from 000002_add_supabase_suggested_indexes:
CREATE INDEX IF NOT EXISTS idx_card_access_log_card_number
    ON card_access_log(card_number);
CREATE INDEX IF NOT EXISTS idx_campaign_purchases_updated_at
    ON campaign_purchases(updated_at);
CREATE INDEX IF NOT EXISTS idx_campaign_purchases_received_at
    ON campaign_purchases(received_at);

-- Originally from 000001_initial_schema:
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_tokens_user_id ON user_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_tokens_session_id ON user_tokens(session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tokens_session_unique ON user_tokens(session_id);
CREATE INDEX IF NOT EXISTS idx_user_tokens_expires_at ON user_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_oauth_states_expires ON oauth_states(expires_at);
CREATE INDEX IF NOT EXISTS idx_campaign_purchases_ebay_export_flagged_at
    ON campaign_purchases (ebay_export_flagged_at)
    WHERE ebay_export_flagged_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_purchases_dh_cert_status ON campaign_purchases(dh_cert_status)
    WHERE dh_cert_status != '';
CREATE INDEX IF NOT EXISTS idx_purchases_gem_rate_id ON campaign_purchases(gem_rate_id) WHERE gem_rate_id != '';
CREATE INDEX IF NOT EXISTS idx_sales_channel ON campaign_sales(sale_channel);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_revocation_flags_status ON revocation_flags(status);
CREATE INDEX IF NOT EXISTS idx_revocation_flags_segment ON revocation_flags(segment_label, segment_dimension);
CREATE INDEX IF NOT EXISTS idx_ai_calls_timestamp ON ai_calls(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ai_calls_operation ON ai_calls(operation, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_api_calls_timestamp ON api_calls(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_api_calls_errors ON api_calls(provider, status_code) WHERE status_code >= 400;
CREATE INDEX IF NOT EXISTS idx_cl_sales_comps_gem_rate ON cl_sales_comps(gem_rate_id, sale_date DESC);
CREATE INDEX IF NOT EXISTS idx_dh_suggestions_card ON dh_suggestions(card_name, set_name);
CREATE INDEX IF NOT EXISTS idx_scoring_gaps_factor ON scoring_data_gaps(factor_name, recorded_at);
CREATE INDEX IF NOT EXISTS idx_card_cache_demand_score ON dh_card_cache(demand_score DESC);
CREATE INDEX IF NOT EXISTS idx_dh_state_events_purchase ON dh_state_events(purchase_id, event_at);
CREATE INDEX IF NOT EXISTS idx_dh_state_events_cert ON dh_state_events(cert_number, event_at);
CREATE INDEX IF NOT EXISTS idx_card_price_trajectory_card ON card_price_trajectory(dh_card_id, week_start DESC);

-- ================================================================
-- Undo Fix 4: Drop FK indexes
-- ================================================================

DROP INDEX IF EXISTS public.idx_allowed_emails_added_by;
DROP INDEX IF EXISTS public.idx_price_flags_flagged_by;
DROP INDEX IF EXISTS public.idx_price_flags_resolved_by;

-- ================================================================
-- Undo Fix 3: Recreate duplicate index
-- ================================================================

CREATE UNIQUE INDEX idx_users_google_id ON users(google_id);

-- ================================================================
-- Undo Fix 2: Disable RLS and drop bypass policies
-- ================================================================

DROP POLICY IF EXISTS "service role bypass" ON public.oauth_states;
ALTER TABLE public.oauth_states DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.allowed_emails;
ALTER TABLE public.allowed_emails DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.campaigns;
ALTER TABLE public.campaigns DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.campaign_purchases;
ALTER TABLE public.campaign_purchases DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.campaign_sales;
ALTER TABLE public.campaign_sales DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.invoices;
ALTER TABLE public.invoices DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.cashflow_config;
ALTER TABLE public.cashflow_config DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.revocation_flags;
ALTER TABLE public.revocation_flags DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.price_flags;
ALTER TABLE public.price_flags DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.card_access_log;
ALTER TABLE public.card_access_log DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.card_id_mappings;
ALTER TABLE public.card_id_mappings DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.sync_state;
ALTER TABLE public.sync_state DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.ai_calls;
ALTER TABLE public.ai_calls DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.api_calls;
ALTER TABLE public.api_calls DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.api_rate_limits;
ALTER TABLE public.api_rate_limits DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.cardladder_config;
ALTER TABLE public.cardladder_config DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.cl_card_mappings;
ALTER TABLE public.cl_card_mappings DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.cl_sales_comps;
ALTER TABLE public.cl_sales_comps DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.marketmovers_config;
ALTER TABLE public.marketmovers_config DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.mm_card_mappings;
ALTER TABLE public.mm_card_mappings DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.market_intelligence;
ALTER TABLE public.market_intelligence DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.dh_suggestions;
ALTER TABLE public.dh_suggestions DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.scoring_data_gaps;
ALTER TABLE public.scoring_data_gaps DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.sell_sheet_items;
ALTER TABLE public.sell_sheet_items DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.dh_push_config;
ALTER TABLE public.dh_push_config DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.dh_card_cache;
ALTER TABLE public.dh_card_cache DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.dh_character_cache;
ALTER TABLE public.dh_character_cache DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.dh_state_events;
ALTER TABLE public.dh_state_events DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.card_price_trajectory;
ALTER TABLE public.card_price_trajectory DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.psa_pending_items;
ALTER TABLE public.psa_pending_items DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.scheduler_run_stats;
ALTER TABLE public.scheduler_run_stats DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.schema_migrations;
ALTER TABLE public.schema_migrations DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.advisor_cache;
ALTER TABLE public.advisor_cache DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.users;
ALTER TABLE public.users DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.user_sessions;
ALTER TABLE public.user_sessions DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service role bypass" ON public.user_tokens;
ALTER TABLE public.user_tokens DISABLE ROW LEVEL SECURITY;

-- ================================================================
-- Undo Fix 1: Recreate views without security_invoker
-- ================================================================

DROP VIEW IF EXISTS public.active_sessions;
CREATE VIEW public.active_sessions AS
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
CREATE VIEW public.expired_sessions AS
SELECT id
FROM user_sessions
WHERE expires_at <= NOW();

DROP VIEW IF EXISTS public.ai_usage_summary;
CREATE VIEW public.ai_usage_summary AS
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
CREATE VIEW public.ai_usage_by_operation AS
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
CREATE VIEW public.api_usage_summary AS
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
CREATE VIEW public.api_hourly_distribution AS
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
CREATE VIEW public.api_daily_summary AS
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
