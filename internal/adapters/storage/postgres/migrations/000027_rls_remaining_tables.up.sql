-- Retrofit row-level security onto the six live tables that were created after
-- the 000003 blanket RLS pass and never picked it up: dh_comp_cache (000005),
-- dh_card_tombstones (000011), psa_portal_token (000016), psa_portal_snapshot
-- (000017), psa_campaign_snapshot (000018), psa_portal_catalog (000025).
--
-- Deliberately NOT the 000003 pattern. Every policy in 000003 is
--     CREATE POLICY "service role bypass" ON t USING (true) WITH CHECK (true);
-- with no TO clause, which defaults to TO PUBLIC and therefore passes anon and
-- authenticated as well — 000003's own header (lines 111-112) says so. Copying
-- it here would enable RLS and change nothing. The policies below are scoped
-- TO service_role, so a PostgREST caller arriving as anon or authenticated
-- matches no policy and is denied.
--
-- This is a no-op for the application: the Go backend connects as the table
-- owner, and owners are exempt from RLS unless FORCE ROW LEVEL SECURITY is set.
--
-- The role-dependent statements are guarded on pg_roles because anon,
-- authenticated and service_role exist in Supabase but not in a local or CI
-- Postgres. Where they are absent the tables simply end up with RLS enabled and
-- no policies at all, which is the same effective deny for every non-owner role.

ALTER TABLE public.dh_comp_cache ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.dh_card_tombstones ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_portal_token ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_portal_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_campaign_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_portal_catalog ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
        CREATE POLICY "service role bypass" ON public.dh_comp_cache TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.dh_card_tombstones TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.psa_portal_token TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.psa_portal_snapshot TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.psa_campaign_snapshot TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.psa_portal_catalog TO service_role USING (true) WITH CHECK (true);
    END IF;

    -- Defense in depth. RLS decides which rows a role may touch, but the table
    -- GRANT decides whether PostgREST will surface the table at all, and
    -- Supabase's default privileges hand new public tables to anon and
    -- authenticated automatically. No migration in this repo has ever issued a
    -- GRANT or REVOKE, so these two roles still hold whatever the defaults gave
    -- them.
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE
            public.dh_comp_cache,
            public.dh_card_tombstones,
            public.psa_portal_token,
            public.psa_portal_snapshot,
            public.psa_campaign_snapshot,
            public.psa_portal_catalog
        FROM anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE
            public.dh_comp_cache,
            public.dh_card_tombstones,
            public.psa_portal_token,
            public.psa_portal_snapshot,
            public.psa_campaign_snapshot,
            public.psa_portal_catalog
        FROM authenticated;
    END IF;
END
$$;
