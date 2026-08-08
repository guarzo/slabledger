-- Rollback: recreate marketmovers_config (structure only -- the discarded
-- credential row is not restored).
--
-- The column definitions are the originals from 000001_initial_schema. The RLS
-- shape is the post-000028 one -- policy scoped TO service_role, anon and
-- authenticated revoked -- not 000003's TO PUBLIC original, because 000028 runs
-- before this migration and reverting to the looser shape would reopen the hole
-- 000028 closed. Role-dependent statements are guarded on pg_roles so this also
-- applies to a local Postgres, where Supabase's roles do not exist.

CREATE TABLE IF NOT EXISTS marketmovers_config (
    id                       BIGINT PRIMARY KEY CHECK (id = 1),
    username                 TEXT NOT NULL DEFAULT '',                  -- MarketMoversConfig.Username string
    encrypted_refresh_token  TEXT NOT NULL DEFAULT '',                  -- encrypted MarketMoversConfig.RefreshToken
    updated_at               TEXT NOT NULL DEFAULT ''                   -- stored as text per SQLite history
);

ALTER TABLE public.marketmovers_config ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS "service role bypass" ON public.marketmovers_config;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
        CREATE POLICY "service role bypass" ON public.marketmovers_config
            TO service_role USING (true) WITH CHECK (true);
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON public.marketmovers_config FROM anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON public.marketmovers_config FROM authenticated;
    END IF;
END
$$;
