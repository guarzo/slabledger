-- Brings psa_campaign_push_queue (created in 000017) under row-level security.
--
-- 000027 covered the six tables created after the 000003 blanket pass but
-- deliberately left this one out, tracking it as a separate ticket (SLA-9)
-- because the queue's exposure is not only a grants problem. This migration
-- closes the mechanical half with exactly the 000027 pattern: a policy scoped
-- TO service_role, plus a REVOKE of the privileges Supabase's default
-- privileges hand anon and authenticated on new public tables. The GRANT is
-- what decides whether PostgREST surfaces the table at all; the policy decides
-- which rows a role may touch once it is there.
--
-- What this does and does not buy. After it, an anon or authenticated PostgREST
-- caller can no longer flip a queued row to approved and have the drain loop
-- push it to the live PSA portal. It does nothing against an actor holding
-- service_role or direct database credentials — that actor can still write any
-- status it likes, because the status column is a claim the database cannot
-- verify. The control for that case is in the application: DrainPushQueue
-- claims a row (approved -> pushing) before acting on it, and MarkResult only
-- records an outcome for a row it still holds that claim on. Making the queue
-- self-authenticating against a database writer needs an approval signature the
-- database cannot forge, which is SLA-44.
--
-- This is a no-op for the application: the Go backend connects as the table
-- owner, and owners are exempt from RLS unless FORCE ROW LEVEL SECURITY is set.
--
-- The role-dependent statements are guarded on pg_roles because anon,
-- authenticated and service_role exist in Supabase but not in a local or CI
-- Postgres. Where they are absent the table ends up with RLS enabled and no
-- policies, which is the same effective deny.

ALTER TABLE public.psa_campaign_push_queue ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
        CREATE POLICY "service role bypass" ON public.psa_campaign_push_queue
            TO service_role USING (true) WITH CHECK (true);
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE public.psa_campaign_push_queue FROM anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE public.psa_campaign_push_queue FROM authenticated;
    END IF;
END
$$;
