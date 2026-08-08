-- Reverses 000027. Drops the service-role policies, disables RLS, and re-grants
-- the DML privileges that Supabase's default privileges give anon and
-- authenticated on new public tables. Restoring those grants re-exposes these
-- tables to PostgREST, which is what reversing this migration means.
--
-- The re-grant is best-effort, not an exact restore: the up migration REVOKEs
-- ALL, so it cannot record which privileges each role actually held. The grant
-- below is deliberately the four DML privileges rather than ALL — ALL would also
-- hand back TRUNCATE, REFERENCES and TRIGGER, which Supabase's defaults never
-- granted, leaving these roles wider than before 000027 ran.
--
-- Role-dependent statements are guarded on pg_roles for the same reason as the
-- up migration: anon, authenticated and service_role do not exist outside
-- Supabase. DROP POLICY IF EXISTS needs no guard.

DROP POLICY IF EXISTS "service role bypass" ON public.dh_comp_cache;
DROP POLICY IF EXISTS "service role bypass" ON public.dh_card_tombstones;
DROP POLICY IF EXISTS "service role bypass" ON public.psa_portal_token;
DROP POLICY IF EXISTS "service role bypass" ON public.psa_portal_snapshot;
DROP POLICY IF EXISTS "service role bypass" ON public.psa_campaign_snapshot;
DROP POLICY IF EXISTS "service role bypass" ON public.psa_portal_catalog;

ALTER TABLE public.dh_comp_cache DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.dh_card_tombstones DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_portal_token DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_portal_snapshot DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_campaign_snapshot DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.psa_portal_catalog DISABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
            public.dh_comp_cache,
            public.dh_card_tombstones,
            public.psa_portal_token,
            public.psa_portal_snapshot,
            public.psa_campaign_snapshot,
            public.psa_portal_catalog
        TO anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
            public.dh_comp_cache,
            public.dh_card_tombstones,
            public.psa_portal_token,
            public.psa_portal_snapshot,
            public.psa_campaign_snapshot,
            public.psa_portal_catalog
        TO authenticated;
    END IF;
END
$$;
