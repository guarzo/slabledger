-- Reverses 000028: puts the 000003 tables back on the TO PUBLIC policy shape and
-- re-grants anon/authenticated.
--
-- The re-grant is best-effort, not an exact restore: the up migration REVOKEs
-- ALL, so it cannot record which privileges each role actually held. It grants
-- the four DML privileges rather than ALL, because ALL would also hand back
-- TRUNCATE/REFERENCES/TRIGGER that Supabase's defaults never granted.
--
-- Policies are only recreated on tables (relkind 'r'), matching 000003 — the
-- seven views appear in the target list for the grant handling only.
--
-- The six tables 000027 covers are deliberately absent from this list, so
-- reversing 000028 does not loosen them.

DO $$
DECLARE
    target       text;
    has_anon     boolean := EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon');
    has_authed   boolean := EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated');
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

        IF EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'public' AND c.relname = target AND c.relkind = 'r'
        ) THEN
            EXECUTE format('DROP POLICY IF EXISTS "service role bypass" ON public.%I', target);
            EXECUTE format(
                'CREATE POLICY "service role bypass" ON public.%I USING (true) WITH CHECK (true)',
                target);
        END IF;

        IF has_anon THEN
            EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO anon', target);
        END IF;

        IF has_authed THEN
            EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO authenticated', target);
        END IF;
    END LOOP;
END
$$;
