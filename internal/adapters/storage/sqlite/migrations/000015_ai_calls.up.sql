CREATE TABLE ai_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL CHECK(operation IN (
        'digest', 'campaign_analysis', 'liquidation',
        'purchase_assessment', 'social_caption', 'social_suggestion'
    )),
    status TEXT NOT NULL CHECK(status IN ('success', 'error', 'rate_limited')),
    error_message TEXT DEFAULT '',
    latency_ms INTEGER NOT NULL DEFAULT 0,
    tool_rounds INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ai_calls_timestamp ON ai_calls(timestamp DESC);
CREATE INDEX idx_ai_calls_operation ON ai_calls(operation, timestamp DESC);

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
    STRFTIME('%Y-%m-%d %H:%M:%S', MAX(timestamp)) as last_call_at,
    COUNT(CASE WHEN timestamp > DATETIME('now', '-24 hours') THEN 1 END) as calls_last_24h
FROM ai_calls
WHERE timestamp > DATETIME('now', '-7 days');

CREATE VIEW ai_usage_by_operation AS
SELECT
    operation,
    COUNT(*) as calls,
    COUNT(CASE WHEN status = 'error' OR status = 'rate_limited' THEN 1 END) as errors,
    COALESCE(AVG(latency_ms), 0) as avg_latency_ms,
    COALESCE(SUM(total_tokens), 0) as total_tokens
FROM ai_calls
WHERE timestamp > DATETIME('now', '-7 days')
GROUP BY operation;
