-- Restore CHECK constraints on source/provider columns.
-- price_history is restored to the 000036 schema (with justtcg+doubleholo CHECK).
-- Other tables are restored to the 000028 schema.

-- ── 0. price_history ─────────────────────────────────────────────────────────

DROP VIEW IF EXISTS stale_prices;

CREATE TABLE price_history_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    confidence REAL DEFAULT 1.0,
    source TEXT NOT NULL CHECK(source IN (
        'pricecharting', 'cardhedger', 'fusion', 'justtcg', 'doubleholo'
    )),
    fusion_source_count INTEGER,
    fusion_outliers_removed INTEGER,
    fusion_method TEXT,
    price_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, card_number, grade, source, price_date)
);

INSERT INTO price_history_new
    SELECT id, card_name, set_name, card_number, grade, price_cents, confidence,
           source, fusion_source_count, fusion_outliers_removed, fusion_method,
           price_date, created_at, updated_at
    FROM price_history
    WHERE source IN ('pricecharting', 'cardhedger', 'fusion', 'justtcg', 'doubleholo');

DROP TABLE price_history;
ALTER TABLE price_history_new RENAME TO price_history;

CREATE INDEX idx_price_history_card ON price_history(card_name, set_name, grade);
CREATE INDEX idx_price_history_staleness ON price_history(source, updated_at DESC);
CREATE INDEX idx_price_history_date ON price_history(price_date DESC);
CREATE INDEX idx_price_history_lookup ON price_history(card_name, set_name, card_number, grade, source, price_date DESC);

CREATE VIEW stale_prices AS
WITH recent_access AS (
    SELECT DISTINCT card_name, set_name, card_number
    FROM card_access_log
    WHERE accessed_at > DATETIME('now', '-24 hours')
)
SELECT
    ph.card_name,
    ph.card_number,
    ph.set_name,
    ph.grade,
    ph.source,
    ph.price_cents,
    ph.price_date,
    ph.updated_at,
    ROUND((JULIANDAY('now') - JULIANDAY(ph.updated_at)) * 24, 1) as hours_old,
    CASE
        WHEN ph.price_cents > 10000 THEN 1
        WHEN ph.price_cents > 5000 THEN 2
        ELSE 3
    END as priority,
    CASE WHEN ra.card_name IS NOT NULL THEN 1 ELSE 0 END as recently_accessed,
    COALESCE(cp.psa_listing_title, '') as psa_listing_title
FROM price_history ph
LEFT JOIN recent_access ra ON ra.card_name = ph.card_name AND ra.set_name = ph.set_name AND ra.card_number = ph.card_number
LEFT JOIN (
    SELECT card_name, card_number, set_name, psa_listing_title,
           ROW_NUMBER() OVER (PARTITION BY card_name, card_number, set_name ORDER BY created_at DESC) as rn
    FROM campaign_purchases
    WHERE psa_listing_title != ''
) cp ON cp.card_name = ph.card_name AND cp.set_name = ph.set_name AND cp.card_number = ph.card_number AND cp.rn = 1
WHERE
    (ph.price_cents > 10000 AND ph.updated_at < DATETIME('now', '-12 hours'))
    OR (ph.price_cents > 5000 AND ph.price_cents <= 10000 AND ph.updated_at < DATETIME('now', '-24 hours'))
    OR (ph.price_cents <= 5000 AND ph.updated_at < DATETIME('now', '-48 hours'));

-- ── 1. api_calls ─────────────────────────────────────────────────────────────

DROP VIEW IF EXISTS api_daily_summary;
DROP VIEW IF EXISTS api_hourly_distribution;
DROP VIEW IF EXISTS api_usage_summary;

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

INSERT INTO api_calls_new SELECT * FROM api_calls
    WHERE provider IN ('pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion', 'justtcg', 'doubleholo');
DROP TABLE api_calls;
ALTER TABLE api_calls_new RENAME TO api_calls;

CREATE INDEX idx_api_calls_provider ON api_calls(provider, timestamp DESC);
CREATE INDEX idx_api_calls_timestamp ON api_calls(timestamp DESC);
CREATE INDEX idx_api_calls_errors ON api_calls(provider, status_code) WHERE status_code >= 400;

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

-- ── 2. price_refresh_queue (child — drop first) ───────────────────────────────

CREATE TABLE price_refresh_queue_save AS SELECT * FROM price_refresh_queue;
DROP TABLE price_refresh_queue;

-- ── 3. api_rate_limits ────────────────────────────────────────────────────────

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

INSERT INTO api_rate_limits_new SELECT * FROM api_rate_limits
    WHERE provider IN ('pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion', 'justtcg', 'doubleholo');
DROP TABLE api_rate_limits;
ALTER TABLE api_rate_limits_new RENAME TO api_rate_limits;

-- ── 4. price_refresh_queue (recreate with original source CHECK) ─────────────

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

INSERT INTO price_refresh_queue SELECT * FROM price_refresh_queue_save
    WHERE source IN ('pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion', 'justtcg', 'doubleholo');
DROP TABLE price_refresh_queue_save;

CREATE INDEX idx_refresh_queue_priority ON price_refresh_queue(priority ASC, scheduled_at ASC)
    WHERE status = 'pending';
CREATE INDEX idx_refresh_queue_status ON price_refresh_queue(status, last_attempted_at);
