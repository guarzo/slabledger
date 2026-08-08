-- Rollback: recreate ai_calls, scoring_data_gaps and the two ai_calls views
-- (structure only -- the dropped rows are not restored).
--
-- Column definitions are the originals from 000001_initial_schema. Two things
-- are deliberately NOT the 000001 state:
--
--   Indexes. 000003's Fix 5 dropped idx_ai_calls_timestamp, idx_ai_calls_operation
--   and idx_scoring_gaps_factor as unused per Supabase telemetry. 000003 runs
--   before this migration, so recreating them here would leave a rolled-back
--   database with three indexes the head schema does not have. Only
--   idx_scoring_gaps_recorded survived, and only it comes back.
--
--   RLS. 000003 created both policies with no TO clause, which defaults to
--   TO PUBLIC and therefore admits anon and authenticated; 000028 rewrote them
--   TO service_role and revoked the default grants. Restoring 000003's looser
--   statement would reopen the hole 000028 closed.
--
-- The views are recreated WITH (security_invoker = true), as 000003 left them,
-- and are revoked from anon and authenticated alongside the tables. 000028
-- listed both views among its targets for exactly this reason: a view holds no
-- rows of its own but is still a separate grantable object, and Supabase's
-- default privileges grant on it. A view restored without the REVOKE would be
-- readable by anon while the table underneath stays locked.
--
-- Role-dependent statements are guarded on pg_roles so this also applies to a
-- local Postgres, where Supabase's roles do not exist. There, the tables end up
-- with RLS enabled and no policies, which is the same effective deny.

CREATE TABLE IF NOT EXISTS ai_calls (
    id                  BIGSERIAL PRIMARY KEY,
    operation           TEXT NOT NULL CHECK(operation IN (
        'digest', 'campaign_analysis', 'liquidation',
        'purchase_assessment', 'social_caption', 'social_suggestion'
    )),                                                                  -- AICallRecord.Operation (named string)
    status              TEXT NOT NULL CHECK(status IN ('success', 'error', 'rate_limited')),
    error_message       TEXT DEFAULT '',                                 -- AICallRecord.ErrorMessage string
    latency_ms          BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.LatencyMS int64
    tool_rounds         BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.ToolRounds int
    input_tokens        BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.InputTokens int
    output_tokens       BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.OutputTokens int
    total_tokens        BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.TotalTokens int
    timestamp           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,             -- AICallRecord.Timestamp time.Time
    cost_estimate_cents BIGINT NOT NULL DEFAULT 0                        -- AICallRecord.CostEstimateCents int
);

CREATE TABLE IF NOT EXISTS scoring_data_gaps (
    id           BIGSERIAL PRIMARY KEY,
    factor_name  TEXT NOT NULL,                                        -- GapRecord.FactorName string
    reason       TEXT NOT NULL,                                        -- GapRecord.Reason string
    entity_type  TEXT NOT NULL,                                        -- GapRecord.EntityType string
    entity_id    TEXT NOT NULL,                                        -- GapRecord.EntityID string
    card_name    TEXT NOT NULL DEFAULT '',                             -- GapRecord.CardName string
    set_name     TEXT NOT NULL DEFAULT '',                             -- GapRecord.SetName string
    recorded_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP          -- GapRecord.RecordedAt time.Time
);

-- Post-000003 index set: the two ai_calls indexes and idx_scoring_gaps_factor
-- stay dropped.
CREATE INDEX IF NOT EXISTS idx_scoring_gaps_recorded ON scoring_data_gaps(recorded_at);

-- The views, exactly as 000003 left them.
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

ALTER TABLE public.ai_calls ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS "service role bypass" ON public.ai_calls;

ALTER TABLE public.scoring_data_gaps ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS "service role bypass" ON public.scoring_data_gaps;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
        CREATE POLICY "service role bypass" ON public.ai_calls
            TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.scoring_data_gaps
            TO service_role USING (true) WITH CHECK (true);
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON public.ai_calls FROM anon;
        REVOKE ALL ON public.scoring_data_gaps FROM anon;
        REVOKE ALL ON public.ai_usage_summary FROM anon;
        REVOKE ALL ON public.ai_usage_by_operation FROM anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON public.ai_calls FROM authenticated;
        REVOKE ALL ON public.scoring_data_gaps FROM authenticated;
        REVOKE ALL ON public.ai_usage_summary FROM authenticated;
        REVOKE ALL ON public.ai_usage_by_operation FROM authenticated;
    END IF;
END
$$;
