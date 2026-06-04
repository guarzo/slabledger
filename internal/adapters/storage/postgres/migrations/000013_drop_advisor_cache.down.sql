-- Rollback: restore the advisor_cache table to its pre-000013 state,
-- including the updated_at trigger + function (from 000001) and the
-- RLS policy (from 000003), so version-12 is faithfully reproduced.

CREATE TABLE IF NOT EXISTS advisor_cache (
    id            BIGSERIAL PRIMARY KEY,
    analysis_type TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    content       TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TEXT DEFAULT NULL,
    completed_at  TEXT DEFAULT NULL,
    created_at    TEXT NOT NULL DEFAULT (NOW()::text),
    updated_at    TEXT NOT NULL DEFAULT (NOW()::text)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_advisor_cache_type ON advisor_cache(analysis_type);

-- updated_at trigger (from 000001_initial_schema.up.sql)
CREATE OR REPLACE FUNCTION trg_advisor_cache_updated_at_fn() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := (NOW())::text;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_advisor_cache_updated_at ON advisor_cache;
CREATE TRIGGER trg_advisor_cache_updated_at
BEFORE UPDATE ON advisor_cache
FOR EACH ROW
EXECUTE FUNCTION trg_advisor_cache_updated_at_fn();

-- RLS policy (from 000003_supabase_security_and_perf_fixes.up.sql)
ALTER TABLE public.advisor_cache ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS "service role bypass" ON public.advisor_cache;
CREATE POLICY "service role bypass" ON public.advisor_cache USING (true) WITH CHECK (true);
