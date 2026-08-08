-- Tighten the row-level security policies created by the 000003 blanket pass.
--
-- 000003 enabled RLS on 36 tables and gave each one
--     CREATE POLICY "service role bypass" ON t USING (true) WITH CHECK (true);
-- A policy with no TO clause defaults to TO PUBLIC, so that policy passes every
-- role including anon and authenticated. 000003's own header (lines 111-112)
-- states this outright. The result is that enabling RLS on those tables bought
-- nothing: they are exactly as reachable as the six tables that never got RLS
-- at all, which 000027 has just fixed. This migration brings the 000003 tables
-- up to the same standard as 000027.
--
-- For each table: drop the TO PUBLIC policy, recreate it scoped TO service_role,
-- and revoke the grants Supabase's default privileges gave anon and
-- authenticated. The GRANT is what decides whether PostgREST surfaces a table at
-- all; the policy decides which rows a role may touch once it is there.
--
-- Only policies that are actually TO PUBLIC are rewritten, so re-running this
-- against a database where 000027 already installed TO service_role policies
-- leaves those alone.
--
-- This is a no-op for the application: the Go backend connects as the table
-- owner, and owners are exempt from RLS unless FORCE ROW LEVEL SECURITY is set.
-- Nothing in this repository reaches Postgres through PostgREST — there is no
-- supabase-js client in web/ and no Supabase anon/service key in the config.
--
-- The role-dependent statements are guarded on pg_roles because anon,
-- authenticated and service_role exist in Supabase but not in a local or CI
-- Postgres. Where they are absent the tables end up with RLS enabled and no
-- policies, which is the same effective deny.
--
-- Tables dropped by later migrations (sell_sheet_items in 000007, advisor_cache
-- in 000013, mm_card_mappings in 000021) are skipped via to_regclass rather than
-- removed from the list, so the list stays a faithful record of what 000003 did.

DO $$
DECLARE
    target       text;
    has_service  boolean := EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role');
    has_anon     boolean := EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon');
    has_authed   boolean := EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated');
    -- Every table 000003 enabled RLS on, plus the seven security_invoker views
    -- it recreated. The views hold no rows of their own but are still separate
    -- grantable objects.
    targets      constant text[] := ARRAY[
        'oauth_states', 'allowed_emails', 'campaigns', 'campaign_purchases',
        'campaign_sales', 'invoices', 'cashflow_config', 'revocation_flags',
        'price_flags', 'card_access_log', 'card_id_mappings', 'sync_state',
        'ai_calls', 'api_calls', 'api_rate_limits', 'cardladder_config',
        'cl_card_mappings', 'cl_sales_comps', 'marketmovers_config',
        'mm_card_mappings', 'market_intelligence', 'dh_suggestions',
        'scoring_data_gaps', 'sell_sheet_items', 'dh_push_config',
        'dh_card_cache', 'dh_character_cache', 'dh_state_events',
        'card_price_trajectory', 'psa_pending_items', 'scheduler_run_stats',
        'schema_migrations', 'advisor_cache', 'users', 'user_sessions',
        'user_tokens',
        'active_sessions', 'expired_sessions', 'ai_usage_summary',
        'ai_usage_by_operation', 'api_usage_summary', 'api_hourly_distribution',
        'api_daily_summary'
    ];
BEGIN
    FOREACH target IN ARRAY targets LOOP
        CONTINUE WHEN to_regclass('public.' || quote_ident(target)) IS NULL;

        -- Replace the TO PUBLIC policy, if that is still its shape. A policy
        -- already scoped to a role is left untouched.
        IF EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'public'
              AND tablename = target
              AND policyname = 'service role bypass'
              AND roles::text = '{public}'
        ) THEN
            EXECUTE format('DROP POLICY "service role bypass" ON public.%I', target);

            IF has_service THEN
                EXECUTE format(
                    'CREATE POLICY "service role bypass" ON public.%I TO service_role USING (true) WITH CHECK (true)',
                    target);
            END IF;
        END IF;

        IF has_anon THEN
            EXECUTE format('REVOKE ALL ON public.%I FROM anon', target);
        END IF;

        IF has_authed THEN
            EXECUTE format('REVOKE ALL ON public.%I FROM authenticated', target);
        END IF;
    END LOOP;
END
$$;
