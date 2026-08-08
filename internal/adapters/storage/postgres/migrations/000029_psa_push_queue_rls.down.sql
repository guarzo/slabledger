-- Reverses 000029. Drops the service-role policy, disables RLS, and re-grants
-- the DML privileges that Supabase's default privileges give anon and
-- authenticated on new public tables. Restoring those grants re-exposes the
-- queue to PostgREST, which is what reversing this migration means.
--
-- The re-grant is best-effort, not an exact restore: the up migration REVOKEs
-- ALL, so it cannot record which privileges each role actually held. The grant
-- below is deliberately the four DML privileges rather than ALL — ALL would also
-- hand back TRUNCATE, REFERENCES and TRIGGER, which Supabase's defaults never
-- granted, leaving these roles wider than before 000029 ran.
--
-- Role-dependent statements are guarded on pg_roles for the same reason as the
-- up migration. DROP POLICY IF EXISTS needs no guard.

DROP POLICY IF EXISTS "service role bypass" ON public.psa_campaign_push_queue;

ALTER TABLE public.psa_campaign_push_queue DISABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.psa_campaign_push_queue TO anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.psa_campaign_push_queue TO authenticated;
    END IF;
END
$$;
