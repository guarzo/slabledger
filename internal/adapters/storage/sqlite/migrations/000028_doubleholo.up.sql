-- Market intelligence from DoubleHolo Tier 3
CREATE TABLE market_intelligence (
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    dh_card_id TEXT NOT NULL,
    sentiment_score REAL,
    sentiment_mentions INTEGER,
    sentiment_trend TEXT,
    forecast_price_cents INTEGER,
    forecast_confidence REAL,
    forecast_date TEXT,
    grading_roi TEXT,
    recent_sales TEXT,
    population TEXT,
    insights_headline TEXT,
    insights_detail TEXT,
    fetched_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, card_number)
);

CREATE INDEX idx_market_intelligence_dh_card_id ON market_intelligence(dh_card_id);
CREATE INDEX idx_market_intelligence_fetched_at ON market_intelligence(fetched_at);

-- Daily buy/sell suggestions from DoubleHolo
CREATE TABLE dh_suggestions (
    suggestion_date TEXT NOT NULL,
    type TEXT NOT NULL,
    category TEXT NOT NULL,
    rank INTEGER NOT NULL,
    is_manual BOOLEAN NOT NULL,
    dh_card_id TEXT NOT NULL,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    image_url TEXT,
    current_price_cents INTEGER,
    confidence_score REAL,
    reasoning TEXT,
    structured_reasoning TEXT,
    metrics TEXT,
    sentiment_score REAL,
    sentiment_trend REAL,
    sentiment_mentions INTEGER,
    fetched_at TIMESTAMP NOT NULL,
    PRIMARY KEY (suggestion_date, type, category, rank)
);

CREATE INDEX idx_dh_suggestions_date ON dh_suggestions(suggestion_date);
CREATE INDEX idx_dh_suggestions_card ON dh_suggestions(card_name, set_name);

-- Add 'doubleholo' to provider CHECK constraints on api_calls, api_rate_limits, and price_refresh_queue.
-- SQLite cannot ALTER CHECK constraints, so we recreate the tables (same approach as 000026).

-- Drop views first (they depend on api_calls)
DROP VIEW IF EXISTS api_daily_summary;
DROP VIEW IF EXISTS api_hourly_distribution;
DROP VIEW IF EXISTS api_usage_summary;

-- 1. api_calls: recreate with updated CHECK constraint
CREATE TABLE api_calls_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL CHECK(provider IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion', 'justtcg', 'doubleholo'
    )),
    endpoint TEXT,
    status_code INTEGER,
    error TEXT,
    latency_ms INTEGER,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO api_calls_new SELECT * FROM api_calls;
DROP TABLE api_calls;
ALTER TABLE api_calls_new RENAME TO api_calls;

CREATE INDEX idx_api_calls_provider ON api_calls(provider, timestamp DESC);
CREATE INDEX idx_api_calls_timestamp ON api_calls(timestamp DESC);
CREATE INDEX idx_api_calls_errors ON api_calls(provider, status_code) WHERE status_code >= 400;

-- 2. price_refresh_queue: save data and drop child table before touching api_rate_limits
CREATE TABLE price_refresh_queue_save AS SELECT * FROM price_refresh_queue;
DROP TABLE price_refresh_queue;

-- 3. api_rate_limits: now safe to recreate (no FK children remain)
CREATE TABLE api_rate_limits_new (
    provider TEXT PRIMARY KEY CHECK(provider IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion', 'justtcg', 'doubleholo'
    )),
    calls_last_minute INTEGER DEFAULT 0,
    calls_last_hour INTEGER DEFAULT 0,
    calls_last_day INTEGER DEFAULT 0,
    last_429_at TIMESTAMP,
    blocked_until TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO api_rate_limits_new SELECT * FROM api_rate_limits;
DROP TABLE api_rate_limits;
ALTER TABLE api_rate_limits_new RENAME TO api_rate_limits;

-- 4. price_refresh_queue: recreate with updated CHECK and FK to new api_rate_limits
CREATE TABLE price_refresh_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    grade TEXT NOT NULL CHECK(grade IN (
        'PSA 10', 'PSA 9', 'PSA 8', 'PSA 7', 'PSA 6',
        'PSA 5', 'PSA 4', 'PSA 3', 'PSA 2', 'PSA 1',
        'BGS 10', 'BGS 9.5', 'BGS 9', 'BGS 8.5', 'BGS 8',
        'CGC 10', 'CGC 9.5', 'CGC 9', 'CGC 8.5', 'CGC 8',
        'Raw', 'Ungraded'
    )),
    source TEXT NOT NULL CHECK(source IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion', 'justtcg', 'doubleholo'
    )),
    priority INTEGER DEFAULT 2 CHECK(priority IN (1, 2, 3)),
    scheduled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_attempted_at TIMESTAMP,
    attempts INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'in_progress', 'completed', 'failed')),
    error TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, grade, source),
    FOREIGN KEY (source) REFERENCES api_rate_limits(provider) ON UPDATE CASCADE ON DELETE RESTRICT
);

INSERT INTO price_refresh_queue SELECT * FROM price_refresh_queue_save;
DROP TABLE price_refresh_queue_save;

CREATE INDEX idx_refresh_queue_priority ON price_refresh_queue(priority ASC, scheduled_at ASC)
    WHERE status = 'pending';
CREATE INDEX idx_refresh_queue_status ON price_refresh_queue(status, last_attempted_at);

-- 5. Recreate views that depend on api_calls
CREATE VIEW api_usage_summary AS
SELECT
    provider,
    COUNT(*) as total_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limit_hits,
    AVG(latency_ms) as avg_latency_ms,
    MAX(timestamp) as last_call_at,
    COUNT(CASE WHEN timestamp > DATETIME('now', '-1 hour') THEN 1 END) as calls_last_hour,
    COUNT(CASE WHEN timestamp > DATETIME('now', '-5 minutes') THEN 1 END) as calls_last_5min
FROM api_calls
WHERE timestamp > DATETIME('now', '-24 hours')
GROUP BY provider;

CREATE VIEW api_hourly_distribution AS
SELECT
    provider,
    STRFTIME('%Y-%m-%d %H:00', timestamp) as hour,
    COUNT(*) as call_count,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limit_hits
FROM api_calls
WHERE timestamp > DATETIME('now', '-7 days')
GROUP BY provider, hour
ORDER BY hour DESC;

CREATE VIEW api_daily_summary AS
SELECT
    provider,
    DATE(timestamp) as date,
    COUNT(*) as total_calls,
    COUNT(CASE WHEN status_code < 400 THEN 1 END) as successful_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limit_hits,
    ROUND(100.0 * COUNT(CASE WHEN status_code < 400 THEN 1 END) / COUNT(*), 1) as success_rate_pct,
    ROUND(AVG(latency_ms)) as avg_latency_ms,
    MIN(timestamp) as first_call,
    MAX(timestamp) as last_call
FROM api_calls
WHERE timestamp > DATETIME('now', '-7 days')
GROUP BY provider, DATE(timestamp)
ORDER BY date DESC, provider;

-- 6. Seed rate limit entry for doubleholo provider
INSERT OR IGNORE INTO api_rate_limits (provider, calls_last_minute, calls_last_hour, calls_last_day, last_429_at, blocked_until)
VALUES ('doubleholo', 0, 0, 0, NULL, NULL);
