ALTER TABLE ai_calls ADD COLUMN cost_estimate_cents INTEGER NOT NULL DEFAULT 0;

-- Recreate views to include cost.
DROP VIEW IF EXISTS ai_usage_summary;
CREATE VIEW ai_usage_summary AS
SELECT
    COUNT(*) as total_calls,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_calls,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_calls,
    COUNT(CASE WHEN status = 'rate_limited' THEN 1 END) as rate_limit_hits,
    COALESCE(AVG(latency_ms), 0) as avg_latency_ms,
    COALESCE(SUM(input_tokens), 0) as total_input_tokens,
    COALESCE(SUM(output_tokens), 0) as total_output_tokens,
    COALESCE(SUM(total_tokens), 0) as total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) as total_cost_cents,
    STRFTIME('%Y-%m-%d %H:%M:%S', MAX(timestamp)) as last_call_at,
    COUNT(CASE WHEN timestamp > DATETIME('now', '-24 hours') THEN 1 END) as calls_last_24h
FROM ai_calls
WHERE timestamp > DATETIME('now', '-7 days');

DROP VIEW IF EXISTS ai_usage_by_operation;
CREATE VIEW ai_usage_by_operation AS
SELECT
    operation,
    COUNT(*) as calls,
    COUNT(CASE WHEN status = 'error' OR status = 'rate_limited' THEN 1 END) as errors,
    COALESCE(AVG(latency_ms), 0) as avg_latency_ms,
    COALESCE(SUM(total_tokens), 0) as total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) as total_cost_cents
FROM ai_calls
WHERE timestamp > DATETIME('now', '-7 days')
GROUP BY operation;
